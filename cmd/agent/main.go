package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Task struct {
	ID            string  `json:"id"`
	ExpressionID  string  `json:"expression_id"`
	Arg1          string  `json:"arg1"`
	Arg2          string  `json:"arg2"`
	Operator      string  `json:"operation"`
	OperationTime int     `json:"operation_time"`
	Result        float64 `json:"result,omitempty"`
	UserID        string  `json:"user_id"`
}

type TaskResponse struct {
	Task *Task `json:"task"`
}

const (
	maxRetries     = 5
	baseRetryDelay = 2 * time.Second
)

var (
	taskMutex        sync.Mutex
	activeWorkers    int
	maxWorkers       int
	agentToken       string
	orchestratorHost string
)

func main() {
	orchestratorHost = os.Getenv("ORCHESTRATOR_HOST")
	if orchestratorHost == "" {
		orchestratorHost = "localhost"
	}

	agentToken = fetchToken()

	maxWorkers = getEnvAsInt("COMPUTING_POWER", 10)
	log.Printf("Starting agent with %d workers", maxWorkers)

	for i := 0; i < maxWorkers; i++ {
		go worker(i + 1)
	}

	select {} // блокируем main навсегда
}

// TOKEN

func fetchToken() string {
	url := fmt.Sprintf("http://%s:8080/internal/agent/token", orchestratorHost)
	log.Printf("Fetching token from %s", url)

	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Failed to connect to orchestrator: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Error while fetching token: %d - %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		log.Fatalf("Failed to parse token: %v", err)
	}

	if tokenResp.Token == "" {
		log.Fatal("Received empty token")
	}

	log.Println("Token successfully received")
	return tokenResp.Token
}

// WORKER

func worker(id int) {
	for {
		task, ok := fetchTask()
		if !ok {
			time.Sleep(1 * time.Second)
			continue
		}

		log.Printf("Worker %d: Processing task %s (%s %s %s)", id, task.ID, task.Arg1, task.Operator, task.Arg2)

		result, err := processTask(task)
		if err != nil {
			log.Printf("Worker %d: Task %s failed: %v", id, task.ID, err)
			updateWorkerCount(-1)
			continue
		}

		if err := sendResultWithRetry(task.ID, result); err != nil {
			log.Printf("Worker %d: Failed to send result: %v", id, err)
		} else {
			log.Printf("Worker %d: Result %.2f for task %s sent", id, result, task.ID)
		}

		updateWorkerCount(-1)
	}
}

func updateWorkerCount(delta int) {
	taskMutex.Lock()
	activeWorkers += delta
	taskMutex.Unlock()
}

// TASK FETCH

func fetchTask() (*Task, bool) {
	taskMutex.Lock()
	defer taskMutex.Unlock()

	if activeWorkers >= maxWorkers {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://%s:8080/internal/task", orchestratorHost)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Printf("Request creation error: %v", err)
		return nil, false
	}
	addAuthHeader(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Fetch error: %v", err)
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Server error: %d - %s", resp.StatusCode, string(body))
		return nil, false
	}

	var response TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Decode error: %v", err)
		return nil, false
	}
	if response.Task == nil {
		return nil, false
	}

	activeWorkers++
	return response.Task, true
}

// PROCESSING

func processTask(task *Task) (float64, error) {
	arg1, err := resolveArgument(task.Arg1)
	if err != nil {
		return 0, fmt.Errorf("arg1: %w", err)
	}
	arg2, err := resolveArgument(task.Arg2)
	if err != nil {
		return 0, fmt.Errorf("arg2: %w", err)
	}
	time.Sleep(time.Duration(task.OperationTime) * time.Millisecond)
	return calculate(task.Operator, arg1, arg2)
}

func resolveArgument(arg string) (float64, error) {
	if strings.HasPrefix(arg, "task:") {
		id := strings.TrimPrefix(arg, "task:")
		return fetchTaskResultWithRetry(id)
	}
	return strconv.ParseFloat(arg, 64)
}

func calculate(op string, a, b float64) (float64, error) {
	switch op {
	case "+":
		return a + b, nil
	case "-":
		return a - b, nil
	case "*":
		return a * b, nil
	case "/":
		if b == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return a / b, nil
	default:
		return 0, fmt.Errorf("unknown operator: %s", op)
	}
}

// TASK RESULT FETCH

func fetchTaskResult(id string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://%s:8080/internal/task/result/%s", orchestratorHost, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("request error: %w", err)
	}
	addAuthHeader(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result float64 `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("decode error: %w", err)
	}
	return result.Result, nil
}

func fetchTaskResultWithRetry(id string) (float64, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		res, err := fetchTaskResult(id)
		if err == nil {
			return res, nil
		}
		lastErr = err
		delay := time.Duration(1<<uint(i)) * baseRetryDelay
		log.Printf("Retry %d/%d task %s: %v", i+1, maxRetries, id, err)
		time.Sleep(delay)
	}
	return 0, fmt.Errorf("max retries for task %s: %v", id, lastErr)
}

// TASK RESULT SEND

func sendResult(taskID string, result float64) error {
	payload := struct {
		ID     string  `json:"id"`
		Result float64 `json:"result"`
	}{taskID, result}

	data, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://%s:8080/internal/task", orchestratorHost)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	addAuthHeader(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func sendResultWithRetry(taskID string, result float64) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := sendResult(taskID, result); err == nil {
			return nil
		} else {
			lastErr = err
		}
		delay := time.Duration(1<<uint(i)) * baseRetryDelay
		log.Printf("Retry %d/%d sending result for task %s: %v", i+1, maxRetries, taskID, lastErr)
		time.Sleep(delay)
	}
	return fmt.Errorf("max retries for sending result %s: %v", taskID, lastErr)
}

// UTILS

func addAuthHeader(req *http.Request) {
	if agentToken != "" {
		req.Header.Set("Authorization", "Bearer "+agentToken)
	}
}

func getEnvAsInt(key string, def int) int {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		log.Printf("Invalid value for %s, using default %d", key, def)
		return def
	}
	return n
}

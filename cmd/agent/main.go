package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"calc-service/pkg/config"
)

type Task struct {
	ID            string  `json:"id"`
	ExpressionID  string  `json:"expression_id"`
	Arg1          string  `json:"arg1"`
	Arg2          string  `json:"arg2"`
	Operator      string  `json:"operation"`
	OperationTime int     `json:"operation_time"`
	Result        float64 `json:"result"`
	UserID        string  `json:"user_id"`
}

type TaskResponse struct {
	Task *Task `json:"task"`
}

type agent struct {
	cfg    *config.Config
	client *http.Client

	mu    sync.RWMutex
	token string
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	a := &agent{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
	if err := a.refreshToken(context.Background()); err != nil {
		log.Fatalf("cannot obtain agent token: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup
	workers := workerCount(cfg)
	log.Printf("starting agent with %d workers, orchestrator %s", workers, cfg.BaseURL())
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			a.worker(ctx, id)
		}(i + 1)
	}

	<-ctx.Done()
	log.Println("shutting down, waiting for workers")
	wg.Wait()
}

func workerCount(cfg *config.Config) int {
	if v := os.Getenv("COMPUTING_POWER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

// refreshToken получает токен агента по bootstrap-ключу.
func (a *agent) refreshToken(ctx context.Context) error {
	url := a.cfg.BaseURL() + "/internal/agent/token"
	var lastErr error
	for i := 0; i < a.cfg.MaxRetries; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-Bootstrap-Key", a.cfg.AgentBootstrapKey)
		resp, err := a.client.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				err = readErr
			} else if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			} else {
				var result struct {
					Token string `json:"token"`
				}
				if err = json.Unmarshal(body, &result); err == nil && result.Token != "" {
					a.mu.Lock()
					a.token = result.Token
					a.mu.Unlock()
					log.Println("agent token received")
					return nil
				}
				if err == nil {
					err = fmt.Errorf("empty token")
				}
			}
		}
		lastErr = err
		if i < a.cfg.MaxRetries-1 {
			delay := retryDelay(a.cfg, i)
			log.Printf("token fetch retry %d/%d in %s: %v", i+1, a.cfg.MaxRetries, delay, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return fmt.Errorf("fetch agent token: %w", lastErr)
}

func (a *agent) authToken() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.token
}

// doRequest выполняет запрос и обновляет токен при 401.
func (a *agent) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	do := func() (*http.Response, error) {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, a.cfg.BaseURL()+path, reader)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+a.authToken())
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		return a.client.Do(req)
	}

	resp, err := do()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		if err := a.refreshToken(ctx); err != nil {
			return nil, err
		}
		return do()
	}
	return resp, nil
}

func (a *agent) worker(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		task, err := a.fetchTask(ctx)
		if err != nil {
			log.Printf("worker %d: fetch task: %v", id, err)
			if !a.pause(ctx, a.cfg.TaskPollInterval) {
				return
			}
			continue
		}
		if task == nil {
			// Очередь пуста — ждём, но после реальной задачи сразу идём за новой.
			if !a.pause(ctx, a.cfg.TaskPollInterval) {
				return
			}
			continue
		}
		a.process(ctx, id, task)
	}
}

// pause ждёт d или завершения контекста. Возвращает false при остановке.
func (a *agent) pause(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func (a *agent) fetchTask(ctx context.Context) (*Task, error) {
	resp, err := a.doRequest(ctx, http.MethodGet, "/internal/task", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode task: %w", err)
	}
	return result.Task, nil
}

func (a *agent) process(ctx context.Context, id int, task *Task) {
	log.Printf("worker %d: task %s (%s %s %s)", id, task.ID, task.Arg1, task.Operator, task.Arg2)
	result, err := a.compute(ctx, task)
	if err != nil {
		log.Printf("worker %d: task %s failed: %v", id, task.ID, err)
		a.reportFailure(ctx, task.ID, err.Error())
		return
	}
	if err := a.withRetry(ctx, func() error { return a.sendResult(ctx, task.ID, result) }); err != nil {
		log.Printf("worker %d: cannot send result for %s: %v", id, task.ID, err)
		return
	}
	log.Printf("worker %d: result %.4f for %s sent", id, result, task.ID)
}

func (a *agent) compute(ctx context.Context, task *Task) (float64, error) {
	arg1, err := a.resolveArgument(ctx, task.Arg1)
	if err != nil {
		return 0, fmt.Errorf("arg1: %w", err)
	}
	arg2, err := a.resolveArgument(ctx, task.Arg2)
	if err != nil {
		return 0, fmt.Errorf("arg2: %w", err)
	}
	if task.OperationTime < 0 || task.OperationTime > a.cfg.MaxOperationTimeMS {
		return 0, fmt.Errorf("invalid operation time %d", task.OperationTime)
	}
	if task.OperationTime > 0 {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Duration(task.OperationTime) * time.Millisecond):
		}
	}
	return calculate(task.Operator, arg1, arg2)
}

func (a *agent) resolveArgument(ctx context.Context, arg string) (float64, error) {
	if strings.HasPrefix(arg, "task:") {
		id := strings.TrimPrefix(arg, "task:")
		var value float64
		err := a.withRetry(ctx, func() error {
			resp, err := a.doRequest(ctx, http.MethodGet, "/internal/task/result/"+id, nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			}
			var result struct {
				Result float64 `json:"result"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return err
			}
			value = result.Result
			return nil
		})
		return value, err
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
		return 0, fmt.Errorf("unknown operator %q", op)
	}
}

func (a *agent) sendResult(ctx context.Context, taskID string, result float64) error {
	payload, err := json.Marshal(struct {
		ID     string  `json:"id"`
		Result float64 `json:"result"`
	}{taskID, result})
	if err != nil {
		return err
	}
	resp, err := a.doRequest(ctx, http.MethodPost, "/internal/task", payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (a *agent) reportFailure(ctx context.Context, taskID, errText string) {
	payload, err := json.Marshal(struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}{taskID, errText})
	if err != nil {
		return
	}
	err = a.withRetry(ctx, func() error {
		resp, err := a.doRequest(ctx, http.MethodPost, "/internal/task/fail", payload)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return nil
	})
	if err != nil {
		log.Printf("cannot report failure for %s: %v", taskID, err)
	}
}

func (a *agent) withRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	for i := 0; i < a.cfg.MaxRetries; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i == a.cfg.MaxRetries-1 {
			break
		}
		delay := retryDelay(a.cfg, i)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

func retryDelay(cfg *config.Config, attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 10 {
		attempt = 10
	}
	delay := cfg.RetryBaseDelay * time.Duration(1<<uint(attempt))
	max := 30 * time.Second
	if delay > max {
		return max
	}
	return delay
}

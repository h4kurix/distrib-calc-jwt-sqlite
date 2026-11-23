package handler

import (
	"calc-service/internal/calculator"
	"calc-service/internal/store"
	"calc-service/pkg/logger"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type TaskResponse struct {
	Task *store.Task `json:"task"`
}

type TaskResultRequest struct {
	ID     string  `json:"id"`
	Result float64 `json:"result"`
}

const staleTaskTimeout = 2 * time.Minute

// TaskHandler обрабатывает выдачу задач и приём результатов.
func TaskHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetTask(w)
	case http.MethodPost:
		handlePostTaskResult(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetTask(w http.ResponseWriter) {
	task, found, err := store.ClaimNextTask()
	if err != nil {
		logger.Error("claim task: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, TaskResponse{Task: task})
}

func handlePostTaskResult(w http.ResponseWriter, r *http.Request) {
	var req TaskResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	task, found, err := store.GetTask(req.ID)
	if err != nil {
		logger.Error("get task: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	if err := store.CompleteTask(req.ID, req.Result); err != nil {
		logger.Error("complete task: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := finalizeExpression(task.ExpressionID, task.UserID); err != nil {
		logger.Error("finalize expression %s: %v", task.ExpressionID, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// finalizeExpression обновляет статус выражения после завершения задачи.
func finalizeExpression(exprID, userID string) error {
	failed, err := store.CountFailedTasks(exprID)
	if err != nil {
		return err
	}
	if failed > 0 {
		return store.UpdateExpressionStatus(exprID, "error", 0, "one or more tasks failed")
	}

	incomplete, err := store.CountIncompleteTasks(exprID)
	if err != nil {
		return err
	}
	if incomplete > 0 {
		expr, found, err := store.GetExpression(exprID)
		if err != nil {
			return err
		}
		if found && expr.Status == "pending" {
			return store.UpdateExpressionStatus(exprID, "in_progress", 0, "")
		}
		return nil
	}

	tasks, err := store.GetTasksByExpression(exprID, userID)
	if err != nil {
		return err
	}
	result, err := calculator.AggregateResults(tasks)
	if err != nil {
		return store.UpdateExpressionStatus(exprID, "error", 0, err.Error())
	}
	return store.UpdateExpressionStatus(exprID, "completed", result, "")
}

// HandleTaskByID возвращает результат задачи владельцу.
func HandleTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := getUserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	task, found, err := store.GetTask(id)
	if err != nil {
		logger.Error("get task: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !found || task.UserID != userID {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	if !task.Completed {
		http.Error(w, "Task not completed", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Result float64 `json:"result"`
	}{Result: task.Result})
}

// HandleTaskFailure принимает сообщение об ошибке вычисления задачи.
func HandleTaskFailure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	task, found, err := store.GetTask(req.ID)
	if err != nil {
		logger.Error("get task: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	if err := store.FailTask(req.ID, req.Error); err != nil {
		logger.Error("fail task %s: %v", req.ID, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err := finalizeExpression(task.ExpressionID, task.UserID); err != nil {
		logger.Error("finalize expression %s after failure: %v", task.ExpressionID, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// HandleInternalTaskByID.return result for internal clients.
func HandleInternalTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/internal/task/result/")
	task, found, err := store.GetTask(id)
	if err != nil {
		logger.Error("get task: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !found || !task.Completed {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Result float64 `json:"result"`
	}{Result: task.Result})
}

// ProcessPendingTasks выполняет только обслуживание очереди. Финализация
// выражений делается в handlePostTaskResult, поэтому гонки с тикером нет.
func ProcessPendingTasks() {
	n, err := store.ReleaseStaleTasks(staleTaskTimeout)
	if err != nil {
		logger.Error("release stale tasks: %v", err)
		return
	}
	if n > 0 {
		logger.Warn("released %d stale tasks", n)
	}
}

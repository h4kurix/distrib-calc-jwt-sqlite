package handler

import (
	"net/http"
	"time"

	"calc-service/internal/calculator"
	"calc-service/internal/store"
	"calc-service/pkg/logger"
)

type TaskResponse struct {
	Task *store.Task `json:"task"`
}

type TaskResultRequest struct {
	ID     string  `json:"id"`
	Result float64 `json:"result"`
}

type TaskFailureRequest struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

const (
	staleTaskTimeout  = 2 * time.Minute
	expressionTimeout = 10 * time.Minute
)

// TaskHandler обрабатывает выдачу задач и приём результатов.
func TaskHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetTask(w)
	case http.MethodPost:
		handlePostTaskResult(w, r)
	default:
		writeMethodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func handleGetTask(w http.ResponseWriter) {
	task, found, err := store.ClaimNextTask()
	if err != nil {
		logAndFail(w, "claim task", err)
		return
	}
	if !found {
		// Пустая очередь — это не ошибка: агент просто повторит запрос.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, TaskResponse{Task: task})
}

func handlePostTaskResult(w http.ResponseWriter, r *http.Request) {
	var req TaskResultRequest
	if err := decodeJSONBody(w, r, &req); err != nil || req.ID == "" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	task, found, err := store.GetTask(req.ID)
	if err != nil {
		logAndFail(w, "get task", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err := store.CompleteTask(req.ID, req.Result); err != nil {
		logAndFail(w, "complete task", err)
		return
	}

	if err := finalizeExpression(task.ExpressionID, task.UserID); err != nil {
		logAndFail(w, "finalize expression "+task.ExpressionID, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// finalizeExpression обновляет статус выражения в одной транзакции.
func finalizeExpression(exprID, userID string) error {
	return store.FinalizeExpression(exprID, userID, calculator.AggregateResults)
}

// HandleTaskByID возвращает результат задачи владельцу.
func HandleTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	userID := getUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, ok := pathID(r.URL.Path, "/api/v1/tasks/")
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	task, found, err := store.GetTask(id)
	if err != nil {
		logAndFail(w, "get task", err)
		return
	}
	if !found || task.UserID != userID {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if !task.Completed {
		writeError(w, http.StatusNotFound, "task not completed")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Result float64 `json:"result"`
	}{Result: task.Result})
}

// HandleTaskFailure принимает сообщение об ошибке вычисления задачи.
func HandleTaskFailure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	var req TaskFailureRequest
	if err := decodeJSONBody(w, r, &req); err != nil || req.ID == "" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	task, found, err := store.GetTask(req.ID)
	if err != nil {
		logAndFail(w, "get task", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err := store.FailTask(req.ID, req.Error); err != nil {
		logAndFail(w, "fail task "+req.ID, err)
		return
	}
	if err := finalizeExpression(task.ExpressionID, task.UserID); err != nil {
		logAndFail(w, "finalize expression "+task.ExpressionID, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// HandleInternalTaskByID отдаёт результат задачи агенту по внутреннему API.
func HandleInternalTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	id, ok := pathID(r.URL.Path, "/internal/task/result/")
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	task, found, err := store.GetTask(id)
	if err != nil {
		logAndFail(w, "get task", err)
		return
	}
	if !found || !task.Completed {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Result float64 `json:"result"`
	}{Result: task.Result})
}

// ProcessPendingTasks выполняет обслуживание очереди: возвращает зависшие
// задачи и закрывает выражения, которые уже не могут завершиться.
func ProcessPendingTasks() {
	n, err := store.ReleaseStaleTasks(staleTaskTimeout)
	if err != nil {
		logger.Error("release stale tasks: %v", err)
		return
	}
	if n > 0 {
		logger.Warn("released %d stale tasks", n)
	}

	expired, err := store.ExpireStaleExpressions(expressionTimeout)
	if err != nil {
		logger.Error("expire stale expressions: %v", err)
		return
	}
	if expired > 0 {
		logger.Warn("expired %d stale expressions", expired)
	}
}

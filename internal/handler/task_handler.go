package handler

import (
	"context"
	"net/http"

	"calc-service/internal/calculator"
	"calc-service/internal/logger"
	"calc-service/internal/store"
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

// TaskHandler обрабатывает выдачу задач и приём результатов.
func (d *Deps) TaskHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.handleGetTask(w, r.Context())
	case http.MethodPost:
		d.handlePostTaskResult(w, r)
	default:
		writeMethodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (d *Deps) handleGetTask(w http.ResponseWriter, ctx context.Context) {
	task, found, err := d.Store.ClaimNextTask(ctx)
	if err != nil {
		logAndFail(w, "claim task", err)
		return
	}
	if !found {
		// Пустая очередь — не ошибка: агент просто повторит запрос.
		// Тело остаётся JSON, как у всех остальных ответов API.
		writeError(w, http.StatusNotFound, "queue empty")
		return
	}
	writeJSON(w, http.StatusOK, TaskResponse{Task: task})
}

func (d *Deps) handlePostTaskResult(w http.ResponseWriter, r *http.Request) {
	var req TaskResultRequest
	if err := decodeJSONBody(w, r, &req); err != nil || req.ID == "" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	task, found, err := d.Store.GetTask(r.Context(), req.ID)
	if err != nil {
		logAndFail(w, "get task", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err := d.Store.CompleteTask(r.Context(), req.ID, req.Result); err != nil {
		logAndFail(w, "complete task", err)
		return
	}

	if err := d.finalizeExpression(r.Context(), task.ExpressionID, task.UserID); err != nil {
		logAndFail(w, "finalize expression "+task.ExpressionID, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// finalizeExpression обновляет статус выражения в одной транзакции.
func (d *Deps) finalizeExpression(ctx context.Context, exprID, userID string) error {
	return d.Store.FinalizeExpression(ctx, exprID, userID, calculator.AggregateResults)
}

// HandleTaskByID возвращает результат задачи владельцу.
func (d *Deps) HandleTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	userID := getUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	task, found, err := d.Store.GetTask(r.Context(), id)
	if err != nil {
		logAndFail(w, "get task", err)
		return
	}
	if !found || task.UserID != userID {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if task.Status != store.StatusCompleted {
		writeError(w, http.StatusNotFound, "task not completed")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Result float64 `json:"result"`
	}{Result: task.Result})
}

// HandleTaskFailure принимает сообщение об ошибке вычисления задачи.
func (d *Deps) HandleTaskFailure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	var req TaskFailureRequest
	if err := decodeJSONBody(w, r, &req); err != nil || req.ID == "" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	task, found, err := d.Store.GetTask(r.Context(), req.ID)
	if err != nil {
		logAndFail(w, "get task", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err := d.Store.FailTask(r.Context(), req.ID, req.Error); err != nil {
		logAndFail(w, "fail task "+req.ID, err)
		return
	}
	if err := d.finalizeExpression(r.Context(), task.ExpressionID, task.UserID); err != nil {
		logAndFail(w, "finalize expression "+task.ExpressionID, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// HandleInternalTaskByID отдаёт результат задачи агенту по внутреннему API.
func (d *Deps) HandleInternalTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	task, found, err := d.Store.GetTask(r.Context(), id)
	if err != nil {
		logAndFail(w, "get task", err)
		return
	}
	if !found || task.Status != store.StatusCompleted {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Result float64 `json:"result"`
	}{Result: task.Result})
}

// ProcessPendingTasks выполняет обслуживание очереди: возвращает зависшие
// задачи и закрывает выражения, которые уже не могут завершиться.
func (d *Deps) ProcessPendingTasks(ctx context.Context) {
	n, err := d.Store.ReleaseStaleTasks(ctx, d.staleTaskTimeout)
	if err != nil {
		logger.Error("release stale tasks: %v", err)
		return
	}
	if n > 0 {
		logger.Warn("released %d stale tasks", n)
	}

	expired, err := d.Store.ExpireStaleExpressions(ctx, d.expressionExpiry)
	if err != nil {
		logger.Error("expire stale expressions: %v", err)
		return
	}
	if expired > 0 {
		logger.Warn("expired %d stale expressions", expired)
	}
}

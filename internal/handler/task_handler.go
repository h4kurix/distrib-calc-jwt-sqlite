package handler

import (
	"calc-service/internal/calculator"
	"calc-service/internal/store"
	"calc-service/pkg/logger"
	"encoding/json"
	"net/http"
	"strings"
)

type TaskResponse struct {
	Task *store.Task `json:"task"`
}

type TaskResultRequest struct {
	ID     string  `json:"id"`
	Result float64 `json:"result"`
}

// TaskHandler handles getting executable tasks and posting task results
func TaskHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetTask(w, r)
	case http.MethodPost:
		handlePostTaskResult(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetTask returns an executable task from any user
func handleGetTask(w http.ResponseWriter, _ *http.Request) {
	task, found := store.GetNextExecutableTask()
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	response := TaskResponse{Task: task}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error("Failed to encode task: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handlePostTaskResult updates a task with its result and rolls up expression status
func handlePostTaskResult(w http.ResponseWriter, r *http.Request) {
	var req TaskResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("Failed to decode task result: %v", err)
		http.Error(w, "Invalid request body", http.StatusUnprocessableEntity)
		return
	}

	task, exists := store.GetTask(req.ID)
	if !exists {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	if err := store.CompleteTask(req.ID, req.Result); err != nil {
		logger.Error("Failed to complete task: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// апдейтим статус выражения
	exprID := task.ExpressionID
	remaining, err := store.CountIncompleteTasks(exprID)
	if err != nil {
		logger.Error("CountIncompleteTasks: %v", err)
	} else if remaining == 0 {
		// все таски готовы → completed, сохраняем финальный результат
		if err := store.UpdateExpressionStatus(exprID, "completed", req.Result); err != nil {
			logger.Error("UpdateExpressionStatus to completed: %v", err)
		}
	} else {
		// первый результат пришёл → in_progress
		if err := store.UpdateExpressionStatus(exprID, "in_progress", 0); err != nil {
			logger.Error("UpdateExpressionStatus to in_progress: %v", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// HandleTaskByID gets a completed task by ID
func HandleTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get userID from context for authorization
	userID := getUserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	task, exists := store.GetTask(id)
	if !exists {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	// Verify this task belongs to the authenticated user
	if task.UserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if !task.Completed {
		http.Error(w, "Task not completed", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(struct {
		Result float64 `json:"result"`
	}{Result: task.Result})
}

// HandleInternalTaskByID возвращает результат задачи любому внутреннему клиенту
func HandleInternalTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/internal/task/result/")
	task, exists := store.GetTask(id)
	if !exists || !task.Completed {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Result float64 `json:"result"`
	}{Result: task.Result})
}

// ProcessPendingTasks processes all pending tasks from all users
func ProcessPendingTasks() {
	// Get all users with pending expressions
	userIDs, err := store.GetUsersWithPendingExpressions()
	if err != nil {
		logger.Error("ProcessPendingTasks: Failed to get users with pending expressions: %v", err)
		return
	}

	// Process tasks for each user
	for _, userID := range userIDs {
		// Check for executable tasks and update expressions
		ProcessUserTasks(userID)
	}
}

// ProcessUserTasks processes tasks for a specific user
func ProcessUserTasks(userID string) {
	expressions := store.ListExpressions(userID)

	for _, expr := range expressions {
		if expr.Status != "pending" && expr.Status != "in_progress" {
			continue
		}

		// Проверяем executable таски и их выполнение
		executableTasks, err := store.GetExecutableTasks(expr.ID, userID)
		if err != nil {
			logger.Error("ProcessUserTasks: Failed to get executable tasks for expr %s: %v", expr.ID, err)
			continue
		}

		// Есть ли таски для выполнения?
		if len(executableTasks) == 0 {
			// Проверяем, остались ли незавершенные таски в принципе
			incomplete, err := store.CountIncompleteTasks(expr.ID)
			if err != nil {
				logger.Error("ProcessUserTasks: CountIncompleteTasks failed: %v", err)
				continue
			}

			// Если всё завершено, финализируем результат
			if incomplete == 0 {
				tasks, err := store.GetTasksByExpression(expr.ID, userID)
				if err != nil {
					logger.Error("ProcessUserTasks: GetTasksByExpression failed: %v", err)
					continue
				}

				result, err := calculator.AggregateResults(tasks)
				if err != nil {
					logger.Error("ProcessUserTasks: AggregateResults failed: %v", err)
					continue
				}

				err = store.UpdateExpressionStatus(expr.ID, "completed", result)
				if err != nil {
					logger.Error("ProcessUserTasks: UpdateExpressionStatus failed: %v", err)
				}
			}
		} else if expr.Status == "pending" {
			// Если есть executable таски и статус был pending, меняем на in_progress
			err = store.UpdateExpressionStatus(expr.ID, "in_progress", 0)
			if err != nil {
				logger.Error("ProcessUserTasks: UpdateExpressionStatus to in_progress failed: %v", err)
			}
		}
	}
}

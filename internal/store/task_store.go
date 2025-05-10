package store

import (
	"calc-service/pkg/database"
	"calc-service/pkg/logger"
	"database/sql"
	"fmt"
)

// Task represents an atomic calculation operation
type Task struct {
	ID            string  `json:"id"`
	ExpressionID  string  `json:"expression_id"`
	Arg1          string  `json:"arg1"`
	Arg2          string  `json:"arg2"`
	Operator      string  `json:"operation"`
	OperationTime int     `json:"operation_time"`
	Result        float64 `json:"result,omitempty"`
	Completed     bool    `json:"-"`
	UserID        string  `json:"user_id"`
}

// RegisterTasks ассоциирует задачи с выражением и пользователем
func RegisterTasks(exprID, userID string, tasks []*Task) error {
	return database.Transaction(func(tx *sql.Tx) error {
		for _, task := range tasks {
			_, err := tx.Exec(
				`INSERT INTO tasks (
					id, expression_id, user_id, arg1, arg2, operator, operation_time, completed
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				task.ID, exprID, userID, task.Arg1, task.Arg2, task.Operator, task.OperationTime,
				task.Completed,
			)
			if err != nil {
				return fmt.Errorf("failed to insert task %s: %w", task.ID, err)
			}
		}
		return nil
	})
}

// GetTasksByExpression возвращает все задачи для данного выражения и пользователя
func GetTasksByExpression(exprID, userID string) ([]*Task, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT 
			id, expression_id, user_id, arg1, arg2, operator, operation_time, 
			COALESCE(result, 0), completed 
		FROM tasks 
		WHERE expression_id = ? AND user_id = ?`,
		exprID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		var task Task
		if err := rows.Scan(
			&task.ID, &task.ExpressionID, &task.UserID, &task.Arg1, &task.Arg2, &task.Operator, &task.OperationTime,
			&task.Result, &task.Completed,
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, &task)
	}

	return tasks, nil
}

// GetExecutableTasks возвращает задачи, готовые к выполнению
func GetExecutableTasks(exprID, userID string) ([]*Task, error) {
	tasks, err := GetTasksByExpression(exprID, userID)
	if err != nil {
		return nil, err
	}

	// Create lookup map for completed tasks
	completedTasks := make(map[string]bool)
	for _, task := range tasks {
		if task.Completed {
			completedTasks[task.ID] = true
		}
	}

	// Filter executable tasks (not completed and all dependencies resolved)
	var executableTasks []*Task
	for _, task := range tasks {
		if task.Completed {
			continue
		}

		// Check if dependencies are resolved
		arg1Ready := !isTaskReference(task.Arg1) || isTaskCompleted(task.Arg1[5:], completedTasks)
		arg2Ready := !isTaskReference(task.Arg2) || isTaskCompleted(task.Arg2[5:], completedTasks)

		if arg1Ready && arg2Ready {
			executableTasks = append(executableTasks, task)
		}
	}

	return executableTasks, nil
}

// GetNextExecutableTask returns a task that is ready to be processed
func GetNextExecutableTask() (*Task, bool) {
	db := database.GetDB()

	// Подзапрос для получения ID задач, у которых есть зависимости от незавершенных задач
	query := `
		SELECT 
			t.id, t.expression_id, t.arg1, t.arg2, t.operator, t.operation_time, 
			COALESCE(t.result, 0), t.completed 
		FROM tasks t
		WHERE t.completed = false
		AND NOT EXISTS (
			-- Проверка зависимостей Arg1
			SELECT 1 FROM tasks t2
			WHERE t2.completed = false
			AND CONCAT('task:', t2.id) = t.arg1
		)
		AND NOT EXISTS (
			-- Проверка зависимостей Arg2
			SELECT 1 FROM tasks t2
			WHERE t2.completed = false
			AND CONCAT('task:', t2.id) = t.arg2
		)
		LIMIT 1
	`

	var task Task
	err := db.QueryRow(query).Scan(
		&task.ID, &task.ExpressionID, &task.Arg1, &task.Arg2, &task.Operator, &task.OperationTime,
		&task.Result, &task.Completed,
	)

	if err != nil {
		if err != sql.ErrNoRows {
			logger.Error("Database error in GetNextExecutableTask: %v", err)
		}
		return nil, false
	}

	return &task, true
}

// GetTask retrieves a task by ID
func GetTask(taskID string) (*Task, bool) {
	db := database.GetDB()

	var task Task
	err := db.QueryRow(
		`SELECT 
			id, expression_id, arg1, arg2, operator, operation_time, 
			COALESCE(result, 0), completed 
		FROM tasks 
		WHERE id = ?`,
		taskID,
	).Scan(
		&task.ID, &task.ExpressionID, &task.Arg1, &task.Arg2, &task.Operator, &task.OperationTime,
		&task.Result, &task.Completed,
	)

	if err != nil {
		if err != sql.ErrNoRows {
			logger.Error("Database error in GetTask: %v", err)
		}
		return nil, false
	}

	return &task, true
}

// CompleteTask marks a task as completed
func CompleteTask(taskID string, result float64) error {
	db := database.GetDB()
	_, err := db.Exec(
		"UPDATE tasks SET completed = true, result = ? WHERE id = ?",
		result, taskID,
	)
	if err != nil {
		return fmt.Errorf("CompleteTask: %w", err)
	}
	return nil
}

// Helper functions
func isTaskReference(arg string) bool {
	return len(arg) > 5 && arg[:5] == "task:"
}

func isTaskCompleted(taskID string, completedTasks map[string]bool) bool {
	return completedTasks[taskID]
}

// GetUsersWithPendingExpressions returns a list of user IDs who have pending expressions
func GetUsersWithPendingExpressions() ([]string, error) {
	db := database.GetDB()

	// Query for distinct users with pending expressions
	rows, err := db.Query(`
		SELECT DISTINCT user_id 
		FROM expressions 
		WHERE status = 'pending'
	`)
	if err != nil {
		logger.Error("Database error in GetUsersWithPendingExpressions: %v", err)
		return nil, err
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			logger.Error("Error scanning user ID: %v", err)
			continue
		}
		userIDs = append(userIDs, userID)
	}

	return userIDs, nil
}

// CountPendingTasks returns the count of all tasks that are pending
func CountPendingTasks() (int, error) {
	db := database.GetDB()

	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) 
		FROM tasks 
		WHERE completed = false
	`).Scan(&count)

	if err != nil {
		logger.Error("Database error in CountPendingTasks: %v", err)
		return 0, err
	}

	return count, nil
}

// CountIncompleteTasks возвращает число тасков для exprID, которые ещё не completed
func CountIncompleteTasks(exprID string) (int, error) {
	db := database.GetDB()
	var cnt int
	err := db.QueryRow(
		`SELECT count(*) 
        FROM tasks 
        WHERE expression_id = ? 
        AND completed = 0`,
		exprID,
	).Scan(&cnt)
	if err != nil {
		return 0, fmt.Errorf("CountIncompleteTasks: %w", err)
	}
	return cnt, nil
}

// UpdateExpressionStatus обновляет status и result в таблице expressions
func UpdateExpressionStatus(exprID, status string, result float64) error {
	db := database.GetDB()
	res, err := db.Exec(
		`UPDATE expressions
        SET status = ?, result = ?
        WHERE id = ?`,
		status, result, exprID,
	)
	if err != nil {
		return fmt.Errorf("UpdateExpressionStatus exec: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("UpdateExpressionStatus: no rows affected for exprID=%s", exprID)
	}
	return nil
}

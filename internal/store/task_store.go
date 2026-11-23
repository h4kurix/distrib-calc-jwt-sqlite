package store

import (
	"calc-service/pkg/database"
	"database/sql"
	"fmt"
	"time"
)

// Task represents an atomic calculation operation.
type Task struct {
	ID            string  `json:"id"`
	ExpressionID  string  `json:"expression_id"`
	Arg1          string  `json:"arg1"`
	Arg2          string  `json:"arg2"`
	Operator      string  `json:"operation"`
	OperationTime int     `json:"operation_time"`
	Result        float64 `json:"result"`
	Completed     bool    `json:"-"`
	Status        string  `json:"status,omitempty"`
	Attempts      int     `json:"attempts,omitempty"`
	Error         string  `json:"error,omitempty"`
	UserID        string  `json:"user_id"`
}

const taskColumns = `id, expression_id, user_id, arg1, arg2, operator, operation_time,
	COALESCE(result, 0), completed, status, attempts, COALESCE(error, '')`

func scanTask(row rowScanner) (*Task, error) {
	var t Task
	err := row.Scan(
		&t.ID, &t.ExpressionID, &t.UserID, &t.Arg1, &t.Arg2, &t.Operator,
		&t.OperationTime, &t.Result, &t.Completed, &t.Status, &t.Attempts, &t.Error,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTasksByExpression возвращает задачи в порядке создания.
func GetTasksByExpression(exprID, userID string) ([]*Task, error) {
	rows, err := database.GetDB().Query(
		`SELECT `+taskColumns+` FROM tasks
		WHERE expression_id = ? AND user_id = ?
		ORDER BY rowid ASC`, exprID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return tasks, nil
}

// ClaimNextTask атомарно захватывает одну готовую задачу.
func ClaimNextTask() (*Task, bool, error) {
	for attempt := 0; attempt < 8; attempt++ {
		var task *Task
		err := database.Transaction(func(tx *sql.Tx) error {
			row := tx.QueryRow(
				`SELECT ` + taskColumns + ` FROM tasks t
				WHERE t.status = 'pending'
				AND NOT EXISTS (
					SELECT 1 FROM tasks d
					WHERE d.status != 'completed' AND 'task:' || d.id IN (t.arg1, t.arg2)
				)
				ORDER BY t.rowid ASC LIMIT 1`,
			)
			t, err := scanTask(row)
			if err == sql.ErrNoRows {
				return nil
			}
			if err != nil {
				return fmt.Errorf("select task: %w", err)
			}
			res, err := tx.Exec(
				`UPDATE tasks SET status = 'claimed', attempts = attempts + 1, claimed_at = ?
				 WHERE id = ? AND status = 'pending'`, time.Now(), t.ID,
			)
			if err != nil {
				return fmt.Errorf("claim task: %w", err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("rows affected: %w", err)
			}
			if n == 1 {
				t.Status = "claimed"
				t.Attempts++
				task = t
			}
			return nil
		})
		if err != nil {
			return nil, false, err
		}
		if task != nil {
			return task, true, nil
		}
	}
	return nil, false, nil
}

// ReleaseStaleTasks возвращает в очередь задачи, зависшие после падения агента.
func ReleaseStaleTasks(timeout time.Duration) (int, error) {
	res, err := database.GetDB().Exec(
		`UPDATE tasks SET status = 'pending' WHERE status = 'claimed' AND claimed_at < ?`,
		time.Now().Add(-timeout),
	)
	if err != nil {
		return 0, fmt.Errorf("release stale tasks: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// GetTask возвращает задачу по ID.
func GetTask(taskID string) (*Task, bool, error) {
	t, err := scanTask(database.GetDB().QueryRow(
		`SELECT `+taskColumns+` FROM tasks WHERE id = ?`, taskID,
	))
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get task %s: %w", taskID, err)
	}
	return t, true, nil
}

// CompleteTask идемпотентно завершает задачу и не перезаписывает готовый результат.
func CompleteTask(taskID string, result float64) error {
	return database.Transaction(func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRow(`SELECT status FROM tasks WHERE id = ?`, taskID).Scan(&status); err != nil {
			return fmt.Errorf("task %s: %w", taskID, err)
		}
		if status == "completed" {
			return nil
		}
		res, err := tx.Exec(
			`UPDATE tasks SET completed = 1, result = ?, status = 'completed', error = ''
			 WHERE id = ? AND status IN ('claimed', 'error')`,
			result, taskID,
		)
		if err != nil {
			return fmt.Errorf("complete task: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		if n != 1 {
			return fmt.Errorf("task %s is not claimed (status=%s)", taskID, status)
		}
		return nil
	})
}

// FailTask отмечает задачу как ошибочную.
func FailTask(taskID, errText string) error {
	res, err := database.GetDB().Exec(
		`UPDATE tasks SET status = 'error', error = ?, claimed_at = NULL WHERE id = ? AND status = 'claimed'`,
		errText, taskID,
	)
	if err != nil {
		return fmt.Errorf("fail task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("fail task rows: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("task %s is not claimed", taskID)
	}
	return nil
}

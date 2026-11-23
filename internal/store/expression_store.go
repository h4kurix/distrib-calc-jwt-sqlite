package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Expression represents a mathematical expression.
type Expression struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Expression string    `json:"expression"`
	Status     string    `json:"status"`
	Result     float64   `json:"result"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

const exprColumns = `id, user_id, expression, status, COALESCE(result, 0), COALESCE(error, ''), created_at`

func scanExpression(rows rowScanner) (*Expression, error) {
	var e Expression
	err := rows.Scan(&e.ID, &e.UserID, &e.Expression, &e.Status, &e.Result, &e.Error, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CreateExpressionWithTasks создаёт выражение и его задачи в одной транзакции.
func (s *Store) CreateExpressionWithTasks(ctx context.Context, id, exprText, userID string, tasks []*Task) (*Expression, error) {
	expr := &Expression{
		ID: id, UserID: userID, Expression: exprText,
		Status: "pending", CreatedAt: time.Now(),
	}
	err := s.db.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO expressions (id, user_id, expression, status, created_at) VALUES (?, ?, ?, ?, ?)`,
			expr.ID, expr.UserID, expr.Expression, expr.Status, expr.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert expression: %w", err)
		}
		for _, t := range tasks {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO tasks (
					id, expression_id, user_id, arg1, arg2, operator, operation_time, completed, status
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
				t.ID, expr.ID, expr.UserID, t.Arg1, t.Arg2, t.Operator, t.OperationTime, t.Completed,
			); err != nil {
				return fmt.Errorf("insert task %s: %w", t.ID, err)
			}
			if err := insertDependencies(ctx, tx, t.ID, t.Arg1, t.Arg2); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return expr, nil
}

// GetExpression возвращает выражение по ID. Владельца проверяет вызывающий.
func (s *Store) GetExpression(ctx context.Context, id string) (*Expression, bool, error) {
	e, err := scanExpression(s.db.Conn().QueryRowContext(ctx,
		`SELECT `+exprColumns+` FROM expressions WHERE id = ?`, id,
	))
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get expression %s: %w", id, err)
	}
	return e, true, nil
}

// ListExpressions возвращает выражения пользователя.
func (s *Store) ListExpressions(ctx context.Context, userID string) ([]*Expression, error) {
	rows, err := s.db.Conn().QueryContext(ctx,
		`SELECT `+exprColumns+` FROM expressions WHERE user_id = ? ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list expressions: %w", err)
	}
	defer rows.Close()

	var result []*Expression
	for rows.Next() {
		e, err := scanExpression(rows)
		if err != nil {
			return nil, fmt.Errorf("scan expression: %w", err)
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expressions: %w", err)
	}
	return result, nil
}

// UpdateExpressionStatus обновляет статус, результат и ошибку выражения.
func (s *Store) UpdateExpressionStatus(ctx context.Context, exprID, status string, result float64, errText string) error {
	res, err := s.db.Conn().ExecContext(ctx,
		`UPDATE expressions SET status = ?, result = ?, error = ? WHERE id = ?`,
		status, result, errText, exprID,
	)
	if err != nil {
		return fmt.Errorf("update expression status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("expression %s not found", exprID)
	}
	return nil
}

// ExpireStaleExpressions закрывает выражения, которые не завершились за timeout.
// Без этого клиент опрашивает статус вечно, если агенты недоступны.
func (s *Store) ExpireStaleExpressions(ctx context.Context, timeout time.Duration) (int, error) {
	cutoff := time.Now().Add(-timeout)
	res, err := s.db.Conn().ExecContext(ctx,
		`UPDATE expressions SET status = 'error', error = 'expression timed out'
		 WHERE status IN ('pending', 'in_progress') AND created_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("expire expressions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("expire expressions rows: %w", err)
	}
	return int(n), nil
}

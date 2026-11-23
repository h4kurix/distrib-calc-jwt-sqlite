package store

import (
	"context"
	"database/sql"
	"fmt"
)

// FinalizeExpression обновляет статус выражения в одной транзакции.
// aggregate вызывается внутри транзакции, поэтому счётчики и результат
// всегда соответствуют одному и тому же снимку задач.
func (s *Store) FinalizeExpression(ctx context.Context, exprID, userID string, aggregate func([]*Task) (float64, error)) error {
	return s.db.Transaction(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM expressions WHERE id = ?`, exprID).Scan(&status); err != nil {
			return fmt.Errorf("expression %s: %w", exprID, err)
		}
		if status == "completed" || status == "error" {
			return nil
		}

		var failed int
		var failedErr string
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*), COALESCE(MIN(NULLIF(TRIM(error), '')), '') FROM tasks
			 WHERE expression_id = ? AND status = 'error'`, exprID,
		).Scan(&failed, &failedErr); err != nil {
			return fmt.Errorf("read failed tasks: %w", err)
		}
		if failed > 0 {
			// Показываем причину отказа, а не общее "one or more tasks failed".
			if failedErr == "" {
				failedErr = "one or more tasks failed"
			}
			return updateExprTx(ctx, tx, exprID, "error", 0, failedErr)
		}

		var incomplete int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM tasks WHERE expression_id = ? AND status != 'completed'`, exprID,
		).Scan(&incomplete); err != nil {
			return fmt.Errorf("count incomplete tasks: %w", err)
		}
		if incomplete > 0 {
			if status == "pending" {
				return updateExprTx(ctx, tx, exprID, "in_progress", 0, "")
			}
			return nil
		}

		rows, err := tx.QueryContext(ctx,
			`SELECT `+taskColumns+` FROM tasks WHERE expression_id = ? AND user_id = ? ORDER BY rowid ASC`,
			exprID, userID,
		)
		if err != nil {
			return fmt.Errorf("get tasks: %w", err)
		}
		var tasks []*Task
		for rows.Next() {
			t, err := scanTask(rows)
			if err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan task: %w", err)
			}
			tasks = append(tasks, t)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return fmt.Errorf("iterate tasks: %w", err)
		}

		result, err := aggregate(tasks)
		if err != nil {
			return updateExprTx(ctx, tx, exprID, "error", 0, err.Error())
		}
		return updateExprTx(ctx, tx, exprID, "completed", result, "")
	})
}

// updateExprTx пишет статус только если выражение ещё не финализировано.
func updateExprTx(ctx context.Context, tx *sql.Tx, exprID, status string, result float64, errText string) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE expressions SET status = ?, result = ?, error = ?
		 WHERE id = ? AND status IN ('pending', 'in_progress')`,
		status, result, errText, exprID,
	)
	if err != nil {
		return fmt.Errorf("update expression: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		// Другой обработчик уже зафиксировал результат.
		return nil
	}
	return nil
}

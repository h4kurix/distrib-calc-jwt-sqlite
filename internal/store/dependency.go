package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const taskRefPrefix = "task:"

// dependencyID возвращает ID задачи из ссылки вида task:<id>.
func dependencyID(arg string) string {
	if !strings.HasPrefix(arg, taskRefPrefix) {
		return ""
	}
	return strings.TrimPrefix(arg, taskRefPrefix)
}

// insertDependencies сохраняет связи задач в отдельную таблицу.
func insertDependencies(ctx context.Context, tx *sql.Tx, taskID, arg1, arg2 string) error {
	for _, arg := range []string{arg1, arg2} {
		dependsOn := dependencyID(arg)
		if dependsOn == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO task_dependencies (task_id, depends_on) VALUES (?, ?)`,
			taskID, dependsOn,
		); err != nil {
			return fmt.Errorf("insert dependency %s -> %s: %w", taskID, dependsOn, err)
		}
	}
	return nil
}

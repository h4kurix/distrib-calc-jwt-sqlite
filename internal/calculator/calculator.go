package calculator

import (
	"calc-service/internal/store"
	"calc-service/pkg/logger"
	"fmt"
	"strings"
)

var (
	maxExpressionLen = 256
	maxTasksPerExpr  = 256
	maxOperationMS   = 30000
)

// Configure задаёт ограничения парсера и планировщика.
func Configure(exprLen, tasks int, operationMS int) {
	if exprLen > 0 {
		maxExpressionLen = exprLen
	}
	if tasks > 0 {
		maxTasksPerExpr = tasks
	}
	if operationMS > 0 {
		maxOperationMS = operationMS
	}
}

// ProcessExpression проверяет выражение и атомарно создаёт выражение с задачами.
func ProcessExpression(exprStr, userID string) (*store.Expression, error) {
	exprStr = strings.ReplaceAll(exprStr, " ", "")
	if len([]rune(exprStr)) > maxExpressionLen {
		return nil, fmt.Errorf("expression is too long (limit %d)", maxExpressionLen)
	}
	if err := ValidateExpression(exprStr); err != nil {
		return nil, err
	}
	tokens, err := tokenize(exprStr)
	if err != nil {
		return nil, err
	}
	tree, err := buildExpressionTree(tokens)
	if err != nil {
		return nil, err
	}

	id := newID("expr")
	tasks, err := createTasksFromTree(id, tree)
	if err != nil {
		return nil, err
	}
	if len(tasks) > maxTasksPerExpr {
		return nil, fmt.Errorf("too many operations (limit %d)", maxTasksPerExpr)
	}

	expr, err := store.CreateExpressionWithTasks(id, exprStr, userID, tasks)
	if err != nil {
		logger.Error("create expression: %v", err)
		return nil, fmt.Errorf("create expression: %w", err)
	}
	return expr, nil
}

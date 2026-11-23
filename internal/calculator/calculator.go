package calculator

import (
	"calc-service/internal/id"
	"calc-service/internal/store"
	"calc-service/pkg/logger"
	"errors"
	"fmt"
	"strings"
)

// ErrInternal отделяет внутренние ошибки от ошибок валидации выражения.
// Handler показывает клиенту только валидацию.
var ErrInternal = errors.New("internal error")

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

	exprID := id.New("expr")
	tasks, err := createTasksFromTree(exprID, tree)
	if err != nil {
		return nil, err
	}
	if len(tasks) > maxTasksPerExpr {
		return nil, fmt.Errorf("too many operations (limit %d)", maxTasksPerExpr)
	}

	expr, err := store.CreateExpressionWithTasks(exprID, exprStr, userID, tasks)
	if err != nil {
		logger.Error("create expression: %v", err)
		return nil, fmt.Errorf("%w: create expression: %v", ErrInternal, err)
	}
	return expr, nil
}

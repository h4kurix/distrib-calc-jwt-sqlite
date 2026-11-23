package calculator

import (
	"calc-service/internal/id"
	"calc-service/internal/logger"
	"calc-service/internal/store"
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
)

// ErrInternal отделяет внутренние ошибки от ошибок валидации выражения.
// Handler показывает клиенту только валидацию.
var ErrInternal = errors.New("internal error")

// Limits задаёт ограничения парсера, планировщика и время операций.
type Limits struct {
	MaxExpressionLen int
	MaxTasksPerExpr  int
	MaxOperationMS   int
	AdditionMS       int
	SubtractionMS    int
	MultiplicationMS int
	DivisionMS       int
}

var (
	maxExpressionLen = 256
	maxTasksPerExpr  = 256
	maxOperationMS   = 30000

	defaultOperationTimes = map[string]int{"+": 100, "-": 100, "*": 200, "/": 300}
	operationTimes        = maps.Clone(defaultOperationTimes)
)

// Configure задаёт ограничения из конфигурации сервиса.
func Configure(l Limits) {
	if l.MaxExpressionLen > 0 {
		maxExpressionLen = l.MaxExpressionLen
	}
	if l.MaxTasksPerExpr > 0 {
		maxTasksPerExpr = l.MaxTasksPerExpr
	}
	if l.MaxOperationMS > 0 {
		maxOperationMS = l.MaxOperationMS
	}
	operationTimes = maps.Clone(defaultOperationTimes)
	if l.AdditionMS >= 0 {
		operationTimes["+"] = l.AdditionMS
	}
	if l.SubtractionMS >= 0 {
		operationTimes["-"] = l.SubtractionMS
	}
	if l.MultiplicationMS >= 0 {
		operationTimes["*"] = l.MultiplicationMS
	}
	if l.DivisionMS >= 0 {
		operationTimes["/"] = l.DivisionMS
	}
}

// ProcessExpression проверяет выражение и атомарно создаёт выражение с задачами.
func ProcessExpression(st *store.Store, ctx context.Context, exprStr, userID string) (*store.Expression, error) {
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

	expr, err := st.CreateExpressionWithTasks(ctx, exprID, exprStr, userID, tasks)
	if err != nil {
		logger.Error("create expression: %v", err)
		return nil, fmt.Errorf("%w: create expression: %v", ErrInternal, err)
	}
	return expr, nil
}

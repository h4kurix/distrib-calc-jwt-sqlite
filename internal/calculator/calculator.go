package calculator

import (
	"calc-service/internal/store"
	"calc-service/pkg/logger"
	"fmt"
	"strings"
)

// ProcessExpression processes a mathematical expression and returns the expression object
func ProcessExpression(exprStr string, userID string) (*store.Expression, error) {
	//logger.Info("Processing expression: %s (user: %s)", exprStr, userID)

	exprStr = strings.ReplaceAll(exprStr, " ", "")
	if err := ValidateExpression(exprStr); err != nil {
		logger.Error("ProcessExpression: Validation error: %v", err)
		return nil, err
	}

	tokens, err := tokenize(exprStr)
	if err != nil {
		logger.Error("ProcessExpression: Tokenization failed: %v", err)
		return nil, err
	}

	tree, err := buildExpressionTree(tokens)
	if err != nil {
		logger.Error("ProcessExpression: Expression tree build failed: %v", err)
		return nil, err
	}

	expr, err := store.NewExpression(exprStr, userID)
	if err != nil {
		logger.Error("ProcessExpression: Failed to create expression record: %v", err)
		return nil, fmt.Errorf("failed to create expression record: %w", err)
	}
	logger.Info("ProcessExpression: Created expression: %s", expr.ID)

	tasks, err := createTasksFromTree(expr.ID, tree)
	if err != nil {
		logger.Error("Task generation failed: %v", err)
		return nil, err
	}
	logger.Info("ProcessExpression: Generated %d tasks for expression %s", len(tasks), expr.ID)

	if err := store.RegisterTasks(expr.ID, userID, tasks); err != nil {
		logger.Error("Failed to register tasks: %v", err)
		return nil, fmt.Errorf("failed to register tasks: %w", err)
	}

	// Проверяем наличие выполнимых задач (задач без зависимостей)
	executableTasks, err := store.GetExecutableTasks(expr.ID, userID)
	if err != nil {
		logger.Error("ProcessExpression: Failed to get executable tasks: %v", err)
		return nil, fmt.Errorf("failed to get executable tasks: %w", err)
	}

	// Если есть задачи, которые можно выполнить немедленно, обновляем статус выражения
	if len(executableTasks) > 0 {
		logger.Info("ProcessExpression: Expression %s has %d immediately executable tasks", expr.ID, len(executableTasks))
	} else {
		logger.Info("ProcessExpression: Expression %s has no immediately executable tasks", expr.ID)
	}

	logger.Info("ProcessExpression: Expression %s processed successfully", expr.ID)
	return expr, nil
}

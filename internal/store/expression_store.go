package store

import (
	"calc-service/pkg/database"
	"calc-service/pkg/logger"
	"database/sql"
	"fmt"
	"time"
)

// Expression represents a mathematical expression
type Expression struct {
	ID         string    `json:"id"`
	Expression string    `json:"expression"`
	Status     string    `json:"status"`
	Result     float64   `json:"result,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// NewExpression creates a new expression record
func NewExpression(exprText, userID string) (*Expression, error) {
	id := fmt.Sprintf("expr-%d", time.Now().UnixNano())
	now := time.Now()

	expr := &Expression{
		ID:         id,
		Expression: exprText,
		Status:     "pending",
		CreatedAt:  now,
	}

	// Вставка в базу данных с учетом userID
	db := database.GetDB()
	_, err := db.Exec(
		"INSERT INTO expressions (id, user_id, expression, status, created_at) VALUES (?, ?, ?, ?, ?)",
		expr.ID, userID, expr.Expression, expr.Status, expr.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert expression: %w", err)
	}

	return expr, nil
}

// GetExpression retrieves an expression by ID
func GetExpression(id string) (*Expression, bool) {
	db := database.GetDB()
	var expr Expression

	err := db.QueryRow(
		"SELECT id, expression, status, COALESCE(result, 0), created_at FROM expressions WHERE id = ?",
		id,
	).Scan(&expr.ID, &expr.Expression, &expr.Status, &expr.Result, &expr.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false
		}
		logger.Error("Database error in GetExpression: %v", err)
		return nil, false
	}

	return &expr, true
}

// ListExpressions возвращает все выражения для конкретного пользователя
func ListExpressions(userID string) []*Expression {
	db := database.GetDB()
	rows, err := db.Query(
		"SELECT id, expression, status, COALESCE(result, 0), created_at FROM expressions WHERE user_id = ? ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		logger.Error("Database error in ListExpressions: %v", err)
		return []*Expression{}
	}
	defer rows.Close()

	var expressions []*Expression
	for rows.Next() {
		var expr Expression
		if err := rows.Scan(&expr.ID, &expr.Expression, &expr.Status, &expr.Result, &expr.CreatedAt); err != nil {
			logger.Error("Error scanning expression row: %v", err)
			continue
		}
		expressions = append(expressions, &expr)
	}

	return expressions
}

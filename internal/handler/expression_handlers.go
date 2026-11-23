package handler

import (
	"calc-service/internal/calculator"
	"calc-service/internal/store"
	"calc-service/pkg/logger"
	"encoding/json"
	"net/http"
	"strings"
)

type CalculateRequest struct {
	Expression string `json:"expression"`
}

type CalculateResponse struct {
	ID string `json:"id"`
}

type ExpressionsResponse struct {
	Expressions []ExpressionResponse `json:"expressions"`
}

type ExpressionResponse struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Result float64 `json:"result,omitempty"`
	Error  string  `json:"error,omitempty"`
}

type ExpressionDetailResponse struct {
	Expression ExpressionResponse `json:"expression"`
}

func HandleCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := getUserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CalculateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	expr, err := calculator.ProcessExpression(req.Expression, userID)
	if err != nil {
		logger.Error("process expression: %v", err)
		http.Error(w, "Invalid expression", http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusCreated, CalculateResponse{ID: expr.ID})
}

func HandleExpressions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := getUserIDFromContext(r.Context())
	expressions, err := store.ListExpressions(userID)
	if err != nil {
		logger.Error("list expressions: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := make([]ExpressionResponse, 0, len(expressions))
	for _, e := range expressions {
		response = append(response, ExpressionResponse{
			ID: e.ID, Status: e.Status, Result: e.Result, Error: e.Error,
		})
	}
	writeJSON(w, http.StatusOK, ExpressionsResponse{Expressions: response})
}

func HandleExpressionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := getUserIDFromContext(r.Context())
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/expressions/")

	expr, found, err := store.GetExpression(id)
	if err != nil {
		logger.Error("get expression: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !found || expr.UserID != userID {
		http.Error(w, "Expression not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, ExpressionDetailResponse{Expression: ExpressionResponse{
		ID: expr.ID, Status: expr.Status, Result: expr.Result, Error: expr.Error,
	}})
}

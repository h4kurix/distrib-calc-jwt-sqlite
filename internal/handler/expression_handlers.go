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
}

type ExpressionDetailResponse struct {
	Expression ExpressionResponse `json:"expression"`
}

func HandleCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logger.Warn("HandleCalculate: Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := getUserIDFromContext(r.Context())
	logger.Info("HandleCalculate: Received request from userID: %s", userID)

	var req CalculateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("HandleCalculate: Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusUnprocessableEntity)
		return
	}

	logger.Info("HandleCalculate: Processing expression: %s", req.Expression)

	expr, err := calculator.ProcessExpression(req.Expression, userID)
	if err != nil {
		logger.Error("HandleCalculate: Expression processing error: %v", err)
		http.Error(w, "Invalid expression: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	logger.Info("HandleCalculate: Task created with ID: %s, status: %s", expr.ID, expr.Status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CalculateResponse{ID: expr.ID})
}

func HandleExpressions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем userID из контекста
	userID := getUserIDFromContext(r.Context())

	expressions := store.ListExpressions(userID) // фильтруем по userID
	response := make([]ExpressionResponse, 0, len(expressions))

	for _, expr := range expressions {
		response = append(response, ExpressionResponse{
			ID:     expr.ID,
			Status: expr.Status,
			Result: expr.Result,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ExpressionsResponse{Expressions: response})
}

func HandleExpressionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logger.Warn("HandleExpressionByID: Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/expressions/")
	logger.Info("HandleExpressionByID: Looking for expression ID: %s", id)

	expr, exists := store.GetExpression(id)
	if !exists {
		logger.Warn("HandleExpressionByID: Expression not found: %s", id)
		http.Error(w, "Expression not found", http.StatusNotFound)
		return
	}

	logger.Info("HandleExpressionByID: Found expression ID: %s, status: %s", expr.ID, expr.Status)

	response := ExpressionResponse{
		ID:     expr.ID,
		Status: expr.Status,
		Result: expr.Result,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ExpressionDetailResponse{Expression: response})
}

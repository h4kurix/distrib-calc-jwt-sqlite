package handler

import (
	"errors"
	"net/http"

	"calc-service/internal/calculator"
	"calc-service/internal/store"
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

// ExpressionResponse — публичное представление выражения.
// Result без omitempty: ноль — законный результат, и клиент должен его видеть.
type ExpressionResponse struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Result float64 `json:"result"`
	Error  string  `json:"error,omitempty"`
}

type ExpressionDetailResponse struct {
	Expression ExpressionResponse `json:"expression"`
}

func HandleCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	userID := getUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CalculateRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	expr, err := calculator.ProcessExpression(req.Expression, userID)
	if err != nil {
		if errors.Is(err, calculator.ErrInternal) {
			logAndFail(w, "process expression", err)
			return
		}
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, CalculateResponse{ID: expr.ID})
}

func HandleExpressions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	userID := getUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	expressions, err := store.ListExpressions(userID)
	if err != nil {
		logAndFail(w, "list expressions", err)
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
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	userID := getUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusNotFound, "expression not found")
		return
	}

	expr, found, err := store.GetExpression(id)
	if err != nil {
		logAndFail(w, "get expression", err)
		return
	}
	if !found || expr.UserID != userID {
		writeError(w, http.StatusNotFound, "expression not found")
		return
	}

	writeJSON(w, http.StatusOK, ExpressionDetailResponse{Expression: ExpressionResponse{
		ID: expr.ID, Status: expr.Status, Result: expr.Result, Error: expr.Error,
	}})
}

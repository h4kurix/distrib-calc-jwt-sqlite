package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Клиент разбирает любой ответ как JSON, поэтому ошибки тоже должны быть JSON.
func TestWriteErrorReturnsJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusUnprocessableEntity, "bad expression")

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content type = %q, want application/json", ct)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%q)", err, rec.Body.String())
	}
	if body.Error != "bad expression" {
		t.Fatalf("error = %q", body.Error)
	}
}

// Ноль — законный результат. Он обязан попасть в JSON, иначе UI покажет undefined.
func TestExpressionResponseKeepsZeroResult(t *testing.T) {
	raw, err := json.Marshal(ExpressionResponse{ID: "expr-1", Status: "completed", Result: 0})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"result":0`) {
		t.Fatalf("zero result missing from %s", raw)
	}

	var back ExpressionResponse
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Result != 0 || back.Status != "completed" {
		t.Fatalf("round trip = %+v", back)
	}
}

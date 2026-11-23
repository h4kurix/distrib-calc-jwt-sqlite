package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"calc-service/pkg/logger"
)

// maxBodyBytes ограничивает тело запроса: API принимает только короткий JSON.
const maxBodyBytes = 64 << 10

// errorResponse — единый формат ошибки. Клиент всегда получает JSON,
// поэтому разбор ответа не падает на текстовом теле.
type errorResponse struct {
	Error string `json:"error"`
}

// writeError отправляет ошибку в JSON.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

// WriteError — экспорт для пакетов вне handler (например, маршрутов в main).
func WriteError(w http.ResponseWriter, status int, message string) {
	writeError(w, status, message)
}

// WriteJSON — экспорт успешных ответов для маршрутов вне handler.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, v)
}

// decodeJSONBody читает один JSON-объект с ограничением размера.
// Неизвестные поля допускаются: это часть текущего контракта API.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

// HandleNotFound отвечает JSON: клиент всегда разбирает ответ как JSON.
func HandleNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "not found")
}

// writeMethodNotAllowed отвечает 405 с заголовком Allow.
func writeMethodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// logAndFail пишет ошибку в лог и отвечает 500.
func logAndFail(w http.ResponseWriter, message string, err error) {
	logger.Error("%s: %v", message, err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

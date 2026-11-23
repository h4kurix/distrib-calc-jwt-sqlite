package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"calc-service/internal/store"
	"calc-service/pkg/logger"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

var agentTokenTTL = 24 * time.Hour

// SetAgentTokenTTL задаёт срок жизни токена агента.
func SetAgentTokenTTL(ttl time.Duration) { agentTokenTTL = ttl }

func HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	user, err := store.CreateUser(req.Username, req.Password)
	if err != nil {
		logger.Error("register %q failed: %v", req.Username, err)
		http.Error(w, "Could not create user", http.StatusConflict)
		return
	}
	token, err := generateToken(user.ID)
	if err != nil {
		logger.Error("token generation failed: %v", err)
		http.Error(w, "Token generation failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, AuthResponse{Token: token})
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	user, found, err := store.GetUserByUsername(req.Username)
	if err != nil {
		logger.Error("login lookup failed: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := comparePassword(user.Password, req.Password); err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}
	token, err := generateToken(user.ID)
	if err != nil {
		logger.Error("token generation failed: %v", err)
		http.Error(w, "Token generation failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, AuthResponse{Token: token})
}

func comparePassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func generateToken(userID string) (string, error) {
	if len(jwtSecret) == 0 {
		return "", jwt.ErrInvalidKey
	}
	claims := jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
}

// GenerateAgentToken создаёт токен для агента.
func GenerateAgentToken() (string, error) {
	if len(jwtSecret) == 0 {
		return "", jwt.ErrInvalidKey
	}
	claims := jwt.MapClaims{
		"role": "agent",
		"exp":  time.Now().Add(agentTokenTTL).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
}

// writeJSON кодирует ответ в буфер и ставит статус только после успешного кодирования.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		logger.Error("encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		logger.Error("write response: %v", err)
	}
}

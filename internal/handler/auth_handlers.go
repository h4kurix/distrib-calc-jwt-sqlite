package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"calc-service/internal/store"
	"calc-service/pkg/logger"
	"time"

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

func HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("Invalid request data: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	logger.Info("Attempting to register user: %s", req.Username)
	user, err := store.CreateUser(req.Username, req.Password)
	if err != nil {
		logger.Error("Failed to create user: %v", err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	tokenString, err := generateToken(user.ID)
	if err != nil {
		logger.Error("Failed to generate token: %v", err)
		http.Error(w, "Token generation failed", http.StatusInternalServerError)
		return
	}

	logger.Info("User registered successfully: %s", req.Username)
	writeJSON(w, AuthResponse{Token: tokenString})
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("Invalid login request data: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	logger.Info("Attempting to log in user: %s", req.Username)
	user, found := store.GetUserByUsername(req.Username)
	if !found {
		logger.Error("Invalid login attempt: user not found %s", req.Username)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		logger.Error("Invalid password for user: %s", req.Username)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	tokenString, err := generateToken(user.ID)
	if err != nil {
		logger.Error("Failed to generate token for user %s: %v", req.Username, err)
		http.Error(w, "Token generation failed", http.StatusInternalServerError)
		return
	}

	logger.Info("User logged in successfully: %s", req.Username)
	writeJSON(w, AuthResponse{Token: tokenString})
}

func generateToken(userID string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "", ErrMissingJWTSecret
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})
	return token.SignedString([]byte(secret))
}

var ErrMissingJWTSecret = jwt.ErrInvalidKey

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// GenerateAgentToken создает JWT токен для агента
func GenerateAgentToken() (string, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		err := errors.New("JWT_SECRET is not set")
		// Логируем ошибку для диагностики
		fmt.Println(err)
		return "", err
	}

	claims := jwt.MapClaims{
		"role": "agent",
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		// Логируем ошибку подписи токена
		fmt.Printf("Error signing token: %v\n", err)
		return "", err
	}

	return signedToken, nil
}

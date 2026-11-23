package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"calc-service/internal/store"
	"calc-service/pkg/logger"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	minUsernameLen = 3
	maxUsernameLen = 64
	minPasswordLen = 8
	maxPasswordLen = 72 // bcrypt игнорирует байты после 72-го
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

var (
	agentTokenTTL = 24 * time.Hour
	userTokenTTL  = 24 * time.Hour
)

// SetAgentTokenTTL задаёт срок жизни токена агента.
func SetAgentTokenTTL(ttl time.Duration) { agentTokenTTL = ttl }

// SetUserTokenTTL задаёт срок жизни пользовательского токена.
func SetUserTokenTTL(ttl time.Duration) { userTokenTTL = ttl }

func HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	var req RegisterRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateCredentials(req.Username, req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := store.CreateUser(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, store.ErrUsernameExists) {
			writeError(w, http.StatusConflict, "username already exists")
			return
		}
		logAndFail(w, "register "+req.Username, err)
		return
	}
	token, err := generateToken(user.ID)
	if err != nil {
		logAndFail(w, "token generation", err)
		return
	}
	writeJSON(w, http.StatusCreated, AuthResponse{Token: token})
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	var req LoginRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, found, err := store.GetUserByUsername(req.Username)
	if err != nil {
		logAndFail(w, "login lookup", err)
		return
	}
	if !found || comparePassword(user.Password, req.Password) != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := generateToken(user.ID)
	if err != nil {
		logAndFail(w, "token generation", err)
		return
	}
	writeJSON(w, http.StatusOK, AuthResponse{Token: token})
}

// validateCredentials проверяет логин и пароль до обращения к базе.
func validateCredentials(username, password string) error {
	name := strings.TrimSpace(username)
	if utf8.RuneCountInString(name) < minUsernameLen || utf8.RuneCountInString(name) > maxUsernameLen {
		return errors.New("username must be 3 to 64 characters long")
	}
	for _, ch := range name {
		ok := ch == '_' || ch == '-' || ch == '.' ||
			(ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
		if !ok {
			return errors.New("username may contain only letters, digits, dot, dash and underscore")
		}
	}
	if len(password) < minPasswordLen {
		return errors.New("password must be at least 8 characters long")
	}
	if len(password) > maxPasswordLen {
		return errors.New("password must be at most 72 bytes long")
	}
	return nil
}

func comparePassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func generateToken(userID string) (string, error) {
	if len(jwtSecret) == 0 {
		return "", jwt.ErrInvalidKey
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": userID,
		"iat": now.Unix(),
		"exp": now.Add(userTokenTTL).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
}

// GenerateAgentToken создаёт токен для агента.
func GenerateAgentToken() (string, error) {
	if len(jwtSecret) == 0 {
		return "", jwt.ErrInvalidKey
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"role": "agent",
		"iat":  now.Unix(),
		"exp":  now.Add(agentTokenTTL).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
}

// writeJSON кодирует ответ в буфер и ставит статус только после успешного кодирования.
func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		logger.Error("encode response: %v", err)
		// Кодирование упало: отдаём готовый JSON, чтобы клиент не получил text/plain.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{\"error\":\"internal server error\"}\n"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		logger.Error("write response: %v", err)
	}
}

package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const userIDKey contextKey = "userID"

var jwtSecret []byte

// SetJWTSecret задаёт ключ подписи. Вызывается при старте сервиса.
func SetJWTSecret(secret string) { jwtSecret = []byte(secret) }

func parseToken(r *http.Request) (*jwt.Token, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return nil, fmt.Errorf("missing bearer token")
	}
	tokenString := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if tokenString == "" {
		return nil, fmt.Errorf("empty bearer token")
	}
	if len(jwtSecret) == 0 {
		return nil, fmt.Errorf("server auth is not configured")
	}
	return jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
}

// AuthMiddleware пропускает только пользовательские токены с непустым sub.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := parseToken(r)
		if err != nil || !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, "Invalid token claims", http.StatusUnauthorized)
			return
		}
		userID, ok := claims["sub"].(string)
		if !ok || userID == "" {
			http.Error(w, "Invalid token subject", http.StatusUnauthorized)
			return
		}
		if role, _ := claims["role"].(string); role == "agent" {
			http.Error(w, "Agent token is not allowed here", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getUserIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(userIDKey).(string)
	return id
}

// AgentAuthMiddleware пропускает только токены агента.
func AgentAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := parseToken(r)
		if err != nil || !token.Valid {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if role, _ := claims["role"].(string); role != "agent" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

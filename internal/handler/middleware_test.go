package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func makeToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return token
}

func TestAuthMiddleware(t *testing.T) {
	deps := NewDeps(nil, testSecret, 24*time.Hour, 24*time.Hour)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := deps.AuthMiddleware(next)

	cases := []struct {
		name   string
		token  string
		status int
	}{
		{"user", makeToken(t, jwt.MapClaims{"sub": "user-1", "exp": time.Now().Add(time.Hour).Unix()}), http.StatusNoContent},
		{"no exp", makeToken(t, jwt.MapClaims{"sub": "user-1"}), http.StatusUnauthorized},
		{"empty sub", makeToken(t, jwt.MapClaims{"sub": "", "exp": time.Now().Add(time.Hour).Unix()}), http.StatusUnauthorized},
		{"agent token", makeToken(t, jwt.MapClaims{"role": "agent", "exp": time.Now().Add(time.Hour).Unix()}), http.StatusUnauthorized},
		{"expired", makeToken(t, jwt.MapClaims{"sub": "user-1", "exp": time.Now().Add(-time.Hour).Unix()}), http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/expressions", nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

func TestAgentAuthMiddleware(t *testing.T) {
	deps := NewDeps(nil, testSecret, 24*time.Hour, 24*time.Hour)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := deps.AgentAuthMiddleware(next)

	agentToken := makeToken(t, jwt.MapClaims{"role": "agent", "exp": time.Now().Add(time.Hour).Unix()})
	userToken := makeToken(t, jwt.MapClaims{"sub": "user-1", "exp": time.Now().Add(time.Hour).Unix()})

	for _, tc := range []struct {
		token  string
		status int
	}{{agentToken, http.StatusNoContent}, {userToken, http.StatusUnauthorized}} {
		req := httptest.NewRequest(http.MethodGet, "/internal/task", nil)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("got %d, want %d", rec.Code, tc.status)
		}
	}
}

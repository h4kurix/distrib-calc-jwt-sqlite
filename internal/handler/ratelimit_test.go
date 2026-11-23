package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	called := 0
	next := func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}
	wrap := NewRateLimiter(2, time.Minute)

	r1 := httptest.NewRequest(http.MethodPost, "/api/v1/login", nil)
	r1.RemoteAddr = "192.0.2.1:1234"
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		wrap(next)(w, r1)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: got status %d, want 200", i, w.Code)
		}
	}

	w := httptest.NewRecorder()
	wrap(next)(w, r1)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("third request: got status %d, want 429", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type: got %q, want application/json", got)
	}
	if called != 2 {
		t.Fatalf("next called %d times, want 2", called)
	}

	// Другой адрес имеет свою квоту.
	r2 := httptest.NewRequest(http.MethodPost, "/api/v1/login", nil)
	r2.RemoteAddr = "192.0.2.2:1234"
	w = httptest.NewRecorder()
	wrap(next)(w, r2)
	if w.Code != http.StatusOK {
		t.Fatalf("other address: got status %d, want 200", w.Code)
	}
}

func TestRateLimiterSweepsStaleKeys(t *testing.T) {
	old := time.Now().Add(-2 * time.Minute)
	rl := &rateLimiter{
		limit:     2,
		window:    time.Minute,
		hits:      make(map[string][]time.Time),
		lastSweep: time.Now().Add(-time.Hour),
	}
	for i := 0; i < maxTrackedKeys+10; i++ {
		key := fmt.Sprintf("10.0.%d.%d:1", i/256, i%256)
		rl.hits[key] = []time.Time{old}
	}

	next := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", nil)
	req.RemoteAddr = "192.0.2.1:1"
	w := httptest.NewRecorder()
	rl.wrap(next)(w, req)

	if len(rl.hits) != 1 {
		t.Fatalf("tracked keys after sweep: got %d, want 1", len(rl.hits))
	}
	if _, ok := rl.hits["192.0.2.1"]; !ok {
		t.Fatal("new address was not recorded")
	}
}

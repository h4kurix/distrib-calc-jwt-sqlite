package handler

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// rateLimiter считает запросы с одного адреса за скользящее окно.
const (
	maxTrackedKeys = 1024
	sweepInterval  = 10 * time.Second
)

type rateLimiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	hits      map[string][]time.Time
	lastSweep time.Time
}

// NewRateLimiter возвращает middleware с лимитом запросов на адрес.
func NewRateLimiter(limit int, window time.Duration) func(http.HandlerFunc) http.HandlerFunc {
	rl := &rateLimiter{
		limit:  limit,
		window: window,
		hits:   make(map[string][]time.Time),
	}
	return rl.wrap
}

func (rl *rateLimiter) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			key = r.RemoteAddr
		}

		rl.mu.Lock()
		now := time.Now()
		if len(rl.hits) > maxTrackedKeys && now.Sub(rl.lastSweep) >= sweepInterval {
			rl.sweep(now)
			rl.lastSweep = now
		}
		cutoff := now.Add(-rl.window)
		previous := rl.hits[key]
		recent := previous[:0]
		for _, t := range previous {
			if t.After(cutoff) {
				recent = append(recent, t)
			}
		}
		if len(recent) >= rl.limit {
			rl.hits[key] = recent
			rl.mu.Unlock()
			w.Header().Set("Retry-After", formatSeconds(rl.window))
			writeError(w, http.StatusTooManyRequests, "too many attempts")
			return
		}
		rl.hits[key] = append(recent, now)
		rl.mu.Unlock()

		next(w, r)
	}
}

func formatSeconds(d time.Duration) string {
	return strconv.Itoa(int(d.Seconds()))
}

// sweep удаляет адреса, с которых давно не было запросов.
func (rl *rateLimiter) sweep(now time.Time) {
	cutoff := now.Add(-rl.window)
	for key, times := range rl.hits {
		if len(times) == 0 || times[len(times)-1].Before(cutoff) {
			delete(rl.hits, key)
		}
	}
}

package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a per-key token bucket rate limiter.
type RateLimiter struct {
	rate    float64 // tokens per second
	burst   int
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

// NewRateLimiter creates a rate limiter. rate is requests per second, burst is max burst size.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		rate:    rate,
		burst:   burst,
		buckets: make(map[string]*bucket),
	}
}

// Allow checks if a request from the given key is allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(rl.burst), lastCheck: time.Now()}
		rl.buckets[key] = b
	}

	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.lastCheck = now
	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// RateLimitMiddleware returns HTTP middleware that rate-limits by the key returned by keyFunc.
func RateLimitMiddleware(rl *RateLimiter, keyFunc func(*http.Request) string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := keyFunc(r)
		if !rl.Allow(key) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

// IPKey extracts the client IP from the request for rate limiting.
func IPKey(r *http.Request) string {
	// Use X-Forwarded-For if behind a proxy, otherwise RemoteAddr
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// AgentIDKey extracts the agent ID header for rate limiting.
func AgentIDKey(r *http.Request) string {
	return r.Header.Get("X-Agent-Id")
}

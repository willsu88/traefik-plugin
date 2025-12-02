package traefik_plugin

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Config holds the plugin configuration
type Config struct {
	Rate  int `json:"rate,omitempty"`  // requests per second
	Burst int `json:"burst,omitempty"` // max burst
}

// CreateConfig creates the default plugin configuration
func CreateConfig() *Config {
	return &Config{
		Rate:  10,
		Burst: 20,
	}
}

// FreeTierLimiter is the plugin struct
type FreeTierLimiter struct {
	next    http.Handler
	name    string
	rate    int
	burst   int
	clients map[string]*clientLimiter
	mu      sync.RWMutex
}

type clientLimiter struct {
	tokens    float64
	lastCheck time.Time
}

// New creates a new plugin instance
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return &FreeTierLimiter{
		next:    next,
		name:    name,
		rate:    config.Rate,
		burst:   config.Burst,
		clients: make(map[string]*clientLimiter),
	}, nil
}

func (f *FreeTierLimiter) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Check if this is a free:free auth request
	if !f.isFreeUser(req) {
		// Not a free user, pass through without rate limiting
		f.next.ServeHTTP(rw, req)
		return
	}

	// Get client IP
	clientIP := f.getClientIP(req)

	// Check rate limit
	if !f.allow(clientIP) {
		rw.Header().Set("Retry-After", "1")
		http.Error(rw, "Rate limit exceeded for free tier", http.StatusTooManyRequests)
		return
	}

	f.next.ServeHTTP(rw, req)
}

func (f *FreeTierLimiter) isFreeUser(req *http.Request) bool {
	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Basic ") {
		return false
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
	if err != nil {
		return false
	}

	return string(decoded) == "free:free"
}

func (f *FreeTierLimiter) getClientIP(req *http.Request) string {
	// Try X-Forwarded-For first
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Try X-Real-IP
	if xri := req.Header.Get("X-Real-Ip"); xri != "" {
		return xri
	}

	// Fall back to remote addr
	if idx := strings.LastIndex(req.RemoteAddr, ":"); idx != -1 {
		return req.RemoteAddr[:idx]
	}
	return req.RemoteAddr
}

func (f *FreeTierLimiter) allow(clientIP string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()

	client, exists := f.clients[clientIP]
	if !exists {
		f.clients[clientIP] = &clientLimiter{
			tokens:    float64(f.burst - 1),
			lastCheck: now,
		}
		return true
	}

	// Token bucket algorithm
	elapsed := now.Sub(client.lastCheck).Seconds()
	client.tokens += elapsed * float64(f.rate)
	if client.tokens > float64(f.burst) {
		client.tokens = float64(f.burst)
	}
	client.lastCheck = now

	if client.tokens < 1 {
		return false
	}

	client.tokens--
	return true
}

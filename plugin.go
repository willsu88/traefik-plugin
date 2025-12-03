package traefik_plugin

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

type TierConfig struct {
	Rate  int `json:"rate,omitempty"`  // req/s
	Burst int `json:"burst,omitempty"` // max burst
}

type Config struct {
	HeaderName  string                `json:"headerName,omitempty"`  // e.g. X-User-Category
	DefaultTier string                `json:"defaultTier,omitempty"` // fallback if header missing
	Tiers       map[string]TierConfig `json:"tiers,omitempty"`       // tier -> limits
}

func CreateConfig() *Config {
	return &Config{
		HeaderName:  "X-User-Category",
		DefaultTier: "free",
		Tiers: map[string]TierConfig{
			"free":       {Rate: 2, Burst: 5},
			"pro":        {Rate: 20, Burst: 40},
			"enterprise": {Rate: 100, Burst: 200},
		},
	}
}

type Middleware struct {
	next        http.Handler
	headerName  string
	defaultTier string
	tiers       map[string]TierConfig
	mu          sync.Mutex
	buckets     map[string]*bucket // key: tier + ":" + clientID
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

func New(_ context.Context, next http.Handler, cfg *Config, _ string) (http.Handler, error) {
	return &Middleware{
		next:        next,
		headerName:  cfg.HeaderName,
		defaultTier: cfg.DefaultTier,
		tiers:       cfg.Tiers,
		buckets:     make(map[string]*bucket),
	}, nil
}

func (m *Middleware) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	tier := strings.TrimSpace(req.Header.Get(m.headerName))
	if tier == "" {
		tier = m.defaultTier
	}
	lim, ok := m.tiers[tier]
	if !ok || (lim.Rate == 0 && lim.Burst == 0) {
		m.next.ServeHTTP(rw, req)
		return
	}

	clientID := clientIP(req) // or IP+header if you want finer keys
	key := tier + ":" + clientID

	if !m.allow(key, lim) {
		rw.Header().Set("Retry-After", "1")
		http.Error(rw, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	m.next.ServeHTTP(rw, req)
}

func (m *Middleware) allow(key string, lim TierConfig) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	b, ok := m.buckets[key]
	if !ok {
		m.buckets[key] = &bucket{tokens: float64(lim.Burst - 1), lastCheck: now}
		return true
	}

	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * float64(lim.Rate)
	if b.tokens > float64(lim.Burst) {
		b.tokens = float64(lim.Burst)
	}
	b.lastCheck = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func clientIP(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := req.Header.Get("X-Real-Ip"); xri != "" {
		return xri
	}
	host := req.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		return host[:idx]
	}
	return host
}

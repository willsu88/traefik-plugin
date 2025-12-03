package traefik_plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newHandler(cfg *Config) http.Handler {
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})
	h, err := New(context.Background(), next, cfg, "test")
	if err != nil {
		panic(err)
	}
	return h
}

func TestDefaultTierWhenHeaderMissing(t *testing.T) {
	cfg := CreateConfig()
	cfg.Tiers = map[string]TierConfig{
		"free": {Rate: 1, Burst: 1},
	}
	h := newHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Second request should be rate-limited (burst 1, rate 1/s)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr2.Code)
	}
}

func TestTierSpecificLimits(t *testing.T) {
	cfg := CreateConfig()
	cfg.HeaderName = "X-User-Category"
	cfg.DefaultTier = "free"
	cfg.Tiers = map[string]TierConfig{
		"free": {Rate: 1, Burst: 1},
		"pro":  {Rate: 10, Burst: 10},
	}
	h := newHandler(cfg)

	// Free tier should rate-limit after first request
	reqFree := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	reqFree.Header.Set("X-User-Category", "free")
	reqFree.Header.Set("X-Forwarded-For", "1.1.1.1")

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, reqFree)
	if rr1.Code != http.StatusOK {
		t.Fatalf("free first: expected 200, got %d", rr1.Code)
	}
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, reqFree)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("free second: expected 429, got %d", rr2.Code)
	}

	// Pro tier should allow multiple within burst
	reqPro := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	reqPro.Header.Set("X-User-Category", "pro")
	reqPro.Header.Set("X-Forwarded-For", "2.2.2.2")

	for i := 0; i < 5; i++ {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, reqPro)
		if rr.Code != http.StatusOK {
			t.Fatalf("pro request %d: expected 200, got %d", i+1, rr.Code)
		}
	}
}

func TestBucketsAreTierIsolated(t *testing.T) {
	cfg := CreateConfig()
	cfg.Tiers = map[string]TierConfig{
		"free": {Rate: 1, Burst: 1},
		"pro":  {Rate: 1, Burst: 1},
	}
	h := newHandler(cfg)

	reqFree := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	reqFree.Header.Set("X-User-Category", "free")
	reqFree.Header.Set("X-Forwarded-For", "3.3.3.3")

	reqPro := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	reqPro.Header.Set("X-User-Category", "pro")
	reqPro.Header.Set("X-Forwarded-For", "3.3.3.3") // same IP, different tier

	// Consume free burst
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, reqFree)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, reqFree)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("free second should be 429, got %d", rr2.Code)
	}

	// Pro should still have its own bucket
	rrPro := httptest.NewRecorder()
	h.ServeHTTP(rrPro, reqPro)
	if rrPro.Code != http.StatusOK {
		t.Fatalf("pro should be 200, got %d", rrPro.Code)
	}
}

func TestTokensRefillOverTime(t *testing.T) {
	cfg := CreateConfig()
	cfg.Tiers = map[string]TierConfig{
		"free": {Rate: 1, Burst: 1},
	}
	h := newHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req.Header.Set("X-User-Category", "free")
	req.Header.Set("X-Forwarded-For", "4.4.4.4")

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request should be 429, got %d", rr2.Code)
	}

	// Wait for 1s to refill
	time.Sleep(time.Second)

	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req)
	if rr3.Code != http.StatusOK {
		t.Fatalf("after refill expected 200, got %d", rr3.Code)
	}
}

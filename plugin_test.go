package traefik_free_tier_plugin

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFreeTierLimiter_NonFreeUser(t *testing.T) {
	cfg := CreateConfig()
	cfg.Rate = 1
	cfg.Burst = 1

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	handler, err := New(context.Background(), next, cfg, "test")
	if err != nil {
		t.Fatal(err)
	}

	// Request without auth should pass through
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}
	}
}

func TestFreeTierLimiter_FreeUser_RateLimited(t *testing.T) {
	cfg := CreateConfig()
	cfg.Rate = 1
	cfg.Burst = 2

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	handler, err := New(context.Background(), next, cfg, "test")
	if err != nil {
		t.Fatal(err)
	}

	freeAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("free:free"))

	// First 2 requests should succeed (burst)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
		req.Header.Set("Authorization", freeAuth)
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: Expected 200, got %d", i, rr.Code)
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req.Header.Set("Authorization", freeAuth)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429, got %d", rr.Code)
	}
}

func TestFreeTierLimiter_DifferentIPs_IndependentLimits(t *testing.T) {
	cfg := CreateConfig()
	cfg.Rate = 1
	cfg.Burst = 1

	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	handler, err := New(context.Background(), next, cfg, "test")
	if err != nil {
		t.Fatal(err)
	}

	freeAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("free:free"))

	// First IP uses its burst
	req1 := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req1.Header.Set("Authorization", freeAuth)
	req1.Header.Set("X-Forwarded-For", "1.1.1.1")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Errorf("First IP first request: Expected 200, got %d", rr1.Code)
	}

	// Second IP should still have its own burst available
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req2.Header.Set("Authorization", freeAuth)
	req2.Header.Set("X-Forwarded-For", "2.2.2.2")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Second IP first request: Expected 200, got %d", rr2.Code)
	}

	// First IP should now be rate limited
	req3 := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	req3.Header.Set("Authorization", freeAuth)
	req3.Header.Set("X-Forwarded-For", "1.1.1.1")
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)

	if rr3.Code != http.StatusTooManyRequests {
		t.Errorf("First IP second request: Expected 429, got %d", rr3.Code)
	}
}

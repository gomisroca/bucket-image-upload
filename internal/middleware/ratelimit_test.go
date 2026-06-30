package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func nextHandlerCalled(t *testing.T) (http.Handler, *bool) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	return h, &called
}

func TestRequireRateLimit_NoURLConfigured_SkipsEntirely(t *testing.T) {
	next, called := nextHandlerCalled(t)
	handler := RequireRateLimit(NewLimiterClient(), "", "", true, next)

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !*called {
		t.Fatal("expected next handler to be called when limiterURL is empty")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequireRateLimit_Allowed_CallsNext(t *testing.T) {
	limiter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"allowed":true,"limit":10,"remaining":9,"resetAfterSeconds":0}`))
	}))
	defer limiter.Close()

	next, called := nextHandlerCalled(t)
	handler := RequireRateLimit(NewLimiterClient(), limiter.URL, "", true, next)

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !*called {
		t.Fatal("expected next handler to be called when limiter allows the request")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequireRateLimit_Denied_ReturnsTooManyRequests(t *testing.T) {
	limiter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"allowed":false,"limit":10,"remaining":0,"resetAfterSeconds":7}`))
	}))
	defer limiter.Close()

	next, called := nextHandlerCalled(t)
	handler := RequireRateLimit(NewLimiterClient(), limiter.URL, "", true, next)

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if *called {
		t.Fatal("expected next handler NOT to be called when limiter denies the request")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "7" {
		t.Fatalf("expected Retry-After to be forwarded as '7', got %q", got)
	}
}

func TestRequireRateLimit_Unreachable_FailOpen_CallsNext(t *testing.T) {
	limiter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	limiterURL := limiter.URL
	limiter.Close() // closed before any request - connection will be refused

	next, called := nextHandlerCalled(t)
	handler := RequireRateLimit(NewLimiterClient(), limiterURL, "", true, next)

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !*called {
		t.Fatal("expected next handler to be called when failOpen=true and limiter is unreachable")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequireRateLimit_Unreachable_FailClosed_Returns503(t *testing.T) {
	limiter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	limiterURL := limiter.URL
	limiter.Close()

	next, called := nextHandlerCalled(t)
	handler := RequireRateLimit(NewLimiterClient(), limiterURL, "", false, next)

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if *called {
		t.Fatal("expected next handler NOT to be called when failOpen=false and limiter is unreachable")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestRequireRateLimit_UnexpectedStatus_TreatedAsOutage(t *testing.T) {
	limiter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // limiter itself is broken, not denying
	}))
	defer limiter.Close()

	next, called := nextHandlerCalled(t)
	handlerFailOpen := RequireRateLimit(NewLimiterClient(), limiter.URL, "", true, next)

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	rec := httptest.NewRecorder()
	handlerFailOpen.ServeHTTP(rec, req)

	if !*called {
		t.Fatal("expected fail-open to call next when the limiter itself errors (not a denial)")
	}
}

func TestRequireRateLimit_ForwardsClientKeyHeaderAsQueryParam(t *testing.T) {
	var receivedKey string
	limiter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.URL.Query().Get("key")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"allowed":true,"limit":10,"remaining":9,"resetAfterSeconds":0}`))
	}))
	defer limiter.Close()

	next, _ := nextHandlerCalled(t)
	handler := RequireRateLimit(NewLimiterClient(), limiter.URL, "", true, next)

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	req.Header.Set("X-Client-Key", "user-42")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if receivedKey != "user-42" {
		t.Fatalf("expected limiter to receive key 'user-42', got %q", receivedKey)
	}
}

func TestRequireRateLimit_FallsBackToIPWhenNoClientKey(t *testing.T) {
	var receivedKey string
	limiter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.URL.Query().Get("key")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"allowed":true,"limit":10,"remaining":9,"resetAfterSeconds":0}`))
	}))
	defer limiter.Close()

	next, _ := nextHandlerCalled(t)
	handler := RequireRateLimit(NewLimiterClient(), limiter.URL, "", true, next)

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	req.RemoteAddr = "203.0.113.7:54321" // no X-Client-Key set
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if receivedKey != "203.0.113.7" {
		t.Fatalf("expected fallback key '203.0.113.7', got %q", receivedKey)
	}
}

func TestRequireRateLimit_SendsAPIKeyHeaderWhenConfigured(t *testing.T) {
	var receivedAPIKey string
	limiter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKey = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"allowed":true,"limit":10,"remaining":9,"resetAfterSeconds":0}`))
	}))
	defer limiter.Close()

	next, _ := nextHandlerCalled(t)
	handler := RequireRateLimit(NewLimiterClient(), limiter.URL, "limiter-secret", true, next)

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if receivedAPIKey != "limiter-secret" {
		t.Fatalf("expected X-API-Key 'limiter-secret' to be sent to limiter, got %q", receivedAPIKey)
	}
}

func TestNewLimiterClient_HasTimeout(t *testing.T) {
	c := NewLimiterClient()
	if c.Timeout != 3*time.Second {
		t.Fatalf("expected 3s timeout, got %v", c.Timeout)
	}
}
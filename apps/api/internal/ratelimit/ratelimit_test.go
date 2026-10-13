package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiterAllowsNormalTrafficAndLimitsExcess(t *testing.T) {
	limiter := New(2, time.Minute, 10)
	for i := 0; i < 2; i++ {
		if allowed, _ := limiter.Allow("192.0.2.1"); !allowed {
			t.Fatalf("request %d was rejected below the limit", i+1)
		}
	}
	allowed, retryAfter := limiter.Allow("192.0.2.1")
	if allowed || retryAfter != 60 {
		t.Fatalf("excess request allowed=%t retryAfter=%d, want false and 60", allowed, retryAfter)
	}
	if allowed, _ := limiter.Allow("192.0.2.2"); !allowed {
		t.Fatal("a different client was not given an independent limit")
	}
}

func TestLimiterExpiresWindow(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	limiter := New(1, time.Minute, 10)
	limiter.now = func() time.Time { return now }
	if allowed, _ := limiter.Allow("192.0.2.1"); !allowed {
		t.Fatal("initial request was rejected")
	}
	if allowed, _ := limiter.Allow("192.0.2.1"); allowed {
		t.Fatal("request above the limit was allowed")
	}
	now = now.Add(time.Minute)
	if allowed, _ := limiter.Allow("192.0.2.1"); !allowed {
		t.Fatal("request was not allowed after the window expired")
	}
}

func TestLimiterConcurrentRequestsRespectLimit(t *testing.T) {
	const limit = 23
	limiter := New(limit, time.Minute, 10)
	var allowedCount atomic.Int32
	var wait sync.WaitGroup
	for i := 0; i < 200; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if allowed, _ := limiter.Allow("192.0.2.1"); allowed {
				allowedCount.Add(1)
			}
		}()
	}
	wait.Wait()
	if got := allowedCount.Load(); got != limit {
		t.Fatalf("concurrent allowed requests=%d, want exactly %d", got, limit)
	}
}

func TestMiddlewareReturns429AndRetryAfter(t *testing.T) {
	limiter := New(1, 30*time.Second, 10)
	var nextCalls atomic.Int32
	handler := Middleware(limiter, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 2; i++ {
		request := httptest.NewRequest(http.MethodPost, "/api/booking-requests", nil)
		request.RemoteAddr = "192.0.2.1:4567"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if i == 0 && response.Code != http.StatusNoContent {
			t.Fatalf("normal request status=%d", response.Code)
		}
		if i == 1 {
			if response.Code != http.StatusTooManyRequests {
				t.Fatalf("excess request status=%d, want 429", response.Code)
			}
			retryAfter, err := strconv.Atoi(response.Header().Get("Retry-After"))
			if err != nil || retryAfter < 1 {
				t.Fatalf("Retry-After=%q, want positive integer", response.Header().Get("Retry-After"))
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("429 response must not be cached")
			}
			if response.Body.Len() == 0 {
				t.Fatal("429 response must explain the rate limit")
			}
		}
	}
	if got := nextCalls.Load(); got != 1 {
		t.Fatalf("next handler calls=%d, want only the allowed request", got)
	}
}

func TestMiddlewareIgnoresForwardingHeaders(t *testing.T) {
	limiter := New(1, time.Minute, 10)
	handler := Middleware(limiter, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	newRequest := func(remoteAddr, forwardedFor string) *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/api/booking-requests", nil)
		request.RemoteAddr = remoteAddr
		request.Header.Set("X-Forwarded-For", forwardedFor)
		request.Header.Set("Forwarded", "for="+forwardedFor)
		return request
	}

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, newRequest("192.0.2.10:1000", "198.51.100.1"))
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request status=%d", first.Code)
	}
	replayed := httptest.NewRecorder()
	handler.ServeHTTP(replayed, newRequest("192.0.2.10:1000", "198.51.100.2"))
	if replayed.Code != http.StatusTooManyRequests {
		t.Fatalf("changing X-Forwarded-For bypassed per-client limit: status=%d", replayed.Code)
	}
	differentRemote := httptest.NewRecorder()
	handler.ServeHTTP(differentRemote, newRequest("192.0.2.11:1000", "198.51.100.1"))
	if differentRemote.Code != http.StatusNoContent {
		t.Fatalf("same forwarded address merged distinct RemoteAddr clients: status=%d", differentRemote.Code)
	}
}

func TestVerifiedProxyRequestsUseDistinctClientIPBuckets(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	limiter := New(1, time.Minute, 10)
	handler := MiddlewareWithProxySecret(limiter, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), secret)
	request := func(clientIP string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/booking-requests", nil)
		r.RemoteAddr = "10.0.0.5:8080"
		r.Header.Set(proxySecretHeader, secret)
		r.Header.Set(proxyClientIPHeader, clientIP)
		return r
	}

	for _, ip := range []string{"198.51.100.1", "198.51.100.2"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request(ip))
		if response.Code != http.StatusNoContent {
			t.Fatalf("verified proxy client %s status=%d, want separate bucket", ip, response.Code)
		}
	}
}

func TestUnverifiedProxyIdentityFallsBackToRemoteAddr(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	for _, test := range []struct {
		name          string
		proxySecret   string
		configuredKey string
	}{
		{name: "missing secret", configuredKey: secret},
		{name: "invalid secret", proxySecret: "not-the-configured-secret", configuredKey: secret},
	} {
		t.Run(test.name, func(t *testing.T) {
			limiter := New(1, time.Minute, 10)
			handler := MiddlewareWithProxySecret(limiter, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}), test.configuredKey)
			request := func(forwardedIP string) *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/api/booking-requests", nil)
				r.RemoteAddr = "192.0.2.8:8080"
				r.Header.Set("X-Forwarded-For", forwardedIP)
				r.Header.Set(proxyClientIPHeader, forwardedIP)
				if test.proxySecret != "" {
					r.Header.Set(proxySecretHeader, test.proxySecret)
				}
				return r
			}

			for i, forwardedIP := range []string{"198.51.100.1", "198.51.100.2"} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request(forwardedIP))
				want := http.StatusNoContent
				if i == 1 {
					want = http.StatusTooManyRequests
				}
				if response.Code != want {
					t.Fatalf("request %d status=%d, want %d (RemoteAddr bucket)", i+1, response.Code, want)
				}
			}
		})
	}
}

func TestVerifiedProxyRejectsMalformedClientIP(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	handler := MiddlewareWithProxySecret(New(1, time.Minute, 10), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), secret)
	request := httptest.NewRequest(http.MethodPost, "/api/booking-requests", nil)
	request.RemoteAddr = "192.0.2.8:8080"
	request.Header.Set(proxySecretHeader, secret)
	request.Header.Set(proxyClientIPHeader, "198.51.100.1, 203.0.113.1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed verified proxy IP status=%d, want 400", response.Code)
	}
}

func TestMissingProxyConfigurationRejectsProxyMetadataButAllowsDirectRequest(t *testing.T) {
	handler := MiddlewareWithProxySecret(New(1, time.Minute, 10), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "")

	proxyRequest := httptest.NewRequest(http.MethodPost, "/api/booking-requests", nil)
	proxyRequest.RemoteAddr = "10.0.0.5:8080"
	proxyRequest.Header.Set(proxySecretHeader, "proxy-is-configured-but-api-is-not")
	proxyRequest.Header.Set(proxyClientIPHeader, "198.51.100.1")
	proxyResponse := httptest.NewRecorder()
	handler.ServeHTTP(proxyResponse, proxyRequest)
	if proxyResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("proxy request without API secret status=%d, want 503", proxyResponse.Code)
	}

	directRequest := httptest.NewRequest(http.MethodPost, "/api/booking-requests", nil)
	directRequest.RemoteAddr = "192.0.2.8:8080"
	directResponse := httptest.NewRecorder()
	handler.ServeHTTP(directResponse, directRequest)
	if directResponse.Code != http.StatusNoContent {
		t.Fatalf("direct request without proxy configuration status=%d, want 204", directResponse.Code)
	}
}

func TestLimiterBoundsStoredClientKeys(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	limiter := New(1, time.Minute, 2)
	limiter.now = func() time.Time { return now }
	for _, key := range []string{"192.0.2.1", "192.0.2.2"} {
		if allowed, _ := limiter.Allow(key); !allowed {
			t.Fatalf("initial client %s was rejected", key)
		}
	}
	if allowed, retryAfter := limiter.Allow("192.0.2.3"); allowed || retryAfter < 1 {
		t.Fatalf("new key allowed=%t retryAfter=%d at full capacity", allowed, retryAfter)
	}
	now = now.Add(time.Minute)
	if allowed, _ := limiter.Allow("192.0.2.3"); !allowed {
		t.Fatal("expired keys were not pruned to admit a new client")
	}
	if len(limiter.entries) > limiter.maxKeys {
		t.Fatalf("retained %d keys, limit is %d", len(limiter.entries), limiter.maxKeys)
	}
}

func TestClientIPParsesCanonicalAddresses(t *testing.T) {
	for _, test := range []struct {
		remote, want string
	}{
		{"192.0.2.4:8443", "192.0.2.4"},
		{"[2001:db8::1]:8443", "2001:db8::1"},
		{"::ffff:192.0.2.4", "192.0.2.4"},
		{"not-an-ip:port", "unknown"},
	} {
		if got := clientIP(test.remote); got != test.want {
			t.Errorf("clientIP(%q)=%q, want %q", test.remote, got, test.want)
		}
	}
}

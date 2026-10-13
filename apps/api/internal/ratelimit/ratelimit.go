package ratelimit

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	proxySecretHeader   = "X-Mmsu-Proxy-Secret"
	proxyClientIPHeader = "X-Mmsu-Client-IP"
)

type entry struct {
	started time.Time
	count   int
}

// Limiter bounds the number of distinct keys retained in process memory.
type Limiter struct {
	mu      sync.Mutex
	entries map[string]entry
	limit   int
	window  time.Duration
	maxKeys int
	now     func() time.Time
}

func New(limit int, window time.Duration, maxKeys int) *Limiter {
	if limit < 1 || window <= 0 || maxKeys < 1 {
		panic("rate limiter requires positive limits, window, and key capacity")
	}
	return &Limiter{
		entries: make(map[string]entry),
		limit:   limit,
		window:  window,
		maxKeys: maxKeys,
		now:     time.Now,
	}
}

// Allow consumes one request for key or returns the seconds until it can retry.
func (l *Limiter) Allow(key string) (bool, int) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	current, ok := l.entries[key]
	if ok && now.Sub(current.started) < l.window {
		if current.count >= l.limit {
			return false, retrySeconds(current.started.Add(l.window).Sub(now))
		}
		current.count++
		l.entries[key] = current
		return true, 0
	}

	if !ok && len(l.entries) >= l.maxKeys {
		l.pruneExpired(now)
		if len(l.entries) >= l.maxKeys {
			return false, l.earliestRetry(now)
		}
	}

	l.entries[key] = entry{started: now, count: 1}
	return true, 0
}

func (l *Limiter) pruneExpired(now time.Time) {
	for key, value := range l.entries {
		if now.Sub(value.started) >= l.window {
			delete(l.entries, key)
		}
	}
}

func (l *Limiter) earliestRetry(now time.Time) int {
	remaining := l.window
	for _, value := range l.entries {
		untilExpiry := value.started.Add(l.window).Sub(now)
		if untilExpiry < remaining {
			remaining = untilExpiry
		}
	}
	return retrySeconds(remaining)
}

func retrySeconds(duration time.Duration) int {
	seconds := int(math.Ceil(duration.Seconds()))
	if seconds < 1 {
		return 1
	}
	return seconds
}

// Middleware derives client identity only from RemoteAddr.
func Middleware(limiter *Limiter, next http.Handler) http.Handler {
	return MiddlewareWithProxySecret(limiter, next, "")
}

// MiddlewareWithProxySecret accepts a proxy-provided client IP only when the
// request also carries the configured server-to-server secret.
func MiddlewareWithProxySecret(limiter *Limiter, next http.Handler, secret string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, status := rateLimitClientIP(r, secret)
		if status != 0 {
			writeProxyError(w, status)
			return
		}
		allowed, retryAfter := limiter.Allow(key)
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "too many requests; retry after the indicated delay"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func rateLimitClientIP(r *http.Request, secret string) (string, int) {
	proxyIP := r.Header.Get(proxyClientIPHeader)
	proxySecret := r.Header.Get(proxySecretHeader)
	if secret == "" {
		if proxySecret != "" {
			return "", http.StatusServiceUnavailable
		}
		return clientIP(r.RemoteAddr), 0
	}

	if proxyIP != "" && secretsMatch(secret, proxySecret) {
		address, err := netip.ParseAddr(strings.TrimSpace(proxyIP))
		if err != nil || address.Zone() != "" {
			return "", http.StatusBadRequest
		}
		return address.Unmap().String(), 0
	}
	return clientIP(r.RemoteAddr), 0
}

func secretsMatch(expected, actual string) bool {
	expectedHash := sha256.Sum256([]byte(expected))
	actualHash := sha256.Sum256([]byte(actual))
	return subtle.ConstantTimeCompare(expectedHash[:], actualHash[:]) == 1
}

func writeProxyError(w http.ResponseWriter, status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if status == http.StatusServiceUnavailable {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "trusted proxy rate-limit configuration is unavailable"})
		return
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "trusted proxy client IP is invalid"})
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = strings.Trim(remoteAddr, "[]")
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	return address.Unmap().String()
}

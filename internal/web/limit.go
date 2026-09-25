package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

type Limiter struct {
	Rate  float64
	Burst float64
	Now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	swept   time.Time
}

func NewLimiter(perMinute, burst float64) *Limiter {
	return &Limiter{Rate: perMinute / 60, Burst: burst, Now: time.Now, buckets: map[string]*bucket{}}
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	if now.Sub(l.swept) > 10*time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.last) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.swept = now
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.Burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.Burst, b.tokens+now.Sub(b.last).Seconds()*l.Rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
			return ip
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (l *Limiter) Middleware(trustProxy bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !l.Allow(clientIP(r, trustProxy)) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "slow down, degen"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

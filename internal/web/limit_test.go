package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewLimiter(60, 3)
	l.Now = func() time.Time { return now }
	for i := range 3 {
		if !l.Allow("a") {
			t.Fatalf("burst request %d denied", i)
		}
	}
	if l.Allow("a") {
		t.Fatal("request past burst allowed")
	}
	if !l.Allow("b") {
		t.Fatal("other key should have its own bucket")
	}
	now = now.Add(time.Second)
	if !l.Allow("a") {
		t.Fatal("token should refill after a second at 60/min")
	}
}

func TestMiddlewareOnlyLimitsWrites(t *testing.T) {
	l := NewLimiter(60, 1)
	h := Middleware(l, l, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	do := func(method string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/", nil)
		req.RemoteAddr = "1.2.3.4:5"
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if do("POST") != 200 || do("POST") != 429 {
		t.Fatal("second POST should be limited")
	}
	if do("GET") != 200 {
		t.Fatal("GET should not be limited")
	}
}

func TestSharedIPUsersHaveSeparateBuckets(t *testing.T) {
	h := Middleware(NewLimiter(60, 1), NewLimiter(60, 1), false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	do := func(cookie string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/", nil)
		req.RemoteAddr = "10.0.0.1:5"
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
		}
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if do("alice") != 200 || do("bob") != 200 || do("alice") != 429 {
		t.Fatal("users behind one campus IP should be limited separately")
	}
}

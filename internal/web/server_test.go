package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jkong7/sidebet/internal/core"
	"github.com/jkong7/sidebet/internal/hub"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newServer(t *testing.T) *httptest.Server {
	st, err := core.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := httptest.NewServer((&Server{Store: st, Hub: hub.New(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Routes())
	t.Cleanup(srv.Close)
	return srv
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestFullFlow(t *testing.T) {
	srv := newServer(t)
	alice, bob, eve := newClient(t, srv), newClient(t, srv), newClient(t, srv)

	if code := alice.do("POST", "/api/groups", map[string]string{"name": "x"}, nil); code != 401 {
		t.Fatalf("anonymous create group = %d", code)
	}
	var anon *core.User
	if code := alice.do("GET", "/api/me", nil, &anon); code != 200 || anon != nil {
		t.Fatalf("anonymous /api/me = %d %+v", code, anon)
	}
	var me core.User
	alice.do("POST", "/api/me", map[string]string{"name": "Alice"}, &me)
	bob.do("POST", "/api/me", map[string]string{"name": "Bob"}, nil)
	eve.do("POST", "/api/me", map[string]string{"name": "Eve"}, nil)

	var g core.Group
	if code := alice.do("POST", "/api/groups", map[string]string{"name": "the boys"}, &g); code != 201 {
		t.Fatalf("create group = %d", code)
	}
	base := "/api/groups/" + g.Code
	if code := bob.do("GET", base+"/markets", nil, nil); code != 403 {
		t.Fatalf("non-member markets = %d", code)
	}
	bob.do("POST", base+"/join", nil, nil)

	var board []core.Member
	alice.do("GET", base+"/leaderboard", nil, &board)
	var bobID int64
	for _, m := range board {
		if m.Name == "Bob" {
			bobID = m.ID
		}
	}

	var m core.Market
	code := alice.do("POST", base+"/markets", map[string]any{"question": "Does Bob text her back?", "subject_id": bobID,
		"closes_in_hours": 24}, &m)
	if code != 201 || m.Subject != "Bob" {
		t.Fatalf("create market = %d %+v", code, m)
	}
	mpath := fmt.Sprintf("%s/markets/%d", base, m.ID)

	var buy struct {
		Trade  core.Trade
		Market core.Market
	}
	if code := bob.do("POST", mpath+"/buy", map[string]any{"side": "no", "amount": 100}, &buy); code != 200 || !buy.Trade.Insider {
		t.Fatalf("insider buy = %d %+v", code, buy.Trade)
	}
	if code := bob.do("POST", mpath+"/buy", map[string]any{"side": "yes", "amount": 99999}, nil); code != 402 {
		t.Fatalf("broke buy = %d", code)
	}
	if code := eve.do("POST", mpath+"/buy", map[string]any{"side": "yes", "amount": 10}, nil); code != 403 {
		t.Fatalf("outsider buy = %d", code)
	}
	if code := bob.do("POST", mpath+"/resolve", map[string]any{"outcome": "yes"}, nil); code != 403 {
		t.Fatalf("non-creator resolve = %d", code)
	}

	var detail struct {
		Market  core.Market
		History []core.Point
		Trades  []core.Trade
	}
	alice.do("GET", mpath, nil, &detail)
	if len(detail.History) != 2 || len(detail.Trades) != 1 || detail.Market.Chance >= 0.5 {
		t.Fatalf("detail = %+v", detail)
	}

	var eg core.Group
	eve.do("POST", "/api/groups", map[string]string{"name": "other"}, &eg)
	if code := eve.do("GET", fmt.Sprintf("/api/groups/%s/markets/%d", eg.Code, m.ID), nil, nil); code != 404 {
		t.Fatalf("cross-group market read = %d", code)
	}

	var res core.Market
	if code := alice.do("POST", mpath+"/resolve", map[string]any{"outcome": "no"}, &res); code != 200 || res.Status != "resolved" {
		t.Fatalf("resolve = %d %+v", code, res)
	}
	var p preview
	eve.do("GET", base, nil, &p)
	if p.Group.Name != "the boys" || p.Members != 2 || p.Member {
		t.Fatalf("preview = %+v", p)
	}
	if code := bob.do("POST", base+"/bailout", nil, nil); code != 200 {
		t.Fatalf("bailout = %d", code)
	}
	if code := bob.do("POST", base+"/bailout", nil, nil); code != 429 {
		t.Fatalf("second bailout = %d", code)
	}
}

func TestEventsStream(t *testing.T) {
	srv := newServer(t)
	a, b := newClient(t, srv), newClient(t, srv)
	a.do("POST", "/api/me", map[string]string{"name": "A"}, nil)
	b.do("POST", "/api/me", map[string]string{"name": "B"}, nil)
	var g core.Group
	a.do("POST", "/api/groups", map[string]string{"name": "g"}, &g)
	b.do("POST", "/api/groups/"+g.Code+"/join", nil, nil)

	req, _ := http.NewRequest("GET", srv.URL+"/api/groups/"+g.Code+"/events", nil)
	resp, err := b.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := make(chan string, 10)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "data: ") {
				lines <- sc.Text()
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	a.do("POST", "/api/groups/"+g.Code+"/markets", map[string]any{"question": "Q?", "closes_in_hours": 1}, nil)
	select {
	case l := <-lines:
		if !strings.Contains(l, `"type":"market"`) || strings.Contains(l, `"mine"`) {
			t.Fatalf("event = %s", l)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event received")
	}
}

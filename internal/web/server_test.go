package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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

type mailbox struct {
	mu   sync.Mutex
	sent map[string]string
}

func (m *mailbox) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent[to] = subject[:6]
	return nil
}

var outbox = &mailbox{sent: map[string]string{}}

func newServer(t *testing.T) *httptest.Server {
	st, err := core.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.EnsureCampus(context.Background(), core.Campus{Code: "nu", Name: "Northwestern",
		Domains: []string{"u.northwestern.edu"}, Admins: []string{"mod@u.northwestern.edu"}}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer((&Server{Store: st, Hub: hub.New(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Mailer: outbox, DevCodes: true}).Routes())
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

	var board core.Board
	alice.do("GET", base+"/leaderboard", nil, &board)
	var bobID int64
	for _, m := range board.Top {
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

func (c *client) verify(email, name string) int {
	c.t.Helper()
	if code := c.do("POST", "/api/verify/start", map[string]string{"email": email, "group": "nu"}, nil); code != 200 {
		return code
	}
	outbox.mu.Lock()
	code := outbox.sent[strings.ToLower(email)]
	outbox.mu.Unlock()
	return c.do("POST", "/api/verify/finish", map[string]string{"email": email, "code": code, "name": name}, nil)
}

func TestCampusFlow(t *testing.T) {
	srv := newServer(t)
	stu, mod, rando := newClient(t, srv), newClient(t, srv), newClient(t, srv)

	var p preview
	rando.do("GET", "/api/groups/nu", nil, &p)
	if !p.Group.Campus() || p.Member {
		t.Fatalf("public preview = %+v", p)
	}
	if code := rando.do("POST", "/api/verify/start", map[string]string{"email": "x@gmail.com", "group": "nu"}, nil); code != 400 {
		t.Fatalf("gmail start = %d", code)
	}
	rando.do("POST", "/api/me", map[string]string{"name": "Rando"}, nil)
	var joinErr struct {
		Error  string
		Verify bool
	}
	if code := rando.do("POST", "/api/groups/nu/join", nil, &joinErr); code != 403 || !joinErr.Verify {
		t.Fatalf("unverified join = %d %+v", code, joinErr)
	}

	if code := stu.verify("Stu@u.northwestern.edu", "Stu"); code != 200 {
		t.Fatalf("verify = %d", code)
	}
	var me meView
	stu.do("GET", "/api/me", nil, &me)
	if !me.Verified || me.Name != "Stu" {
		t.Fatalf("me = %+v", me)
	}
	mod.verify("mod@u.northwestern.edu", "Mod")
	for _, c := range []*client{stu, mod} {
		if code := c.do("POST", "/api/groups/nu/join", nil, nil); code != 200 {
			t.Fatalf("join = %d", code)
		}
	}
	var m core.Market
	var board core.Board
	stu.do("GET", "/api/groups/nu/leaderboard", nil, &board)
	modID := board.Top[0].ID
	if board.Top[0].Name != "Mod" {
		modID = board.Top[1].ID
	}
	if code := stu.do("POST", "/api/groups/nu/markets", map[string]any{"question": "Is Mod single?", "subject_id": modID,
		"closes_in_hours": 24}, nil); code != 400 {
		t.Fatalf("person market = %d", code)
	}
	if code := stu.do("POST", "/api/groups/nu/markets", map[string]any{"question": "Wildcats cover Saturday",
		"closes_in_hours": 24}, &m); code != 201 {
		t.Fatalf("create = %d", code)
	}
	path := fmt.Sprintf("/api/groups/nu/markets/%d", m.ID)
	stu.do("POST", path+"/buy", map[string]any{"side": "yes", "amount": 100}, nil)
	if code := stu.do("POST", path+"/resolve", map[string]string{"outcome": "yes"}, nil); code != 403 {
		t.Fatalf("creator resolve on campus = %d", code)
	}
	if code := stu.do("POST", path+"/report", map[string]string{"reason": "test"}, nil); code != 200 {
		t.Fatalf("report = %d", code)
	}
	if code := stu.do("GET", "/api/groups/nu/reports", nil, nil); code != 403 {
		t.Fatalf("student reading reports = %d", code)
	}
	var queue []core.ReportedMarket
	if code := mod.do("GET", "/api/groups/nu/reports", nil, &queue); code != 200 || len(queue) != 1 {
		t.Fatalf("mod reports = %d %+v", code, queue)
	}
	if code := mod.do("POST", path+"/resolve", map[string]string{"outcome": "yes"}, nil); code != 200 {
		t.Fatalf("mod resolve = %d", code)
	}
	rando.do("GET", "/api/groups/nu", nil, &p)
	if p.Members != 2 {
		t.Fatalf("members = %d", p.Members)
	}

	phone := newClient(t, srv)
	if code := phone.verify("stu@u.northwestern.edu", ""); code != 200 {
		t.Fatalf("second device login = %d", code)
	}
	phone.do("GET", "/api/me", nil, &me)
	if me.Name != "Stu" {
		t.Fatalf("second device user = %+v", me)
	}
}

package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jkong7/sidebet/internal/core"
	"github.com/jkong7/sidebet/internal/hub"
)

const cookieName = "sb"

type Server struct {
	Store  *core.Store
	Hub    *hub.Hub
	Log    *slog.Logger
	Secure bool
}

type ctxKey struct{}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("POST /api/me", s.saveMe)
	mux.HandleFunc("POST /api/groups", s.auth(s.createGroup))
	mux.HandleFunc("GET /api/groups/{code}", s.groupPreview)
	mux.HandleFunc("POST /api/groups/{code}/join", s.auth(s.join))
	mux.HandleFunc("GET /api/groups/{code}/markets", s.member(s.markets))
	mux.HandleFunc("POST /api/groups/{code}/markets", s.member(s.createMarket))
	mux.HandleFunc("GET /api/groups/{code}/leaderboard", s.member(s.leaderboard))
	mux.HandleFunc("GET /api/groups/{code}/feed", s.member(s.feed))
	mux.HandleFunc("POST /api/groups/{code}/bailout", s.member(s.bailout))
	mux.HandleFunc("GET /api/groups/{code}/events", s.member(s.events))
	mux.HandleFunc("GET /api/groups/{code}/markets/{id}", s.member(s.market))
	mux.HandleFunc("POST /api/groups/{code}/markets/{id}/buy", s.member(s.buy))
	mux.HandleFunc("POST /api/groups/{code}/markets/{id}/sell", s.member(s.sell))
	mux.HandleFunc("POST /api/groups/{code}/markets/{id}/resolve", s.member(s.resolve))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	s.pages(mux)
	return headers(mux)
}

func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	status, msg := http.StatusInternalServerError, "something broke"
	switch {
	case errors.Is(err, core.ErrNotFound):
		status, msg = http.StatusNotFound, "not found"
	case errors.Is(err, core.ErrNotMember):
		status, msg = http.StatusForbidden, "join the group first"
	case errors.Is(err, core.ErrForbidden):
		status, msg = http.StatusForbidden, "only the market creator can resolve it"
	case errors.Is(err, core.ErrClosed):
		status, msg = http.StatusConflict, "this market is closed"
	case errors.Is(err, core.ErrBroke):
		status, msg = http.StatusPaymentRequired, "you're broke. take the bailout"
	case errors.Is(err, core.ErrTooSoon):
		status, msg = http.StatusTooManyRequests, "one bailout per day, degen"
	case errors.Is(err, core.ErrInvalidInput):
		status, msg = http.StatusBadRequest, "that doesn't look right"
	default:
		s.Log.Error("request failed", "err", err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return core.ErrInvalidInput
	}
	return nil
}

func (s *Server) currentUser(r *http.Request) (core.User, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return core.User{}, false
	}
	u, err := s.Store.UserByToken(r.Context(), c.Value)
	return u, err == nil
}

func userFrom(ctx context.Context) core.User {
	u, _ := ctx.Value(ctxKey{}).(core.User)
	return u
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.currentUser(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "pick a name first"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	}
}

type groupKey struct{}

func (s *Server) member(next http.HandlerFunc) http.HandlerFunc {
	return s.auth(func(w http.ResponseWriter, r *http.Request) {
		g, err := s.Store.GroupByCode(r.Context(), r.PathValue("code"))
		if err != nil {
			s.fail(w, err)
			return
		}
		ok, err := s.Store.IsMember(r.Context(), g.ID, userFrom(r.Context()).ID)
		if err != nil {
			s.fail(w, err)
			return
		}
		if !ok {
			s.fail(w, core.ErrNotMember)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), groupKey{}, g)))
	})
}

func groupFrom(ctx context.Context) core.Group {
	g, _ := ctx.Value(groupKey{}).(core.Group)
	return g
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) saveMe(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if u, ok := s.currentUser(r); ok {
		if err := s.Store.Rename(r.Context(), u.ID, in.Name); err != nil {
			s.fail(w, err)
			return
		}
		u, _ = s.currentUser(r)
		writeJSON(w, http.StatusOK, u)
		return
	}
	u, token, err := s.Store.CreateUser(r.Context(), in.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.Secure,
		SameSite: http.SameSiteLaxMode, MaxAge: 400 * 24 * 3600})
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	g, err := s.Store.CreateGroup(r.Context(), userFrom(r.Context()).ID, in.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

type preview struct {
	Group   core.Group   `json:"group"`
	Members int          `json:"members"`
	Member  bool         `json:"member"`
	Hottest *core.Market `json:"hottest,omitempty"`
}

func (s *Server) buildPreview(ctx context.Context, g core.Group, viewer int64) (preview, error) {
	p := preview{Group: g}
	board, err := s.Store.Leaderboard(ctx, g.ID)
	if err != nil {
		return p, err
	}
	p.Members = len(board)
	for _, m := range board {
		if m.ID == viewer {
			p.Member = true
		}
	}
	list, err := s.Store.Markets(ctx, g.ID, 0)
	if err != nil {
		return p, err
	}
	for i := range list {
		if list[i].Status == "open" && (p.Hottest == nil || list[i].Volume > p.Hottest.Volume) {
			p.Hottest = &list[i]
		}
	}
	return p, nil
}

func (s *Server) groupPreview(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.GroupByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		s.fail(w, err)
		return
	}
	u, _ := s.currentUser(r)
	p, err := s.buildPreview(r.Context(), g, u.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.GroupByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		s.fail(w, err)
		return
	}
	u := userFrom(r.Context())
	already, _ := s.Store.IsMember(r.Context(), g.ID, u.ID)
	if err := s.Store.Join(r.Context(), g.ID, u.ID); err != nil {
		s.fail(w, err)
		return
	}
	if !already {
		s.Hub.Publish(g.ID, "member", u)
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) markets(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Markets(r.Context(), groupFrom(r.Context()).ID, userFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if list == nil {
		list = []core.Market{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createMarket(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Question      string  `json:"question"`
		SubjectID     *int64  `json:"subject_id"`
		ClosesInHours float64 `json:"closes_in_hours"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if in.ClosesInHours <= 0 || in.ClosesInHours > 24*365 {
		s.fail(w, core.ErrInvalidInput)
		return
	}
	g, u := groupFrom(r.Context()), userFrom(r.Context())
	m, err := s.Store.CreateMarket(r.Context(), g.ID, u.ID, core.NewMarket{Question: in.Question, SubjectID: in.SubjectID,
		ClosesAt: s.Store.Now().Add(time.Duration(in.ClosesInHours * float64(time.Hour)))})
	if err != nil {
		s.fail(w, err)
		return
	}
	pub := m
	pub.Mine = nil
	s.Hub.Publish(g.ID, "market", pub)
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) marketInGroup(r *http.Request) (core.Market, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return core.Market{}, core.ErrNotFound
	}
	m, err := s.Store.Market(r.Context(), id, userFrom(r.Context()).ID)
	if err != nil {
		return m, err
	}
	if m.GroupID != groupFrom(r.Context()).ID {
		return m, core.ErrNotFound
	}
	return m, nil
}

func (s *Server) market(w http.ResponseWriter, r *http.Request) {
	m, err := s.marketInGroup(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	history, err := s.Store.History(r.Context(), m)
	if err != nil {
		s.fail(w, err)
		return
	}
	trades, err := s.Store.Trades(r.Context(), m.GroupID, m.ID, 50)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"market": m, "history": history, "trades": trades})
}

func (s *Server) afterTrade(w http.ResponseWriter, r *http.Request, t core.Trade) {
	g := groupFrom(r.Context())
	m, err := s.Store.Market(r.Context(), t.MarketID, userFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	pub := m
	pub.Mine = nil
	t.User = userFrom(r.Context()).Name
	s.Hub.Publish(g.ID, "trade", map[string]any{"trade": t, "market": pub})
	writeJSON(w, http.StatusOK, map[string]any{"trade": t, "market": m})
}

func (s *Server) buy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Side   string  `json:"side"`
		Amount float64 `json:"amount"`
	}
	m, err := s.marketInGroup(r)
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil && in.Side != "yes" && in.Side != "no" {
		err = core.ErrInvalidInput
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	t, err := s.Store.Buy(r.Context(), m.ID, userFrom(r.Context()).ID, in.Side == "yes", in.Amount)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.afterTrade(w, r, t)
}

func (s *Server) sell(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Side   string  `json:"side"`
		Shares float64 `json:"shares"`
	}
	m, err := s.marketInGroup(r)
	if err == nil {
		err = decode(r, &in)
	}
	if err == nil && in.Side != "yes" && in.Side != "no" {
		err = core.ErrInvalidInput
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	t, err := s.Store.Sell(r.Context(), m.ID, userFrom(r.Context()).ID, in.Side == "yes", in.Shares)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.afterTrade(w, r, t)
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	var in struct{ Outcome string }
	m, err := s.marketInGroup(r)
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.Store.Resolve(r.Context(), m.ID, userFrom(r.Context()).ID, in.Outcome)
	if err != nil {
		s.fail(w, err)
		return
	}
	pub := res
	pub.Mine = nil
	s.Hub.Publish(m.GroupID, "resolve", pub)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) leaderboard(w http.ResponseWriter, r *http.Request) {
	board, err := s.Store.Leaderboard(r.Context(), groupFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}

func (s *Server) feed(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Trades(r.Context(), groupFrom(r.Context()).ID, 0, 50)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) bailout(w http.ResponseWriter, r *http.Request) {
	g, u := groupFrom(r.Context()), userFrom(r.Context())
	coins, err := s.Store.Bailout(r.Context(), g.ID, u.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.Hub.Publish(g.ID, "bailout", map[string]any{"user": u})
	writeJSON(w, http.StatusOK, map[string]float64{"coins": coins})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	g := groupFrom(r.Context())
	ch, unsub := s.Hub.Subscribe(g.ID)
	defer unsub()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

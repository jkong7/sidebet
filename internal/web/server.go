package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/jkong7/sidebet/internal/core"
	"github.com/jkong7/sidebet/internal/hub"
	"github.com/jkong7/sidebet/internal/mail"
)

const cookieName = "sb"

type Server struct {
	Store      *core.Store
	Hub        *hub.Hub
	Log        *slog.Logger
	Secure     bool
	TrustProxy bool
	Limiter    *Limiter
	Mailer     mail.Sender
	DevCodes   bool
}

type ctxKey struct{}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("POST /api/me", s.saveMe)
	mux.HandleFunc("POST /api/verify/start", s.verifyStart)
	mux.HandleFunc("POST /api/verify/finish", s.verifyFinish)
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
	mux.HandleFunc("POST /api/groups/{code}/markets/{id}/report", s.member(s.report))
	mux.HandleFunc("GET /api/groups/{code}/reports", s.member(s.reports))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	s.pages(mux)
	var h http.Handler = mux
	if s.Limiter != nil {
		h = s.Limiter.Middleware(s.TrustProxy, h)
	}
	return headers(h)
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
		status, msg = http.StatusForbidden, "you can't settle this one"
	case errors.Is(err, core.ErrUnverified):
		status, msg = http.StatusForbidden, "verify your school email first"
	case errors.Is(err, core.ErrNoPeople):
		status, msg = http.StatusBadRequest, "campus bets can't be about a specific person. bet on events, not people"
	case errors.Is(err, core.ErrBlocked):
		status, msg = http.StatusUnprocessableEntity, "that got blocked. no slurs, phone numbers or emails"
	case errors.Is(err, core.ErrBadCode):
		status, msg = http.StatusBadRequest, "wrong or expired code"
	case errors.Is(err, core.ErrSlowDown):
		status, msg = http.StatusTooManyRequests, "we just sent a code. check your inbox or wait a minute"
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

type meView struct {
	core.User
	Verified bool `json:"verified"`
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	email, _ := s.Store.Email(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, meView{User: u, Verified: email != ""})
}

func (s *Server) setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.Secure,
		SameSite: http.SameSiteLaxMode, MaxAge: 400 * 24 * 3600})
}

func (s *Server) verifyStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Group string `json:"group"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	g, err := s.Store.GroupByCode(r.Context(), in.Group)
	if err != nil {
		s.fail(w, err)
		return
	}
	email, err := core.NormalizeEmail(in.Email)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !g.Campus() || !core.DomainAllowed(email, g.Domains) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "use your @" + firstDomain(g) + " email"})
		return
	}
	code, err := s.Store.StartVerification(r.Context(), email)
	if err != nil {
		s.fail(w, err)
		return
	}
	subject, body := mail.CodeEmail(g.Name, code)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.Mailer.Send(ctx, email, subject, body); err != nil {
		s.Log.Error("send code", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "couldn't send the email. try again"})
		return
	}
	out := map[string]any{"sent": true}
	if s.DevCodes {
		out["dev_code"] = code
	}
	writeJSON(w, http.StatusOK, out)
}

func firstDomain(g core.Group) string {
	if len(g.Domains) > 0 {
		return g.Domains[0]
	}
	return "school"
}

func (s *Server) verifyFinish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Code  string `json:"code"`
		Name  string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	cur, _ := s.currentUser(r)
	v, err := s.Store.FinishVerification(r.Context(), in.Email, in.Code, cur.ID, in.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	if v.Token != "" {
		s.setSession(w, v.Token)
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": meView{User: v.User, Verified: true}, "returning": v.Existing})
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
	s.setSession(w, token)
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
	Group   core.Group    `json:"group"`
	Members int           `json:"members"`
	Member  bool          `json:"member"`
	Role    string        `json:"role,omitempty"`
	Hottest *core.Market  `json:"hottest,omitempty"`
	Top     []core.Market `json:"top,omitempty"`
}

func (s *Server) buildPreview(ctx context.Context, g core.Group, viewer int64) (preview, error) {
	p := preview{Group: g}
	n, err := s.Store.MemberCount(ctx, g.ID)
	if err != nil {
		return p, err
	}
	p.Members = n
	if viewer != 0 {
		if role, err := s.Store.Role(ctx, g.ID, viewer); err == nil {
			p.Member, p.Role = true, role
		}
	}
	list, err := s.Store.Markets(ctx, g.ID, 0)
	if err != nil {
		return p, err
	}
	var open []core.Market
	for i := range list {
		if list[i].Status == "open" {
			open = append(open, list[i])
			if p.Hottest == nil || list[i].Volume > p.Hottest.Volume {
				p.Hottest = &list[i]
			}
		}
	}
	if g.Campus() {
		sort.SliceStable(open, func(i, j int) bool { return open[i].Volume > open[j].Volume })
		p.Top = open[:min(5, len(open))]
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
		if errors.Is(err, core.ErrUnverified) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "verify your @" + firstDomain(g) + " email first", "verify": true})
			return
		}
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
	g := groupFrom(r.Context())
	limit := 500
	if g.Campus() {
		limit = 100
	}
	board, err := s.Store.Board(r.Context(), g.ID, userFrom(r.Context()).ID, limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	m, err := s.marketInGroup(r)
	if err == nil {
		err = decode(r, &in)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	n, err := s.Store.Report(r.Context(), m.ID, userFrom(r.Context()).ID, in.Reason)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.Log.Warn("market reported", "market", m.ID, "reports", n, "reason", in.Reason)
	writeJSON(w, http.StatusOK, map[string]int{"reports": n})
}

func (s *Server) reports(w http.ResponseWriter, r *http.Request) {
	g, u := groupFrom(r.Context()), userFrom(r.Context())
	role, err := s.Store.Role(r.Context(), g.ID, u.ID)
	if err != nil || (role != "admin" && g.CreatedBy != u.ID) {
		s.fail(w, core.ErrForbidden)
		return
	}
	list, err := s.Store.Reported(r.Context(), g.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
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

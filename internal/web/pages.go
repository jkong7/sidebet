package web

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"net/http"
	"strconv"

	"github.com/jkong7/sidebet/internal/card"
	"github.com/jkong7/sidebet/internal/core"
)

//go:embed static
var static embed.FS

var page = template.Must(template.ParseFS(static, "static/index.html"))

type meta struct {
	Title       string
	Description string
	Image       string
	URL         string
}

func (s *Server) origin(r *http.Request) string {
	scheme := "http"
	if s.Secure || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, m meta) {
	if m.Title == "" {
		m.Title = "sidebet: put odds on your friends"
	}
	if m.Description == "" {
		m.Description = "Group-chat prediction markets. Bet play money on your friends' lives. Odds move live."
	}
	if m.Image == "" {
		m.Image = "/og/default"
	}
	o := s.origin(r)
	m.Image, m.URL = o+m.Image, o+r.URL.Path
	var buf bytes.Buffer
	if err := page.Execute(&buf, m); err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(buf.Bytes())
}

func (s *Server) pages(mux *http.ServeMux) {
	assets, _ := fs.Sub(static, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(assets)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { s.render(w, r, meta{}) })
	mux.HandleFunc("GET /g/{code}", s.groupPage)
	mux.HandleFunc("GET /g/{code}/m/{id}", s.marketPage)
	mux.HandleFunc("GET /og/default", func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		card.RenderMarket(&buf, card.Market{Question: "Will your friends find out you bet against them?", Chance: 0.87,
			Group: "put odds on your friends", Status: "open", Traders: 6, Volume: 4200,
			History: []float64{0.5, 0.58, 0.52, 0.66, 0.71, 0.87}})
		pngHeaders(w)
		w.Write(buf.Bytes())
	})
	mux.HandleFunc("GET /og/g/{code}", s.groupCard)
	mux.HandleFunc("GET /og/g/{code}/m/{id}", s.marketCard)
}

func pct(p float64) int { return int(math.Round(p * 100)) }

func (s *Server) publicMarket(ctx context.Context, code, id string) (core.Group, core.Market, error) {
	g, err := s.Store.GroupByCode(ctx, code)
	if err != nil {
		return g, core.Market{}, err
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return g, core.Market{}, core.ErrNotFound
	}
	m, err := s.Store.Market(ctx, n, 0)
	if err == nil && m.GroupID != g.ID {
		err = core.ErrNotFound
	}
	return g, m, err
}

func (s *Server) groupPage(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.GroupByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		s.render(w, r, meta{})
		return
	}
	s.render(w, r, meta{
		Title:       "Join " + g.Name + " on sidebet",
		Description: "Your friends are betting on each other. Get in before they bet on you.",
		Image:       "/og/g/" + g.Code,
	})
}

func (s *Server) marketPage(w http.ResponseWriter, r *http.Request) {
	g, m, err := s.publicMarket(r.Context(), r.PathValue("code"), r.PathValue("id"))
	if err != nil {
		s.render(w, r, meta{})
		return
	}
	desc := fmt.Sprintf("%d%% chance · %d betting in %s. Think they're wrong? Bet against them.", pct(m.Chance), m.Traders, g.Name)
	if m.Status != "open" {
		desc = fmt.Sprintf("Settled: %s. %s", m.Outcome, g.Name)
	}
	s.render(w, r, meta{Title: m.Question, Description: desc, Image: fmt.Sprintf("/og/g/%s/m/%d", g.Code, m.ID)})
}

func pngHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=60")
}

func (s *Server) marketCard(w http.ResponseWriter, r *http.Request) {
	g, m, err := s.publicMarket(r.Context(), r.PathValue("code"), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	hist, _ := s.Store.History(r.Context(), m)
	pts := make([]float64, len(hist))
	for i, h := range hist {
		pts[i] = h.Chance
	}
	var buf bytes.Buffer
	if err := card.RenderMarket(&buf, card.Market{Question: m.Question, Chance: m.Chance, Group: g.Name, Status: m.Status,
		Outcome: m.Outcome, Volume: m.Volume, Traders: m.Traders, History: pts}); err != nil {
		s.fail(w, err)
		return
	}
	pngHeaders(w)
	w.Write(buf.Bytes())
}

func (s *Server) groupCard(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.GroupByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := s.buildPreview(r.Context(), g, 0)
	if err != nil {
		s.fail(w, err)
		return
	}
	c := card.Group{Name: g.Name, Members: p.Members}
	if p.Hottest != nil {
		c.Hottest, c.Chance = p.Hottest.Question, p.Hottest.Chance
	}
	var buf bytes.Buffer
	if err := card.RenderGroup(&buf, c); err != nil {
		s.fail(w, err)
		return
	}
	pngHeaders(w)
	w.Write(buf.Bytes())
}

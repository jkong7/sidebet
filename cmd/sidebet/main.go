package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jkong7/sidebet/internal/core"
	"github.com/jkong7/sidebet/internal/hub"
	"github.com/jkong7/sidebet/internal/mail"
	"github.com/jkong7/sidebet/internal/web"
)

func campusFromEnv() core.Campus {
	c := core.Campus{Code: "nu", Name: "Northwestern", Domains: []string{"u.northwestern.edu", "northwestern.edu"}}
	if v := os.Getenv("CAMPUS"); v != "" {
		parts := strings.SplitN(v, "|", 3)
		if len(parts) == 3 {
			c = core.Campus{Code: parts[0], Name: parts[1], Domains: strings.Split(parts[2], ",")}
		}
	}
	if v := os.Getenv("CAMPUS_ADMINS"); v != "" {
		c.Admins = strings.Split(v, ",")
	}
	return c
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := flag.String("addr", env("ADDR", ":8090"), "listen address")
	dbPath := flag.String("db", env("DB_PATH", "sidebet.db"), "SQLite database path")
	secure := flag.Bool("secure", os.Getenv("SECURE") == "1", "set Secure on cookies (behind HTTPS)")
	trustProxy := flag.Bool("trust-proxy", os.Getenv("TRUST_PROXY") == "1", "read client IPs from proxy headers")
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	store, err := core.Open(*dbPath)
	if err != nil {
		log.Error("open database", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	campus := campusFromEnv()
	if _, err := store.EnsureCampus(ctx, campus); err != nil {
		log.Error("campus", "err", err)
		os.Exit(1)
	}
	var mailer mail.Sender = mail.Log{Logger: log}
	if host := os.Getenv("SMTP_HOST"); host != "" {
		mailer = mail.SMTP{Host: host, Port: env("SMTP_PORT", "587"), User: os.Getenv("SMTP_USER"), Pass: os.Getenv("SMTP_PASS"),
			From: env("MAIL_FROM", "sidebet <codes@sidebet.app>")}
	} else {
		log.Warn("SMTP_HOST not set; verification codes are logged, not emailed")
	}
	devCodes := os.Getenv("DEV_CODES") == "1" && !*secure
	log.Info("campus ready", "code", campus.Code, "name", campus.Name, "domains", campus.Domains, "admins", len(campus.Admins))
	srv := &http.Server{
		Addr: *addr,
		Handler: (&web.Server{Store: store, Hub: hub.New(), Log: log, Secure: *secure, TrustProxy: *trustProxy,
			Limiter: web.NewLimiter(60, 20), Mailer: mailer, DevCodes: devCodes}).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Info("sidebet listening", "addr", *addr, "db", *dbPath)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}

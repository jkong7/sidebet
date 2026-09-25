package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jkong7/sidebet/internal/core"
	"github.com/jkong7/sidebet/internal/hub"
	"github.com/jkong7/sidebet/internal/web"
)

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
	srv := &http.Server{
		Addr:              *addr,
		Handler:           (&web.Server{Store: store, Hub: hub.New(), Log: log, Secure: *secure, TrustProxy: *trustProxy,
			Limiter: web.NewLimiter(60, 20)}).Routes(),
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

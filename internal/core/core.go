package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

var (
	ErrNotFound     = errors.New("not found")
	ErrNotMember    = errors.New("not a member of this group")
	ErrForbidden    = errors.New("not allowed")
	ErrClosed       = errors.New("market is closed")
	ErrBroke        = errors.New("not enough coins")
	ErrInvalidInput = errors.New("invalid input")
	ErrTooSoon      = errors.New("bailout already claimed today")
	ErrUnverified   = errors.New("verify a campus email first")
	ErrNoPeople     = errors.New("campus markets can't be about individual people")
	ErrBadCode      = errors.New("wrong or expired code")
	ErrSlowDown     = errors.New("wait before requesting another code")
	ErrBlocked      = errors.New("blocked by moderation")
)

const (
	StartingCoins = 1000.0
	BailoutCoins  = 100.0
	BailoutEvery  = 20 * time.Hour
	DefaultB      = 150.0
)

type Store struct {
	db  *sql.DB
	Now func() time.Time

	adminMu sync.RWMutex
	admins  map[int64]map[string]bool
}

func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, Now: time.Now, admins: map[int64]map[string]bool{}}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) now() int64 { return s.Now().UnixMilli() }

func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

const codeAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

func randomCode(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(b)
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func cleanText(s string, max int) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || len([]rune(s)) > max {
		return "", ErrInvalidInput
	}
	return s, nil
}

func membership(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotMember
	}
	return err
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

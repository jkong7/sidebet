package core

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

const (
	CodeTTL      = 10 * time.Minute
	CodeResend   = 60 * time.Second
	CodeAttempts = 5
	systemToken  = "system"
)

type Campus struct {
	Code    string
	Name    string
	Domains []string
	Admins  []string
}

func NormalizeEmail(email string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil || !strings.Contains(addr.Address, "@") || len(addr.Address) > 254 {
		return "", ErrInvalidInput
	}
	return strings.ToLower(addr.Address), nil
}

func DomainAllowed(email string, domains []string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	host := strings.ToLower(email[at+1:])
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" && host == d {
			return true
		}
	}
	return false
}

func (s *Store) systemUser(ctx context.Context, tx *sql.Tx) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE token_hash = ?`, systemToken).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO users (name, token_hash, created_at) VALUES ('sidebet', ?, ?)`, systemToken, s.now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) EnsureCampus(ctx context.Context, c Campus) (Group, error) {
	code := strings.ToLower(strings.TrimSpace(c.Code))
	if code == "" || c.Name == "" || len(c.Domains) == 0 {
		return Group{}, ErrInvalidInput
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		sys, err := s.systemUser(ctx, tx)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO groups (code, name, kind, domains, created_by, created_at)
			VALUES (?, ?, 'campus', ?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET name = excluded.name, kind = 'campus', domains = excluded.domains`,
			code, c.Name, strings.Join(c.Domains, ","), sys, s.now())
		return err
	})
	if err != nil {
		return Group{}, err
	}
	g, err := s.GroupByCode(ctx, code)
	if err != nil {
		return g, err
	}
	admins := map[string]bool{}
	for _, a := range c.Admins {
		if e, err := NormalizeEmail(a); err == nil {
			admins[e] = true
		}
	}
	s.adminMu.Lock()
	s.admins[g.ID] = admins
	s.adminMu.Unlock()
	if _, err := s.db.ExecContext(ctx, `UPDATE members SET role = CASE WHEN (SELECT email FROM users WHERE id = members.user_id)
		IN (SELECT value FROM json_each(?)) THEN 'admin' ELSE 'member' END WHERE group_id = ?`, jsonList(admins), g.ID); err != nil {
		return g, err
	}
	return g, nil
}

func jsonList(set map[string]bool) string {
	var b strings.Builder
	b.WriteString("[")
	first := true
	for k := range set {
		if !first {
			b.WriteString(",")
		}
		first = false
		b.WriteString(`"` + strings.ReplaceAll(k, `"`, "") + `"`)
	}
	b.WriteString("]")
	return b.String()
}

func (s *Store) isAdmin(groupID int64, email string) bool {
	s.adminMu.RLock()
	defer s.adminMu.RUnlock()
	return s.admins[groupID][strings.ToLower(email)]
}

func (s *Store) Email(ctx context.Context, userID int64) (string, error) {
	var email sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id = ?`, userID).Scan(&email)
	return email.String, notFound(err)
}

func sixDigits() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	return fmt.Sprintf("%06d", n.Int64())
}

func (s *Store) StartVerification(ctx context.Context, email string) (string, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return "", err
	}
	code := sixDigits()
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var sent int64
		err := tx.QueryRowContext(ctx, `SELECT sent_at FROM verifications WHERE email = ?`, email).Scan(&sent)
		if err == nil && s.now()-sent < CodeResend.Milliseconds() {
			return ErrSlowDown
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO verifications (email, code_hash, expires_at, attempts, sent_at) VALUES (?, ?, ?, 0, ?)
			ON CONFLICT(email) DO UPDATE SET code_hash = excluded.code_hash, expires_at = excluded.expires_at, attempts = 0,
			sent_at = excluded.sent_at`, email, hashToken(email+":"+code), s.Now().Add(CodeTTL).UnixMilli(), s.now())
		return err
	})
	return code, err
}

type Verified struct {
	User     User
	Token    string
	Existing bool
}

func (s *Store) FinishVerification(ctx context.Context, email, code string, currentUserID int64, name string) (Verified, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return Verified{}, err
	}
	var out Verified
	wrong := false
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var hash string
		var expires int64
		var attempts int
		if err := tx.QueryRowContext(ctx, `SELECT code_hash, expires_at, attempts FROM verifications WHERE email = ?`, email).
			Scan(&hash, &expires, &attempts); err != nil {
			return notFoundAs(err, ErrBadCode)
		}
		if attempts >= CodeAttempts || s.now() > expires {
			return ErrBadCode
		}
		if subtle.ConstantTimeCompare([]byte(hash), []byte(hashToken(email+":"+strings.TrimSpace(code)))) != 1 {
			wrong = true
			_, err := tx.ExecContext(ctx, `UPDATE verifications SET attempts = attempts + 1 WHERE email = ?`, email)
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM verifications WHERE email = ?`, email); err != nil {
			return err
		}
		var owner User
		err := tx.QueryRowContext(ctx, `SELECT id, name FROM users WHERE email = ?`, email).Scan(&owner.ID, &owner.Name)
		switch {
		case err == nil:
			if owner.ID != currentUserID {
				token := randomString(32)
				if _, err := tx.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, created_at) VALUES (?, ?, ?)`,
					hashToken(token), owner.ID, s.now()); err != nil {
					return err
				}
				out.Token = token
			}
			out.User, out.Existing = owner, true
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		if currentUserID != 0 {
			var cur User
			var existing sql.NullString
			if err := tx.QueryRowContext(ctx, `SELECT id, name, email FROM users WHERE id = ?`, currentUserID).
				Scan(&cur.ID, &cur.Name, &existing); err != nil {
				return notFound(err)
			}
			if !existing.Valid {
				if _, err := tx.ExecContext(ctx, `UPDATE users SET email = ? WHERE id = ?`, email, cur.ID); err != nil {
					return err
				}
				out.User = cur
				return nil
			}
		}
		clean, err := cleanText(name, 24)
		if err != nil {
			return err
		}
		token := randomString(32)
		res, err := tx.ExecContext(ctx, `INSERT INTO users (name, token_hash, email, created_at) VALUES (?, ?, ?, ?)`,
			clean, hashToken(token), email, s.now())
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		out.User, out.Token = User{ID: id, Name: clean}, token
		return nil
	})
	if err == nil && wrong {
		return Verified{}, ErrBadCode
	}
	return out, err
}

var blocked = regexp.MustCompile(`(?i)\b(n[i1!]gg(a|[e3]r)s?|f[a@4]gg?([o0]t)?s?|r[e3]t[a@4]rd([e3]d|s)?|k[i1!]k[e3]|sp[i1!]c|ch[i1!]nk|tr[a@4]nn(y|[i1!][e3]s)|kys|kill yourself)\b`)
var contact = regexp.MustCompile(`(?i)(\b\d{3}[-.\s]?\d{3}[-.\s]?\d{4}\b|\b[\w.+-]+@[\w-]+\.[\w.]+\b)`)

func Allowed(text string) bool {
	return !blocked.MatchString(text) && !contact.MatchString(text)
}

func (s *Store) Report(ctx context.Context, marketID, userID int64, reason string) (int, error) {
	reason, err := cleanText(reason, 200)
	if err != nil {
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO reports (market_id, user_id, reason, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT DO NOTHING`, marketID, userID, reason, s.now()); err != nil {
		return 0, err
	}
	var n int
	err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM reports WHERE market_id = ?`, marketID).Scan(&n)
	return n, err
}

type ReportedMarket struct {
	Market  Market   `json:"market"`
	Reports int      `json:"reports"`
	Reasons []string `json:"reasons"`
}

func (s *Store) Reported(ctx context.Context, groupID int64) ([]ReportedMarket, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.market_id, count(*), group_concat(r.reason, ' | ') FROM reports r
		JOIN markets m ON m.id = r.market_id WHERE m.group_id = ? AND m.status = 'open' GROUP BY r.market_id
		ORDER BY count(*) DESC`, groupID)
	if err != nil {
		return nil, err
	}
	type row struct {
		id      int64
		n       int
		reasons string
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.n, &r.reasons); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, r)
	}
	rows.Close()
	out := []ReportedMarket{}
	for _, r := range list {
		m, err := s.Market(ctx, r.id, 0)
		if err != nil {
			return nil, err
		}
		out = append(out, ReportedMarket{Market: m, Reports: r.n, Reasons: strings.Split(r.reasons, " | ")})
	}
	return out, nil
}

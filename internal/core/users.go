package core

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Group struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Domains   []string  `json:"domains,omitempty"`
	CreatedBy int64     `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (g Group) Campus() bool { return g.Kind == "campus" }

type Member struct {
	User
	Coins     float64   `json:"coins"`
	NetWorth  float64   `json:"net_worth"`
	LastBonus time.Time `json:"last_bonus"`
	JoinedAt  time.Time `json:"joined_at"`
	OpenBets  int       `json:"open_bets"`
	Rank      int       `json:"rank,omitempty"`
	Title     string    `json:"title,omitempty"`
}

func (s *Store) CreateUser(ctx context.Context, name string) (User, string, error) {
	name, err := cleanText(name, 24)
	if err != nil {
		return User{}, "", err
	}
	token := randomString(32)
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (name, token_hash, created_at) VALUES (?, ?, ?)`,
		name, hashToken(token), s.now())
	if err != nil {
		return User{}, "", err
	}
	id, _ := res.LastInsertId()
	return User{ID: id, Name: name}, token, nil
}

func (s *Store) UserByToken(ctx context.Context, token string) (User, error) {
	var u User
	h := hashToken(token)
	err := s.db.QueryRowContext(ctx, `SELECT id, name FROM users WHERE token_hash = ?
		UNION ALL SELECT u.id, u.name FROM sessions se JOIN users u ON u.id = se.user_id WHERE se.token_hash = ? LIMIT 1`,
		h, h).Scan(&u.ID, &u.Name)
	return u, notFound(err)
}

func (s *Store) Rename(ctx context.Context, userID int64, name string) error {
	name, err := cleanText(name, 24)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET name = ? WHERE id = ?`, name, userID)
	return err
}

func (s *Store) CreateGroup(ctx context.Context, userID int64, name string) (Group, error) {
	name, err := cleanText(name, 40)
	if err != nil {
		return Group{}, err
	}
	g := Group{Name: name, Kind: "friends", CreatedBy: userID, CreatedAt: time.UnixMilli(s.now())}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		for range 5 {
			g.Code = randomCode(6)
			res, err := tx.ExecContext(ctx, `INSERT INTO groups (code, name, created_by, created_at) VALUES (?, ?, ?, ?)
				ON CONFLICT(code) DO NOTHING`, g.Code, g.Name, userID, g.CreatedAt.UnixMilli())
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 1 {
				g.ID, _ = res.LastInsertId()
				_, err = tx.ExecContext(ctx, `INSERT INTO members (group_id, user_id, coins, joined_at) VALUES (?, ?, ?, ?)`,
					g.ID, userID, StartingCoins, s.now())
				return err
			}
		}
		return ErrInvalidInput
	})
	return g, err
}

func (s *Store) GroupByCode(ctx context.Context, code string) (Group, error) {
	var g Group
	var created int64
	var domains string
	err := s.db.QueryRowContext(ctx, `SELECT id, code, name, kind, domains, created_by, created_at FROM groups WHERE code = ?`,
		strings.ToLower(code)).Scan(&g.ID, &g.Code, &g.Name, &g.Kind, &domains, &g.CreatedBy, &created)
	g.CreatedAt = time.UnixMilli(created)
	if domains != "" {
		g.Domains = strings.Split(domains, ",")
	}
	return g, notFound(err)
}

func (s *Store) Join(ctx context.Context, groupID, userID int64) error {
	var kind, domains string
	if err := s.db.QueryRowContext(ctx, `SELECT kind, domains FROM groups WHERE id = ?`, groupID).Scan(&kind, &domains); err != nil {
		return notFound(err)
	}
	role := "member"
	if kind == "campus" {
		email, err := s.Email(ctx, userID)
		if err != nil {
			return err
		}
		if !DomainAllowed(email, strings.Split(domains, ",")) {
			return ErrUnverified
		}
		if s.isAdmin(groupID, email) {
			role = "admin"
		}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO members (group_id, user_id, coins, joined_at, role) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`, groupID, userID, StartingCoins, s.now(), role)
	return err
}

func (s *Store) Role(ctx context.Context, groupID, userID int64) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx, `SELECT role FROM members WHERE group_id = ? AND user_id = ?`, groupID, userID).Scan(&role)
	return role, membership(err)
}

func (s *Store) MemberCount(ctx context.Context, groupID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM members WHERE group_id = ?`, groupID).Scan(&n)
	return n, err
}

func (s *Store) IsMember(ctx context.Context, groupID, userID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM members WHERE group_id = ? AND user_id = ?`, groupID, userID).Scan(&n)
	return n > 0, err
}

func (s *Store) Bailout(ctx context.Context, groupID, userID int64) (float64, error) {
	var coins float64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var last int64
		if err := tx.QueryRowContext(ctx, `SELECT coins, last_bonus FROM members WHERE group_id = ? AND user_id = ?`,
			groupID, userID).Scan(&coins, &last); err != nil {
			return membership(err)
		}
		if s.now()-last < BailoutEvery.Milliseconds() {
			return ErrTooSoon
		}
		coins += BailoutCoins
		_, err := tx.ExecContext(ctx, `UPDATE members SET coins = ?, last_bonus = ? WHERE group_id = ? AND user_id = ?`,
			coins, s.now(), groupID, userID)
		return err
	})
	return coins, err
}

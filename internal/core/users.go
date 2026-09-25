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
	CreatedBy int64     `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type Member struct {
	User
	Coins      float64   `json:"coins"`
	NetWorth   float64   `json:"net_worth"`
	LastBonus  time.Time `json:"last_bonus"`
	JoinedAt   time.Time `json:"joined_at"`
	OpenBets   int       `json:"open_bets"`
	Title      string    `json:"title,omitempty"`
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
	err := s.db.QueryRowContext(ctx, `SELECT id, name FROM users WHERE token_hash = ?`, hashToken(token)).Scan(&u.ID, &u.Name)
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
	g := Group{Name: name, CreatedBy: userID, CreatedAt: time.UnixMilli(s.now())}
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
	err := s.db.QueryRowContext(ctx, `SELECT id, code, name, created_by, created_at FROM groups WHERE code = ?`,
		strings.ToLower(code)).Scan(&g.ID, &g.Code, &g.Name, &g.CreatedBy, &created)
	g.CreatedAt = time.UnixMilli(created)
	return g, notFound(err)
}

func (s *Store) Join(ctx context.Context, groupID, userID int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO members (group_id, user_id, coins, joined_at) VALUES (?, ?, ?, ?)
		ON CONFLICT DO NOTHING`, groupID, userID, StartingCoins, s.now())
	return err
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

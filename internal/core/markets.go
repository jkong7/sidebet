package core

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/jkong7/sidebet/internal/lmsr"
)

type Market struct {
	ID         int64      `json:"id"`
	GroupID    int64      `json:"group_id"`
	CreatorID  int64      `json:"creator_id"`
	Creator    string     `json:"creator"`
	SubjectID  *int64     `json:"subject_id,omitempty"`
	Subject    string     `json:"subject,omitempty"`
	Question   string     `json:"question"`
	B          float64    `json:"-"`
	QYes       float64    `json:"-"`
	QNo        float64    `json:"-"`
	Chance     float64    `json:"chance"`
	Status     string     `json:"status"`
	Outcome    string     `json:"outcome,omitempty"`
	ClosesAt   time.Time  `json:"closes_at"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Volume     float64    `json:"volume"`
	Traders    int        `json:"traders"`
	Mine       *Position  `json:"mine,omitempty"`
}

type Position struct {
	Yes   float64 `json:"yes"`
	No    float64 `json:"no"`
	Spent float64 `json:"spent"`
	Value float64 `json:"value"`
}

type Trade struct {
	ID         int64     `json:"id"`
	MarketID   int64     `json:"market_id"`
	Question   string    `json:"question,omitempty"`
	UserID     int64     `json:"user_id"`
	User       string    `json:"user"`
	Side       string    `json:"side"`
	Shares     float64   `json:"shares"`
	Coins      float64   `json:"coins"`
	PriceAfter float64   `json:"price_after"`
	Insider    bool      `json:"insider"`
	At         time.Time `json:"at"`
}

type Point struct {
	At     time.Time `json:"at"`
	Chance float64   `json:"chance"`
}

func (m Market) book() lmsr.Book { return lmsr.Book{B: m.B, Yes: m.QYes, No: m.QNo} }

func (m Market) open(now time.Time) bool { return m.Status == "open" && now.Before(m.ClosesAt) }

func round2(v float64) float64 { return math.Round(v*100) / 100 }

type NewMarket struct {
	Question  string
	SubjectID *int64
	ClosesAt  time.Time
}

func (s *Store) CreateMarket(ctx context.Context, groupID, userID int64, in NewMarket) (Market, error) {
	q, err := cleanText(in.Question, 140)
	if err != nil {
		return Market{}, err
	}
	if !Allowed(q) {
		return Market{}, ErrBlocked
	}
	now := s.Now()
	if !in.ClosesAt.After(now) || in.ClosesAt.After(now.AddDate(1, 0, 0)) {
		return Market{}, ErrInvalidInput
	}
	var id int64
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if ok, err := memberTx(ctx, tx, groupID, userID); err != nil || !ok {
			return firstErr(err, ErrNotMember)
		}
		var kind string
		if err := tx.QueryRowContext(ctx, `SELECT kind FROM groups WHERE id = ?`, groupID).Scan(&kind); err != nil {
			return err
		}
		if kind == "campus" && in.SubjectID != nil {
			return ErrNoPeople
		}
		if in.SubjectID != nil {
			if ok, err := memberTx(ctx, tx, groupID, *in.SubjectID); err != nil || !ok {
				return firstErr(err, ErrInvalidInput)
			}
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO markets (group_id, creator_id, subject_id, question, b, closes_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, groupID, userID, in.SubjectID, q, DefaultB, in.ClosesAt.UnixMilli(), now.UnixMilli())
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		return Market{}, err
	}
	return s.Market(ctx, id, userID)
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

func memberTx(ctx context.Context, tx *sql.Tx, groupID, userID int64) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM members WHERE group_id = ? AND user_id = ?`, groupID, userID).Scan(&n)
	return n > 0, err
}

const marketCols = `m.id, m.group_id, m.creator_id, c.name, m.subject_id, COALESCE(su.name, ''), m.question, m.b, m.q_yes, m.q_no,
	m.status, COALESCE(m.outcome, ''), m.closes_at, m.created_at, m.resolved_at,
	(SELECT COALESCE(sum(abs(coins)), 0) FROM trades t WHERE t.market_id = m.id),
	(SELECT count(DISTINCT user_id) FROM trades t WHERE t.market_id = m.id)`

const marketFrom = ` FROM markets m JOIN users c ON c.id = m.creator_id LEFT JOIN users su ON su.id = m.subject_id`

type scanner interface{ Scan(...any) error }

func scanMarket(r scanner) (Market, error) {
	var m Market
	var closes, created int64
	var resolved sql.NullInt64
	var subject sql.NullInt64
	err := r.Scan(&m.ID, &m.GroupID, &m.CreatorID, &m.Creator, &subject, &m.Subject, &m.Question, &m.B, &m.QYes, &m.QNo,
		&m.Status, &m.Outcome, &closes, &created, &resolved, &m.Volume, &m.Traders)
	if err != nil {
		return m, err
	}
	if subject.Valid {
		m.SubjectID = &subject.Int64
	}
	m.ClosesAt, m.CreatedAt = time.UnixMilli(closes), time.UnixMilli(created)
	if resolved.Valid {
		t := time.UnixMilli(resolved.Int64)
		m.ResolvedAt = &t
	}
	m.Chance = m.book().PriceYes()
	switch m.Outcome {
	case "yes":
		m.Chance = 1
	case "no":
		m.Chance = 0
	}
	m.Volume = round2(m.Volume)
	return m, nil
}

func (s *Store) attachPosition(ctx context.Context, m *Market, userID int64) error {
	var p Position
	err := s.db.QueryRowContext(ctx, `SELECT yes, no, spent FROM positions WHERE market_id = ? AND user_id = ?`, m.ID, userID).
		Scan(&p.Yes, &p.No, &p.Spent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	p.Value = round2(p.Yes*m.Chance + p.No*(1-m.Chance))
	if m.Status == "void" {
		p.Value = 0
	}
	m.Mine = &p
	return nil
}

func (s *Store) Market(ctx context.Context, id, viewerID int64) (Market, error) {
	m, err := scanMarket(s.db.QueryRowContext(ctx, `SELECT `+marketCols+marketFrom+` WHERE m.id = ?`, id))
	if err != nil {
		return m, notFound(err)
	}
	if viewerID != 0 {
		err = s.attachPosition(ctx, &m, viewerID)
	}
	return m, err
}

func (s *Store) Markets(ctx context.Context, groupID, viewerID int64) ([]Market, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+marketCols+marketFrom+` WHERE m.group_id = ?
		ORDER BY (m.status = 'open') DESC, m.created_at DESC, m.id DESC LIMIT 200`, groupID)
	if err != nil {
		return nil, err
	}
	var out []Market
	for rows.Next() {
		m, err := scanMarket(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.attachPosition(ctx, &out[i], viewerID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) loadForUpdate(ctx context.Context, tx *sql.Tx, marketID int64) (Market, error) {
	m, err := scanMarket(tx.QueryRowContext(ctx, `SELECT `+marketCols+marketFrom+` WHERE m.id = ?`, marketID))
	return m, notFound(err)
}

func (s *Store) recordTrade(ctx context.Context, tx *sql.Tx, m Market, userID int64, side string, shares, coins float64,
	next lmsr.Book) (Trade, error) {
	now := s.now()
	if _, err := tx.ExecContext(ctx, `UPDATE markets SET q_yes = ?, q_no = ? WHERE id = ?`, next.Yes, next.No, m.ID); err != nil {
		return Trade{}, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO trades (market_id, user_id, side, shares, coins, price_after, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, m.ID, userID, side, shares, coins, next.PriceYes(), now)
	if err != nil {
		return Trade{}, err
	}
	id, _ := res.LastInsertId()
	insider := m.SubjectID != nil && *m.SubjectID == userID
	return Trade{ID: id, MarketID: m.ID, Question: m.Question, UserID: userID, Side: side, Shares: shares, Coins: coins,
		PriceAfter: next.PriceYes(), Insider: insider, At: time.UnixMilli(now)}, nil
}

func (s *Store) Buy(ctx context.Context, marketID, userID int64, yes bool, spend float64) (Trade, error) {
	spend = round2(spend)
	if spend < 1 {
		return Trade{}, ErrInvalidInput
	}
	var t Trade
	err := s.tx(ctx, func(tx *sql.Tx) error {
		m, err := s.loadForUpdate(ctx, tx, marketID)
		if err != nil {
			return err
		}
		if !m.open(s.Now()) {
			return ErrClosed
		}
		var coins float64
		if err := tx.QueryRowContext(ctx, `SELECT coins FROM members WHERE group_id = ? AND user_id = ?`, m.GroupID, userID).
			Scan(&coins); err != nil {
			return membership(err)
		}
		if coins+1e-9 < spend {
			return ErrBroke
		}
		next, shares, err := m.book().Buy(yes, spend)
		if err != nil {
			return ErrInvalidInput
		}
		if _, err := tx.ExecContext(ctx, `UPDATE members SET coins = ? WHERE group_id = ? AND user_id = ?`,
			round2(coins-spend), m.GroupID, userID); err != nil {
			return err
		}
		col := "no"
		if yes {
			col = "yes"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO positions (market_id, user_id, `+col+`, spent) VALUES (?, ?, ?, ?)
			ON CONFLICT(market_id, user_id) DO UPDATE SET `+col+` = `+col+` + excluded.`+col+`, spent = spent + excluded.spent`,
			m.ID, userID, shares, spend); err != nil {
			return err
		}
		t, err = s.recordTrade(ctx, tx, m, userID, "buy_"+col, shares, spend, next)
		return err
	})
	return t, err
}

func (s *Store) Sell(ctx context.Context, marketID, userID int64, yes bool, shares float64) (Trade, error) {
	if shares <= 0 || math.IsNaN(shares) {
		return Trade{}, ErrInvalidInput
	}
	var t Trade
	err := s.tx(ctx, func(tx *sql.Tx) error {
		m, err := s.loadForUpdate(ctx, tx, marketID)
		if err != nil {
			return err
		}
		if !m.open(s.Now()) {
			return ErrClosed
		}
		var p Position
		if err := tx.QueryRowContext(ctx, `SELECT yes, no, spent FROM positions WHERE market_id = ? AND user_id = ?`,
			m.ID, userID).Scan(&p.Yes, &p.No, &p.Spent); err != nil {
			return notFoundAs(err, ErrInvalidInput)
		}
		held, col := p.No, "no"
		if yes {
			held, col = p.Yes, "yes"
		}
		if shares > held+1e-9 {
			return ErrInvalidInput
		}
		shares = math.Min(shares, held)
		next, proceeds, err := m.book().Sell(yes, shares)
		if err != nil {
			return ErrInvalidInput
		}
		proceeds = math.Floor(proceeds*100) / 100
		if _, err := tx.ExecContext(ctx, `UPDATE members SET coins = round(coins + ?, 2) WHERE group_id = ? AND user_id = ?`,
			proceeds, m.GroupID, userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE positions SET `+col+` = max(0, `+col+` - ?), spent = spent - ?
			WHERE market_id = ? AND user_id = ?`, shares, proceeds, m.ID, userID); err != nil {
			return err
		}
		t, err = s.recordTrade(ctx, tx, m, userID, "sell_"+col, shares, -proceeds, next)
		return err
	})
	return t, err
}

func notFoundAs(err, as error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return as
	}
	return err
}

func (s *Store) Resolve(ctx context.Context, marketID, userID int64, outcome string) (Market, error) {
	if outcome != "yes" && outcome != "no" && outcome != "void" {
		return Market{}, ErrInvalidInput
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		m, err := s.loadForUpdate(ctx, tx, marketID)
		if err != nil {
			return err
		}
		var owner int64
		var kind string
		if err := tx.QueryRowContext(ctx, `SELECT created_by, kind FROM groups WHERE id = ?`, m.GroupID).Scan(&owner, &kind); err != nil {
			return err
		}
		if kind == "campus" {
			var role string
			if err := tx.QueryRowContext(ctx, `SELECT role FROM members WHERE group_id = ? AND user_id = ?`, m.GroupID, userID).
				Scan(&role); err != nil || role != "admin" {
				return ErrForbidden
			}
		} else if m.CreatorID != userID && (owner != userID || outcome != "void") {
			return ErrForbidden
		}
		if m.Status != "open" {
			return ErrClosed
		}
		payout := `CASE WHEN ? = 'yes' THEN p.yes WHEN ? = 'no' THEN p.no ELSE max(p.spent, 0) END`
		if _, err := tx.ExecContext(ctx, `UPDATE members SET coins = round(coins + COALESCE((
				SELECT `+payout+` FROM positions p WHERE p.market_id = ? AND p.user_id = members.user_id), 0), 2)
			WHERE group_id = ?`, outcome, outcome, m.ID, m.GroupID); err != nil {
			return err
		}
		status := "resolved"
		if outcome == "void" {
			status = "void"
		}
		_, err = tx.ExecContext(ctx, `UPDATE markets SET status = ?, outcome = ?, resolved_at = ? WHERE id = ?`,
			status, outcome, s.now(), m.ID)
		return err
	})
	if err != nil {
		return Market{}, err
	}
	return s.Market(ctx, marketID, userID)
}

func (s *Store) Trades(ctx context.Context, groupID, marketID int64, limit int) ([]Trade, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.market_id, m.question, t.user_id, u.name, t.side, t.shares, t.coins,
			t.price_after, t.created_at, COALESCE(m.subject_id = t.user_id, 0)
		FROM trades t JOIN markets m ON m.id = t.market_id JOIN users u ON u.id = t.user_id
		WHERE m.group_id = ? AND (? = 0 OR t.market_id = ?) ORDER BY t.id DESC LIMIT ?`, groupID, marketID, marketID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Trade{}
	for rows.Next() {
		var t Trade
		var at int64
		if err := rows.Scan(&t.ID, &t.MarketID, &t.Question, &t.UserID, &t.User, &t.Side, &t.Shares, &t.Coins, &t.PriceAfter,
			&at, &t.Insider); err != nil {
			return nil, err
		}
		t.At = time.UnixMilli(at)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) History(ctx context.Context, m Market) ([]Point, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT created_at, price_after FROM trades WHERE market_id = ? ORDER BY id`, m.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Point{{At: m.CreatedAt, Chance: 0.5}}
	for rows.Next() {
		var at int64
		var p Point
		if err := rows.Scan(&at, &p.Chance); err != nil {
			return nil, err
		}
		p.At = time.UnixMilli(at)
		out = append(out, p)
	}
	if m.Status != "open" && m.ResolvedAt != nil {
		out = append(out, Point{At: *m.ResolvedAt, Chance: m.Chance})
	}
	return out, rows.Err()
}

func (s *Store) Leaderboard(ctx context.Context, groupID int64) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.name, mb.coins, mb.last_bonus, mb.joined_at,
			COALESCE((SELECT sum(p.yes * (1.0 / (1.0 + exp((m.q_no - m.q_yes) / m.b))) +
			                    p.no  * (1.0 - 1.0 / (1.0 + exp((m.q_no - m.q_yes) / m.b))))
			          FROM positions p JOIN markets m ON m.id = p.market_id
			          WHERE p.user_id = u.id AND m.group_id = mb.group_id AND m.status = 'open'), 0),
			(SELECT count(*) FROM positions p JOIN markets m ON m.id = p.market_id
			  WHERE p.user_id = u.id AND m.group_id = mb.group_id AND m.status = 'open' AND (p.yes > 0.001 OR p.no > 0.001))
		FROM members mb JOIN users u ON u.id = mb.user_id WHERE mb.group_id = ?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		var last, joined int64
		var open float64
		if err := rows.Scan(&m.ID, &m.Name, &m.Coins, &last, &joined, &open, &m.OpenBets); err != nil {
			return nil, err
		}
		m.LastBonus, m.JoinedAt = time.UnixMilli(last), time.UnixMilli(joined)
		m.NetWorth = round2(m.Coins + open)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].NetWorth > out[j].NetWorth })
	if len(out) >= 2 {
		out[0].Title = "Top Degen"
		out[len(out)-1].Title = "Down Bad"
	}
	return out, nil
}

type Board struct {
	Top     []Member `json:"top"`
	Me      *Member  `json:"me,omitempty"`
	Members int      `json:"members"`
}

func (s *Store) Board(ctx context.Context, groupID, viewerID int64, limit int) (Board, error) {
	all, err := s.Leaderboard(ctx, groupID)
	if err != nil {
		return Board{}, err
	}
	b := Board{Members: len(all), Top: []Member{}}
	for i := range all {
		all[i].Rank = i + 1
		if i < limit {
			b.Top = append(b.Top, all[i])
		}
		if all[i].ID == viewerID {
			me := all[i]
			b.Me = &me
		}
	}
	return b, nil
}

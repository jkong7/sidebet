package core

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"
)

type fixture struct {
	s     *Store
	ctx   context.Context
	now   time.Time
	g     Group
	alice User
	bob   User
	cara  User
}

func setup(t *testing.T) *fixture {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	f := &fixture{s: s, ctx: context.Background(), now: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	s.Now = func() time.Time { return f.now }
	f.alice, _, _ = s.CreateUser(f.ctx, "Alice")
	f.bob, _, _ = s.CreateUser(f.ctx, "Bob")
	f.cara, _, _ = s.CreateUser(f.ctx, "Cara")
	f.g, err = s.CreateGroup(f.ctx, f.alice.ID, "  the   boys ")
	if err != nil {
		t.Fatal(err)
	}
	s.Join(f.ctx, f.g.ID, f.bob.ID)
	s.Join(f.ctx, f.g.ID, f.cara.ID)
	return f
}

func (f *fixture) market(t *testing.T, subject *int64) Market {
	m, err := f.s.CreateMarket(f.ctx, f.g.ID, f.alice.ID, NewMarket{Question: "Does Bob text her back?", SubjectID: subject,
		ClosesAt: f.now.Add(48 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (f *fixture) coins(t *testing.T, u User) float64 {
	board, err := f.s.Leaderboard(f.ctx, f.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range board {
		if m.ID == u.ID {
			return m.Coins
		}
	}
	t.Fatalf("user %d not on board", u.ID)
	return 0
}

func TestUsersAndGroups(t *testing.T) {
	f := setup(t)
	if f.g.Name != "the boys" || len(f.g.Code) != 6 {
		t.Fatalf("group = %+v", f.g)
	}
	got, err := f.s.GroupByCode(f.ctx, f.g.Code)
	if err != nil || got.ID != f.g.ID {
		t.Fatalf("lookup: %+v %v", got, err)
	}
	u, token, _ := f.s.CreateUser(f.ctx, "Dan")
	if back, err := f.s.UserByToken(f.ctx, token); err != nil || back.ID != u.ID {
		t.Fatalf("token lookup: %+v %v", back, err)
	}
	if _, err := f.s.UserByToken(f.ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad token: %v", err)
	}
	if _, _, err := f.s.CreateUser(f.ctx, "   "); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("blank name accepted")
	}
	if ok, _ := f.s.IsMember(f.ctx, f.g.ID, u.ID); ok {
		t.Fatal("non-member reported as member")
	}
	f.s.Join(f.ctx, f.g.ID, f.bob.ID)
	if c := f.coins(t, f.bob); c != StartingCoins {
		t.Fatalf("rejoin reset coins to %v", c)
	}
}

func TestBuySellAndLeaderboard(t *testing.T) {
	f := setup(t)
	m := f.market(t, &f.bob.ID)
	tr, err := f.s.Buy(f.ctx, m.ID, f.cara.ID, true, 200)
	if err != nil {
		t.Fatal(err)
	}
	if tr.PriceAfter <= 0.5 || tr.Shares < 200 || tr.Insider {
		t.Fatalf("trade = %+v", tr)
	}
	if c := f.coins(t, f.cara); c != 800 {
		t.Fatalf("cara coins = %v", c)
	}
	ins, _ := f.s.Buy(f.ctx, m.ID, f.bob.ID, false, 50)
	if !ins.Insider {
		t.Fatal("subject trading on own market should be flagged insider")
	}
	got, _ := f.s.Market(f.ctx, m.ID, f.cara.ID)
	if got.Mine == nil || math.Abs(got.Mine.Yes-tr.Shares) > 1e-9 || got.Traders != 2 || got.Volume != 250 {
		t.Fatalf("market view = %+v mine=%+v", got, got.Mine)
	}
	board, _ := f.s.Leaderboard(f.ctx, f.g.ID)
	if board[0].Title != "Top Degen" || board[len(board)-1].Title != "Down Bad" {
		t.Fatalf("titles: %+v", board)
	}
	for _, b := range board {
		if b.ID == f.cara.ID && (b.NetWorth <= 800 || b.OpenBets != 1) {
			t.Fatalf("cara net worth should include position: %+v", b)
		}
	}
	sold, err := f.s.Sell(f.ctx, m.ID, f.cara.ID, true, tr.Shares)
	if err != nil || sold.Coins >= 0 {
		t.Fatalf("sell = %+v %v", sold, err)
	}
	if c := f.coins(t, f.cara); c <= 900 || c >= 1000 {
		t.Fatalf("cara after selling back = %v", c)
	}
	if _, err := f.s.Sell(f.ctx, m.ID, f.cara.ID, true, 1); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("overselling: %v", err)
	}
}

func TestBuyRejections(t *testing.T) {
	f := setup(t)
	m := f.market(t, nil)
	if _, err := f.s.Buy(f.ctx, m.ID, f.bob.ID, true, 5000); !errors.Is(err, ErrBroke) {
		t.Fatalf("overspend: %v", err)
	}
	if _, err := f.s.Buy(f.ctx, m.ID, f.bob.ID, true, 0.5); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("dust: %v", err)
	}
	outsider, _, _ := f.s.CreateUser(f.ctx, "Eve")
	if _, err := f.s.Buy(f.ctx, m.ID, outsider.ID, true, 10); !errors.Is(err, ErrNotMember) {
		t.Fatalf("outsider: %v", err)
	}
	f.now = f.now.Add(49 * time.Hour)
	if _, err := f.s.Buy(f.ctx, m.ID, f.bob.ID, true, 10); !errors.Is(err, ErrClosed) {
		t.Fatalf("after close: %v", err)
	}
}

func TestResolvePaysWinners(t *testing.T) {
	f := setup(t)
	m := f.market(t, nil)
	yes, _ := f.s.Buy(f.ctx, m.ID, f.bob.ID, true, 100)
	f.s.Buy(f.ctx, m.ID, f.cara.ID, false, 100)
	if _, err := f.s.Resolve(f.ctx, m.ID, f.bob.ID, "yes"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-creator resolve: %v", err)
	}
	r, err := f.s.Resolve(f.ctx, m.ID, f.alice.ID, "yes")
	if err != nil || r.Status != "resolved" || r.Chance != 1 {
		t.Fatalf("resolve = %+v %v", r, err)
	}
	if c := f.coins(t, f.bob); math.Abs(c-(900+yes.Shares)) > 0.01 {
		t.Fatalf("bob = %v want %v", c, 900+yes.Shares)
	}
	if c := f.coins(t, f.cara); c != 900 {
		t.Fatalf("cara = %v", c)
	}
	if _, err := f.s.Resolve(f.ctx, m.ID, f.alice.ID, "no"); !errors.Is(err, ErrClosed) {
		t.Fatalf("double resolve: %v", err)
	}
	h, _ := f.s.History(f.ctx, r)
	if len(h) != 4 || h[0].Chance != 0.5 || h[3].Chance != 1 {
		t.Fatalf("history = %+v", h)
	}
}

func TestVoidRefunds(t *testing.T) {
	f := setup(t)
	m := f.market(t, nil)
	f.s.Buy(f.ctx, m.ID, f.bob.ID, true, 300)
	f.s.Buy(f.ctx, m.ID, f.cara.ID, false, 40)
	if _, err := f.s.Resolve(f.ctx, m.ID, f.alice.ID, "void"); err != nil {
		t.Fatal(err)
	}
	if b, c := f.coins(t, f.bob), f.coins(t, f.cara); b != 1000 || c != 1000 {
		t.Fatalf("void refunds: bob=%v cara=%v", b, c)
	}
}

func TestBailout(t *testing.T) {
	f := setup(t)
	if c, err := f.s.Bailout(f.ctx, f.g.ID, f.bob.ID); err != nil || c != 1100 {
		t.Fatalf("first bailout = %v %v", c, err)
	}
	if _, err := f.s.Bailout(f.ctx, f.g.ID, f.bob.ID); !errors.Is(err, ErrTooSoon) {
		t.Fatalf("second bailout: %v", err)
	}
	f.now = f.now.Add(BailoutEvery)
	if c, _ := f.s.Bailout(f.ctx, f.g.ID, f.bob.ID); c != 1200 {
		t.Fatalf("next day bailout = %v", c)
	}
}

func TestTradesFeed(t *testing.T) {
	f := setup(t)
	m1 := f.market(t, nil)
	m2 := f.market(t, &f.cara.ID)
	f.s.Buy(f.ctx, m1.ID, f.bob.ID, true, 10)
	f.s.Buy(f.ctx, m2.ID, f.cara.ID, true, 10)
	all, _ := f.s.Trades(f.ctx, f.g.ID, 0, 10)
	one, _ := f.s.Trades(f.ctx, f.g.ID, m1.ID, 10)
	if len(all) != 2 || len(one) != 1 || !all[0].Insider || all[0].Question == "" {
		t.Fatalf("feed: all=%+v one=%+v", all, one)
	}
	list, _ := f.s.Markets(f.ctx, f.g.ID, f.bob.ID)
	if len(list) != 2 || list[0].ID != m2.ID || list[1].Mine == nil {
		t.Fatalf("markets list: %+v", list)
	}
}

func TestGroupOwnerCanOnlyVoid(t *testing.T) {
	f := setup(t)
	m, err := f.s.CreateMarket(f.ctx, f.g.ID, f.bob.ID, NewMarket{Question: "Something mean about Cara", SubjectID: &f.cara.ID,
		ClosesAt: f.now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	f.s.Buy(f.ctx, m.ID, f.bob.ID, true, 100)
	if _, err := f.s.Resolve(f.ctx, m.ID, f.alice.ID, "yes"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("owner settling someone else's market: %v", err)
	}
	if _, err := f.s.Resolve(f.ctx, m.ID, f.cara.ID, "void"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-owner void: %v", err)
	}
	r, err := f.s.Resolve(f.ctx, m.ID, f.alice.ID, "void")
	if err != nil || r.Status != "void" || f.coins(t, f.bob) != 1000 {
		t.Fatalf("owner void: %+v %v bob=%v", r, err, f.coins(t, f.bob))
	}
}

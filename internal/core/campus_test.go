package core

import (
	"errors"
	"testing"
	"time"
)

func campus(t *testing.T, f *fixture) Group {
	g, err := f.s.EnsureCampus(f.ctx, Campus{Code: "NU", Name: "Northwestern", Domains: []string{"u.northwestern.edu", "northwestern.edu"},
		Admins: []string{"Mod@u.northwestern.edu"}})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func verify(t *testing.T, f *fixture, email string, current int64, name string) Verified {
	code, err := f.s.StartVerification(f.ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	v, err := f.s.FinishVerification(f.ctx, email, code, current, name)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestEnsureCampusIdempotent(t *testing.T) {
	f := setup(t)
	g := campus(t, f)
	again := campus(t, f)
	if g.ID != again.ID || !g.Campus() || g.Code != "nu" || len(g.Domains) != 2 {
		t.Fatalf("campus = %+v / %+v", g, again)
	}
}

func TestDomains(t *testing.T) {
	d := []string{"u.northwestern.edu", "northwestern.edu"}
	for email, want := range map[string]bool{
		"jk@u.northwestern.edu": true, "prof@northwestern.edu": true, "x@evil-northwestern.edu": false,
		"x@northwestern.edu.evil.com": false, "x@gmail.com": false, "nope": false,
	} {
		if got := DomainAllowed(email, d); got != want {
			t.Errorf("DomainAllowed(%s) = %v", email, got)
		}
	}
}

func TestVerificationFlow(t *testing.T) {
	f := setup(t)
	code, err := f.s.StartVerification(f.ctx, " JK@U.Northwestern.edu ")
	if err != nil || len(code) != 6 {
		t.Fatalf("start = %q %v", code, err)
	}
	if _, err := f.s.StartVerification(f.ctx, "jk@u.northwestern.edu"); !errors.Is(err, ErrSlowDown) {
		t.Fatalf("resend too soon: %v", err)
	}
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	if _, err := f.s.FinishVerification(f.ctx, "jk@u.northwestern.edu", wrong, 0, "JK"); !errors.Is(err, ErrBadCode) {
		t.Fatalf("wrong code: %v", err)
	}
	v, err := f.s.FinishVerification(f.ctx, "jk@u.northwestern.edu", code, 0, "JK")
	if err != nil || v.Token == "" || v.Existing {
		t.Fatalf("finish = %+v %v", v, err)
	}
	if _, err := f.s.FinishVerification(f.ctx, "jk@u.northwestern.edu", code, 0, "JK"); !errors.Is(err, ErrBadCode) {
		t.Fatal("code must be single use")
	}
	f.now = f.now.Add(CodeResend)
	again := verify(t, f, "jk@u.northwestern.edu", 0, "")
	if !again.Existing || again.User.ID != v.User.ID || again.Token == "" || again.Token == v.Token {
		t.Fatalf("login on a new device = %+v", again)
	}
	if u, err := f.s.UserByToken(f.ctx, again.Token); err != nil || u.ID != v.User.ID {
		t.Fatalf("session token lookup = %+v %v", u, err)
	}
}

func TestCodeLockoutAndExpiry(t *testing.T) {
	f := setup(t)
	code, _ := f.s.StartVerification(f.ctx, "a@u.northwestern.edu")
	for range CodeAttempts {
		f.s.FinishVerification(f.ctx, "a@u.northwestern.edu", "nope", 0, "A")
	}
	if _, err := f.s.FinishVerification(f.ctx, "a@u.northwestern.edu", code, 0, "A"); !errors.Is(err, ErrBadCode) {
		t.Fatal("correct code must fail after too many attempts")
	}
	f.now = f.now.Add(time.Hour)
	code, _ = f.s.StartVerification(f.ctx, "a@u.northwestern.edu")
	f.now = f.now.Add(CodeTTL + time.Second)
	if _, err := f.s.FinishVerification(f.ctx, "a@u.northwestern.edu", code, 0, "A"); !errors.Is(err, ErrBadCode) {
		t.Fatal("expired code accepted")
	}
}

func TestAttachEmailToExistingAnonUser(t *testing.T) {
	f := setup(t)
	v := verify(t, f, "bob@northwestern.edu", f.bob.ID, "")
	if v.User.ID != f.bob.ID || v.Token != "" {
		t.Fatalf("attach = %+v", v)
	}
	if e, _ := f.s.Email(f.ctx, f.bob.ID); e != "bob@northwestern.edu" {
		t.Fatalf("email = %q", e)
	}
}

func TestCampusRules(t *testing.T) {
	f := setup(t)
	g := campus(t, f)
	if err := f.s.Join(f.ctx, g.ID, f.alice.ID); !errors.Is(err, ErrUnverified) {
		t.Fatalf("unverified join: %v", err)
	}
	stu := verify(t, f, "stu@u.northwestern.edu", 0, "Stu").User
	mod := verify(t, f, "mod@u.northwestern.edu", 0, "Mod").User
	outsider := verify(t, f, "x@gmail.com", 0, "X").User
	for _, u := range []User{stu, mod} {
		if err := f.s.Join(f.ctx, g.ID, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.Join(f.ctx, g.ID, outsider.ID); !errors.Is(err, ErrUnverified) {
		t.Fatalf("gmail join: %v", err)
	}
	if r, _ := f.s.Role(f.ctx, g.ID, mod.ID); r != "admin" {
		t.Fatalf("mod role = %q", r)
	}
	if _, err := f.s.CreateMarket(f.ctx, g.ID, stu.ID, NewMarket{Question: "Is Mod dating someone?", SubjectID: &mod.ID,
		ClosesAt: f.now.Add(time.Hour)}); !errors.Is(err, ErrNoPeople) {
		t.Fatalf("person market: %v", err)
	}
	if _, err := f.s.CreateMarket(f.ctx, g.ID, stu.ID, NewMarket{Question: "text 312-555-1234 for answers",
		ClosesAt: f.now.Add(time.Hour)}); !errors.Is(err, ErrBlocked) {
		t.Fatalf("blocked text: %v", err)
	}
	m, err := f.s.CreateMarket(f.ctx, g.ID, stu.ID, NewMarket{Question: "Wildcats cover the spread Saturday",
		ClosesAt: f.now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	f.s.Buy(f.ctx, m.ID, stu.ID, true, 100)
	if _, err := f.s.Resolve(f.ctx, m.ID, stu.ID, "yes"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("creator resolving a campus market: %v", err)
	}
	if r, err := f.s.Resolve(f.ctx, m.ID, mod.ID, "yes"); err != nil || r.Status != "resolved" {
		t.Fatalf("mod resolve = %+v %v", r, err)
	}
	if n, _ := f.s.Report(f.ctx, m.ID, stu.ID, "spam"); n != 1 {
		t.Fatalf("report count = %d", n)
	}
	b, err := f.s.Board(f.ctx, g.ID, stu.ID, 1)
	if err != nil || b.Members != 2 || len(b.Top) != 1 || b.Me == nil || b.Top[0].Rank != 1 {
		t.Fatalf("board = %+v %v", b, err)
	}
}

func TestReportedQueue(t *testing.T) {
	f := setup(t)
	g := campus(t, f)
	stu := verify(t, f, "stu@u.northwestern.edu", 0, "Stu").User
	f.s.Join(f.ctx, g.ID, stu.ID)
	m, _ := f.s.CreateMarket(f.ctx, g.ID, stu.ID, NewMarket{Question: "q", ClosesAt: f.now.Add(time.Hour)})
	f.s.Report(f.ctx, m.ID, stu.ID, "mean")
	f.s.Report(f.ctx, m.ID, f.bob.ID, "gross")
	list, err := f.s.Reported(f.ctx, g.ID)
	if err != nil || len(list) != 1 || list[0].Reports != 2 || len(list[0].Reasons) != 2 {
		t.Fatalf("reported = %+v %v", list, err)
	}
}

package lmsr

import (
	"math"
	"testing"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) < eps }

func TestFreshBookIsEven(t *testing.T) {
	k := Book{B: 100}
	if !near(k.PriceYes(), 0.5, 1e-12) || !near(k.Cost(), 100*math.Ln2, 1e-9) {
		t.Fatalf("fresh book: price=%v cost=%v", k.PriceYes(), k.Cost())
	}
}

func TestBuyCostsExactlySpend(t *testing.T) {
	k := Book{B: 150, Yes: 40, No: 10}
	for _, yes := range []bool{true, false} {
		for _, spend := range []float64{0.01, 1, 50, 1000, 100000} {
			next, shares, err := k.Buy(yes, spend)
			if err != nil {
				t.Fatal(err)
			}
			if !near(next.Cost()-k.Cost(), spend, 1e-6*math.Max(1, spend)) {
				t.Fatalf("yes=%v spend=%v cost delta=%v", yes, spend, next.Cost()-k.Cost())
			}
			if shares < spend {
				t.Fatalf("shares %v should be at least spend %v since price < 1", shares, spend)
			}
			if yes && next.PriceYes() <= k.PriceYes() || !yes && next.PriceYes() >= k.PriceYes() {
				t.Fatalf("buying yes=%v moved price the wrong way", yes)
			}
		}
	}
}

func TestBuyThenSellRoundTrips(t *testing.T) {
	k := Book{B: 100}
	mid, shares, _ := k.Buy(true, 250)
	back, refund, err := mid.Sell(true, shares)
	if err != nil || !near(refund, 250, 1e-6) || !near(back.Yes, 0, 1e-9) {
		t.Fatalf("round trip: refund=%v book=%+v err=%v", refund, back, err)
	}
}

func TestPricesStayBounded(t *testing.T) {
	k := Book{B: 100}
	k, _, _ = k.Buy(true, 1e6)
	if p := k.PriceYes(); p <= 0.99 || p > 1 {
		t.Fatalf("price after huge buy = %v", p)
	}
	if !near(k.Price(true)+k.Price(false), 1, 1e-12) {
		t.Fatal("prices must sum to 1")
	}
}

func TestMakerLossBounded(t *testing.T) {
	k := Book{B: 100}
	k, shares, _ := k.Buy(true, 5000)
	loss := shares - 5000
	if loss > MaxSubsidy(100)+1e-6 {
		t.Fatalf("maker loss %v exceeds b ln 2", loss)
	}
}

func TestInvalid(t *testing.T) {
	k := Book{B: 100}
	for _, v := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, _, err := k.Buy(true, v); err == nil {
			t.Errorf("buy %v should fail", v)
		}
		if _, _, err := k.Sell(true, v); err == nil {
			t.Errorf("sell %v should fail", v)
		}
	}
}

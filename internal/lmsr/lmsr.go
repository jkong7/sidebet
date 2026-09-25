package lmsr

import (
	"errors"
	"math"
)

var ErrInvalid = errors.New("lmsr: invalid amount")

type Book struct {
	B   float64
	Yes float64
	No  float64
}

func logSumExp(a, b float64) float64 {
	m := math.Max(a, b)
	return m + math.Log(math.Exp(a-m)+math.Exp(b-m))
}

func (k Book) Cost() float64 {
	return k.B * logSumExp(k.Yes/k.B, k.No/k.B)
}

func (k Book) PriceYes() float64 {
	return 1 / (1 + math.Exp((k.No-k.Yes)/k.B))
}

func (k Book) Price(yes bool) float64 {
	if yes {
		return k.PriceYes()
	}
	return 1 - k.PriceYes()
}

func (k Book) with(yes bool, delta float64) Book {
	if yes {
		k.Yes += delta
	} else {
		k.No += delta
	}
	return k
}

func (k Book) SharesFor(yes bool, spend float64) (float64, error) {
	if spend <= 0 || math.IsNaN(spend) || math.IsInf(spend, 0) {
		return 0, ErrInvalid
	}
	own, other := k.Yes, k.No
	if !yes {
		own, other = k.No, k.Yes
	}
	target := (k.Cost() + spend) / k.B
	o := other / k.B
	shares := k.B*(target+math.Log1p(-math.Exp(o-target))) - own
	if math.IsNaN(shares) || shares <= 0 {
		return 0, ErrInvalid
	}
	return shares, nil
}

func (k Book) Buy(yes bool, spend float64) (Book, float64, error) {
	shares, err := k.SharesFor(yes, spend)
	if err != nil {
		return k, 0, err
	}
	return k.with(yes, shares), shares, nil
}

func (k Book) Sell(yes bool, shares float64) (Book, float64, error) {
	if shares <= 0 || math.IsNaN(shares) || math.IsInf(shares, 0) {
		return k, 0, ErrInvalid
	}
	next := k.with(yes, -shares)
	return next, k.Cost() - next.Cost(), nil
}

func MaxSubsidy(b float64) float64 {
	return b * math.Ln2
}

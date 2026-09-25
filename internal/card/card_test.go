package card

import (
	"bytes"
	"image/png"
	"os"
	"testing"
)

func TestRender(t *testing.T) {
	cases := []Market{
		{Question: "Does Bob text her back before Friday?", Chance: 0.72, Group: "the boys", Status: "open", Volume: 1234,
			Traders: 5, History: []float64{0.5, 0.6, 0.55, 0.72}},
		{Question: "Will Priya get the Stripe offer after her final round next Tuesday, or will she get ghosted like the last three times and cope on Twitter about it for a week",
			Chance: 0.2, Status: "open"},
		{Question: "Marcus finishes the half marathon", Chance: 1, Status: "resolved", Outcome: "yes"},
		{Question: "Void me", Status: "void"},
	}
	for i, m := range cases {
		var buf bytes.Buffer
		if err := RenderMarket(&buf, m); err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(buf.Bytes()))
		if err != nil || img.Bounds().Dx() != W || img.Bounds().Dy() != H {
			t.Fatalf("case %d: bad png %v", i, err)
		}
		if dir := os.Getenv("CARD_OUT"); dir != "" {
			os.WriteFile(dir+"/market"+string(rune('0'+i))+".png", buf.Bytes(), 0o644)
		}
	}
	var buf bytes.Buffer
	if err := RenderGroup(&buf, Group{Name: "the boys", Members: 6, Hottest: "Does Bob text her back?", Chance: 0.72}); err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("CARD_OUT"); dir != "" {
		os.WriteFile(dir+"/group.png", buf.Bytes(), 0o644)
	}
}

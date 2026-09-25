package card

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

const (
	W = 1200
	H = 630
)

var (
	bg     = color.RGBA{0x0b, 0x0b, 0x10, 0xff}
	fg     = color.RGBA{0xf5, 0xf5, 0xf4, 0xff}
	muted  = color.RGBA{0x9c, 0x9a, 0xa6, 0xff}
	yesCol = color.RGBA{0x22, 0xc5, 0x5e, 0xff}
	noCol  = color.RGBA{0xef, 0x44, 0x44, 0xff}
	brand  = color.RGBA{0xa3, 0xe6, 0x35, 0xff}
)

var (
	fontsOnce sync.Once
	bold      *opentype.Font
	regular   *opentype.Font
)

func face(f *opentype.Font, size float64) font.Face {
	fc, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	return fc
}

func loadFonts() {
	bold, _ = opentype.Parse(gobold.TTF)
	regular, _ = opentype.Parse(goregular.TTF)
}

type Market struct {
	Question string
	Chance   float64
	Group    string
	Status   string
	Outcome  string
	Volume   float64
	Traders  int
	History  []float64
}

type Group struct {
	Name    string
	Members int
	Hottest string
	Chance  float64
}

func text(dst draw.Image, fc font.Face, c color.Color, x, y int, s string) int {
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: fc, Dot: fixed.P(x, y)}
	d.DrawString(s)
	return d.Dot.X.Round()
}

func wrap(fc font.Face, s string, width int, maxLines int) []string {
	var lines []string
	cur := ""
	for _, w := range strings.Fields(s) {
		try := strings.TrimSpace(cur + " " + w)
		if font.MeasureString(fc, try).Round() > width && cur != "" {
			lines = append(lines, cur)
			cur = w
		} else {
			cur = try
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		last := lines[maxLines-1]
		for font.MeasureString(fc, last+"…").Round() > width && len(last) > 0 {
			last = last[:len(last)-1]
		}
		lines[maxLines-1] = strings.TrimSpace(last) + "…"
	}
	return lines
}

func fitQuestion(q string, width, maxH int) (font.Face, []string, int) {
	for _, size := range []float64{64, 56, 48, 42, 36} {
		fc := face(bold, size)
		lh := int(size * 1.18)
		lines := wrap(fc, q, width, 10)
		if len(lines)*lh <= maxH {
			return fc, lines, lh
		}
	}
	fc := face(bold, 36)
	lh := 42
	return fc, wrap(fc, q, width, maxH/lh), lh
}

func base() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	return img
}

func brandMark(img *image.RGBA, right string) {
	x := text(img, face(bold, 30), brand, 64, 80, "sidebet")
	if right != "" {
		text(img, face(regular, 28), muted, x+18, 80, "· "+right)
	}
}

func sparkline(img *image.RGBA, pts []float64, x0, y0, w, h float64, c color.RGBA) {
	if len(pts) < 2 {
		return
	}
	r := vector.NewRasterizer(W, H)
	step := w / float64(len(pts)-1)
	thick := 5.0
	for i := 0; i < len(pts)-1; i++ {
		ax, ay := x0+float64(i)*step, y0+(1-pts[i])*h
		bx, by := x0+float64(i+1)*step, y0+(1-pts[i+1])*h
		dx, dy := bx-ax, by-ay
		l := math.Hypot(dx, dy)
		if l == 0 {
			continue
		}
		nx, ny := -dy/l*thick/2, dx/l*thick/2
		r.MoveTo(float32(ax+nx), float32(ay+ny))
		r.LineTo(float32(bx+nx), float32(by+ny))
		r.LineTo(float32(bx-nx), float32(by-ny))
		r.LineTo(float32(ax-nx), float32(ay-ny))
		r.ClosePath()
	}
	r.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{})
}

func RenderMarket(w io.Writer, m Market) error {
	fontsOnce.Do(loadFonts)
	img := base()
	brandMark(img, m.Group)

	fc, lines, lh := fitQuestion(m.Question, W-128, 250)
	y := 170
	for _, l := range lines {
		text(img, fc, fg, 64, y, l)
		y += lh
	}

	pct := int(math.Round(m.Chance * 100))
	label, col := "chance", yesCol
	switch {
	case m.Status == "resolved" && m.Outcome == "yes":
		label, col = "it happened", yesCol
	case m.Status == "resolved" && m.Outcome == "no":
		label, col = "didn't happen", noCol
	case m.Status == "void":
		label, col = "voided", muted
	case m.Chance < 0.5:
		col = noCol
	}
	big := face(bold, 150)
	var head string
	if m.Status == "resolved" {
		head = map[string]string{"yes": "YES", "no": "NO"}[m.Outcome]
	} else if m.Status == "void" {
		head = "VOID"
	} else {
		head = fmt.Sprintf("%d%%", pct)
	}
	x := text(img, big, col, 60, 560, head)
	text(img, face(regular, 40), muted, x+20, 545, label)

	sparkline(img, m.History, 760, 430, 380, 130, col)
	stats := "be the first to bet"
	if m.Traders > 0 {
		stats = fmt.Sprintf("%d betting · %s coins in", m.Traders, compact(m.Volume))
	}
	text(img, face(regular, 26), muted, 760, 600, stats)
	return png.Encode(w, img)
}

func RenderGroup(w io.Writer, g Group) error {
	fontsOnce.Do(loadFonts)
	img := base()
	brandMark(img, "")
	fc, lines, lh := fitQuestion("You're invited to "+g.Name, W-128, 150)
	y := 200
	for _, l := range lines {
		text(img, fc, fg, 64, y, l)
		y += lh
	}
	text(img, face(regular, 36), muted, 64, y+20, fmt.Sprintf("%d friends putting odds on each other", g.Members))
	if g.Hottest != "" {
		q := wrap(face(bold, 34), "“"+g.Hottest+"”", W-340, 2)
		yy := 500
		for _, l := range q {
			text(img, face(bold, 34), fg, 64, yy, l)
			yy += 42
		}
		col := yesCol
		if g.Chance < 0.5 {
			col = noCol
		}
		text(img, face(bold, 90), col, W-250, 560, fmt.Sprintf("%d%%", int(math.Round(g.Chance*100))))
	} else {
		text(img, face(bold, 44), brand, 64, 540, "Put odds on your friends.")
	}
	return png.Encode(w, img)
}

func compact(v float64) string {
	switch {
	case v >= 1e6:
		return fmt.Sprintf("%.1fM", v/1e6)
	case v >= 1e4:
		return fmt.Sprintf("%.0fk", v/1e3)
	case v >= 1e3:
		return fmt.Sprintf("%.1fk", v/1e3)
	}
	return fmt.Sprintf("%.0f", v)
}

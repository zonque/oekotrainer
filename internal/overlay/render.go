// Package overlay renders the status strip shown on top of a video player.
package overlay

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"kinobox/internal/config"
	"kinobox/internal/model"
	"kinobox/internal/web"
)

// State is what the renderer shows.
type State struct {
	// Data is the last response from the server, nil before the first one.
	Data *web.Readings
	// Err is set while the server cannot be reached.
	Err error
}

// idleW is the power below which a rider counts as not pedalling.
const idleW = 5

// Renderer draws the strip into an RGBA image. It caches font faces per size.
type Renderer struct {
	cfg     config.Overlay
	bold    *opentype.Font
	regular *opentype.Font
	faces   map[faceKey]font.Face
}

type faceKey struct {
	bold bool
	size float64
}

func NewRenderer(cfg config.Overlay) (*Renderer, error) {
	r := &Renderer{cfg: cfg, faces: map[faceKey]font.Face{}}
	var err error
	if cfg.Font != "" {
		b, err := os.ReadFile(cfg.Font)
		if err != nil {
			return nil, fmt.Errorf("overlay font: %w", err)
		}
		if r.regular, err = opentype.Parse(b); err != nil {
			return nil, fmt.Errorf("overlay font %s: %w", cfg.Font, err)
		}
		r.bold = r.regular
		return r, nil
	}
	if r.bold, err = opentype.Parse(gobold.TTF); err != nil {
		return nil, err
	}
	if r.regular, err = opentype.Parse(gomedium.TTF); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Renderer) face(bold bool, size float64) font.Face {
	size = math.Round(size*2) / 2
	k := faceKey{bold, size}
	if f, ok := r.faces[k]; ok {
		return f
	}
	src := r.regular
	if bold {
		src = r.bold
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err) // only fails for invalid sizes
	}
	r.faces[k] = f
	return f
}

// canvas wraps an image with the drawing primitives the strip needs.
type canvas struct {
	img *image.RGBA
	ras *vector.Rasterizer
}

func (c canvas) roundRect(x, y, w, h, rad float64, col color.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	rad = min(rad, w/2, h/2)
	// Rasterize only the shape's bounding box, in coordinates relative to it.
	box := image.Rect(int(math.Floor(x)), int(math.Floor(y)), int(math.Ceil(x+w)), int(math.Ceil(y+h))).
		Intersect(c.img.Bounds())
	if box.Empty() {
		return
	}
	c.ras.Reset(box.Dx(), box.Dy())
	X, Y := float32(x-float64(box.Min.X)), float32(y-float64(box.Min.Y))
	W, H, R := float32(w), float32(h), float32(rad)
	k := R * 0.4477 // control point distance for a quarter circle approximation
	c.ras.MoveTo(X+R, Y)
	c.ras.LineTo(X+W-R, Y)
	c.ras.CubeTo(X+W-k, Y, X+W, Y+k, X+W, Y+R)
	c.ras.LineTo(X+W, Y+H-R)
	c.ras.CubeTo(X+W, Y+H-k, X+W-k, Y+H, X+W-R, Y+H)
	c.ras.LineTo(X+R, Y+H)
	c.ras.CubeTo(X+k, Y+H, X, Y+H-k, X, Y+H-R)
	c.ras.LineTo(X, Y+R)
	c.ras.CubeTo(X, Y+k, X+k, Y, X+R, Y)
	c.ras.ClosePath()
	c.ras.Draw(c.img, box, image.NewUniform(col), image.Point{})
}

func (c canvas) circle(cx, cy, rad float64, col color.Color) {
	c.roundRect(cx-rad, cy-rad, 2*rad, 2*rad, rad, col)
}

func textWidth(f font.Face, s string) float64 {
	return float64(font.MeasureString(f, s)) / 64
}

// text draws s with its baseline at y; align is 0 (left), 0.5 (centre) or 1
// (right) relative to x. It returns the drawn width.
func (c canvas) text(f font.Face, s string, x, y, align float64, col color.Color) float64 {
	w := textWidth(f, s)
	d := font.Drawer{Dst: c.img, Src: image.NewUniform(col), Face: f,
		Dot: fixed.Point26_6{X: fixed.Int26_6((x - align*w) * 64), Y: fixed.Int26_6(y * 64)}}
	d.DrawString(s)
	return w
}

// capCentre returns the baseline that vertically centres capital letters of f
// in the band [top, bottom].
func capCentre(f font.Face, top, bottom float64) float64 {
	capH := float64(f.Metrics().CapHeight) / 64
	if capH == 0 {
		capH = float64(f.Metrics().Ascent) / 64 * 0.7
	}
	return (top+bottom)/2 + capH/2
}

func formatW(w float64) string { return fmt.Sprintf("%.0f W", w) }

func formatEnergy(j float64) string {
	wh := j / 3600
	switch {
	case wh < 10:
		return fmt.Sprintf("%.1f Wh", wh)
	case wh < 1000:
		return fmt.Sprintf("%.0f Wh", wh)
	}
	return fmt.Sprintf("%.2f kWh", wh/1000)
}

// Render draws the strip at w×h physical pixels. All sizes derive from h, so
// the strip scales with the configured height and the display scale factor.
func (r *Renderer) Render(st State, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	c := canvas{img: img, ras: vector.NewRasterizer(w, h)}
	col := r.cfg.Colors
	draw.Draw(img, img.Bounds(), image.NewUniform(col.Background), image.Point{}, draw.Src)

	H := float64(h)
	pad := math.Round(H * 0.12)
	gap := math.Round(H * 0.1)
	rad := H * 0.14
	big := r.face(true, H*0.40*r.cfg.FontScale)
	small := r.face(false, H*0.25*r.cfg.FontScale)
	top, bottom := pad, H-pad

	right := float64(w) - pad
	if r.cfg.ShowTotals {
		tw := math.Min(H*5.6, float64(w)*0.3)
		r.drawTotals(c, st, right-tw, top, tw, bottom-top, rad, big, small)
		right -= tw + gap
	}

	var riders []model.Rider
	var energy []float64
	maxW := 150.0
	online := false
	// Keep showing the riders while the server is unreachable or the box is
	// offline, dimmed and without values, so the strip doesn't jump around.
	if d := st.Data; d != nil {
		online = d.Online && st.Err == nil
		if d.Reading != nil {
			riders = d.Reading.Riders
		}
		energy = d.RiderEnergyJ
		if d.Config.RiderMaxW > 0 {
			maxW = d.Config.RiderMaxW
		}
	}
	if len(riders) == 0 {
		return img
	}

	leader, best := -1, float64(idleW)
	for i, rd := range riders {
		if rd.Power > best {
			leader, best = i, rd.Power
		}
	}

	n := float64(len(riders))
	segW := (right - pad - gap*(n-1)) / n
	barH := math.Max(2, math.Round(H*0.08))
	for i, rd := range riders {
		x := pad + float64(i)*(segW+gap)
		c.roundRect(x, top, segW, bottom-top, rad, col.Segment)

		inner := math.Round(H * 0.14)
		bx, bw := x+inner, segW-2*inner
		by := bottom - inner*0.8 - barH
		c.roundRect(bx, by, bw, barH, barH/2, col.BarTrack)
		if online {
			c.roundRect(bx, by, bw*math.Min(rd.Power/maxW, 1), barH, barH/2, col.Bar)
		}

		baseline := capCentre(big, top, by-inner*0.2)
		textCol, numCol := col.Text, col.TextMuted
		if !online || rd.Power < idleW {
			textCol = col.TextMuted
		}

		// position badge; filled for the strongest rider
		label := fmt.Sprint(rd.Position)
		br := float64(small.Metrics().Height) / 64 * 0.62
		cx := bx + br
		cy := baseline - float64(big.Metrics().CapHeight)/64/2
		if i == leader && online {
			c.circle(cx, cy, br, col.Leader)
			numCol = col.Background
			numCol.A = 0xff
		}
		c.text(small, label, cx, capCentre(small, cy-br, cy+br), 0.5, numCol)

		tx := cx + br + inner*0.6
		power := "–"
		if online {
			power = formatW(rd.Power)
		}
		pw := textWidth(big, power)
		if tx+pw > bx+bw {
			continue // segment too narrow even for the power
		}
		c.text(big, power, tx, baseline, 0, textCol)
		if r.cfg.ShowEnergy && online && i < len(energy) {
			e := formatEnergy(energy[i])
			if tx+pw+inner+textWidth(small, e) <= bx+bw {
				c.text(small, e, bx+bw, baseline, 1, col.TextMuted)
			}
		}
	}
	return img
}

func (r *Renderer) drawTotals(c canvas, st State, x, y, w, h, rad float64, big, small font.Face) {
	col := r.cfg.Colors
	c.roundRect(x, y, w, h, rad, col.Segment)
	inner := math.Round(h * 0.2)
	baseline := capCentre(big, y, y+h)
	smallBase := baseline

	// status dots sit next to small text, centred on its capitals
	dot := func(cx float64, cl color.Color) {
		c.circle(cx, smallBase-float64(small.Metrics().CapHeight)/64/2, h*0.11, cl)
	}

	d := st.Data
	switch {
	case st.Err != nil:
		dot(x+inner, col.Critical)
		c.text(small, "no connection to server", x+inner*2.2, smallBase, 0, col.TextMuted)
		return
	case d == nil || !d.Online || d.Reading == nil:
		dot(x+inner, col.Critical)
		c.text(small, "KinoBox offline", x+inner*2.2, smallBase, 0, col.TextMuted)
		return
	}

	rd := d.Reading
	cx := x + inner
	cx += c.text(big, formatW(rd.GeneratedPower), cx, baseline, 0, col.Text) + inner*0.6
	c.text(small, formatW(rd.LoadPower)+" used", cx, smallBase, 0, col.TextMuted)

	// battery on the right, coloured by the worst of voltage and protection
	state := col.Good
	cfg := d.Config
	switch {
	case rd.Status.Worst() >= model.Tripped || rd.Voltage <= cfg.VoltTripLow || rd.Voltage >= cfg.VoltTripHigh:
		state = col.Critical
	case rd.Status.Worst() == model.Warning || rd.Voltage <= cfg.VoltWarnLow || rd.Voltage >= cfg.VoltWarnHigh:
		state = col.Warning
	}
	volt := fmt.Sprintf("%.1f V", rd.Voltage)
	vx := x + w - inner
	vw := textWidth(small, volt)
	c.text(small, volt, vx, smallBase, 1, col.Text)
	dot(vx-vw-h*0.22, state)
}

package overlay

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"kinobox/internal/config"
	"kinobox/internal/model"
	"kinobox/internal/monitor"
	"kinobox/internal/web"
)

func sampleState() State {
	riders := []model.Rider{
		{Position: 1, Power: 75}, {Position: 2, Power: 150}, {Position: 3, Power: 0}, {Position: 4, Power: 210},
	}
	return State{Data: &web.Readings{
		Snapshot: monitor.Snapshot{
			Online: true,
			Reading: &model.Reading{
				Voltage: 12.8, GeneratedPower: 435, LoadPower: 280, Riders: riders,
			},
			RiderEnergyJ: []float64{3600, 7200, 0, 36000},
		},
		Config: web.UIConfig{RiderMaxW: 150, VoltTripLow: 11, VoltWarnLow: 11.5, VoltWarnHigh: 14, VoltTripHigh: 15},
	}}
}

func near(a, b color.Color) bool {
	r1, g1, b1, _ := a.RGBA()
	r2, g2, b2, _ := b.RGBA()
	d := func(x, y uint32) uint32 {
		if x > y {
			return x - y
		}
		return y - x
	}
	return d(r1, r2) < 0x1800 && d(g1, g2) < 0x1800 && d(b1, b2) < 0x1800
}

// over composites c on top of bg, as the renderer does.
func over(c, bg color.Color) color.Color {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, bg)
	cr, cg, cb, ca := c.RGBA()
	br, bgc, bb, _ := img.At(0, 0).RGBA()
	a := 0xffff - ca
	return color.RGBA64{uint16(cr + br*a/0xffff), uint16(cg + bgc*a/0xffff), uint16(cb + bb*a/0xffff), 0xffff}
}

func render(t *testing.T, cfg config.Overlay, st State) *image.RGBA {
	t.Helper()
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	img := r.Render(st, 1920, 56)
	if p := os.Getenv("OVERLAY_PNG"); p != "" {
		f, _ := os.Create(p)
		png.Encode(f, img)
		f.Close()
	}
	return img
}

func TestRender(t *testing.T) {
	cfg := config.Default().Overlay
	cfg.Colors.Background = config.Color{R: 0x10, G: 0x10, B: 0x10, A: 0xff} // opaque, for exact checks
	img := render(t, cfg, sampleState())

	if got := img.At(1, 1); !near(got, cfg.Colors.Background) {
		t.Errorf("corner = %v, want background", got)
	}
	// Rider 2 is at full scale: its bar is filled to the right end.
	// Layout: pad 7, gap 6, totals min(56*5.6, 576)=314 wide.
	right := 1920.0 - 7 - 314 - 6
	segW := (right - 7 - 3*6) / 4
	x2 := 7 + segW + 6
	barY := 56 - 7 - 8*0.8 - 4 + 2 // middle of the bar
	inner := 8.0
	end := image.Pt(int(x2+segW-inner-3), int(barY))
	if got := img.At(end.X, end.Y); !near(got, cfg.Colors.Bar) {
		t.Errorf("rider 2 bar end at %v = %v, want bar color", end, got)
	}
	// Rider 1 at half scale: the right end of its bar shows the track.
	end1 := image.Pt(int(7+segW-inner-3), int(barY))
	seg := over(cfg.Colors.Segment, cfg.Colors.Background)
	track := over(cfg.Colors.BarTrack, seg)
	if got := img.At(end1.X, end1.Y); !near(got, track) {
		t.Errorf("rider 1 bar end at %v = %v, want track %v", end1, got, track)
	}
	// Rider 4 is the leader: its badge is filled with the leader color.
	x4 := 7 + 3*(segW+6)
	found := false
	for y := 10; y < 40 && !found; y++ {
		for x := int(x4 + inner); x < int(x4+inner+20); x++ {
			if near(img.At(x, y), cfg.Colors.Leader) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("leader badge not found on rider 4")
	}
}

func TestRenderStates(t *testing.T) {
	cfg := config.Default().Overlay
	// must not panic and must draw something for every state
	for name, st := range map[string]State{
		"no data":     {},
		"unreachable": {Err: errors.New("connection refused")},
		"offline":     {Data: &web.Readings{Snapshot: monitor.Snapshot{Online: false}}},
		"online":      sampleState(),
	} {
		r, _ := NewRenderer(cfg)
		img := r.Render(st, 800, 40)
		if img.Bounds().Dx() != 800 {
			t.Errorf("%s: wrong size", name)
		}
	}
}

func BenchmarkRender(b *testing.B) {
	r, _ := NewRenderer(config.Default().Overlay)
	st := sampleState()
	for b.Loop() {
		r.Render(st, 3072, 90)
	}
}

func TestRenderKeepsRidersWhenUnreachable(t *testing.T) {
	cfg := config.Default().Overlay
	cfg.Colors.Background = config.Color{R: 0x10, G: 0x10, B: 0x10, A: 0xff}
	st := sampleState()
	st.Err = errors.New("connection refused")
	img := render(t, cfg, st)
	// the first segment is still drawn
	seg := over(cfg.Colors.Segment, cfg.Colors.Background)
	if got := img.At(12, 10); !near(got, seg) {
		t.Errorf("segment pixel = %v, want %v", got, seg)
	}
	// but its bar shows no power
	if got := img.At(20, 40); near(got, cfg.Colors.Bar) {
		t.Error("power bar drawn while unreachable")
	}
}

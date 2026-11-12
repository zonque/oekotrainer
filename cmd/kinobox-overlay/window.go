package main

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"kinobox/internal/config"
	"kinobox/internal/overlay"
)

// strip is the ebiten game: it blits the renderer's output and keeps the
// window glued to the configured screen edge.
type strip struct {
	cfg      config.Overlay
	client   *overlay.Client
	renderer *overlay.Renderer
	monitor  *ebiten.MonitorType

	img      *ebiten.Image
	version  uint64
	w, h     int
	placed   time.Time
	platform bool // platform-specific window setup done
}

// replaceEvery is how often the window size and position are reapplied.
const replaceEvery = 2 * time.Second

func selectMonitor(index int) *ebiten.MonitorType {
	ms := ebiten.AppendMonitors(nil)
	for i, m := range ms {
		w, h := m.Size()
		slog.Info("monitor", "index", i, "name", m.Name(), "size", [2]int{w, h}, "scale", m.DeviceScaleFactor())
	}
	if index < 0 || index >= len(ms) {
		slog.Warn("configured monitor does not exist, using the primary one", "monitor", index)
		return ebiten.Monitor()
	}
	return ms[index]
}

// place sizes the window to the full monitor width and moves it to the edge.
func (s *strip) place() {
	mw, mh := s.monitor.Size()
	ebiten.SetMonitor(s.monitor)
	ebiten.SetWindowSize(mw, s.cfg.Height)
	y := 0
	if s.cfg.Position == "bottom" {
		y = mh - s.cfg.Height
	}
	ebiten.SetWindowPosition(0, y)
	s.placed = time.Now()
}

func (s *strip) Update() error {
	if !s.platform {
		s.platform = platformSetup()
	}
	// Follow resolution changes and window managers that move us around.
	if time.Since(s.placed) > replaceEvery {
		s.place()
	}
	if !s.cfg.ClickThrough && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	return nil
}

func (s *strip) Draw(screen *ebiten.Image) {
	st, v := s.client.State()
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	if s.img == nil || v != s.version || w != s.w || h != s.h {
		rgba := s.renderer.Render(st, w, h)
		if s.img == nil || w != s.w || h != s.h {
			s.img = ebiten.NewImage(w, h)
		}
		s.img.WritePixels(rgba.Pix)
		s.version, s.w, s.h = v, w, h
	}
	screen.DrawImage(s.img, nil)
}

// Layout renders at the physical resolution so text stays sharp on HiDPI
// screens.
func (s *strip) Layout(w, h int) (int, int) {
	fw, fh := s.LayoutF(float64(w), float64(h))
	return int(fw), int(fh)
}

func (s *strip) LayoutF(w, h float64) (float64, float64) {
	scale := s.monitor.DeviceScaleFactor()
	return math.Ceil(w * scale), math.Ceil(h * scale)
}

func runWindow(cfg config.Overlay, client *overlay.Client, renderer *overlay.Renderer) error {
	s := &strip{cfg: cfg, client: client, renderer: renderer, monitor: selectMonitor(cfg.Monitor)}

	ebiten.SetWindowTitle("KinoBox overlay")
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetWindowMousePassthrough(cfg.ClickThrough)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	ebiten.SetRunnableOnUnfocused(true)
	// Only draw when something changed: new data, or time to re-place the
	// window. Rendering at the display refresh rate costs half a CPU core
	// for a picture that changes twice a second. FPSModeVsyncOffMinimum is
	// deprecated but still the only way to get on-demand frames.
	//lint:ignore SA1019 see above
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOffMinimum)
	client.OnChange(ebiten.ScheduleFrame)
	go client.Run(context.Background())
	go func() {
		for range time.Tick(replaceEvery) {
			ebiten.ScheduleFrame()
		}
	}()
	s.place()

	return ebiten.RunGameWithOptions(s, &ebiten.RunGameOptions{
		ScreenTransparent: true,
		InitUnfocused:     true, // don't take the focus away from the video player
		SkipTaskbar:       true,
		SingleThread:      singleThread,
		X11ClassName:      "kinobox-overlay",
		X11InstanceName:   "kinobox-overlay",
	})
}

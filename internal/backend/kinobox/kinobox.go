// Package kinobox is the backend for a KinoBox on a serial port.
//
// The box streams 28-byte frames (14 little-endian 16-bit words) at 115200 8N1
// without any header or checksum. See docs/PROTOCOL.md.
package kinobox

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"kinobox/internal/model"
	"kinobox/internal/serial"
)

type Config struct {
	Device  string
	Scaling Scaling
	Riders  int
	// StaleAfter: without a valid frame for this long Run returns, so the
	// port gets reopened.
	StaleAfter time.Duration
	// GapAfter: idle time on the line that marks a frame boundary.
	GapAfter time.Duration
}

type Backend struct {
	cfg  Config
	open func() (io.ReadCloser, error)
	now  func() time.Time

	frames    atomic.Uint64
	discarded atomic.Uint64
}

func New(cfg Config) *Backend {
	if cfg.StaleAfter == 0 {
		cfg.StaleAfter = 3 * time.Second
	}
	if cfg.GapAfter == 0 {
		cfg.GapAfter = 50 * time.Millisecond
	}
	return &Backend{
		cfg:  cfg,
		open: func() (io.ReadCloser, error) { return serial.Open(cfg.Device) },
		now:  time.Now,
	}
}

func (b *Backend) Name() string { return b.cfg.Device }

func (b *Backend) Counters() map[string]uint64 {
	return map[string]uint64{
		"frames":          b.frames.Load(),
		"discarded_bytes": b.discarded.Load(),
	}
}

type staleError struct{}

func (staleError) Error() string { return "no valid frame received" }

// Run opens the serial port and decodes frames until ctx is cancelled, the
// port fails, or no valid frame arrived for StaleAfter.
func (b *Backend) Run(ctx context.Context, emit func(model.Reading)) error {
	src, err := b.open()
	if err != nil {
		return err
	}
	defer src.Close()
	slog.Info("serial port opened", "device", b.cfg.Device)
	return b.readLoop(ctx, src, emit)
}

// readLoop expects src.Read to return (0, nil) when the line was idle for a
// while, as serial.Port does.
func (b *Backend) readLoop(ctx context.Context, src io.Reader, emit func(model.Reading)) error {
	fr := NewFramer(b.cfg.Scaling)
	buf := make([]byte, 512)
	opened := b.now()
	lastData := opened
	lastFrame := opened
	var prevDiscarded uint64

	for ctx.Err() == nil {
		n, err := src.Read(buf)
		now := b.now()
		if err != nil {
			return err
		}
		if n == 0 || now.Sub(lastData) > b.cfg.GapAfter {
			fr.Gap()
		}
		if n > 0 {
			lastData = now
			for _, f := range fr.Feed(buf[:n]) {
				b.frames.Add(1)
				r := b.cfg.Scaling.Decode(f, b.cfg.Riders)
				slog.Debug("frame", "voltage_v", r.Voltage, "generated_w", r.GeneratedPower,
					"load_w", r.LoadPower, "temperature_c", r.Temperature, "status", f.StatusWord)
				emit(r)
				lastFrame = now
			}
		}
		if d := fr.Stats().Discarded; d != prevDiscarded {
			slog.Debug("discarded bytes while resyncing", "count", d-prevDiscarded)
			b.discarded.Add(d - prevDiscarded)
			prevDiscarded = d
		}
		if now.Sub(lastFrame) > b.cfg.StaleAfter {
			return staleError{}
		}
	}
	return ctx.Err()
}

// Package sim is a backend that generates plausible readings, for demos and
// testing without hardware.
package sim

import (
	"context"
	"math"
	"math/rand/v2"
	"time"

	"kinobox/internal/model"
)

type Config struct {
	Riders int
	Period time.Duration // time between readings
}

type Backend struct {
	cfg     Config
	start   time.Time
	voltage float64
	phase   []float64
	effort  []float64
}

func New(cfg Config) *Backend {
	if cfg.Period == 0 {
		cfg.Period = 500 * time.Millisecond
	}
	b := &Backend{cfg: cfg, start: time.Now(), voltage: 12.8}
	for range cfg.Riders {
		b.phase = append(b.phase, rand.Float64()*2*math.Pi)
		b.effort = append(b.effort, 0.5+rand.Float64())
	}
	return b
}

func (b *Backend) Name() string { return "simulator" }

func (b *Backend) Run(ctx context.Context, emit func(model.Reading)) error {
	tick := time.NewTicker(b.cfg.Period)
	defer tick.Stop()
	for {
		emit(b.reading(time.Since(b.start).Seconds()))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func (b *Backend) reading(t float64) model.Reading {
	var r model.Reading
	for i := range b.cfg.Riders {
		// Riders pedal in bursts of roughly a minute with pauses in between.
		cycle := math.Sin(t/37*(1+0.1*float64(i)) + b.phase[i])
		w := 0.0
		if cycle > -0.3 {
			w = b.effort[i] * (70 + 60*cycle + 15*math.Sin(t*1.7+b.phase[i]) + rand.NormFloat64()*6)
		}
		w = max(w, 0)
		r.Riders = append(r.Riders, model.Rider{Position: i + 1, Current: w / b.voltage, Power: w})
		r.GeneratedPower += w
	}
	// Projector plus sound system, with a little ripple.
	r.LoadPower = max(280+20*math.Sin(t/5)+rand.NormFloat64()*4, 0)
	r.LoadCurrent = r.LoadPower / b.voltage

	// Battery voltage drifts with the power balance.
	b.voltage += (r.GeneratedPower - r.LoadPower) / 2e5
	b.voltage = min(max(b.voltage, 11.6), 13.9)
	r.Voltage = b.voltage
	if b.voltage >= 13.85 {
		r.Status.HighVoltage = model.Warning
	}
	r.Temperature = 31 + 3*math.Sin(t/90)
	return r
}

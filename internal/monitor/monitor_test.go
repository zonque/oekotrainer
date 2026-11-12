package monitor

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"kinobox/internal/model"
)

func newTestMonitor(now *time.Time) *Monitor {
	m := New(Config{})
	m.now = func() time.Time { return *now }
	return m
}

// reading with one 1000 W rider and the given load.
func reading(loadW float64) model.Reading {
	return model.Reading{
		Voltage:        20,
		LoadPower:      loadW,
		GeneratedPower: 1000,
		Riders:         []model.Rider{{Position: 1, Current: 50, Power: 1000}, {Position: 2}},
	}
}

func TestEnergyIntegration(t *testing.T) {
	now := time.Unix(1000, 0)
	m := newTestMonitor(&now)

	m.handleReading(reading(0))
	now = now.Add(2 * time.Second)
	m.handleReading(reading(0))

	s := m.Snapshot()
	if math.Abs(s.GeneratedEnergyJ-2000) > 1e-6 {
		t.Errorf("generated energy = %v J, want 2000", s.GeneratedEnergyJ)
	}
	if len(s.RiderEnergyJ) != 2 || math.Abs(s.RiderEnergyJ[0]-2000) > 1e-6 {
		t.Errorf("rider energy = %v, want [2000 0]", s.RiderEnergyJ)
	}
	if s.ConsumedEnergyJ != 0 {
		t.Errorf("consumed = %v, want 0", s.ConsumedEnergyJ)
	}
	if s.Readings != 2 {
		t.Errorf("readings = %d", s.Readings)
	}
}

func TestEnergySkipsLongGaps(t *testing.T) {
	now := time.Unix(1000, 0)
	m := newTestMonitor(&now)
	m.handleReading(reading(500))
	now = now.Add(time.Minute) // connection was lost in between
	m.handleReading(reading(500))
	if e := m.Snapshot().ConsumedEnergyJ; e != 0 {
		t.Errorf("consumed energy across outage = %v, want 0", e)
	}
}

func TestOnline(t *testing.T) {
	now := time.Unix(1000, 0)
	m := newTestMonitor(&now)
	if m.Snapshot().Online {
		t.Error("online before first reading")
	}
	m.handleReading(reading(0))
	if !m.Snapshot().Online {
		t.Error("not online after reading")
	}
	now = now.Add(m.cfg.OfflineAfter + time.Millisecond)
	if m.Snapshot().Online {
		t.Error("still online after offline timeout")
	}
}

func TestHistory(t *testing.T) {
	now := time.Unix(1000, 0)
	m := newTestMonitor(&now)
	for range 10 * 60 * 2 { // 10 minutes of readings at 2 Hz
		m.handleReading(reading(0))
		now = now.Add(500 * time.Millisecond)
	}
	h := m.History()
	if len(h) < 295 || len(h) > 301 {
		t.Fatalf("history has %d samples, want ~300 (one per second for 5 min)", len(h))
	}
	if span := h[len(h)-1].T - h[0].T; span > HistoryWindow.Milliseconds() {
		t.Errorf("history spans %d ms, more than the window", span)
	}
	if h[len(h)-1].Gen != 1000 {
		t.Errorf("latest gen = %v, want 1000", h[len(h)-1].Gen)
	}
}

// fakeBackend emits one reading per Run and fails, until the third run.
type fakeBackend struct {
	runs   int
	cancel context.CancelFunc
}

func (f *fakeBackend) Name() string                { return "fake" }
func (f *fakeBackend) Counters() map[string]uint64 { return map[string]uint64{"widgets": 7} }
func (f *fakeBackend) Run(ctx context.Context, emit func(model.Reading)) error {
	f.runs++
	emit(reading(0))
	if f.runs == 3 {
		f.cancel()
		return ctx.Err()
	}
	return errors.New("boom")
}

func TestRunRestartsBackend(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m := New(Config{RetryEvery: time.Millisecond})
	fb := &fakeBackend{cancel: cancel}
	m.Run(ctx, fb)

	s := m.Snapshot()
	if fb.runs != 3 || s.Restarts != 2 || s.Readings != 3 {
		t.Errorf("runs=%d restarts=%d readings=%d", fb.runs, s.Restarts, s.Readings)
	}
	if s.Source != "fake" || s.BackendCounters["widgets"] != 7 {
		t.Errorf("source=%q counters=%v", s.Source, s.BackendCounters)
	}
}

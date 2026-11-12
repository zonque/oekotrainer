// Package monitor runs a backend, keeps the latest reading and integrates
// energy.
package monitor

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"kinobox/internal/backend"
	"kinobox/internal/model"
)

type Config struct {
	// OfflineAfter: without a reading for this long the source is reported
	// offline, and energy is not integrated across the gap.
	OfflineAfter time.Duration
	// RetryEvery: delay before a failed backend is restarted.
	RetryEvery time.Duration
}

// Snapshot is a consistent copy of the monitor state.
type Snapshot struct {
	Online      bool           `json:"online"`
	Source      string         `json:"source"`
	LastReading time.Time      `json:"last_reading"`
	Reading     *model.Reading `json:"reading"`

	GeneratedEnergyJ float64   `json:"generated_energy_j"`
	ConsumedEnergyJ  float64   `json:"consumed_energy_j"`
	RiderEnergyJ     []float64 `json:"rider_energy_j"`
	Since            time.Time `json:"since"`

	Readings uint64 `json:"readings"`
	Restarts uint64 `json:"restarts"`
	// BackendCounters are source-specific counters, see backend.Counters.
	BackendCounters map[string]uint64 `json:"backend_counters,omitempty"`
}

// HistoryWindow is how far back History reaches; one sample per second.
const HistoryWindow = 5 * time.Minute

// Sample is a point of the power history.
type Sample struct {
	T    int64   `json:"t"` // Unix milliseconds
	Gen  float64 `json:"gen"`
	Load float64 `json:"load"`
}

type Monitor struct {
	cfg Config
	now func() time.Time

	mu          sync.Mutex
	backend     backend.Backend
	reading     *model.Reading
	lastReading time.Time
	genJ        float64
	loadJ       float64
	riderJ      []float64
	since       time.Time
	readings    uint64
	restarts    uint64
	history     []Sample
}

func New(cfg Config) *Monitor {
	if cfg.OfflineAfter == 0 {
		cfg.OfflineAfter = 3 * time.Second
	}
	if cfg.RetryEvery == 0 {
		cfg.RetryEvery = 2 * time.Second
	}
	m := &Monitor{cfg: cfg, now: time.Now}
	m.since = m.now()
	return m
}

// Run runs the backend until ctx is cancelled, restarting it whenever it
// returns an error.
func (m *Monitor) Run(ctx context.Context, b backend.Backend) {
	m.mu.Lock()
	m.backend = b
	m.mu.Unlock()

	for first := true; ctx.Err() == nil; first = false {
		if !first {
			m.mu.Lock()
			m.restarts++
			m.mu.Unlock()
		}
		err := b.Run(ctx, m.handleReading)
		if ctx.Err() != nil {
			return
		}
		slog.Warn("backend stopped, restarting", "source", b.Name(), "err", err, "in", m.cfg.RetryEvery)
		select {
		case <-ctx.Done():
		case <-time.After(m.cfg.RetryEvery):
		}
	}
}

func (m *Monitor) handleReading(r model.Reading) {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.reading != nil {
		// Rectangle rule with the previous sample over real elapsed time.
		// Outages are not bridged.
		if dt := now.Sub(m.lastReading).Seconds(); dt > 0 && dt <= m.cfg.OfflineAfter.Seconds() {
			m.genJ += m.reading.GeneratedPower * dt
			m.loadJ += m.reading.LoadPower * dt
			for i, rd := range m.reading.Riders {
				m.riderJ[i] += rd.Power * dt
			}
		}
	}
	r.Riders = append([]model.Rider(nil), r.Riders...)
	for len(m.riderJ) < len(r.Riders) {
		m.riderJ = append(m.riderJ, 0)
	}
	m.reading = &r
	m.lastReading = now
	m.readings++

	if n := len(m.history); n == 0 || now.UnixMilli()-m.history[n-1].T >= 1000 {
		m.history = append(m.history, Sample{T: now.UnixMilli(), Gen: r.GeneratedPower, Load: r.LoadPower})
		cut := 0
		for cut < len(m.history) && m.history[cut].T < now.Add(-HistoryWindow).UnixMilli() {
			cut++
		}
		m.history = append(m.history[:0], m.history[cut:]...)
	}
}

// History returns the power samples of the last HistoryWindow.
func (m *Monitor) History() []Sample {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Sample(nil), m.history...)
}

func (m *Monitor) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Snapshot{
		Online:           m.reading != nil && m.now().Sub(m.lastReading) <= m.cfg.OfflineAfter,
		LastReading:      m.lastReading,
		GeneratedEnergyJ: m.genJ,
		ConsumedEnergyJ:  m.loadJ,
		RiderEnergyJ:     append([]float64(nil), m.riderJ...),
		Since:            m.since,
		Readings:         m.readings,
		Restarts:         m.restarts,
	}
	if m.backend != nil {
		s.Source = m.backend.Name()
		if c, ok := m.backend.(backend.Counters); ok {
			s.BackendCounters = c.Counters()
		}
	}
	if m.reading != nil {
		r := *m.reading
		r.Riders = append([]model.Rider(nil), r.Riders...)
		s.Reading = &r
	}
	return s
}

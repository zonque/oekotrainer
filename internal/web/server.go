// Package web serves the dashboard, a JSON API and Prometheus metrics.
package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"

	"kinobox/internal/monitor"
)

//go:embed static
var static embed.FS

// Source provides the current state.
type Source interface {
	Snapshot() monitor.Snapshot
	History() []monitor.Sample
}

// UIConfig is passed to the dashboard to scale its displays.
type UIConfig struct {
	RiderMaxW float64 `json:"rider_max_w"` // full scale of a rider bar
	// Battery voltage limits for colouring: warning and trip levels.
	VoltWarnLow  float64 `json:"volt_warn_low"`
	VoltTripLow  float64 `json:"volt_trip_low"`
	VoltWarnHigh float64 `json:"volt_warn_high"`
	VoltTripHigh float64 `json:"volt_trip_high"`
}

// Readings is the response of /api/readings.
type Readings struct {
	monitor.Snapshot
	Config UIConfig `json:"config"`
}

func Handler(src Source, ui UIConfig) http.Handler {
	mux := http.NewServeMux()

	sub, _ := fs.Sub(static, "static")
	mux.Handle("GET /", http.FileServerFS(sub))

	mux.HandleFunc("GET /api/readings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(Readings{Snapshot: src.Snapshot(), Config: ui})
	})

	mux.HandleFunc("GET /api/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(src.History())
	})

	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		writeMetrics(w, src.Snapshot())
	})
	return mux
}

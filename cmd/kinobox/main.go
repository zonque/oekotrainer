// Command kinobox reads data from a backend (a KinoBox on a serial port, or the
// simulator) and serves a live dashboard, a JSON API and Prometheus metrics.
package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kinobox/internal/backend"
	"kinobox/internal/backend/kinobox"
	"kinobox/internal/backend/sim"
	"kinobox/internal/config"
	"kinobox/internal/monitor"
	"kinobox/internal/web"
)

const defaultConfig = "kinobox.toml"

// version is set at release build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	configPath := flag.String("config", defaultConfig, "path to the TOML configuration file")
	debug := flag.Bool("debug", false, "log at debug level")
	flag.Parse()

	if *debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		slog.Error("configuration", "err", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	b := newBackend(cfg)
	mon := monitor.New(monitor.Config{OfflineAfter: cfg.OfflineAfter})
	go mon.Run(ctx, b)

	d := cfg.Dashboard
	ui := web.UIConfig{
		RiderMaxW:   d.RiderMaxW,
		VoltWarnLow: d.VoltWarnLow, VoltTripLow: d.VoltTripLow,
		VoltWarnHigh: d.VoltWarnHigh, VoltTripHigh: d.VoltTripHigh,
	}
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           web.Handler(mon, ui),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	slog.Info("serving dashboard", "version", version, "addr", cfg.Listen, "backend", cfg.BackendName(), "source", b.Name())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("http server", "err", err)
		os.Exit(1)
	}
}

func newBackend(cfg config.Config) backend.Backend {
	switch cfg.BackendName() {
	case config.BackendKinobox:
		k := cfg.Kinobox
		return kinobox.New(kinobox.Config{
			Device: k.Device, Scaling: k.Scaling(), Riders: cfg.Riders, StaleAfter: k.StaleTimeout,
		})
	default:
		if cfg.Backend == "" {
			slog.Info("no serial device configured, using simulated data")
		}
		return sim.New(sim.Config{Riders: cfg.Riders, Period: cfg.Simulator.Period})
	}
}

// loadConfig reads the configuration file. A missing file is only an error if
// it was named explicitly; otherwise the defaults are used.
func loadConfig(path string) (config.Config, error) {
	explicit := false
	flag.Visit(func(f *flag.Flag) { explicit = explicit || f.Name == "config" })

	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) && !explicit {
		slog.Info("no configuration file, using defaults", "path", path)
		return config.Default(), nil
	}
	cfg, err := config.Load(path)
	if err == nil {
		slog.Info("configuration loaded", "path", path)
	}
	return cfg, err
}

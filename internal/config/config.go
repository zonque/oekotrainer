// Package config loads the TOML configuration file.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"kinobox/internal/backend/kinobox"
)

const (
	BackendKinobox   = "kinobox"
	BackendSimulator = "simulator"
)

type Config struct {
	// Backend selects the data source: "kinobox" or "simulator". Empty picks
	// kinobox if kinobox.device is set, the simulator otherwise.
	Backend string `toml:"backend"`
	Listen  string `toml:"listen"`
	Riders  int    `toml:"riders"`
	// OfflineAfter: without a reading for this long the source is shown as
	// offline.
	OfflineAfter time.Duration `toml:"offline_after"`

	Kinobox   Kinobox   `toml:"kinobox"`
	Simulator Simulator `toml:"simulator"`
	Dashboard Dashboard `toml:"dashboard"`
	Overlay   Overlay   `toml:"overlay"`
}

// Kinobox configures the serial KinoBox backend.
type Kinobox struct {
	Device string `toml:"device"`
	// StaleTimeout: reopen the device after this long without a valid frame.
	StaleTimeout time.Duration `toml:"stale_timeout"`
	ADCBits      uint          `toml:"adc_bits"`
	MaxV         float64       `toml:"max_v"`
	BattMaxA     float64       `toml:"batt_max_a"`
	UserMaxA     float64       `toml:"user_max_a"`
}

// Simulator configures the simulator backend.
type Simulator struct {
	Period time.Duration `toml:"period"`
}

type Dashboard struct {
	RiderMaxW    float64 `toml:"rider_max_w"`
	VoltWarnLow  float64 `toml:"volt_warn_low"`
	VoltTripLow  float64 `toml:"volt_trip_low"`
	VoltWarnHigh float64 `toml:"volt_warn_high"`
	VoltTripHigh float64 `toml:"volt_trip_high"`
}

func Default() Config {
	sc := kinobox.DefaultScaling()
	return Config{
		Listen:       ":8080",
		Riders:       8,
		OfflineAfter: 3 * time.Second,
		Kinobox: Kinobox{
			StaleTimeout: 3 * time.Second,
			ADCBits:      sc.ADCBits, MaxV: sc.MaxV, BattMaxA: sc.BattMaxA, UserMaxA: sc.UserMaxA,
		},
		Simulator: Simulator{Period: 500 * time.Millisecond},
		Dashboard: Dashboard{RiderMaxW: 150, VoltWarnLow: 11.5, VoltTripLow: 11, VoltWarnHigh: 14, VoltTripHigh: 15},
		Overlay:   defaultOverlay(),
	}
}

// Load reads path on top of the defaults and validates the result.
func Load(path string) (Config, error) {
	c := Default()
	md, err := toml.DecodeFile(path, &c)
	if err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		keys := make([]string, len(und))
		for i, k := range und {
			keys[i] = k.String()
		}
		return c, fmt.Errorf("%s: unknown keys: %s", path, strings.Join(keys, ", "))
	}
	if err := c.Validate(); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// BackendName resolves the configured backend, including the automatic choice.
func (c Config) BackendName() string {
	if c.Backend != "" {
		return c.Backend
	}
	if c.Kinobox.Device != "" {
		return BackendKinobox
	}
	return BackendSimulator
}

func (c Config) Validate() error {
	var errs []error
	if c.Riders < 1 || c.Riders > 32 {
		errs = append(errs, errors.New("riders must be between 1 and 32"))
	}
	if c.OfflineAfter <= 0 {
		errs = append(errs, errors.New("offline_after must be positive"))
	}
	switch c.BackendName() {
	case BackendKinobox:
		errs = append(errs, c.Kinobox.validate(c.Riders))
	case BackendSimulator:
		if c.Simulator.Period < 10*time.Millisecond {
			errs = append(errs, errors.New("simulator.period must be at least 10ms"))
		}
	default:
		errs = append(errs, fmt.Errorf("unknown backend %q (want %q or %q)", c.Backend, BackendKinobox, BackendSimulator))
	}
	if c.Dashboard.RiderMaxW <= 0 {
		errs = append(errs, errors.New("dashboard.rider_max_w must be positive"))
	}
	errs = append(errs, c.Overlay.validate())
	return errors.Join(errs...)
}

func (k Kinobox) validate(riders int) error {
	var errs []error
	if k.Device == "" {
		errs = append(errs, errors.New("kinobox.device must be set"))
	}
	if riders > kinobox.NumChannels {
		errs = append(errs, fmt.Errorf("the kinobox backend supports at most %d riders", kinobox.NumChannels))
	}
	if k.ADCBits < 1 || k.ADCBits > 16 {
		errs = append(errs, errors.New("kinobox.adc_bits must be between 1 and 16"))
	}
	if k.MaxV <= 0 || k.BattMaxA <= 0 || k.UserMaxA <= 0 {
		errs = append(errs, errors.New("kinobox.max_v, batt_max_a and user_max_a must be positive"))
	}
	if k.StaleTimeout <= 0 {
		errs = append(errs, errors.New("kinobox.stale_timeout must be positive"))
	}
	return errors.Join(errs...)
}

func (k Kinobox) Scaling() kinobox.Scaling {
	return kinobox.Scaling{ADCBits: k.ADCBits, MaxV: k.MaxV, BattMaxA: k.BattMaxA, UserMaxA: k.UserMaxA}
}

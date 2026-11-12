package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"time"
)

// Overlay configures the kinobox-overlay status strip.
type Overlay struct {
	// Server is the base URL of the kinobox HTTP server.
	Server       string        `toml:"server"`
	PollInterval time.Duration `toml:"poll_interval"`
	// Position is "top" or "bottom".
	Position string `toml:"position"`
	// Height of the strip in device-independent pixels.
	Height int `toml:"height"`
	// Monitor index (0 = primary), see the log output for the list.
	Monitor int `toml:"monitor"`
	// ClickThrough passes mouse input to the window below.
	ClickThrough bool `toml:"click_through"`
	// ShowTotals adds generated/consumed power and battery on the right.
	ShowTotals bool `toml:"show_totals"`
	// ShowEnergy shows each rider's energy next to the power.
	ShowEnergy bool `toml:"show_energy"`
	// Font is an optional TrueType/OpenType font file; empty uses Go Sans.
	Font string `toml:"font"`
	// FontScale scales all text relative to the strip height.
	FontScale float64       `toml:"font_scale"`
	Colors    OverlayColors `toml:"colors"`
}

type OverlayColors struct {
	Background Color `toml:"background"`
	Segment    Color `toml:"segment"` // background of a rider segment
	Text       Color `toml:"text"`
	TextMuted  Color `toml:"text_muted"`
	Bar        Color `toml:"bar"`
	BarTrack   Color `toml:"bar_track"`
	Leader     Color `toml:"leader"` // marks the strongest rider
	Good       Color `toml:"good"`
	Warning    Color `toml:"warning"`
	Critical   Color `toml:"critical"`
}

func defaultOverlay() Overlay {
	return Overlay{
		Server:       "http://localhost:8080",
		PollInterval: 500 * time.Millisecond,
		Position:     "top",
		Height:       56,
		ClickThrough: true,
		ShowTotals:   true,
		ShowEnergy:   true,
		FontScale:    1,
		Colors: OverlayColors{
			Background: mustColor("#0d0d0dc0"),
			Segment:    mustColor("#ffffff0d"),
			Text:       mustColor("#f0efec"),
			TextMuted:  mustColor("#a9a8a0"),
			Bar:        mustColor("#3987e5"),
			BarTrack:   mustColor("#3987e533"),
			Leader:     mustColor("#fab219"),
			Good:       mustColor("#0ca30c"),
			Warning:    mustColor("#fab219"),
			Critical:   mustColor("#e34948"),
		},
	}
}

func (o Overlay) validate() error {
	var errs []error
	if o.Position != "top" && o.Position != "bottom" {
		errs = append(errs, errors.New(`overlay.position must be "top" or "bottom"`))
	}
	if o.Height < 16 || o.Height > 400 {
		errs = append(errs, errors.New("overlay.height must be between 16 and 400"))
	}
	if o.PollInterval < 50*time.Millisecond {
		errs = append(errs, errors.New("overlay.poll_interval must be at least 50ms"))
	}
	if o.FontScale <= 0 {
		errs = append(errs, errors.New("overlay.font_scale must be positive"))
	}
	if !strings.HasPrefix(o.Server, "http://") && !strings.HasPrefix(o.Server, "https://") {
		errs = append(errs, errors.New("overlay.server must be an http:// or https:// URL"))
	}
	return errors.Join(errs...)
}

// Color is written as "#rrggbb" or "#rrggbbaa" in the config file.
type Color color.NRGBA

func (c *Color) UnmarshalText(b []byte) error {
	s := string(b)
	if !strings.HasPrefix(s, "#") || (len(s) != 7 && len(s) != 9) {
		return fmt.Errorf("invalid color %q, want #rrggbb or #rrggbbaa", s)
	}
	v, err := hex.DecodeString(s[1:])
	if err != nil {
		return fmt.Errorf("invalid color %q: %w", s, err)
	}
	*c = Color{v[0], v[1], v[2], 0xff}
	if len(v) == 4 {
		c.A = v[3]
	}
	return nil
}

func (c Color) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)), nil
}

// RGBA implements color.Color.
func (c Color) RGBA() (r, g, b, a uint32) { return color.NRGBA(c).RGBA() }

func mustColor(s string) Color {
	var c Color
	if err := c.UnmarshalText([]byte(s)); err != nil {
		panic(err)
	}
	return c
}

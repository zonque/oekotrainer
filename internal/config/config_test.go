package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "kinobox.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad(t *testing.T) {
	p := write(t, `
listen = "127.0.0.1:9000"
riders = 6
offline_after = "5s"

[kinobox]
device = "/dev/ttyUSB1"
stale_timeout = "4s"
adc_bits = 12
max_v = 25.0

[dashboard]
rider_max_w = 200
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:9000" || c.Riders != 6 || c.OfflineAfter != 5*time.Second {
		t.Errorf("top level: %+v", c)
	}
	k := c.Kinobox
	if k.Device != "/dev/ttyUSB1" || k.StaleTimeout != 4*time.Second || k.ADCBits != 12 || k.MaxV != 25 {
		t.Errorf("kinobox: %+v", k)
	}
	// unset keys keep their defaults
	if k.BattMaxA != 150 || k.UserMaxA != 50 {
		t.Errorf("kinobox defaults lost: %+v", k)
	}
	if c.Dashboard.RiderMaxW != 200 || c.Dashboard.VoltTripHigh != 15 {
		t.Errorf("dashboard: %+v", c.Dashboard)
	}
	if c.BackendName() != BackendKinobox {
		t.Errorf("backend = %q, want kinobox when a device is set", c.BackendName())
	}
}

func TestBackendSelection(t *testing.T) {
	if got := Default().BackendName(); got != BackendSimulator {
		t.Errorf("default backend = %q, want simulator", got)
	}
	if err := Default().Validate(); err != nil {
		t.Errorf("default config invalid: %v", err)
	}
	// explicit simulator wins over a configured device
	c, err := Load(write(t, "backend = \"simulator\"\n[kinobox]\ndevice = \"/dev/ttyUSB0\"\n"))
	if err != nil || c.BackendName() != BackendSimulator {
		t.Errorf("backend = %q, err = %v", c.BackendName(), err)
	}
	if _, err := Load(write(t, "backend = \"kinobox\"\n")); err == nil {
		t.Error("kinobox backend without device accepted")
	}
	if _, err := Load(write(t, "backend = \"carrier-pigeon\"\n")); err == nil {
		t.Error("unknown backend accepted")
	}
}

func TestUnknownKey(t *testing.T) {
	p := write(t, "[kinobox]\ndevcie = \"/dev/ttyUSB0\"\n")
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "devcie") {
		t.Fatalf("want error naming the unknown key, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		toml string
	}{
		{"riders", "riders = 40"},
		{"kinobox riders", "riders = 11\n[kinobox]\ndevice = \"/dev/x\""},
		{"adc_bits", "[kinobox]\ndevice = \"/dev/x\"\nadc_bits = 0"},
		{"max_v", "[kinobox]\ndevice = \"/dev/x\"\nmax_v = -1"},
	} {
		if _, err := Load(write(t, tc.toml)); err == nil {
			t.Errorf("%s: invalid config accepted", tc.name)
		}
	}
}

func TestExampleConfigLoads(t *testing.T) {
	c, err := Load("../../kinobox.example.toml")
	if err != nil {
		t.Fatalf("example config: %v", err)
	}
	if c.BackendName() != BackendKinobox {
		t.Errorf("example config backend = %q", c.BackendName())
	}
}

func TestOverlay(t *testing.T) {
	c, err := Load(write(t, `
[overlay]
server = "http://box.local:8080"
position = "bottom"
height = 40

[overlay.colors]
background = "#10203080"
bar = "#ff0000"
`))
	if err != nil {
		t.Fatal(err)
	}
	o := c.Overlay
	if o.Server != "http://box.local:8080" || o.Position != "bottom" || o.Height != 40 {
		t.Errorf("overlay: %+v", o)
	}
	if got := o.Colors.Background; got != (Color{R: 0x10, G: 0x20, B: 0x30, A: 0x80}) {
		t.Errorf("background = %v", got)
	}
	if got := o.Colors.Bar; got != (Color{R: 0xff, A: 0xff}) {
		t.Errorf("bar = %v (no alpha means opaque)", got)
	}
	if o.Colors.Text != Default().Overlay.Colors.Text {
		t.Error("unset color lost its default")
	}
}

func TestOverlayInvalid(t *testing.T) {
	for _, s := range []string{
		"[overlay]\nposition = \"left\"",
		"[overlay]\nheight = 5",
		"[overlay.colors]\nbar = \"red\"",
		"[overlay.colors]\nbar = \"#12345\"",
	} {
		if _, err := Load(write(t, s)); err == nil {
			t.Errorf("accepted: %s", s)
		}
	}
}

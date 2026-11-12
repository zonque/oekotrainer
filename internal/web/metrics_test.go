package web

import (
	"strings"
	"testing"
	"time"

	"kinobox/internal/model"
	"kinobox/internal/monitor"
)

func TestWriteMetrics(t *testing.T) {
	s := monitor.Snapshot{
		Online:      true,
		Source:      `/dev/tty"x"`,
		LastReading: time.Unix(1700000000, 500e6),
		Reading: &model.Reading{
			Voltage:        12.5,
			LoadCurrent:    20,
			LoadPower:      250,
			GeneratedPower: 300,
			Temperature:    31.25,
			Riders: []model.Rider{
				{Position: 1, Current: 10, Power: 125},
				{Position: 2, Current: 14, Power: 175},
			},
			Status: model.Status{HighVoltage: model.Warning, Countdown: 4},
		},
		GeneratedEnergyJ: 3600,
		ConsumedEnergyJ:  1800,
		RiderEnergyJ:     []float64{1000, 2600},
		Readings:         42,
		Restarts:         1,
		BackendCounters:  map[string]uint64{"frames": 42, "discarded_bytes": 3},
	}
	var b strings.Builder
	writeMetrics(&b, s)
	out := b.String()

	for _, want := range []string{
		"# TYPE kinobox_up gauge\nkinobox_up 1\n",
		"kinobox_battery_voltage_volts 12.5\n",
		"kinobox_load_power_watts 250\n",
		"kinobox_generated_power_watts 300\n",
		"kinobox_temperature_celsius 31.25\n",
		`kinobox_rider_power_watts{rider="2"} 175` + "\n",
		`kinobox_rider_current_amperes{rider="1"} 10` + "\n",
		"# TYPE kinobox_rider_energy_joules_total counter\n",
		`kinobox_rider_energy_joules_total{rider="2"} 2600` + "\n",
		"kinobox_generated_energy_joules_total 3600\n",
		"kinobox_consumed_energy_joules_total 1800\n",
		`kinobox_protection_state{protection="high_voltage"} 1` + "\n",
		`kinobox_protection_state{protection="low_voltage"} 0` + "\n",
		"kinobox_protection_countdown_seconds 4\n",
		`kinobox_info{source="/dev/tty\"x\""} 1` + "\n",
		"kinobox_readings_total 42\n",
		"kinobox_backend_restarts_total 1\n",
		"kinobox_backend_frames_total 42\n",
		"kinobox_backend_discarded_bytes_total 3\n",
		"kinobox_last_reading_timestamp_seconds 1.7000000005e+09\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestWriteMetricsNoReading(t *testing.T) {
	var b strings.Builder
	writeMetrics(&b, monitor.Snapshot{})
	out := b.String()
	if !strings.Contains(out, "kinobox_up 0\n") {
		t.Errorf("missing kinobox_up 0 in:\n%s", out)
	}
	if strings.Contains(out, "kinobox_battery_voltage_volts") {
		t.Errorf("gauges emitted without a reading:\n%s", out)
	}
}

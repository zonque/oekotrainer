package kinobox

import (
	"encoding/binary"
	"math"
	"testing"

	"kinobox/internal/model"
)

func encode(words [NumWords]uint16) []byte {
	b := make([]byte, FrameSize)
	for i, w := range words {
		binary.LittleEndian.PutUint16(b[2*i:], w)
	}
	return b
}

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-3 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestParseFrame(t *testing.T) {
	var w [NumWords]uint16
	w[0] = 100   // load current
	w[1] = 716   // voltage
	w[2] = 50    // channel 0
	w[11] = 1023 // channel 9
	w[12] = 2<<14 | 1<<12 | 0<<10 | 1<<8 | 7
	w[13] = 2345 // 23.45 °C

	f, err := ParseFrame(encode(w))
	if err != nil {
		t.Fatal(err)
	}
	if f.LoadCurrent != 100 || f.Voltage != 716 || f.Channels[0] != 50 || f.Channels[9] != 1023 {
		t.Errorf("unexpected raw values: %+v", f)
	}
	if f.Temperature != 2345 {
		t.Errorf("temperature raw = %d", f.Temperature)
	}
	st := f.Status()
	if st.LowVoltage != model.Tripped || st.HighVoltage != model.Warning || st.HighTemperature != model.OK || st.HighCurrent != model.Warning {
		t.Errorf("status = %+v", st)
	}
	if st.Countdown != 7 {
		t.Errorf("countdown = %d", st.Countdown)
	}
}

func TestParseFrameWrongSize(t *testing.T) {
	if _, err := ParseFrame(make([]byte, 27)); err == nil {
		t.Fatal("expected error for short frame")
	}
}

func TestValidate(t *testing.T) {
	sc := DefaultScaling()
	var w [NumWords]uint16
	w[1] = 700
	f, _ := ParseFrame(encode(w))
	if err := f.Validate(sc); err != nil {
		t.Errorf("valid frame rejected: %v", err)
	}

	w[1] = 1024 // above ADC range
	f, _ = ParseFrame(encode(w))
	if f.Validate(sc) == nil {
		t.Error("out-of-range voltage accepted")
	}

	w[1] = 700
	w[12] = 3 << 10 // invalid 2-bit state
	f, _ = ParseFrame(encode(w))
	if f.Validate(sc) == nil {
		t.Error("invalid status state accepted")
	}
}

func TestDecode(t *testing.T) {
	sc := DefaultScaling()
	var w [NumWords]uint16
	w[0] = 1023 // 150 A
	w[1] = 716  // ≈ 14.0 V
	w[2] = 1023 // 50 A on slot 0
	w[7] = 1023 // slot 5 -> rider 5 on an 8-rider box
	w[6] = 1023 // slot 4 is unused on an 8-rider box
	w[13] = 2500
	f, _ := ParseFrame(encode(w))

	r := sc.Decode(f, 8)
	volt := 716.0 * 20 / 1023
	approx(t, "Voltage", r.Voltage, volt)
	approx(t, "LoadCurrent", r.LoadCurrent, 150)
	approx(t, "LoadPower", r.LoadPower, 150*volt)
	approx(t, "Temperature", r.Temperature, 25)

	if len(r.Riders) != 8 {
		t.Fatalf("riders = %d, want 8", len(r.Riders))
	}
	approx(t, "rider1 current", r.Riders[0].Current, 50)
	approx(t, "rider1 power", r.Riders[0].Power, 50*volt)
	approx(t, "rider5 current", r.Riders[4].Current, 50) // slot 5
	approx(t, "rider4 current", r.Riders[3].Current, 0)
	approx(t, "GeneratedPower", r.GeneratedPower, 2*50*volt) // slot 4 not counted
}

func TestRiderSlots(t *testing.T) {
	got := RiderSlots(8)
	want := []int{0, 1, 2, 3, 5, 6, 7, 8}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("RiderSlots(8) = %v, want %v", got, want)
		}
	}
	if got := RiderSlots(3); len(got) != 3 || got[2] != 2 {
		t.Errorf("RiderSlots(3) = %v", got)
	}
	if got := RiderSlots(12); len(got) != NumChannels {
		t.Errorf("RiderSlots(12) = %v, want clamped to %d", got, NumChannels)
	}
}

package kinobox

import (
	"encoding/binary"
	"fmt"
	"math"

	"kinobox/internal/model"
)

const (
	FrameSize   = 28
	NumWords    = FrameSize / 2
	NumChannels = 10
)

// Frame holds the raw word values of one frame.
type Frame struct {
	LoadCurrent uint16
	Voltage     uint16
	Channels    [NumChannels]uint16
	StatusWord  uint16
	Temperature uint16 // 1/100 °C
}

// ParseFrame splits a 28-byte frame into its words.
func ParseFrame(b []byte) (Frame, error) {
	if len(b) != FrameSize {
		return Frame{}, fmt.Errorf("frame must be %d bytes, got %d", FrameSize, len(b))
	}
	w := func(i int) uint16 { return binary.LittleEndian.Uint16(b[2*i:]) }
	f := Frame{
		LoadCurrent: w(0),
		Voltage:     w(1),
		StatusWord:  w(12),
		Temperature: w(13),
	}
	for i := range f.Channels {
		f.Channels[i] = w(2 + i)
	}
	return f, nil
}

// Status decodes the status word.
func (f Frame) Status() model.Status {
	s := f.StatusWord
	return model.Status{
		LowVoltage:      model.State(s >> 14 & 3),
		HighVoltage:     model.State(s >> 12 & 3),
		HighTemperature: model.State(s >> 10 & 3),
		HighCurrent:     model.State(s >> 8 & 3),
		Countdown:       uint8(s),
	}
}

// Validate is a plausibility check used to detect misaligned frames.
func (f Frame) Validate(sc Scaling) error {
	adcMax := sc.ADCMax()
	if f.LoadCurrent > adcMax || f.Voltage > adcMax {
		return fmt.Errorf("analog value out of ADC range")
	}
	for i, c := range f.Channels {
		if c > adcMax {
			return fmt.Errorf("channel %d out of ADC range", i)
		}
	}
	st := f.Status()
	if st.Worst() > model.Tripped {
		return fmt.Errorf("invalid status word %#04x", f.StatusWord)
	}
	return nil
}

// Scaling converts ADC counts to physical units.
type Scaling struct {
	ADCBits  uint    // ADC resolution in bits
	MaxV     float64 // battery voltage at full scale
	BattMaxA float64 // load current at full scale
	UserMaxA float64 // generator channel current at full scale
}

func DefaultScaling() Scaling {
	return Scaling{ADCBits: 10, MaxV: 20, BattMaxA: 150, UserMaxA: 50}
}

func (sc Scaling) ADCMax() uint16 { return uint16(1<<sc.ADCBits - 1) }

// RiderSlots maps rider positions to channel slots. On the 8-rider box slot 4
// is unused; otherwise the mapping is 1:1.
func RiderSlots(riders int) []int {
	riders = min(max(riders, 0), NumChannels)
	slots := make([]int, riders)
	for i := range slots {
		slots[i] = i
		if riders == 8 && i > 3 {
			slots[i] = i + 1
		}
	}
	return slots
}

// Decode converts a frame to physical units.
func (sc Scaling) Decode(f Frame, riders int) model.Reading {
	adcMax := float64(sc.ADCMax())
	scale := func(v uint16, full float64) float64 { return float64(v) * full / adcMax }

	r := model.Reading{
		Voltage:     scale(f.Voltage, sc.MaxV),
		LoadCurrent: scale(f.LoadCurrent, sc.BattMaxA),
		Temperature: float64(f.Temperature) / 100,
		Status:      f.Status(),
	}
	r.LoadPower = r.Voltage * r.LoadCurrent
	for i, slot := range RiderSlots(riders) {
		cur := scale(f.Channels[slot], sc.UserMaxA)
		rd := model.Rider{Position: i + 1, Current: cur, Power: cur * r.Voltage}
		r.GeneratedPower += rd.Power
		r.Riders = append(r.Riders, rd)
	}
	return r
}

// Encode is the inverse of ParseFrame; used by the simulator and tests.
func (f Frame) Encode() []byte {
	b := make([]byte, FrameSize)
	put := func(i int, v uint16) { binary.LittleEndian.PutUint16(b[2*i:], v) }
	put(0, f.LoadCurrent)
	put(1, f.Voltage)
	for i, c := range f.Channels {
		put(2+i, c)
	}
	put(12, f.StatusWord)
	put(13, f.Temperature)
	return b
}

// Counts converts a physical value back to ADC counts (simulator helper).
func (sc Scaling) Counts(v, full float64) uint16 {
	c := math.Round(v / full * float64(sc.ADCMax()))
	return uint16(min(max(c, 0), float64(sc.ADCMax())))
}

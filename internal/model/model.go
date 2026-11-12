// Package model holds the backend-independent representation of a reading.
package model

// State is the state of one protection function.
type State uint8

const (
	OK      State = 0
	Warning State = 1
	Tripped State = 2
)

func (s State) String() string {
	switch s {
	case OK:
		return "ok"
	case Warning:
		return "warning"
	case Tripped:
		return "tripped"
	}
	return "invalid"
}

func (s State) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

func (s *State) UnmarshalText(b []byte) error {
	switch string(b) {
	case "ok":
		*s = OK
	case "warning":
		*s = Warning
	case "tripped":
		*s = Tripped
	default:
		*s = 3
	}
	return nil
}

// Status is the protection state of the system.
type Status struct {
	LowVoltage      State `json:"low_voltage"`
	HighVoltage     State `json:"high_voltage"`
	HighTemperature State `json:"high_temperature"`
	HighCurrent     State `json:"high_current"`
	// Countdown is the number of seconds until a tripped protection resets.
	Countdown uint8 `json:"countdown_s"`
}

// Worst returns the most severe of the four protection states.
func (s Status) Worst() State {
	return max(s.LowVoltage, s.HighVoltage, s.HighTemperature, s.HighCurrent)
}

type Rider struct {
	Position int     `json:"position"` // 1-based
	Current  float64 `json:"current_a"`
	Power    float64 `json:"power_w"`
}

// Reading is one measurement in physical units.
type Reading struct {
	Voltage        float64 `json:"voltage_v"`
	LoadCurrent    float64 `json:"load_current_a"`
	LoadPower      float64 `json:"load_power_w"`
	GeneratedPower float64 `json:"generated_power_w"`
	Temperature    float64 `json:"temperature_c"`
	Riders         []Rider `json:"riders"`
	Status         Status  `json:"status"`
}

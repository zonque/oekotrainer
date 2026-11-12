package kinobox

import (
	"testing"
	"time"

	"kinobox/internal/model"
)

// scriptedReader returns one step per Read and advances the fake clock.
type scriptedReader struct {
	now   *time.Time
	steps []step
}

type step struct {
	data  []byte
	after time.Duration
}

func (r *scriptedReader) Read(b []byte) (int, error) {
	if len(r.steps) == 0 {
		*r.now = r.now.Add(100 * time.Millisecond)
		return 0, nil // idle line
	}
	s := r.steps[0]
	r.steps = r.steps[1:]
	*r.now = r.now.Add(s.after)
	return copy(b, s.data), nil
}

func TestReadLoop(t *testing.T) {
	now := time.Unix(1000, 0)
	b := New(Config{Scaling: DefaultScaling(), Riders: 8})
	b.now = func() time.Time { return now }

	f1 := Frame{Voltage: 700}.Encode()
	f2 := Frame{Voltage: 701}.Encode()
	src := &scriptedReader{now: &now, steps: []step{
		{data: f1[5:], after: 10 * time.Millisecond},   // tail of a frame we joined late
		{data: f2[:10], after: 500 * time.Millisecond}, // gap -> boundary
		{data: f2[10:], after: 2 * time.Millisecond},
	}}
	var got []model.Reading
	err := b.readLoop(t.Context(), src, func(r model.Reading) { got = append(got, r) })
	if _, ok := err.(staleError); !ok {
		t.Fatalf("readLoop returned %v, want staleError", err)
	}
	if len(got) != 1 || got[0].Voltage != 701.0*20/1023 {
		t.Errorf("readings = %+v", got)
	}
	c := b.Counters()
	if c["frames"] != 1 || c["discarded_bytes"] != uint64(len(f1)-5) {
		t.Errorf("counters = %v", c)
	}
}

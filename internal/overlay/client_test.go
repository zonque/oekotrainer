package overlay

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"kinobox/internal/model"
	"kinobox/internal/monitor"
	"kinobox/internal/web"
)

type fixedSource struct{ s monitor.Snapshot }

func (f fixedSource) Snapshot() monitor.Snapshot { return f.s }
func (f fixedSource) History() []monitor.Sample  { return nil }

func TestClientRoundTrip(t *testing.T) {
	snap := monitor.Snapshot{
		Online: true,
		Source: "test",
		Reading: &model.Reading{
			Voltage: 12.5,
			Riders:  []model.Rider{{Position: 1, Power: 99}},
			Status:  model.Status{HighCurrent: model.Tripped},
		},
		RiderEnergyJ: []float64{42},
	}
	srv := httptest.NewServer(web.Handler(fixedSource{snap}, web.UIConfig{RiderMaxW: 123}))
	defer srv.Close()

	c := NewClient(srv.URL+"/", 10*time.Millisecond)
	ctx, cancel := context.WithCancel(t.Context())
	go c.Run(ctx)
	deadline := time.Now().Add(2 * time.Second)
	var st State
	for v := uint64(0); v == 0 && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
		st, v = c.State()
	}
	cancel()

	if st.Err != nil || st.Data == nil {
		t.Fatalf("state = %+v", st)
	}
	d := st.Data
	if !d.Online || d.Source != "test" || d.Config.RiderMaxW != 123 {
		t.Errorf("snapshot = %+v", d)
	}
	if d.Reading.Riders[0].Power != 99 || d.Reading.Status.HighCurrent != model.Tripped || d.RiderEnergyJ[0] != 42 {
		t.Errorf("reading = %+v", d.Reading)
	}
}

func TestClientUnreachable(t *testing.T) {
	srv := httptest.NewServer(nil)
	url := srv.URL
	srv.Close() // nothing listens there any more

	c := NewClient(url, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(t.Context())
	go c.Run(ctx)
	defer cancel()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st, _ := c.State(); st.Err != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no error reported for an unreachable server")
}

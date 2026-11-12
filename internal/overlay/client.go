package overlay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"kinobox/internal/web"
)

// Client polls the kinobox server and keeps the latest State.
type Client struct {
	url      string
	interval time.Duration
	http     *http.Client

	mu       sync.Mutex
	state    State
	version  uint64 // bumped on every change, so the window knows when to redraw
	onChange func()
}

// OnChange registers fn to be called (from the polling goroutine) after the
// state changed. Call it before Run.
func (c *Client) OnChange(fn func()) { c.onChange = fn }

func NewClient(server string, interval time.Duration) *Client {
	return &Client{
		url:      strings.TrimRight(server, "/") + "/api/readings",
		interval: interval,
		http:     &http.Client{Timeout: max(2*interval, 2*time.Second)},
	}
}

// Run polls until ctx is cancelled.
func (c *Client) Run(ctx context.Context) {
	tick := time.NewTicker(c.interval)
	defer tick.Stop()
	var lastErr string
	for {
		d, err := c.fetch(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil && err.Error() != lastErr {
			slog.Warn("cannot reach server", "url", c.url, "err", err)
		} else if err == nil && lastErr != "" {
			slog.Info("server reachable again", "url", c.url)
		}
		lastErr = ""
		if err != nil {
			lastErr = err.Error()
		}

		c.mu.Lock()
		if err != nil {
			c.state.Err = err // keep the last data, but show the error
		} else {
			c.state = State{Data: d}
		}
		c.version++
		c.mu.Unlock()
		if c.onChange != nil {
			c.onChange()
		}

		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (c *Client) fetch(ctx context.Context) (*web.Readings, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", res.Status)
	}
	var d web.Readings
	if err := json.NewDecoder(res.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &d, nil
}

// State returns the current state and its version.
func (c *Client) State() (State, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state, c.version
}

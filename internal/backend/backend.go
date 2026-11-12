// Package backend defines the interface between data sources and the monitor.
package backend

import (
	"context"

	"kinobox/internal/model"
)

// Backend is a source of readings, e.g. a KinoBox on a serial port.
type Backend interface {
	// Name identifies the source in logs, the API and metrics.
	Name() string
	// Run delivers readings to emit until ctx is cancelled or the source
	// fails. A non-nil error makes the caller retry after a delay; Run is
	// then called again.
	Run(ctx context.Context, emit func(model.Reading)) error
}

// Counters is optionally implemented by backends that keep source-specific
// counters. Keys are snake_case; values only ever increase.
type Counters interface {
	Counters() map[string]uint64
}

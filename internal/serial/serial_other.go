//go:build !linux

package serial

import (
	"fmt"
	"runtime"
)

// Port is not implemented on this platform.
type Port struct{}

func Open(device string) (*Port, error) {
	return nil, fmt.Errorf("serial ports are not supported on %s yet", runtime.GOOS)
}

func (p *Port) Read(b []byte) (int, error) { return 0, fmt.Errorf("not supported") }

func (p *Port) Close() error { return nil }

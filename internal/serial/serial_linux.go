// Package serial opens a tty in raw mode for talking to the KinoBox.
package serial

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Port is a raw serial port. Read returns (0, nil) when no byte arrived within
// the read timeout, which callers use to detect gaps on the line.
type Port struct {
	f *os.File
}

// Open configures device for 115200 8N1 without flow control, with a read
// timeout of 100 ms, and asserts DTR (the box only talks while DTR is set).
func Open(device string) (*Port, error) {
	fd, err := unix.Open(device, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", device, err)
	}
	if err := configure(fd); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("configure %s: %w", device, err)
	}
	// Back to blocking mode; VMIN/VTIME now govern reads.
	if err := unix.SetNonblock(fd, false); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return &Port{f: os.NewFile(uintptr(fd), device)}, nil
}

func configure(fd int) error {
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return err
	}
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR |
		unix.IGNCR | unix.ICRNL | unix.IXON | unix.IXOFF | unix.IXANY
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB | unix.CSTOPB | unix.CRTSCTS | unix.CBAUD
	t.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL | unix.B115200
	t.Ispeed = unix.B115200
	t.Ospeed = unix.B115200
	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = 1 // 1/10 s
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, t); err != nil {
		return err
	}
	// Pseudo-terminals (used for testing) have no modem lines.
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCMBIS, unix.TIOCM_DTR); err != nil && !errors.Is(err, unix.ENOTTY) {
		return fmt.Errorf("set DTR: %w", err)
	}
	// Drop whatever queued up before we were listening.
	return unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH)
}

func (p *Port) Read(b []byte) (int, error) {
	n, err := p.f.Read(b)
	if n == 0 && errors.Is(err, io.EOF) {
		// VTIME expiry surfaces as io.EOF from os.File; it is not an error here.
		return 0, nil
	}
	return n, err
}

func (p *Port) Close() error { return p.f.Close() }

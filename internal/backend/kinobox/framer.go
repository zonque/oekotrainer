package kinobox

// Framer splits a raw byte stream into frames. The protocol has no sync
// marker, so it relies on two signals: a pause on the line (Gap) marks a frame
// boundary, and frames that fail Validate are shifted out byte by byte.
type Framer struct {
	sc    Scaling
	buf   []byte
	stats FramerStats
}

type FramerStats struct {
	Frames    uint64 // valid frames emitted
	Discarded uint64 // bytes dropped while resyncing
}

func NewFramer(sc Scaling) *Framer {
	return &Framer{sc: sc, buf: make([]byte, 0, 4*FrameSize)}
}

// Feed appends data and returns all complete, valid frames.
func (fr *Framer) Feed(data []byte) []Frame {
	fr.buf = append(fr.buf, data...)
	var out []Frame
	for len(fr.buf) >= FrameSize {
		f, _ := ParseFrame(fr.buf[:FrameSize])
		if f.Validate(fr.sc) != nil {
			fr.drop(1)
			continue
		}
		out = append(out, f)
		fr.stats.Frames++
		fr.buf = fr.buf[:copy(fr.buf, fr.buf[FrameSize:])]
	}
	return out
}

// Gap signals that the line was idle; any partial frame is discarded.
func (fr *Framer) Gap() {
	fr.drop(len(fr.buf))
}

func (fr *Framer) drop(n int) {
	fr.stats.Discarded += uint64(n)
	fr.buf = fr.buf[:copy(fr.buf, fr.buf[n:])]
}

func (fr *Framer) Stats() FramerStats { return fr.stats }

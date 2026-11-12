package kinobox

import "testing"

func testFrame(volt uint16) []byte {
	return Frame{Voltage: volt, Channels: [NumChannels]uint16{1, 2, 3}}.Encode()
}

func TestFramerSplitsStream(t *testing.T) {
	fr := NewFramer(DefaultScaling())
	stream := append(testFrame(700), testFrame(701)...)

	// feed in awkward chunk sizes
	var got []Frame
	for i := 0; i < len(stream); i += 5 {
		got = append(got, fr.Feed(stream[i:min(i+5, len(stream))])...)
	}
	if len(got) != 2 || got[0].Voltage != 700 || got[1].Voltage != 701 {
		t.Fatalf("got %+v", got)
	}
}

func TestFramerGapDiscardsPartial(t *testing.T) {
	fr := NewFramer(DefaultScaling())
	if f := fr.Feed(testFrame(700)[:11]); len(f) != 0 {
		t.Fatal("partial frame emitted")
	}
	fr.Gap()
	got := fr.Feed(testFrame(702))
	if len(got) != 1 || got[0].Voltage != 702 {
		t.Fatalf("got %+v", got)
	}
	if fr.Stats().Discarded != 11 {
		t.Errorf("discarded = %d, want 11", fr.Stats().Discarded)
	}
}

func TestFramerResyncsOnGarbage(t *testing.T) {
	fr := NewFramer(DefaultScaling())
	// One stray byte in front misaligns everything: the high bytes become
	// low bytes and fail the ADC range check.
	stream := append([]byte{0xff}, testFrame(700)...)
	stream = append(stream, testFrame(701)...)
	got := fr.Feed(stream)
	if len(got) != 2 || got[0].Voltage != 700 || got[1].Voltage != 701 {
		t.Fatalf("got %+v", got)
	}
	if fr.Stats().Discarded != 1 {
		t.Errorf("discarded = %d, want 1", fr.Stats().Discarded)
	}
}

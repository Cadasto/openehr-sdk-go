package canxml

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
)

// textValue is a value a Decoder given no option decodes through encoding/xml
// alone, so the stream runs through the recorder and nothing else.
type textValue struct {
	Value string `xml:"value"`
}

// TestRecorderGivesBackALargeValuesRoom pins that a long-lived Decoder reading
// a stream it cannot seek does not keep its largest value's size: once a value
// larger than maxIdleRoom is decoded, the next value starts in a buffer sized
// for what the recorder still holds. The values after the large one decode as
// written, from a source read a byte at a time and from one read in blocks,
// which leaves bytes read ahead in the buffer for the next value.
func TestRecorderGivesBackALargeValuesRoom(t *testing.T) {
	t.Parallel()
	large := strings.Repeat("x", 1<<20)
	stream := "<dv><value>" + large + "</value></dv><dv><value>a</value></dv><dv><value>b</value></dv>"
	sources := map[string]func() io.Reader{
		"byte reader": func() io.Reader { return bufio.NewReader(strings.NewReader(stream)) },
		"block reader": func() io.Reader {
			return struct{ io.Reader }{bytes.NewBufferString(stream)} // neither an io.ByteReader nor an io.Seeker
		},
	}
	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := NewDecoder(src())
			if d.rec == nil {
				t.Fatal("premise gone: NewDecoder over this source records no bytes, so nothing here is measured")
			}
			var got []string
			for i := range 3 {
				var v textValue
				if err := d.Decode(&v); err != nil {
					t.Fatalf("Decode(value %d): %v", i, err)
				}
				got = append(got, v.Value)
				if i == 0 && cap(d.rec.buf) < len(large) {
					t.Fatalf("premise gone: after the large value the recorder holds %d bytes of room, want at least %d", cap(d.rec.buf), len(large))
				}
			}
			if got[0] != large || got[1] != "a" || got[2] != "b" {
				t.Errorf("decoded values (%d bytes, %q, %q); want (%d bytes, %q, %q)", len(got[0]), got[1], got[2], len(large), "a", "b")
			}
			if c := cap(d.rec.buf); c > maxIdleRoom+minRead {
				t.Errorf("after two small values the recorder keeps %d bytes of room; want at most %d", c, maxIdleRoom+minRead)
			}
		})
	}
}

// TestByteReplayLetsGoOfTheRecordedBytes pins that a replay reading a
// recorder's bytes lets go of them once it has returned the last: an empty
// slice left pointing into the buffer would keep the whole buffer, the size
// of the value replayed, for as long as the decoder reading the replay lives.
func TestByteReplayLetsGoOfTheRecordedBytes(t *testing.T) {
	t.Parallel()
	rec := &recorder{bsrc: bufio.NewReader(strings.NewReader("tail"))}
	rec.buf = append(make([]byte, 0, 64), "recorded"...)
	r, ok := rec.replay().(*byteReplay)
	if !ok {
		t.Fatalf("replay() of a recorder over an io.ByteReader = %T; want *byteReplay", rec.replay())
	}
	got, err := io.ReadAll(r)
	if err != nil || string(got) != "recordedtail" {
		t.Fatalf("ReadAll(replay) = %q, %v; want %q, nil", got, err, "recordedtail")
	}
	if r.rec != nil {
		t.Errorf("after the recorded bytes the replay still holds a slice of %d bytes of room; want nil", cap(r.rec))
	}
}

package rm

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// directNumberOptions are the option sets the number writers are held to
// MarshalJSON's bytes under: the defaults, the two that make an encoder
// rewrite a raw number it is handed (so -0 becomes 0, and a long integer is
// canonicalised), and whitespace.
var directNumberOptions = []struct {
	name string
	opts []json.Options
}{
	{name: "defaults"},
	{name: "CanonicalizeRawFloats", opts: []json.Options{jsontext.CanonicalizeRawFloats(true)}},
	{name: "CanonicalizeRawInts", opts: []json.Options{jsontext.CanonicalizeRawInts(true)}},
	{name: "Multiline", opts: []json.Options{jsontext.Multiline(true)}},
}

// writeWith runs write on a new encoder with opts and returns what it wrote.
func writeWith(t *testing.T, write func(*jsontext.Encoder) error, opts ...json.Options) string {
	t.Helper()
	var buf bytes.Buffer
	enc := jsontext.NewEncoder(&buf, opts...)
	if err := write(enc); err != nil {
		t.Fatalf("write: %v", err)
	}
	return buf.String()
}

// marshalJSONWritten is what an encoder with opts writes for the bytes a
// MarshalJSON method returns: json/v2 hands them to the encoder as a raw
// value.
func marshalJSONWritten(t *testing.T, marshal func() ([]byte, error), opts ...json.Options) string {
	t.Helper()
	b, err := marshal()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	return writeWith(t, func(enc *jsontext.Encoder) error { return enc.WriteValue(b) }, opts...)
}

// TestREQ052DirectNumberWritersWriteMarshalJSONBytes pins that the writers the
// direct canonical-JSON encoders write a Real, an Integer and an Integer64
// through write, under every option set, the bytes the type's MarshalJSON
// gives the encoder, which the wrapper writes. The reals cover zero and -0,
// both ends of the exponent-free range (1e-6 and just under 1e21), the
// smallest subnormal, the largest float, and 4096 seeded random bit patterns;
// the integers cover both ends of each range and 4096 seeded random values.
func TestREQ052DirectNumberWritersWriteMarshalJSONBytes(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(3, 4))
	reals := []float64{
		0, math.Copysign(0, -1), 1, -1, 0.1, 80.5, 1e-6, 9.999999999999999e-7, 1e-7, 1e20, 1e21,
		999999999999999900000, 1.2345678901234567e-300, 5e-324, 0x1p-1022, math.MaxFloat64, -math.MaxFloat64,
		1 << 53, 1<<53 + 2, 123456789.123,
	}
	for len(reals) < 4096+20 {
		if f := math.Float64frombits(rng.Uint64()); !math.IsNaN(f) && !math.IsInf(f, 0) {
			reals = append(reals, f)
		}
	}
	ints := []int64{0, 1, -1, math.MaxInt32, math.MinInt32, math.MaxInt64, math.MinInt64, 1 << 53, 1<<53 + 1}
	for range 4096 {
		ints = append(ints, int64(rng.Uint64()))
	}
	for _, set := range directNumberOptions {
		t.Run(set.name, func(t *testing.T) {
			t.Parallel()
			for _, f := range reals {
				r := Real(f)
				got := writeWith(t, func(enc *jsontext.Encoder) error { return writeJSONReal(enc, r) }, set.opts...)
				if want := marshalJSONWritten(t, r.MarshalJSON, set.opts...); got != want {
					t.Fatalf("writeJSONReal(%v [%#x]) wrote %s, want %s, as MarshalJSON writes it", f, math.Float64bits(f), got, want)
				}
			}
			for _, n := range ints {
				i, i64 := Integer(n), Integer64(n)
				got := writeWith(t, func(enc *jsontext.Encoder) error { return writeJSONInteger(enc, i) }, set.opts...)
				if want := marshalJSONWritten(t, i.MarshalJSON, set.opts...); got != want {
					t.Fatalf("writeJSONInteger(%d) wrote %s, want %s, as MarshalJSON writes it", i, got, want)
				}
				got = writeWith(t, func(enc *jsontext.Encoder) error { return writeJSONInteger64(enc, i64) }, set.opts...)
				if want := marshalJSONWritten(t, i64.MarshalJSON, set.opts...); got != want {
					t.Fatalf("writeJSONInteger64(%d) wrote %s, want %s, as MarshalJSON writes it", i64, got, want)
				}
			}
		})
	}
}

// TestREQ052DirectRealWriterRefusesAsJSONV2 pins that writeJSONReal refuses a
// NaN or an infinity with the error json/v2 gives the same Real, and writes
// nothing first.
func TestREQ052DirectRealWriterRefusesAsJSONV2(t *testing.T) {
	t.Parallel()
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		r := Real(f)
		var buf bytes.Buffer
		err := writeJSONReal(jsontext.NewEncoder(&buf), r)
		_, want := json.Marshal(r, typereg.EncodeOptions())
		if err == nil || want == nil || err.Error() != want.Error() {
			t.Errorf("writeJSONReal(%v) = %v, want json/v2's %v", f, err, want)
		}
		if buf.Len() != 0 {
			t.Errorf("writeJSONReal(%v) wrote %q before it refused", f, buf.String())
		}
	}
}

// TestREQ052DirectNumberWritersDoNotAllocate pins that the number writers
// build their digits on the stack.
func TestREQ052DirectNumberWritersDoNotAllocate(t *testing.T) {
	enc := jsontext.NewEncoder(io.Discard)
	for name, write := range map[string]func() error{
		"Real":      func() error { return writeJSONReal(enc, 80.5) },
		"Integer":   func() error { return writeJSONInteger(enc, -12345) },
		"Integer64": func() error { return writeJSONInteger64(enc, math.MinInt64) },
	} {
		if n := testing.AllocsPerRun(100, func() {
			enc.Reset(io.Discard)
			if err := write(); err != nil {
				t.Fatal(err)
			}
		}); n != 0 {
			t.Errorf("the %s writer makes %.0f allocations, want 0", name, n)
		}
	}
}

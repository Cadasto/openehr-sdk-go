package rm_test

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// numericEncodeOptions are the option sets the numeric MarshalJSONTo methods
// are held to MarshalJSON's output under: the defaults, the two that make an
// encoder rewrite a raw number it is handed (so -0 becomes 0, and a long
// integer is canonicalised), whitespace, and the v1 defaults.
var numericEncodeOptions = []struct {
	name string
	opts []json.Options
}{
	{name: "defaults"},
	{name: "CanonicalizeRawFloats", opts: []json.Options{jsontext.CanonicalizeRawFloats(true)}},
	{name: "CanonicalizeRawInts", opts: []json.Options{jsontext.CanonicalizeRawInts(true)}},
	{name: "Multiline", opts: []json.Options{jsontext.Multiline(true)}},
	{name: "v1 defaults", opts: []json.Options{jsonv1.DefaultOptionsV1()}},
}

// marshalJSONBytes is what an encoder writes for v through its MarshalJSON
// method: json/v2 hands the method's bytes to the encoder as a raw value, as
// it hands a jsontext.Value's.
func marshalJSONBytes(t *testing.T, v jsonv1.Marshaler, opts ...json.Options) string {
	t.Helper()
	b, err := v.MarshalJSON()
	if err != nil {
		t.Fatalf("%T(%v).MarshalJSON() = %v", v, v, err)
	}
	out, err := json.Marshal(jsontext.Value(b), opts...)
	if err != nil {
		t.Fatalf("json.Marshal(jsontext.Value(%s)) = %v", b, err)
	}
	return string(out)
}

// TestNumericMarshalJSONToWritesMarshalJSONBytes pins that the MarshalJSONTo
// methods of Real, Integer and Integer64 write, under every option set, the
// bytes their MarshalJSON method gives the encoder: json/v2 calls MarshalJSONTo
// in preference, so encoded output depends on the two agreeing.
// The reals cover zero and -0, both ends of the exponent-free range (1e-6 and
// just under 1e21), the smallest subnormal, the largest float, and 4096 seeded
// random bit patterns; the integers cover both ends of each range and 4096
// seeded random values.
//
// Can-fail: a float written with Go's own 'g' form fails the 1e-7 and 1e21
// rows; one written as a jsontext.Float token, which the encoder does not
// rewrite, fails -0 under CanonicalizeRawFloats; an integer written as a token
// fails the 64-bit ends under CanonicalizeRawInts.
func TestNumericMarshalJSONToWritesMarshalJSONBytes(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(1, 2))
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
	for _, set := range numericEncodeOptions {
		t.Run(set.name, func(t *testing.T) {
			t.Parallel()
			for _, f := range reals {
				r := rm.Real(f)
				got, err := json.Marshal(r, set.opts...)
				if want := marshalJSONBytes(t, r, set.opts...); err != nil || string(got) != want {
					t.Fatalf("json.Marshal(rm.Real(%v [%#x])) = %s, %v; want %s, as MarshalJSON writes it", f, math.Float64bits(f), got, err, want)
				}
			}
			for _, n := range ints {
				i, i64 := rm.Integer(n), rm.Integer64(n)
				got, err := json.Marshal(i, set.opts...)
				if want := marshalJSONBytes(t, i, set.opts...); err != nil || string(got) != want {
					t.Fatalf("json.Marshal(rm.Integer(%d)) = %s, %v; want %s, as MarshalJSON writes it", i, got, err, want)
				}
				got, err = json.Marshal(i64, set.opts...)
				if want := marshalJSONBytes(t, i64, set.opts...); err != nil || string(got) != want {
					t.Fatalf("json.Marshal(rm.Integer64(%d)) = %s, %v; want %s, as MarshalJSON writes it", i64, got, err, want)
				}
			}
		})
	}
}

// realViaMarshalJSON is a Real whose only JSON method is MarshalJSON: the
// oracle for how a Real that JSON cannot spell is refused.
type realViaMarshalJSON rm.Real

func (r realViaMarshalJSON) MarshalJSON() ([]byte, error) { return rm.Real(r).MarshalJSON() }

// TestRealMarshalJSONToLeavesNonFiniteToMarshalJSON pins that a NaN or an
// infinity is refused, through json/v2 and through encoding/json, with the
// error a Real with only its MarshalJSON method gets, and that the
// errors.ErrUnsupported MarshalJSONTo hands back to json/v2 never reaches the
// caller. The two errors differ in nothing but the oracle's type name, which
// encoding/json's message carries.
//
// Can-fail: a MarshalJSONTo that refused the value itself, rather than
// leaving it to MarshalJSON, fails the encoding/json rows: their message names
// the method that refused.
func TestRealMarshalJSONToLeavesNonFiniteToMarshalJSON(t *testing.T) {
	t.Parallel()
	type viaReal struct {
		M rm.Real `json:"magnitude"`
	}
	type viaOracle struct {
		M realViaMarshalJSON `json:"magnitude"`
	}
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, got := json.Marshal(viaReal{M: rm.Real(f)})
		_, want := json.Marshal(viaOracle{M: realViaMarshalJSON(f)})
		checkRefusal(t, "json.Marshal", f, got, want)
		_, got = jsonv1.Marshal(viaReal{M: rm.Real(f)})
		_, want = jsonv1.Marshal(viaOracle{M: realViaMarshalJSON(f)})
		checkRefusal(t, "encoding/json.Marshal", f, got, want)
	}
}

func checkRefusal(t *testing.T, call string, f float64, got, want error) {
	t.Helper()
	if got == nil || want == nil {
		t.Fatalf("%s(magnitude %v) = %v; the oracle gives %v; want both to refuse", call, f, got, want)
	}
	if errors.Is(got, errors.ErrUnsupported) {
		t.Errorf("%s(magnitude %v) = %v; want no errors.ErrUnsupported in the chain", call, f, got)
	}
	wantText := strings.ReplaceAll(want.Error(), "rm_test.realViaMarshalJSON", "rm.Real")
	if got.Error() != wantText {
		t.Errorf("%s(magnitude %v) = %q; want %q, as MarshalJSON refuses it", call, f, got, wantText)
	}
}

// TestNumericUnmarshalJSONReadsNumbersAsJSONV2Does pins that Real, Integer and
// Integer64 read a bare number with the value encoding/json/v2 gives it, and
// refuse it where json/v2 does, Real with json/v2's own error. The methods
// parse a well-formed number without json/v2, so the literals include
// every spelling strconv reads but JSON does not: a plus sign, a leading zero,
// a bare fraction or point, a cut-off exponent, hexadecimal, an underscore,
// Infinity and NaN. The Real literals are shortest spellings, which the
// lossy-literal refusal never meets, so every difference from json/v2 is the
// parse's. Integer refusals are checked to leave the receiver as it was; their
// errors, and whole numbers written with a fraction or an exponent, are pinned
// in the Integer tests.
//
// Can-fail: without the JSON grammar check before strconv, `+1`, `01`, `.5`,
// `1.`, `Infinity` and `NaN` decode instead of being refused.
func TestNumericUnmarshalJSONReadsNumbersAsJSONV2Does(t *testing.T) {
	t.Parallel()
	malformed := []string{
		`+1`, `01`, `-01`, `00`, `.5`, `1.`, `1e`, `1e+`, `-`, `0x10`, `1_0`, `Infinity`,
		`-Infinity`, `NaN`, `inf`, `true`, `[1]`, `{}`, `1 2`, ``,
	}
	reals := append([]string{
		`0`, `-0`, `1`, `-1`, `0.5`, `80.5`, `1e5`, `1E5`, `1e+5`, `-1.5E-5`, `5e-324`,
		`1e400`, `-1e400`, ` 7 `,
	}, malformed...)
	ints := append([]string{
		`0`, `-0`, `1`, `-1`, ` 7 `, `2147483647`, `-2147483648`, `2147483648`, `-2147483649`,
		`9223372036854775807`, `-9223372036854775808`, `9223372036854775808`, `-9223372036854775809`,
	}, malformed...)
	rng := rand.New(rand.NewPCG(3, 4))
	for range 2048 {
		if f := math.Float64frombits(rng.Uint64()); !math.IsNaN(f) && !math.IsInf(f, 0) {
			reals = append(reals, strconv.FormatFloat(f, 'g', -1, 64))
		}
		ints = append(ints, strconv.FormatInt(int64(rng.Uint64()), 10), strconv.FormatInt(int64(rng.Int32()), 10))
	}
	for _, lit := range reals {
		var f float64
		wantErr := json.Unmarshal([]byte(lit), &f)
		r := rm.Real(7)
		err := r.UnmarshalJSON([]byte(lit))
		switch {
		case wantErr == nil && (err != nil || math.Float64bits(float64(r)) != math.Float64bits(f)):
			t.Errorf("rm.Real.UnmarshalJSON(%s) = %v, read %v; want nil, %v as json/v2 reads it", lit, err, float64(r), f)
		case wantErr != nil && (err == nil || r != 7):
			t.Errorf("rm.Real.UnmarshalJSON(%s) = %v, read %v; want a refusal that keeps the receiver", lit, err, float64(r))
		case wantErr != nil && lit != `` && err.Error() != "rm.Real: "+wantErr.Error():
			t.Errorf("rm.Real.UnmarshalJSON(%s) = %q; want %q, json/v2's own refusal", lit, err, "rm.Real: "+wantErr.Error())
		}
	}
	for _, lit := range ints {
		var n32 int32
		wantErr := json.Unmarshal([]byte(lit), &n32)
		i := rm.Integer(7)
		err := i.UnmarshalJSON([]byte(lit))
		if (wantErr == nil) != (err == nil) || (err == nil && int32(i) != n32) || (err != nil && i != 7) {
			t.Errorf("rm.Integer.UnmarshalJSON(%s) = %v, read %d; json/v2 gives %v, %d", lit, err, i, wantErr, n32)
		}
		var n64 int64
		wantErr = json.Unmarshal([]byte(lit), &n64)
		i64 := rm.Integer64(7)
		err = i64.UnmarshalJSON([]byte(lit))
		if (wantErr == nil) != (err == nil) || (err == nil && int64(i64) != n64) || (err != nil && i64 != 7) {
			t.Errorf("rm.Integer64.UnmarshalJSON(%s) = %v, read %d; json/v2 gives %v, %d", lit, err, i64, wantErr, n64)
		}
	}
}

// TestNumericJSONMethodsDoNotAllocate pins the cost of the numeric primitives
// on the canonical-JSON hot path: reading a well-formed bare number, and
// writing one to an encoder, allocates nothing. A json.Marshal or
// json.Unmarshal of the value would allocate on every call.
func TestNumericJSONMethodsDoNotAllocate(t *testing.T) {
	enc := jsontext.NewEncoder(io.Discard)
	r, n, n64 := rm.Real(80.5), rm.Integer(42), rm.Integer64(-9000000000)
	rLit, nLit, n64Lit := []byte("80.5"), []byte("42"), []byte("-9000000000")
	cases := []struct {
		name string
		call func() error
	}{
		{name: "Real.UnmarshalJSON", call: func() error { return r.UnmarshalJSON(rLit) }},
		{name: "Integer.UnmarshalJSON", call: func() error { return n.UnmarshalJSON(nLit) }},
		{name: "Integer64.UnmarshalJSON", call: func() error { return n64.UnmarshalJSON(n64Lit) }},
		{name: "Real.MarshalJSONTo", call: func() error { return r.MarshalJSONTo(enc) }},
		{name: "Integer.MarshalJSONTo", call: func() error { return n.MarshalJSONTo(enc) }},
		{name: "Integer64.MarshalJSONTo", call: func() error { return n64.MarshalJSONTo(enc) }},
	}
	for _, tc := range cases {
		if err := tc.call(); err != nil {
			t.Fatalf("%s = %v; want nil", tc.name, err)
		}
		if got := testing.AllocsPerRun(100, func() { _ = tc.call() }); got != 0 {
			t.Errorf("%s allocates %v times per call; want 0", tc.name, got)
		}
	}
}

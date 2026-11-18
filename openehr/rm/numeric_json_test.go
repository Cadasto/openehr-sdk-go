package rm_test

import (
	jsonv1 "encoding/json"
	json "encoding/json/v2"
	"math"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// embedsInteger, embedsInteger64 and embedsReal each embed one numeric
// primitive beside a member of their own, and write both through a
// MarshalJSON of their own.
type embedsInteger struct {
	rm.Integer
	Unit string
}

func (w embedsInteger) MarshalJSON() ([]byte, error) {
	return []byte(`{"value":` + strconv.Itoa(int(w.Integer)) + `,"unit":"` + w.Unit + `"}`), nil
}

type embedsInteger64 struct {
	rm.Integer64
	Unit string
}

func (w embedsInteger64) MarshalJSON() ([]byte, error) {
	return []byte(`{"value":` + strconv.FormatInt(int64(w.Integer64), 10) + `,"unit":"` + w.Unit + `"}`), nil
}

type embedsReal struct {
	rm.Real
	Unit string
}

func (w embedsReal) MarshalJSON() ([]byte, error) {
	return []byte(`{"value":` + strconv.FormatFloat(float64(w.Real), 'g', -1, 64) + `,"unit":"` + w.Unit + `"}`), nil
}

// TestNumericEmbedKeepsOuterMarshalJSON pins that a struct embedding Integer,
// Integer64 or Real is written by its own MarshalJSON, through json/v2 and
// through encoding/json. Each primitive has MarshalJSON as its only JSON
// encode method, which the outer type's own method shadows.
//
// Can-fail: a MarshalJSONTo on any of the three is promoted to the outer type,
// and json/v2 and encoding/json prefer it to the outer MarshalJSON, so that
// row writes the bare number.
func TestNumericEmbedKeepsOuterMarshalJSON(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		v    any
		want string
	}{
		{name: "Integer", v: embedsInteger{Integer: 7, Unit: "kg"}, want: `{"value":7,"unit":"kg"}`},
		{name: "Integer64", v: embedsInteger64{Integer64: 9000000000, Unit: "s"}, want: `{"value":9000000000,"unit":"s"}`},
		{name: "Real", v: embedsReal{Real: 80.5, Unit: "kg"}, want: `{"value":80.5,"unit":"kg"}`},
	}
	for _, tc := range cases {
		got, err := json.Marshal(tc.v)
		if err != nil || string(got) != tc.want {
			t.Errorf("json.Marshal(%s wrapper) = %s, %v; want %s", tc.name, got, err, tc.want)
		}
		got, err = jsonv1.Marshal(tc.v)
		if err != nil || string(got) != tc.want {
			t.Errorf("encoding/json.Marshal(%s wrapper) = %s, %v; want %s", tc.name, got, err, tc.want)
		}
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

// TestNumericUnmarshalJSONDoesNotAllocate pins the cost of the numeric
// primitives on the canonical-JSON decode path: reading a well-formed bare
// number allocates nothing. A json.Unmarshal of the value would allocate on
// every call.
func TestNumericUnmarshalJSONDoesNotAllocate(t *testing.T) {
	r, n, n64 := rm.Real(80.5), rm.Integer(42), rm.Integer64(-9000000000)
	rLit, nLit, n64Lit := []byte("80.5"), []byte("42"), []byte("-9000000000")
	cases := []struct {
		name string
		call func() error
	}{
		{name: "Real.UnmarshalJSON", call: func() error { return r.UnmarshalJSON(rLit) }},
		{name: "Integer.UnmarshalJSON", call: func() error { return n.UnmarshalJSON(nLit) }},
		{name: "Integer64.UnmarshalJSON", call: func() error { return n64.UnmarshalJSON(n64Lit) }},
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

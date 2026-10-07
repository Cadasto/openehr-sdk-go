package constraints_test

// redacted_test.go: REQ-168 § The redacting carrier. A Redacted holds one
// submitted value. No fmt verb or raw fmt path, no JSON or gob encoder and no
// slog handler writes it; == on it never panics; Reveal returns it unchanged.

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	jsonv1 "encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// methodVerbs are the fmt verbs a caller might print a diagnostic with. Under
// each of them fmt calls the Redacted's own Format method.
var methodVerbs = []string{"%v", "%+v", "%#v", "%s", "%d", "%q", "%x", "%X"}

// allVerbs adds %p, under which fmt prints a value without calling its
// methods.
var allVerbs = slices.Concat(methodVerbs, []string{"%p"})

// callerRecord stands for a caller's own struct that keeps a Violation in an
// unexported field. fmt reads such a field without calling its methods.
type callerRecord struct {
	Path      string
	violation constraints.Violation
}

// callerValue is callerRecord for a bare Redacted.
type callerValue struct {
	Path  string
	value constraints.Redacted
}

// markedViolation returns the violation CString reports for markerString,
// whose Value holds the marker.
func markedViolation(t *testing.T) constraints.Violation {
	t.Helper()
	c := constraints.CString{List: []string{"alpha", "beta"}}
	got := c.Validate(markerString)
	if len(got) != 1 {
		t.Fatalf("CString.Validate(%q) = %d violation(s), want 1", markerString, len(got))
	}
	if reveal := got[0].Value.Reveal(); reveal != markerString {
		t.Fatalf("CString.Validate(%q) Value.Reveal() = %#v, want %q", markerString, reveal, markerString)
	}
	return got[0]
}

// heldMarkers are values a Redacted may hold, each carrying markerString: a
// comparable one the SDK stores, and two that cannot be compared, which a
// caller's own Redact call may store.
func heldMarkers() []struct {
	name  string
	value any
} {
	return []struct {
		name  string
		value any
	}{
		{name: "string", value: markerString},
		{name: "slice", value: []string{markerString}},
		{name: "map", value: map[string]string{markerString: markerString}},
	}
}

// leaksMarker reports whether out contains markerString as written, quoted, or
// hex-encoded in either case.
func leaksMarker(out string) bool {
	h := hex.EncodeToString([]byte(markerString))
	for _, form := range []string{markerString, h, strings.ToUpper(h)} {
		if strings.Contains(out, form) {
			return true
		}
	}
	return false
}

func TestREQ168_RedactedPrintsNoValue(t *testing.T) {
	t.Parallel()
	v := markedViolation(t)
	r := constraints.Redact(markerString)
	subjects := []struct {
		name  string
		value any
	}{
		{name: "Violation", value: v},
		{name: "*Violation", value: &v},
		{name: "[]Violation", value: []constraints.Violation{v}},
		{name: "Redacted", value: r},
		{name: "*Redacted", value: &r},
	}
	for _, s := range subjects {
		for _, verb := range allVerbs {
			t.Run(s.name+" "+verb, func(t *testing.T) {
				t.Parallel()
				out := fmt.Sprintf(verb, s.value)
				if leaksMarker(out) {
					t.Errorf("fmt.Sprintf(%q, %s) = %q, which contains the submitted value", verb, s.name, out)
				}
				// %p prints an address or a bad-verb dump; only the verbs that
				// call Format print the placeholder.
				if slices.Contains(methodVerbs, verb) && !strings.Contains(out, "[redacted]") {
					t.Errorf("fmt.Sprintf(%q, %s) = %q, want it to contain [redacted]", verb, s.name, out)
				}
			})
		}
	}
	if got := r.String(); got != "[redacted]" {
		t.Errorf("Redact(%q).String() = %q, want %q", markerString, got, "[redacted]")
	}
}

// TestREQ168_RawFmtPathsPrintNoValue covers the paths where fmt prints a
// Redacted without calling its methods: %p, and an unexported field of the
// caller's own struct, printed directly and through slog.
func TestREQ168_RawFmtPathsPrintNoValue(t *testing.T) {
	t.Parallel()
	for _, held := range heldMarkers() {
		r := constraints.Redact(held.value)
		v := constraints.Violation{Code: constraints.CodeNotInList, Detail: "value not in allowed list", Value: r}
		record := callerRecord{Path: "/a", violation: v}
		value := callerValue{Path: "/a", value: r}
		subjects := []struct {
			name  string
			value any
		}{
			{name: "Violation", value: v},
			{name: "Redacted", value: r},
			{name: "callerRecord", value: record},
			{name: "*callerRecord", value: &record},
			{name: "callerValue", value: value},
			{name: "*callerValue", value: &value},
		}
		for _, s := range subjects {
			t.Run(held.name+" "+s.name+" fmt", func(t *testing.T) {
				t.Parallel()
				for _, verb := range allVerbs {
					if out := fmt.Sprintf(verb, s.value); leaksMarker(out) {
						t.Errorf("fmt.Sprintf(%q, %s holding a %s) = %q, which contains the submitted value", verb, s.name, held.name, out)
					}
				}
			})
			t.Run(held.name+" "+s.name+" slog", func(t *testing.T) {
				t.Parallel()
				for _, newHandler := range []func(*bytes.Buffer) slog.Handler{
					func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
					func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
				} {
					var buf bytes.Buffer
					slog.New(newHandler(&buf)).LogAttrs(t.Context(), slog.LevelInfo, "validation failed", slog.Any("subject", s.value))
					out := buf.String()
					if leaksMarker(out) {
						t.Errorf("slog wrote %q for a %s holding a %s, which contains the submitted value", out, s.name, held.name)
					}
					if !strings.Contains(out, "subject") {
						t.Errorf("slog wrote %q for a %s holding a %s, want it to log the subject attribute", out, s.name, held.name)
					}
				}
			})
		}
	}
}

func TestREQ168_ZeroRedactedHoldsNoValue(t *testing.T) {
	t.Parallel()
	var zero constraints.Redacted
	if reveal := zero.Reveal(); reveal != nil {
		t.Errorf("Redacted{}.Reveal() = %#v, want nil", reveal)
	}
	if s := zero.String(); s != "" {
		t.Errorf("Redacted{}.String() = %q, want \"\"", s)
	}
	for _, verb := range methodVerbs {
		if out := fmt.Sprintf(verb, zero); out != "" {
			t.Errorf("fmt.Sprintf(%q, Redacted{}) = %q, want \"\"", verb, out)
		}
	}
	if constraints.Redact(nil) != zero {
		t.Errorf("Redact(nil) = %v, want the zero Redacted", constraints.Redact(nil).Reveal())
	}
}

func TestREQ168_RedactRevealReturnsTheValueUnchanged(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value any
	}{
		{name: "string", value: markerString},
		{name: "int", value: markerInt},
		{name: "int32", value: int32(markerInt)},
		{name: "float64", value: markerReal},
		{name: "bool", value: false},
		{name: "CodedTermRef", value: markerSymbol},
		{name: "OrdinalSymbol", value: constraints.OrdinalSymbol{Value: markerOrdinal, Symbol: markerSymbol}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := constraints.Redact(tc.value).Reveal(); got != tc.value {
				t.Errorf("Redact(%#v).Reveal() = %#v (%T), want %#v (%T)", tc.value, got, got, tc.value, tc.value)
			}
			// Two Redacted values that hold equal values compare equal, so two
			// diagnostics about the same input do too.
			first, second := constraints.Redact(tc.value), constraints.Redact(tc.value)
			if first != second {
				t.Errorf("Redact(%#v) == Redact(%#v) = false, want true", tc.value, tc.value)
			}
		})
	}
	// Values of different Go types are not equal, even when they print alike.
	if constraints.Redact(int32(markerInt)) == constraints.Redact(markerInt) {
		t.Errorf("Redact(int32(%d)) == Redact(%d) = true, want false", markerInt, markerInt)
	}
}

// TestREQ168_RedactUncomparableValue covers a value that == cannot compare,
// such as a slice or a map. Reveal returns it, and comparing a Violation that
// holds it never panics.
func TestREQ168_RedactUncomparableValue(t *testing.T) {
	t.Parallel()
	slice := []string{markerString}
	m := map[string]string{markerString: markerString}

	t.Run("slice", func(t *testing.T) {
		t.Parallel()
		got, ok := constraints.Redact(slice).Reveal().([]string)
		if !ok || !slices.Equal(got, slice) || &got[0] != &slice[0] {
			t.Errorf("Redact(%q).Reveal() = %#v, want the same slice back", slice, got)
		}
	})
	t.Run("map", func(t *testing.T) {
		t.Parallel()
		got, ok := constraints.Redact(m).Reveal().(map[string]string)
		// A copy holds equal entries in other storage, so compare where the
		// two maps live.
		if !ok || !maps.Equal(got, m) || reflect.ValueOf(got).UnsafePointer() != reflect.ValueOf(m).UnsafePointer() {
			t.Errorf("Redact(%v).Reveal() = %#v, want the same map back", m, got)
		}
	})
	for _, held := range []struct {
		name  string
		value any
	}{{name: "slice", value: slice}, {name: "map", value: m}} {
		t.Run(held.name+" comparison", func(t *testing.T) {
			t.Parallel()
			v := constraints.Violation{Code: constraints.CodeNotInList, Detail: "value not in allowed list", Value: constraints.Redact(held.value)}
			eq, panicked := equal(v, v)
			if panicked != nil || !eq {
				t.Errorf("a Violation holding a %s == its copy = %v (panic: %v), want true", held.name, eq, panicked)
			}
			// Each Redact call holds its own reference to a value that cannot be
			// compared, so two calls are not equal, and comparing them is safe.
			w := constraints.Violation{Code: v.Code, Detail: v.Detail, Value: constraints.Redact(held.value)}
			eq, panicked = equal(v, w)
			if panicked != nil || eq {
				t.Errorf("two Violations holding separate Redact calls on one %s compare %v (panic: %v), want false", held.name, eq, panicked)
			}
		})
	}
}

// TestREQ168_RevealReturnsNaNAsNaN covers the value Reveal cannot return
// equal under ==: a NaN comes back as a NaN in the same place, alone or
// inside an array or a struct, and every other part comes back equal. The
// [2]float64 rows are the pairs a DV_PROPORTION or DV_INTERVAL issue holds.
func TestREQ168_RevealReturnsNaNAsNaN(t *testing.T) {
	t.Parallel()
	nan := math.NaN()
	t.Run("float64", func(t *testing.T) {
		t.Parallel()
		got := constraints.Redact(nan).Reveal()
		if f, ok := got.(float64); !ok || !math.IsNaN(f) {
			t.Errorf("Redact(NaN).Reveal() = %#v (%T), want a float64 NaN", got, got)
		}
	})
	t.Run("float32", func(t *testing.T) {
		t.Parallel()
		got := constraints.Redact(float32(nan)).Reveal()
		if f, ok := got.(float32); !ok || !math.IsNaN(float64(f)) {
			t.Errorf("Redact(float32(NaN)).Reveal() = %#v (%T), want a float32 NaN", got, got)
		}
	})
	for _, pair := range [][2]float64{{nan, markerReal}, {markerReal, nan}, {nan, nan}} {
		t.Run(fmt.Sprintf("[2]float64%v", pair), func(t *testing.T) {
			t.Parallel()
			got, ok := constraints.Redact(pair).Reveal().([2]float64)
			if !ok {
				t.Fatalf("Redact(%v).Reveal() is not a [2]float64", pair)
			}
			for i := range pair {
				if math.IsNaN(pair[i]) != math.IsNaN(got[i]) || !math.IsNaN(pair[i]) && got[i] != pair[i] {
					t.Errorf("Redact(%v).Reveal()[%d] = %v, want %v", pair, i, got[i], pair[i])
				}
			}
		})
	}
	t.Run("struct", func(t *testing.T) {
		t.Parallel()
		q := constraints.QuantityValue{Magnitude: nan, Units: markerUnits, Precision: markerPrecision}
		got, ok := constraints.Redact(q).Reveal().(constraints.QuantityValue)
		if !ok || !math.IsNaN(got.Magnitude) || got.Units != q.Units || got.Precision != q.Precision {
			t.Errorf("Redact(%+v).Reveal() = %+v, want a NaN Magnitude and the other fields unchanged", q, got)
		}
	})
}

// TestREQ168_RevealIsTheOnlyMethodThatReturnsTheValue lists every method of
// Redacted with the test that shows it does not return the held value, or,
// for Reveal, that it does. A method added to Redacted fails this test until
// it is checked for the held value and listed here.
func TestREQ168_RevealIsTheOnlyMethodThatReturnsTheValue(t *testing.T) {
	t.Parallel()
	checked := map[string]string{
		"Reveal":      "TestREQ168_RedactRevealReturnsTheValueUnchanged: the one method that returns the value",
		"String":      "TestREQ168_RedactedPrintsNoValue",
		"Format":      "TestREQ168_RedactedPrintsNoValue",
		"MarshalJSON": "TestREQ168_JSONLeavesTheValueOut",
		"GobEncode":   "TestREQ168_GobDropsTheValue",
		"GobDecode":   "TestREQ168_GobDropsTheValue",
		"Equal":       "TestREQ168_RedactedEqualAgreesWithEquality: returns a bool",
	}
	// The method set of *Redacted holds the methods of Redacted too.
	rt := reflect.TypeFor[*constraints.Redacted]()
	seen := map[string]bool{}
	for m := range rt.Methods() {
		seen[m.Name] = true
		if _, ok := checked[m.Name]; !ok {
			t.Errorf("Redacted has a method %s that no test checks for the held value; check it, then list it here", m.Name)
		}
	}
	for name := range checked {
		if !seen[name] {
			t.Errorf("Redacted has no method %s, which this test lists; drop it from the list", name)
		}
	}
}

func TestREQ168_JSONLeavesTheValueOut(t *testing.T) {
	t.Parallel()
	v := markedViolation(t)
	vSlice := constraints.Violation{Code: v.Code, Detail: v.Detail, Value: constraints.Redact([]string{markerString})}
	encoders := []struct {
		name    string
		marshal func(any) ([]byte, error)
	}{
		{name: "encoding/json", marshal: jsonv1.Marshal},
		{name: "encoding/json/v2", marshal: func(in any) ([]byte, error) { return jsonv2.Marshal(in) }},
	}
	for _, enc := range encoders {
		t.Run(enc.name+" Violation", func(t *testing.T) {
			t.Parallel()
			for _, in := range []any{v, &v, vSlice} {
				out, err := enc.marshal(in)
				if err != nil {
					t.Fatalf("Marshal(%T) error = %v", in, err)
				}
				if leaksMarker(string(out)) {
					t.Errorf("Marshal(%T) = %s, which contains the submitted value", in, out)
				}
				var members map[string]any
				if err := jsonv1.Unmarshal(out, &members); err != nil {
					t.Fatalf("Marshal(%T) = %s, which does not decode as an object: %v", in, out, err)
				}
				for name := range members {
					if strings.EqualFold(name, "value") {
						t.Errorf("Marshal(%T) = %s, which has a %q member, want none", in, out, name)
					}
				}
				if got := members["Code"]; got != string(constraints.CodeNotInList) {
					t.Errorf("Marshal(%T) member Code = %#v, want %q", in, got, constraints.CodeNotInList)
				}
			}
		})
		t.Run(enc.name+" Redacted", func(t *testing.T) {
			t.Parallel()
			for _, r := range []constraints.Redacted{constraints.Redact(markerString), constraints.Redact([]string{markerString}), {}} {
				out, err := enc.marshal(r)
				if err != nil {
					t.Fatalf("Marshal(Redacted) error = %v", err)
				}
				if string(out) != "null" {
					t.Errorf("Marshal(Redacted) = %s, want null", out)
				}
			}
		})
	}
}

// gobRoundTrip encodes in with encoding/gob and decodes the bytes into a new
// T. It returns the decoded value and the encoded bytes.
func gobRoundTrip[T any](in T) (T, []byte, error) {
	var out T
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(in); err != nil {
		return out, nil, fmt.Errorf("encode %T: %w", in, err)
	}
	wire := slices.Clone(buf.Bytes())
	if err := gob.NewDecoder(&buf).Decode(&out); err != nil {
		return out, wire, fmt.Errorf("decode %T: %w", out, err)
	}
	return out, wire, nil
}

func TestREQ168_GobDropsTheValue(t *testing.T) {
	t.Parallel()
	marked := markedViolation(t)
	withSlice := constraints.Violation{Code: marked.Code, Detail: marked.Detail, Value: constraints.Redact([]string{markerString})}

	for _, tc := range []struct {
		name string
		in   constraints.Violation
	}{{name: "Violation holding a string", in: marked}, {name: "Violation holding a slice", in: withSlice}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, wire, err := gobRoundTrip(tc.in)
			if err != nil {
				t.Fatalf("gob round trip: %v", err)
			}
			if leaksMarker(string(wire)) {
				t.Errorf("gob encoded %s as %q, which contains the submitted value", tc.name, wire)
			}
			if got.Code != tc.in.Code || got.Detail != tc.in.Detail {
				t.Errorf("gob decoded {Code: %q, Detail: %q}, want {Code: %q, Detail: %q}", got.Code, got.Detail, tc.in.Code, tc.in.Detail)
			}
			if got.Value != (constraints.Redacted{}) || got.Value.Reveal() != nil {
				t.Errorf("gob decoded Value.Reveal() = %#v, want the zero Redacted", got.Value.Reveal())
			}
		})
	}

	t.Run("[]Violation", func(t *testing.T) {
		t.Parallel()
		got, wire, err := gobRoundTrip([]constraints.Violation{marked})
		if err != nil {
			t.Fatalf("gob round trip: %v", err)
		}
		if leaksMarker(string(wire)) {
			t.Errorf("gob encoded []Violation as %q, which contains the submitted value", wire)
		}
		if len(got) != 1 || got[0].Code != marked.Code || got[0].Value != (constraints.Redacted{}) {
			t.Errorf("gob decoded %+v, want one Violation with Code %q and an empty Value", got, marked.Code)
		}
	})

	t.Run("Redacted", func(t *testing.T) {
		t.Parallel()
		got, wire, err := gobRoundTrip(constraints.Redact(markerString))
		if err != nil {
			t.Fatalf("gob round trip: %v", err)
		}
		if leaksMarker(string(wire)) {
			t.Errorf("gob encoded Redacted as %q, which contains the submitted value", wire)
		}
		if got != (constraints.Redacted{}) {
			t.Errorf("gob decoded Redacted.Reveal() = %#v, want the zero Redacted", got.Reveal())
		}
	})

	t.Run("Redacted decoded over one that holds a value", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(constraints.Redact(markerString)); err != nil {
			t.Fatalf("gob Encode(Redacted) error = %v", err)
		}
		dst := constraints.Redact("held before decoding")
		if err := gob.NewDecoder(&buf).Decode(&dst); err != nil {
			t.Fatalf("gob Decode(*Redacted) error = %v", err)
		}
		if dst != (constraints.Redacted{}) {
			t.Errorf("gob decoded into a Redacted holding %q left Reveal() = %#v, want the zero Redacted", "held before decoding", dst.Reveal())
		}
	})
}

func TestREQ168_SlogWritesNoValue(t *testing.T) {
	t.Parallel()
	v := markedViolation(t)
	handlers := []struct {
		name    string
		handler func(*bytes.Buffer) slog.Handler
	}{
		{name: "text", handler: func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) }},
		{name: "JSON", handler: func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) }},
	}
	attrs := []struct {
		name string
		attr slog.Attr
		// want is a substring each handler's output must contain, keyed by
		// handler name, so a run that logged nothing does not pass.
		want map[string]string
	}{
		{
			name: "Violation",
			attr: slog.Any("violation", v),
			want: map[string]string{"text": "Value:[redacted]", "JSON": `"Code":"not_in_list"`},
		},
		{
			name: "*Violation",
			attr: slog.Any("violation", &v),
			want: map[string]string{"text": "Value:[redacted]", "JSON": `"Code":"not_in_list"`},
		},
		{
			name: "[]Violation",
			attr: slog.Any("violations", []constraints.Violation{v}),
			want: map[string]string{"text": "Value:[redacted]", "JSON": `"Code":"not_in_list"`},
		},
		{
			name: "Redacted",
			attr: slog.Any("value", v.Value),
			want: map[string]string{"text": "value=[redacted]", "JSON": `"value":null`},
		},
	}
	for _, h := range handlers {
		for _, a := range attrs {
			t.Run(h.name+" "+a.name, func(t *testing.T) {
				t.Parallel()
				var buf bytes.Buffer
				slog.New(h.handler(&buf)).LogAttrs(t.Context(), slog.LevelInfo, "validation failed", a.attr)
				out := buf.String()
				if leaksMarker(out) {
					t.Errorf("slog %s handler wrote %q, which contains the submitted value", h.name, out)
				}
				if want := a.want[h.name]; !strings.Contains(out, want) {
					t.Errorf("slog %s handler wrote %q, want it to contain %q", h.name, out, want)
				}
			})
		}
	}
}

// Redacted has the Equal method that comparison libraries look for when a
// type has unexported fields (REQ-168).
var _ interface {
	Equal(constraints.Redacted) bool
} = constraints.Redacted{}

// TestREQ168_RedactedEqualAgreesWithEquality checks that Redacted.Equal gives
// the same answer as == in both directions, for the zero value, for comparable
// values and for a value that is held by reference.
func TestREQ168_RedactedEqualAgreesWithEquality(t *testing.T) {
	t.Parallel()
	boxed := constraints.Redact([]string{markerString})
	boxedCopy := boxed
	cases := []struct {
		name string
		a, b constraints.Redacted
		want bool
	}{
		{name: "two zero values", a: constraints.Redacted{}, b: constraints.Redacted{}, want: true},
		{name: "a zero value and a held value", a: constraints.Redacted{}, b: constraints.Redact(markerString), want: false},
		{name: "two Redact calls on an equal string", a: constraints.Redact(markerString), b: constraints.Redact(strings.Clone(markerString)), want: true},
		{name: "int32 7 and int 7", a: constraints.Redact(int32(7)), b: constraints.Redact(7), want: false},
		{name: "a boxed slice and its copy", a: boxed, b: boxedCopy, want: true},
		{name: "two Redact calls on equal slices", a: constraints.Redact([]string{markerString}), b: constraints.Redact([]string{markerString}), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.a == tc.b; got != tc.want {
				t.Fatalf("a == b = %v, want %v; the case no longer pins what == does", got, tc.want)
			}
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Errorf("a.Equal(b) = %v, want %v, as a == b", got, tc.want)
			}
			if got := tc.b.Equal(tc.a); got != tc.want {
				t.Errorf("b.Equal(a) = %v, want %v, as b == a", got, tc.want)
			}
		})
	}
}

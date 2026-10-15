package simplified

// REQ-169 — the FLAT and STRUCTURED interval keys. An absent `|*_unbounded`
// follows its bound: false beside a bound, true where the bound is absent or
// null. A side marked `|*_unbounded: false` with no bound is refused in every
// mode where the RM holds the bound by value, and decodes with no bound where
// it holds it as an interface.

import (
	"bytes"
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// intervalSides is what a decoded interval says about its two sides.
type intervalSides struct {
	lowerUnbounded, lowerIncluded, upperUnbounded, upperIncluded bool
	// lowerBound and upperBound report whether the side holds a bound: a
	// non-nil one for an interface bound, one other than the Go zero for a
	// bound held by value.
	lowerBound, upperBound bool
}

// sidesOf reads the sides of an interval of any instantiation.
func sidesOf[T any](iv rm.Interval[T]) intervalSides {
	var zero T
	held := func(b T) bool { return !reflect.DeepEqual(any(b), any(zero)) }
	return intervalSides{
		lowerUnbounded: iv.LowerUnbounded, lowerIncluded: iv.LowerIncluded,
		upperUnbounded: iv.UpperUnbounded, upperIncluded: iv.UpperIncluded,
		lowerBound: held(iv.Lower), upperBound: held(iv.Upper),
	}
}

// intervalPosition is one place the FLAT grammar spells an interval, with
// the keys of a lower and an upper bound and a reader of the decoded sides.
type intervalPosition struct {
	name string
	// base is the interval's FLAT prefix; boundKeys spells one bound under
	// base+"/"+end, and nullKeys the same bound with every value null.
	base      string
	boundKeys func(end string) map[string]any
	nullKeys  func(end string) map[string]any
	// extra are the keys the position needs to exist besides its bounds.
	extra map[string]any
	// byValue is whether the RM holds the bound by value.
	byValue bool
	sides   func(t *testing.T, comp *rm.Composition) intervalSides
}

// intervalPositions are the FLAT interval positions REQ-169 binds: a normal
// range the RM holds by value (DV_COUNT, DV_QUANTITY) and as an interface
// (DV_DATE_TIME), a reference range (always an interface) and a DV_INTERVAL
// leaf (an interface).
func intervalPositions() []intervalPosition {
	count := rmattrCount + "/_normal_range"
	quantity := rmattrElement + "/_normal_range"
	dateTime := rmattrDateTime + "/_normal_range"
	ref := rmattrElement + "/_other_reference_ranges:0"
	quantityBound := func(base string) func(string) map[string]any {
		return func(end string) map[string]any {
			return map[string]any{base + "/" + end + "|magnitude": 2.0, base + "/" + end + "|unit": "unit"}
		}
	}
	quantityNull := func(base string) func(string) map[string]any {
		return func(end string) map[string]any {
			return map[string]any{base + "/" + end + "|magnitude": nil, base + "/" + end + "|unit": nil}
		}
	}
	return []intervalPosition{
		{
			name:      "DV_COUNT normal range, held by value",
			base:      count,
			boundKeys: func(end string) map[string]any { return map[string]any{count + "/" + end: 3} },
			nullKeys:  func(end string) map[string]any { return map[string]any{count + "/" + end: nil} },
			extra:     map[string]any{rmattrCount: 7},
			byValue:   true,
			sides: func(t *testing.T, comp *rm.Composition) intervalSides {
				c := elementValue[rm.DVCount](t, comp)
				if c.NormalRange == nil {
					t.Fatal("normal_range not decoded")
				}
				return sidesOf(c.NormalRange.Interval)
			},
		},
		{
			name:      "DV_QUANTITY normal range, held by value",
			base:      quantity,
			boundKeys: quantityBound(quantity),
			nullKeys:  quantityNull(quantity),
			byValue:   true,
			sides: func(t *testing.T, comp *rm.Composition) intervalSides {
				q := elementValue[rm.DVQuantity](t, comp)
				if q.NormalRange == nil {
					t.Fatal("normal_range not decoded")
				}
				return sidesOf(q.NormalRange.Interval)
			},
		},
		{
			name: "DV_DATE_TIME normal range, held as an interface",
			base: dateTime,
			boundKeys: func(end string) map[string]any {
				return map[string]any{dateTime + "/" + end: "2022-01-12T13:22:34+01:00"}
			},
			nullKeys: func(end string) map[string]any { return map[string]any{dateTime + "/" + end: nil} },
			extra:    map[string]any{rmattrDateTime: "2022-01-12T13:22:34+01:00"},
			sides: func(t *testing.T, comp *rm.Composition) intervalSides {
				d := elementValue[rm.DVDateTime](t, comp)
				if d.NormalRange == nil {
					t.Fatal("normal_range not decoded")
				}
				return sidesOf(d.NormalRange.Interval)
			},
		},
		{
			name:      "reference range, held as an interface",
			base:      ref,
			boundKeys: quantityBound(ref),
			nullKeys:  quantityNull(ref),
			extra: map[string]any{
				ref + "/meaning|code": "260360000", ref + "/meaning|terminology": "SNOMED-CT", ref + "/meaning|value": "very high",
			},
			sides: func(t *testing.T, comp *rm.Composition) intervalSides {
				q := elementValue[rm.DVQuantity](t, comp)
				if len(q.OtherReferenceRanges) != 1 {
					t.Fatalf("other_reference_ranges = %d, want 1", len(q.OtherReferenceRanges))
				}
				return sidesOf(q.OtherReferenceRanges[0].Range.Interval)
			},
		},
		{
			name:      "DV_INTERVAL leaf, held as an interface",
			base:      rmattrInterval,
			boundKeys: quantityBound(rmattrInterval),
			nullKeys:  quantityNull(rmattrInterval),
			extra:     map[string]any{rmattrIntervalEvent + "/time": "2022-01-12T09:00:11.7842493+01:00"},
			sides: func(t *testing.T, comp *rm.Composition) intervalSides {
				return sidesOf(intervalLeafOf(t, comp).Interval)
			},
		},
	}
}

// body is the position's FLAT body with the given keys.
func (p intervalPosition) body(keys ...map[string]any) map[string]any {
	b := rmattrBody(p.extra)
	for _, k := range keys {
		maps.Copy(b, k)
	}
	return b
}

// structuredForm is the STRUCTURED form of a FLAT body. A null bare bound,
// which FlatToStructured writes as a null array element, is spelled as the
// null `"|"` member instead, the other STRUCTURED spelling of a null bare
// value; the null array element is pinned by
// TestREQ053NullStructuredMemberAndElementAreAbsent.
func structuredForm(t *testing.T, body map[string]any) []byte {
	t.Helper()
	s, err := FlatToStructured(mustJSON(t, body))
	if err != nil {
		t.Fatalf("FlatToStructured = %v", err)
	}
	return bytes.ReplaceAll(s, []byte(":[null]"), []byte(`:[{"|":null}]`))
}

// decodeBothForms decodes a FLAT body and its STRUCTURED form under strict,
// failing the test on an error, and returns the two compositions.
func decodeBothForms(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any) map[string]*rm.Composition {
	t.Helper()
	flat, err := UnmarshalFlat(mustJSON(t, body), wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat = %v", err)
	}
	structured, err := UnmarshalStructured(structuredForm(t, body), wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalStructured = %v", err)
	}
	return map[string]*rm.Composition{"FLAT": flat, "STRUCTURED": structured}
}

// TestREQ169FlatUnboundedFollowsItsBound — REQ-169: "an absent `*_unbounded`
// as `false` when that side carries a bound, and as `true` when its bound is
// absent or `null` too". At every FLAT interval position, by value or as an
// interface, in FLAT and in STRUCTURED: a side with no bound key, or with
// only null ones, and no `|*_unbounded` is open and not included, the other
// side keeping its bound; a side with a bound and no flag is closed.
func TestREQ169FlatUnboundedFollowsItsBound(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, p := range intervalPositions() {
		for _, tc := range []struct {
			name string
			keys map[string]any
			want intervalSides
		}{
			{"only an upper bound", p.boundKeys("upper"), intervalSides{lowerUnbounded: true, upperIncluded: true, upperBound: true}},
			{"only a lower bound", p.boundKeys("lower"), intervalSides{upperUnbounded: true, lowerIncluded: true, lowerBound: true}},
			{"a null lower bound", mergeKeys(p.nullKeys("lower"), p.boundKeys("upper")), intervalSides{lowerUnbounded: true, upperIncluded: true, upperBound: true}},
			{"both bounds", mergeKeys(p.boundKeys("lower"), p.boundKeys("upper")), intervalSides{lowerIncluded: true, upperIncluded: true, lowerBound: true, upperBound: true}},
		} {
			t.Run(p.name+"/"+tc.name, func(t *testing.T) {
				for form, comp := range decodeBothForms(t, wt, p.body(tc.keys)) {
					if got := p.sides(t, comp); got != tc.want {
						t.Errorf("%s sides = %+v, want %+v", form, got, tc.want)
					}
				}
			})
		}
	}
}

// TestREQ169FlatClosedSideWithNoBound — REQ-169: "A side marked
// `*_unbounded: false` with an absent or `null` bound is refused in every
// reader and mode where the bound is held by value …, and decodes with no
// bound where it is held by pointer or as an interface". Where the RM holds
// the normal range's bound by value (DV_COUNT, DV_QUANTITY) such a side is
// refused in every mode with ErrUnsupportedDatatype, in FLAT and STRUCTURED;
// held as an interface it decodes as written, closed, included and with no
// bound.
func TestREQ169FlatClosedSideWithNoBound(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, p := range intervalPositions() {
		closed := map[string]any{p.base + "|lower_unbounded": false}
		for _, tc := range []struct {
			name string
			keys map[string]any
		}{
			{"absent lower bound", mergeKeys(closed, p.boundKeys("upper"))},
			{"null lower bound", mergeKeys(closed, p.nullKeys("lower"), p.boundKeys("upper"))},
		} {
			t.Run(p.name+"/"+tc.name, func(t *testing.T) {
				body := p.body(tc.keys)
				if !p.byValue {
					want := intervalSides{lowerIncluded: true, upperIncluded: true, upperBound: true}
					for form, comp := range decodeBothForms(t, wt, body) {
						if got := p.sides(t, comp); got != want {
							t.Errorf("%s sides = %+v, want %+v", form, got, want)
						}
					}
					return
				}
				for form, err := range map[string]error{
					"FLAT":       refusedInEveryMode(t, wt, body),
					"STRUCTURED": structuredDocRefusedInEveryMode(t, wt, structuredForm(t, body)),
				} {
					if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), "lower_unbounded") {
						t.Errorf("%s err = %v, want ErrUnsupportedDatatype naming |lower_unbounded", form, err)
					}
				}
			})
		}
	}
}

// TestREQ169NormalRangeBoundByValueMatchesRM — REQ-169. The decode refuses a
// closed side with no bound where the RM holds the normal range's bound by
// value, and reads which datatypes those are off a table, since the codec
// takes no reflection (REQ-024). The table must name exactly the datatypes
// whose normal_range bound the generated RM types hold as a concrete type.
func TestREQ169NormalRangeBoundByValueMatchesRM(t *testing.T) {
	for name, v := range map[string]any{
		"DV_COUNT": rm.DVCount{}, "DV_QUANTITY": rm.DVQuantity{}, "DV_PROPORTION": rm.DVProportion{},
		"DV_ORDINAL": rm.DVOrdinal{}, "DV_SCALE": rm.DVScale{}, "DV_DURATION": rm.DVDuration{},
		"DV_DATE": rm.DVDate{}, "DV_DATE_TIME": rm.DVDateTime{}, "DV_TIME": rm.DVTime{},
	} {
		field, ok := reflect.TypeOf(v).FieldByName("NormalRange")
		if !ok {
			t.Fatalf("%s has no NormalRange field", name)
		}
		lower, ok := field.Type.Elem().FieldByName("Lower")
		if !ok {
			t.Fatalf("%s.NormalRange has no Lower bound", name)
		}
		if byValue := lower.Type.Kind() != reflect.Interface; normalRangeBoundByValue[name] != byValue {
			t.Errorf("normalRangeBoundByValue[%s] = %v, but the RM holds the bound by value: %v", name, normalRangeBoundByValue[name], byValue)
		}
	}
}

// mergeKeys returns the union of key sets.
func mergeKeys(sets ...map[string]any) map[string]any {
	out := make(map[string]any)
	for _, s := range sets {
		maps.Copy(out, s)
	}
	return out
}

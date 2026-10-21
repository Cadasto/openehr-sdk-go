package rmwrite

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_EnsureSingleWritesOrderedValueParts checks that EnsureSingle
// writes the attributes of a DV_ORDINAL, a DV_SCALE, a DV_PROPORTION and a
// REFERENCE_RANGE the instance generator reaches: an ordinal's or a
// scale's value and symbol; a proportion's numerator, denominator, type,
// precision, accuracy, accuracy_is_percent and magnitude_status; and a
// reference range's meaning, a DV_TEXT or a DV_CODED_TEXT. A coded text is
// taken as a value or as a pointer.
func TestREQ107_EnsureSingleWritesOrderedValueParts(t *testing.T) {
	t.Parallel()
	symbol := rm.DVCodedText{Value: "mild", DefiningCode: rm.CodePhrase{CodeString: "at0001", TerminologyID: rm.TerminologyID{Value: "local"}}}
	ordinal, scale, proportion := &rm.DVOrdinal{}, &rm.DVScale{}, &rm.DVProportion{}
	textRange, codedRange := &rm.ReferenceRange[rm.DVOrdered]{}, &rm.ReferenceRange[rm.DVOrdered]{}
	cases := []valueWrite{
		{"DV_ORDINAL value", ordinal, "value", int64(3), func() any { return ordinal.Value }, rm.Integer(3)},
		{"DV_ORDINAL symbol", ordinal, "symbol", &symbol, func() any { return ordinal.Symbol.DefiningCode }, symbol.DefiningCode},
		{"DV_SCALE value", scale, "value", 1.5, func() any { return scale.Value }, rm.Real(1.5)},
		{"DV_SCALE symbol", scale, "symbol", symbol, func() any { return scale.Symbol.DefiningCode }, symbol.DefiningCode},
		{"DV_PROPORTION numerator", proportion, "numerator", 3.0, func() any { return proportion.Numerator }, rm.Real(3)},
		{"DV_PROPORTION denominator", proportion, "denominator", int64(4), func() any { return proportion.Denominator }, rm.Real(4)},
		{"DV_PROPORTION type", proportion, "type", int64(2), func() any { return proportion.Type }, rm.Integer(2)},
		{"DV_PROPORTION precision", proportion, "precision", int64(-1), func() any { return *proportion.Precision }, rm.Integer(-1)},
		{"DV_PROPORTION accuracy", proportion, "accuracy", 2.5, func() any { return *proportion.Accuracy }, rm.Real(2.5)},
		{"DV_PROPORTION accuracy_is_percent", proportion, "accuracy_is_percent", true, func() any { return *proportion.AccuracyIsPercent }, true},
		{"DV_PROPORTION magnitude_status", proportion, "magnitude_status", "~", func() any { return *proportion.MagnitudeStatus }, "~"},
		{"REFERENCE_RANGE meaning DV_TEXT", textRange, "meaning", &rm.DVText{Value: "critical"}, func() any { return textRange.Meaning.GetValue() }, "critical"},
		{"REFERENCE_RANGE meaning DV_CODED_TEXT", codedRange, "meaning", symbol, func() any { return codedRange.Meaning.GetValue() }, "mild"},
	}
	for _, tc := range cases {
		if err := EnsureSingle(tc.parent, tc.attr, tc.child); err != nil {
			t.Errorf("%s: EnsureSingle(%T, %q, %T) = %v, want nil", tc.name, tc.parent, tc.attr, tc.child, err)
			continue
		}
		if got := tc.got(); got != tc.want {
			t.Errorf("%s: %T.%s = %#v, want %#v", tc.name, tc.parent, tc.attr, got, tc.want)
		}
	}
	if _, coded := codedRange.Meaning.GetDefiningCode(); !coded {
		t.Errorf("REFERENCE_RANGE.meaning = %T, want the coded text kept coded", codedRange.Meaning)
	}
}

// orderedParent is one DV_ORDERED class the writers write the DV_ORDERED
// attributes of, and how to read them back.
type orderedParent struct {
	name string
	// fresh builds an empty value of the class.
	fresh func() any
	// bound is a bound of the class's own type, by value.
	bound rm.DVOrdered
	// orderedField is whether the class holds normal_range over
	// DV_ORDERED rather than over itself.
	orderedField bool
	// status reads normal_status.
	status func(v any) *rm.CodePhrase
	// normalRange reads normal_range as its four flags and its two bounds,
	// nil for a bound a DV_ORDERED field does not hold and the zero value
	// of the class for one a typed field does not.
	normalRange func(v any) (parts fieldRange, set bool)
	// references reads other_reference_ranges as meaning and range pairs.
	references func(v any) []rm.ReferenceRange[rm.DVOrdered]
}

// fieldRange is a DV_INTERVAL field read back as its four flags and its
// two bounds, each bound as the field holds it.
type fieldRange struct {
	lower, upper                   any
	lowerUnbounded, upperUnbounded bool
	lowerIncluded, upperIncluded   bool
}

// partsOfField reads a DV_INTERVAL field back as its parts, keeping each
// bound as the field holds it.
func partsOfField[T rm.DVOrdered](iv *rm.DVInterval[T]) (fieldRange, bool) {
	if iv == nil {
		return fieldRange{}, false
	}
	return fieldRange{
		lower: any(iv.Lower), upper: any(iv.Upper),
		lowerUnbounded: iv.LowerUnbounded, upperUnbounded: iv.UpperUnbounded,
		lowerIncluded: iv.LowerIncluded, upperIncluded: iv.UpperIncluded,
	}, true
}

// referencesOf copies a field's reference ranges into the DV_ORDERED
// instantiation, which holds the same two attributes.
func referencesOf[T rm.DVOrdered](list []rm.ReferenceRange[T]) []rm.ReferenceRange[rm.DVOrdered] {
	out := make([]rm.ReferenceRange[rm.DVOrdered], 0, len(list))
	for _, r := range list {
		out = append(out, rm.ReferenceRange[rm.DVOrdered](r))
	}
	return out
}

// orderedParents are the nine DV_ORDERED classes the type registry builds.
func orderedParents() []orderedParent {
	return []orderedParent{
		{
			name: "DV_COUNT", fresh: func() any { return &rm.DVCount{} }, bound: rm.DVCount{Magnitude: 7},
			status:      func(v any) *rm.CodePhrase { return v.(*rm.DVCount).NormalStatus },
			normalRange: func(v any) (fieldRange, bool) { return partsOfField(v.(*rm.DVCount).NormalRange) },
			references: func(v any) []rm.ReferenceRange[rm.DVOrdered] {
				return referencesOf(v.(*rm.DVCount).OtherReferenceRanges)
			},
		},
		{
			name: "DV_QUANTITY", fresh: func() any { return &rm.DVQuantity{} }, bound: rm.DVQuantity{Magnitude: 7, Units: "mm"},
			status:      func(v any) *rm.CodePhrase { return v.(*rm.DVQuantity).NormalStatus },
			normalRange: func(v any) (fieldRange, bool) { return partsOfField(v.(*rm.DVQuantity).NormalRange) },
			references: func(v any) []rm.ReferenceRange[rm.DVOrdered] {
				return referencesOf(v.(*rm.DVQuantity).OtherReferenceRanges)
			},
		},
		{
			name: "DV_PROPORTION", fresh: func() any { return &rm.DVProportion{} }, bound: rm.DVProportion{Numerator: 7, Denominator: 1, Type: 1},
			status:      func(v any) *rm.CodePhrase { return v.(*rm.DVProportion).NormalStatus },
			normalRange: func(v any) (fieldRange, bool) { return partsOfField(v.(*rm.DVProportion).NormalRange) },
			references: func(v any) []rm.ReferenceRange[rm.DVOrdered] {
				return referencesOf(v.(*rm.DVProportion).OtherReferenceRanges)
			},
		},
		{
			name: "DV_ORDINAL", orderedField: true, fresh: func() any { return &rm.DVOrdinal{} }, bound: rm.DVOrdinal{Value: 7},
			status:      func(v any) *rm.CodePhrase { return v.(*rm.DVOrdinal).NormalStatus },
			normalRange: func(v any) (fieldRange, bool) { return partsOfField(v.(*rm.DVOrdinal).NormalRange) },
			references: func(v any) []rm.ReferenceRange[rm.DVOrdered] {
				return referencesOf(v.(*rm.DVOrdinal).OtherReferenceRanges)
			},
		},
		{
			name: "DV_SCALE", orderedField: true, fresh: func() any { return &rm.DVScale{} }, bound: rm.DVScale{Value: 7.5},
			status:      func(v any) *rm.CodePhrase { return v.(*rm.DVScale).NormalStatus },
			normalRange: func(v any) (fieldRange, bool) { return partsOfField(v.(*rm.DVScale).NormalRange) },
			references: func(v any) []rm.ReferenceRange[rm.DVOrdered] {
				return referencesOf(v.(*rm.DVScale).OtherReferenceRanges)
			},
		},
		{
			name: "DV_DATE", orderedField: true, fresh: func() any { return &rm.DVDate{} }, bound: rm.DVDate{Value: "2020-01-07"},
			status:      func(v any) *rm.CodePhrase { return v.(*rm.DVDate).NormalStatus },
			normalRange: func(v any) (fieldRange, bool) { return partsOfField(v.(*rm.DVDate).NormalRange) },
			references: func(v any) []rm.ReferenceRange[rm.DVOrdered] {
				return referencesOf(v.(*rm.DVDate).OtherReferenceRanges)
			},
		},
		{
			name: "DV_TIME", orderedField: true, fresh: func() any { return &rm.DVTime{} }, bound: rm.DVTime{Value: "07:00:00"},
			status:      func(v any) *rm.CodePhrase { return v.(*rm.DVTime).NormalStatus },
			normalRange: func(v any) (fieldRange, bool) { return partsOfField(v.(*rm.DVTime).NormalRange) },
			references: func(v any) []rm.ReferenceRange[rm.DVOrdered] {
				return referencesOf(v.(*rm.DVTime).OtherReferenceRanges)
			},
		},
		{
			name: "DV_DATE_TIME", orderedField: true, fresh: func() any { return &rm.DVDateTime{} }, bound: rm.DVDateTime{Value: "2020-01-07T07:00:00Z"},
			status:      func(v any) *rm.CodePhrase { return v.(*rm.DVDateTime).NormalStatus },
			normalRange: func(v any) (fieldRange, bool) { return partsOfField(v.(*rm.DVDateTime).NormalRange) },
			references: func(v any) []rm.ReferenceRange[rm.DVOrdered] {
				return referencesOf(v.(*rm.DVDateTime).OtherReferenceRanges)
			},
		},
		{
			name: "DV_DURATION", orderedField: true, fresh: func() any { return &rm.DVDuration{} }, bound: rm.DVDuration{Value: "P7D"},
			status:      func(v any) *rm.CodePhrase { return v.(*rm.DVDuration).NormalStatus },
			normalRange: func(v any) (fieldRange, bool) { return partsOfField(v.(*rm.DVDuration).NormalRange) },
			references: func(v any) []rm.ReferenceRange[rm.DVOrdered] {
				return referencesOf(v.(*rm.DVDuration).OtherReferenceRanges)
			},
		},
	}
}

// typedInterval builds a DV_INTERVAL over the concrete type of bound, closed
// below at bound and open above, one case per class.
func typedInterval(bound rm.DVOrdered) any {
	switch b := bound.(type) {
	case rm.DVCount:
		return lowerOnly(b)
	case rm.DVQuantity:
		return lowerOnly(b)
	case rm.DVProportion:
		return lowerOnly(b)
	case rm.DVOrdinal:
		return lowerOnly(b)
	case rm.DVScale:
		return lowerOnly(b)
	case rm.DVDate:
		return lowerOnly(b)
	case rm.DVTime:
		return lowerOnly(b)
	case rm.DVDateTime:
		return lowerOnly(b)
	case rm.DVDuration:
		return lowerOnly(b)
	}
	panic(fmt.Sprintf("typedInterval: no case for %T", bound))
}

// lowerOnly is a DV_INTERVAL over T closed and included below at lower and
// open above, holding a non-zero upper bound the open side must not keep.
func lowerOnly[T rm.DVOrdered](lower T) *rm.DVInterval[T] {
	iv := &rm.DVInterval[T]{}
	iv.Lower, iv.Upper = lower, lower
	iv.LowerIncluded, iv.UpperUnbounded = true, true
	return iv
}

// boundValue returns a bound as the value it holds, dereferencing a
// pointer, and whether it was held by pointer.
func boundValue(b any) (value any, byPointer bool) {
	switch v := b.(type) {
	case *rm.DVCount:
		return *v, true
	case *rm.DVQuantity:
		return *v, true
	case *rm.DVProportion:
		return *v, true
	case *rm.DVOrdinal:
		return *v, true
	case *rm.DVScale:
		return *v, true
	case *rm.DVDate:
		return *v, true
	case *rm.DVTime:
		return *v, true
	case *rm.DVDateTime:
		return *v, true
	case *rm.DVDuration:
		return *v, true
	}
	return b, false
}

// sameBound reports whether got holds the bound want, by value or by
// pointer. A bound that is no comparable struct is compared by its text.
func sameBound(got any, want rm.DVOrdered) bool {
	v, _ := boundValue(got)
	return fmt.Sprintf("%+v", v) == fmt.Sprintf("%+v", want)
}

// TestREQ107_OrderedAttributesAreWritten checks that the writers write the
// three attributes every DV_ORDERED class declares, on each of the nine
// classes the type registry builds: normal_status takes a CODE_PHRASE by
// value or by pointer; normal_range takes a DV_INTERVAL of the class's own
// instantiation, or of DV_ORDERED, by value or by pointer, and converts it
// into the instantiation the field holds, the flags copied and the open
// side holding no bound (nil over DV_ORDERED, the zero value over a
// concrete class), a bound over DV_ORDERED held by pointer; and
// AppendMultiple appends to other_reference_ranges a REFERENCE_RANGE of
// DV_ORDERED, the instantiation the type registry builds, by value or by
// pointer, keeping its meaning and its range.
func TestREQ107_OrderedAttributesAreWritten(t *testing.T) {
	t.Parallel()
	normal := rm.CodePhrase{CodeString: "N", TerminologyID: rm.TerminologyID{Value: "openehr_normal_statuses"}}
	for _, p := range orderedParents() {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			for _, child := range []any{normal, &normal} {
				v := p.fresh()
				if err := EnsureSingle(v, "normal_status", child); err != nil {
					t.Fatalf("EnsureSingle(%s, normal_status, %T) = %v, want nil", p.name, child, err)
				}
				if got := p.status(v); got == nil || *got != normal {
					t.Errorf("%s.normal_status after %T = %+v, want %+v", p.name, child, got, normal)
				}
			}

			bare := &rm.DVInterval[rm.DVOrdered]{}
			bare.Lower = p.bound
			bare.LowerIncluded, bare.UpperUnbounded = true, true
			barePointer := &rm.DVInterval[rm.DVOrdered]{}
			barePointer.Lower = pointerTo(p.bound)
			barePointer.LowerIncluded, barePointer.UpperUnbounded = true, true
			sources := []struct {
				name  string
				child any
				// byPointer is whether a DV_ORDERED field holds the lower
				// bound by pointer: a bound of a concrete instantiation is
				// copied to one, a bound over DV_ORDERED is kept as held.
				byPointer bool
			}{
				{"its own instantiation by pointer", typedInterval(p.bound), true},
				{"DV_ORDERED by pointer, holding a value", bare, false},
				{"DV_ORDERED by value, holding a value", *bare, false},
				{"DV_ORDERED by pointer, holding a pointer", barePointer, true},
			}
			for _, src := range sources {
				v := p.fresh()
				if err := EnsureSingle(v, "normal_range", src.child); err != nil {
					t.Errorf("%s.normal_range from %s: EnsureSingle = %v, want nil", p.name, src.name, err)
					continue
				}
				got, set := p.normalRange(v)
				if !set {
					t.Errorf("%s.normal_range from %s is unset", p.name, src.name)
					continue
				}
				if !got.lowerIncluded || got.upperIncluded || got.lowerUnbounded || !got.upperUnbounded {
					t.Errorf("%s.normal_range from %s: flags %+v, want lower closed and included, upper open", p.name, src.name, got)
				}
				if !sameBound(got.lower, p.bound) {
					t.Errorf("%s.normal_range from %s: lower = %#v, want %#v", p.name, src.name, got.lower, p.bound)
				}
				_, byPointer := boundValue(got.lower)
				switch {
				case !p.orderedField:
					if byPointer {
						t.Errorf("%s.normal_range from %s: lower %T, want the value a typed field holds", p.name, src.name, got.lower)
					}
					if !sameBound(got.upper, zeroOf(p.bound)) {
						t.Errorf("%s.normal_range from %s: the open upper side holds %#v, want the zero value", p.name, src.name, got.upper)
					}
				case byPointer != src.byPointer:
					t.Errorf("%s.normal_range from %s: lower %T, by pointer %t, want %t", p.name, src.name, got.lower, byPointer, src.byPointer)
				case got.upper != nil:
					t.Errorf("%s.normal_range from %s: the open upper side holds %#v, want no bound", p.name, src.name, got.upper)
				}
			}

			// The converted interval shares no bound with the child.
			typed := typedInterval(p.bound)
			v := p.fresh()
			if err := EnsureSingle(v, "normal_range", typed); err != nil {
				t.Fatalf("%s.normal_range: EnsureSingle = %v, want nil", p.name, err)
			}
			clearLower(typed)
			if got, _ := p.normalRange(v); !sameBound(got.lower, p.bound) {
				t.Errorf("%s.normal_range: lower = %#v after the child changed, want %#v kept", p.name, got.lower, p.bound)
			}

			meaning := &rm.DVText{Value: "critical"}
			member := rm.ReferenceRange[rm.DVOrdered]{Meaning: meaning, Range: *bare}
			v = p.fresh()
			for _, child := range []any{member, &member} {
				if err := AppendMultiple(v, "other_reference_ranges", child); err != nil {
					t.Fatalf("AppendMultiple(%s, other_reference_ranges, %T) = %v, want nil", p.name, child, err)
				}
			}
			refs := p.references(v)
			if len(refs) != 2 {
				t.Fatalf("%s.other_reference_ranges has %d members, want 2", p.name, len(refs))
			}
			for i, r := range refs {
				if r.Meaning != rm.DVTextLike(meaning) || !sameBound(r.Range.Lower, p.bound) || !r.Range.UpperUnbounded || r.Range.Upper != nil {
					t.Errorf("%s.other_reference_ranges[%d] = %+v, want the member appended", p.name, i, r)
				}
			}
		})
	}
}

// pointerTo returns a pointer to a copy of b.
func pointerTo(b rm.DVOrdered) rm.DVOrdered {
	switch v := b.(type) {
	case rm.DVCount:
		return &v
	case rm.DVQuantity:
		return &v
	case rm.DVProportion:
		return &v
	case rm.DVOrdinal:
		return &v
	case rm.DVScale:
		return &v
	case rm.DVDate:
		return &v
	case rm.DVTime:
		return &v
	case rm.DVDateTime:
		return &v
	case rm.DVDuration:
		return &v
	}
	return nil
}

// clearLower sets the lower bound of iv, a typed interval typedInterval
// built, to the zero value of its class.
func clearLower(iv any) {
	switch v := iv.(type) {
	case *rm.DVInterval[rm.DVCount]:
		v.Lower = rm.DVCount{}
	case *rm.DVInterval[rm.DVQuantity]:
		v.Lower = rm.DVQuantity{}
	case *rm.DVInterval[rm.DVProportion]:
		v.Lower = rm.DVProportion{}
	case *rm.DVInterval[rm.DVOrdinal]:
		v.Lower = rm.DVOrdinal{}
	case *rm.DVInterval[rm.DVScale]:
		v.Lower = rm.DVScale{}
	case *rm.DVInterval[rm.DVDate]:
		v.Lower = rm.DVDate{}
	case *rm.DVInterval[rm.DVTime]:
		v.Lower = rm.DVTime{}
	case *rm.DVInterval[rm.DVDateTime]:
		v.Lower = rm.DVDateTime{}
	case *rm.DVInterval[rm.DVDuration]:
		v.Lower = rm.DVDuration{}
	}
}

// zeroOf returns the zero value of b's class, the bound a typed field
// keeps on an open side.
func zeroOf(b rm.DVOrdered) rm.DVOrdered {
	switch b.(type) {
	case rm.DVCount:
		return rm.DVCount{}
	case rm.DVQuantity:
		return rm.DVQuantity{}
	case rm.DVProportion:
		return rm.DVProportion{}
	case rm.DVOrdinal:
		return rm.DVOrdinal{}
	case rm.DVScale:
		return rm.DVScale{}
	case rm.DVDate:
		return rm.DVDate{}
	case rm.DVTime:
		return rm.DVTime{}
	case rm.DVDateTime:
		return rm.DVDateTime{}
	case rm.DVDuration:
		return rm.DVDuration{}
	}
	return nil
}

// TestREQ107_OrderedAttributesTakeAnyInstantiation checks that a
// REFERENCE_RANGE's range and an other_reference_ranges member are taken
// from any instantiation: a range of a DV_INTERVAL<DV_QUANTITY> is held
// over DV_ORDERED, its bound by pointer, and a REFERENCE_RANGE of
// DV_QUANTITY appended to a DV_ORDINAL keeps its meaning and its range.
func TestREQ107_OrderedAttributesTakeAnyInstantiation(t *testing.T) {
	t.Parallel()
	bound := rm.DVQuantity{Magnitude: 3, Units: "mm"}
	r := &rm.ReferenceRange[rm.DVOrdered]{}
	if err := EnsureSingle(r, "range", lowerOnly(bound)); err != nil {
		t.Fatalf("EnsureSingle(REFERENCE_RANGE, range, *DVInterval[DVQuantity]) = %v, want nil", err)
	}
	if _, ok := r.Range.Lower.(*rm.DVQuantity); !ok || !sameBound(r.Range.Lower, bound) || r.Range.Upper != nil || !r.Range.UpperUnbounded {
		t.Errorf("REFERENCE_RANGE.range = %+v, want lower *%+v and an open upper side with no bound", r.Range, bound)
	}

	member := rm.ReferenceRange[rm.DVQuantity]{Meaning: &rm.DVText{Value: "high"}, Range: r.Range}
	o := &rm.DVOrdinal{}
	if err := AppendMultiple(o, "other_reference_ranges", &member); err != nil {
		t.Fatalf("AppendMultiple(DV_ORDINAL, other_reference_ranges, *ReferenceRange[DVQuantity]) = %v, want nil", err)
	}
	if len(o.OtherReferenceRanges) != 1 || o.OtherReferenceRanges[0].Meaning.GetValue() != "high" || o.OtherReferenceRanges[0].Range.Lower != r.Range.Lower {
		t.Errorf("DV_ORDINAL.other_reference_ranges = %+v, want the member appended", o.OtherReferenceRanges)
	}
}

// TestREQ107_OrderedWritersRefuseWrongChildren checks that the ordered
// value writers refuse a child of the wrong Go type with ErrTypeMismatch
// and leave the field as it was: a normal_range that is no DV_INTERVAL, or
// whose bound is of another class than the field holds; a nil or a
// string normal_status; a whole number past the 32-bit range of an RM
// Integer; a REFERENCE_RANGE part, or an other_reference_ranges member,
// of the wrong type, or a nil one. An attribute the class does not
// declare stays ErrUnknownAttribute.
func TestREQ107_OrderedWritersRefuseWrongChildren(t *testing.T) {
	t.Parallel()
	otherBound := &rm.DVInterval[rm.DVOrdered]{}
	otherBound.Lower = &rm.DVQuantity{Magnitude: 1, Units: "mm"}
	otherBound.UpperUnbounded = true
	refused := []struct {
		parent any
		attr   string
		child  any
		// kept reports whether the attribute still holds what it held.
		kept func(parent any) bool
	}{
		{&rm.DVCount{}, "normal_range", "not an interval", func(p any) bool { return p.(*rm.DVCount).NormalRange == nil }},
		{&rm.DVCount{}, "normal_range", lowerOnly(rm.DVQuantity{Magnitude: 1}), func(p any) bool { return p.(*rm.DVCount).NormalRange == nil }},
		{&rm.DVCount{}, "normal_range", otherBound, func(p any) bool { return p.(*rm.DVCount).NormalRange == nil }},
		{&rm.DVCount{}, "normal_range", (*rm.DVInterval[rm.DVCount])(nil), func(p any) bool { return p.(*rm.DVCount).NormalRange == nil }},
		{&rm.DVOrdinal{}, "normal_range", rm.DVOrdinal{}, func(p any) bool { return p.(*rm.DVOrdinal).NormalRange == nil }},
		{&rm.DVQuantity{}, "normal_status", (*rm.CodePhrase)(nil), func(p any) bool { return p.(*rm.DVQuantity).NormalStatus == nil }},
		{&rm.DVScale{}, "normal_status", "N", func(p any) bool { return p.(*rm.DVScale).NormalStatus == nil }},
		{&rm.DVOrdinal{Value: 2}, "value", int64(1) << 40, func(p any) bool { return p.(*rm.DVOrdinal).Value == 2 }},
		{&rm.DVOrdinal{}, "value", 1.5, func(p any) bool { return p.(*rm.DVOrdinal).Value == 0 }},
		{&rm.DVOrdinal{}, "symbol", &rm.DVText{Value: "x"}, func(p any) bool { return p.(*rm.DVOrdinal).Symbol.Value == "" }},
		{&rm.DVScale{Value: 2}, "value", "x", func(p any) bool { return p.(*rm.DVScale).Value == 2 }},
		{&rm.DVProportion{Type: 1}, "type", int64(1) << 40, func(p any) bool { return p.(*rm.DVProportion).Type == 1 }},
		{&rm.DVProportion{Numerator: 2}, "numerator", "x", func(p any) bool { return p.(*rm.DVProportion).Numerator == 2 }},
		{&rm.DVProportion{}, "precision", int64(1) << 40, func(p any) bool { return p.(*rm.DVProportion).Precision == nil }},
		{&rm.DVProportion{}, "accuracy_is_percent", 1, func(p any) bool { return p.(*rm.DVProportion).AccuracyIsPercent == nil }},
		{&rm.ReferenceRange[rm.DVOrdered]{}, "meaning", 42, func(p any) bool { return p.(*rm.ReferenceRange[rm.DVOrdered]).Meaning == nil }},
		{&rm.ReferenceRange[rm.DVOrdered]{}, "range", "x", func(p any) bool { return !p.(*rm.ReferenceRange[rm.DVOrdered]).Range.LowerUnbounded }},
	}
	for _, tc := range refused {
		if err := EnsureSingle(tc.parent, tc.attr, tc.child); !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("EnsureSingle(%T, %q, %#v) = %v, want ErrTypeMismatch", tc.parent, tc.attr, tc.child, err)
		}
		if !tc.kept(tc.parent) {
			t.Errorf("EnsureSingle(%T, %q, %#v) changed the attribute, want it kept", tc.parent, tc.attr, tc.child)
		}
	}
	for _, child := range []any{lowerOnly(rm.DVCount{}), (*rm.ReferenceRange[rm.DVOrdered])(nil), "x"} {
		c := &rm.DVCount{}
		if err := AppendMultiple(c, "other_reference_ranges", child); !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("AppendMultiple(DV_COUNT, other_reference_ranges, %T) = %v, want ErrTypeMismatch", child, err)
		}
		if len(c.OtherReferenceRanges) != 0 {
			t.Errorf("AppendMultiple(DV_COUNT, other_reference_ranges, %T) appended %+v, want nothing", child, c.OtherReferenceRanges)
		}
	}
	for _, parent := range []any{&rm.DVOrdinal{}, &rm.DVScale{}, &rm.DVProportion{}, &rm.ReferenceRange[rm.DVOrdered]{}} {
		if err := EnsureSingle(parent, "no_such_attr", "x"); !errors.Is(err, ErrUnknownAttribute) {
			t.Errorf("EnsureSingle(%T, no_such_attr) = %v, want ErrUnknownAttribute", parent, err)
		}
	}
	for _, parent := range []any{&rm.DVOrdinal{}, &rm.DVCount{}, &rm.DVDate{}} {
		if err := AppendMultiple(parent, "symbol", &rm.DVCodedText{}); !errors.Is(err, ErrUnknownAttribute) {
			t.Errorf("AppendMultiple(%T, symbol) = %v, want ErrUnknownAttribute", parent, err)
		}
	}
}

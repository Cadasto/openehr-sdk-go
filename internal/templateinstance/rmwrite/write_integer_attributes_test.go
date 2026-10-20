package rmwrite

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// integerSlot is an Integer attribute of a data value or an event, written
// from a whole number, and how it reads back.
type integerSlot struct {
	name   string
	parent func() any
	attr   string
	// got reads the attribute back from the parent EnsureSingle wrote.
	got func(parent any) rm.Integer
}

// integerSlots are the Integer attributes EnsureSingle writes. An RM Integer
// is 32 bits wide.
var integerSlots = []integerSlot{
	{
		name:   "DV_QUANTITY precision",
		parent: func() any { return &rm.DVQuantity{} },
		attr:   "precision",
		got:    func(p any) rm.Integer { return *p.(*rm.DVQuantity).Precision },
	},
	{
		name:   "DV_MULTIMEDIA size",
		parent: func() any { return &rm.DVMultimedia{} },
		attr:   "size",
		got:    func(p any) rm.Integer { return p.(*rm.DVMultimedia).Size },
	},
	{
		name:   "INTERVAL_EVENT sample_count",
		parent: func() any { return &rm.IntervalEvent[rm.ItemStructure]{} },
		attr:   "sample_count",
		got:    func(p any) rm.Integer { return *p.(*rm.IntervalEvent[rm.ItemStructure]).SampleCount },
	},
}

// TestREQ107_EnsureSingleWritesAnIntegerWithinItsRange checks that EnsureSingle
// writes a whole number of any Go integer shape on an Integer attribute, at
// both ends of the 32-bit range an Integer holds, and refuses a nil pointer.
func TestREQ107_EnsureSingleWritesAnIntegerWithinItsRange(t *testing.T) {
	t.Parallel()
	children := []struct {
		name  string
		child any
		want  rm.Integer
	}{
		{"int64", int64(7), 7},
		{"int", 8, 8},
		{"rm.Integer", rm.Integer(9), 9},
		{"*rm.Integer", new(rm.Integer(10)), 10},
		{"the largest Integer", int64(math.MaxInt32), math.MaxInt32},
		{"the smallest Integer", int64(math.MinInt32), math.MinInt32},
	}
	for _, slot := range integerSlots {
		for _, tc := range children {
			parent := slot.parent()
			if err := EnsureSingle(parent, slot.attr, tc.child); err != nil {
				t.Errorf("%s: EnsureSingle(%T, %q, %s %v) = %v, want nil", slot.name, parent, slot.attr, tc.name, tc.child, err)
				continue
			}
			if got := slot.got(parent); got != tc.want {
				t.Errorf("%s: after writing %s, the attribute = %d, want %d", slot.name, tc.name, got, tc.want)
			}
		}
		if err := EnsureSingle(slot.parent(), slot.attr, (*rm.Integer)(nil)); !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("%s: EnsureSingle(nil *rm.Integer) = %v, want ErrTypeMismatch", slot.name, err)
		}
	}
}

// TestREQ107_EnsureSingleSaysAnIntegerOutOfRangeIsOutOfRange checks that a
// whole number outside the 32-bit range of an Integer, at either end, is
// refused with ErrTypeMismatch and a message that says so: it is the range
// that fails, not the type, so the message must not read as a type error. A
// value that is no whole number keeps the type message.
func TestREQ107_EnsureSingleSaysAnIntegerOutOfRangeIsOutOfRange(t *testing.T) {
	t.Parallel()
	const rangeMessage = "outside the 32-bit Integer range"
	for _, slot := range integerSlots {
		for _, child := range []any{int64(math.MaxInt32) + 1, int64(math.MinInt32) - 1, int64(1) << 40, -int64(1) << 40} {
			err := EnsureSingle(slot.parent(), slot.attr, child)
			if !errors.Is(err, ErrTypeMismatch) {
				t.Errorf("%s: EnsureSingle(%v) = %v, want ErrTypeMismatch", slot.name, child, err)
				continue
			}
			if !strings.Contains(err.Error(), rangeMessage) {
				t.Errorf("%s: EnsureSingle(%v) = %q, want the message to say %q", slot.name, child, err, rangeMessage)
			}
			if strings.Contains(err.Error(), "int64") {
				t.Errorf("%s: EnsureSingle(%v) = %q, want no type complaint for a whole number", slot.name, child, err)
			}
		}
		err := EnsureSingle(slot.parent(), slot.attr, 2.5)
		if !errors.Is(err, ErrTypeMismatch) || strings.Contains(err.Error(), rangeMessage) || !strings.Contains(err.Error(), "expects Integer, got float64") {
			t.Errorf("%s: EnsureSingle(2.5) = %v, want ErrTypeMismatch saying it expects Integer, got float64", slot.name, err)
		}
	}
}

// TestREQ107_EnsureSingleWritesCodePhrasesOfADVMultimedia checks that a
// DV_MULTIMEDIA's charset, language, compression_algorithm and
// integrity_check_algorithm take a code phrase as a value or as a pointer,
// as every other code phrase attribute does, and refuse a nil pointer and a
// child of another type with ErrTypeMismatch.
func TestREQ107_EnsureSingleWritesCodePhrasesOfADVMultimedia(t *testing.T) {
	t.Parallel()
	code := rm.CodePhrase{CodeString: "UTF-8", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}}
	slots := []struct {
		attr string
		got  func(m *rm.DVMultimedia) *rm.CodePhrase
	}{
		{"charset", func(m *rm.DVMultimedia) *rm.CodePhrase { return m.Charset }},
		{"language", func(m *rm.DVMultimedia) *rm.CodePhrase { return m.Language }},
		{"compression_algorithm", func(m *rm.DVMultimedia) *rm.CodePhrase { return m.CompressionAlgorithm }},
		{"integrity_check_algorithm", func(m *rm.DVMultimedia) *rm.CodePhrase { return m.IntegrityCheckAlgorithm }},
	}
	for _, slot := range slots {
		for name, child := range map[string]any{"value": code, "pointer": &code} {
			m := &rm.DVMultimedia{}
			if err := EnsureSingle(m, slot.attr, child); err != nil {
				t.Errorf("DV_MULTIMEDIA %s: EnsureSingle(%s) = %v, want nil", slot.attr, name, err)
				continue
			}
			if got := slot.got(m); got == nil || *got != code {
				t.Errorf("DV_MULTIMEDIA %s after writing a %s = %+v, want %+v", slot.attr, name, got, code)
			}
		}
		for _, child := range []any{(*rm.CodePhrase)(nil), "UTF-8", 42} {
			if err := EnsureSingle(&rm.DVMultimedia{}, slot.attr, child); !errors.Is(err, ErrTypeMismatch) {
				t.Errorf("DV_MULTIMEDIA %s: EnsureSingle(%#v) = %v, want ErrTypeMismatch", slot.attr, child, err)
			}
		}
	}
}

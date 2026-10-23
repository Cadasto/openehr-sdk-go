package rmwrite

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// valueWrite is one write of a child to an attribute of a data value, and
// what the attribute then holds.
type valueWrite struct {
	name   string
	parent any
	attr   string
	child  any
	// got reads the attribute back from parent.
	got  func() any
	want any
}

// TestREQ107_EnsureSingleWritesDataValueAttributes checks that EnsureSingle
// writes the data value attributes the instance generator reaches beside a
// value's main one: a DV_TEXT's or DV_CODED_TEXT's language, encoding,
// formatting and hyperlink (a DV_URI or a DV_EHR_URI); a DV_MULTIMEDIA's
// uri and thumbnail; the optional quantified attributes of a DV_QUANTITY, a
// DV_COUNT, a DV_DURATION and the three temporal values; a DV_STATE's value
// and is_terminal; and the DV_PARSABLE value of both time specifications.
// AppendMultiple appends a DV_TEXT or a DV_CODED_TEXT to a DV_PARAGRAPH's
// items. A code phrase, a coded text and a parsable are taken as a value or
// as a pointer.
func TestREQ107_EnsureSingleWritesDataValueAttributes(t *testing.T) {
	t.Parallel()
	code := rm.CodePhrase{CodeString: "nl", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
	utf8 := rm.CodePhrase{CodeString: "UTF-8", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}}
	link := &rm.DVURI{Value: "http://example.org/a"}
	ehrLink := &rm.DVEHRURI{Value: "ehr://example/a"}
	state := rm.DVCodedText{Value: "active", DefiningCode: rm.CodePhrase{CodeString: "245", TerminologyID: rm.TerminologyID{Value: "openehr"}}}
	spec := rm.DVParsable{Value: "daily", Formalism: "HL7:PIVL"}

	text, coded := &rm.DVText{}, &rm.DVCodedText{}
	linked, ehrLinked := &rm.DVText{}, &rm.DVCodedText{}
	media, ehrMedia := &rm.DVMultimedia{}, &rm.DVMultimedia{}
	quantity, count, duration := &rm.DVQuantity{}, &rm.DVCount{}, &rm.DVDuration{}
	date, clock, stamp := &rm.DVDate{}, &rm.DVTime{}, &rm.DVDateTime{}
	dvState := &rm.DVState{}
	general, periodic := &rm.DVGeneralTimeSpecification{}, &rm.DVPeriodicTimeSpecification{}
	cases := []valueWrite{
		{"DV_TEXT value", text, "value", "x", func() any { return text.Value }, "x"},
		{"DV_TEXT language", text, "language", &code, func() any { return *text.Language }, code},
		{"DV_TEXT encoding", text, "encoding", utf8, func() any { return *text.Encoding }, utf8},
		{"DV_TEXT formatting", text, "formatting", "plain", func() any { return *text.Formatting }, "plain"},
		{"DV_TEXT hyperlink DV_URI", linked, "hyperlink", link, func() any { return linked.Hyperlink }, rm.DVURILike(link)},
		{"DV_CODED_TEXT value", coded, "value", "y", func() any { return coded.Value }, "y"},
		{"DV_CODED_TEXT language", coded, "language", code, func() any { return *coded.Language }, code},
		{"DV_CODED_TEXT encoding", coded, "encoding", &utf8, func() any { return *coded.Encoding }, utf8},
		{"DV_CODED_TEXT formatting", coded, "formatting", "markdown", func() any { return *coded.Formatting }, "markdown"},
		{"DV_CODED_TEXT hyperlink DV_EHR_URI", ehrLinked, "hyperlink", ehrLink, func() any { return ehrLinked.Hyperlink }, rm.DVURILike(ehrLink)},
		{"DV_MULTIMEDIA uri DV_URI", media, "uri", link, func() any { return media.URI }, rm.DVURILike(link)},
		{"DV_MULTIMEDIA uri DV_EHR_URI", ehrMedia, "uri", ehrLink, func() any { return ehrMedia.URI }, rm.DVURILike(ehrLink)},
		{"DV_MULTIMEDIA thumbnail", media, "thumbnail", &rm.DVMultimedia{Size: 7}, func() any { return media.Thumbnail.Size }, rm.Integer(7)},
		{"DV_QUANTITY accuracy", quantity, "accuracy", 0.5, func() any { return *quantity.Accuracy }, rm.Real(0.5)},
		{"DV_QUANTITY accuracy_is_percent", quantity, "accuracy_is_percent", true, func() any { return *quantity.AccuracyIsPercent }, true},
		{"DV_QUANTITY magnitude_status", quantity, "magnitude_status", "~", func() any { return *quantity.MagnitudeStatus }, "~"},
		{"DV_QUANTITY precision", quantity, "precision", int64(2), func() any { return *quantity.Precision }, rm.Integer(2)},
		{"DV_QUANTITY units_system", quantity, "units_system", "http://unitsofmeasure.org", func() any { return *quantity.UnitsSystem }, "http://unitsofmeasure.org"},
		{"DV_QUANTITY units_display_name", quantity, "units_display_name", "mm", func() any { return *quantity.UnitsDisplayName }, "mm"},
		{"DV_COUNT accuracy", count, "accuracy", int64(1), func() any { return *count.Accuracy }, rm.Real(1)},
		{"DV_COUNT accuracy_is_percent", count, "accuracy_is_percent", false, func() any { return *count.AccuracyIsPercent }, false},
		{"DV_COUNT magnitude_status", count, "magnitude_status", "<=", func() any { return *count.MagnitudeStatus }, "<="},
		{"DV_DURATION value", duration, "value", "PT1H", func() any { return duration.Value }, "PT1H"},
		{"DV_DURATION accuracy", duration, "accuracy", 2.5, func() any { return *duration.Accuracy }, rm.Real(2.5)},
		{"DV_DURATION accuracy_is_percent", duration, "accuracy_is_percent", true, func() any { return *duration.AccuracyIsPercent }, true},
		{"DV_DURATION magnitude_status", duration, "magnitude_status", "=", func() any { return *duration.MagnitudeStatus }, "="},
		{"DV_DATE value", date, "value", "2020-01-01", func() any { return date.Value }, "2020-01-01"},
		{"DV_DATE magnitude_status", date, "magnitude_status", ">", func() any { return *date.MagnitudeStatus }, ">"},
		{"DV_DATE accuracy", date, "accuracy", &rm.DVDuration{Value: "P1D"}, func() any { return date.Accuracy.Value }, "P1D"},
		{"DV_TIME value", clock, "value", "12:00:00", func() any { return clock.Value }, "12:00:00"},
		{"DV_TIME magnitude_status", clock, "magnitude_status", "<", func() any { return *clock.MagnitudeStatus }, "<"},
		{"DV_TIME accuracy", clock, "accuracy", rm.DVDuration{Value: "PT1M"}, func() any { return clock.Accuracy.Value }, "PT1M"},
		{"DV_DATE_TIME value", stamp, "value", "2020-01-01T12:00:00Z", func() any { return stamp.Value }, "2020-01-01T12:00:00Z"},
		{"DV_DATE_TIME magnitude_status", stamp, "magnitude_status", ">=", func() any { return *stamp.MagnitudeStatus }, ">="},
		{"DV_DATE_TIME accuracy", stamp, "accuracy", &rm.DVDuration{Value: "PT1S"}, func() any { return stamp.Accuracy.Value }, "PT1S"},
		{"DV_STATE value", dvState, "value", &state, func() any { return dvState.Value.DefiningCode }, state.DefiningCode},
		{"DV_STATE is_terminal", dvState, "is_terminal", true, func() any { return dvState.IsTerminal }, true},
		{"DV_GENERAL_TIME_SPECIFICATION value", general, "value", spec, func() any { return general.Value }, spec},
		{"DV_PERIODIC_TIME_SPECIFICATION value", periodic, "value", &spec, func() any { return periodic.Value }, spec},
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

	t.Run("DV_PARAGRAPH items", func(t *testing.T) {
		t.Parallel()
		p := &rm.DVParagraph{}
		for _, child := range []any{&rm.DVText{Value: "first"}, &state} {
			if err := AppendMultiple(p, "items", child); err != nil {
				t.Fatalf("AppendMultiple(DV_PARAGRAPH, items, %T) = %v, want nil", child, err)
			}
		}
		if len(p.Items) != 2 || p.Items[0].GetValue() != "first" || p.Items[1].GetValue() != state.Value {
			t.Errorf("DV_PARAGRAPH.items = %+v, want the texts first and %q", p.Items, state.Value)
		}
		if _, coded := p.Items[1].GetDefiningCode(); !coded {
			t.Errorf("DV_PARAGRAPH.items[1] = %T, want the coded text kept coded", p.Items[1])
		}
	})
}

// TestREQ107_EnsureSingleRefusesWrongDataValueChildren checks that each new
// data value attribute refuses a child of the wrong Go type with
// ErrTypeMismatch, and so does a nil code phrase, a typed-nil uri and a
// precision past the 32-bit range of an Integer. An attribute the class
// does not declare stays ErrUnknownAttribute.
func TestREQ107_EnsureSingleRefusesWrongDataValueChildren(t *testing.T) {
	t.Parallel()
	refused := []struct {
		parent any
		attr   string
		child  any
	}{
		{&rm.DVText{}, "language", "nl"},
		{&rm.DVText{}, "language", (*rm.CodePhrase)(nil)},
		{&rm.DVText{}, "encoding", 42},
		{&rm.DVText{}, "formatting", 42},
		{&rm.DVText{}, "hyperlink", "http://example.org"},
		{&rm.DVText{}, "hyperlink", (*rm.DVURI)(nil)},
		{&rm.DVCodedText{}, "language", 42},
		{&rm.DVCodedText{}, "hyperlink", &rm.DVText{}},
		{&rm.DVMultimedia{}, "uri", "http://example.org"},
		{&rm.DVMultimedia{}, "uri", (*rm.DVEHRURI)(nil)},
		{&rm.DVMultimedia{}, "thumbnail", &rm.DVParsable{}},
		{&rm.DVQuantity{}, "accuracy", "x"},
		{&rm.DVQuantity{}, "accuracy_is_percent", 1},
		{&rm.DVQuantity{}, "magnitude_status", 42},
		{&rm.DVQuantity{}, "precision", 2.5},
		{&rm.DVQuantity{}, "precision", int64(1) << 40},
		{&rm.DVQuantity{}, "units_system", 42},
		{&rm.DVQuantity{}, "units_display_name", 42},
		{&rm.DVCount{}, "accuracy", "x"},
		{&rm.DVCount{}, "accuracy_is_percent", "true"},
		{&rm.DVCount{}, "magnitude_status", 42},
		{&rm.DVDuration{}, "accuracy", "x"},
		{&rm.DVDuration{}, "accuracy_is_percent", 1},
		{&rm.DVDuration{}, "magnitude_status", 42},
		{&rm.DVDate{}, "accuracy", "P1D"},
		{&rm.DVTime{}, "magnitude_status", 42},
		{&rm.DVDateTime{}, "accuracy", (*rm.DVDuration)(nil)},
		{&rm.DVState{}, "value", &rm.DVText{}},
		{&rm.DVState{}, "is_terminal", "true"},
		{&rm.DVGeneralTimeSpecification{}, "value", "daily"},
		{&rm.DVPeriodicTimeSpecification{}, "value", &rm.DVText{}},
	}
	for _, tc := range refused {
		if err := EnsureSingle(tc.parent, tc.attr, tc.child); !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("EnsureSingle(%T, %q, %#v) = %v, want ErrTypeMismatch", tc.parent, tc.attr, tc.child, err)
		}
	}
	if err := AppendMultiple(&rm.DVParagraph{}, "items", &rm.DVURI{}); !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("AppendMultiple(DV_PARAGRAPH, items, *rm.DVURI) = %v, want ErrTypeMismatch", err)
	}
	for _, parent := range []any{
		&rm.DVText{}, &rm.DVCodedText{}, &rm.DVDate{}, &rm.DVTime{}, &rm.DVDateTime{}, &rm.DVDuration{},
		&rm.DVState{}, &rm.DVGeneralTimeSpecification{}, &rm.DVPeriodicTimeSpecification{},
	} {
		if err := EnsureSingle(parent, "no_such_attr", "x"); !errors.Is(err, ErrUnknownAttribute) {
			t.Errorf("EnsureSingle(%T, no_such_attr) = %v, want ErrUnknownAttribute", parent, err)
		}
	}
	if err := AppendMultiple(&rm.DVParagraph{}, "no_such_attr", &rm.DVText{}); !errors.Is(err, ErrUnknownAttribute) {
		t.Errorf("AppendMultiple(DV_PARAGRAPH, no_such_attr) = %v, want ErrUnknownAttribute", err)
	}
}

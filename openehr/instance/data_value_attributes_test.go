package instance_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// The OPTs below put one data value under an ELEMENT and constrain
// attributes of it beside its main value, for the REQ-107 rules on leaf
// values, RM defaults and Precedence 2: a text's language, encoding,
// hyperlink and formatting; a multimedia's uri, thumbnail, language and
// charset; a parsable's language and charset; the secondary attributes of a
// quantity, a count, a duration and a date; and the attributes of a state,
// a paragraph and the two time specifications. Each attribute comes out
// written, as the OPT constrains it, or with its RM default where the OPT
// gives it no value.

// dvLanguage is the Options.Language of the cases below. It is not the
// template's language (en), so a default taken from Options is told apart
// from one taken from the template.
const dvLanguage = "nl"

var (
	dvLanguageCode = rm.CodePhrase{CodeString: dvLanguage, TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
	dvUTF8         = rm.CodePhrase{CodeString: "UTF-8", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}}
	dvTextPlain    = rm.CodePhrase{CodeString: "text/plain", TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}}
)

// dvBarePhrase is a CODE_PHRASE the OPT names with no constraint, so the
// walk writes the placeholder local::at0000 in it.
var dvBarePhrase = optNode("CODE_PHRASE", "")

// dvOnlyTrue is a C_BOOLEAN that admits true alone, so a value the walk
// writes from it is told apart from the zero value false.
var dvOnlyTrue = optPrimitive("BOOLEAN", "C_BOOLEAN", "<true_valid>true</true_valid><false_valid>false</false_valid>")

// dvOnlyFalse is a C_BOOLEAN that admits false alone.
var dvOnlyFalse = optPrimitive("BOOLEAN", "C_BOOLEAN", "<true_valid>false</true_valid><false_valid>true</false_valid>")

// dvInteger is a C_INTEGER listing n alone.
func dvInteger(n string) string { return optPrimitive("INTEGER", "C_INTEGER", "<list>"+n+"</list>") }

// dvReal is a C_REAL listing r alone.
func dvReal(r string) string { return optPrimitive("REAL", "C_REAL", "<list>"+r+"</list>") }

// dvURI is a DV_URI whose value a C_STRING pins to value.
func dvURI(value string) string { return optNode("DV_URI", "", optSingle("value", guardString(value))) }

// dvCase is one ELEMENT value the cases below generate.
type dvCase struct {
	name  string
	value string // the ELEMENT's value node
	check func(t *testing.T, out any)
	// policies are the policies the case runs under; every policy when
	// empty.
	policies []instance.Policy
	// floorOnly is set where an RM rule breaks a prohibition of the OPT
	// (REQ-107 § Exceptions and known gaps), so the template validator
	// reports it and only the RM floor is checked.
	floorOnly bool
	// unchecked is set where the OPT pins a value no RM rule accepts, which
	// the generator keeps (REQ-107 § Exceptions and known gaps), so no
	// validator is run: a validator that stays silent does so only because
	// it does not evaluate that rule.
	unchecked bool
}

// runDataValueCases generates each case under its policies, both value
// fills, and with and without the implicit attributes, checks what it
// asserts, and checks the root against both validators.
func runDataValueCases(t *testing.T, cases []dvCase) {
	t.Helper()
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value)), implicit)
			for _, opts := range optionsFor(tc.policies) {
				opts.Language = dvLanguage
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v, want a root", err)
					}
					tc.check(t, out)
					if tc.unchecked {
						return
					}
					if tc.floorOnly {
						noFloorErrors(t, out)
						return
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}

// dvValue returns the value of the ELEMENT root out as a T.
func dvValue[T rm.DataValue](t *testing.T, out any) T {
	t.Helper()
	el, ok := out.(*rm.Element)
	if !ok {
		t.Fatalf("generated root is %T, want *rm.Element", out)
	}
	v, ok := el.Value.(T)
	if !ok {
		var want T
		t.Fatalf("ELEMENT.value is %T, want %T", el.Value, want)
	}
	return v
}

// checkHeldCode fails t unless got is set and equals want.
func checkHeldCode(t *testing.T, what string, got *rm.CodePhrase, want rm.CodePhrase) {
	t.Helper()
	if got == nil {
		t.Errorf("%s is unset, want %+v", what, want)
		return
	}
	if *got != want {
		t.Errorf("%s = %+v, want %+v", what, *got, want)
	}
}

// checkHeldString fails t unless got is set and equals want.
func checkHeldString(t *testing.T, what string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s is unset, want %q", what, want)
		return
	}
	if *got != want {
		t.Errorf("%s = %q, want %q", what, *got, want)
	}
}

// checkURI fails t unless got is a DV_URI whose value is want.
func checkURI(t *testing.T, what string, got rm.DVURILike, want string) {
	t.Helper()
	uri, ok := got.(*rm.DVURI)
	if !ok || uri == nil {
		t.Errorf("%s = %#v, want a *rm.DVURI of %q", what, got, want)
		return
	}
	if uri.Value != want {
		t.Errorf("%s = %q, want %q", what, uri.Value, want)
	}
}

// textCodes returns a check that the text's language and encoding are
// language and encoding.
func textCodes(language, encoding rm.CodePhrase) func(t *testing.T, out any) {
	return func(t *testing.T, out any) {
		t.Helper()
		v := dvValue[*rm.DVText](t, out)
		checkHeldCode(t, "DV_TEXT.language", v.Language, language)
		checkHeldCode(t, "DV_TEXT.encoding", v.Encoding, encoding)
	}
}

// TestREQ107_TextAttributesAreWritten is the REQ-107 check that a DV_TEXT's
// or DV_CODED_TEXT's language, encoding, hyperlink and formatting the OPT
// constrains are written: a code the OPT gives is kept, and where it names
// the attribute with no code (a bare CODE_PHRASE, a C_CODE_PHRASE with an
// empty code list, or no child at all) the language takes Options.Language
// in ISO_639-1 and the encoding UTF-8 in IANA_character-sets (RM defaults),
// so Language_valid and Encoding_valid hold. It holds under every policy,
// both value fills, and with and without the implicit attributes, and the
// root passes both validators.
func TestREQ107_TextAttributesAreWritten(t *testing.T) {
	pinnedLanguage := rm.CodePhrase{CodeString: "de", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
	pinnedEncoding := rm.CodePhrase{CodeString: "US-ASCII", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}}
	codes := func(language, encoding string) string {
		return optNode("DV_TEXT", "", optSingle("language", language), optSingle("encoding", encoding))
	}
	runDataValueCases(t, []dvCase{
		{
			name:  "DV_TEXT language and encoding pinned",
			value: codes(optCodePhrase("ISO_639-1", "de"), optCodePhrase("IANA_character-sets", "US-ASCII")),
			check: textCodes(pinnedLanguage, pinnedEncoding),
		},
		{
			name:  "DV_TEXT language and encoding, empty-list C_CODE_PHRASE",
			value: codes(optCodePhrase("ISO_639-1"), optCodePhrase("IANA_character-sets")),
			check: textCodes(dvLanguageCode, dvUTF8),
		},
		{
			name:  "DV_TEXT language and encoding, bare CODE_PHRASE",
			value: codes(dvBarePhrase, dvBarePhrase),
			check: textCodes(dvLanguageCode, dvUTF8),
		},
		{
			name:  "DV_TEXT language and encoding named with no child",
			value: optNode("DV_TEXT", "", optSingle("language"), optSingle("encoding")),
			check: textCodes(dvLanguageCode, dvUTF8),
		},
		{
			name: "DV_CODED_TEXT language and encoding, bare CODE_PHRASE",
			value: optNode("DV_CODED_TEXT", "",
				optSingle("defining_code", optCodePhrase("SNOMED-CT", "248152002")),
				optSingle("language", dvBarePhrase), optSingle("encoding", dvBarePhrase)),
			check: func(t *testing.T, out any) {
				t.Helper()
				v := dvValue[*rm.DVCodedText](t, out)
				checkHeldCode(t, "DV_CODED_TEXT.language", v.Language, dvLanguageCode)
				checkHeldCode(t, "DV_CODED_TEXT.encoding", v.Encoding, dvUTF8)
			},
		},
		{
			name:  "DV_TEXT hyperlink",
			value: optNode("DV_TEXT", "", optSingle("hyperlink", dvURI("http://example.org/a"))),
			check: func(t *testing.T, out any) {
				t.Helper()
				checkURI(t, "DV_TEXT.hyperlink", dvValue[*rm.DVText](t, out).Hyperlink, "http://example.org/a")
			},
		},
		{
			name:  "DV_TEXT formatting",
			value: optNode("DV_TEXT", "", optSingle("formatting", guardString("plain"))),
			check: func(t *testing.T, out any) {
				t.Helper()
				v := dvValue[*rm.DVText](t, out)
				checkHeldString(t, "DV_TEXT.formatting", v.Formatting, "plain")
				// The OPT names neither language nor encoding, so the walk
				// writes neither, and no RM default is written there.
				if v.Language != nil || v.Encoding != nil {
					t.Errorf("DV_TEXT language %+v, encoding %+v, want both unset", v.Language, v.Encoding)
				}
			},
		},
	})
}

// checkExampleMultimedia fails t unless m is a DV_MULTIMEDIA with the RM
// defaults the generator gives one: the media type text/plain, the example
// payload as its data, and the size of that payload.
func checkExampleMultimedia(t *testing.T, what string, m *rm.DVMultimedia) {
	t.Helper()
	if m == nil {
		t.Errorf("%s is unset, want a DV_MULTIMEDIA", what)
		return
	}
	if m.MediaType != dvTextPlain {
		t.Errorf("%s.media_type = %+v, want %+v", what, m.MediaType, dvTextPlain)
	}
	if string(m.Data) != "example" || int(m.Size) != len(m.Data) {
		t.Errorf("%s: data %q of size %d, want %q sized by its bytes", what, m.Data, m.Size, "example")
	}
}

// TestREQ107_EncapsulatedAttributesAreWritten is the REQ-107 check that a
// DV_MULTIMEDIA's uri and thumbnail the OPT constrains are written, the
// thumbnail with the RM defaults of a DV_MULTIMEDIA whether the OPT names a
// node for it or none; that a DV_MULTIMEDIA's or DV_PARSABLE's language and
// charset the OPT names with no code take Options.Language in ISO_639-1 and
// UTF-8 in IANA_character-sets (RM defaults); and that where the OPT
// prohibits data, the uri RM Not_empty needs is built from its first allowed
// OPT child, whether the policy would not visit it or the OPT prohibits it
// too (Precedence 2). It holds under every policy, both value fills, and with
// and without the implicit attributes; an optional attribute the case
// asserts is checked under Large and Medium, the policies that visit it.
func TestREQ107_EncapsulatedAttributesAreWritten(t *testing.T) {
	const scan = "http://example.org/scan"
	multimediaCodes := func(t *testing.T, out any) {
		t.Helper()
		v := dvValue[*rm.DVMultimedia](t, out)
		checkHeldCode(t, "DV_MULTIMEDIA.language", v.Language, dvLanguageCode)
		checkHeldCode(t, "DV_MULTIMEDIA.charset", v.Charset, dvUTF8)
	}
	multimediaURI := func(t *testing.T, out any) {
		t.Helper()
		v := dvValue[*rm.DVMultimedia](t, out)
		checkURI(t, "DV_MULTIMEDIA.uri", v.URI, scan)
		if len(v.Data) != 0 {
			t.Errorf("DV_MULTIMEDIA.data = %q, want none where the OPT prohibits it", v.Data)
		}
	}
	runDataValueCases(t, []dvCase{
		{
			name: "DV_MULTIMEDIA uri and thumbnail",
			value: optNode("DV_MULTIMEDIA", "",
				optSingle("uri", dvURI(scan)), optSingle("thumbnail", optNode("DV_MULTIMEDIA", ""))),
			check: func(t *testing.T, out any) {
				t.Helper()
				v := dvValue[*rm.DVMultimedia](t, out)
				checkURI(t, "DV_MULTIMEDIA.uri", v.URI, scan)
				checkExampleMultimedia(t, "DV_MULTIMEDIA.thumbnail", v.Thumbnail)
				if v.Language != nil || v.Charset != nil {
					t.Errorf("DV_MULTIMEDIA language %+v, charset %+v, want both unset: the OPT names neither", v.Language, v.Charset)
				}
			},
		},
		{
			name:  "DV_MULTIMEDIA thumbnail named with no child",
			value: optNode("DV_MULTIMEDIA", "", optSingle("thumbnail")),
			check: func(t *testing.T, out any) {
				t.Helper()
				checkExampleMultimedia(t, "DV_MULTIMEDIA.thumbnail", dvValue[*rm.DVMultimedia](t, out).Thumbnail)
			},
		},
		{
			name:  "DV_MULTIMEDIA language and charset, bare CODE_PHRASE",
			value: optNode("DV_MULTIMEDIA", "", optSingle("language", dvBarePhrase), optSingle("charset", dvBarePhrase)),
			check: multimediaCodes,
		},
		{
			name: "DV_MULTIMEDIA language and charset, empty-list C_CODE_PHRASE",
			value: optNode("DV_MULTIMEDIA", "",
				optSingle("language", optCodePhrase("ISO_639-1")), optSingle("charset", optCodePhrase("IANA_character-sets"))),
			check: multimediaCodes,
		},
		{
			name:  "DV_MULTIMEDIA language and charset named with no child",
			value: optNode("DV_MULTIMEDIA", "", optSingle("language"), optSingle("charset")),
			check: multimediaCodes,
		},
		{
			name: "DV_PARSABLE language and charset, bare CODE_PHRASE",
			value: optNode("DV_PARSABLE", "",
				optOptionalSingleOver("language", dvBarePhrase), optOptionalSingleOver("charset", dvBarePhrase)),
			check: func(t *testing.T, out any) {
				t.Helper()
				v := dvValue[*rm.DVParsable](t, out)
				checkHeldCode(t, "DV_PARSABLE.language", v.Language, dvLanguageCode)
				checkHeldCode(t, "DV_PARSABLE.charset", v.Charset, dvUTF8)
			},
			policies: largeAndMedium,
		},
		{
			name:  "DV_MULTIMEDIA uri the RM needs where the OPT prohibits data and leaves uri optional",
			value: optNode("DV_MULTIMEDIA", "", optProhibitedSingle("data"), optOptionalSingleOver("uri", dvURI(scan))),
			check: multimediaURI,
		},
		{
			name:      "DV_MULTIMEDIA uri the RM needs where the OPT prohibits data and uri",
			value:     optNode("DV_MULTIMEDIA", "", optProhibitedSingle("data"), optProhibitedSingle("uri", dvURI(scan))),
			check:     multimediaURI,
			floorOnly: true,
		},
	})
}

// TestREQ107_QuantifiedAttributesAreWritten is the REQ-107 Leaf values
// check that a primitive leaf on a secondary attribute of a quantified value
// is written as the value fill says: a DV_QUANTITY's magnitude, precision,
// accuracy, accuracy_is_percent, magnitude_status, units_system and
// units_display_name; a DV_COUNT's accuracy, accuracy_is_percent and
// magnitude_status; a DV_DURATION's accuracy, accuracy_is_percent and
// magnitude_status; and a DV_DATE's magnitude_status, beside the DV_DURATION
// the OPT puts under its accuracy. Each constraint lists one value, so both
// value fills write it. It holds under every policy and with and without
// the implicit attributes, and the root passes both validators.
func TestREQ107_QuantifiedAttributesAreWritten(t *testing.T) {
	checkAccuracy := func(t *testing.T, what string, accuracy *rm.Real, isPercent *bool, want rm.Real) {
		t.Helper()
		if accuracy == nil || *accuracy != want {
			t.Errorf("%s.accuracy = %v, want %v", what, accuracy, want)
		}
		if isPercent == nil || *isPercent {
			t.Errorf("%s.accuracy_is_percent = %v, want false", what, isPercent)
		}
	}
	runDataValueCases(t, []dvCase{
		{
			name: "DV_QUANTITY",
			value: optNode("DV_QUANTITY", "",
				optSingle("magnitude", dvReal("3.5")),
				optSingle("units", guardString("mm")),
				optSingle("precision", dvInteger("2")),
				optSingle("accuracy", dvReal("0.5")),
				optSingle("accuracy_is_percent", dvOnlyFalse),
				optSingle("magnitude_status", guardString("~")),
				optSingle("units_system", guardString("http://unitsofmeasure.org")),
				optSingle("units_display_name", guardString("millimetre"))),
			check: func(t *testing.T, out any) {
				t.Helper()
				q := dvValue[*rm.DVQuantity](t, out)
				if q.Magnitude != 3.5 || q.Units != "mm" {
					t.Errorf("DV_QUANTITY = %v %q, want 3.5 \"mm\"", q.Magnitude, q.Units)
				}
				if q.Precision == nil || *q.Precision != 2 {
					t.Errorf("DV_QUANTITY.precision = %v, want 2", q.Precision)
				}
				checkAccuracy(t, "DV_QUANTITY", q.Accuracy, q.AccuracyIsPercent, 0.5)
				checkHeldString(t, "DV_QUANTITY.magnitude_status", q.MagnitudeStatus, "~")
				checkHeldString(t, "DV_QUANTITY.units_system", q.UnitsSystem, "http://unitsofmeasure.org")
				checkHeldString(t, "DV_QUANTITY.units_display_name", q.UnitsDisplayName, "millimetre")
			},
		},
		{
			name: "DV_COUNT",
			value: optNode("DV_COUNT", "",
				optSingle("magnitude", dvInteger("4")),
				optSingle("accuracy", dvReal("1.5")),
				optSingle("accuracy_is_percent", dvOnlyFalse),
				optSingle("magnitude_status", guardString("~"))),
			check: func(t *testing.T, out any) {
				t.Helper()
				c := dvValue[*rm.DVCount](t, out)
				if c.Magnitude != 4 {
					t.Errorf("DV_COUNT.magnitude = %d, want 4", c.Magnitude)
				}
				checkAccuracy(t, "DV_COUNT", c.Accuracy, c.AccuracyIsPercent, 1.5)
				checkHeldString(t, "DV_COUNT.magnitude_status", c.MagnitudeStatus, "~")
			},
		},
		{
			name: "DV_DURATION",
			value: optNode("DV_DURATION", "",
				optSingle("accuracy", dvReal("2")),
				optSingle("accuracy_is_percent", dvOnlyFalse),
				optSingle("magnitude_status", guardString("~"))),
			check: func(t *testing.T, out any) {
				t.Helper()
				d := dvValue[*rm.DVDuration](t, out)
				checkAccuracy(t, "DV_DURATION", d.Accuracy, d.AccuracyIsPercent, 2)
				checkHeldString(t, "DV_DURATION.magnitude_status", d.MagnitudeStatus, "~")
			},
		},
		{
			name:  "DV_PROPORTION magnitude_status",
			value: optNode("DV_PROPORTION", "", optSingle("magnitude_status", guardString("~"))),
			check: func(t *testing.T, out any) {
				t.Helper()
				checkHeldString(t, "DV_PROPORTION.magnitude_status", dvValue[*rm.DVProportion](t, out).MagnitudeStatus, "~")
			},
		},
		{
			name:  "DV_PROPORTION magnitude_status named with no child",
			value: optNode("DV_PROPORTION", "", optSingle("magnitude_status")),
			check: func(t *testing.T, out any) {
				t.Helper()
				checkHeldString(t, "DV_PROPORTION.magnitude_status", dvValue[*rm.DVProportion](t, out).MagnitudeStatus, "=")
			},
		},
		{
			name:  "DV_COUNT magnitude_status named with no child",
			value: optNode("DV_COUNT", "", optSingle("magnitude_status")),
			check: func(t *testing.T, out any) {
				t.Helper()
				checkHeldString(t, "DV_COUNT.magnitude_status", dvValue[*rm.DVCount](t, out).MagnitudeStatus, "=")
			},
		},
		{
			name:  "DV_DURATION magnitude_status named with no child",
			value: optNode("DV_DURATION", "", optSingle("magnitude_status")),
			check: func(t *testing.T, out any) {
				t.Helper()
				checkHeldString(t, "DV_DURATION.magnitude_status", dvValue[*rm.DVDuration](t, out).MagnitudeStatus, "=")
			},
		},
		{
			name:  "DV_DATE magnitude_status named with no child",
			value: optNode("DV_DATE", "", optSingle("magnitude_status")),
			check: func(t *testing.T, out any) {
				t.Helper()
				checkHeldString(t, "DV_DATE.magnitude_status", dvValue[*rm.DVDate](t, out).MagnitudeStatus, "=")
			},
		},
		{
			name: "DV_DATE",
			value: optNode("DV_DATE", "",
				optSingle("magnitude_status", guardString("~")),
				optSingle("accuracy", optNode("DV_DURATION", ""))),
			check: func(t *testing.T, out any) {
				t.Helper()
				d := dvValue[*rm.DVDate](t, out)
				checkHeldString(t, "DV_DATE.magnitude_status", d.MagnitudeStatus, "~")
				if d.Accuracy == nil || d.Accuracy.Value != "P0D" {
					t.Errorf("DV_DATE.accuracy = %+v, want a DV_DURATION of the sentinel P0D", d.Accuracy)
				}
			},
		},
	})
}

// TestREQ107_StateParagraphAndTimeSpecificationsAreWritten is the REQ-107
// check that an ELEMENT value of a class with no main primitive value is
// built with what the OPT constrains on it: a DV_STATE's value and
// is_terminal; a DV_PARAGRAPH's items; and the DV_PARSABLE value of a
// DV_GENERAL_TIME_SPECIFICATION and of a DV_PERIODIC_TIME_SPECIFICATION.
// Where the OPT gives the periodic specification's value no formalism, it
// takes HL7:PIVL (RM defaults), so Value_valid holds; a formalism the OPT
// gives is kept. A state or a paragraph the OPT names with no attribute
// gets what the RM requires. It holds under every policy, both value
// fills, and with and without the implicit attributes, and the root passes
// both validators.
func TestREQ107_StateParagraphAndTimeSpecificationsAreWritten(t *testing.T) {
	periodicFormalism := func(want string) func(t *testing.T, out any) {
		return func(t *testing.T, out any) {
			t.Helper()
			if got := dvValue[*rm.DVPeriodicTimeSpecification](t, out).Value.Formalism; got != want {
				t.Errorf("DV_PERIODIC_TIME_SPECIFICATION.value.formalism = %q, want %q", got, want)
			}
		}
	}
	paragraphItems := func(want string) func(t *testing.T, out any) {
		return func(t *testing.T, out any) {
			t.Helper()
			p := dvValue[*rm.DVParagraph](t, out)
			if len(p.Items) != 1 || p.Items[0].GetValue() != want {
				t.Errorf("DV_PARAGRAPH.items = %+v, want one text %q", p.Items, want)
			}
		}
	}
	runDataValueCases(t, []dvCase{
		{
			name: "DV_STATE",
			value: optNode("DV_STATE", "",
				optSingle("value", optCodedText("SNOMED-CT", "248152002")),
				optSingle("is_terminal", dvOnlyTrue)),
			check: func(t *testing.T, out any) {
				t.Helper()
				s := dvValue[*rm.DVState](t, out)
				if want := (rm.CodePhrase{CodeString: "248152002", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}}); s.Value.DefiningCode != want {
					t.Errorf("DV_STATE.value.defining_code = %+v, want %+v", s.Value.DefiningCode, want)
				}
				if !s.IsTerminal {
					t.Error("DV_STATE.is_terminal = false, want the true its C_BOOLEAN admits alone")
				}
			},
		},
		{
			name:  "DV_STATE named with no attribute",
			value: optNode("DV_STATE", ""),
			check: func(t *testing.T, out any) {
				t.Helper()
				// REQ-107 Placeholders: the code at0000 in local, where neither
				// the OPT nor an RM default gives a value.
				local := rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "local"}}
				if s := dvValue[*rm.DVState](t, out); s.Value.DefiningCode != local {
					t.Errorf("DV_STATE.value.defining_code = %+v, want %+v", s.Value.DefiningCode, local)
				}
			},
		},
		{
			name:  "DV_PARAGRAPH",
			value: optNode("DV_PARAGRAPH", "", optMultiple("items", optNode("DV_TEXT", "", optSingle("value", guardString("first line"))))),
			check: paragraphItems("first line"),
		},
		{
			name:  "DV_PARAGRAPH named with no attribute",
			value: optNode("DV_PARAGRAPH", ""),
			check: paragraphItems("example"),
		},
		{
			name: "DV_GENERAL_TIME_SPECIFICATION",
			value: optNode("DV_GENERAL_TIME_SPECIFICATION", "", optSingle("value", optNode("DV_PARSABLE", "",
				optSingle("value", guardString("daily")), optSingle("formalism", guardString("HL7:GTS"))))),
			check: func(t *testing.T, out any) {
				t.Helper()
				v := dvValue[*rm.DVGeneralTimeSpecification](t, out).Value
				if v.Value != "daily" || v.Formalism != "HL7:GTS" {
					t.Errorf("DV_GENERAL_TIME_SPECIFICATION.value = %q in %q, want \"daily\" in \"HL7:GTS\"", v.Value, v.Formalism)
				}
			},
		},
		{
			name:  "DV_PERIODIC_TIME_SPECIFICATION with a bare value",
			value: optNode("DV_PERIODIC_TIME_SPECIFICATION", "", optSingle("value", optNode("DV_PARSABLE", ""))),
			check: periodicFormalism("HL7:PIVL"),
		},
		{
			name:  "DV_PERIODIC_TIME_SPECIFICATION named with no attribute",
			value: optNode("DV_PERIODIC_TIME_SPECIFICATION", ""),
			check: periodicFormalism("HL7:PIVL"),
		},
		{
			name: "DV_PERIODIC_TIME_SPECIFICATION with its formalism pinned",
			value: optNode("DV_PERIODIC_TIME_SPECIFICATION", "", optSingle("value", optNode("DV_PARSABLE", "",
				optSingle("formalism", guardString("HL7:EIVL"))))),
			check: periodicFormalism("HL7:EIVL"),
		},
		{
			// Value_valid admits HL7:PIVL and HL7:EIVL alone, so the root
			// breaks it. The RM floor does not evaluate that invariant (it is
			// out of scope in its inventory), so it would pass on that gap
			// alone, and no validator is run.
			name: "DV_PERIODIC_TIME_SPECIFICATION with its formalism pinned to one the RM rejects",
			value: optNode("DV_PERIODIC_TIME_SPECIFICATION", "", optSingle("value", optNode("DV_PARSABLE", "",
				optSingle("formalism", guardString("HL7:GTS"))))),
			check:     periodicFormalism("HL7:GTS"),
			unchecked: true,
		},
	})
}

// TestREQ107_PeriodicFormalismTheOPTGivesIsKept is the REQ-107 Leaf values
// check that a DV_PERIODIC_TIME_SPECIFICATION's formalism the walk takes
// from the OPT's C_STRING, here HL7:EIVL, the first of HL7:EIVL and
// HL7:PIVL, is kept under ExampleFill: Value_valid admits HL7:EIVL, so the
// HL7:PIVL default does not replace it, though the C_STRING admits that too.
func TestREQ107_PeriodicFormalismTheOPTGivesIsKept(t *testing.T) {
	value := optNode("DV_PERIODIC_TIME_SPECIFICATION", "", optSingle("value", optNode("DV_PARSABLE", "",
		optSingle("formalism", optPrimitive("STRING", "C_STRING", "<list>HL7:EIVL</list><list>HL7:PIVL</list>")))))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", value)), implicit)
		for _, policy := range everyPolicy {
			t.Run(fmt.Sprintf("implicit=%t/%v", implicit, policy), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
				if err != nil {
					t.Fatalf("Generate: %v, want a root", err)
				}
				if got := dvValue[*rm.DVPeriodicTimeSpecification](t, out).Value.Formalism; got != "HL7:EIVL" {
					t.Errorf("DV_PERIODIC_TIME_SPECIFICATION.value.formalism = %q, want the example HL7:EIVL", got)
				}
				checkValid(t, out, c)
			})
		}
	}
}

// TestREQ107_TextCodeDefaultYieldsToTheOPT is the REQ-107 Precedence 1
// check that the RM default of a DV_TEXT's language and encoding yields to
// an OPT whose constraint rejects it: a C_CODE_PHRASE in terminology local
// with an empty code list keeps the placeholder local::at0000 the walk
// wrote, under every policy and both value fills. The placeholder breaks
// Language_valid and Encoding_valid, as § Exceptions and known gaps says
// of an OPT constraint that admits no value an RM rule accepts, so no
// validator is run.
func TestREQ107_TextCodeDefaultYieldsToTheOPT(t *testing.T) {
	local := rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "local"}}
	value := optNode("DV_TEXT", "", optSingle("language", optCodePhrase("local")), optSingle("encoding", optCodePhrase("local")))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", value)), implicit)
		for _, opts := range optionsFor(everyPolicy) {
			opts.Language = dvLanguage
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v, want a root", err)
				}
				textCodes(local, local)(t, out)
			})
		}
	}
}

// TestREQ107_MultimediaURIOnlyWhereTheRMNeedsIt is the REQ-107 Visit rule
// and Precedence 2 check that a DV_MULTIMEDIA's optional uri is not built
// under Required, which does not visit it, where the OPT allows data: the
// data the generator writes meets Not_empty, so no RM rule needs the uri.
// Large and Medium visit it and build it from its OPT child.
func TestREQ107_MultimediaURIOnlyWhereTheRMNeedsIt(t *testing.T) {
	const scan = "http://example.org/scan"
	value := optNode("DV_MULTIMEDIA", "", optOptionalSingleOver("uri", dvURI(scan)))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", value)), implicit)
		for _, opts := range optionsFor(everyPolicy) {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v, want a root", err)
				}
				m := dvValue[*rm.DVMultimedia](t, out)
				checkExampleMultimedia(t, "DV_MULTIMEDIA", m)
				if opts.Policy == instance.Required {
					if m.URI != nil {
						t.Errorf("DV_MULTIMEDIA.uri = %#v under Required, want none: the policy does not visit it and data meets Not_empty", m.URI)
					}
				} else {
					checkURI(t, "DV_MULTIMEDIA.uri", m.URI, scan)
				}
				checkValid(t, out, c)
			})
		}
	}
}

// TestREQ107_PrimitiveOnAnAttributeTheRMDoesNotHave is the REQ-107 check,
// for § Exceptions and known gaps, that a primitive leaf the OPT puts on an
// attribute the pinned RM does not have is left unwritten: Generate still
// returns a root, under every policy and both value fills, and writes the
// attributes the RM has. The template validator then reports the
// constraint, so it is not run.
func TestREQ107_PrimitiveOnAnAttributeTheRMDoesNotHave(t *testing.T) {
	value := optNode("DV_QUANTITY", "", optSingle("no_such_attribute", dvInteger("1")), optSingle("precision", dvInteger("2")))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", value)), implicit)
		for _, opts := range optionsFor(everyPolicy) {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v, want a root", err)
				}
				if q := dvValue[*rm.DVQuantity](t, out); q.Precision == nil || *q.Precision != 2 {
					t.Errorf("DV_QUANTITY.precision = %v, want 2", q.Precision)
				}
				noFloorErrors(t, out)
			})
		}
	}
}

// TestREQ107_WrongTypeLeafOnARoutedAttributeFailsGenerate is the REQ-107
// check that a primitive leaf whose value cannot be the type of the
// attribute the OPT puts it on makes Generate fail, where the attribute is
// one the generator writes through the template-instance writer: a C_REAL
// on a DV_STATE's is_terminal, and a C_REAL and a C_BOOLEAN on a
// DV_QUANTITY's precision. Only an attribute the pinned RM does not have is
// left unwritten (TestREQ107_PrimitiveOnAnAttributeTheRMDoesNotHave). The
// error wraps the writer's type mismatch and names the RM type, the
// attribute and the OPT path. It holds under every policy, both value fills
// and both compile modes.
func TestREQ107_WrongTypeLeafOnARoutedAttributeFailsGenerate(t *testing.T) {
	realLeaf := dvReal("1.5")
	cases := []struct {
		name  string
		value string
		// where is what the error names: the RM type and the attribute.
		where string
	}{
		{
			name:  "DV_STATE is_terminal, C_REAL",
			value: optNode("DV_STATE", "", optSingle("is_terminal", realLeaf)),
			where: "DV_STATE.is_terminal",
		},
		{
			name:  "DV_QUANTITY precision, C_REAL",
			value: optNode("DV_QUANTITY", "", optSingle("precision", realLeaf)),
			where: "DV_QUANTITY.precision",
		},
		{
			name:  "DV_QUANTITY precision, C_BOOLEAN",
			value: optNode("DV_QUANTITY", "", optSingle("precision", dvOnlyTrue)),
			where: "DV_QUANTITY.precision",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value)), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err == nil {
						t.Fatalf("Generate = %T, nil error, want an error: the leaf is not a value of the attribute", out)
					}
					if !errors.Is(err, rmwrite.ErrTypeMismatch) {
						t.Errorf("Generate error = %v, want one wrapping rmwrite.ErrTypeMismatch", err)
					}
					if !strings.Contains(err.Error(), tc.where) {
						t.Errorf("Generate error = %q, want it to name %s", err, tc.where)
					}
				})
			}
		}
	}
}

package xmlunmatched

// drop_internal_test.go: DropElement, the helper a generated decoder calls for
// an element its class's schema accepts that no member holds, which a root
// schema names in its dropped_elements (REQ-041), such as the `match_negated`
// of REQ-176: read and dropped, neither reported nor refused, when its text is
// the one value it may hold; refused in every mode with ErrInvalidShape when it
// is not.

import (
	"encoding/xml"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// flagged reads as a generated decoder reads a class with one member, `n`, and
// one dropped element, `legacy`, whose one value is `false`.
type flagged struct{ n int }

var flaggedClass = &Class{Name: "FLAGGED"}

func (f *flagged) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := Enter(dec, start, flaggedClass); err != nil {
		return err
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			switch LocalName(tk.Name) {
			case "n":
				if err := DecodeElement(dec, &tk, &f.n); err != nil {
					return err
				}
			case "legacy":
				if err := DropElement(dec, &tk, "false"); err != nil {
					return err
				}
			default:
				if err := SkipUnmatched(dec, tk); err != nil {
					return err
				}
			}
		case xml.CharData:
			if err := CheckUnmatchedText(dec, tk); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// flaggedDoc returns a FLAGGED document whose `legacy` element, written before
// and after `n`, holds text.
func flaggedDoc(attr, text string) string {
	return `<flagged xmlns="http://schemas.openehr.org/v1"><legacy` + attr + `>` + text + `</legacy><n>1</n><legacy>false</legacy></flagged>`
}

// TestDropElement_REQ041_REQ176_DropsItsOneValue pins that an element holding
// its one value, with or without XML white space around it, is read and
// dropped in every mode: the decode succeeds, decodes the members around it,
// reports nothing and refuses nothing.
func TestDropElement_REQ041_REQ176_DropsItsOneValue(t *testing.T) {
	for _, text := range []string{"false", " false\n\t"} {
		for mode, tracker := range documentModes() {
			t.Run(mode+"/"+text, func(t *testing.T) {
				var reported []string
				tr := tracker()
				if tr != nil && !tr.strict {
					tr = NewTracker(func(p string) { reported = append(reported, p) }, false)
				}
				var f flagged
				in := flaggedDoc("", text)
				if err := Unmarshal([]byte(in), &f, tr); err != nil {
					t.Fatalf("Unmarshal(%q) = %v; want nil, the element read and dropped", in, err)
				}
				if f.n != 1 {
					t.Errorf("Unmarshal(%q): n = %d; want 1", in, f.n)
				}
				if len(reported) != 0 {
					t.Errorf("Unmarshal(%q) reported %q; want nothing, the element is no unmatched content", in, reported)
				}
			})
		}
	}
}

// TestDropElement_REQ041_REQ176_RefusesAnyOtherText pins that any other text,
// none included, is refused in every mode as a primitive failure: a
// *typereg.DecodeError naming the element by its path, wrapping ErrInvalidShape
// and carrying none of the text.
func TestDropElement_REQ041_REQ176_RefusesAnyOtherText(t *testing.T) {
	const planted = "planted-secret-value"
	for _, text := range []string{"true", "0", "", planted} {
		for mode, tracker := range documentModes() {
			t.Run(mode+"/"+text, func(t *testing.T) {
				in := flaggedDoc("", text)
				err := Unmarshal([]byte(in), new(flagged), tracker())
				de, ok := errors.AsType[*typereg.DecodeError](err)
				if !ok || de == nil || !errors.Is(err, ErrInvalidShape) || !IsPrimitiveFailure(err) {
					t.Fatalf("Unmarshal(%q) = %v; want a primitive failure wrapping ErrInvalidShape", in, err)
				}
				if de.Path != "/flagged[1]/legacy[1]" {
					t.Errorf("Unmarshal(%q) err.Path = %q; want /flagged[1]/legacy[1]", in, de.Path)
				}
				if strings.Contains(err.Error(), planted) {
					t.Errorf("Unmarshal(%q) err = %q; it carries the element's text", in, err)
				}
			})
		}
	}
}

// TestDropElement_REQ041_REQ170_AttributeIsUnmatched pins that an attribute on
// the dropped element is unmatched content, as on any element read as text:
// reported by a reporting decode, refused by a strict one, skipped otherwise.
func TestDropElement_REQ041_REQ170_AttributeIsUnmatched(t *testing.T) {
	in := flaggedDoc(` x="1"`, "false")
	if err := Unmarshal([]byte(in), new(flagged), nil); err != nil {
		t.Errorf("Unmarshal(%q, untracked) = %v; want nil", in, err)
	}
	var reported []string
	if err := Unmarshal([]byte(in), new(flagged), NewTracker(func(p string) { reported = append(reported, p) }, false)); err != nil {
		t.Errorf("Unmarshal(%q, report) = %v; want nil", in, err)
	}
	if want := []string{"/flagged[1]/legacy[1]/@x"}; !slices.Equal(reported, want) {
		t.Errorf("Unmarshal(%q, report) reported %q; want %q", in, reported, want)
	}
	if err := Unmarshal([]byte(in), new(flagged), NewTracker(nil, true)); !errors.Is(err, ErrUnknownElement) {
		t.Errorf("Unmarshal(%q, strict) = %v; want ErrUnknownElement", in, err)
	}
}

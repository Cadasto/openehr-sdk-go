package rmwrite

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// REQ-107: EnsureSingle writes the String attributes of the four data
// values it used to refuse (DV_URI, DV_EHR_URI, DV_PARSABLE,
// DV_IDENTIFIER), so the generator's String fill reaches them.
func TestREQ107_EnsureSingleTextLikeParents(t *testing.T) {
	t.Parallel()

	type writeCase struct {
		name   string
		parent any
		attr   string
		child  any
		got    func() any
		want   any
	}
	code := rm.CodePhrase{CodeString: "UTF-8", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}}
	uri := &rm.DVURI{}
	ehr := &rm.DVEHRURI{}
	parsable := &rm.DVParsable{}
	ident := &rm.DVIdentifier{}
	cases := []writeCase{
		{"DV_URI value", uri, "value", "http://example.com", func() any { return uri.Value }, "http://example.com"},
		{"DV_EHR_URI value", ehr, "value", "ehr://example", func() any { return ehr.Value }, "ehr://example"},
		{"DV_PARSABLE value", parsable, "value", "x", func() any { return parsable.Value }, "x"},
		{"DV_PARSABLE formalism", parsable, "formalism", "text/plain", func() any { return parsable.Formalism }, "text/plain"},
		{"DV_PARSABLE charset", parsable, "charset", code, func() any { return parsable.Charset.CodeString }, "UTF-8"},
		{"DV_PARSABLE language", parsable, "language", &code, func() any { return parsable.Language.CodeString }, "UTF-8"},
		{"DV_IDENTIFIER id", ident, "id", "A-1", func() any { return ident.ID }, "A-1"},
		{"DV_IDENTIFIER issuer", ident, "issuer", "hospital", func() any { return *ident.Issuer }, "hospital"},
		{"DV_IDENTIFIER assigner", ident, "assigner", "registry", func() any { return *ident.Assigner }, "registry"},
		{"DV_IDENTIFIER type", ident, "type", "MRN", func() any { return *ident.Type }, "MRN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := EnsureSingle(tc.parent, "", tc.attr, tc.child); err != nil {
				t.Fatalf("EnsureSingle(%T, %q) = %v", tc.parent, tc.attr, err)
			}
			if got := tc.got(); got != tc.want {
				t.Errorf("%T.%s = %v, want %v", tc.parent, tc.attr, got, tc.want)
			}
		})
	}

	t.Run("wrong child type", func(t *testing.T) {
		t.Parallel()
		for _, parent := range []any{&rm.DVURI{}, &rm.DVEHRURI{}, &rm.DVParsable{}, &rm.DVIdentifier{}} {
			attr := "value"
			if _, ok := parent.(*rm.DVIdentifier); ok {
				attr = "id"
			}
			if err := EnsureSingle(parent, "", attr, 42); !errors.Is(err, ErrTypeMismatch) {
				t.Errorf("EnsureSingle(%T, %q, 42) = %v, want ErrTypeMismatch", parent, attr, err)
			}
		}
	})

	t.Run("unknown attribute", func(t *testing.T) {
		t.Parallel()
		for _, parent := range []any{&rm.DVURI{}, &rm.DVEHRURI{}, &rm.DVParsable{}, &rm.DVIdentifier{}} {
			if err := EnsureSingle(parent, "", "no_such_attr", "x"); !errors.Is(err, ErrUnknownAttribute) {
				t.Errorf("EnsureSingle(%T, no_such_attr) = %v, want ErrUnknownAttribute", parent, err)
			}
		}
	})
}

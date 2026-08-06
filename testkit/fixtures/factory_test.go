package fixtures_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestFactoryForXMLBody_prologue pins the root-element scan: the factory is
// chosen from the first element that is not a processing instruction, so any
// number of <?…?> prologue tokens must be skipped without shifting the match.
func TestFactoryForXMLBody_prologue(t *testing.T) {
	tests := []struct {
		name string
		body string
		want any // nil ⇒ expect ok == false
	}{
		{"bare root", `<COMPOSITION/>`, &rm.Composition{}},
		{"lowercase root", `<folder/>`, &rm.Folder{}},
		{"leading whitespace", "\n  \t<EHR_STATUS/>", &rm.EHRStatus{}},
		{"xml declaration", `<?xml version="1.0"?><COMPOSITION/>`, &rm.Composition{}},
		{"two prologue tokens", `<?xml version="1.0"?><?pi data?><FOLDER/>`, &rm.Folder{}},
		{"text before root", `junk<DV_QUANTITY/>`, &rm.DVQuantity{}},
		// A processing instruction with an empty target is malformed XML,
		// but it must still be consumed as one token — scanning past only
		// part of it would let the *next* element decide the type.
		{"empty PI target", `<?><COMPOSITION/>`, &rm.Composition{}},
		{"empty PI then folder", `<?><FOLDER/>`, &rm.Folder{}},
		{"unterminated PI", `<?xml version="1.0"`, nil},
		{"no element", `not xml at all`, nil},
		{"unknown root", `<WIDGET/>`, nil},
		{"empty", ``, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			factory, ok := fixtures.FactoryForXMLBody([]byte(tc.body))
			if tc.want == nil {
				if ok {
					t.Fatalf("FactoryForXMLBody(%q) = %T, true; want _, false", tc.body, factory())
				}
				return
			}
			if !ok {
				t.Fatalf("FactoryForXMLBody(%q) = _, false; want %T, true", tc.body, tc.want)
			}
			if got := factory(); !sameType(got, tc.want) {
				t.Errorf("FactoryForXMLBody(%q) = %T, want %T", tc.body, got, tc.want)
			}
		})
	}
}

func sameType(a, b any) bool {
	//nolint:gocritic // a type switch would not be shorter for a 2-value compare
	return typeName(a) == typeName(b)
}

func typeName(v any) string {
	switch v.(type) {
	case *rm.Composition:
		return "COMPOSITION"
	case *rm.Folder:
		return "FOLDER"
	case *rm.EHRStatus:
		return "EHR_STATUS"
	case *rm.DVQuantity:
		return "DV_QUANTITY"
	default:
		return "OTHER"
	}
}

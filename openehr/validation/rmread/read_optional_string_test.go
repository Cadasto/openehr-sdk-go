package rmread_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// TestREQ107_ReadSingle_OptionalString checks that the optional String
// attributes the generator can fill are readable: set, they read present
// and give back the pointer; unset, they read absent. Without an arm the
// floor and the template walker report a filled attribute as absent.
func TestREQ107_ReadSingle_OptionalString(t *testing.T) {
	s := "plain"
	cases := []struct {
		name        string
		set, unset  any
		rmType, att string
	}{
		{"DV_TEXT.formatting", &rm.DVText{Formatting: &s}, &rm.DVText{}, "DV_TEXT", "formatting"},
		{"DV_CODED_TEXT.formatting", &rm.DVCodedText{Formatting: &s}, &rm.DVCodedText{}, "DV_CODED_TEXT", "formatting"},
		{"DV_MULTIMEDIA.alternate_text", &rm.DVMultimedia{AlternateText: &s}, &rm.DVMultimedia{}, "DV_MULTIMEDIA", "alternate_text"},
		{"DV_QUANTITY.magnitude_status", &rm.DVQuantity{MagnitudeStatus: &s}, &rm.DVQuantity{}, "DV_QUANTITY", "magnitude_status"},
		{"DV_IDENTIFIER.issuer", &rm.DVIdentifier{Issuer: &s}, &rm.DVIdentifier{}, "DV_IDENTIFIER", "issuer"},
		{"DV_IDENTIFIER.assigner", &rm.DVIdentifier{Assigner: &s}, &rm.DVIdentifier{}, "DV_IDENTIFIER", "assigner"},
		{"DV_IDENTIFIER.type", &rm.DVIdentifier{Type: &s}, &rm.DVIdentifier{}, "DV_IDENTIFIER", "type"},
		{"CODE_PHRASE.preferred_term", &rm.CodePhrase{PreferredTerm: &s}, &rm.CodePhrase{}, "CODE_PHRASE", "preferred_term"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := rmread.ReadSingle(tc.set, tc.rmType, tc.att)
			if p, isPtr := got.(*string); !ok || !isPtr || p != &s {
				t.Errorf("ReadSingle(set) = (%v, %v), want the *string and true", got, ok)
			}
			if _, ok := rmread.ReadSingle(tc.unset, tc.rmType, tc.att); ok {
				t.Errorf("ReadSingle(unset) ok=true, want false")
			}
		})
	}
}

// TestReadSingle_IsmTransition checks the ISM_TRANSITION reader: the
// RM-mandatory current_state reads present once it carries a value, and the
// optional careflow_step and transition read present only when set.
func TestReadSingle_IsmTransition(t *testing.T) {
	step := rm.DVCodedText{
		Value:        "step",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at1"},
	}
	full := &rm.IsmTransition{CurrentState: step, CareflowStep: &step, Transition: &step}
	empty := &rm.IsmTransition{}
	for _, attr := range []string{"current_state", "careflow_step", "transition"} {
		if _, ok := rmread.ReadSingle(full, "ISM_TRANSITION", attr); !ok {
			t.Errorf("ReadSingle(populated, %q) ok=false, want true", attr)
		}
		if _, ok := rmread.ReadSingle(*full, "ISM_TRANSITION", attr); !ok {
			t.Errorf("ReadSingle(populated value form, %q) ok=false, want true", attr)
		}
		if _, ok := rmread.ReadSingle(empty, "ISM_TRANSITION", attr); ok {
			t.Errorf("ReadSingle(zero, %q) ok=true, want false", attr)
		}
	}
}

package instance

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_BMMFilledCodePhraseKeepsItsTerminology is the REQ-107 check
// that the fill of a code phrase's BMM-mandatory attributes keeps the
// terminology the code phrase carries: the TERMINOLOGY_ID it builds from
// the BMM for terminology_id is empty, and must not replace local. It
// calls the fill directly, because the template-instance writer gives a
// coded text's or a media type's code phrase a terminology again when it
// attaches one with none, so the output of Generate cannot show the loss.
func TestREQ107_BMMFilledCodePhraseKeepsItsTerminology(t *testing.T) {
	cp := &rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "local"}}
	(&generator{}).populateBMMRequiredAttrs(cp, "CODE_PHRASE", 0)
	if want := (rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "local"}}); *cp != want {
		t.Errorf("populateBMMRequiredAttrs(CODE_PHRASE) = %+v, want %+v", *cp, want)
	}
}

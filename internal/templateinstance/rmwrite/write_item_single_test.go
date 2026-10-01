package rmwrite

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_EnsureSingleItemSingleName is the REQ-107 check that the
// generator can attach the RM-mandatory name of an ITEM_SINGLE, as a
// plain DV_TEXT and as a DV_CODED_TEXT.
func TestREQ107_EnsureSingleItemSingleName(t *testing.T) {
	t.Parallel()
	coded := &rm.DVCodedText{
		Value:        "coded name",
		DefiningCode: rm.CodePhrase{CodeString: "at0001", TerminologyID: rm.TerminologyID{Value: "local"}},
	}
	for _, child := range []any{&rm.DVText{Value: "plain name"}, coded} {
		parent := &rm.ItemSingle{}
		if err := EnsureSingle(parent, "ITEM_SINGLE", "name", child); err != nil {
			t.Fatalf("EnsureSingle(ITEM_SINGLE, name, %T) = %v, want nil", child, err)
		}
		if parent.Name == nil {
			t.Fatalf("EnsureSingle(ITEM_SINGLE, name, %T) left name unset", child)
		}
		want := child.(rm.DVTextLike)
		if got := parent.Name.GetValue(); got != want.GetValue() {
			t.Errorf("ITEM_SINGLE.name value = %q after EnsureSingle(%T), want %q", got, child, want.GetValue())
		}
	}
}

package instance

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_PartyRelationshipNodeIDPlaceholder is the REQ-107 check that
// fillPartyRelationship writes the node id at0000 on a PARTY_RELATIONSHIP
// that has none. It calls the fill directly: Generate names every
// relationship before the fill runs, so a test through Generate cannot
// reach this write.
func TestREQ107_PartyRelationshipNodeIDPlaceholder(t *testing.T) {
	rel := &rm.PartyRelationship{}
	(&generator{}).fillPartyRelationship(nil, rel)
	if got := rel.GetArchetypeNodeID(); got != "at0000" {
		t.Errorf("fillPartyRelationship: archetype_node_id = %q, want %q", got, "at0000")
	}
}

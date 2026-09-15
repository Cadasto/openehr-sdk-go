package serializeprobes

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestProbe038GuardCatchesWireEquivFixpointBreak is the can-fail control for
// PROBE-038's wire-equivalence fixpoint leg. The lossy secondMarshal drops
// `units` on the b2 encode only, so the discriminator multiset from the first
// re-marshal still matches the input while b1 and b2 are not wire-equivalent.
// The mutation that turns this red is removing the wireequiv check in
// probe038PolymorphicDecode: then a field lost across the second decode would
// slip through with Status == "pass".
func TestProbe038GuardCatchesWireEquivFixpointBreak(t *testing.T) {
	body := []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`)
	r, err := probe038PolymorphicDecode(body, func() any { return new(rm.DVQuantity) }, dropMemberReEncoder("units"))
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("status = %q, want fail: dropping units on the second encode must break the wire-equivalence fixpoint", r.Status)
	}
	if !strings.Contains(r.Detail, "wire-equivalent") {
		t.Fatalf("detail = %q; want the wire-equivalence fixpoint leg to fire after the discriminator multiset passes", r.Detail)
	}
}

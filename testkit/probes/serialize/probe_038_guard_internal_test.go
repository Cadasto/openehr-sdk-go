package serializeprobes

import (
	"encoding/json"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestProbe038GuardCatchesWireEquivFixpointBreak is the can-fail control for
// PROBE-038's wire-equivalence fixpoint leg. The lossy secondMarshal drops
// `units` on the b2 encode only, so the discriminator multiset from the first
// re-marshal still matches the input while b1 and b2 are not wire-equivalent.
// The mutation that turns this red is removing the wireequiv check in
// probe038PolymorphicDecode: then a b2 that is not wire-equivalent to b1 would
// slip through with Status == "pass". The detail match is "not wire-equivalent"
// because the pass detail also says "wire-equivalent".
//
// REQ-052: a check on the encoded form is a wire-equivalence comparison,
// never a byte comparison; this proves that leg can fail.
func TestProbe038GuardCatchesWireEquivFixpointBreak(t *testing.T) {
	body := []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`)
	r, err := probe038PolymorphicDecode(body, func() any { return new(rm.DVQuantity) }, dropMemberReEncoder("units"))
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("status = %q, want fail: dropping units on the second encode must break the wire-equivalence fixpoint", r.Status)
	}
	if !strings.Contains(r.Detail, "not wire-equivalent") {
		t.Fatalf("detail = %q; want the wire-equivalence fixpoint leg to fire after the discriminator multiset passes", r.Detail)
	}
}

// TestProbe038IgnoresTypeInsideAnUnmatchedMember pins that a `_type` inside a
// member the RM does not define is not counted (REQ-052, PROBE-038). The input
// is a vendored polymorphic fixture with two vendor members added in the test,
// one at the root and one inside a nested CODE_PHRASE, each holding a typed
// object. The decoder ignores both, so their discriminators are not part of the
// value and the re-marshal rightly lacks them. Counting the raw input, as the
// probe did before, reports them lost and fails this input: revert
// inputDiscriminators to collectDiscriminators(body) and this test reddens.
func TestProbe038IgnoresTypeInsideAnUnmatchedMember(t *testing.T) {
	raw, err := os.ReadFile(fixtures.RMJSON("polymorphic/name_dv_coded_text"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	doc["vendor_note"] = map[string]any{"_type": "DV_TEXT", "value": "not an RM attribute"}
	language, ok := doc["language"].(map[string]any)
	if !ok {
		t.Fatal("fixture has no language object to plant the nested member in")
	}
	language["vendor_extension"] = map[string]any{"_type": "DV_BOOLEAN", "value": true}
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode planted input: %v", err)
	}
	if got := collectDiscriminators(body); got["DV_BOOLEAN"] != 1 || got["DV_TEXT"] == 0 {
		t.Fatalf("raw input discriminators = %v; the plant must carry DV_BOOLEAN and DV_TEXT", got)
	}
	r, err := Probe038CanjsonRMPolymorphicDecode(body, func() any { return new(rm.Composition) })
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("status = %q (detail: %s), want pass: a _type inside a member the decoder ignores must not be counted", r.Status, r.Detail)
	}
}

// TestProbe038CountsTypesOnDefinedMembers pins the other side of the filter
// (REQ-052, PROBE-038): every `_type` on a member the RM defines is counted, at
// the root, nested and inside a container, as a multiset, and only the one
// inside the unmatched member is left out. An input filter that dropped more
// than the unmatched members would make the discriminator leg vacuous; this
// fails then.
func TestProbe038CountsTypesOnDefinedMembers(t *testing.T) {
	body := []byte(`{"_type":"ELEMENT","archetype_node_id":"at1",` +
		`"name":{"_type":"DV_CODED_TEXT","value":"n","defining_code":{"_type":"CODE_PHRASE","code_string":"1","terminology_id":{"_type":"TERMINOLOGY_ID","value":"local"}}},` +
		`"links":[{"_type":"LINK","meaning":{"_type":"DV_TEXT","value":"m"},"type":{"_type":"DV_TEXT","value":"t"},"target":{"_type":"DV_EHR_URI","value":"ehr://x"}}],` +
		`"value":{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"},` +
		`"vendor":{"_type":"DV_TEXT","value":"ignored"}}`)
	got, err := inputDiscriminators(body, "ELEMENT")
	if err != nil {
		t.Fatalf("inputDiscriminators: %v", err)
	}
	want := typeSet{"ELEMENT": 1, "DV_CODED_TEXT": 1, "CODE_PHRASE": 1, "TERMINOLOGY_ID": 1, "LINK": 1, "DV_TEXT": 2, "DV_EHR_URI": 1, "DV_QUANTITY": 1}
	if !maps.Equal(got, want) {
		t.Errorf("inputDiscriminators = %v, want %v", got, want)
	}
}

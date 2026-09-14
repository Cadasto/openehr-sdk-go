package bmmgen

import (
	"context"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// marshalLeadsWithType reports whether the rendered marshaller chunk emits
// `_type` as the leading member. It handles both wire shapes:
//
//   - flat wire struct: `_type` must be the FIRST json tag in the chunk.
//   - alias: the anonymous wrapper's `_type` field must precede the embedded
//     `*<alias>` (the embed carries no tag, so a tag-order check alone would
//     miss a reordering — this compares the field positions).
//
// aliasEmbed is "*"+aliasTypeName(GoName) for an alias-shape class, or "" for a
// flat-shape class.
func marshalLeadsWithType(chunk, aliasEmbed string) bool {
	tPos := strings.Index(chunk, "`json:\"_type\"`")
	if tPos < 0 {
		return false
	}
	// No other json tag may precede `_type`.
	if first := strings.Index(chunk, "`json:\""); first >= 0 && first < tPos {
		return false
	}
	// For the alias shape, the embedded alias must come after `_type`.
	if aliasEmbed != "" {
		if ePos := strings.Index(chunk, aliasEmbed); ePos >= 0 && ePos < tPos {
			return false
		}
	}
	return true
}

// TestRenderMarshalLeadsWithType pins the REQ-052 SHOULD that `_type` is the
// leading member. `_type` is never a declared BMM property, so a class's fields
// are always "reordered" relative to it; the generator INJECTS it first in the
// anonymous wrapper (alias shape) or as the flat wire struct's first field. The
// census below renders every concrete class and asserts each marshaller leads
// with `_type`, regardless of the class's own field order.
//
// Can-fail control: [marshalLeadsWithType] is proved discriminating on a
// deliberately reordered rendering — one whose `_type` follows the alias embed
// — so the census is not trivially satisfied. Moving the `_type` field after
// the alias in renderMarshalAlias, or after another field in renderMarshalFlat,
// turns the census red.
func TestRenderMarshalLeadsWithType(t *testing.T) {
	// Prove the witness discriminates: an anonymous wrapper whose alias embed
	// precedes `_type` must read as NOT leading with `_type`.
	const reordered = "\t}{\n\t\t*rawX\n\t\tType string `json:\"_type\"`\n"
	if marshalLeadsWithType(reordered, "*rawX") {
		t.Fatal("marshalLeadsWithType accepted a wrapper whose alias embed precedes `_type` — the witness does not discriminate, so the census below proves nothing")
	}

	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	var checked int
	for _, file := range plan.Files {
		for _, pc := range concreteClassesIn(file) {
			chunk, err := renderMarshalJSON(plan, pc)
			if err != nil {
				t.Fatalf("renderMarshalJSON %s: %v", pc.BMMName, err)
			}
			aliasEmbed := ""
			if !embedsMarshalerBearingConcrete(plan, pc) {
				aliasEmbed = "*" + aliasTypeName(pc.GoName)
			}
			if !marshalLeadsWithType(chunk, aliasEmbed) {
				t.Errorf("%s marshaller does not lead with `_type` — the discriminator must be the first member (REQ-052)", pc.BMMName)
			}
			checked++
		}
	}
	if checked < 100 {
		t.Errorf("census rendered only %d concrete classes; expected the full RM inventory", checked)
	}
}

// TestRenderFlatDecodeDropPropertyLosesCopy is the generator-side can-fail
// control for TestRoundTripStructuralEquivalence
// (openehr/serialize/canjson/roundtrip_test.go): a flat-shape decode copies
// every wire field back to the receiver, so dropping one property from the plan
// drops its copy — the field would then be silently lost on the round trip.
//
// DV_CODED_TEXT is flat-shape (it embeds the marshaler-bearing DVText, so it
// cannot use the zero-copy alias, ruling R19), which is exactly the shape whose
// per-field copy a dropped assignment would break.
func TestRenderFlatDecodeDropPropertyLosesCopy(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	pc, ok := plan.Classes["DV_CODED_TEXT"]
	if !ok {
		t.Fatal("DV_CODED_TEXT not in plan")
	}
	if !embedsMarshalerBearingConcrete(plan, pc) {
		t.Fatal("DV_CODED_TEXT is not flat-shape; this control needs a class whose decode copies fields")
	}
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		t.Fatalf("effectiveFields: %v", err)
	}
	if len(fields) < 2 {
		t.Fatalf("DV_CODED_TEXT has %d effective fields; need at least 2 to drop one", len(fields))
	}
	dropped := fields[len(fields)-1]
	droppedCopy := "d." + FieldName(dropped.Prop.PropertyName()) + " = wire." + FieldName(dropped.Prop.PropertyName())

	full, err := renderUnmarshalJSON(plan, pc, fields)
	if err != nil {
		t.Fatalf("renderUnmarshalJSON (full): %v", err)
	}
	if !strings.Contains(full, droppedCopy) {
		t.Fatalf("baseline decode does not copy %q; the control cannot demonstrate the loss", droppedCopy)
	}

	trimmed, err := renderUnmarshalJSON(plan, pc, fields[:len(fields)-1])
	if err != nil {
		t.Fatalf("renderUnmarshalJSON (dropped): %v", err)
	}
	if strings.Contains(trimmed, droppedCopy) {
		t.Errorf("dropping %s from the plan still emitted its copy %q — a lost property would go unnoticed on the round trip", dropped.Prop.PropertyName(), droppedCopy)
	}
}

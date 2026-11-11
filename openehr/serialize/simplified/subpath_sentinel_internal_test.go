package simplified

// REQ-170 — the refusals of a key that cannot be placed inside an underscore
// family's grammar wrap ErrUnknownPath, as the ones on a Web Template path do:
// two spellings of one sub-path, and a list attribute something else already
// holds. These reach the branches a decode's own groups never take (a group
// that defers no pair of spellings), so they are driven directly.

import (
	"errors"
	"testing"
)

// TestREQ170SubPathCollisionRefusedAsPlacement — REQ-170. A group that defers
// no pair of spellings to the decode's judgement refuses two spellings of one
// sub-path at once, in a value family's grammar and in FEEDER_AUDIT's, with
// ErrUnknownPath, naming the key.
func TestREQ170SubPathCollisionRefusedAsPlacement(t *testing.T) {
	value := rmattrGroup{
		base: "r/o", family: "_normal_range", index: -1,
		tails: map[string]any{"/lower|magnitude": 1.0, "/lower:0|magnitude": 2.0},
	}
	if _, err := splitRMAttrTails(value, intervalSubTails, nil); !errors.Is(err, ErrUnknownPath) {
		t.Errorf("splitRMAttrTails = %v, want ErrUnknownPath", err)
	}
	feeder := rmattrGroup{
		base: "r/o", family: "_feeder_audit", index: -1,
		tails: map[string]any{"/originating_system_audit|system_id": "a", "/originating_system_audit:0|system_id": "b"},
	}
	if _, _, err := rmattrChildGroups(feeder, feederAuditSegs); !errors.Is(err, ErrUnknownPath) {
		t.Errorf("rmattrChildGroups = %v, want ErrUnknownPath", err)
	}
}

// TestREQ170ListAttributeConflictRefusedAsPlacement — REQ-170. A list family
// whose attribute another key already holds as a single value is a slot
// conflict between two keys: ErrUnknownPath.
func TestREQ170ListAttributeConflictRefusedAsPlacement(t *testing.T) {
	node := map[string]any{"links": map[string]any{"_type": "LINK"}}
	g := rmattrGroup{base: "r/o", family: "_link", index: 0}
	if err := placeRMAttrList(node, "links", g, map[string]any{"_type": "LINK"}, &allocBudget{limit: maxTotalNodes}); !errors.Is(err, ErrUnknownPath) {
		t.Errorf("placeRMAttrList = %v, want ErrUnknownPath", err)
	}
}

package bmmgen

import (
	"context"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestMarshalJSONFieldOrderBMMArder asserts canonical JSON wire structs
// list flattened properties in BMM declaration order, not alphabetically.
// HISTORY is used because its own properties are not alphabetical
// (origin before duration before events).
func TestMarshalJSONFieldOrderBMMArder(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	pc, ok := plan.Classes["HISTORY"]
	if !ok {
		t.Fatal("HISTORY not in plan")
	}
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		t.Fatalf("effectiveFields: %v", err)
	}
	var names []string
	for _, f := range fields {
		names = append(names, f.Prop.PropertyName())
	}
	// HISTORY own props in BMM order (after embedded LOCATABLE/... fields).
	wantTail := []string{"origin", "period", "duration", "summary", "events"}
	if len(names) < len(wantTail) {
		t.Fatalf("got %d fields, want at least %d: %v", len(names), len(wantTail), names)
	}
	gotTail := names[len(names)-len(wantTail):]
	for i, w := range wantTail {
		if gotTail[i] != w {
			t.Fatalf("HISTORY tail field order: got %v, want %v (full %v)", gotTail, wantTail, names)
		}
	}
}

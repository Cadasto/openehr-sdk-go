package bmmgen

import (
	"context"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestPolymorphicPropertyEmittingClassNarrowing pins the REQ-052
// Phase 1 fix: when an inherited open generic parameter (e.g.
// `Interval[T Ordered].lower: T`) is rendered for an emitting class
// that narrows the bound to an abstract Go type (e.g. DVInterval
// narrows T to DV_ORDERED), polymorphicProperty MUST classify the
// property as polySingle and route the field through typereg dispatch
// — even though the original DECLARING class's bound (Ordered, in
// BASE) is not classified as abstract in the plan.
//
// Before the Phase 1 fix the emitting class's narrowed bound was
// ignored; the helper consulted only the declaring class's bound and
// returned polyNone, leaving `Lower T` typed concretely. Go's
// encoding/json then failed at runtime whenever T was instantiated
// with an interface (DVInterval[DVOrdered] used by every
// ReferenceRange / NormalRange in the RM).
func TestPolymorphicPropertyEmittingClassNarrowing(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	dvInterval, ok := plan.Classes["DV_INTERVAL"]
	if !ok {
		t.Fatal("DV_INTERVAL not in plan")
	}
	dvIntervalSC, ok := dvInterval.Class.(*bmm.SimpleClass)
	if !ok {
		t.Fatalf("DV_INTERVAL is %T, want *bmm.SimpleClass", dvInterval.Class)
	}
	intervalPC, ok := plan.Classes["Interval"]
	if !ok {
		t.Fatal("Interval (BASE) not in plan")
	}
	intervalSC, ok := intervalPC.Class.(*bmm.SimpleClass)
	if !ok {
		t.Fatalf("Interval is %T, want *bmm.SimpleClass", intervalPC.Class)
	}
	// Interval.lower is declared as `T` (P_BMM_SINGLE_PROPERTY_OPEN
	// constrained to BASE's `Ordered`). DV_INTERVAL inherits it and
	// narrows T to DV_ORDERED (abstract).
	lower, ok := intervalSC.Properties["lower"]
	if !ok {
		t.Fatal("Interval.lower property not found")
	}
	if _, openProp := lower.(*bmm.SinglePropertyOpen); !openProp {
		t.Fatalf("Interval.lower is %T, want *bmm.SinglePropertyOpen", lower)
	}
	// Owner = declaring class (Interval). Emitting = narrowing class (DV_INTERVAL).
	iface, kind := polymorphicProperty(plan, intervalSC, dvIntervalSC, lower)
	if kind != polySingle {
		t.Errorf("kind = %v, want polySingle (emitting class's narrowed T bound resolves to abstract DV_ORDERED)", kind)
	}
	if iface != "T" {
		t.Errorf("iface = %q, want %q (the type-parameter name)", iface, "T")
	}
	// Sanity: WITHOUT the emitting-class narrowing (passing only the
	// declaring class as emitting), we fall back to the declaring
	// class's bound (Ordered, not abstract in the plan), and the
	// helper SHOULD return polyNone. Confirms the new code path is
	// the one fixing the gap, not an unrelated change.
	if _, kindNoNarrowing := polymorphicProperty(plan, intervalSC, intervalSC, lower); kindNoNarrowing != polyNone {
		t.Errorf("without emitting-class narrowing, kind = %v, want polyNone (BASE Ordered is not an abstract Go type in the plan)", kindNoNarrowing)
	}
}

// TestPolymorphicPropertyRendersTyperegDispatch is the integration
// half: regenerate DV_INTERVAL's JSON unmarshaller and confirm the
// rendered source contains the typereg.DecodeAs[T] dispatch line for
// both `lower` and `upper` (instead of the pre-Phase-1 direct
// `d.Lower = aux.Lower` assignment).
func TestPolymorphicPropertyRendersTyperegDispatch(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	var file *PlannedFile
	for _, f := range plan.Files {
		if f.FileBase == "data_types_quantity" {
			file = f
			break
		}
	}
	if file == nil {
		t.Fatal("data_types_quantity file not in plan")
	}
	got, err := RenderUnmarshalJSONFile(plan, file)
	if err != nil {
		t.Fatalf("RenderUnmarshalJSONFile: %v", err)
	}
	src := string(got)
	for _, want := range []string{
		`Lower json.RawMessage`,
		`Upper json.RawMessage`,
		`typereg.DecodeAs[T](aux.Lower)`,
		`typereg.DecodeAs[T](aux.Upper)`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("rendered DVInterval unmarshaller missing %q", want)
		}
	}
	for _, banned := range []string{
		"d.Lower = aux.Lower",
		"d.Upper = aux.Upper",
	} {
		if strings.Contains(src, banned) {
			t.Errorf("rendered DVInterval unmarshaller still contains pre-fix line %q", banned)
		}
	}
}

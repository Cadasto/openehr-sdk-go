package bmmgen

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestIntervalShapedCensus pins which generated classes carry the BASE
// Interval shape (`lower`, `upper`, `lower_unbounded`, `upper_unbounded`) and
// so get encoders that leave out an open side's empty bound (REQ-052,
// REQ-056). The shape is read from the codec fields, not from the ancestry,
// so a class that declares the four members itself is caught too. A new
// interval class in the pinned schemas changes this list and is covered
// without further work.
func TestIntervalShapedCensus(t *testing.T) {
	for _, tc := range []struct {
		target Target
		want   []string
	}{
		{target: TargetRM, want: []string{"DV_INTERVAL", "Point_interval", "Proper_interval"}},
		{target: TargetAOM14, want: nil},
	} {
		t.Run(tc.target.RootID, func(t *testing.T) {
			plan, err := BuildPlanForTarget(context.Background(), tc.target, bmm.FSResolver{Root: testResources})
			if err != nil {
				t.Fatalf("BuildPlanForTarget(%s): %v", tc.target.RootID, err)
			}
			var got []string
			for _, f := range plan.Files {
				for _, pc := range concreteClassesIn(f) {
					shaped, err := intervalShaped(plan, pc)
					if err != nil {
						t.Fatalf("intervalShaped(%s): %v", pc.BMMName, err)
					}
					if shaped {
						got = append(got, pc.BMMName)
					}
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("interval-shaped classes = %q, want %q", got, tc.want)
			}

			body, err := RenderIntervalBoundFile(plan)
			if err != nil {
				t.Fatalf("RenderIntervalBoundFile(%s): %v", tc.target.RootID, err)
			}
			if gotFile, wantFile := body != nil, tc.want != nil; gotFile != wantFile {
				t.Errorf("RenderIntervalBoundFile(%s) emits a file = %v, want %v", tc.target.RootID, gotFile, wantFile)
			}
		})
	}
}

// TestIntervalBoundClasses pins the concrete bound types the emptiness test
// switches over (REQ-052): the concrete descendants of DV_ORDERED, the bound
// of DV_INTERVAL's type parameter. BASE Interval's bound, the primitive
// Ordered, is covered by the scalar case instead.
func TestIntervalBoundClasses(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	classes, err := intervalBoundClasses(plan)
	if err != nil {
		t.Fatalf("intervalBoundClasses: %v", err)
	}
	var got []string
	for _, pc := range classes {
		got = append(got, pc.BMMName)
	}
	want := []string{
		"DV_COUNT", "DV_DATE", "DV_DATE_TIME", "DV_DURATION", "DV_ORDINAL",
		"DV_PROPORTION", "DV_QUANTITY", "DV_SCALE", "DV_TIME",
	}
	if !slices.Equal(got, want) {
		t.Errorf("interval bound classes = %q, want %q", got, want)
	}
}

// TestZeroPredicateRefusals pins the generator's refusal to derive a zero
// predicate it cannot spell field by field: a generic class, whose fields
// depend on its type argument, and a class another target owns. Emitting
// nothing for either would let the emptiness test fall back to "never empty"
// without a word.
func TestZeroPredicateRefusals(t *testing.T) {
	rmPlan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	aomPlan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}
	for _, tc := range []struct {
		name  string
		plan  *Plan
		class string
	}{
		{name: "generic class", plan: rmPlan, class: "DV_INTERVAL"},
		{name: "class of another target", plan: aomPlan, class: "TERMINOLOGY_ID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pc, ok := tc.plan.Classes[tc.class]
			if !ok {
				t.Fatalf("%s not in plan", tc.class)
			}
			if tc.plan == aomPlan && !pc.External {
				t.Fatalf("%s is owned by the AOM 1.4 target, want a class of another target", tc.class)
			}
			if src, _, err := renderZeroPredicate(tc.plan, pc); err == nil {
				t.Errorf("renderZeroPredicate(%s) succeeded, want a refusal:\n%s", tc.class, src)
			}
		})
	}
}

// TestIntervalBoundFileWithoutBoundClasses pins that the emptiness test is
// emitted whenever the target owns an interval-shaped class (REQ-052,
// REQ-056), even when no bound type needs a zero predicate. Without
// DV_INTERVAL the remaining interval classes bound their parameter by the
// primitive Ordered, which the scalar case covers; their encoders still call
// omitIntervalBound, so a missing file would not compile.
func TestIntervalBoundFileWithoutBoundClasses(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	for _, f := range plan.Files {
		f.Classes = slices.DeleteFunc(f.Classes, func(pc *PlannedClass) bool { return pc.BMMName == "DV_INTERVAL" })
	}
	body, err := RenderIntervalBoundFile(plan)
	if err != nil {
		t.Fatalf("RenderIntervalBoundFile without DV_INTERVAL: %v", err)
	}
	if body == nil {
		t.Fatal("RenderIntervalBoundFile without DV_INTERVAL emits no file, but Proper_interval and Point_interval still call omitIntervalBound")
	}
	for _, want := range []string{"func omitIntervalBound[", "func isEmptyIntervalBound["} {
		if !strings.Contains(string(body), want) {
			t.Errorf("interval bound file without DV_INTERVAL lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(string(body), "func isZeroDVQuantity(") {
		t.Errorf("interval bound file without DV_INTERVAL still carries the DV_ORDERED predicates:\n%s", body)
	}
}

// TestIntervalBoundFromFieldType pins that the bound types are read from the
// lower / upper fields (REQ-052): a bound declared with a concrete class, not
// a generic parameter, gets that class's zero predicate. The test retypes
// BASE Interval's bounds as DV_BOOLEAN, which no generic parameter names.
func TestIntervalBoundFromFieldType(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	iv, ok := plan.Classes["Interval"].Class.(*bmm.SimpleClass)
	if !ok {
		t.Fatal("Interval is not a simple class in the plan")
	}
	for _, name := range []string{"lower", "upper"} {
		p := &bmm.SingleProperty{TypeName: "DV_BOOLEAN"}
		p.Name = name
		p.IsMandatory = true
		iv.Properties[name] = p
	}
	body, err := RenderIntervalBoundFile(plan)
	if err != nil {
		t.Fatalf("RenderIntervalBoundFile: %v", err)
	}
	for _, want := range []string{"case DVBoolean:", "func isZeroDVBoolean("} {
		if !strings.Contains(string(body), want) {
			t.Errorf("interval bound file with DV_BOOLEAN bounds lacks %q:\n%s", want, body)
		}
	}
}

// TestIntervalBoundRefusesUnhandledBound pins the generator's refusal of an
// interval class whose bound type has neither a zero predicate nor a scalar
// case (REQ-052): emitting the file anyway would leave that bound "never
// empty" without a word.
func TestIntervalBoundRefusesUnhandledBound(t *testing.T) {
	for _, bound := range []string{"NO_SUCH_TYPE", "REFERENCE_RANGE"} {
		t.Run(bound, func(t *testing.T) {
			plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
			if err != nil {
				t.Fatalf("BuildPlanForTarget(RM): %v", err)
			}
			proper, ok := plan.Classes["Proper_interval"].Class.(*bmm.SimpleClass)
			if !ok {
				t.Fatal("Proper_interval is not a simple class in the plan")
			}
			proper.GenericParameterDefs["T"].ConformsToType = bound
			if body, err := RenderIntervalBoundFile(plan); err == nil {
				t.Errorf("RenderIntervalBoundFile with Proper_interval<T: %s> succeeded, want a refusal:\n%s", bound, body)
			}
		})
	}
}

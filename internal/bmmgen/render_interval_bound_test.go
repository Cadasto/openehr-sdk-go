package bmmgen

import (
	"context"
	"slices"
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

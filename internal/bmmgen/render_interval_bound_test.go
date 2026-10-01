package bmmgen

import (
	"context"
	"maps"
	"regexp"
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
	shaped, classes, err := intervalBounds(plan)
	if err != nil {
		t.Fatalf("intervalBounds: %v", err)
	}
	if !shaped {
		t.Fatal("intervalBounds reports no interval-shaped class in the RM plan")
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

// intervalSwitchArms splits the rendered MarshalJSONTo of an interval class
// into the arms of its open-side switch, keyed by the case label
// ("omitLower && omitUpper", "omitLower", "omitUpper") with the arm's body as
// the value. The `return` after the switch is keyed by "".
func intervalSwitchArms(t *testing.T, src string) map[string]string {
	t.Helper()
	_, afterSwitch, ok := strings.Cut(src, "\tswitch {\n")
	if !ok {
		t.Fatalf("rendered MarshalJSONTo has no switch:\n%s", src)
	}
	body, tail, ok := strings.Cut(afterSwitch, "\n\t}\n")
	if !ok {
		t.Fatalf("rendered MarshalJSONTo switch is not closed:\n%s", src)
	}
	arms := map[string]string{"": tail}
	var label string
	for line := range strings.Lines(body + "\n") {
		if rest, isCase := strings.CutPrefix(line, "\tcase "); isCase {
			label = strings.TrimSuffix(strings.TrimSpace(rest), ":")
			if _, dup := arms[label]; dup {
				t.Fatalf("rendered MarshalJSONTo has two arms for %q:\n%s", label, src)
			}
			arms[label] = ""
			continue
		}
		arms[label] += line
	}
	return arms
}

// TestIntervalMarshalJSONOpenSideArms pins the switch the generator renders
// into the canonical JSON marshaller of every interval-shaped class (REQ-052,
// REQ-056): one arm for each open-side combination, each declaring a
// zero-size `omitzero` field for exactly the bound members it leaves out, and
// a plain wire after the switch. Without this test, removing an arm from the
// generator leaves that combination writing its empty bound and only
// `make codegen-verify` notices, as a diff.
func TestIntervalMarshalJSONOpenSideArms(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	const (
		lowerField = "Lower struct{} `json:\"lower,omitzero\"`"
		upperField = "Upper struct{} `json:\"upper,omitzero\"`"
	)
	for _, class := range []string{"Point_interval", "Proper_interval", "DV_INTERVAL"} {
		t.Run(class, func(t *testing.T) {
			pc, ok := plan.Classes[class]
			if !ok {
				t.Fatalf("%s not in the RM plan", class)
			}
			src, err := renderMarshalJSON(plan, pc)
			if err != nil {
				t.Fatalf("renderMarshalJSON(%s): %v", class, err)
			}
			recv := jsonmarReceiverName(pc.GoName)
			for _, want := range []string{
				"omitLower := omitIntervalBound(" + recv + ".LowerUnbounded, " + recv + ".Lower)",
				"omitUpper := omitIntervalBound(" + recv + ".UpperUnbounded, " + recv + ".Upper)",
			} {
				if !strings.Contains(src, want) {
					t.Errorf("MarshalJSONTo of %s lacks %q:\n%s", class, want, src)
				}
			}

			arms := intervalSwitchArms(t, src)
			if got, want := slices.Sorted(maps.Keys(arms)), []string{"", "omitLower", "omitLower && omitUpper", "omitUpper"}; !slices.Equal(got, want) {
				t.Errorf("MarshalJSONTo of %s has arms %q, want %q", class, got, want)
			}
			for _, tc := range []struct {
				label                string
				wantLower, wantUpper bool
			}{
				{label: "omitLower && omitUpper", wantLower: true, wantUpper: true},
				{label: "omitLower", wantLower: true},
				{label: "omitUpper", wantUpper: true},
				{label: ""},
			} {
				body, ok := arms[tc.label]
				if !ok {
					continue
				}
				if !strings.Contains(body, "return json.MarshalEncode(enc, &struct {") {
					t.Errorf("arm %q of %s does not return the wrapper:\n%s", tc.label, class, body)
				}
				if got := strings.Contains(body, lowerField); got != tc.wantLower {
					t.Errorf("arm %q of %s declares the omitted lower field = %v, want %v:\n%s", tc.label, class, got, tc.wantLower, body)
				}
				if got := strings.Contains(body, upperField); got != tc.wantUpper {
					t.Errorf("arm %q of %s declares the omitted upper field = %v, want %v:\n%s", tc.label, class, got, tc.wantUpper, body)
				}
			}
		})
	}
}

// TestIntervalMarshalXMLOpenSideGuards pins the XML half of the same rule
// (REQ-052, REQ-056): the generated canonical XML marshaller of an
// interval-shaped class wraps the `lower` element in the guard that reads the
// lower open flag, and the `upper` element in the one that reads the upper
// flag.
func TestIntervalMarshalXMLOpenSideGuards(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	var file *PlannedFile
	for _, f := range plan.Files {
		if f.FileBase == "foundation_types_interval" {
			file = f
		}
	}
	if file == nil {
		t.Fatal("foundation_types_interval file not in the RM plan")
	}
	body, err := RenderMarshalXMLFile(plan, file)
	if err != nil {
		t.Fatalf("RenderMarshalXMLFile: %v", err)
	}
	src := string(body)
	for _, tc := range []struct{ name, re string }{
		{"lower", `\tif !omitIntervalBound\(p\.LowerUnbounded, p\.Lower\) \{\n\t\tif err := _e\.EncodeElement\(&p\.Lower, `},
		{"upper", `\tif !omitIntervalBound\(p\.UpperUnbounded, p\.Upper\) \{\n\t\tif err := _e\.EncodeElement\(&p\.Upper, `},
	} {
		// Point_interval and Proper_interval each carry one.
		if got := len(regexp.MustCompile(tc.re).FindAllString(src, -1)); got != 2 {
			t.Errorf("foundation_types_interval XML marshallers guard the %s element %d times, want 2:\n%s", tc.name, got, src)
		}
	}
}

// TestGuardOpenIntervalBoundXML pins the guard's mapping from a bound to its
// own open flag, and that any other property passes through unchanged
// (REQ-052).
func TestGuardOpenIntervalBoundXML(t *testing.T) {
	const lines = "\tif err := x(); err != nil {\n\t\treturn err\n\t}\n"
	const wrapped = "\t\tif err := x(); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n"
	for _, tc := range []struct{ prop, want string }{
		{"lower", "\tif !omitIntervalBound(p.LowerUnbounded, p.Lower) {\n" + wrapped},
		{"upper", "\tif !omitIntervalBound(p.UpperUnbounded, p.Upper) {\n" + wrapped},
		{"lower_unbounded", lines},
		{"upper_unbounded", lines},
		{"lower_included", lines},
	} {
		if got := guardOpenIntervalBoundXML("p", tc.prop, lines); got != tc.want {
			t.Errorf("guardOpenIntervalBoundXML(%q) = %q, want %q", tc.prop, got, tc.want)
		}
	}
}

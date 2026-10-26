package bmmgen

import (
	"context"
	"fmt"
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
// REQ-056): the concrete interval classes and the abstract BASE Interval
// itself, which AOM 1.4 holds by value. The shape is read from the codec
// fields, not from the ancestry, so a class that declares the four members
// itself is caught too. A new interval class in the pinned schemas changes
// this list and is covered without further work.
func TestIntervalShapedCensus(t *testing.T) {
	for _, tc := range []struct {
		target Target
		want   []string
	}{
		{target: TargetRM, want: []string{"DV_INTERVAL", "Interval", "Point_interval", "Proper_interval"}},
		{target: TargetAOM14, want: nil},
	} {
		t.Run(tc.target.RootID, func(t *testing.T) {
			plan, err := BuildPlanForTarget(context.Background(), tc.target, bmm.FSResolver{Root: testResources})
			if err != nil {
				t.Fatalf("BuildPlanForTarget(%s): %v", tc.target.RootID, err)
			}
			var got []string
			for _, f := range plan.Files {
				classes, err := codecClassesIn(plan, f)
				if err != nil {
					t.Fatalf("codecClassesIn(%s): %v", f.FileBase, err)
				}
				for _, pc := range classes {
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
	for _, want := range []string{"func omitIntervalBound[", "func IsEmptyIntervalBound["} {
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
// `make codegen-check` notices, as a diff.
func TestIntervalMarshalJSONOpenSideArms(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	const (
		lowerField = "Lower struct{} `json:\"lower,omitzero\"`"
		upperField = "Upper struct{} `json:\"upper,omitzero\"`"
	)
	for _, class := range []string{"Interval", "Point_interval", "Proper_interval", "DV_INTERVAL"} {
		t.Run(class, func(t *testing.T) {
			pc, ok := plan.Classes[class]
			if !ok {
				t.Fatalf("%s not in the RM plan", class)
			}
			src, err := renderMarshalJSON(plan, pc)
			if err != nil {
				t.Fatalf("renderMarshalJSON(%s): %v", class, err)
			}
			if got, want := strings.Contains(src, "`json:\"_type\"`"), class != "Interval"; got != want {
				t.Errorf("MarshalJSONTo of %s writes `_type` = %v, want %v: BASE Interval is abstract and carries none:\n%s", class, got, want, src)
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
				if !strings.Contains(body, "return json.MarshalEncode(enc, ") {
					t.Errorf("arm %q of %s does not return the wire value:\n%s", tc.label, class, body)
				}
				if tc.wantLower || tc.wantUpper {
					if !strings.Contains(body, "return json.MarshalEncode(enc, &struct {") {
						t.Errorf("arm %q of %s does not return a wrapper that can drop a member:\n%s", tc.label, class, body)
					}
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
		{"lower", `\tif !omitIntervalBound\([pi]\.LowerUnbounded, [pi]\.Lower\) \{\n\t\tif err := _e\.EncodeElement\(&[pi]\.Lower, `},
		{"upper", `\tif !omitIntervalBound\([pi]\.UpperUnbounded, [pi]\.Upper\) \{\n\t\tif err := _e\.EncodeElement\(&[pi]\.Upper, `},
	} {
		// Interval, Point_interval and Proper_interval each carry one.
		if got := len(regexp.MustCompile(tc.re).FindAllString(src, -1)); got != 3 {
			t.Errorf("foundation_types_interval XML marshallers guard the %s element %d times, want 3:\n%s", tc.name, got, src)
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

// TestIntervalBaseCodecClasses pins which abstract classes get a codec beside
// the concrete ones (REQ-052, REQ-056): BASE Interval, the abstract generic
// struct that carries the interval shape, and nothing else. EVENT is abstract
// and generic too, but it is emitted as an interface, and a target that does
// not own Interval (AOM 1.4 reaches it through rm) emits no codec for it.
func TestIntervalBaseCodecClasses(t *testing.T) {
	for _, tc := range []struct {
		target Target
		want   []string
	}{
		{target: TargetRM, want: []string{"Interval"}},
		{target: TargetAOM14, want: nil},
	} {
		t.Run(tc.target.RootID, func(t *testing.T) {
			plan, err := BuildPlanForTarget(context.Background(), tc.target, bmm.FSResolver{Root: testResources})
			if err != nil {
				t.Fatalf("BuildPlanForTarget(%s): %v", tc.target.RootID, err)
			}
			var got []string
			for _, f := range plan.Files {
				classes, err := codecClassesIn(plan, f)
				if err != nil {
					t.Fatalf("codecClassesIn(%s): %v", f.FileBase, err)
				}
				for _, pc := range classes {
					if sc := pc.Class.(*bmm.SimpleClass); sc.IsAbstract() {
						got = append(got, pc.BMMName)
					}
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("abstract classes with a codec = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestIntervalBaseHasNoDiscriminatorMethods pins that the abstract BASE
// Interval gets MarshalXML and UnmarshalXML but no BMMName: it is no
// polymorphic discriminator value, and a BMMName on it would be promoted into
// every class that embeds it.
func TestIntervalBaseHasNoDiscriminatorMethods(t *testing.T) {
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
	marshal, err := RenderMarshalXMLFile(plan, file)
	if err != nil {
		t.Fatalf("RenderMarshalXMLFile: %v", err)
	}
	unmarshal, err := RenderUnmarshalXMLFile(plan, file)
	if err != nil {
		t.Fatalf("RenderUnmarshalXMLFile: %v", err)
	}
	if !strings.Contains(string(marshal), "func (i *Interval[T]) MarshalXML(") {
		t.Errorf("no MarshalXML on Interval:\n%s", marshal)
	}
	if !strings.Contains(string(unmarshal), "func (i *Interval[T]) UnmarshalXML(") {
		t.Errorf("no UnmarshalXML on Interval:\n%s", unmarshal)
	}
	if strings.Contains(string(marshal), "func (i *Interval[T]) BMMName(") {
		t.Errorf("BASE Interval got a BMMName, which every embedding class would promote:\n%s", marshal)
	}
}

// TestIntervalDerivedWireEmbedsMethodFreeBase pins the wire struct of every
// class that embeds BASE Interval (REQ-052, REQ-056). BASE Interval now has a
// MarshalJSONTo, and a method-free alias of the embedding class would promote
// it: encoding/json/v2 would then encode the wrapper through it, silently
// emitting the base's payload without `_type` or the open-side omission, and
// fail to decode into it with "must not implement marshal or unmarshal
// methods". The wire struct must reach
// the base through the base's own alias, and no embedding class may keep a
// `type rawX X` alias of itself.
func TestIntervalDerivedWireEmbedsMethodFreeBase(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	for _, class := range []string{"Point_interval", "Proper_interval", "DV_INTERVAL"} {
		t.Run(class, func(t *testing.T) {
			pc := plan.Classes[class]
			base, err := embeddedIntervalBase(plan, pc)
			if err != nil || base == nil {
				t.Fatalf("embeddedIntervalBase(%s) = %v, %v; want BASE Interval", class, base, err)
			}
			src, err := renderMarshalJSON(plan, pc)
			if err != nil {
				t.Fatalf("renderMarshalJSON(%s): %v", class, err)
			}
			if want := "\t*" + aliasTypeName(base.GoName) + "[T]\n"; !strings.Contains(src, want) {
				t.Errorf("wire struct of %s does not embed the base's method-free alias %q:\n%s", class, want, src)
			}
			if banned := "type " + aliasTypeName(pc.GoName) + "["; strings.Contains(src, banned) {
				t.Errorf("%s still declares %q: that alias would promote BASE Interval's MarshalJSONTo:\n%s", class, banned, src)
			}
		})
	}
}

// TestIntervalBoundFileExportsEmptinessTest pins the public shape of the
// emptiness test (REQ-052): the generated file exports it as
// IsEmptyIntervalBound, keeps the encoders' unexported omitIntervalBound
// wrapper over it, and documents it for SDK users, as a sentence starting with
// the name and free of requirement identifiers (AGENTS.md).
func TestIntervalBoundFileExportsEmptinessTest(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	body, err := RenderIntervalBoundFile(plan)
	if err != nil {
		t.Fatalf("RenderIntervalBoundFile: %v", err)
	}
	src := string(body)
	if strings.Contains(src, "func isEmptyIntervalBound[") {
		t.Errorf("the emptiness test is still unexported:\n%s", src)
	}
	if !strings.Contains(src, "\treturn unbounded && IsEmptyIntervalBound(bound)\n") {
		t.Errorf("omitIntervalBound does not delegate to IsEmptyIntervalBound:\n%s", src)
	}
	doc, _, ok := strings.Cut(src, "func IsEmptyIntervalBound[")
	if !ok {
		t.Fatalf("no exported IsEmptyIntervalBound in the interval bound file:\n%s", src)
	}
	doc = doc[strings.LastIndex(doc, "\n\n")+2:]
	if !strings.HasPrefix(doc, "// IsEmptyIntervalBound reports ") {
		t.Errorf("IsEmptyIntervalBound doc comment does not start with its name:\n%s", doc)
	}
	if regexp.MustCompile(`REQ-\d+|PROBE-\d+`).MatchString(doc) {
		t.Errorf("IsEmptyIntervalBound doc comment carries a requirement identifier:\n%s", doc)
	}
}

// TestIntervalBaseReadsLegacyXMLNames pins that the BASE Interval XML decoder
// also reads the Go field names SDK v0.28.0 and earlier wrote (REQ-056), and
// that no other interval class does: their encoders always wrote snake_case.
func TestIntervalBaseReadsLegacyXMLNames(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	for _, tc := range []struct {
		class      string
		wantLegacy bool
	}{
		{"Interval", true},
		{"Point_interval", false},
		{"Proper_interval", false},
		{"DV_INTERVAL", false},
	} {
		t.Run(tc.class, func(t *testing.T) {
			pc := plan.Classes[tc.class]
			fields, err := effectiveFields(plan, pc)
			if err != nil {
				t.Fatalf("effectiveFields: %v", err)
			}
			src, err := renderUnmarshalXML(plan, pc, fields)
			if err != nil {
				t.Fatalf("renderUnmarshalXML: %v", err)
			}
			for _, prop := range []string{"lower", "upper", "lower_unbounded", "upper_unbounded", "lower_included", "upper_included"} {
				want := fmt.Sprintf("case %q, %q:", prop, FieldName(prop))
				if got := strings.Contains(src, want); got != tc.wantLegacy {
					t.Errorf("UnmarshalXML of %s reads the legacy name with %q = %v, want %v", tc.class, want, got, tc.wantLegacy)
				}
				if !strings.Contains(src, fmt.Sprintf("case %q", prop)) {
					t.Errorf("UnmarshalXML of %s lost the canonical case for %q", tc.class, prop)
				}
			}
		})
	}
}

// TestREQ169IntervalDecodeReadsFlagsInPlace pins the generator side of the
// flag defaults (REQ-169) and of the failed-decode state (REQ-052) on
// Point_interval, whose own members are the four flags. Its decoder reads each
// flag straight into the receiver's own field through an intervalFlagJSON, so
// nothing is copied back from the wire, and it gives the absent flags their
// defaults only after a decode that succeeds: a failed decode returns first
// and leaves what was read, as an in-place decode does.
func TestREQ169IntervalDecodeReadsFlagsInPlace(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	pc := plan.Classes["Point_interval"]
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		t.Fatalf("effectiveFields: %v", err)
	}
	src, err := renderUnmarshalJSON(plan, pc, fields)
	if err != nil {
		t.Fatalf("renderUnmarshalJSON: %v", err)
	}
	for _, prop := range intervalFlagProps {
		fn := FieldName(prop)
		want := fmt.Sprintf("%s: intervalFlagJSON{field: &p.%s, seen: &seen.%s}", fn, fn, seenFlagField(prop))
		if !strings.Contains(src, want) {
			t.Errorf("Point_interval decoder does not read %s in place, want %q:\n%s", prop, want, src)
		}
		if copyBack := "p." + fn + " = wire." + fn; strings.Contains(src, copyBack) {
			t.Errorf("Point_interval decoder still copies %s back from the wire (%q):\n%s", prop, copyBack, src)
		}
	}
	// The error branch that follows the decode; the null branch before it
	// returns too, so the search starts at the decode.
	decode := strings.Index(src, "typereg.DecodeInto(")
	ret := indexFrom(src, "\t\treturn err\n", decode)
	defaults := strings.Index(src, intervalFlagSettle("seen", "p"))
	if decode < 0 || ret < 0 || defaults < 0 || ret >= defaults {
		t.Errorf("Point_interval decoder does not return a decode error before it gives the defaults (positions %d, %d, %d):\n%s", decode, ret, defaults, src)
	}
}

// TestREQ169IntervalFlagDecodeCopiesOwnMembersBeforeReturning pins the order
// the shared flag decoder keeps for a class with own members besides the
// flags (REQ-052, REQ-169): it copies them back whether or not the decode
// fails, then returns a decode error, and only then gives the absent flags
// their defaults. No pinned class has such members today, so the renderer is
// driven directly.
func TestREQ169IntervalFlagDecodeCopiesOwnMembersBeforeReturning(t *testing.T) {
	var b strings.Builder
	renderIntervalFlagJSONDecode(&b, "x", "X_INTERVAL", "*jsonWireXInterval[T]", "&jsonWireXInterval[T]{}", "", "&wire.Type", []string{"Extra"})
	src := b.String()
	decode := strings.Index(src, "err := typereg.DecodeInto(dec, \"X_INTERVAL\", wire, &wire.Type)")
	copyBack := strings.Index(src, "x.Extra = wire.Extra")
	ret := indexFrom(src, "\t\treturn err\n", copyBack)
	defaults := strings.Index(src, "seen.settle(")
	if decode < 0 || copyBack < 0 || ret < 0 || defaults < 0 || decode >= copyBack || copyBack >= ret || ret >= defaults {
		t.Errorf("decoder does not decode, copy the own members back, return the error, then give the defaults (positions %d, %d, %d, %d):\n%s", decode, copyBack, ret, defaults, src)
	}
}

// TestREQ169IntervalFlaggedDecoderCensus pins which generated classes carry
// the four boundary flags and so get decoders that give an absent flag its
// default (REQ-169, REQ-052, REQ-056), and that each of them does, in JSON
// and in XML. AOM 1.4 owns no interval class: its constraint intervals are
// `rm.Interval` and decode through BASE Interval's methods. A class without
// the flags gets neither the wrapper nor the record.
func TestREQ169IntervalFlaggedDecoderCensus(t *testing.T) {
	for _, tc := range []struct {
		target Target
		want   []string
	}{
		{target: TargetRM, want: []string{"DV_INTERVAL", "Interval", "Point_interval", "Proper_interval"}},
		{target: TargetAOM14, want: nil},
	} {
		t.Run(tc.target.RootID, func(t *testing.T) {
			plan, err := BuildPlanForTarget(context.Background(), tc.target, bmm.FSResolver{Root: testResources})
			if err != nil {
				t.Fatalf("BuildPlanForTarget(%s): %v", tc.target.RootID, err)
			}
			var got []string
			for _, f := range plan.Files {
				classes, err := codecClassesIn(plan, f)
				if err != nil {
					t.Fatalf("codecClassesIn(%s): %v", f.FileBase, err)
				}
				for _, pc := range classes {
					fields, err := effectiveFields(plan, pc)
					if err != nil {
						t.Fatalf("effectiveFields(%s): %v", pc.BMMName, err)
					}
					flagged, err := intervalFlagged(fields)
					if err != nil {
						t.Fatalf("intervalFlagged(%s): %v", pc.BMMName, err)
					}
					jsonSrc, err := renderUnmarshalJSON(plan, pc, fields)
					if err != nil {
						t.Fatalf("renderUnmarshalJSON(%s): %v", pc.BMMName, err)
					}
					xmlSrc, err := renderUnmarshalXML(plan, pc, fields)
					if err != nil {
						t.Fatalf("renderUnmarshalXML(%s): %v", pc.BMMName, err)
					}
					if !flagged {
						if strings.Contains(jsonSrc, "intervalFlag") || strings.Contains(xmlSrc, "intervalFlagsSeen") {
							t.Errorf("%s carries no boundary flags but its decoders read them as flags", pc.BMMName)
						}
						continue
					}
					got = append(got, pc.BMMName)
					recv := jsonmarReceiverName(pc.GoName)
					defaults := intervalFlagSettle("seen", recv)
					if !strings.Contains(jsonSrc, defaults) {
						t.Errorf("%s JSON decoder does not give the absent flags their defaults (%q):\n%s", pc.BMMName, defaults, jsonSrc)
					}
					// A side written closed with no bound held by value is
					// refused with the shape sentinel of each codec.
					if want := fmt.Sprintf("return typereg.WrapShapeError(%q, err)", pc.BMMName); !strings.Contains(jsonSrc, want) {
						t.Errorf("%s JSON decoder does not refuse a closed side with no bound through the shape sentinel (%q):\n%s", pc.BMMName, want, jsonSrc)
					}
					if want := "canxml.ErrInvalidShape, _err)"; !strings.Contains(xmlSrc, want) {
						t.Errorf("%s XML decoder does not refuse a closed side with no bound through the shape sentinel (%q):\n%s", pc.BMMName, want, xmlSrc)
					}
					for _, prop := range intervalBoundProps {
						if want := fmt.Sprintf("%s intervalBoundJSON `json:%q`", FieldName(prop), prop); !strings.Contains(jsonSrc, want) {
							t.Errorf("%s JSON decoder does not record whether the document carried %s (%q):\n%s", pc.BMMName, prop, want, jsonSrc)
						}
						if want := "_seen." + seenFlagField(prop) + " = true"; !strings.Contains(xmlSrc, want) {
							t.Errorf("%s XML decoder does not record whether the document carried %s (%q):\n%s", pc.BMMName, prop, want, xmlSrc)
						}
					}
					// A null in place of the interval reads as the member
					// left out (REQ-052): it is consumed before the decode,
					// so no default is given.
					null := strings.Index(jsonSrc, "if dec.PeekKind() == 'n' {")
					decode := strings.Index(jsonSrc, "typereg.DecodeInto(")
					if null < 0 || decode < 0 || null >= decode {
						t.Errorf("%s JSON decoder does not consume a null interval before the decode (positions %d, %d):\n%s", pc.BMMName, null, decode, jsonSrc)
					}
					if !strings.Contains(xmlSrc, "var _seen intervalFlagsSeen") || !strings.Contains(xmlSrc, "_"+defaults) {
						t.Errorf("%s XML decoder does not record the flags and give the absent ones their defaults:\n%s", pc.BMMName, xmlSrc)
					}
					for _, prop := range intervalFlagProps {
						if want := fmt.Sprintf("%s intervalFlagJSON `json:%q`", FieldName(prop), prop); !strings.Contains(jsonSrc, want) {
							t.Errorf("%s JSON decoder does not read %s through the flag wrapper (%q):\n%s", pc.BMMName, prop, want, jsonSrc)
						}
						fn := FieldName(prop)
						want := fmt.Sprintf("decodeIntervalFlagXML(_dec, &_t, &%s.%s, &_seen.%s)", recv, fn, seenFlagField(prop))
						if !strings.Contains(xmlSrc, want) {
							t.Errorf("%s XML decoder does not read %s through the shared flag reader (%q):\n%s", pc.BMMName, prop, want, xmlSrc)
						}
					}
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("classes with the four boundary flags = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestREQ169IntervalFlaggedRefusals pins that the generator refuses a class
// whose boundary flags it cannot default (REQ-169): one that carries some of
// the four but not all, and one whose flag is not a mandatory Boolean. Either
// would otherwise get a decoder that reads an absent flag as false, or
// generated code that does not compile.
func TestREQ169IntervalFlaggedRefusals(t *testing.T) {
	flag := func(name, typ string, mandatory bool) emittedField {
		p := &bmm.SingleProperty{TypeName: typ}
		p.Name = name
		p.IsMandatory = mandatory
		return emittedField{Prop: p, OwnerName: "X_INTERVAL"}
	}
	openFlag := func(name string) emittedField {
		p := &bmm.SinglePropertyOpen{TypeName: "T"}
		p.Name = name
		p.IsMandatory = true
		return emittedField{Prop: p, OwnerName: "X_INTERVAL"}
	}
	all := func() []emittedField {
		out := make([]emittedField, 0, len(intervalFlagProps))
		for _, prop := range intervalFlagProps {
			out = append(out, flag(prop, "Boolean", true))
		}
		return out
	}
	for _, tc := range []struct {
		name        string
		fields      []emittedField
		wantFlagged bool
		wantErr     string
	}{
		{name: "all four mandatory Booleans", fields: all(), wantFlagged: true},
		{name: "no flags", fields: []emittedField{flag("lower", "Integer", false)}},
		{name: "two of the four", fields: all()[:2], wantErr: "not all four"},
		{name: "an optional flag", fields: append(all()[:3], flag(propUpperUnbounded, "Boolean", false)), wantErr: "not a mandatory Boolean"},
		{name: "a flag of another type", fields: append(all()[:3], flag(propUpperUnbounded, "String", true)), wantErr: "not a mandatory Boolean"},
		{name: "a flag that is no single property", fields: append(all()[:3], openFlag(propUpperUnbounded)), wantErr: "not a mandatory Boolean"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := intervalFlagged(tc.fields)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("intervalFlagged = %v, %v; want an error containing %q", got, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.wantFlagged {
				t.Fatalf("intervalFlagged = %v, %v; want %v, nil", got, err, tc.wantFlagged)
			}
		})
	}
}

// TestREQ169IntervalFlagHelpersSettleOpenFlagsFirst pins the shared default
// (REQ-169) in the generated interval file: the open flags take their
// defaults before the included flags, which read them, so an included default
// follows its side's final open flag whatever the member order. The XML flag
// decoder reads its text through xmlunmatched.DecodeText, so a flag that is no
// boolean fails as a primitive element's text fails (REQ-056).
func TestREQ169IntervalFlagHelpersSettleOpenFlagsFirst(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	body, err := RenderIntervalBoundFile(plan)
	if err != nil {
		t.Fatalf("RenderIntervalBoundFile: %v", err)
	}
	src := string(body)
	for _, want := range []string{
		"type intervalFlagsSeen struct", "type intervalFlagJSON struct", "func (f intervalFlagJSON) UnmarshalJSONFrom(",
		"func decodeIntervalFlagXML(", "return xmlunmatched.DecodeText(dec, start, func(text string) error {", "text = strings.TrimSpace(text)",
		"type intervalBoundJSON struct", "func (f intervalBoundJSON) UnmarshalJSONFrom(",
		"func intervalBoundHeldByValue[T any](*T) bool", "func errIntervalClosedSide(side string) error",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("interval file lacks %q:\n%s", want, src)
		}
	}
	lastOpen := max(strings.Index(src, "*lowerUnbounded = !s.lower"), strings.Index(src, "*upperUnbounded = !s.upper"))
	firstIncluded := min(strings.Index(src, "*lowerIncluded = !*lowerUnbounded"), strings.Index(src, "*upperIncluded = !*upperUnbounded"))
	if lastOpen < 0 || firstIncluded < 0 || lastOpen >= firstIncluded {
		t.Errorf("settle does not settle the open flags before the included ones (positions %d, %d):\n%s", lastOpen, firstIncluded, src)
	}
}

// TestJSONWireShapeCensus pins the one decision both the marshaller and the
// unmarshaller read (REQ-052): the shape of each interval class and of a few
// neighbours, and the refusal of an interval-shaped class that embeds a
// marshaler-bearing concrete class.
func TestJSONWireShapeCensus(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	for class, want := range map[string]jsonWireShape{
		"Interval":        shapeIntervalBase,
		"Point_interval":  shapeIntervalDerived,
		"Proper_interval": shapeIntervalDerived,
		"DV_INTERVAL":     shapeIntervalDerived,
		"DV_CODED_TEXT":   shapeFlat,
		"DV_QUANTITY":     shapeAlias,
	} {
		got, _, err := classJSONWireShape(plan, plan.Classes[class])
		if err != nil || got != want {
			t.Errorf("classJSONWireShape(%s) = %v, %v; want %v", class, got, err, want)
		}
	}
	// Multiplicity_interval embeds Proper_interval, a marshaler-bearing
	// concrete class, and is interval-shaped: the flat shape cannot serve it.
	if _, _, err := classJSONWireShape(plan, plan.Classes["Multiplicity_interval"]); err == nil {
		t.Error("classJSONWireShape(Multiplicity_interval) succeeded, want a refusal")
	}
}

// indexFrom returns the index of the first substr in s at or after from, or
// -1 when from is negative or substr does not occur there.
func indexFrom(s, substr string, from int) int {
	if from < 0 {
		return -1
	}
	i := strings.Index(s[from:], substr)
	if i < 0 {
		return -1
	}
	return from + i
}

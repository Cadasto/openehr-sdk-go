package bmmgen

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestGoldenDataTypesQuantity regenerates the data_types_quantity
// file and diffs it against the checked-in golden.
//
// If the golden drift is intentional, update with:
//
//	cp openehr/rm/data_types_quantity_gen.go \
//	   internal/bmmgen/testdata/data_types_quantity_gen.go.golden
//
// REQ-043: § Mapping rules, for one whole BMM package. The golden holds the
// class, property, type and container mapping: abstract classes as marker
// interfaces, mandatory properties as values and optional ones as pointers,
// containers as slices, generic classes as type parameters, and JSON tags
// that keep the BMM property names.
func TestGoldenDataTypesQuantity(t *testing.T) {
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
		t.Fatalf("data_types_quantity file not in plan")
	}
	got, err := RenderFile(plan, file)
	if err != nil {
		t.Fatalf("RenderFile: %v", err)
	}
	goldenPath := filepath.Join("testdata", "data_types_quantity_gen.go.golden")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("data_types_quantity_gen.go differs from golden\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}
}

// TestIdempotent regenerates the full RM into a temp dir twice and
// asserts byte-identity across runs. This catches any non-stable
// iteration order (Go map iteration is randomised).
//
// REQ-042: the generator is reproducible; two runs over the same inputs are
// byte-identical.
func TestIdempotent(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	for _, dir := range []string{dir1, dir2} {
		if _, err := Run(Options{
			ResourcesDir: testResources,
			OutDir:       dir,
			RootID:       "openehr_rm_1.2.0",
		}); err != nil {
			t.Fatalf("Run %s: %v", dir, err)
		}
	}
	compareDirs(t, filepath.Join(dir1, "openehr", "rm"), filepath.Join(dir2, "openehr", "rm"))
}

// TestDriftDetection generates the RM into a temp dir, mutates one
// byte of a generated file, and asserts that -verify reports a
// drift on that file.
//
// REQ-042: the drift check reports a generated file that no longer matches
// the generator's output.
func TestDriftDetection(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{
		ResourcesDir: testResources,
		OutDir:       dir,
		RootID:       "openehr_rm_1.2.0",
	}); err != nil {
		t.Fatalf("initial Run: %v", err)
	}
	target := filepath.Join(dir, "openehr", "rm", "data_types_quantity_gen.go")
	orig, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	// Flip the first byte of the BMM-package marker comment.
	mutated := append([]byte{}, orig...)
	// Find a safe spot to mutate (the "// BMM package:" line).
	idx := bytes.Index(mutated, []byte("// BMM package:"))
	if idx < 0 {
		t.Fatalf("could not find marker comment in %s", target)
	}
	// idx points to "// BMM package:" — change "BMM" to "BBM" by
	// flipping the M at offset 4.
	mutated[idx+4] = 'B'
	if err := os.WriteFile(target, mutated, 0o644); err != nil {
		t.Fatalf("write mutated: %v", err)
	}
	result, err := Run(Options{
		ResourcesDir: testResources,
		OutDir:       dir,
		RootID:       "openehr_rm_1.2.0",
		Verify:       true,
	})
	if err != nil {
		t.Fatalf("verify Run: %v", err)
	}
	if len(result.Drifts) == 0 {
		t.Fatalf("expected at least one drift after mutation, got 0")
	}
	hasMutated := false
	for _, d := range result.Drifts {
		if d.Path == target {
			hasMutated = true
			break
		}
	}
	if !hasMutated {
		t.Errorf("drift not reported for %s; got %v", target, result.Drifts)
	}
}

// TestMethodStubsForDVQuantity asserts that the DV_QUANTITY class
// receives a method stub with the correct shape:
//   - PascalCase method name (e.g. IsStrictlyComparableTo)
//   - first doc line begins with the Go method name
//   - panic message uses the BMM names verbatim
//
// Phase 3 contract: every BMM function maps to one Go method stub.
//
// REQ-043: § Mapping rules, Functions. A BMM function becomes a method stub
// that carries its pre- and post-conditions as comments and panics with a
// not-implemented message.
func TestMethodStubsForDVQuantity(t *testing.T) {
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
		t.Fatalf("data_types_quantity file not in plan")
	}
	got, err := RenderFile(plan, file)
	if err != nil {
		t.Fatalf("RenderFile: %v", err)
	}
	src := string(got)
	wantSnippets := []string{
		"// IsStrictlyComparableTo True if this quantity and `_other_` have the same `_units_` and also `_units_system_` if it exists.",
		"func (d *DVQuantity) IsStrictlyComparableTo(other DVOrdered) bool {",
		`panic("not implemented: DV_QUANTITY.is_strictly_comparable_to — implement in a non-generated file")`,
		// Pre/Post propagation
		"// Pre: is_strictly_comparable_to (other)",
		"// Post: Result = magnitude < other.magnitude",
		// Operator alias
		"// Aliases: + (Go does not support operator overloading)",
		"func (d *DVQuantity) Add(other DVQuantity) DVQuantity {",
	}
	for _, snip := range wantSnippets {
		if !bytes.Contains(got, []byte(snip)) {
			t.Errorf("expected snippet not found in generated output:\n  want: %s", snip)
		}
	}
	// Phase 3: emission count is non-trivial.
	if plan.MethodStubsEmitted == 0 {
		t.Errorf("expected Plan.MethodStubsEmitted > 0, got 0")
	}
	_ = src
}

// TestManualImplementationSkip asserts the manuallyImplemented set
// (manual_impl.go) suppresses stub emission for the REQ-120..123
// hand-written functions, while leaving the deferred functions as
// generated fail-loud panic stubs. Guards the generator hook that lets
// openehr/rm/*_funcs.go and openehr/rm/rmpath provide those surfaces
// without a "method redeclared" collision (ADR 0002 § D7, ADR 0011).
//
// REQ-043: § Mapping rules, Functions. A function that has no hand-written
// body keeps its fail-loud stub.
func TestManualImplementationSkip(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	render := func(fileBase string) string {
		var file *PlannedFile
		for _, f := range plan.Files {
			if f.FileBase == fileBase {
				file = f
				break
			}
		}
		if file == nil {
			t.Fatalf("%s file not in plan", fileBase)
		}
		got, err := RenderFile(plan, file)
		if err != nil {
			t.Fatalf("RenderFile(%s): %v", fileBase, err)
		}
		return string(got)
	}

	// Suppressed: these stubs MUST NOT appear (hand-written elsewhere).
	suppressed := map[string][]string{
		"base_types_identification": {
			"not implemented: UID_BASED_ID.root",
			"not implemented: OBJECT_VERSION_ID.is_branch",
			"not implemented: VERSION_TREE_ID.is_branch",
			"not implemented: ARCHETYPE_ID.domain_concept",
			"not implemented: TERMINOLOGY_ID.name",
			"not implemented: LOCATABLE_REF.as_uri",
		},
		"common_archetyped":             {"not implemented: PATHABLE.item_at_path", "not implemented: PATHABLE.path_unique"},
		"common_change_control":         {"not implemented: VERSION.is_branch"},
		"data_types_quantity_date_time": {"not implemented: DV_DATE.magnitude", "not implemented: DV_DURATION.less_than"},
	}
	for fileBase, msgs := range suppressed {
		src := render(fileBase)
		for _, m := range msgs {
			if bytes.Contains([]byte(src), []byte(m)) {
				t.Errorf("%s: stub %q should be suppressed (manuallyImplemented) but was emitted", fileBase, m)
			}
		}
	}

	// Deferred: these stubs MUST remain (out of scope, fail loud).
	deferred := map[string][]string{
		"common_archetyped":             {"not implemented: PATHABLE.parent", "not implemented: PATHABLE.path_of_item"},
		"data_types_quantity_date_time": {"not implemented: DV_DATE.add", "not implemented: DV_DURATION.multiply"},
	}
	for fileBase, msgs := range deferred {
		src := render(fileBase)
		for _, m := range msgs {
			if !bytes.Contains([]byte(src), []byte(m)) {
				t.Errorf("%s: deferred stub %q should remain but was not emitted", fileBase, m)
			}
		}
	}
}

// TestOptionalFieldsThatAreNotPointers pins the optional fields the generator
// leaves unpointed, each with its omitempty tag, beside an optional field
// typed by a class type parameter, which is a pointer with omitzero.
//
// REQ-043: § Mapping rules, Property → Go field. A non-mandatory property
// whose type is emitted as a Go interface (an abstract class, or a concrete
// class with subtypes emitted as a `…Like` interface) stays `T` and carries
// `omitempty`, and so does a P_BMM_SINGLE_PROPERTY_OPEN.
func TestOptionalFieldsThatAreNotPointers(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	for _, tc := range []struct {
		name, fileBase, field string
	}{
		{"abstract class", "composition_content_entry", "Protocol ItemStructure `json:\"protocol,omitempty\"`"},
		{"concrete class with subtypes", "composition_content_entry", "GuidelineID ObjectRefLike `json:\"guideline_id,omitempty\"`"},
		{"single property open", "foundation_types_interval", "Lower T `json:\"lower,omitempty\"`"},
		{"single property typed by a type parameter", "common_change_control", "Data *T `json:\"data,omitzero\"`"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var file *PlannedFile
			for _, f := range plan.Files {
				if f.FileBase == tc.fileBase {
					file = f
					break
				}
			}
			if file == nil {
				t.Fatalf("%s file not in plan", tc.fileBase)
			}
			got, err := RenderFile(plan, file)
			if err != nil {
				t.Fatalf("RenderFile(%s): %v", tc.fileBase, err)
			}
			if !fieldDecl(tc.field).Match(got) {
				t.Errorf("%s_gen.go does not declare the field %q", tc.fileBase, tc.field)
			}
		})
	}
}

// TestMandatoryFieldsCarryNoOmitOption pins the "no omit option" half of the
// JSON tag rule for the three property kinds that choose between a plain tag
// and an omit option by their mandatory flag or cardinality, each on a real
// RM field, beside an optional field of the same kind that does carry one.
//
// REQ-043: § Mapping rules, Property → Go field. A P_BMM_CONTAINER_PROPERTY
// whose cardinality lower bound is 1 or more carries no omit option, and
// neither does a mandatory P_BMM_GENERIC_PROPERTY or a mandatory
// P_BMM_SINGLE_PROPERTY_OPEN. Without these rows a flip in the generator
// shows only as a `make codegen-verify` diff.
func TestMandatoryFieldsCarryNoOmitOption(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	for _, tc := range []struct {
		name, fileBase, field string
	}{
		{"container, lower bound 1: CLUSTER.items", "data_structures_representation", "Items []Item `json:\"items\"`"},
		{"container, lower bound 0: PARTY.contacts", "demographic", "Contacts []Contact `json:\"contacts,omitempty\"`"},
		{"mandatory generic over Hash: RESOURCE_DESCRIPTION.details", "common_resource", "Details map[string]ResourceDescriptionItem `json:\"details\"`"},
		{"mandatory generic class: OBSERVATION.data", "composition_content_entry", "Data History[ItemStructure] `json:\"data\"`"},
		{"optional generic: PARTICIPATION.time", "common_generic", "Time *DVInterval[DVDateTime] `json:\"time,omitzero\"`"},
		{"mandatory open parameter: EVENT.data", "data_structures_history", "Data T `json:\"data\"`"},
		{"optional open parameter: INTERVAL.lower", "foundation_types_interval", "Lower T `json:\"lower,omitempty\"`"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var file *PlannedFile
			for _, f := range plan.Files {
				if f.FileBase == tc.fileBase {
					file = f
					break
				}
			}
			if file == nil {
				t.Fatalf("%s file not in plan", tc.fileBase)
			}
			got, err := RenderFile(plan, file)
			if err != nil {
				t.Fatalf("RenderFile(%s): %v", tc.fileBase, err)
			}
			if !fieldDecl(tc.field).Match(got) {
				t.Errorf("%s_gen.go does not declare the field %q", tc.fileBase, tc.field)
			}
		})
	}
}

// TestAnyPropertyType pins the Go type of a single property whose BMM type
// is the primitive Any, on a property built for the test and on the real
// AOM 1.4 fields of each kind.
//
// REQ-043: § Mapping rules, Property → Go field. A non-mandatory property
// whose BMM type is Any is emitted as *any, because Any is a primitive and
// not a class the interface exception covers. A mandatory one is any.
func TestAnyPropertyType(t *testing.T) {
	rmPlan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	aomPlan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}
	// built is a property of type Any on a real owner, so the owner's
	// generic parameters and cycles are those the generator meets.
	built := func(mandatory bool) *bmm.SingleProperty {
		p := &bmm.SingleProperty{TypeName: "Any"}
		p.Name = "test_any"
		p.IsMandatory = mandatory
		return p
	}
	for _, tc := range []struct {
		name  string
		plan  *Plan
		owner string
		prop  func(t *testing.T, owner *bmm.SimpleClass) *bmm.SingleProperty
		want  string
	}{
		{
			name: "optional, built", plan: rmPlan, owner: "ELEMENT", want: "*any",
			prop: func(*testing.T, *bmm.SimpleClass) *bmm.SingleProperty { return built(false) },
		},
		{
			name: "mandatory, built", plan: rmPlan, owner: "ELEMENT", want: "any",
			prop: func(*testing.T, *bmm.SimpleClass) *bmm.SingleProperty { return built(true) },
		},
		{
			name: "optional: C_DEFINED_OBJECT.assumed_value", plan: aomPlan, owner: "C_DEFINED_OBJECT", want: "*any",
			prop: func(t *testing.T, owner *bmm.SimpleClass) *bmm.SingleProperty {
				return anyProperty(t, owner, "assumed_value", false)
			},
		},
		{
			name: "mandatory: EXPR_LEAF.item", plan: aomPlan, owner: "EXPR_LEAF", want: "any",
			prop: func(t *testing.T, owner *bmm.SimpleClass) *bmm.SingleProperty {
				return anyProperty(t, owner, "item", true)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pc, ok := tc.plan.Classes[tc.owner]
			if !ok {
				t.Fatalf("%s is not in the plan", tc.owner)
			}
			owner, ok := pc.Class.(*bmm.SimpleClass)
			if !ok {
				t.Fatalf("%s is %T, want a simple class", tc.owner, pc.Class)
			}
			p := tc.prop(t, owner)
			got, err := singlePropTypeExpr(tc.plan, owner, tc.owner, p)
			if err != nil {
				t.Fatalf("singlePropTypeExpr(%s.%s): %v", tc.owner, p.Name, err)
			}
			if got != tc.want {
				t.Errorf("singlePropTypeExpr(%s.%s, mandatory %t) = %q, want %q", tc.owner, p.Name, p.IsMandatory, got, tc.want)
			}
		})
	}
}

// anyProperty returns owner's single property name, and stops the test
// unless it is typed Any with the given mandatory flag.
func anyProperty(t *testing.T, owner *bmm.SimpleClass, name string, mandatory bool) *bmm.SingleProperty {
	t.Helper()
	p, ok := owner.Properties[name].(*bmm.SingleProperty)
	if !ok {
		t.Fatalf("%s is %T, want a single property", name, owner.Properties[name])
	}
	if p.TypeName != "Any" || p.IsMandatory != mandatory {
		t.Fatalf("%s is %s, mandatory %t; want Any, mandatory %t", name, p.TypeName, p.IsMandatory, mandatory)
	}
	return p
}

// fieldDecl matches a struct field written as "Name Type `tag`" on one line,
// with any run of spaces or tabs between the three parts: gofmt aligns a
// field's type and tag with its neighbours', so the spacing changes when an
// unrelated field does. The last part must be followed by a space, a tab or
// the end of the line, so "UID *rm.HierObjectID" does not match a field of
// type *rm.HierObjectIDList.
func fieldDecl(field string) *regexp.Regexp {
	parts := strings.SplitN(field, " ", 3)
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile(`(?m)^[ \t]*` + strings.Join(parts, `[ \t]+`) + `(?:[ \t]|$)`)
}

// TestFieldDeclIgnoresAlignment checks that fieldDecl accepts a field padded
// for alignment and still tells the field from one with another name, type or
// tag.
func TestFieldDeclIgnoresAlignment(t *testing.T) {
	const field = "Data *T `json:\"data,omitzero\"`"
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{src: "\tData *T `json:\"data,omitzero\"`\n", want: true},
		{src: "\tData        *T      `json:\"data,omitzero\"`\n", want: true},
		{src: "\tData T `json:\"data,omitzero\"`\n", want: false},
		{src: "\tMetaData *T `json:\"data,omitzero\"`\n", want: false},
		{src: "\tData *T `json:\"data,omitempty\"`\n", want: false},
	} {
		if got := fieldDecl(field).MatchString(tc.src); got != tc.want {
			t.Errorf("fieldDecl(%q).MatchString(%q) = %v, want %v", field, tc.src, got, tc.want)
		}
	}

	// A field without a tag ends at its type, so a longer type name that
	// starts with the same text is another field, and so is the same text
	// split over two lines.
	const tagless = "UID *rm.HierObjectID"
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{src: "\tUID *rm.HierObjectID\n", want: true},
		{src: "\tUID   *rm.HierObjectID `json:\"uid\"`\n", want: true},
		{src: "\tUID *rm.HierObjectID", want: true},
		{src: "\tUID *rm.HierObjectIDList\n", want: false},
		{src: "\tUID\n\t*rm.HierObjectID\n", want: false},
	} {
		if got := fieldDecl(tagless).MatchString(tc.src); got != tc.want {
			t.Errorf("fieldDecl(%q).MatchString(%q) = %v, want %v", tagless, tc.src, got, tc.want)
		}
	}
}

// TestVerifyOnFreshTreeIsClean asserts that immediately after a
// generation the working tree passes -verify with no drifts.
func TestVerifyOnFreshTreeIsClean(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{
		ResourcesDir: testResources,
		OutDir:       dir,
		RootID:       "openehr_rm_1.2.0",
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	result, err := Run(Options{
		ResourcesDir: testResources,
		OutDir:       dir,
		RootID:       "openehr_rm_1.2.0",
		Verify:       true,
	})
	if err != nil {
		t.Fatalf("verify Run: %v", err)
	}
	if len(result.Drifts) != 0 {
		t.Errorf("expected no drift, got %d", len(result.Drifts))
		for _, d := range result.Drifts {
			t.Errorf("  drift: %s (existing=%v)", d.Path, d.Existing)
		}
	}
}

// compareDirs asserts that every file in a exists in b with the
// same content, and vice versa. Recurses into sub-directories so
// targets that emit into sub-packages (e.g. openehr/rm/rminfo/) are
// covered.
func compareDirs(t *testing.T, a, b string) {
	t.Helper()
	aEntries, err := os.ReadDir(a)
	if err != nil {
		t.Fatalf("read %s: %v", a, err)
	}
	bEntries, err := os.ReadDir(b)
	if err != nil {
		t.Fatalf("read %s: %v", b, err)
	}
	if len(aEntries) != len(bEntries) {
		t.Fatalf("dir entry counts differ: %s=%d vs %s=%d", a, len(aEntries), b, len(bEntries))
	}
	for _, ae := range aEntries {
		ap := filepath.Join(a, ae.Name())
		bp := filepath.Join(b, ae.Name())
		if ae.IsDir() {
			compareDirs(t, ap, bp)
			continue
		}
		ab, err := os.ReadFile(ap)
		if err != nil {
			t.Fatalf("read %s: %v", ap, err)
		}
		bb, err := os.ReadFile(bp)
		if err != nil {
			t.Fatalf("read %s: %v", bp, err)
		}
		if !bytes.Equal(ab, bb) {
			t.Errorf("file %s differs across runs (len %d vs %d)", ae.Name(), len(ab), len(bb))
		}
	}
}

package instance_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// testComposer returns a stable *rm.PartyIdentified test fixture.
// The Name field is *string in the generated RM, so callers can't
// inline a struct literal — this helper centralises the conversion.
func testComposer() *rm.PartyIdentified {
	name := "Test Composer"
	return &rm.PartyIdentified{Name: &name}
}

func compileFixture(t *testing.T, name string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("ParseFile %s: %v", name, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile %s: %v", name, err)
	}
	return c
}

func TestGenerateNilCompiled(t *testing.T) {
	_, err := instance.Generate(context.Background(), nil, instance.Options{})
	if !errors.Is(err, instance.ErrNilCompiled) {
		t.Fatalf("want ErrNilCompiled, got %v", err)
	}
}

func TestGenerateCompositionRequiresOptions(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	if _, err := instance.Generate(context.Background(), c, instance.Options{}); !errors.Is(err, instance.ErrComposerRequired) {
		t.Errorf("missing composer: want ErrComposerRequired, got %v", err)
	}
	x := "X"
	if _, err := instance.Generate(context.Background(), c, instance.Options{Composer: &rm.PartyIdentified{Name: &x}}); !errors.Is(err, instance.ErrTerritoryRequired) {
		t.Errorf("missing territory: want ErrTerritoryRequired, got %v", err)
	}
}

func TestGenerateVitalSignsMinimal(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	out, err := instance.Generate(context.Background(), c, instance.Options{
		Policy:    instance.Minimal,
		Territory: "NL",
		Composer:  testComposer(),
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}
	if comp.Category.DefiningCode.CodeString != "433" {
		t.Errorf("Category.DefiningCode.CodeString = %q, want 433", comp.Category.DefiningCode.CodeString)
	}
	if comp.Category.DefiningCode.TerminologyID.Value != "openehr" {
		t.Errorf("Category.DefiningCode.TerminologyID.Value = %q, want openehr", comp.Category.DefiningCode.TerminologyID.Value)
	}
	if comp.Context == nil {
		t.Fatal("Context is nil")
	}
	if comp.Context.StartTime.Value == "" {
		t.Error("Context.StartTime.Value is empty")
	}
	if len(comp.Content) == 0 {
		t.Errorf("Content empty under Minimal policy (vital_signs OPT pins multiple OBSERVATIONs)")
	}
	if comp.Composer == nil {
		t.Error("Composer is nil")
	}
	if comp.Language.CodeString == "" {
		t.Error("Language.CodeString is empty")
	}
	if comp.Territory.CodeString != "NL" {
		t.Errorf("Territory.CodeString = %q, want NL", comp.Territory.CodeString)
	}
	if comp.ArchetypeDetails == nil {
		t.Error("ArchetypeDetails is nil on template root")
	} else if comp.ArchetypeDetails.TemplateID == nil || comp.ArchetypeDetails.TemplateID.Value == "" {
		t.Error("ArchetypeDetails.TemplateID is empty on template root")
	}
	if comp.UID == nil {
		t.Error("UID is nil on template root")
	}
}

func TestGenerateVitalSignsExamplePopulatesPrimitives(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	out, err := instance.Generate(context.Background(), c, instance.Options{
		Policy:    instance.Example,
		Territory: "NL",
		Composer:  testComposer(),
	})
	if err != nil {
		t.Fatalf("Generate Example: %v", err)
	}
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}

	// Locate one Element with a DV_QUANTITY value somewhere under
	// /content[OBSERVATION]/data/events[]/data — i.e. systolic. We
	// only need to prove "some primitive leaf has a non-zero
	// magnitude or non-empty units".
	found := findQuantityLeaf(comp)
	if found == nil {
		t.Fatal("no DV_QUANTITY leaf found under Example-policy composition")
	}
	if found.Units == "" {
		t.Errorf("DV_QUANTITY leaf units empty under Example policy")
	}
}

func findQuantityLeaf(c *rm.Composition) *rm.DVQuantity {
	for _, item := range c.Content {
		if q := scanContentItemForQuantity(item); q != nil {
			return q
		}
	}
	return nil
}

func scanContentItemForQuantity(it rm.ContentItem) *rm.DVQuantity {
	if obs, ok := it.(*rm.Observation); ok {
		for _, ev := range obs.Data.Events {
			if q := scanEventForQuantity(ev); q != nil {
				return q
			}
		}
	}
	return nil
}

func scanEventForQuantity(ev rm.Event) *rm.DVQuantity {
	switch e := ev.(type) {
	case *rm.PointEvent[rm.ItemStructure]:
		return scanItemStructureForQuantity(e.Data)
	case *rm.IntervalEvent[rm.ItemStructure]:
		return scanItemStructureForQuantity(e.Data)
	}
	return nil
}

func scanItemStructureForQuantity(is rm.ItemStructure) *rm.DVQuantity {
	switch v := is.(type) {
	case *rm.ItemList:
		for i := range v.Items {
			if q := elementQuantity(&v.Items[i]); q != nil {
				return q
			}
		}
	case *rm.ItemTree:
		for _, it := range v.Items {
			if q := scanItemForQuantity(it); q != nil {
				return q
			}
		}
	case *rm.ItemSingle:
		if q := elementQuantity(&v.Item); q != nil {
			return q
		}
	}
	return nil
}

func scanItemForQuantity(it rm.Item) *rm.DVQuantity {
	switch x := it.(type) {
	case *rm.Element:
		return elementQuantity(x)
	case *rm.Cluster:
		for _, c := range x.Items {
			if q := scanItemForQuantity(c); q != nil {
				return q
			}
		}
	}
	return nil
}

func elementQuantity(e *rm.Element) *rm.DVQuantity {
	if e == nil {
		return nil
	}
	if dv, ok := e.Value.(*rm.DVQuantity); ok && dv != nil {
		if dv.Units != "" || dv.Magnitude != 0 {
			return dv
		}
		return dv
	}
	return nil
}

// TestGenerateClinicalNoteMinimal pins the PR #18 re-review finding:
// clinical_note.opt uses the AOM 1.4 primitive-short-name shape
// (DV_DURATION → value → C_PRIMITIVE_OBJECT → DURATION → C_DURATION).
// Before the materialiseSingle / isAOMPrimitiveShortName fix, the
// generator tried to attach a fresh *DVDuration to the parent DV's
// .value (a String slot) and failed; this regression keeps the
// generator end-to-end on the second vendored OPT fixture.
//
// The leaf primitive constraint flows through the parser via the
// C_PRIMITIVE_OBJECT inner-`<item>` extraction; CDuration's
// ExampleValue happens to return the same "P0D" sentinel as the
// pre-Phase-1 fallback, so the asserted value is stable across
// the two regression scopes.
func TestGenerateClinicalNoteMinimal(t *testing.T) {
	c := compileFixture(t, "clinical_note")
	name := "Test Composer"
	out, err := instance.Generate(context.Background(), c, instance.Options{
		Policy:    instance.Minimal,
		Territory: "NL",
		Composer:  &rm.PartyIdentified{Name: &name},
	})
	if err != nil {
		t.Fatalf("Generate clinical_note.opt: %v", err)
	}
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}
	dur := findFirstDVDuration(comp)
	if dur == nil {
		t.Fatal("no DV_DURATION found in synthesised clinical_note tree")
	}
	if dur.Value != "P0D" {
		t.Errorf("DV_DURATION.value = %q, want %q (default sentinel)", dur.Value, "P0D")
	}
}

// findFirstDVDuration walks a Composition's content depth-first
// looking for a DV_DURATION leaf in any ELEMENT.value. Returns the
// first match or nil. REQ-024: closed dispatch on the RM types the
// clinical_note.opt subtree traverses (CLUSTER + ELEMENT).
func findFirstDVDuration(comp *rm.Composition) *rm.DVDuration {
	for _, c := range comp.Content {
		if d := findInContent(c); d != nil {
			return d
		}
	}
	return nil
}

func findInContent(c rm.ContentItem) *rm.DVDuration {
	switch v := c.(type) {
	case *rm.Instruction:
		for _, a := range v.Activities {
			tree, ok := a.Description.(*rm.ItemTree)
			if !ok {
				continue
			}
			if d := findInItemTree(tree); d != nil {
				return d
			}
		}
	}
	return nil
}

func findInItemTree(t *rm.ItemTree) *rm.DVDuration {
	if t == nil {
		return nil
	}
	for _, it := range t.Items {
		switch e := it.(type) {
		case *rm.Element:
			if d, ok := e.Value.(*rm.DVDuration); ok {
				return d
			}
		case *rm.Cluster:
			for _, inner := range e.Items {
				if el, ok := inner.(*rm.Element); ok {
					if d, ok := el.Value.(*rm.DVDuration); ok {
						return d
					}
				}
			}
		}
	}
	return nil
}

// TestGenerateUIDCarriesType pins the PR #20 re-review deferral:
// canjson's polymorphic dispatch on Composition.uid (an interface
// `UIDBasedID` whose concrete should be HierObjectID) requires a
// pointer receiver path. Before the
// [`docs/plans/archive/2026-05-26-c-primitive-object-wire-parser.md`] Phase 2
// fix, `newHierObjectID()` returned a value, and canjson emitted
// `uid` WITHOUT a `_type` discriminator — breaking the unmarshal
// round-trip PROBE-023's spec wording promised.
//
// Phase 2 of the plan landed `newHierObjectID()` returning
// `*rm.HierObjectID`; this is now the regression gate.
func TestGenerateUIDCarriesType(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	name := "Test Composer"
	out, err := instance.Generate(context.Background(), c, instance.Options{
		Policy:    instance.Minimal,
		Territory: "NL",
		Composer:  &rm.PartyIdentified{Name: &name},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}
	b, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	// The uid field must carry `_type:"HIER_OBJECT_ID"` so that
	// canjson.Unmarshal can resolve the polymorphic UIDBasedID
	// interface. The string-contains assertion is stable against
	// canjson's field-order convention.
	if !bytes.Contains(b, []byte(`"uid":{"_type":"HIER_OBJECT_ID"`)) {
		t.Errorf("canjson(Composition).uid missing _type discriminator; got: %s", uidSlice(b))
	}
}

// uidSlice returns the canjson `uid` object substring for the
// failing-test diagnostic, without quoting the whole composition.
func uidSlice(b []byte) string {
	const k = `"uid":`
	i := bytes.Index(b, []byte(k))
	if i < 0 {
		return "<uid not present>"
	}
	rest := b[i+len(k):]
	// Walk a balanced { … } object — small handwritten scanner so
	// the test does not pull in an extra dependency.
	if len(rest) == 0 || rest[0] != '{' {
		return string(rest[:min(64, len(rest))])
	}
	depth := 0
	for j := 0; j < len(rest); j++ {
		switch rest[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return string(rest[:j+1])
			}
		}
	}
	return string(rest[:min(128, len(rest))])
}

func TestPolicyString(t *testing.T) {
	if instance.Minimal.String() != "minimal" {
		t.Error("Minimal.String() != minimal")
	}
	if instance.Example.String() != "example" {
		t.Error("Example.String() != example")
	}
	if !strings.Contains(instance.Policy(99).String(), "unknown") {
		t.Error("Policy(99).String() should contain 'unknown'")
	}
}

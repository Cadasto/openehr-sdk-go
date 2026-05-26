package instance_test

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// optPath resolves a vendored OPT path relative to this test file
// so `go test` works from any cwd.
func optPath(t *testing.T, name string) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Join(filepath.Dir(here), "..", "..", "openehr", "template", "testdata")
	return filepath.Join(root, name)
}

// testComposer returns a stable *rm.PartyIdentified test fixture.
// The Name field is *string in the generated RM, so callers can't
// inline a struct literal — this helper centralises the conversion.
func testComposer() *rm.PartyIdentified {
	name := "Test Composer"
	return &rm.PartyIdentified{Name: &name}
}

func compileFixture(t *testing.T, name string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(optPath(t, name))
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
	c := compileFixture(t, "vital_signs.opt")
	if _, err := instance.Generate(context.Background(), c, instance.Options{}); !errors.Is(err, instance.ErrComposerRequired) {
		t.Errorf("missing composer: want ErrComposerRequired, got %v", err)
	}
	x := "X"
	if _, err := instance.Generate(context.Background(), c, instance.Options{Composer: &rm.PartyIdentified{Name: &x}}); !errors.Is(err, instance.ErrTerritoryRequired) {
		t.Errorf("missing territory: want ErrTerritoryRequired, got %v", err)
	}
}

func TestGenerateVitalSignsMinimal(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
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
	c := compileFixture(t, "vital_signs.opt")
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

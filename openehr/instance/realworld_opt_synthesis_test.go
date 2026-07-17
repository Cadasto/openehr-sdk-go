package instance_test

import (
	"context"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func compileRealWorldFixture(t *testing.T, name string) *templatecompile.Compiled {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("ReadFile %s: %v", name, err)
	}
	opt, err := fixtures.ParseOPTBytes(raw)
	if err != nil {
		t.Fatalf("ParseOPTBytes %s: %v", name, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile %s: %v", name, err)
	}
	return c
}

// TestGenerateSocialMinimal_respectsContentUpper pins REQ-107 Task 3:
// social.opt content has existence.upper=1 with multiple optional
// archetype roots sharing node_id at0000 — Minimal synthesis must
// emit at most one content entry so validation binds cleanly.
func TestGenerateSocialMinimal_respectsContentUpper(t *testing.T) {
	c := compileRealWorldFixture(t, "social")
	name := "Test Composer"
	out, err := instance.Generate(context.Background(), c, instance.Options{
		Policy:    instance.Minimal,
		Territory: "NL",
		Composer:  &rm.PartyIdentified{Name: &name},
	})
	if err != nil {
		t.Fatalf("Generate social: %v", err)
	}
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}
	if n := len(comp.Content); n != 1 {
		t.Fatalf("content len = %d, want 1 (existence.upper=1)", n)
	}
	if _, ok := comp.Content[0].(*rm.Observation); !ok {
		t.Fatalf("content[0] type = %T, want *rm.Observation (first colliding optional sibling)", comp.Content[0])
	}
}

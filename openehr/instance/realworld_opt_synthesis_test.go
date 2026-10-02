package instance_test

import (
	jsonv2 "encoding/json/v2"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
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

// TestREQ107_GenerateSocialMinimal_respectsContentUpper pins REQ-107:
// social.opt content has existence.upper=1 with multiple optional
// archetype roots sharing node_id at0000 — Minimal synthesis must
// emit exactly one content entry, an Observation, so validation binds
// cleanly. Minimal and Example must each contain at least one ELEMENT;
// a hollow tree passes the content-length check and both validators.
func TestREQ107_GenerateSocialMinimal_respectsContentUpper(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("social"))
	if err != nil {
		t.Fatalf("ParseFile social.opt: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile social.opt: %v", err)
	}
	name := "Test Composer"
	for _, tc := range []struct {
		name   string
		policy instance.Policy
	}{
		{name: "minimal", policy: instance.Minimal},
		{name: "example", policy: instance.Example},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := instance.Generate(t.Context(), c, instance.Options{
				Policy:    tc.policy,
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
			if tc.policy == instance.Minimal {
				if n := len(comp.Content); n != 1 {
					t.Fatalf("content len = %d, want 1 (existence.upper=1)", n)
				}
				if _, ok := comp.Content[0].(*rm.Observation); !ok {
					t.Fatalf("content[0] type = %T, want *rm.Observation (first colliding optional sibling)", comp.Content[0])
				}
			}
			if n := compositionElementCount(t, comp); n < 1 {
				t.Fatalf("ELEMENT count = %d, want at least 1", n)
			}
		})
	}
}

// compositionElementCount counts ELEMENT nodes in a composition the same
// way the corpus census does: canonical JSON nodes whose _type is ELEMENT.
func compositionElementCount(t *testing.T, comp *rm.Composition) int {
	t.Helper()
	raw, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var tree any
	if err := jsonv2.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	return countJSONElements(tree)
}

func countJSONElements(v any) int {
	n := 0
	switch node := v.(type) {
	case map[string]any:
		if typ, _ := node["_type"].(string); typ == "ELEMENT" {
			n++
		}
		for _, child := range node {
			n += countJSONElements(child)
		}
	case []any:
		for _, child := range node {
			n += countJSONElements(child)
		}
	}
	return n
}

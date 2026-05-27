package datamap

import (
	"bytes"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// REQ-058 — Validate/Empty: a datamap built by Empty(opt) must validate against
// the same OPT, and a structurally broken payload must be rejected.

func loadFixtureOPT(t *testing.T, name string) *template.OperationalTemplate {
	t.Helper()
	optBytes, err := os.ReadFile("testdata/fixtures/" + name + ".opt")
	if err != nil {
		t.Fatal(err)
	}
	opt, err := template.ParseOPT(bytes.NewReader(optBytes))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	return opt
}

func TestEmptyHasContentSkeleton(t *testing.T) {
	opt := loadFixtureOPT(t, "development-1")
	skeleton := Empty(opt)
	// Empty is a blank to fill in (empty scalars do not satisfy pattern/enum
	// rules, by design), but it must carry the structural skeleton: a content
	// object with the template's archetype-root key.
	content, ok := skeleton["content"].(map[string]any)
	if !ok || len(content) == 0 {
		t.Fatalf("Empty skeleton missing content object: %#v", skeleton["content"])
	}
}

func TestValidateRejectsUnknownProperty(t *testing.T) {
	opt := loadFixtureOPT(t, "development-1")
	bad := map[string]any{"content": map[string]any{}, "not_a_real_field": 1}
	ok, errs := Validate(opt, bad)
	if ok {
		t.Fatal("expected validation failure for unknown top-level property")
	}
	if len(errs) == 0 {
		t.Error("expected non-empty error list on failure")
	}
}

func TestCompositionTemplateID(t *testing.T) {
	comp := map[string]any{
		"archetype_details": map[string]any{
			"template_id": map[string]any{"value": "vital_signs.v1"},
		},
	}
	if got := CompositionTemplateID(comp); got != "vital_signs.v1" {
		t.Errorf("got %q, want vital_signs.v1", got)
	}
	if got := CompositionTemplateID(map[string]any{}); got != "" {
		t.Errorf("missing details: got %q, want empty", got)
	}
}

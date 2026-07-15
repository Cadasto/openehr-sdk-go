package simplified_test

// REQ-053 — FLAT encode: *rm.Composition -> FLAT map driven by the Web Template.
import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

const minimalObsOPT = "../../../testkit/cassettes/templates/minimal_observation.en.v1.opt"

// genComposition compiles an OPT, builds its Web Template, and synthesises an
// Example composition against it (REQ-107).
func genComposition(t *testing.T, optPath string) (*rm.Composition, *webtemplate.WebTemplate) {
	t.Helper()
	opt, err := template.ParseFile(optPath)
	if err != nil {
		t.Fatalf("parse %s: %v", optPath, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile %s: %v", optPath, err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build wt %s: %v", optPath, err)
	}
	composer := "Test Composer"
	out, err := instance.Generate(context.Background(), c, instance.Options{
		Policy:    instance.Example,
		Territory: "NL",
		Composer:  &rm.PartyIdentified{Name: &composer},
		Now:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("generate %s: %v", optPath, err)
	}
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("as composition %s: %v", optPath, err)
	}
	return comp, wt
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func TestMarshalFlatDVTextLeaf(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)

	data, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal flat: %v", err)
	}
	if len(m) == 0 {
		t.Fatal("empty flat map")
	}
	// Every clinical key is rooted at the template's tree id; ctx/ context keys
	// are the documented exception (composition-level metadata).
	for k := range m {
		if strings.HasPrefix(k, "ctx/") {
			continue
		}
		if !strings.HasPrefix(k, wt.Tree.ID+"/") {
			t.Errorf("key %q not rooted at %q", k, wt.Tree.ID)
		}
	}
	// The DV_TEXT leaf under the (single) observation/event carries its value
	// as a bare (suffix-less) FLAT entry.
	const key = "minimal/minimal:0/cualquier_evento/text"
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %q; keys=%v", key, sortedKeys(m))
	}
	if s, _ := v.(string); s == "" {
		t.Errorf("key %q = %v, want a non-empty string (DV_TEXT bare value)", key, v)
	}
}

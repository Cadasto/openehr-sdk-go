package simplified

// REQ-116 / REQ-053 — FLAT decode through a name-pinned container.
//
// Regression guard: after Phase 3 the WebTemplate carries name-predicated
// aqlPaths, but decode's placeLeaf rebuilds its lookup prefixes in the bare
// spelling (parseAQL strips predicates), so resolveLeaf must key predType /
// predIndex by the bare spelling too. Keyed by the predicated spelling, every
// lookup under a pinned container missed and concreteType fell back to a
// default RM type — GECCO's pinned EVALUATION decoded as OBSERVATION, whose
// `data` then failed as HISTORY. No prior FLAT fixture crossed a name-pinned
// container, which is why the suite stayed green through it.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func TestUnmarshalFlatPinnedContainer(t *testing.T) {
	parsed, err := template.ParseFile(fixtures.WebTemplateOpt("GECCO_Diagnose"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	flat := []byte(`{
		"ctx/language": "de",
		"ctx/territory": "DE",
		"ctx/composer_name": "Dr Test",
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|code": "261665006",
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|value": "Unknown",
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|terminology": "SNOMED-CT"
	}`)

	comp, err := UnmarshalFlat(flat, wt, WithTemplate(c))
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}

	// The pinned container must decode as its own RM type, not a fallback.
	if n := len(comp.Content); n != 1 {
		t.Fatalf("content has %d entries, want 1", n)
	}
	eval, ok := comp.Content[0].(*rm.Evaluation)
	if !ok {
		t.Fatalf("content[0] = %T, want *rm.Evaluation — predType lookup missed the pinned container", comp.Content[0])
	}
	if got := eval.ArchetypeNodeID; got != "openEHR-EHR-EVALUATION.absence.v2" {
		t.Errorf("archetype_node_id = %q, want openEHR-EHR-EVALUATION.absence.v2", got)
	}
	// WithTemplate repopulates LOCATABLE.name from the template-level pinned
	// name (REQ-116) — not the archetype concept term ("Absence of information").
	if got := eval.Name.GetValue(); got != "Unbekannte Diagnose" {
		t.Errorf("decoded name = %q, want the pinned %q", got, "Unbekannte Diagnose")
	}

	// Round-trip: the same leaf keys and values come back.
	out, err := MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("re-decode emitted FLAT: %v", err)
	}
	for key, want := range map[string]string{
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|code":        "261665006",
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|value":       "Unknown",
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|terminology": "SNOMED-CT",
	} {
		if v, ok := got[key]; !ok || v != want {
			t.Errorf("round-trip key %q = %v (present=%v), want %q", key, v, ok, want)
		}
	}
}

// Reused siblings — several pinned nodes sharing one bare path (corona's four
// SECTION.adhoc.v1) — must refuse to decode rather than silently merge one
// sibling's data into another: placement keys on the bare spelling, so their
// instances would collapse onto one list slot. The residual and its owner are
// recorded in deviations.md § Conformance.
func TestUnmarshalFlatReusedSiblingsRefused(t *testing.T) {
	parsed, err := template.ParseFile(fixtures.WebTemplateOpt("Corona_Anamnese"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Find one reused sibling by its ambiguous bare path and spell a FLAT key
	// walking through it, so the guard is exercised on the real fixture
	// rather than a synthetic id chain.
	ambiguous := ambiguousBarePaths(wt)
	if len(ambiguous) == 0 {
		t.Fatal("corona has no ambiguous bare paths any more — fixture changed?")
	}
	var chain []string
	var find func(n *webtemplate.Node, ids []string) bool
	find = func(n *webtemplate.Node, ids []string) bool {
		ids = append(ids, n.ID)
		if n.NodeID != "" && ambiguous[bareAQLPath(n.AQLPath)] {
			chain = append([]string(nil), ids...)
			return true
		}
		for _, ch := range n.Children {
			if find(ch, ids) {
				return true
			}
		}
		return false
	}
	if !find(wt.Tree, nil) {
		t.Fatal("no reachable node on an ambiguous bare path")
	}

	flat := map[string]any{
		"ctx/language":           "de",
		"ctx/territory":          "DE",
		"ctx/composer_name":      "Dr Test",
		strings.Join(chain, "/"): "x",
	}
	b, err := json.Marshal(flat)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalFlat(b, wt); err == nil {
		t.Fatal("UnmarshalFlat through a reused sibling succeeded — decode would silently merge siblings")
	} else if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "reused siblings") {
		t.Fatalf("err = %v, want the reused-siblings ErrUnknownPath refusal", err)
	}
}

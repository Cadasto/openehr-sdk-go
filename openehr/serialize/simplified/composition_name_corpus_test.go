package simplified_test

// REQ-140 — the composition's own name as the reference writes it: the
// cross-format corpus spells a renamed composition `<root>/_name` in FLAT and
// a `_name` member of the root object in STRUCTURED.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// canonicalText renders a name as canonical JSON for a comparison.
func canonicalText(t *testing.T, name rm.DVTextLike) string {
	t.Helper()
	if name == nil {
		return "<nil>"
	}
	b, err := canjson.Marshal(name)
	if err != nil {
		t.Fatalf("canjson.Marshal(name): %v", err)
	}
	return string(b)
}

// TestREQ140CompositionNameDecodesFromTheCorpus — REQ-140: the upstream FLAT
// of ehrn_abdm names its composition "Routine checkup" as
// `opconsultation/_name`, where the template root is named OPConsultation.
// Decode reads the key, so the composition carries the canonical's name and
// the key is not reported as unmatched content.
func TestREQ140CompositionNameDecodesFromTheCorpus(t *testing.T) {
	wt, compiled, flat, want := crossFormatComposerSet(t, "ehrn_abdm")
	var reported []string
	got, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled),
		simplified.WithUnmatched(func(p string) { reported = append(reported, p) }))
	if err != nil {
		t.Fatalf("UnmarshalFlat(ehrn_abdm flat.json): %v", err)
	}
	if g, w := canonicalText(t, got.Name), canonicalText(t, want.Name); g != w {
		t.Errorf("decoded name = %s, want the canonical's %s", g, w)
	}
	if key := wt.Tree.ID + "/_name"; slices.Contains(reported, key) {
		t.Errorf("WithUnmatched reported %q, want it read", key)
	}
}

// TestREQ140CompositionNameDecodesFromTheCorpusStructured — REQ-140: the
// upstream STRUCTURED of consult_record carries the same name as a `_name`
// member of the root object, and decode reads it as the FLAT key.
func TestREQ140CompositionNameDecodesFromTheCorpusStructured(t *testing.T) {
	wt, _, _, want := crossFormatComposerSet(t, "consult_record")
	doc, err := os.ReadFile(filepath.Join(fixtures.CrossFormatRoot(), "consult_record", "structured.json"))
	if err != nil {
		t.Fatalf("read consult_record structured.json: %v", err)
	}
	var reported []string
	got, err := simplified.UnmarshalStructured(doc, wt,
		simplified.WithUnmatched(func(p string) { reported = append(reported, p) }))
	if err != nil {
		t.Fatalf("UnmarshalStructured(consult_record structured.json): %v", err)
	}
	if g, w := canonicalText(t, got.Name), canonicalText(t, want.Name); g != w {
		t.Errorf("decoded name = %s, want the canonical's %s", g, w)
	}
	if member := wt.Tree.ID + "/_name"; slices.Contains(reported, member) {
		t.Errorf("WithUnmatched reported %q, want it read", member)
	}
}

// TestREQ140CompositionNameEncodesFromTheCorpus — REQ-140: the canonical
// ehrn_abdm composition is named "Routine checkup", which is not its template
// root's name, so encode writes it as `opconsultation/_name`, the key the
// upstream FLAT carries.
func TestREQ140CompositionNameEncodesFromTheCorpus(t *testing.T) {
	wt, _, upstream, comp := crossFormatComposerSet(t, "ehrn_abdm")
	data, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat(ehrn_abdm canonical): %v", err)
	}
	var got, want map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal FLAT: %v", err)
	}
	if err := json.Unmarshal(upstream, &want); err != nil {
		t.Fatalf("unmarshal upstream FLAT: %v", err)
	}
	key := wt.Tree.ID + "/_name"
	if got[key] != want[key] || got[key] == nil {
		t.Errorf("FLAT %q = %#v, want the upstream's %#v", key, got[key], want[key])
	}
}

package simplified_test

// REQ-053 / REQ-140 — the composer rides its real path when the ctx/ short
// forms cannot carry it whole (ADR 0029).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// crossFormatComposerSet loads one cross-format corpus set: the Web Template
// and the compiled template of its OPT, its upstream FLAT body and its
// canonical composition.
func crossFormatComposerSet(t *testing.T, name string) (*webtemplate.WebTemplate, *templatecompile.Compiled, []byte, *rm.Composition) {
	t.Helper()
	dir := filepath.Join(fixtures.CrossFormatRoot(), name)
	opt, err := template.ParseFile(filepath.Join(dir, "template.opt"))
	if err != nil {
		t.Fatalf("ParseFile(%s template.opt): %v", name, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", name, err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatalf("Build(%s): %v", name, err)
	}
	flat, err := os.ReadFile(filepath.Join(dir, "flat.json"))
	if err != nil {
		t.Fatalf("read %s flat.json: %v", name, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "canonical.json"))
	if err != nil {
		t.Fatalf("read %s canonical.json: %v", name, err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(raw, &comp); err != nil {
		t.Fatalf("canjson.Unmarshal(%s canonical.json): %v", name, err)
	}
	return wt, compiled, flat, &comp
}

// canonicalComposer renders a composer as canonical JSON for a failure message.
func canonicalComposer(t *testing.T, c rm.PartyProxy) string {
	t.Helper()
	if c == nil {
		return "<nil>"
	}
	b, err := canjson.Marshal(c)
	if err != nil {
		return "<" + err.Error() + ">"
	}
	return string(b)
}

// TestREQ053ComposerRealPathDecodesWhole: the reference writes a referenced
// composer at its real path, `<root>/composer|name` beside `|id`, `|id_scheme`
// and `|id_namespace`. Decode reads the whole composer there, so the
// external_ref lands on the composition exactly as the canonical form of the
// same composition holds it: a PARTY_REF of type PARTY (the reference writes
// no `|type`) whose GENERIC_ID carries the scheme.
func TestREQ053ComposerRealPathDecodesWhole(t *testing.T) {
	wt, compiled, flat, want := crossFormatComposerSet(t, "ehrn_abdm")
	got, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalFlat(ehrn_abdm flat.json): %v", err)
	}
	if !reflect.DeepEqual(got.Composer, want.Composer) {
		t.Errorf("decoded composer = %s,\nwant the canonical's %s",
			canonicalComposer(t, got.Composer), canonicalComposer(t, want.Composer))
	}
}

// TestREQ053ComposerRealPathEncodesWhole: a composer that carries an
// external_ref is not one `ctx/composer_name` can carry, so encode writes it
// whole at its real path, its name included, and writes no ctx/ composer key:
// one body spells its composer one way. The reference's type PARTY is the
// default, so no `|type` key is written.
func TestREQ053ComposerRealPathEncodesWhole(t *testing.T) {
	wt, _, _, comp := crossFormatComposerSet(t, "ehrn_abdm")
	data, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat(ehrn_abdm canonical): %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal FLAT: %v", err)
	}
	root := wt.Tree.ID
	want := map[string]any{
		root + "/composer|name":         "Max Mustermann",
		root + "/composer|id":           "e17fe1c1-5379-4ee7-a0ae-ec5f8065d261",
		root + "/composer|id_scheme":    "Practitioner",
		root + "/composer|id_namespace": "https://sbx.ayushehr.ehr.network",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("FLAT %q = %#v, want %#v", k, got[k], v)
		}
	}
	for _, k := range []string{"ctx/composer_name", "ctx/composer_self", root + "/composer_self", root + "/composer|type"} {
		if v, ok := got[k]; ok {
			t.Errorf("FLAT carries %q = %#v, want no such key for a referenced composer", k, v)
		}
	}
}

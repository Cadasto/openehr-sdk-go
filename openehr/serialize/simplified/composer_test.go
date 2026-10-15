package simplified_test

// REQ-053 / REQ-140 — the composer rides its real path when the ctx/ short
// forms cannot carry it whole (ADR 0029).

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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

// composerValue renders a decoded composer as generic canonical JSON, for a
// comparison that ignores member order.
func composerValue(t *testing.T, c rm.PartyProxy) any {
	t.Helper()
	if c == nil {
		return nil
	}
	b, err := canjson.Marshal(c)
	if err != nil {
		t.Fatalf("canjson.Marshal(composer): %v", err)
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("unmarshal composer JSON: %v", err)
	}
	return v
}

// jsonValue parses a JSON literal written in a test table.
func jsonValue(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("fixture JSON %s: %v", s, err)
	}
	return v
}

// The canonical forms the decode cases below expect.
const (
	composerRefGeneric = `{"_type":"PARTY_REF","id":{"_type":"GENERIC_ID","value":"e17fe1c1","scheme":"Practitioner"},"namespace":"https://ehr.example","type":"PARTY"}`
	composerRefHier    = `{"_type":"PARTY_REF","id":{"_type":"HIER_OBJECT_ID","value":"9f2c"},"namespace":"demographic","type":"PARTY"}`
)

// TestREQ053ComposerRealPathDecodes — REQ-053 / REQ-140, ADR 0029. The
// composer's real path carries the whole composer in the party grammar: its
// external_ref (scheme present ⇒ GENERIC_ID, an absent `|type` ⇒ PARTY), its
// `_identifier:N` list and a PARTY_RELATED's `/relationship`. The name from
// `ctx/composer_name` or its `composer|name` alias merges with the rest; a
// `composer_self: true` in either spelling beside a reference is a PARTY_SELF
// carrying it; the `:0` spellings read as the unindexed ones; and a null key
// is absent. Strict decode proves every key is read. The STRUCTURED form of the
// same body decodes to the same composer, and so does the FLAT that OPT-free
// conversion writes back from it, so the conversion carries every composer key.
func TestREQ053ComposerRealPathDecodes(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root := wt.Tree.ID
	for _, tc := range []struct {
		name string
		keys map[string]any
		want string
		// flatOnly marks a body that spells one key twice, which has one
		// STRUCTURED spelling, so the STRUCTURED leg cannot carry it.
		flatOnly bool
	}{
		{
			name: "named and referenced",
			keys: map[string]any{
				root + "/composer|name": "Max Mustermann", root + "/composer|id": "e17fe1c1",
				root + "/composer|id_scheme": "Practitioner", root + "/composer|id_namespace": "https://ehr.example",
			},
			want: `{"_type":"PARTY_IDENTIFIED","name":"Max Mustermann","external_ref":` + composerRefGeneric + `}`,
		},
		{
			name: "ctx/composer_name merges with the real-path reference",
			keys: map[string]any{
				"ctx/composer_name": "Dr Ctx", root + "/composer|id": "9f2c", root + "/composer|id_namespace": "demographic",
			},
			want: `{"_type":"PARTY_IDENTIFIED","name":"Dr Ctx","external_ref":` + composerRefHier + `}`,
		},
		{
			name: "a reference typed PERSON keeps its type",
			keys: map[string]any{
				root + "/composer|id": "9f2c", root + "/composer|id_namespace": "demographic", root + "/composer|type": "PERSON",
			},
			want: `{"_type":"PARTY_IDENTIFIED","external_ref":{"_type":"PARTY_REF","id":{"_type":"HIER_OBJECT_ID","value":"9f2c"},"namespace":"demographic","type":"PERSON"}}`,
		},
		{
			name: "identifiers only",
			keys: map[string]any{
				root + "/composer/_identifier:0|id": "122", root + "/composer/_identifier:0|issuer": "Hospital",
				root + "/composer/_identifier:0|assigner": "Registry", root + "/composer/_identifier:0|type": "MRN",
				root + "/composer/_identifier:1|id": "77",
			},
			want: `{"_type":"PARTY_IDENTIFIED","identifiers":[{"_type":"DV_IDENTIFIER","issuer":"Hospital","assigner":"Registry","id":"122","type":"MRN"},{"_type":"DV_IDENTIFIER","id":"77"}]}`,
		},
		{
			name: "PARTY_RELATED",
			keys: map[string]any{
				"ctx/composer_name": "Anna", root + "/composer/relationship|code": "10",
				root + "/composer/relationship|value": "mother", root + "/composer/relationship|terminology": "openehr",
			},
			want: `{"_type":"PARTY_RELATED","name":"Anna","relationship":{"_type":"DV_CODED_TEXT","value":"mother","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"10"}}}`,
		},
		{
			name: "ctx/composer_self beside a reference",
			keys: map[string]any{
				"ctx/composer_self": true, root + "/composer|id": "9f2c", root + "/composer|id_namespace": "demographic",
			},
			want: `{"_type":"PARTY_SELF","external_ref":` + composerRefHier + `}`,
		},
		{
			name: "real-path composer_self beside a reference",
			keys: map[string]any{
				root + "/composer_self": true, root + "/composer|id": "e17fe1c1",
				root + "/composer|id_scheme": "Practitioner", root + "/composer|id_namespace": "https://ehr.example",
			},
			want: `{"_type":"PARTY_SELF","external_ref":` + composerRefGeneric + `}`,
		},
		{
			name: "composer_self false beside a reference",
			keys: map[string]any{
				"ctx/composer_self": false, root + "/composer|id": "9f2c", root + "/composer|id_namespace": "demographic",
			},
			want: `{"_type":"PARTY_IDENTIFIED","external_ref":` + composerRefHier + `}`,
		},
		{
			name: "indexed spelling",
			keys: map[string]any{
				root + ":0/composer:0|name": "Max Mustermann", root + ":0/composer:0|id": "e17fe1c1",
				root + "/composer:0|id_scheme": "Practitioner", root + ":0/composer|id_namespace": "https://ehr.example",
				root + ":0/composer:0/_identifier:0|id": "122", root + "/composer:0/relationship:0|code": "10",
				root + "/composer:0/relationship:0|value": "mother", root + "/composer:0/relationship:0|terminology": "openehr",
			},
			want: `{"_type":"PARTY_RELATED","name":"Max Mustermann","external_ref":` + composerRefGeneric + `,` +
				`"identifiers":[{"_type":"DV_IDENTIFIER","id":"122"}],` +
				`"relationship":{"_type":"DV_CODED_TEXT","value":"mother","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"10"}}}`,
		},
		{
			name: "two agreeing spellings of one key",
			keys: map[string]any{
				root + "/composer|id": "9f2c", root + ":0/composer:0|id": "9f2c",
				root + "/composer|id_namespace": "demographic",
			},
			want:     `{"_type":"PARTY_IDENTIFIED","external_ref":` + composerRefHier + `}`,
			flatOnly: true,
		},
		{
			name: "a null spelling beside a valued one",
			keys: map[string]any{
				root + "/composer|id": nil, root + ":0/composer:0|id": "9f2c",
				root + "/composer|id_namespace": "demographic",
			},
			want:     `{"_type":"PARTY_IDENTIFIED","external_ref":` + composerRefHier + `}`,
			flatOnly: true,
		},
		{
			name: "null keys are absent",
			keys: map[string]any{
				"ctx/composer_name": "Dr Ctx", root + "/composer|id": nil, root + "/composer|id_scheme": nil,
				root + "/composer|id_namespace": nil, root + "/composer|type": nil,
				root + "/composer/_identifier:0|id": nil, root + "/composer/relationship|code": nil,
			},
			want: `{"_type":"PARTY_IDENTIFIED","name":"Dr Ctx"}`,
		},
		{
			name: "null keys beside composer_self are absent",
			keys: map[string]any{
				"ctx/composer_self": true, root + "/composer/_identifier:0|id": nil,
				root + "/composer/relationship|code": nil, root + "/composer|id": nil,
			},
			want: `{"_type":"PARTY_SELF"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := indexedMetaBase()
			maps.Copy(body, tc.keys)
			flat := marshalBody(t, body)
			want := jsonValue(t, tc.want)
			got, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalFlat (strict): %v", err)
			}
			if g := composerValue(t, got.Composer); !reflect.DeepEqual(g, want) {
				t.Errorf("FLAT composer = %s,\nwant %s", canonicalComposer(t, got.Composer), tc.want)
			}
			if tc.flatOnly {
				return
			}
			structured, err := simplified.FlatToStructured(flat)
			if err != nil {
				t.Fatalf("FlatToStructured: %v", err)
			}
			got, err = simplified.UnmarshalStructured(structured, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalStructured (strict): %v", err)
			}
			if g := composerValue(t, got.Composer); !reflect.DeepEqual(g, want) {
				t.Errorf("STRUCTURED composer = %s,\nwant %s", canonicalComposer(t, got.Composer), tc.want)
			}
			back, err := simplified.StructuredToFlat(structured)
			if err != nil {
				t.Fatalf("StructuredToFlat: %v", err)
			}
			got, err = simplified.UnmarshalFlat(back, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalFlat (strict) of the FLAT converted back: %v", err)
			}
			if g := composerValue(t, got.Composer); !reflect.DeepEqual(g, want) {
				t.Errorf("FLAT -> STRUCTURED -> FLAT composer = %s,\nwant %s", canonicalComposer(t, got.Composer), tc.want)
			}
		})
	}
}

// TestREQ053ComposerRealPathRefusals — REQ-053 / REQ-140, ADR 0029. The
// composer's real path is under the party grammar's refusals and the
// metadata rules: a `composer_self: true` beside an identifier or a
// relationship (PARTY_SELF has neither), two spellings of one key that
// disagree (ErrUnknownPath, as for every other metadata field), a reference
// missing its `|id` or its RM-mandatory namespace, an empty name, a
// relationship with nothing it qualifies, a value that does not fit its
// attribute, and an index past its bound. Each refusal names the key and none
// carries the value.
func TestREQ053ComposerRealPathRefusals(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root := wt.Tree.ID
	const secret = "secret-composer-0451"
	for _, tc := range []struct {
		name  string
		keys  map[string]any
		want  error
		named []string
		// unsaid are words the refusal must not use: an own suffix such as
		// `|id` is no sub-path.
		unsaid []string
	}{
		{
			name:  "ctx/composer_self beside an identifier",
			keys:  map[string]any{"ctx/composer_self": true, root + "/composer/_identifier:0|id": secret},
			want:  simplified.ErrUnknownPath,
			named: []string{"ctx/composer_self", root + "/composer/_identifier:0|id"},
		},
		{
			name: "real-path composer_self beside a relationship",
			keys: map[string]any{
				root + "/composer_self": true, root + "/composer/relationship|code": "10",
				root + "/composer/relationship|value": secret, root + "/composer|id": "9f2c",
				root + "/composer|id_namespace": "demographic",
			},
			want:  simplified.ErrUnknownPath,
			named: []string{root + "/composer_self", root + "/composer/relationship|code"},
		},
		// Two spellings of one composer key that disagree are two accepted
		// spellings of one composition-metadata field: ErrUnknownPath, whichever
		// key it is and whichever segment carries the `:0` (REQ-053).
		{
			name:  "two spellings of the name that disagree",
			keys:  map[string]any{root + "/composer|name": "Dr A", root + ":0/composer:0|name": "Dr B"},
			want:  simplified.ErrUnknownPath,
			named: []string{root + "/composer|name", root + ":0/composer:0|name"},
		},
		{
			name:   "two spellings of the id that disagree",
			keys:   map[string]any{root + "/composer|id": secret, root + ":0/composer:0|id": "other", root + "/composer|id_namespace": "demographic"},
			want:   simplified.ErrUnknownPath,
			named:  []string{root + "/composer|id", root + ":0/composer:0|id"},
			unsaid: []string{"sub-path"},
		},
		{
			name: "two spellings of an identifier's id that disagree",
			keys: map[string]any{
				"ctx/composer_name": "Dr Ctx", root + "/composer/_identifier:0|id": secret,
				root + ":0/composer:0/_identifier:0|id": "other",
			},
			want:  simplified.ErrUnknownPath,
			named: []string{root + "/composer/_identifier:0|id", root + ":0/composer:0/_identifier:0|id"},
		},
		{
			name: "an identifier with and without its :0 that disagree",
			keys: map[string]any{
				"ctx/composer_name": "Dr Ctx", root + "/composer/_identifier|id": secret,
				root + "/composer/_identifier:0|id": "other",
			},
			want:  simplified.ErrUnknownPath,
			named: []string{root + "/composer/_identifier|id", root + "/composer/_identifier:0|id"},
		},
		{
			name: "a relationship with and without its :0 that disagree",
			keys: map[string]any{
				"ctx/composer_name": "Dr Ctx", root + "/composer/relationship|code": "10",
				root + "/composer/relationship|value": secret, root + "/composer/relationship:0|value": "other",
			},
			want:  simplified.ErrUnknownPath,
			named: []string{root + "/composer/relationship|value", root + "/composer/relationship:0|value"},
		},
		{
			name:  "a reference without its namespace",
			keys:  map[string]any{root + "/composer|id": secret},
			want:  simplified.ErrUnsupportedDatatype,
			named: []string{root + "/composer", "|id_namespace"},
		},
		{
			name:  "a scheme without an id",
			keys:  map[string]any{root + "/composer|id_scheme": secret},
			want:  simplified.ErrUnsupportedDatatype,
			named: []string{root + "/composer", "|id_scheme"},
		},
		{
			name:  "an empty name beside a reference",
			keys:  map[string]any{"ctx/composer_name": "", root + "/composer|id": secret, root + "/composer|id_namespace": "demographic"},
			want:  simplified.ErrUnsupportedDatatype,
			named: []string{"ctx/composer_name", "Name_valid"},
		},
		{
			name:  "a relationship with nothing it qualifies",
			keys:  map[string]any{root + "/composer/relationship|code": "10", root + "/composer/relationship|value": secret},
			want:  simplified.ErrUnsupportedDatatype,
			named: []string{root + "/composer", "Basic_validity"},
		},
		{
			name:  "an id that is not a string",
			keys:  map[string]any{root + "/composer|id": 4510, root + "/composer|id_namespace": "demographic"},
			want:  simplified.ErrUnsupportedDatatype,
			named: []string{root + "/composer|id"},
		},
		{
			name:  "an identifier value that does not fit",
			keys:  map[string]any{"ctx/composer_name": "Dr Ctx", root + "/composer/_identifier:0|id": 4510},
			want:  simplified.ErrUnsupportedDatatype,
			named: []string{root + "/composer/_identifier:0|id"},
		},
		{
			name:  "an identifier past its bound",
			keys:  map[string]any{"ctx/composer_name": "Dr Ctx", root + "/composer/_identifier:10001|id": secret},
			want:  simplified.ErrUnknownPath,
			named: []string{root + "/composer/_identifier:10001|id"},
		},
		{
			name:  "an indexed relationship",
			keys:  map[string]any{"ctx/composer_name": "Dr Ctx", root + "/composer/relationship:1|code": secret},
			want:  simplified.ErrUnsupportedDatatype,
			named: []string{root + "/composer/relationship:1|code"},
		},
		{
			name:  "the composer past :0",
			keys:  map[string]any{root + "/composer:1|id": secret},
			want:  simplified.ErrUnknownPath,
			named: []string{root + "/composer:1|id"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := indexedMetaBase()
			maps.Copy(body, tc.keys)
			flat := marshalBody(t, body)
			_, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled))
			wantRefusal(t, err, tc.want, tc.named...)
			if strings.Contains(err.Error(), secret) {
				t.Errorf("err = %v carries the value", err)
			}
			for _, w := range tc.unsaid {
				if strings.Contains(err.Error(), w) {
					t.Errorf("err = %v says %q", err, w)
				}
			}
		})
	}
}

// TestREQ053ComposerRealPathUnmatched — REQ-170 / REQ-140. At the composer's
// real path, as at every other party position, a key the party grammar does
// not read is unmatched content: a bare value, an unknown suffix and a
// sub-path the grammar has no member for. It is ignored by default, reported
// under WithUnmatched and refused under strict, and the composer the other
// keys spell still decodes.
func TestREQ053ComposerRealPathUnmatched(t *testing.T) {
	wt, compiled := corpusTarget(t)
	root := wt.Tree.ID
	for _, key := range []string{
		root + "/composer",
		root + "/composer|x_note",
		root + "/composer/_identifier:0/x_member|id",
		root + "/composer/relationship|x_note",
	} {
		t.Run(key, func(t *testing.T) {
			body := indexedMetaBase()
			body["ctx/composer_name"] = "Dr Ctx"
			body[key] = "x"
			flat := marshalBody(t, body)
			var reported []string
			got, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled),
				simplified.WithUnmatched(func(p string) { reported = append(reported, p) }))
			if err != nil {
				t.Fatalf("UnmarshalFlat (WithUnmatched): %v", err)
			}
			if !slices.Equal(reported, []string{key}) {
				t.Errorf("reported %q, want [%q]", reported, key)
			}
			if want := jsonValue(t, `{"_type":"PARTY_IDENTIFIED","name":"Dr Ctx"}`); !reflect.DeepEqual(composerValue(t, got.Composer), want) {
				t.Errorf("composer = %s, want the name alone", canonicalComposer(t, got.Composer))
			}
			if _, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled), simplified.WithStrict()); !errors.Is(err, simplified.ErrUnknownPath) {
				t.Errorf("UnmarshalFlat (strict) = %v, want ErrUnknownPath", err)
			}
		})
	}
}

// TestREQ053ComposerRealPathIsOneSegment — REQ-053, ADR 0029. The composer's
// real path is the `composer` segment itself, followed by a suffix or a
// sub-path. A Web Template node whose FLAT id only starts with "composer" is
// content like any other node: strict decode reads its keys at their own path
// and they re-encode unchanged, none of them taken for a composer key.
func TestREQ053ComposerRealPathIsOneSegment(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	var renamed *webtemplate.Node
	for _, n := range wt.Tree.Children {
		if n.RMType == "OBSERVATION" {
			renamed = n
			break
		}
	}
	if renamed == nil {
		t.Fatalf("fixture: %s has no OBSERVATION under the template root", minimalObsOPT)
	}
	renamed.ID = "composer_notes"
	flat, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var want map[string]any
	if err := json.Unmarshal(flat, &want); err != nil {
		t.Fatal(err)
	}
	prefix := wt.Tree.ID + "/" + renamed.ID
	if !slices.ContainsFunc(slices.Collect(maps.Keys(want)), func(k string) bool { return strings.HasPrefix(k, prefix) }) {
		t.Fatalf("fixture: MarshalFlat wrote no key under %s: %v", prefix, slices.Sorted(maps.Keys(want)))
	}
	got, err := simplified.UnmarshalFlat(flat, wt, simplified.WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat (strict): %v", err)
	}
	again, err := simplified.MarshalFlat(got, wt)
	if err != nil {
		t.Fatalf("MarshalFlat (re-encode): %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(again, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, want) {
		t.Errorf("re-encoded FLAT = %v,\nwant %v", back, want)
	}
}

// TestREQ053ComposerEncodeSpellings — REQ-053, ADR 0029. Encode writes a
// composer one ctx/ short form carries whole in that form, and every other
// composer whole at its real path with no ctx/ composer key, so one body spells
// its composer one way: the composer keys of the output are exactly the ones
// listed. The reference's `|type` is written only when it is not PARTY. FLAT
// and STRUCTURED decode each output back to the composer encoded.
func TestREQ053ComposerEncodeSpellings(t *testing.T) {
	name := "Dr Enc"
	generic := &rm.PartyRef{ID: &rm.GenericID{Value: "e17fe1c1", Scheme: "Practitioner"}, Namespace: "https://ehr.example", Type: "PARTY"}
	person := &rm.PartyRef{ID: &rm.HierObjectID{Value: "9f2c"}, Namespace: "demographic", Type: "PERSON"}
	issuer := "Hospital"
	relationship := rm.DVCodedText{
		Value:        "mother",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "10"},
	}
	for _, tc := range []struct {
		name     string
		composer rm.PartyProxy
		// keys holds the composer keys the output carries, root-relative
		// where they start with "/".
		keys map[string]any
	}{
		{"PARTY_SELF", &rm.PartySelf{}, map[string]any{"ctx/composer_self": true}},
		{"PARTY_IDENTIFIED with a name alone", &rm.PartyIdentified{Name: &name}, map[string]any{"ctx/composer_name": name}},
		{
			"PARTY_IDENTIFIED with a name and a reference",
			&rm.PartyIdentified{Name: &name, ExternalRef: generic},
			map[string]any{
				"/composer|name": name, "/composer|id": "e17fe1c1",
				"/composer|id_scheme": "Practitioner", "/composer|id_namespace": "https://ehr.example",
			},
		},
		{
			"PARTY_IDENTIFIED with a reference alone, typed PERSON",
			&rm.PartyIdentified{ExternalRef: person},
			map[string]any{"/composer|id": "9f2c", "/composer|id_namespace": "demographic", "/composer|type": "PERSON"},
		},
		{
			"PARTY_IDENTIFIED with identifiers alone",
			&rm.PartyIdentified{Identifiers: []rm.DVIdentifier{{ID: "122", Issuer: &issuer}, {ID: "77"}}},
			map[string]any{
				"/composer/_identifier:0|id": "122", "/composer/_identifier:0|issuer": issuer,
				"/composer/_identifier:1|id": "77",
			},
		},
		{
			"PARTY_RELATED",
			&rm.PartyRelated{Name: &name, Relationship: relationship},
			map[string]any{
				"/composer|name": name, "/composer/relationship|code": "10",
				"/composer/relationship|value": "mother", "/composer/relationship|terminology": "openehr",
			},
		},
		{
			"PARTY_SELF carrying a reference",
			&rm.PartySelf{ExternalRef: person},
			map[string]any{
				"/composer_self": true, "/composer|id": "9f2c",
				"/composer|id_namespace": "demographic", "/composer|type": "PERSON",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp, wt := genComposition(t, minimalObsOPT)
			comp.Composer = tc.composer
			root := wt.Tree.ID
			want := make(map[string]any, len(tc.keys))
			for k, v := range tc.keys {
				if strings.HasPrefix(k, "/") {
					k = root + k
				}
				want[k] = v
			}
			flat, err := simplified.MarshalFlat(comp, wt)
			if err != nil {
				t.Fatalf("MarshalFlat: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(flat, &m); err != nil {
				t.Fatal(err)
			}
			got := make(map[string]any)
			for k, v := range m {
				if strings.HasPrefix(k, "ctx/composer") || strings.HasPrefix(k, root+"/composer") {
					got[k] = v
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("composer keys = %v,\nwant %v", got, want)
			}
			back, err := simplified.UnmarshalFlat(flat, wt)
			if err != nil {
				t.Fatalf("UnmarshalFlat: %v", err)
			}
			if !reflect.DeepEqual(composerValue(t, back.Composer), composerValue(t, tc.composer)) {
				t.Errorf("FLAT round trip composer = %s, want %s", canonicalComposer(t, back.Composer), canonicalComposer(t, tc.composer))
			}
			structured, err := simplified.MarshalStructured(comp, wt)
			if err != nil {
				t.Fatalf("MarshalStructured: %v", err)
			}
			back, err = simplified.UnmarshalStructured(structured, wt, simplified.WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalStructured (strict): %v", err)
			}
			if !reflect.DeepEqual(composerValue(t, back.Composer), composerValue(t, tc.composer)) {
				t.Errorf("STRUCTURED round trip composer = %s, want %s", canonicalComposer(t, back.Composer), canonicalComposer(t, tc.composer))
			}
		})
	}
}

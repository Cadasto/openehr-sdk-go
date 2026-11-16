package simplified_test

// REQ-053 / REQ-170 — the STRUCTURED spelling of a bare value that sits beside
// its leaf's suffixes: encode writes the `|value` member, the reference's
// spelling (ADR 0014), and decode reads `|value`, `"|"` and `""` as that bare
// value, refusing two spellings of it that disagree.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// bareValueTemplates are the corpus templates whose `any` leaf holds a bare
// value beside the suffixes its datatype requires: a DV_PARSABLE beside its
// |formalism, and a DV_MULTIMEDIA's uri beside its |mediatype and |size.
var bareValueTemplates = []string{
	"Test_dv_parsable_open_constraint.v0",
	"Test_dv_multimedia_open_constraint.v0",
}

// bareValueSpellings are the STRUCTURED members that hold a bare value beside
// suffixes: the reference's, this codec's earlier one, and the empty name.
var bareValueSpellings = []string{"|value", "|", ""}

// spellingLabel names a bare value spelling in a subtest name.
func spellingLabel(spelling string) string {
	switch spelling {
	case "|":
		return "pipe member"
	case "":
		return "empty member"
	}
	return spelling + " member"
}

// bareValueLeaf is the leaf id of the bare value in [bareValueTemplates].
const bareValueLeaf = "any"

// bareValueTarget is one of [bareValueTemplates]: its Web Template, its corpus
// composition, and that composition in FLAT and in STRUCTURED.
type bareValueTarget struct {
	wt   *webtemplate.WebTemplate
	comp *rm.Composition
	flat []byte
	doc  []byte
}

func loadBareValueTarget(t *testing.T, id string) bareValueTarget {
	t.Helper()
	opt, err := readOPTFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatalf("readOPTFile(%s): %v", id, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", id, err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatalf("Build(%s): %v", id, err)
	}
	raw, err := os.ReadFile(fixtures.CompositionJSON(id))
	if err != nil {
		t.Fatalf("read %s composition: %v", id, err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(raw, &comp); err != nil {
		t.Fatalf("canjson.Unmarshal(%s): %v", id, err)
	}
	flat, err := simplified.MarshalFlat(&comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat(%s): %v", id, err)
	}
	doc, err := simplified.MarshalStructured(&comp, wt)
	if err != nil {
		t.Fatalf("MarshalStructured(%s): %v", id, err)
	}
	return bareValueTarget{wt: wt, comp: &comp, flat: flat, doc: doc}
}

// leafElements collects the object elements of every member named id in v, a
// decoded STRUCTURED document.
func leafElements(v any, id string) []map[string]any {
	var out []map[string]any
	switch node := v.(type) {
	case map[string]any:
		for name, member := range node {
			if arr, isArray := member.([]any); isArray && name == id {
				for _, el := range arr {
					if obj, isObject := el.(map[string]any); isObject {
						out = append(out, obj)
					}
				}
				continue
			}
			out = append(out, leafElements(member, id)...)
		}
	case []any:
		for _, el := range node {
			out = append(out, leafElements(el, id)...)
		}
	}
	return out
}

// respell rewrites doc, a STRUCTURED document, so that every element of the
// bareValueLeaf member holds its bare value under each of names, in that
// order, with values[i] in names[i]; a value of nil keeps the element's own.
func respell(t *testing.T, doc []byte, names []string, values []any) []byte {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(doc, &root); err != nil {
		t.Fatalf("unmarshal STRUCTURED: %v", err)
	}
	els := leafElements(root, bareValueLeaf)
	if len(els) == 0 {
		t.Fatalf("no %q element in %s", bareValueLeaf, doc)
	}
	for _, el := range els {
		var bare any
		found := 0
		for _, name := range bareValueSpellings {
			if v, ok := el[name]; ok {
				bare = v
				found++
				delete(el, name)
			}
		}
		if found != 1 {
			t.Fatalf("element %v holds %d bare value spellings, want 1", el, found)
		}
		for i, name := range names {
			el[name] = bare
			if values[i] != nil {
				el[name] = values[i]
			}
		}
	}
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// canonicalOf renders a decoded composition as canonical JSON.
func canonicalOf(t *testing.T, comp *rm.Composition) []byte {
	t.Helper()
	b, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	return b
}

// TestREQ053StructuredBareValueIsTheValueMember — REQ-053: a bare value beside
// its leaf's suffixes is the element's `|value` member, as the reference
// writes it, and neither `"|"` nor `""`. MarshalStructured and the OPT-free
// FlatToStructured write the same document.
func TestREQ053StructuredBareValueIsTheValueMember(t *testing.T) {
	for _, id := range bareValueTemplates {
		t.Run(id, func(t *testing.T) {
			target := loadBareValueTarget(t, id)
			converted, err := simplified.FlatToStructured(target.flat)
			if err != nil {
				t.Fatalf("FlatToStructured: %v", err)
			}
			for name, doc := range map[string][]byte{"MarshalStructured": target.doc, "FlatToStructured": converted} {
				var root map[string]any
				if err := json.Unmarshal(doc, &root); err != nil {
					t.Fatalf("%s: unmarshal: %v", name, err)
				}
				els := leafElements(root, bareValueLeaf)
				if len(els) == 0 {
					t.Fatalf("%s: no %q element in %s", name, bareValueLeaf, doc)
				}
				for _, el := range els {
					_, value := el["|value"]
					_, pipe := el["|"]
					_, empty := el[""]
					if !value || pipe || empty {
						t.Errorf("%s: element %v, want the bare value under |value alone", name, el)
					}
				}
			}
			if !bytes.Equal(converted, target.doc) {
				t.Errorf("FlatToStructured = %s\nwant what MarshalStructured writes, %s", converted, target.doc)
			}
		})
	}
}

// TestREQ053StructuredBareValueReadsEverySpelling — REQ-053 / REQ-170: decode
// reads `|value`, `"|"` and `""` as the bare value beside the leaf's suffixes,
// so a strict decode reports none of them as unmatched and gives the
// composition the FLAT body gives. The OPT-free StructuredToFlat gives a FLAT
// body that decodes the same way.
func TestREQ053StructuredBareValueReadsEverySpelling(t *testing.T) {
	for _, id := range bareValueTemplates {
		target := loadBareValueTarget(t, id)
		want, err := simplified.UnmarshalFlat(target.flat, target.wt, simplified.WithStrict())
		if err != nil {
			t.Fatalf("%s: UnmarshalFlat: %v", id, err)
		}
		for _, spelling := range bareValueSpellings {
			t.Run(id+"/"+spellingLabel(spelling), func(t *testing.T) {
				doc := respell(t, target.doc, []string{spelling}, []any{nil})
				got, err := simplified.UnmarshalStructured(doc, target.wt, simplified.WithStrict())
				if err != nil {
					t.Fatalf("UnmarshalStructured(strict) = %v", err)
				}
				if g, w := canonicalOf(t, got), canonicalOf(t, want); !bytes.Equal(g, w) {
					t.Errorf("UnmarshalStructured =\n%s\nwant\n%s", g, w)
				}
				flat, err := simplified.StructuredToFlat(doc)
				if err != nil {
					t.Fatalf("StructuredToFlat = %v", err)
				}
				viaFlat, err := simplified.UnmarshalFlat(flat, target.wt, simplified.WithStrict())
				if err != nil {
					t.Fatalf("UnmarshalFlat(StructuredToFlat(doc), strict) = %v", err)
				}
				if g, w := canonicalOf(t, viaFlat), canonicalOf(t, want); !bytes.Equal(g, w) {
					t.Errorf("UnmarshalFlat(StructuredToFlat(doc)) =\n%s\nwant\n%s", g, w)
				}
			})
		}
	}
}

// TestREQ053StructuredBareValueSpellingsThatDisagreeAreRefused — REQ-053: two
// spellings of one bare value are read once where they agree, and refused
// with ErrUnknownPath where they differ, in the default mode too, naming no
// value. The OPT-free StructuredToFlat refuses the pair it would fold onto
// one FLAT key, and gives the FLAT decode the other pairs to refuse.
func TestREQ053StructuredBareValueSpellingsThatDisagreeAreRefused(t *testing.T) {
	const secret = "SECRET-BARE-0454"
	pairs := [][]string{{"|value", "|"}, {"|value", ""}, {"|", ""}}
	for _, id := range bareValueTemplates {
		target := loadBareValueTarget(t, id)
		want, err := simplified.UnmarshalFlat(target.flat, target.wt)
		if err != nil {
			t.Fatalf("%s: UnmarshalFlat: %v", id, err)
		}
		for _, pair := range pairs {
			name := id + "/" + spellingLabel(pair[0]) + " and " + spellingLabel(pair[1])
			t.Run(name+"/agree", func(t *testing.T) {
				doc := respell(t, target.doc, pair, []any{nil, nil})
				got, err := simplified.UnmarshalStructured(doc, target.wt, simplified.WithStrict())
				if err != nil {
					t.Fatalf("UnmarshalStructured(strict) = %v, want the two spellings read once", err)
				}
				if g, w := canonicalOf(t, got), canonicalOf(t, want); !bytes.Equal(g, w) {
					t.Errorf("UnmarshalStructured =\n%s\nwant\n%s", g, w)
				}
			})
			t.Run(name+"/disagree", func(t *testing.T) {
				doc := respell(t, target.doc, pair, []any{nil, secret})
				_, err := simplified.UnmarshalStructured(doc, target.wt)
				if err == nil {
					t.Fatal("UnmarshalStructured = nil error, want ErrUnknownPath")
				}
				if !errors.Is(err, simplified.ErrUnknownPath) {
					t.Errorf("UnmarshalStructured = %v, want ErrUnknownPath", err)
				}
				if strings.Contains(err.Error(), secret) {
					t.Errorf("UnmarshalStructured = %v carries a value", err)
				}
				flat, err := simplified.StructuredToFlat(doc)
				if err == nil {
					_, err = simplified.UnmarshalFlat(flat, target.wt)
				}
				if !errors.Is(err, simplified.ErrUnknownPath) {
					t.Errorf("StructuredToFlat then UnmarshalFlat = %v, want ErrUnknownPath", err)
				}
				if err != nil && strings.Contains(err.Error(), secret) {
					t.Errorf("StructuredToFlat then UnmarshalFlat = %v carries a value", err)
				}
			})
		}
	}
}

// TestREQ053StructuredBareValueBesideItsOwnValueKeepsThePipe — REQ-053: two
// values never share one member. Where a FLAT leaf carries both a bare value
// and a `|value` of its own, as a coded text's rubric is, FLAT → STRUCTURED
// keeps the bare value as the `"|"` member beside the `|value` one, whether or
// not the two agree, and STRUCTURED → FLAT gives both keys back as written.
func TestREQ053StructuredBareValueBesideItsOwnValueKeepsThePipe(t *testing.T) {
	for name, tc := range map[string]struct{ flat, want string }{
		"values differ": {
			`{"t/e:0":"bare","t/e:0|code":"c","t/e:0|value":"rubric"}`,
			`{"t":{"e":[{"|":"bare","|code":"c","|value":"rubric"}]}}`,
		},
		"values agree": {
			`{"t/e:0":"same","t/e:0|value":"same"}`,
			`{"t":{"e":[{"|":"same","|value":"same"}]}}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := simplified.FlatToStructured([]byte(tc.flat))
			if err != nil {
				t.Fatalf("FlatToStructured(%s) = %v", tc.flat, err)
			}
			if string(doc) != tc.want {
				t.Errorf("FlatToStructured(%s) = %s, want %s", tc.flat, doc, tc.want)
			}
			back, err := simplified.StructuredToFlat(doc)
			if err != nil {
				t.Fatalf("StructuredToFlat(%s) = %v", doc, err)
			}
			if string(back) != tc.flat {
				t.Errorf("StructuredToFlat(%s) = %s, want %s", doc, back, tc.flat)
			}
		})
	}
}

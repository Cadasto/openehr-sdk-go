package simplified_test

// REQ-053 — the formats have no key for a node's name below the root, so a
// WithTemplate decode gives a node whose name the template constrains as a
// DV_CODED_TEXT the coded name the generator writes there. Over every
// COMPOSITION OPT the PROBE-076 skeleton leg runs, each coded name of the
// Medium and the Large skeleton comes back unchanged at its node through a
// FLAT and a STRUCTURED round trip.

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// codedNameCorpusFloor is the number of COMPOSITION templates the corpus gives
// today. A corpus that silently empties, or a COMPOSITION OPT that stops
// parsing, falls below it. Raise it when the corpus grows.
const codedNameCorpusFloor = 75

// reusedSiblingTemplates are the corpus templates whose skeleton encode
// refuses, as deviations.md registers it (§ Archetype-reuse siblings,
// residual: siblings no name tells apart), because data resolves at reused
// siblings no name tells apart. Such a refusal wraps rmpath.ErrPathAmbiguous;
// the guard skips that format of that template, and fails on any other
// encode error.
var reusedSiblingTemplates = []string{"sdk_crosscheck_siblings.v1"}

// corpusCompositionOPT is one COMPOSITION OPT of the corpus: its template id
// and the path of its file.
type corpusCompositionOPT struct {
	id, path string
}

// corpusCompositionOPTs returns the OPTs the PROBE-076 skeleton leg runs:
// every one fixtures.ListAllOPTs lists that parses and whose root is
// COMPOSITION, one per template id, in the order the listing gives.
func corpusCompositionOPTs(t *testing.T) []corpusCompositionOPT {
	t.Helper()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	seen := make(map[string]bool)
	var out []corpusCompositionOPT
	for _, ref := range refs {
		opt, err := readOPTFile(ref.Path)
		if err != nil {
			continue
		}
		if strings.TrimSpace(opt.Definition.RMTypeName) != "COMPOSITION" {
			continue
		}
		id := strings.TrimSpace(opt.TemplateID.Value)
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, corpusCompositionOPT{id: id, path: ref.Path})
	}
	return out
}

// TestREQ053CorpusCodedNamesRoundTrip — REQ-053, PROBE-076: for every
// COMPOSITION OPT of the corpus, under instance.Medium and instance.Large,
// every DV_CODED_TEXT name the skeleton writes equals the name a WithTemplate
// decode gives the same node after a FLAT and after a STRUCTURED round trip.
func TestREQ053CorpusCodedNamesRoundTrip(t *testing.T) {
	opts := corpusCompositionOPTs(t)
	if len(opts) < codedNameCorpusFloor {
		t.Errorf("the corpus gives %d COMPOSITION templates, want at least %d; did the corpus shrink?", len(opts), codedNameCorpusFloor)
	}
	compared := 0
	for _, o := range opts {
		for _, policy := range []instance.Policy{instance.Medium, instance.Large} {
			t.Run(o.id+"/"+policy.String(), func(t *testing.T) {
				opt, err := readOPTFile(o.path)
				if err != nil {
					t.Fatalf("readOPTFile(%s): %v", o.path, err)
				}
				wt, compiled := compileTemplate(t, o.id, opt)
				comp := skeletonOf(t, compiled, policy)
				written := codedNames(canonicalDoc(t, comp))
				compared += len(written)
				for _, format := range []string{"FLAT", "STRUCTURED"} {
					encode, decode := simplified.MarshalFlat, simplified.UnmarshalFlat
					if format == "STRUCTURED" {
						encode, decode = simplified.MarshalStructured, simplified.UnmarshalStructured
					}
					body, err := encode(comp, wt)
					if err != nil {
						if slices.Contains(reusedSiblingTemplates, o.id) && errors.Is(err, rmpath.ErrPathAmbiguous) {
							continue
						}
						t.Fatalf("Marshal %s: %v", format, err)
					}
					back, err := decode(body, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
					if err != nil {
						t.Fatalf("Unmarshal %s: %v", format, err)
					}
					names := namesAt(canonicalDoc(t, back))
					for _, at := range slices.Sorted(maps.Keys(written)) {
						got, ok := names[at]
						switch {
						case !ok:
							t.Errorf("%s: the decoded composition has no named node at %s, where the skeleton writes %s", format, at, jsonText(written[at]))
						case !reflect.DeepEqual(got, written[at]):
							t.Errorf("%s: decoded name at %s = %s, want the written %s", format, at, jsonText(got), jsonText(written[at]))
						}
					}
				}
			})
		}
	}
	if compared == 0 {
		t.Error("no skeleton writes a DV_CODED_TEXT name; the guard compares nothing")
	}
}

// codedNames returns the DV_CODED_TEXT names of the canonical JSON object doc,
// by the position of their node ([namesAt]).
func codedNames(doc map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any)
	for at, name := range namesAt(doc) {
		if name["_type"] == "DV_CODED_TEXT" {
			out[at] = name
		}
	}
	return out
}

// namesAt returns the name of each named object in the canonical JSON object
// doc, by its position: the attributes from the root, each list member with
// its archetype_node_id and its rank among the members of that list with the
// same one, so that two compositions whose lists hold their members in
// another order still give a node one position.
func namesAt(doc map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any)
	var walk func(at string, obj map[string]any)
	walk = func(at string, obj map[string]any) {
		if name, ok := obj["name"].(map[string]any); ok {
			out[at] = name
		}
		for attr, v := range obj {
			if attr == "name" {
				continue
			}
			switch v := v.(type) {
			case map[string]any:
				walk(at+"/"+attr, v)
			case []any:
				rank := make(map[string]int)
				for i, e := range v {
					m, ok := e.(map[string]any)
					if !ok {
						continue
					}
					id, _ := m["archetype_node_id"].(string)
					seg := attr + "#" + strconv.Itoa(i)
					if id != "" {
						seg = attr + "[" + id + "]#" + strconv.Itoa(rank[id])
						rank[id]++
					}
					walk(at+"/"+seg, m)
				}
			}
		}
	}
	walk("", doc)
	return out
}

package simplified_test

// REQ-053 — FLAT encode: *rm.Composition -> FLAT map driven by the Web Template.
import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

const minimalObsOPT = "../../../testkit/corpus/templates/minimal_observation.en.v1.opt"

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
	return slices.Sorted(maps.Keys(m))
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
	// as a bare (suffix-less) FLAT entry. The event contributes no segment of
	// its own: this fixture's EVENT can occur at most once, and the reference
	// lifts such a degenerate wrapper rather than emitting a node for it
	// (REQ-116 Phase 4, webtemplate.isCollapsedEvent) — a repeating event
	// keeps its segment, as `any_event` does in the upstream FLAT corpus.
	const key = "minimal/minimal:0/text"
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %q; keys=%v", key, sortedKeys(m))
	}
	if s, _ := v.(string); s == "" {
		t.Errorf("key %q = %v, want a non-empty string (DV_TEXT bare value)", key, v)
	}
}

// deepCopyComposition round-trips a composition through canonical JSON so the
// copy has the exact runtime shapes the codecs expect (pointer content items,
// polymorphic fields re-resolved).
func deepCopyComposition(t *testing.T, comp *rm.Composition) *rm.Composition {
	t.Helper()
	b, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var out rm.Composition
	if err := canjson.Unmarshal(b, &out); err != nil {
		t.Fatalf("canjson.Unmarshal: %v", err)
	}
	return &out
}

// TestEncodeSkipsEmptyRepeatInstances: a repeat instance whose subtree carries
// no representable leaf data must not consume a FLAT :index. Stamping the
// index by RM list position would emit a sparse sequence (":0" and ":2", no
// ":1") that the codec's own decoder rejects as phantom gap-fill — breaking
// MarshalFlat -> UnmarshalFlat on a valid composition. The empty instance is
// instead omitted and later indexes close ranks (documented in deviations.md).
func TestEncodeSkipsEmptyRepeatInstances(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	if len(comp.Content) != 1 {
		t.Fatalf("fixture: want 1 content item, got %d", len(comp.Content))
	}
	// "Carries no representable leaf data" has to include every attribute that
	// emits without a Web Template leaf of its own, or the instance would
	// legitimately occupy an index and the test would measure coverage rather
	// than compaction:
	//
	//   - the ENTRY in-context CODE_PHRASE pair — language and encoding are
	//     RM-mandatory and emit as FLAT leaves;
	//   - the LOCATABLE identity attributes the REQ-140 underscore grammar
	//     carries — instance.Generate stamps a `uid` on every archetyped node,
	//     which now emits as `<path>/_uid`.
	middle := deepCopyComposition(t, comp)
	switch o := middle.Content[0].(type) {
	case *rm.Observation:
		o.Data.Events = nil
		o.Language, o.Encoding = rm.CodePhrase{}, rm.CodePhrase{}
		o.UID, o.Links = nil, nil
	case rm.Observation:
		o.Data.Events = nil
		o.Language, o.Encoding = rm.CodePhrase{}, rm.CodePhrase{}
		o.UID, o.Links = nil, nil
		middle.Content[0] = o
	default:
		t.Fatalf("fixture: content[0] is %T, want an Observation", middle.Content[0])
	}
	last := deepCopyComposition(t, comp)
	comp.Content = append(comp.Content, middle.Content[0], last.Content[0])

	flat, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(flat, &m); err != nil {
		t.Fatal(err)
	}
	// The two non-empty instances occupy :0 and :1; nothing may reference :2.
	var saw0, saw1, saw2 bool
	for k := range m {
		saw0 = saw0 || strings.Contains(k, "/minimal:0/")
		saw1 = saw1 || strings.Contains(k, "/minimal:1/")
		saw2 = saw2 || strings.Contains(k, "/minimal:2/")
	}
	if !saw0 || !saw1 || saw2 {
		t.Errorf("index compaction: saw :0=%v :1=%v :2=%v, want true/true/false; keys=%v",
			saw0, saw1, saw2, sortedKeys(m))
	}
	// The encoder's output must decode — and re-encode to the same FLAT map.
	back, err := simplified.UnmarshalFlat(flat, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat of encoder output: %v", err)
	}
	flat2, err := simplified.MarshalFlat(back, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}
	var m2 map[string]any
	if err := json.Unmarshal(flat2, &m2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, m2) {
		t.Errorf("round-trip not idempotent:\n first=%v\nsecond=%v", sortedKeys(m), sortedKeys(m2))
	}
}

// TestEncodePartyRelatedComposerAtRealPath — REQ-053, ADR 0029 (replacing
// the pin that refused a PARTY_RELATED composer). No ctx/ short form carries a
// PARTY_RELATED, so encode writes it whole at the composer's real path, its
// name and its `/relationship`, with no ctx/ composer key, and decode reads it
// back as the same PARTY_RELATED rather than defaulting the composer.
func TestEncodePartyRelatedComposerAtRealPath(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	name := "Related Composer"
	comp.Composer = &rm.PartyRelated{
		Name: &name,
		Relationship: rm.DVCodedText{
			DVText: rm.DVText{Value: "mother"},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "openehr"},
				CodeString:    "10",
			},
		},
	}
	m := flatKeys(t, comp, wt)
	root := wt.Tree.ID
	for key, want := range map[string]any{
		root + "/composer|name":                     name,
		root + "/composer/relationship|code":        "10",
		root + "/composer/relationship|value":       "mother",
		root + "/composer/relationship|terminology": "openehr",
	} {
		if m[key] != want {
			t.Errorf("FLAT %q = %#v, want %#v", key, m[key], want)
		}
	}
	for key := range m {
		if strings.HasPrefix(key, "ctx/composer") {
			t.Errorf("FLAT carries %q beside the real-path composer", key)
		}
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := simplified.UnmarshalFlat(data, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	related, ok := got.Composer.(*rm.PartyRelated)
	if !ok || related.Name == nil || *related.Name != name || related.Relationship.DefiningCode.CodeString != "10" {
		t.Errorf("decoded composer = %#v, want the PARTY_RELATED back", got.Composer)
	}
}

// TestEncodeComposerShapesRefused — REQ-053, ADR 0029. Encode refuses a
// composer PARTY_IDENTIFIED with an empty name (RM invariant `Name_valid`) or
// with no name, external_ref or identifiers (`Basic_validity`), a
// PARTY_RELATED carrying a relationship and nothing else, and a PARTY_RELATED
// carrying no relationship. Unrefused, the empty PARTY_IDENTIFIED would be
// written as nothing and read back as PARTY_SELF, a silent type substitution;
// the PARTY_RELATED with only a relationship would write its `/relationship`
// keys, which decode then refuses under `Basic_validity` as a relationship
// with nothing it qualifies; the PARTY_RELATED with no relationship would
// write its PARTY_IDENTIFIED keys alone and read back as a PARTY_IDENTIFIED;
// and decode reads an absent name where the empty one was. The refusal names
// the composer and carries no value. (A PARTY_SELF carrying an external_ref
// left this test: it now encodes at the real path,
// TestREQ053ComposerEncodeSpellings.)
func TestEncodeComposerShapesRefused(t *testing.T) {
	empty := ""
	secret := "secret-composer-0451"
	ref := &rm.PartyRef{ID: &rm.HierObjectID{Value: "ref-1"}, Namespace: "demographic", Type: "PERSON"}
	for name, composer := range map[string]rm.PartyProxy{
		"PARTY_IDENTIFIED with no name":                    &rm.PartyIdentified{},
		"PARTY_IDENTIFIED with empty name":                 &rm.PartyIdentified{Name: &empty},
		"PARTY_IDENTIFIED with empty name and a reference": &rm.PartyIdentified{Name: &empty, ExternalRef: ref},
		"PARTY_RELATED with only a relationship": &rm.PartyRelated{Relationship: rm.DVCodedText{
			DVText:       rm.DVText{Value: "mother"},
			DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "10"},
		}},
		"PARTY_RELATED with a name and no relationship":      &rm.PartyRelated{Name: &secret},
		"PARTY_RELATED with a reference and no relationship": &rm.PartyRelated{ExternalRef: ref},
	} {
		t.Run(name, func(t *testing.T) {
			comp, wt := genComposition(t, minimalObsOPT)
			comp.Composer = composer
			_, err := simplified.MarshalFlat(comp, wt)
			if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
				t.Fatalf("MarshalFlat err = %v, want ErrUnsupportedDatatype", err)
			}
			if !strings.Contains(err.Error(), wt.Tree.ID+"/composer") {
				t.Errorf("err = %v, want it to name %s/composer", err, wt.Tree.ID)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("err = %v carries the composer's name", err)
			}
		})
	}
}

// flatKeys encodes comp and returns the FLAT map.
func flatKeys(t *testing.T, comp *rm.Composition, wt *webtemplate.WebTemplate) map[string]any {
	t.Helper()
	data, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal flat: %v", err)
	}
	return m
}

// leafNode finds the Web Template node with the given id.
func leafNode(t *testing.T, wt *webtemplate.WebTemplate, id string) *webtemplate.Node {
	t.Helper()
	var found *webtemplate.Node
	var walk func(n *webtemplate.Node)
	walk = func(n *webtemplate.Node) {
		if n.ID == id && found == nil {
			found = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(wt.Tree)
	if found == nil {
		t.Fatalf("no Web Template node with id %q", id)
	}
	return found
}

// TestMarshalFlatZeroCodedTextWritesNoKeys — REQ-053. A zero DV_CODED_TEXT
// at a leaf, a coded text the composition never set, writes no FLAT keys for
// it: no blank |code or |value, and no |raw either, because the zero value
// carries nothing the suffix form cannot hold. The leaf is absent, as a nil
// one would be, and every other key of the composition is unchanged. It is
// checked at a DV_CODED_TEXT leaf and at a DV_TEXT leaf, where DV_CODED_TEXT
// is the spec-sanctioned substitution.
func TestMarshalFlatZeroCodedTextWritesNoKeys(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opt      string
		leafID   string
		wantType string
	}{
		{"DV_CODED_TEXT leaf", "../../../testkit/corpus/templates/Test_dv_coded_text_open_constraint.v0.opt", "my_dv_coded_text", "DV_CODED_TEXT"},
		{"DV_TEXT leaf", "../../../testkit/corpus/templates/minimal_observation.en.v1.opt", "text", "DV_TEXT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp, wt := genComposition(t, tc.opt)
			leaf := leafNode(t, wt, tc.leafID)
			if leaf.RMType != tc.wantType {
				t.Fatalf("leaf %q is a %s, want %s", tc.leafID, leaf.RMType, tc.wantType)
			}
			before := flatKeys(t, comp, wt)
			var leafKeys []string
			for k := range before {
				if base, _, _ := strings.Cut(k, "|"); strings.HasSuffix(base, "/"+tc.leafID) {
					leafKeys = append(leafKeys, k)
				}
			}
			if len(leafKeys) == 0 {
				t.Fatalf("the generated composition writes no key for %q: %v", tc.leafID, sortedKeys(before))
			}

			// Zero the leaf's value in place with a DV_CODED_TEXT nothing set.
			holder, err := rmpath.ItemsAtPath(comp, strings.TrimSuffix(leaf.AQLPath, "/value"))
			if err != nil || len(holder) != 1 {
				t.Fatalf("resolve the leaf's ELEMENT: %d items, %v", len(holder), err)
			}
			el, ok := holder[0].(*rm.Element)
			if !ok {
				t.Fatalf("the leaf's holder is a %T, want *rm.Element", holder[0])
			}
			el.Value = &rm.DVCodedText{}

			after := flatKeys(t, comp, wt)
			for k := range after {
				if base, _, _ := strings.Cut(k, "|"); strings.HasSuffix(base, "/"+tc.leafID) {
					t.Errorf("a zero DV_CODED_TEXT wrote the key %q = %v, want no key for the leaf", k, after[k])
				}
			}
			for k, v := range before {
				if slices.Contains(leafKeys, k) {
					continue
				}
				if !reflect.DeepEqual(after[k], v) {
					t.Errorf("key %q changed from %v to %v when only the leaf was zeroed", k, v, after[k])
				}
			}
		})
	}
}

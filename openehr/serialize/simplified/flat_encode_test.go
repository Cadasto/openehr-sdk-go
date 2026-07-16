package simplified_test

// REQ-053 — FLAT encode: *rm.Composition -> FLAT map driven by the Web Template.
import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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
	middle := deepCopyComposition(t, comp)
	switch o := middle.Content[0].(type) {
	case *rm.Observation:
		o.Data.Events = nil
	case rm.Observation:
		o.Data.Events = nil
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

// TestEncodePartyRelatedComposerErrors: ctx/ short forms carry only
// composer_self / composer_name; a PARTY_RELATED composer (or any other
// unrepresentable PARTY_PROXY) has no encoding. Silently omitting it would let
// a WithTemplate decode default the composer to PARTY_SELF — a silent type
// substitution — so encode must fail loudly instead (see deviations.md).
func TestEncodePartyRelatedComposerErrors(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	name := "Related Composer"
	comp.Composer = &rm.PartyRelated{
		PartyIdentified: rm.PartyIdentified{Name: &name},
		Relationship: rm.DVCodedText{
			DVText: rm.DVText{Value: "mother"},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "openehr"},
				CodeString:    "10",
			},
		},
	}
	if _, err := simplified.MarshalFlat(comp, wt); !errors.Is(err, simplified.ErrUnsupportedDatatype) {
		t.Errorf("MarshalFlat(PARTY_RELATED composer) err = %v, want ErrUnsupportedDatatype", err)
	}
}

package rmaccess_test

// Transitional parity with the hand-written readers (REQ-171 Structural
// access, ADR 0028). The validators move from openehr/validation/rmread to
// this package in one swap; this test pins that, on every vendored corpus
// document and on the zero value of every registered class, the generated
// access answers what rmread answers wherever rmread reads, apart from the
// three named divergences below. It goes with rmread.
//
// The test package imports rmread, canjson and testkit/fixtures, none of which
// the package itself may import.

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The divergences the swap is allowed, each a reading the generated access
// makes on purpose. Every other difference in presence or in the Go type of
// a value is a defect.
const (
	// allowUnread: rmread has no row for the attribute, so it answers the
	// untyped (nil, false) it gives an unknown pair, while the access finds
	// the attribute present. This is the gap ADR 0028 closes. It never covers
	// a DV_INTERVAL bound or a REFERENCE_RANGE range: rmread reads both and
	// can give that same answer for a set field, so a difference there is a
	// defect.
	allowUnread = "an attribute rmread does not read"
	// allowEmptyString: EVENT_CONTEXT location and CODE_PHRASE preferred_term
	// read a set but empty string as absent in rmread; a pointer that is not
	// nil is present.
	allowEmptyString = "a pointer to an empty string"
	// allowNamedHistory: rmread reads an OBSERVATION's HISTORY, held by
	// value, as absent when it has no events, no origin and no node id,
	// whatever else it holds; a value held by value is absent only when it is
	// its zero value.
	allowNamedHistory = "a HISTORY held by value with no events, origin or node id"
)

// emptyStringAttrs are the two attributes allowEmptyString covers.
var emptyStringAttrs = map[string]string{
	"EVENT_CONTEXT": "location",
	"CODE_PHRASE":   "preferred_term",
}

// parityWalk walks one value through the generated access, comparing every
// attribute with rmread on the way down.
type parityWalk struct {
	t        *testing.T
	doc      string
	compared int
	allowed  map[string]int
}

func (w *parityWalk) node(v any, path string, depth int) {
	if depth > 128 {
		w.t.Fatalf("%s: %s is deeper than any corpus document; the walk loops", w.doc, path)
	}
	class, ok := rm.RMTypeName(v)
	if !ok {
		return
	}
	for _, attr := range rminfo.Default.(rminfo.AttributeLister).AttributeNames(class) {
		if rminfo.IsNonStorableAttr(class, attr) {
			continue
		}
		at := path + "/" + attr
		if container, _ := rminfo.Default.IsContainer(class, attr); container {
			items, ok := rmaccess.Multiple(v, attr)
			w.multiple(class, attr, at, items, ok, v)
			for i, item := range items {
				w.node(item, fmt.Sprintf("%s[%d]", at, i), depth+1)
			}
			continue
		}
		value, present := rmaccess.Single(v, attr)
		w.single(class, attr, at, value, present, v)
		if present {
			w.node(value, at, depth+1)
		}
	}
}

// single compares one single-valued attribute.
func (w *parityWalk) single(class, attr, path string, value any, present bool, parent any) {
	old, oldOK := rmread.ReadSingle(parent, class, attr)
	if !present && !oldOK {
		return
	}
	w.compared++
	if present && oldOK {
		if got, want := fmt.Sprintf("%T", value), fmt.Sprintf("%T", old); got != want {
			w.t.Errorf("%s %s: Single(%s, %q) value is a %s, rmread.ReadSingle gives a %s", w.doc, path, class, attr, got, want)
		}
		return
	}
	if allowance := allowedSingle(class, attr, present, old, oldOK); allowance != "" {
		w.allowed[allowance]++
		return
	}
	w.t.Errorf("%s %s: Single(%s, %q) present = %t, rmread.ReadSingle gives %t (value %T)", w.doc, path, class, attr, present, oldOK, old)
}

// allowedSingle names the divergence a single-valued difference falls in,
// or "" when it is none of them.
func allowedSingle(class, attr string, present bool, old any, oldOK bool) string {
	if !present || oldOK {
		return "" // the access never reads absent what rmread reads present
	}
	if old == nil && !readsNil(class, attr) {
		return allowUnread
	}
	if emptyStringAttrs[class] == attr {
		if s, ok := old.(*string); ok && s != nil && *s == "" {
			return allowEmptyString
		}
	}
	if class == "OBSERVATION" && attr == "data" {
		if h, ok := old.(rm.History[rm.ItemStructure]); ok && len(h.Events) == 0 && h.Origin.Value == "" && h.ArchetypeNodeID == "" {
			return allowNamedHistory
		}
	}
	return ""
}

// readsNil reports whether rmread reads attr of class and can answer the
// untyped (nil, false) for a set field: a DV_INTERVAL bound on an open side,
// and a REFERENCE_RANGE range.
func readsNil(class, attr string) bool {
	switch class {
	case "DV_INTERVAL":
		return attr == "lower" || attr == "upper"
	case "REFERENCE_RANGE":
		return attr == "range"
	}
	return false
}

// multiple compares one container.
func (w *parityWalk) multiple(class, attr, path string, items []any, ok bool, parent any) {
	old, oldOK := rmread.ReadMultiple(parent, class, attr)
	w.compared++
	if !ok {
		w.t.Errorf("%s %s: Multiple(%s, %q) ok = false for a container rminfo declares", w.doc, path, class, attr)
		return
	}
	if !oldOK {
		if old != nil {
			w.t.Errorf("%s %s: rmread.ReadMultiple(%s, %q) = (%d items, false)", w.doc, path, class, attr, len(old))
			return
		}
		w.allowed[allowUnread]++
		return
	}
	if len(items) != len(old) {
		w.t.Errorf("%s %s: Multiple(%s, %q) has %d items, rmread.ReadMultiple %d", w.doc, path, class, attr, len(items), len(old))
		return
	}
	for i := range items {
		if got, want := fmt.Sprintf("%T", items[i]), fmt.Sprintf("%T", old[i]); got != want {
			w.t.Errorf("%s %s[%d]: Multiple(%s, %q) item is a %s, rmread.ReadMultiple gives a %s", w.doc, path, i, class, attr, got, want)
		}
	}
}

// paritySamples are hand-built values for the attributes rmread reads that
// the corpus does not reach: the five whose value comes back in another shape
// than the field's own, and the two narrow divergences.
func paritySamples() map[string]any {
	coded := func(code string) rm.DVCodedText {
		return rm.DVCodedText{
			Value:        "text " + code,
			DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: code},
		}
	}
	step, transition := coded("245"), coded("540")
	empty := ""
	precision := rm.Integer(2)
	return map[string]any{
		"ISM_TRANSITION careflow_step and transition": &rm.Action{IsmTransition: rm.IsmTransition{
			CurrentState: coded("524"), CareflowStep: &step, Transition: &transition,
		}},
		"REFERENCE_RANGE range": &rm.DVCount{Magnitude: 1, OtherReferenceRanges: []rm.ReferenceRange[rm.DVCount]{{
			Meaning: &rm.DVText{Value: "normal"},
			Range: rm.DVInterval[rm.DVOrdered]{
				Lower: &rm.DVCount{Magnitude: 1}, LowerIncluded: true, UpperUnbounded: true,
			},
		}}},
		"DV_PROPORTION precision": &rm.DVProportion{Numerator: 1, Denominator: 2, Precision: &precision},
		"ITEM_SINGLE item":        &rm.ItemSingle{ArchetypeNodeID: "at1", Item: rm.Element{ArchetypeNodeID: "at2"}},
		"EVENT_CONTEXT location":  &rm.EventContext{Location: &empty, Setting: coded("238")},
		"CODE_PHRASE preferred_term": &rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at1", PreferredTerm: &empty,
		},
		"OBSERVATION data with a name only": &rm.Observation{
			Data: rm.History[rm.ItemStructure]{Name: &rm.DVText{Value: "history"}},
		},
	}
}

// TestAccess_REQ171_ParityWithRMRead walks every vendored corpus document,
// the zero value of every registered class and the hand-built samples
// through the generated access and through rmread, and fails on any
// difference in presence or in the Go type of a value outside the named
// divergences.
func TestAccess_REQ171_ParityWithRMRead(t *testing.T) {
	allowed := map[string]int{}
	compared := 0

	rels, err := fixtures.ListCompositionJSON()
	if err != nil {
		t.Fatalf("fixtures.ListCompositionJSON: %v", err)
	}
	decoded := 0
	for _, rel := range rels {
		factory, ok := fixtures.FactoryForJSONRel(rel)
		if !ok {
			continue
		}
		body, err := os.ReadFile(fixtures.ResolveCompositionJSON(rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel.Rel, err)
		}
		root := factory()
		if err := canjson.Unmarshal(body, root); err != nil {
			continue // a decode failure is the codec's tests' concern
		}
		decoded++
		w := &parityWalk{t: t, doc: rel.Rel, allowed: allowed}
		w.node(root, "", 0)
		compared += w.compared
	}
	if decoded < 40 {
		t.Fatalf("decoded %d corpus documents, want at least 40; the corpus listing has gone blind", decoded)
	}

	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		w := &parityWalk{t: t, doc: "zero " + name, allowed: allowed}
		w.node(ctor(), "", 0)
		compared += w.compared
	}

	samples := paritySamples()
	for _, name := range slices.Sorted(maps.Keys(samples)) {
		w := &parityWalk{t: t, doc: "sample " + name, allowed: allowed}
		w.node(samples[name], "", 0)
		compared += w.compared
	}

	if compared == 0 {
		t.Fatal("compared no attribute; the walk has gone blind")
	}
	for _, allowance := range []string{allowUnread, allowEmptyString, allowNamedHistory} {
		if allowed[allowance] == 0 {
			t.Errorf("no attribute was allowed as %s; the walk no longer reaches it", allowance)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(allowed)) {
		t.Logf("%d attributes allowed as %s", allowed[k], k)
	}
	t.Logf("%d attributes compared", compared)
}

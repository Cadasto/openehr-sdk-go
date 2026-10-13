package rmaccess_test

// Transitional parity with the hand-written readers (REQ-171 Structural
// access, ADR 0028). The validators move from openehr/validation/rmread to
// this package in one swap; this test pins that, on every vendored corpus
// document, on the zero value of every registered class and on hand-built
// samples, the generated access answers what rmread answers wherever rmread
// reads, apart from the four named divergences below. It goes with rmread.
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
// makes on purpose, as Structural access states it. Every other difference
// in presence or in the Go type of a value is a defect.
const (
	// allowUnread: rmread has no row for the attribute, so it answers the
	// untyped (nil, false) it gives an unknown pair, while the access finds
	// the attribute present. This is the gap ADR 0028 closes. It never covers
	// a DV_INTERVAL bound or a REFERENCE_RANGE range: rmread reads both and
	// can give that same answer for a set field, so a difference there is a
	// defect unless allowSpecReading names the pair.
	allowUnread = "an attribute rmread does not read"
	// allowEmptyString: EVENT_CONTEXT location and CODE_PHRASE preferred_term
	// read a set but empty string as absent in rmread; a pointer that is not
	// nil is present.
	allowEmptyString = "a pointer to an empty string"
	// allowSpecReading: for the pairs [specReadings] lists, and only those,
	// rmread decides presence by one or two members of the value (a
	// DV_DATE_TIME by its value, an ISM_TRANSITION by its current_state, an
	// ELEMENT by its node id and value, a DV_CODED_TEXT by its value and code
	// string, an interval by its bounds read as Void) or a pointer by what it
	// points to, where the spec reads the whole Go value: a value held by
	// value is absent only when it is empty, which for these classes means
	// its zero value, and a pointer is present when it is not nil. On such a
	// value the access reads the attribute present and rmread absent.
	allowSpecReading = "the spec's reading differs from rmread's single-member check"
	// allowNilList: a nil list is absent in the access, while rmread reads
	// every container it serves as there, with no item. It covers only a
	// container the access reads absent with no item where rmread finds no
	// item either; that the access reads absent only a nil slice, never an
	// empty one, is pinned by TestPresence_REQ171_ListReadings and by the
	// generator's own test of every container's reading, and here by the
	// empty-list sample, which must not take this allowance.
	allowNilList = "a nil list is absent"
)

// emptyStringAttrs are the two attributes allowEmptyString covers.
var emptyStringAttrs = map[string]string{
	"EVENT_CONTEXT": "location",
	"CODE_PHRASE":   "preferred_term",
}

// pairDivergence is one allowance taken on one attribute.
type pairDivergence struct {
	allowance   string
	class, attr string
}

// specReading is one attribute allowSpecReading covers, with a sample: a
// parent, built by hand, on which the access reads the attribute present
// and rmread absent. The corpus does not reach any of these values.
type specReading struct {
	class, attr string
	sample      any
}

// specReadings lists every attribute allowSpecReading covers: the by-value
// attributes rmread reads by a member, and ISM_TRANSITION careflow_step and
// transition, which it reads by what the pointer holds. Each sample must
// diverge, so a pair whose divergence disappears fails the test.
func specReadings() []specReading {
	status := "unknown"
	coded := rm.DVCodedText{
		Value:        "planned",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "526"},
	}
	return []specReading{
		// A DV_DATE_TIME or DV_DURATION with no value but another member set.
		{class: "EVENT_CONTEXT", attr: "start_time", sample: &rm.EventContext{StartTime: rm.DVDateTime{MagnitudeStatus: &status}}},
		{class: "ACTION", attr: "time", sample: &rm.Action{Time: rm.DVDateTime{Accuracy: &rm.DVDuration{}}, IsmTransition: rm.IsmTransition{CurrentState: coded}}},
		{class: "HISTORY", attr: "origin", sample: &rm.History[rm.ItemStructure]{Origin: rm.DVDateTime{MagnitudeStatus: &status}}},
		{class: "POINT_EVENT", attr: "time", sample: &rm.PointEvent[rm.ItemStructure]{Time: rm.DVDateTime{MagnitudeStatus: &status}}},
		{class: "INTERVAL_EVENT", attr: "time", sample: &rm.IntervalEvent[rm.ItemStructure]{Time: rm.DVDateTime{MagnitudeStatus: &status}}},
		{class: "INTERVAL_EVENT", attr: "width", sample: &rm.IntervalEvent[rm.ItemStructure]{Time: rm.DVDateTime{Value: "2026-10-13T10:00:00Z"}, Width: rm.DVDuration{MagnitudeStatus: &status}}},
		// A DV_CODED_TEXT whose defining code has a terminology only.
		{class: "EVENT_CONTEXT", attr: "setting", sample: &rm.EventContext{
			StartTime: rm.DVDateTime{Value: "2026-10-13T10:00:00Z"},
			Setting:   rm.DVCodedText{DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}}},
		}},
		// An ISM_TRANSITION with no current_state but a careflow_step.
		{class: "ACTION", attr: "ism_transition", sample: &rm.Action{Time: rm.DVDateTime{Value: "2026-10-13T10:00:00Z"}, IsmTransition: rm.IsmTransition{CareflowStep: &coded}}},
		// A pointer to an empty DV_CODED_TEXT.
		{class: "ISM_TRANSITION", attr: "careflow_step", sample: &rm.IsmTransition{CurrentState: coded, CareflowStep: &rm.DVCodedText{}}},
		{class: "ISM_TRANSITION", attr: "transition", sample: &rm.IsmTransition{CurrentState: coded, Transition: &rm.DVCodedText{}}},
		// An ELEMENT with a name but no node id and no value.
		{class: "ITEM_SINGLE", attr: "item", sample: &rm.ItemSingle{Item: rm.Element{Name: &rm.DVText{Value: "name"}}}},
		// A range whose lower bound is a nil pointer, every flag false.
		{class: "REFERENCE_RANGE", attr: "range", sample: &rm.ReferenceRange[rm.DVOrdered]{
			Meaning: &rm.DVText{Value: "normal"},
			Range:   rm.DVInterval[rm.DVOrdered]{Interval: rm.Interval[rm.DVOrdered]{Lower: (*rm.DVCount)(nil)}},
		}},
		// A HISTORY with a name but no events, origin or node id.
		{class: "OBSERVATION", attr: "data", sample: &rm.Observation{Data: rm.History[rm.ItemStructure]{Name: &rm.DVText{Value: "history"}}}},
	}
}

// specReadingPairs is the set of class and attribute pairs specReadings
// lists.
var specReadingPairs = func() map[[2]string]bool {
	out := map[[2]string]bool{}
	for _, r := range specReadings() {
		out[[2]string{r.class, r.attr}] = true
	}
	return out
}()

// parityWalk walks one value through the generated access, comparing every
// attribute with rmread on the way down.
type parityWalk struct {
	t        *testing.T
	doc      string
	compared int
	allowed  map[string]int
	pairs    map[pairDivergence]int
}

func newParityWalk(t *testing.T, doc string, allowed map[string]int) *parityWalk {
	return &parityWalk{t: t, doc: doc, allowed: allowed, pairs: map[pairDivergence]int{}}
}

// allow records one divergence of attr of class under allowance.
func (w *parityWalk) allow(allowance, class, attr string) {
	w.allowed[allowance]++
	w.pairs[pairDivergence{allowance: allowance, class: class, attr: attr}]++
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
			items, present := rmaccess.Multiple(v, attr)
			w.multiple(class, attr, at, items, present, v)
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
		w.allow(allowance, class, attr)
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
	if specReadingPairs[[2]string{class, attr}] {
		return allowSpecReading
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
func (w *parityWalk) multiple(class, attr, path string, items []any, present bool, parent any) {
	old, oldOK := rmread.ReadMultiple(parent, class, attr)
	if !oldOK && old != nil {
		w.t.Errorf("%s %s: rmread.ReadMultiple(%s, %q) = (%d items, false)", w.doc, path, class, attr, len(old))
		return
	}
	if !present && !oldOK {
		return // an absent list rmread does not read, or a typed-nil parent
	}
	w.compared++
	switch {
	case !oldOK:
		w.allow(allowUnread, class, attr)
		return
	case !present:
		if items == nil && len(old) == 0 {
			w.allow(allowNilList, class, attr)
			return
		}
		w.t.Errorf("%s %s: Multiple(%s, %q) = (%d items, false), rmread.ReadMultiple has %d items", w.doc, path, class, attr, len(items), len(old))
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

// paritySample is a hand-built value for an attribute rmread reads that the
// corpus does not reach, with the divergences it must and must not show.
type paritySample struct {
	value    any
	diverges []pairDivergence // each must be taken at least once
	agrees   []pairDivergence // none may be taken
}

// paritySamples are the hand-built values: the six attributes whose value
// comes back in another shape than the field's own, which must agree with
// rmread, the empty-string divergence, and the two list readings.
func paritySamples() map[string]paritySample {
	coded := func(code string) rm.DVCodedText {
		return rm.DVCodedText{
			Value:        "text " + code,
			DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: code},
		}
	}
	step, transition := coded("245"), coded("540")
	empty := ""
	precision := rm.Integer(2)
	section := &rm.Section{ArchetypeNodeID: "openEHR-EHR-SECTION.adhoc.v1", Name: &rm.DVText{Value: "section"}}
	return map[string]paritySample{
		"ISM_TRANSITION careflow_step and transition": {value: &rm.Action{IsmTransition: rm.IsmTransition{
			CurrentState: coded("524"), CareflowStep: &step, Transition: &transition,
		}}},
		"REFERENCE_RANGE range": {value: &rm.DVCount{Magnitude: 1, OtherReferenceRanges: []rm.ReferenceRange[rm.DVCount]{{
			Meaning: &rm.DVText{Value: "normal"},
			Range: rm.DVInterval[rm.DVOrdered]{
				Lower: &rm.DVCount{Magnitude: 1}, LowerIncluded: true, UpperUnbounded: true,
			},
		}}}},
		"DV_PROPORTION precision":    {value: &rm.DVProportion{Numerator: 1, Denominator: 2, Precision: &precision}},
		"ITEM_SINGLE item":           {value: &rm.ItemSingle{ArchetypeNodeID: "at1", Item: rm.Element{ArchetypeNodeID: "at2"}}},
		"CODE_PHRASE terminology_id": {value: &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at1"}},
		"EVENT_CONTEXT location": {
			value:    &rm.EventContext{Location: &empty, Setting: coded("238")},
			diverges: []pairDivergence{{allowance: allowEmptyString, class: "EVENT_CONTEXT", attr: "location"}},
		},
		"CODE_PHRASE preferred_term": {
			value:    &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at1", PreferredTerm: &empty},
			diverges: []pairDivergence{{allowance: allowEmptyString, class: "CODE_PHRASE", attr: "preferred_term"}},
		},
		"COMPOSITION content, a nil list": {
			value:    &rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"},
			diverges: []pairDivergence{{allowance: allowNilList, class: "COMPOSITION", attr: "content"}},
		},
		"COMPOSITION content, an empty list": {
			value:  &rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1", Content: []rm.ContentItem{}},
			agrees: []pairDivergence{{allowance: allowNilList, class: "COMPOSITION", attr: "content"}},
		},
		"COMPOSITION content, a list with an item": {
			value:  &rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1", Content: []rm.ContentItem{section}},
			agrees: []pairDivergence{{allowance: allowNilList, class: "COMPOSITION", attr: "content"}},
		},
	}
}

// TestAccess_REQ171_ParityWithRMRead walks every vendored corpus document,
// the zero value of every registered class and the hand-built samples
// through the generated access and through rmread, and fails on any
// difference in presence or in the Go type of a value outside the named
// divergences, and on a sample that no longer shows the divergence it is
// there for.
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
		w := newParityWalk(t, rel.Rel, allowed)
		w.node(root, "", 0)
		compared += w.compared
	}
	if decoded < 40 {
		t.Fatalf("decoded %d corpus documents, want at least 40; the corpus listing has gone blind", decoded)
	}

	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		w := newParityWalk(t, "zero "+name, allowed)
		w.node(ctor(), "", 0)
		compared += w.compared
	}

	samples := paritySamples()
	for _, name := range slices.Sorted(maps.Keys(samples)) {
		sample := samples[name]
		w := newParityWalk(t, "sample "+name, allowed)
		w.node(sample.value, "", 0)
		compared += w.compared
		for _, d := range sample.diverges {
			if w.pairs[d] == 0 {
				t.Errorf("sample %s: %s.%s no longer diverges from rmread as %s", name, d.class, d.attr, d.allowance)
			}
		}
		for _, d := range sample.agrees {
			if w.pairs[d] != 0 {
				t.Errorf("sample %s: %s.%s took the allowance %s, want agreement with rmread", name, d.class, d.attr, d.allowance)
			}
		}
	}

	for _, r := range specReadings() {
		w := newParityWalk(t, "spec reading "+r.class+"."+r.attr, allowed)
		w.node(r.sample, "", 0)
		compared += w.compared
		if w.pairs[pairDivergence{allowance: allowSpecReading, class: r.class, attr: r.attr}] == 0 {
			t.Errorf("spec reading %s.%s: the sample no longer diverges from rmread; drop the pair from specReadings", r.class, r.attr)
		}
	}

	if compared == 0 {
		t.Fatal("compared no attribute; the walk has gone blind")
	}
	for _, allowance := range []string{allowUnread, allowEmptyString, allowSpecReading, allowNilList} {
		if allowed[allowance] == 0 {
			t.Errorf("no attribute was allowed as %s; the walk no longer reaches it", allowance)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(allowed)) {
		t.Logf("%d attributes allowed as %s", allowed[k], k)
	}
	t.Logf("%d attributes compared", compared)
}

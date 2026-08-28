package lint

// pathshape_internal_test.go: white-box pins for REQ-164 § The conservative
// segment walk. REQ-164 § Acceptance requires the walk's silent stops to be
// pinned BY NAME rather than left to a general property, and from outside the
// package every stop looks the same — silence. These tests name each one, so
// removing a guard fails a test instead of merely swapping one silent stop for
// another.
//
// The walk's own verdict is what a later REQ-164 check reads
// (aql_fanout_path_grain asks the divergence question of this same walk), so
// the shape of that verdict is pinned here too.
//
// It also carries the pins for [containsChain], aql_contains_redundant_step's
// linearisation of the FROM/CONTAINS tree, for the same reason and one more:
// its flattened-sibling and deep-stop branches answer tree shapes no query
// [parse.Parse] can produce, so only a hand-built tree reaches them (the
// precedent is semantic_internal_test.go's
// TestAndFrontierFlattensHandBuiltNestedAnd).

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

const walkArch = "openEHR-EHR-OBSERVATION.blood_pressure.v1"

// walkOne parses a single-path query and returns the walk's verdict on that
// path.
func walkOne(t *testing.T, q string) pathShape {
	t.Helper()
	doc, err := parse.Parse(q)
	if err != nil {
		t.Fatalf("Parse(%q): %v", q, err)
	}
	if len(doc.Paths) != 1 {
		t.Fatalf("Parse(%q) reported %d identified paths, want 1", q, len(doc.Paths))
	}
	return newSegmentWalker(rminfo.Default, Extract(doc).Aliases).walk(doc.Paths[0])
}

// TestSegmentWalkTypesEverySegmentItCan is the baseline the stop rows are read
// against: nothing stops, every segment is typed, and the container verdict
// lands on the segment the pin calls multi-valued.
func TestSegmentWalkTypesEverySegmentItCan(t *testing.T) {
	t.Parallel()
	sh := walkOne(t, "SELECT o/data/events/time/value FROM OBSERVATION o["+walkArch+"]")
	if sh.Root != "OBSERVATION" {
		t.Errorf("Root = %q, want OBSERVATION", sh.Root)
	}
	if sh.Stop != stopNone || sh.StopAt != 4 {
		t.Errorf("Stop = %v at %d, want none at 4", sh.Stop, sh.StopAt)
	}
	if len(sh.Typed) != 4 {
		t.Fatalf("typed %d segments, want 4: %+v", len(sh.Typed), sh.Typed)
	}
	want := []segmentShape{
		{Index: 0, Name: "data", Parent: "OBSERVATION", RMType: "HISTORY"},
		{Index: 1, Name: "events", Parent: "HISTORY", RMType: "EVENT", Container: true},
		{Index: 2, Name: "time", Parent: "EVENT", RMType: "DV_DATE_TIME"},
		{Index: 3, Name: "value", Parent: "DV_DATE_TIME", RMType: "String"},
	}
	for i, w := range want {
		if sh.Typed[i] != w {
			t.Errorf("Typed[%d] = %+v, want %+v", i, sh.Typed[i], w)
		}
	}
	if off := sh.offending(); len(off) != 1 || off[0].Name != "events" {
		t.Errorf("offending() = %+v, want the one unpredicated container", off)
	}
}

// TestSegmentWalkStopsAtTheGenericParameterByName pins the stop REQ-164
// § Acceptance names explicitly. `EVENT.data` is literally typed `T` on the
// pinned tables, which is not a class of the universe, so the walk cannot
// descend — and it MUST record that it could not, rather than that the pin
// declared no such attribute: the two are different facts, and only this
// assertion tells them apart (from outside the package both are silence).
func TestSegmentWalkStopsAtTheGenericParameterByName(t *testing.T) {
	t.Parallel()
	sh := walkOne(t, "SELECT o/data/events/data/items/value/magnitude "+
		"FROM EHR e CONTAINS OBSERVATION o["+walkArch+"]")
	if sh.Stop != stopTypeOutsideUniverse {
		t.Errorf("Stop = %v, want %v", sh.Stop, stopTypeOutsideUniverse)
	}
	if sh.StopAt != 3 {
		t.Errorf("StopAt = %d, want 3 (the unreached `items`)", sh.StopAt)
	}
	if len(sh.Typed) != 3 {
		t.Fatalf("typed %d segments, want 3 (data, events, data): %+v", len(sh.Typed), sh.Typed)
	}
	last := sh.Typed[2]
	if last.Parent != "EVENT" || last.RMType != "T" {
		t.Errorf("last typed segment = %+v, want EVENT.data typed T", last)
	}
}

// TestSegmentWalkStopsAtAnUndeclaredAttributeByName: `items` is a container on
// SECTION and on ITEM_TREE, and on OBSERVATION it is not an attribute at all.
func TestSegmentWalkStopsAtAnUndeclaredAttributeByName(t *testing.T) {
	t.Parallel()
	sh := walkOne(t, "SELECT o/items/value FROM OBSERVATION o["+walkArch+"]")
	if sh.Stop != stopUndeclaredAttribute || sh.StopAt != 0 {
		t.Errorf("Stop = %v at %d, want %v at 0", sh.Stop, sh.StopAt, stopUndeclaredAttribute)
	}
	if len(sh.Typed) != 0 {
		t.Errorf("typed %+v, want nothing typed above the stop", sh.Typed)
	}
	if sh.Root != "OBSERVATION" {
		t.Errorf("Root = %q, want OBSERVATION — the walk STARTED and then stopped", sh.Root)
	}
}

// TestSegmentWalkStopsAtAnUnknownAliasClassByName covers both spellings that
// leave the walk with no root: a class the pin does not know, and an alias
// bound to nothing at all.
func TestSegmentWalkStopsAtAnUnknownAliasClassByName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, query string }{
		{"class outside the pin", "SELECT x/data/events/time FROM NOT_AN_RM_CLASS x"},
		{"alias bound to nothing", "SELECT zz/data/events/time FROM OBSERVATION o[" + walkArch + "]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sh := walkOne(t, tc.query)
			if sh.Stop != stopUnknownAliasClass {
				t.Errorf("Stop = %v, want %v", sh.Stop, stopUnknownAliasClass)
			}
			if sh.Root != "" || len(sh.Typed) != 0 {
				t.Errorf("verdict = %+v, want an unstarted walk", sh)
			}
		})
	}
}

// TestSegmentWalkStopsAtAParamArchetypeScopeByName: the class is typeable, so
// only this guard keeps the walk off it — the archetype scope is a `$param`
// whose extent the CDR resolves at execution.
func TestSegmentWalkStopsAtAParamArchetypeScopeByName(t *testing.T) {
	t.Parallel()
	sh := walkOne(t, "SELECT o/data/events/time FROM OBSERVATION o[$arch]")
	if sh.Stop != stopParamArchetype {
		t.Errorf("Stop = %v, want %v", sh.Stop, stopParamArchetype)
	}
	if sh.Root != "" || len(sh.Typed) != 0 {
		t.Errorf("verdict = %+v, want an unstarted walk", sh)
	}
}

// TestSegmentWalkRootsAtTheClassWhateverItsCase pins that the root name is
// case-folded before the pin is asked, exactly as the REQ-161 class checks
// fold it ([asciiUpperRMType]): the pin keys on UPPER_CASE BMM names, so an
// unfolded lookup would turn a lower-cased spelling into a silent
// unknown-class stop.
func TestSegmentWalkRootsAtTheClassWhateverItsCase(t *testing.T) {
	t.Parallel()
	sh := walkOne(t, "SELECT o/data/events/time FROM Observation o["+walkArch+"]")
	if sh.Root != "OBSERVATION" || sh.Stop != stopNone {
		t.Fatalf("verdict = %+v, want a completed walk rooted at OBSERVATION", sh)
	}
	if off := sh.offending(); len(off) != 1 || off[0].Name != "events" {
		t.Errorf("offending() = %+v, want the one unpredicated container", off)
	}
}

// TestSegmentWalkReadsPredicatePresenceNotContent pins the suppression rule at
// the walk level: three unrelated predicate spellings all read as present, and
// none is parsed or judged.
func TestSegmentWalkReadsPredicatePresenceNotContent(t *testing.T) {
	t.Parallel()
	for _, predicate := range []string{
		"at0006",
		"at0006, 'Systolic'",
		"name/value='Systolic'",
	} {
		t.Run(predicate, func(t *testing.T) {
			t.Parallel()
			sh := walkOne(t, "SELECT o/data/events["+predicate+"]/time FROM OBSERVATION o["+walkArch+"]")
			if len(sh.Typed) != 3 {
				t.Fatalf("typed %d segments, want 3: %+v", len(sh.Typed), sh.Typed)
			}
			events := sh.Typed[1]
			if !events.Container || !events.Predicated {
				t.Errorf("events = %+v, want a predicated container", events)
			}
			if off := sh.offending(); off != nil {
				t.Errorf("offending() = %+v, want nothing: a predicate suppresses the finding", off)
			}
		})
	}
}

// TestWalkStopNames pins the diagnostic names, including the out-of-range
// fallback: a reordered or extended enum must not leave a stop rendering as
// another stop's name in a failure message.
func TestWalkStopNames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stop walkStop
		want string
	}{
		{stopNone, "none"},
		{stopUnknownAliasClass, "unknown alias class"},
		{stopParamArchetype, "$param archetype scope"},
		{stopUndeclaredAttribute, "undeclared attribute"},
		{stopTypeOutsideUniverse, "type outside the class universe"},
		{walkStop(42), "walkStop(42)"},
	} {
		if got := tc.stop.String(); got != tc.want {
			t.Errorf("walkStop(%d).String() = %q, want %q", int(tc.stop), got, tc.want)
		}
	}
}

// --- aql_contains_redundant_step: the chain linearisation --------------------

// chainNames renders a linearised chain for a failure message.
func chainNames(chain []parse.ClassExpr) []string {
	out := make([]string, len(chain))
	for i, ce := range chain {
		out[i] = ce.RMType
	}
	return out
}

// TestContainsChainOrdersFlattenedSiblings exercises containsChain's own
// depth-first branch on a hand-built tree, for the same reason
// TestAndFrontierFlattensHandBuiltNestedAnd exists: the read-side extractor
// nests a parsed CONTAINS chain one level per keyword (a class node gets a
// single child), so no query [parse.Parse] can produce hands this function a
// class node with SEVERAL children. The flattened spelling is the same tree
// ([parse.Containment.Children] — the chain shape is not stable under
// re-parse), and it must linearise to the same order, or a following sibling
// would be judged against the wrong parent.
//
// `EHR e CONTAINS COMPOSITION c CONTAINS SECTION s CONTAINS OBSERVATION o
// CONTAINS CLUSTER cl`, spelled with SECTION and CLUSTER as siblings of one
// class node.
func TestContainsChainOrdersFlattenedSiblings(t *testing.T) {
	tree := parse.Containment{
		Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"},
		Children: []parse.Containment{
			{
				Class:    parse.ClassExpr{RMType: "SECTION", Alias: "s"},
				Children: []parse.Containment{{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "o"}}},
			},
			{Class: parse.ClassExpr{RMType: "CLUSTER", Alias: "cl"}},
		},
	}
	got, whole := containsChain([]parse.ClassExpr{{RMType: "EHR", Alias: "e"}}, tree)
	want := []string{"EHR", "COMPOSITION", "SECTION", "OBSERVATION", "CLUSTER"}
	if !slices.Equal(chainNames(got), want) {
		t.Errorf("chain = %v, want %v — a sibling follows the PREVIOUS child's chain tail, not the node itself",
			chainNames(got), want)
	}
	if !whole {
		t.Error("chain reported truncated although nothing stopped it")
	}
}

// TestContainsChainTruncatesAtADeepStop pins the consequence of a stop found
// INSIDE one child: everything after it is dropped, the following sibling
// included. Appending that sibling regardless would put it beside a node it
// does not follow, and the redundant-step check would then ask the relation
// about a parent/child pair the query never spelled — a wrong finding, not a
// missed one, which is the direction REQ-164 forbids.
func TestContainsChainTruncatesAtADeepStop(t *testing.T) {
	junction := parse.Containment{Children: []parse.Containment{
		{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "o"}},
		{Class: parse.ClassExpr{RMType: "EVALUATION", Alias: "ev"}},
	}}
	tree := parse.Containment{
		Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"},
		Children: []parse.Containment{
			{
				Class:    parse.ClassExpr{RMType: "SECTION", Alias: "s"},
				Children: []parse.Containment{junction},
			},
			{Class: parse.ClassExpr{RMType: "CLUSTER", Alias: "cl"}},
		},
	}
	got, whole := containsChain([]parse.ClassExpr{{RMType: "EHR", Alias: "e"}}, tree)
	want := []string{"EHR", "COMPOSITION", "SECTION"}
	if !slices.Equal(chainNames(got), want) {
		t.Errorf("chain = %v, want %v — a stop inside a child truncates the whole chain", chainNames(got), want)
	}
	if whole {
		t.Error("chain reported whole although a junction stopped it")
	}
}

// TestContainsChainStopsByName pins each of the three chain stops separately.
// From outside the package they all look alike — silence — so a guard swapped
// for another would pass the external suite unnoticed (the same reasoning the
// walk's own stops are pinned by name for, REQ-164 § Acceptance).
func TestContainsChainStopsByName(t *testing.T) {
	seed := []parse.ClassExpr{{RMType: "EHR", Alias: "e"}}
	for _, tc := range []struct {
		name string
		node parse.Containment
	}{
		{
			"junction",
			parse.Containment{Children: []parse.Containment{
				{Class: parse.ClassExpr{RMType: "OBSERVATION", Alias: "o"}},
				{Class: parse.ClassExpr{RMType: "EVALUATION", Alias: "ev"}},
			}},
		},
		{
			"negated",
			parse.Containment{Class: parse.ClassExpr{RMType: "COMPOSITION", Alias: "c"}, Negated: true},
		},
		{
			// A node with neither a class nor children — the dropped operand of
			// REQ-119. It decides nothing, and no pair built on it could either.
			"empty RM type",
			parse.Containment{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, whole := containsChain(seed, tc.node)
			if !slices.Equal(chainNames(got), []string{"EHR"}) {
				t.Errorf("chain = %v, want just the seed — a %s node contributes nothing", chainNames(got), tc.name)
			}
			if whole {
				t.Errorf("chain reported whole although a %s node stopped it", tc.name)
			}
		})
	}
}

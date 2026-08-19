package rminfo_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// hier asserts Default carries the optional capability interface and returns
// it. Consumers do exactly this assertion, so a signature drift on Hierarchy
// must fail here rather than at some consumer's call site.
func hier(t *testing.T) rminfo.Hierarchy {
	t.Helper()
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default does not implement Hierarchy")
	}
	return h
}

// TestIsAbstractReportsTheBMMFlag — REQ-048. Abstractness is the pinned BMM's
// is_abstract flag verbatim, including the classes where the flag is absent
// and the BMM's answer looks wrong (REQ-047: the BMM wins; STRAND-12 holds the
// wider question open). An unknown class is not a third boolean value — it is
// known=false.
func TestIsAbstractReportsTheBMMFlag(t *testing.T) {
	h := hier(t)
	cases := []struct {
		rmType         string
		abstract, know bool
	}{
		{"LOCATABLE", true, true},
		{"PATHABLE", true, true},
		{"ENTRY", true, true},
		{"CARE_ENTRY", true, true},
		{"DATA_VALUE", true, true},
		{"DV_ORDERED", true, true},
		{"COMPOSITION", false, true},
		{"OBSERVATION", false, true},
		{"DV_QUANTITY", false, true},
		// No is_abstract flag in the pinned BMM despite being a
		// P_BMM_INTERFACE the generator renders as a Go interface. The
		// surface reports the BMM, not a local correction — STRAND-12.
		{"CODE_SET_ACCESS", false, true},
		{"TERMINOLOGY_ACCESS", false, true},
		{"TERMINOLOGY_SERVICE", false, true},
		// Never defined by the pinned RM.
		{"NOT_AN_RM_CLASS", false, false},
		{"", false, false},
		// Excluded from the universe: ehr_extract (REQ-042) and enums.
		{"EXTRACT", false, false},
		{"PROPORTION_KIND", false, false},
	}
	for _, tc := range cases {
		abstract, known := h.IsAbstract(tc.rmType)
		if known != tc.know || abstract != tc.abstract {
			t.Errorf("IsAbstract(%q) = (%t, %t), want (%t, %t)",
				tc.rmType, abstract, known, tc.abstract, tc.know)
		}
	}
}

// TestParentsAreImmediateAndInBMMOrder — REQ-048. Parents is the faithful edge
// set; a transitive closure erases it, which is why both questions exist.
func TestParentsAreImmediateAndInBMMOrder(t *testing.T) {
	h := hier(t)
	cases := []struct {
		rmType string
		want   []string
	}{
		{"OBSERVATION", []string{"CARE_ENTRY"}},
		{"CARE_ENTRY", []string{"ENTRY"}},
		{"LOCATABLE", []string{"PATHABLE"}},
		// PATHABLE's only BMM ancestor is `Any`, outside the universe, so
		// PATHABLE is a root: an EMPTY answer with known=true.
		{"PATHABLE", nil},
		// Genuine multiple inheritance keeps both edges, in BMM order —
		// not sorted, which would lose the declaration order.
		{"TERMINOLOGY_SERVICE", []string{"OPENEHR_TERMINOLOGY_GROUP_IDENTIFIERS", "OPENEHR_CODE_SET_IDENTIFIERS"}},
	}
	for _, tc := range cases {
		got, known := h.Parents(tc.rmType)
		if !known {
			t.Errorf("Parents(%q): known=false, want a known class", tc.rmType)
			continue
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("Parents(%q) = %v, want %v", tc.rmType, got, tc.want)
		}
	}
	if got, known := h.Parents("NOT_AN_RM_CLASS"); known || got != nil {
		t.Errorf("Parents(unknown) = (%v, %t), want (nil, false)", got, known)
	}
}

// TestAncestorsAreTransitiveAndSorted — REQ-048.
func TestAncestorsAreTransitiveAndSorted(t *testing.T) {
	h := hier(t)
	got, known := h.Ancestors("OBSERVATION")
	if !known {
		t.Fatal("Ancestors(OBSERVATION): known=false")
	}
	for _, want := range []string{"CARE_ENTRY", "ENTRY", "CONTENT_ITEM", "LOCATABLE", "PATHABLE"} {
		if !slices.Contains(got, want) {
			t.Errorf("Ancestors(OBSERVATION) = %v, missing %q", got, want)
		}
	}
	if slices.Contains(got, "OBSERVATION") {
		t.Error("Ancestors(OBSERVATION) contains OBSERVATION — ancestors are strict")
	}
	if !slices.IsSorted(got) {
		t.Errorf("Ancestors(OBSERVATION) = %v, not sorted", got)
	}
	// A root reports known with an empty answer, which is a different
	// answer from an unknown class.
	if anc, known := h.Ancestors("PATHABLE"); !known || len(anc) != 0 {
		t.Errorf("Ancestors(PATHABLE) = (%v, %t), want (empty, true)", anc, known)
	}
	if anc, known := h.Ancestors("NOT_AN_RM_CLASS"); known || anc != nil {
		t.Errorf("Ancestors(unknown) = (%v, %t), want (nil, false)", anc, known)
	}
}

// TestAncestorsAreClosedOverTheKnownUniverse — REQ-048. Every name any answer
// returns must itself be askable; a leaked foundation-typing name (Any,
// Ordered, Interval, Iso8601_*) would be a dead end for the caller.
func TestAncestorsAreClosedOverTheKnownUniverse(t *testing.T) {
	h := hier(t)
	known := rminfo.Default.KnownRMTypes()
	for _, class := range known {
		ancestors, ok := h.Ancestors(class)
		if !ok {
			t.Errorf("KnownRMTypes lists %q but Ancestors reports it unknown", class)
			continue
		}
		for _, a := range ancestors {
			if !slices.Contains(known, a) {
				t.Errorf("Ancestors(%q) returns %q, which is not a known RM type", class, a)
			}
		}
		descendants, ok := h.ConcreteDescendants(class)
		if !ok {
			t.Errorf("KnownRMTypes lists %q but ConcreteDescendants reports it unknown", class)
			continue
		}
		for _, d := range descendants {
			if !slices.Contains(known, d) {
				t.Errorf("ConcreteDescendants(%q) returns %q, which is not a known RM type", class, d)
			}
		}
	}
}

// TestConformsToAgreesWithAncestors — REQ-048. ConformsTo is reflexive and
// must not disagree with the ancestor closure it is derived from.
func TestConformsToAgreesWithAncestors(t *testing.T) {
	h := hier(t)
	cases := []struct {
		sub, super     string
		conforms, know bool
	}{
		{"OBSERVATION", "ENTRY", true, true},
		{"OBSERVATION", "LOCATABLE", true, true},
		{"OBSERVATION", "OBSERVATION", true, true}, // reflexive
		{"ENTRY", "OBSERVATION", false, true},      // not symmetric
		{"OBSERVATION", "EVALUATION", false, true},
		{"DV_QUANTITY", "DATA_VALUE", true, true},
		{"DV_CODED_TEXT", "DV_TEXT", true, true},
		{"DV_TEXT", "DV_CODED_TEXT", false, true},
		// known=false when EITHER name is undefined, so "no" and "never
		// heard of it" stay distinguishable.
		{"OBSERVATION", "NOT_AN_RM_CLASS", false, false},
		{"NOT_AN_RM_CLASS", "ENTRY", false, false},
		{"NOT_AN_RM_CLASS", "ALSO_NOT_ONE", false, false},
	}
	for _, tc := range cases {
		conforms, known := h.ConformsTo(tc.sub, tc.super)
		if conforms != tc.conforms || known != tc.know {
			t.Errorf("ConformsTo(%q, %q) = (%t, %t), want (%t, %t)",
				tc.sub, tc.super, conforms, known, tc.conforms, tc.know)
		}
	}
	// Cross-check the whole graph: ConformsTo(sub, super) must hold exactly
	// when super is sub or one of sub's ancestors.
	for _, sub := range rminfo.Default.KnownRMTypes() {
		ancestors, _ := h.Ancestors(sub)
		for _, super := range rminfo.Default.KnownRMTypes() {
			want := super == sub || slices.Contains(ancestors, super)
			got, known := h.ConformsTo(sub, super)
			if !known {
				t.Fatalf("ConformsTo(%q, %q): known=false for two known classes", sub, super)
			}
			if got != want {
				t.Errorf("ConformsTo(%q, %q) = %t, but Ancestors says %t", sub, super, got, want)
			}
		}
	}
}

// TestConcreteDescendantsExpandsAbstractClasses — REQ-048. This is the AQL
// class-expression question: naming an abstract class denotes its concrete
// descendants.
func TestConcreteDescendantsExpandsAbstractClasses(t *testing.T) {
	h := hier(t)

	got, known := h.ConcreteDescendants("ENTRY")
	if !known {
		t.Fatal("ConcreteDescendants(ENTRY): known=false")
	}
	want := []string{"ACTION", "ADMIN_ENTRY", "EVALUATION", "INSTRUCTION", "OBSERVATION"}
	if !slices.Equal(got, want) {
		t.Errorf("ConcreteDescendants(ENTRY) = %v, want %v", got, want)
	}

	// CARE_ENTRY is abstract and excludes ADMIN_ENTRY, which descends from
	// ENTRY directly — the two expansions must differ.
	care, _ := h.ConcreteDescendants("CARE_ENTRY")
	if slices.Contains(care, "ADMIN_ENTRY") {
		t.Errorf("ConcreteDescendants(CARE_ENTRY) = %v, must not contain ADMIN_ENTRY", care)
	}
	if slices.Contains(care, "CARE_ENTRY") {
		t.Errorf("ConcreteDescendants(CARE_ENTRY) = %v: an abstract class is not its own concrete descendant", care)
	}

	// A concrete class denotes itself, and carries its concrete
	// descendants too (DV_TEXT is storable AND has DV_CODED_TEXT under it).
	quantity, _ := h.ConcreteDescendants("DV_QUANTITY")
	if !slices.Equal(quantity, []string{"DV_QUANTITY"}) {
		t.Errorf("ConcreteDescendants(DV_QUANTITY) = %v, want [DV_QUANTITY]", quantity)
	}
	text, _ := h.ConcreteDescendants("DV_TEXT")
	if !slices.Equal(text, []string{"DV_CODED_TEXT", "DV_TEXT"}) {
		t.Errorf("ConcreteDescendants(DV_TEXT) = %v, want [DV_CODED_TEXT DV_TEXT]", text)
	}

	// DATA_VALUE is the abstract apex of the DV_* family (not a root — its
	// parent is OPENEHR_DEFINITIONS) that the pre-REQ-048 table could not
	// even name; its expansion must reach the whole family.
	dv, known := h.ConcreteDescendants("DATA_VALUE")
	if !known {
		t.Fatal("ConcreteDescendants(DATA_VALUE): known=false — the DV_* apex must be askable")
	}
	for _, w := range []string{"DV_QUANTITY", "DV_CODED_TEXT", "DV_DATE_TIME", "DV_ORDINAL", "DV_MULTIMEDIA", "DV_INTERVAL"} {
		if !slices.Contains(dv, w) {
			t.Errorf("ConcreteDescendants(DATA_VALUE) = %v, missing %q", dv, w)
		}
	}
	if slices.Contains(dv, "DATA_VALUE") || slices.Contains(dv, "DV_ORDERED") {
		t.Errorf("ConcreteDescendants(DATA_VALUE) = %v, must exclude the abstract classes", dv)
	}
	if !slices.IsSorted(dv) {
		t.Errorf("ConcreteDescendants(DATA_VALUE) = %v, not sorted", dv)
	}

	// Every member of an expansion is itself non-abstract, on every class.
	for _, class := range rminfo.Default.KnownRMTypes() {
		ds, _ := h.ConcreteDescendants(class)
		for _, d := range ds {
			if abstract, _ := h.IsAbstract(d); abstract {
				t.Errorf("ConcreteDescendants(%q) includes abstract class %q", class, d)
			}
			if conforms, _ := h.ConformsTo(d, class); !conforms {
				t.Errorf("ConcreteDescendants(%q) includes %q, which does not conform to it", class, d)
			}
		}
	}

	if ds, known := h.ConcreteDescendants("NOT_AN_RM_CLASS"); known || ds != nil {
		t.Errorf("ConcreteDescendants(unknown) = (%v, %t), want (nil, false)", ds, known)
	}
}

// TestDeclaredOnRecoversTheInheritanceSite — REQ-048. The flattened tables
// answer the RESOLVED question; DeclaredOn recovers the site the fold erased.
func TestDeclaredOnRecoversTheInheritanceSite(t *testing.T) {
	h := hier(t)
	cases := []struct {
		rmType, attr, want string
	}{
		// Inherited from LOCATABLE by every descendant.
		{"COMPOSITION", "name", "LOCATABLE"},
		{"OBSERVATION", "archetype_node_id", "LOCATABLE"},
		{"DV_QUANTITY", "magnitude", "DV_QUANTITY"},
		// Own attributes report the class itself.
		{"COMPOSITION", "composer", "COMPOSITION"},
		{"OBSERVATION", "data", "OBSERVATION"},
		// Inherited from an intermediate abstract class, not the root.
		{"OBSERVATION", "language", "ENTRY"},
		{"DV_QUANTITY", "units", "DV_QUANTITY"},
	}
	for _, tc := range cases {
		got, ok := h.DeclaredOn(tc.rmType, tc.attr)
		if !ok {
			t.Errorf("DeclaredOn(%q, %q): not found", tc.rmType, tc.attr)
			continue
		}
		if got != tc.want {
			t.Errorf("DeclaredOn(%q, %q) = %q, want %q", tc.rmType, tc.attr, got, tc.want)
		}
	}

	// Not-found rather than a guess, on both axes.
	for _, tc := range []struct{ rmType, attr string }{
		{"COMPOSITION", "no_such_attribute"},
		{"NOT_AN_RM_CLASS", "name"},
		{"COMPOSITION", ""},
	} {
		if got, ok := h.DeclaredOn(tc.rmType, tc.attr); ok {
			t.Errorf("DeclaredOn(%q, %q) = (%q, true), want not-found", tc.rmType, tc.attr, got)
		}
	}

	// The flattened tables stay flattened: the resolved lookup keeps
	// answering for an inherited attribute, unchanged by REQ-048.
	if got, ok := rminfo.Default.AttributeRMType("COMPOSITION", "name"); !ok || got != "DV_TEXT" {
		t.Errorf("AttributeRMType(COMPOSITION, name) = (%q, %t), want (DV_TEXT, true)", got, ok)
	}
}

// foundationDeclaringSites is the pinned set of declaration sites that name a
// class OUTSIDE the known universe — attributes inherited from the foundation
// layer the RM target does not emit. DeclaredOn is faithful to the BMM here
// rather than clamping to the carrying class, which would report the bounds as
// locally declared by three RM classes that declare none of them.
//
// Pinned so that a new out-of-universe site is a deliberate edit: silently
// growing this set would mean an RM attribute quietly became unattributable.
var foundationDeclaringSites = map[string][]string{
	"Interval": {"DV_INTERVAL", "Point_interval", "Proper_interval"},
}

// TestDeclaredOnNamesFoundationSitesFaithfully — REQ-048.
func TestDeclaredOnNamesFoundationSitesFaithfully(t *testing.T) {
	h := hier(t)
	known := rminfo.Default.KnownRMTypes()
	for site, carriers := range foundationDeclaringSites {
		if slices.Contains(known, site) {
			t.Errorf("%q is pinned as a foundation site but IS a known class — update the pin", site)
		}
		for _, carrier := range carriers {
			got, ok := h.DeclaredOn(carrier, "lower")
			if !ok || got != site {
				t.Errorf("DeclaredOn(%s, lower) = (%q, %t), want (%q, true)", carrier, got, ok, site)
			}
		}
	}
	// The carrying class does not itself declare the attribute, which is
	// exactly the distinction a clamped answer would destroy.
	if got, _ := h.DeclaredOn("DV_INTERVAL", "lower"); got == "DV_INTERVAL" {
		t.Error("DeclaredOn(DV_INTERVAL, lower) reports DV_INTERVAL — the foundation site was clamped away")
	}
}

// TestDeclaredOnAgreesWithTheFlattenedTables — REQ-048, the agreement rule.
// For every class and every attribute it carries: the site is the class, one
// of its ancestors, or a pinned foundation site, and an in-universe site
// reports the same type/required/container.
func TestDeclaredOnAgreesWithTheFlattenedTables(t *testing.T) {
	h := hier(t)
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not implement AttributeLister")
	}
	checked := 0
	for _, class := range rminfo.Default.KnownRMTypes() {
		ancestors, _ := h.Ancestors(class)
		required := rminfo.Default.RequiredAttributes(class)
		for _, attr := range lister.AttributeNames(class) {
			checked++
			site, ok := h.DeclaredOn(class, attr)
			if !ok {
				t.Errorf("%s carries %q but DeclaredOn reports no site", class, attr)
				continue
			}
			if carriers, pinned := foundationDeclaringSites[site]; pinned {
				// An out-of-universe site has no row in the tables, so
				// the shape comparison below cannot run. What must hold
				// is that only the pinned carriers reach it.
				if !slices.Contains(carriers, class) {
					t.Errorf("DeclaredOn(%s, %s) = %s, an unpinned carrier of a foundation site",
						class, attr, site)
				}
				continue
			}
			if site != class && !slices.Contains(ancestors, site) {
				t.Errorf("DeclaredOn(%s, %s) = %s, which is neither the class, an ancestor, nor a pinned foundation site",
					class, attr, site)
				continue
			}
			gotType, _ := rminfo.Default.AttributeRMType(class, attr)
			siteType, siteOK := rminfo.Default.AttributeRMType(site, attr)
			if !siteOK {
				t.Errorf("DeclaredOn(%s, %s) = %s, but %s does not carry %s",
					class, attr, site, site, attr)
				continue
			}
			if gotType != siteType {
				t.Errorf("%s.%s is %q but its site %s reports %q", class, attr, gotType, site, siteType)
			}
			gotContainer, _ := rminfo.Default.IsContainer(class, attr)
			siteContainer, _ := rminfo.Default.IsContainer(site, attr)
			if gotContainer != siteContainer {
				t.Errorf("%s.%s container=%t but its site %s says %t",
					class, attr, gotContainer, site, siteContainer)
			}
			siteRequired := slices.Contains(rminfo.Default.RequiredAttributes(site), attr)
			if slices.Contains(required, attr) != siteRequired {
				t.Errorf("%s.%s required=%t but its site %s says %t",
					class, attr, slices.Contains(required, attr), site, siteRequired)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no attribute was checked — the agreement rule went unasserted")
	}
	t.Logf("checked %d attribute sites", checked)
}

// TestHierarchyReturnsCopiesNotPackageState — REQ-048. A caller that sorts or
// truncates a returned slice must not corrupt the next caller's answer.
func TestHierarchyReturnsCopiesNotPackageState(t *testing.T) {
	h := hier(t)
	mutators := map[string]func() []string{
		"Parents":             func() []string { p, _ := h.Parents("TERMINOLOGY_SERVICE"); return p },
		"Ancestors":           func() []string { a, _ := h.Ancestors("OBSERVATION"); return a },
		"ConcreteDescendants": func() []string { d, _ := h.ConcreteDescendants("ENTRY"); return d },
	}
	for name, get := range mutators {
		first := get()
		if len(first) == 0 {
			t.Fatalf("%s: fixture returned nothing to mutate", name)
		}
		before := slices.Clone(first)
		for i := range first {
			first[i] = "CLOBBERED"
		}
		if after := get(); !slices.Equal(after, before) {
			t.Errorf("%s: mutating a returned slice changed the next answer: %v != %v", name, after, before)
		}
	}
}

// TestHierarchyOverSyntheticModel — REQ-048. The New(data) seam must reach
// every question, because the pinned RM cannot supply every shape: a dead-end
// abstract class, and a class whose parent is NOT in the data set.
//
// The dangling parent is the one place closure is a run-time question rather
// than a property of the generated table. The documented behaviour is that the
// name comes back verbatim and then reports known=false — the surface does not
// filter, because filtering would hide a generator defect instead of failing
// on it.
func TestHierarchyOverSyntheticModel(t *testing.T) {
	data := map[string]rminfo.ClassMeta{
		"ROOT": {Abstract: true},
		"MID": {
			Abstract: true,
			Parents:  []string{"ROOT"},
			Attributes: map[string]rminfo.AttrMeta{
				"shared": {TypeName: "String", DeclaredIn: "MID"},
			},
			AttrOrder: []string{"shared"},
		},
		"LEAF": {
			Parents: []string{"MID"},
			Attributes: map[string]rminfo.AttrMeta{
				"shared": {TypeName: "String", DeclaredIn: "MID"},
				"own":    {TypeName: "String", DeclaredIn: "LEAF"},
			},
			AttrOrder: []string{"shared", "own"},
		},
		// Abstract with nothing concrete under it: a dead end that must
		// still report as known with an empty expansion.
		"DEAD_END": {Abstract: true, Parents: []string{"ROOT"}},
		// Names a parent this data set does not define.
		"ORPHAN": {Parents: []string{"NOT_IN_DATA"}},
	}
	h, ok := rminfo.New(data).(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.New(data) does not implement Hierarchy")
	}

	if ds, known := h.ConcreteDescendants("DEAD_END"); !known || len(ds) != 0 {
		t.Errorf("ConcreteDescendants(DEAD_END) = (%v, %t), want (empty, true) — a dead end is known", ds, known)
	}
	if ds, known := h.ConcreteDescendants("ROOT"); !known || !slices.Equal(ds, []string{"LEAF"}) {
		t.Errorf("ConcreteDescendants(ROOT) = (%v, %t), want ([LEAF], true)", ds, known)
	}
	if anc, known := h.Ancestors("LEAF"); !known || !slices.Equal(anc, []string{"MID", "ROOT"}) {
		t.Errorf("Ancestors(LEAF) = (%v, %t), want ([MID ROOT], true)", anc, known)
	}
	if anc, known := h.Ancestors("ROOT"); !known || len(anc) != 0 {
		t.Errorf("Ancestors(ROOT) = (%v, %t), want (empty, true)", anc, known)
	}
	if got, ok := h.DeclaredOn("LEAF", "shared"); !ok || got != "MID" {
		t.Errorf("DeclaredOn(LEAF, shared) = (%q, %t), want (MID, true)", got, ok)
	}
	if got, ok := h.DeclaredOn("LEAF", "own"); !ok || got != "LEAF" {
		t.Errorf("DeclaredOn(LEAF, own) = (%q, %t), want (LEAF, true)", got, ok)
	}

	// The dangling parent: returned verbatim, and then unanswerable. Both
	// halves matter — dropping it would silently shrink the answer, and
	// reporting it as known would be a lie.
	if anc, known := h.Ancestors("ORPHAN"); !known || !slices.Equal(anc, []string{"NOT_IN_DATA"}) {
		t.Errorf("Ancestors(ORPHAN) = (%v, %t), want ([NOT_IN_DATA], true) — an undefined parent is not filtered", anc, known)
	}
	if _, known := h.IsAbstract("NOT_IN_DATA"); known {
		t.Error("IsAbstract(NOT_IN_DATA): known=true for a name the data set does not define")
	}
	if conforms, known := h.ConformsTo("ORPHAN", "NOT_IN_DATA"); known || conforms {
		t.Errorf("ConformsTo(ORPHAN, NOT_IN_DATA) = (%t, %t), want (false, false)", conforms, known)
	}
	// And it does not become a phantom class in the descendant direction.
	if ds, known := h.ConcreteDescendants("NOT_IN_DATA"); known || ds != nil {
		t.Errorf("ConcreteDescendants(NOT_IN_DATA) = (%v, %t), want (nil, false)", ds, known)
	}
}

// TestHierarchyTerminatesOnCyclicSyntheticModel — REQ-048. The pinned BMM has
// no ancestor cycle, but New(data) admits any map, and a walk that trusted
// the data would hang instead of answering. Library code does not hang.
func TestHierarchyTerminatesOnCyclicSyntheticModel(t *testing.T) {
	data := map[string]rminfo.ClassMeta{
		"A": {Parents: []string{"B"}},
		"B": {Parents: []string{"A"}},
		"C": {Parents: []string{"C"}},
	}
	h, ok := rminfo.New(data).(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.New(data) does not implement Hierarchy")
	}
	// Reaching the assertions at all is the test: an unguarded walk never
	// returns from these calls.
	if anc, known := h.Ancestors("A"); !known || !slices.Equal(anc, []string{"B"}) {
		t.Errorf("Ancestors(A) = (%v, %t), want ([B], true) — self excluded, cycle broken", anc, known)
	}
	if anc, known := h.Ancestors("C"); !known || len(anc) != 0 {
		t.Errorf("Ancestors(C) = (%v, %t), want (empty, true) — a self-parent is not its own ancestor", anc, known)
	}
	if ds, known := h.ConcreteDescendants("A"); !known || !slices.Equal(ds, []string{"A", "B"}) {
		t.Errorf("ConcreteDescendants(A) = (%v, %t), want ([A B], true)", ds, known)
	}
}

// TestHierarchyConcurrentFirstUse — REQ-048. The descendant index is built
// lazily under a sync.Once, beside the pre-existing KnownRMTypes cache. Two
// lazy caches on one shared Default is where a data race would live, so the
// first use is driven from many goroutines at once: this test is written to be
// meaningful under `go test -race`, and it also catches a Once that publishes a
// half-built map by returning a wrong answer.
func TestHierarchyConcurrentFirstUse(t *testing.T) {
	// A fresh Lookup, so the caches really are cold when the goroutines start.
	h, ok := rminfo.New(rminfoDefaultDataCopy(t)).(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.New does not implement Hierarchy")
	}
	lookup, ok := h.(rminfo.Lookup)
	if !ok {
		t.Fatal("the Hierarchy is not also a Lookup")
	}
	const goroutines = 32
	descendants := make([][]string, goroutines)
	knownCounts := make([]int, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			// Both lazy caches on the same cold value, from the same
			// goroutine, so childOnce and knownOnce race each other too.
			descendants[i], _ = h.ConcreteDescendants("ENTRY")
			knownCounts[i] = len(lookup.KnownRMTypes())
		})
	}
	wg.Wait()
	want := []string{"ACTION", "ADMIN_ENTRY", "EVALUATION", "INSTRUCTION", "OBSERVATION"}
	wantKnown := len(rminfo.Default.KnownRMTypes())
	for i := range goroutines {
		if !slices.Equal(descendants[i], want) {
			t.Errorf("goroutine %d saw ConcreteDescendants(ENTRY) = %v, want %v", i, descendants[i], want)
		}
		if knownCounts[i] != wantKnown {
			t.Errorf("goroutine %d saw %d known types, want %d", i, knownCounts[i], wantKnown)
		}
	}
}

// rminfoDefaultDataCopy rebuilds a data map equivalent to the generated one,
// through the public surface, so a test can exercise a COLD Lookup without
// reaching into package state.
func rminfoDefaultDataCopy(t *testing.T) map[string]rminfo.ClassMeta {
	t.Helper()
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default does not implement Hierarchy")
	}
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not implement AttributeLister")
	}
	out := map[string]rminfo.ClassMeta{}
	for _, class := range rminfo.Default.KnownRMTypes() {
		abstract, _ := h.IsAbstract(class)
		parents, _ := h.Parents(class)
		order := lister.AttributeNames(class)
		attrs := make(map[string]rminfo.AttrMeta, len(order))
		required := rminfo.Default.RequiredAttributes(class)
		for _, attr := range order {
			typeName, _ := rminfo.Default.AttributeRMType(class, attr)
			container, _ := rminfo.Default.IsContainer(class, attr)
			site, _ := h.DeclaredOn(class, attr)
			attrs[attr] = rminfo.AttrMeta{
				TypeName:   typeName,
				Required:   slices.Contains(required, attr),
				Container:  container,
				DeclaredIn: site,
			}
		}
		out[class] = rminfo.ClassMeta{
			Attributes: attrs,
			AttrOrder:  order,
			Abstract:   abstract,
			Parents:    parents,
		}
	}
	return out
}

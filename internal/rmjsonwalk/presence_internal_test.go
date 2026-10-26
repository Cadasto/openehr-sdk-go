package rmjsonwalk

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// countingLookup is rminfo.Default with its RequiredAttributes calls counted.
type countingLookup struct {
	rminfo.Lookup
	rminfo.Hierarchy
	rminfo.AbsenceReporter
	calls map[string]int
}

func newCountingLookup() *countingLookup {
	return &countingLookup{
		Lookup:          rminfo.Default,
		Hierarchy:       rminfo.Default.(rminfo.Hierarchy),
		AbsenceReporter: rminfo.Default.(rminfo.AbsenceReporter),
		calls:           map[string]int{},
	}
}

func (c *countingLookup) RequiredAttributes(rmType string) []string {
	c.calls[rmType]++
	return c.Lookup.RequiredAttributes(rmType)
}

// TestREQ175_RequiredIsComputedOncePerClass pins the cache: the
// presence-checked list of a class is read from the RM once, on first use,
// and every later walk gets the same list.
func TestREQ175_RequiredIsComputedOncePerClass(t *testing.T) {
	l := newCountingLookup()
	m, err := New(l)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	doc := []byte(`[{"units":"kg"},{"magnitude":1},{"_type":"DV_COUNT"}]`)
	for range 3 {
		err := m.Walk(doc, Root{Nest: "[", Type: "DV_QUANTITY"}, Hooks{Missing: func(string) error { return nil }})
		if err != nil {
			t.Fatalf("Walk(): %v", err)
		}
	}
	for _, class := range []string{"DV_QUANTITY", "DV_COUNT"} {
		if got := l.calls[class]; got != 1 {
			t.Errorf("RequiredAttributes(%s) called %d times over three walks of three objects, want once", class, got)
		}
	}
	a, b := m.required("DV_QUANTITY"), m.required("DV_QUANTITY")
	if len(a) == 0 || &a[0] != &b[0] {
		t.Errorf("required(DV_QUANTITY) = %q then %q, want one kept list", a, b)
	}
}

// TestREQ175_RequiredIsNotReadWithoutTheMissingHook pins that a walk given no
// Missing hook never reads the presence-checked lists: the cost of the
// Unmatched walk is unchanged.
func TestREQ175_RequiredIsNotReadWithoutTheMissingHook(t *testing.T) {
	l := newCountingLookup()
	m, err := New(l)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if err := m.Unmatched([]byte(`{"_type":"DV_QUANTITY","x-a":1}`), Root{}, func(string) error { return nil }); err != nil {
		t.Fatalf("Unmatched(): %v", err)
	}
	if len(l.calls) != 0 {
		t.Errorf("RequiredAttributes called for %v during an Unmatched walk, want no call", l.calls)
	}
}

// TestREQ175_RequiredLeavesOutWhatNoSerializationCarries pins the
// presence-checked list of every class: the RM's mandatory attributes, in
// the RM's order, minus the attributes no serialization carries and the four
// interval boundary flags.
func TestREQ175_RequiredLeavesOutWhatNoSerializationCarries(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	flags := []string{"lower_unbounded", "upper_unbounded", "lower_included", "upper_included"}
	for _, class := range rminfo.Default.KnownRMTypes() {
		got := m.required(class)
		var want []string
		for _, attr := range rminfo.Default.RequiredAttributes(class) {
			if !rminfo.IsNonStorableAttr(class, attr) && !slices.Contains(flags, attr) {
				want = append(want, attr)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("required(%s) = %q, want %q", class, got, want)
		}
	}
	// The exclusions each cut a list in the pinned RM; a list that nothing
	// cuts would not show the filter works.
	for class, cut := range map[string]string{
		"RESOURCE_DESCRIPTION": "parent_resource",
		"DV_INTERVAL":          "lower_unbounded",
		"Point_interval":       "upper_included",
	} {
		if !slices.Contains(rminfo.Default.RequiredAttributes(class), cut) {
			t.Errorf("RequiredAttributes(%s) lacks %s; the control no longer shows the filter", class, cut)
		}
		if slices.Contains(m.required(class), cut) {
			t.Errorf("required(%s) carries %s, want it left out", class, cut)
		}
	}
	if got := m.required("NO_SUCH_CLASS"); len(got) != 0 {
		t.Errorf("required(NO_SUCH_CLASS) = %q, want nothing", got)
	}
}

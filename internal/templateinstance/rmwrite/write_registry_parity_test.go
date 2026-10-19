package rmwrite

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// The two tests below tie the hand-written writers to the RM metadata. The
// instance generator fills BMM-required defaults through EnsureSingle and
// AppendMultiple and drops their error, so a writer case that goes missing
// drops a required default without an error. Neither test keeps a hand list of
// writer cases: they ask the writers about every pair of a registered class
// and an attribute name, and compare the answers with openehr/rm/rminfo.

// probeChild is a child value no writer accepts: it is no RM type and
// implements no RM interface. Handing it to a writer exercises the writer's
// dispatch on the attribute name without completing a write.
type probeChild struct{}

// writeFunc is the shape EnsureSingle and AppendMultiple share.
type writeFunc func(parent any, attrName string, child any) error

// armShape records how the two writers answer one (class, attribute) pair.
type armShape struct {
	single    bool // EnsureSingle has a case for the attribute
	multiple  bool // AppendMultiple has a case for the attribute
	tookProbe bool // a writer stored probeChild instead of refusing it
}

// armTable maps an RM class to the attributes the writers have a case for.
// A class neither writer has a case for is absent.
type armTable map[string]map[string]armShape

// rmModel is the part of the RM metadata the parity checks read.
type rmModel interface {
	rminfo.Lookup
	rminfo.AttributeLister
	rminfo.Hierarchy
}

// modelOf asserts that l carries the optional capabilities the checks read.
func modelOf(t *testing.T, l rminfo.Lookup) rmModel {
	t.Helper()
	m, ok := l.(rmModel)
	if !ok {
		t.Fatalf("%T does not implement AttributeLister and Hierarchy", l)
	}
	return m
}

// probeArms asks both writers about every attribute name on a fresh value of
// every class, handing them probeChild. Any answer other than
// ErrUnknownAttribute means the writer has a case for the attribute. The
// child is never one a case accepts, so the answer is about dispatch, not
// about a write.
func probeArms(parents map[string]func() any, attrs []string, single, multiple writeFunc) armTable {
	arms := armTable{}
	for class, newParent := range parents {
		for _, attr := range attrs {
			var shape armShape
			var tookSingle, tookMultiple bool
			shape.single, tookSingle = dispatches(single, newParent(), attr)
			shape.multiple, tookMultiple = dispatches(multiple, newParent(), attr)
			shape.tookProbe = tookSingle || tookMultiple
			if shape == (armShape{}) {
				continue
			}
			if arms[class] == nil {
				arms[class] = map[string]armShape{}
			}
			arms[class][attr] = shape
		}
	}
	return arms
}

// dispatches reports whether write has a case for attr on parent, and
// whether that case stored probeChild (answered nil).
func dispatches(write writeFunc, parent any, attr string) (hasCase, tookProbe bool) {
	err := write(parent, attr, probeChild{})
	return !errors.Is(err, ErrUnknownAttribute), err == nil
}

// armViolations lists every writer case the RM metadata does not back: a
// case for an attribute the class does not declare, a case in the other
// shape than IsContainer gives the attribute, and a case that stores a child
// no RM attribute holds. The list is sorted by class, then attribute.
func armViolations(arms armTable, model rmModel) []string {
	var out []string
	for _, class := range slices.Sorted(maps.Keys(arms)) {
		for _, attr := range slices.Sorted(maps.Keys(arms[class])) {
			shape := arms[class][attr]
			if shape.tookProbe {
				out = append(out, fmt.Sprintf("%s.%s: a writer stored a child no RM attribute holds", class, attr))
			}
			container, declared := model.IsContainer(class, attr)
			if !declared {
				out = append(out, fmt.Sprintf("%s.%s: a writer has a case for it, but the class does not declare it", class, attr))
				continue
			}
			if shape.single && container {
				out = append(out, fmt.Sprintf("%s.%s: EnsureSingle has a case for it, but it is a list attribute", class, attr))
			}
			if shape.multiple && !container {
				out = append(out, fmt.Sprintf("%s.%s: AppendMultiple has a case for it, but it is single-valued", class, attr))
			}
		}
	}
	return out
}

// unwrittenAttributes returns, for each class the writers write into, the
// attributes the class declares that neither writer has a case for, sorted.
// It leaves out the LOCATABLE attributes other than name. The instance
// generator stamps archetype_node_id, name, uid and archetype_details
// itself; name stays in because rmwrite also writes it when an OPT
// constrains it. links and feeder_audit are optional in the RM, and
// nothing fills them.
func unwrittenAttributes(arms armTable, model rmModel) map[string][]string {
	out := make(map[string][]string, len(arms))
	for class, written := range arms {
		missing := []string{}
		for _, attr := range model.AttributeNames(class) {
			if _, ok := written[attr]; ok {
				continue
			}
			if site, _ := model.DeclaredOn(class, attr); site == "LOCATABLE" && attr != "name" {
				continue
			}
			missing = append(missing, attr)
		}
		slices.Sort(missing)
		out[class] = missing
	}
	return out
}

// residualDrift lists each class whose unwritten attributes differ from the
// pinned ones, sorted by class.
func residualDrift(got, pinned map[string][]string) []string {
	classes := slices.Sorted(maps.Keys(got))
	for class := range pinned {
		if _, ok := got[class]; !ok {
			classes = append(classes, class)
		}
	}
	slices.Sort(classes)

	var out []string
	for _, class := range classes {
		g, written := got[class]
		p, isPinned := pinned[class]
		switch {
		case !written:
			out = append(out, class+": pinned, but no writer writes into it any more")
		case !isPinned:
			out = append(out, fmt.Sprintf("%s: a writer writes into it, but it is not pinned (unwritten: %v)", class, g))
		case !slices.Equal(g, p):
			out = append(out, fmt.Sprintf("%s: newly unwritten %v, newly written %v", class, without(g, p), without(p, g)))
		}
	}
	return out
}

// without returns the members of a that b lacks, in a's order.
func without(a, b []string) []string {
	out := []string{}
	for _, s := range a {
		if !slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}

// registryParents returns a constructor for every class the type registry
// knows, keyed by its RM class name.
func registryParents() map[string]func() any {
	out := map[string]func() any{}
	for _, name := range typereg.Default.Names() {
		if ctor, ok := typereg.Default.Lookup(name); ok {
			out[name] = ctor
		}
	}
	return out
}

// probeAttrNames returns every attribute name any class of model declares,
// sorted, so each class is also asked about the names it does not declare.
func probeAttrNames(model rmModel) []string {
	names := map[string]struct{}{}
	for _, class := range model.KnownRMTypes() {
		for _, attr := range model.AttributeNames(class) {
			names[attr] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(names))
}

// registryArms probes EnsureSingle and AppendMultiple over the registry.
func registryArms(t *testing.T, model rmModel) armTable {
	t.Helper()
	arms := probeArms(registryParents(), probeAttrNames(model), EnsureSingle, AppendMultiple)
	if len(arms) == 0 {
		t.Fatal("no writer case found; is the type registry empty?")
	}
	return arms
}

// --- the can-fail fixture -------------------------------------------------

// The fixture is a two-class model whose writers break each rule once, so
// the checks are seen to fail. FIX_PARENT is written into; FIX_OTHER is a
// class no writer writes into, and the only one that declares other. The
// probe names come from the fixture model, as they do from rminfo for the
// registry.
type (
	fixtureParent struct{}
	fixtureOther  struct{}
)

func fixtureModel(t *testing.T) rmModel {
	t.Helper()
	return modelOf(t, rminfo.New(map[string]rminfo.ClassMeta{
		"FIX_PARENT": {
			Attributes: map[string]rminfo.AttrMeta{
				"name":  {TypeName: "DV_TEXT", Required: true, DeclaredIn: "LOCATABLE"},
				"uid":   {TypeName: "UID_BASED_ID", DeclaredIn: "LOCATABLE"},
				"data":  {TypeName: "ITEM_STRUCTURE", DeclaredIn: "FIX_PARENT"},
				"items": {TypeName: "ITEM", Container: true, DeclaredIn: "FIX_PARENT"},
				"notes": {TypeName: "DV_TEXT", DeclaredIn: "FIX_PARENT"},
				"extra": {TypeName: "DV_TEXT", DeclaredIn: "FIX_PARENT"},
			},
			AttrOrder: []string{"name", "uid", "data", "items", "notes", "extra"},
		},
		"FIX_OTHER": {
			Attributes: map[string]rminfo.AttrMeta{
				"name":  {TypeName: "DV_TEXT", Required: true, DeclaredIn: "LOCATABLE"},
				"other": {TypeName: "DV_TEXT", DeclaredIn: "FIX_OTHER"},
			},
			AttrOrder: []string{"name", "other"},
		},
	}))
}

func fixtureParents() map[string]func() any {
	return map[string]func() any{
		"FIX_PARENT": func() any { return &fixtureParent{} },
		"FIX_OTHER":  func() any { return &fixtureOther{} },
	}
}

// fixtureSingle has a correct case for data, a case for the list attribute
// items, a case for other (which only FIX_OTHER declares), and a case for
// notes that stores any child.
func fixtureSingle(parent any, attr string, child any) error {
	if _, ok := parent.(*fixtureParent); ok {
		switch attr {
		case "data", "items", "other":
			return mismatch(attr, child, "ITEM_STRUCTURE")
		case "notes":
			return nil
		}
	}
	return fmt.Errorf("%w: parent %T, attr %q", ErrUnknownAttribute, parent, attr)
}

// fixtureMultiple has a correct case for items and a case for the
// single-valued data.
func fixtureMultiple(parent any, attr string, child any) error {
	if _, ok := parent.(*fixtureParent); ok {
		switch attr {
		case "items", "data":
			return mismatch(attr, child, "ITEM")
		}
	}
	return fmt.Errorf("%w: parent %T, attr %q", ErrUnknownAttribute, parent, attr)
}

// --- the tests ------------------------------------------------------------

// TestREQ107_WriterArmsDeclaredByRM checks that every case of EnsureSingle
// and AppendMultiple names an attribute the parent class declares in
// openehr/rm/rminfo, in the shape IsContainer gives it: EnsureSingle for a
// single-valued attribute, AppendMultiple for a list. Each class the type
// registry knows is asked about every attribute name the RM declares on any
// class. The registry holds the bare DV_INTERVAL only; the typed
// instantiations share its writer, and TestIntervalWriterParity covers them.
func TestREQ107_WriterArmsDeclaredByRM(t *testing.T) {
	t.Parallel()

	t.Run("fixture control", func(t *testing.T) {
		t.Parallel()
		model := fixtureModel(t)
		arms := probeArms(fixtureParents(), probeAttrNames(model), fixtureSingle, fixtureMultiple)
		got := armViolations(arms, model)
		want := []string{
			"FIX_PARENT.data: AppendMultiple has a case for it, but it is single-valued",
			"FIX_PARENT.items: EnsureSingle has a case for it, but it is a list attribute",
			"FIX_PARENT.notes: a writer stored a child no RM attribute holds",
			"FIX_PARENT.other: a writer has a case for it, but the class does not declare it",
		}
		if !slices.Equal(got, want) {
			t.Errorf("armViolations(fixture) =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
		}
	})

	t.Run("registry", func(t *testing.T) {
		t.Parallel()
		model := modelOf(t, rminfo.Default)
		if got := armViolations(registryArms(t, model), model); len(got) > 0 {
			t.Errorf("rmwrite writer cases the RM metadata does not back:\n  %s\n"+
				"A writer case must name an attribute its class declares, in EnsureSingle for a "+
				"single-valued attribute and in AppendMultiple for a list. Fix the case in write.go, "+
				"or regenerate openehr/rm/rminfo if the BMM changed.",
				strings.Join(got, "\n  "))
		}
	})
}

// pinnedUnwritten lists, for each class rmwrite writes into, the declared
// attributes neither writer has a case for (LOCATABLE attributes other than
// name are left out; unwrittenAttributes says why). It is the measured
// residual, not a judgement that each one is right to be missing: an
// attribute here that the generator must fill is a gap in rmwrite. A class
// with nothing unwritten keeps an empty entry, so a parent dropped from the
// writers fails too. Classes rmwrite never writes into are not listed: an
// OPT that constrains one of their attributes already fails the write with
// ErrUnknownAttribute.
var pinnedUnwritten = map[string][]string{
	"ACTION":         {"guideline_id", "instruction_details", "other_participations", "workflow_id"},
	"ACTIVITY":       {},
	"ADDRESS":        {},
	"ADMIN_ENTRY":    {"other_participations", "workflow_id"},
	"AGENT":          {"languages", "roles"},
	"CAPABILITY":     {},
	"CLUSTER":        {},
	"CODE_PHRASE":    {},
	"COMPOSITION":    {},
	"CONTACT":        {"name", "time_validity"},
	"DV_BOOLEAN":     {},
	"DV_CODED_TEXT":  {"encoding", "formatting", "hyperlink", "language", "mappings"},
	"DV_COUNT":       {"accuracy", "accuracy_is_percent", "magnitude_status", "other_reference_ranges"},
	"DV_DATE":        {"accuracy", "magnitude_status", "normal_range", "normal_status", "other_reference_ranges"},
	"DV_DATE_TIME":   {"accuracy", "magnitude_status", "normal_range", "normal_status", "other_reference_ranges"},
	"DV_DURATION":    {"accuracy", "accuracy_is_percent", "magnitude_status", "normal_range", "normal_status", "other_reference_ranges"},
	"DV_EHR_URI":     {},
	"DV_IDENTIFIER":  {},
	"DV_INTERVAL":    {},
	"DV_MULTIMEDIA":  {"data", "integrity_check", "thumbnail", "uri"},
	"DV_PARSABLE":    {},
	"DV_QUANTITY":    {"accuracy", "accuracy_is_percent", "magnitude_status", "other_reference_ranges", "precision", "units_display_name", "units_system"},
	"DV_TEXT":        {"encoding", "formatting", "hyperlink", "language", "mappings"},
	"DV_TIME":        {"accuracy", "magnitude_status", "normal_range", "normal_status", "other_reference_ranges"},
	"DV_URI":         {},
	"ELEMENT":        {},
	"EVALUATION":     {"guideline_id", "other_participations", "workflow_id"},
	"EVENT_CONTEXT":  {"participations"},
	"GENERIC_ENTRY":  {},
	"GROUP":          {"languages", "roles"},
	"HISTORY":        {},
	"INSTRUCTION":    {"guideline_id", "other_participations", "workflow_id"},
	"INTERVAL_EVENT": {},
	"ISM_TRANSITION": {"reason", "transition"},
	"ITEM_LIST":      {},
	"ITEM_SINGLE":    {},
	"ITEM_TABLE":     {},
	"ITEM_TREE":      {},
	"OBSERVATION":    {"guideline_id", "other_participations", "workflow_id"},
	"ORGANISATION":   {"languages", "roles"},
	"PARTY_IDENTITY": {},
	"PARTY_RELATED":  {"external_ref", "identifiers", "name"},
	"PERSON":         {"languages", "roles"},
	"POINT_EVENT":    {},
	"ROLE":           {},
	"SECTION":        {},
}

// TestREQ107_WriterParentAttributeParity checks that the declared attributes
// rmwrite leaves unwritten, on each class it writes into, are exactly
// pinnedUnwritten. A BMM bump that adds an attribute, or a writer case that is
// removed or renamed, changes the set and fails here.
func TestREQ107_WriterParentAttributeParity(t *testing.T) {
	t.Parallel()

	t.Run("fixture control", func(t *testing.T) {
		t.Parallel()
		model := fixtureModel(t)
		got := unwrittenAttributes(probeArms(fixtureParents(), probeAttrNames(model), fixtureSingle, fixtureMultiple), model)
		want := map[string][]string{"FIX_PARENT": {"extra", "name"}}
		if drift := residualDrift(got, want); len(drift) > 0 {
			t.Errorf("unwrittenAttributes(fixture) drifted from %v:\n  %s", want, strings.Join(drift, "\n  "))
		}
		stale := map[string][]string{"FIX_PARENT": {"name"}, "FIX_OTHER": {"other"}}
		wantDrift := []string{
			"FIX_OTHER: pinned, but no writer writes into it any more",
			"FIX_PARENT: newly unwritten [extra], newly written []",
		}
		if drift := residualDrift(got, stale); !slices.Equal(drift, wantDrift) {
			t.Errorf("residualDrift(fixture, stale) =\n  %s\nwant\n  %s", strings.Join(drift, "\n  "), strings.Join(wantDrift, "\n  "))
		}
		// A class the writers fill that the pin leaves out gets its own line,
		// not a diff against an empty pin.
		wantUnpinned := []string{"FIX_PARENT: a writer writes into it, but it is not pinned (unwritten: [extra name])"}
		if drift := residualDrift(got, map[string][]string{}); !slices.Equal(drift, wantUnpinned) {
			t.Errorf("residualDrift(fixture, empty pin) =\n  %s\nwant\n  %s", strings.Join(drift, "\n  "), strings.Join(wantUnpinned, "\n  "))
		}
	})

	t.Run("registry", func(t *testing.T) {
		t.Parallel()
		model := modelOf(t, rminfo.Default)
		got := unwrittenAttributes(registryArms(t, model), model)
		if drift := residualDrift(got, pinnedUnwritten); len(drift) > 0 {
			t.Errorf("rmwrite and the RM metadata drifted apart:\n  %s\n"+
				"A new BMM attribute on a class rmwrite writes into either gets a writer case in "+
				"write.go, or is added to pinnedUnwritten on purpose. A writer case that was removed "+
				"or renamed makes the instance generator drop that attribute without an error: restore it.",
				strings.Join(drift, "\n  "))
		}
	})
}

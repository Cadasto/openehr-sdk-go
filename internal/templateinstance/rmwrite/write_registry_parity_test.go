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

// The tests below tie the hand-written writers to the RM metadata. The
// instance generator fills BMM-required defaults through EnsureSingle and
// AppendMultiple and drops their error, so a writer case that goes missing
// drops a required default without an error. No test keeps a hand list of
// writer cases: they ask the writers about every pair of a registered class
// and an attribute name, and compare the answers with openehr/rm/rminfo. The
// reachability test also walks rminfo from every concrete LOCATABLE, so it
// sees the classes a value can be that no writer has a case for at all.

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

// skippedLocatableAttrs are the LOCATABLE attributes reachableClasses does
// not follow. The instance generator stamps uid and archetype_details itself,
// and nothing fills links or feeder_audit, so no writer is asked to build
// what they hold.
var skippedLocatableAttrs = []string{"archetype_details", "feeder_audit", "links", "uid"}

// reachableClasses returns, sorted, every registered class a value below a
// LOCATABLE can be. It is the closure over model that starts at every
// concrete LOCATABLE and follows the declared type of each attribute, an
// abstract type standing for its concrete descendants. It does not follow
// the LOCATABLE attributes skippedLocatableAttrs names; an attribute of the
// same name a class declares itself is followed. It does not walk through a
// class missing from registered, since nothing can build a value of it. A
// primitive or a generic parameter (String, T) is not a class of model and
// ends the walk, so a class used only as a type argument, such as the T of a
// DV_INTERVAL<T>, would not be reached.
func reachableClasses(model rmModel, registered map[string]bool) []string {
	seen := map[string]bool{}
	var queue []string
	reach := func(rmType string) {
		concrete, _ := model.ConcreteDescendants(rmType)
		for _, class := range concrete {
			if registered[class] && !seen[class] {
				seen[class] = true
				queue = append(queue, class)
			}
		}
	}
	reach("LOCATABLE")
	for len(queue) > 0 {
		class := queue[0]
		queue = queue[1:]
		for _, attr := range model.AttributeNames(class) {
			if site, _ := model.DeclaredOn(class, attr); site == "LOCATABLE" && slices.Contains(skippedLocatableAttrs, attr) {
				continue
			}
			if rmType, ok := model.AttributeRMType(class, attr); ok {
				reach(rmType)
			}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// unreachedDrift compares the reachable classes neither writer has a case
// for with the pinned ones, and lists each class that differs, sorted: a
// reachable class with no writer case that is not pinned, a pinned class a
// writer now has a case for, and a pinned class the closure no longer
// reaches, because no concrete LOCATABLE reaches it or the type registry
// does not register it.
func unreachedDrift(reachable []string, arms armTable, pinned map[string]string) []string {
	classes := slices.Clone(reachable)
	for class := range pinned {
		if !slices.Contains(reachable, class) {
			classes = append(classes, class)
		}
	}
	slices.Sort(classes)

	var out []string
	for _, class := range classes {
		_, isPinned := pinned[class]
		_, written := arms[class]
		isReachable := slices.Contains(reachable, class)
		switch {
		case isReachable && !written && !isPinned:
			out = append(out, class+": reachable and no writer has a case for it, but it is not pinned")
		case isPinned && !isReachable:
			out = append(out, class+": pinned, but no concrete LOCATABLE reaches it, or the type registry does not register it")
		case isPinned && written:
			out = append(out, class+": pinned, but a writer now has a case for it")
		}
	}
	return out
}

// registrySet returns the names of the classes the type registry knows.
func registrySet() map[string]bool {
	out := map[string]bool{}
	for name := range registryParents() {
		out[name] = true
	}
	return out
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

// The reachability fixture is a small LOCATABLE tree, built fresh on each
// call so a control can change it. FIX_ROOT is the one concrete LOCATABLE.
// Its value names the abstract FIX_VALUE, which stands for FIX_VALUE_A and
// FIX_VALUE_B. Its ref leads to FIX_REF, which is no LOCATABLE and declares
// a links of its own that leads to FIX_LINKED. LOCATABLE's uid and links
// lead to FIX_HIDDEN, which nothing else holds. FIX_ROOT's other leads to
// FIX_UNREGISTERED, which the fixture registry leaves out, and FIX_BELOW is
// reachable only through that class. FIX_ROOT's label is a String.
func reachFixtureData() map[string]rminfo.ClassMeta {
	inherited := map[string]rminfo.AttrMeta{
		"name":  {TypeName: "DV_TEXT", Required: true, DeclaredIn: "LOCATABLE"},
		"uid":   {TypeName: "FIX_HIDDEN", DeclaredIn: "LOCATABLE"},
		"links": {TypeName: "FIX_HIDDEN", Container: true, DeclaredIn: "LOCATABLE"},
	}
	root := maps.Clone(inherited)
	maps.Copy(root, map[string]rminfo.AttrMeta{
		"value": {TypeName: "FIX_VALUE", DeclaredIn: "FIX_ROOT"},
		"ref":   {TypeName: "FIX_REF", DeclaredIn: "FIX_ROOT"},
		"other": {TypeName: "FIX_UNREGISTERED", DeclaredIn: "FIX_ROOT"},
		"label": {TypeName: "String", DeclaredIn: "FIX_ROOT"},
	})
	magnitude := func(class string) rminfo.ClassMeta {
		return rminfo.ClassMeta{
			Attributes: map[string]rminfo.AttrMeta{"magnitude": {TypeName: "Real", Required: true, DeclaredIn: class}},
			AttrOrder:  []string{"magnitude"},
			Parents:    []string{"FIX_VALUE"},
		}
	}
	return map[string]rminfo.ClassMeta{
		"LOCATABLE": {Attributes: inherited, AttrOrder: []string{"name", "uid", "links"}, Abstract: true},
		"FIX_ROOT": {
			Attributes: root,
			AttrOrder:  []string{"name", "uid", "links", "value", "ref", "other", "label"},
			Parents:    []string{"LOCATABLE"},
		},
		"DV_TEXT": {
			Attributes: map[string]rminfo.AttrMeta{"value": {TypeName: "String", Required: true, DeclaredIn: "DV_TEXT"}},
			AttrOrder:  []string{"value"},
		},
		"FIX_VALUE":   {Abstract: true},
		"FIX_VALUE_A": magnitude("FIX_VALUE_A"),
		"FIX_VALUE_B": magnitude("FIX_VALUE_B"),
		"FIX_REF": {
			Attributes: map[string]rminfo.AttrMeta{"links": {TypeName: "FIX_LINKED", Container: true, DeclaredIn: "FIX_REF"}},
			AttrOrder:  []string{"links"},
		},
		"FIX_LINKED": {},
		"FIX_HIDDEN": {},
		"FIX_UNREGISTERED": {
			Attributes: map[string]rminfo.AttrMeta{"below": {TypeName: "FIX_BELOW", DeclaredIn: "FIX_UNREGISTERED"}},
			AttrOrder:  []string{"below"},
		},
		"FIX_BELOW": {},
	}
}

// reachFixtureRegistry holds every concrete fixture class but
// FIX_UNREGISTERED.
func reachFixtureRegistry() map[string]bool {
	return map[string]bool{
		"DV_TEXT": true, "FIX_BELOW": true, "FIX_HIDDEN": true, "FIX_LINKED": true,
		"FIX_REF": true, "FIX_ROOT": true, "FIX_VALUE_A": true, "FIX_VALUE_B": true,
	}
}

// reachFixtureArms is a synthetic arm table with cases for DV_TEXT, FIX_ROOT
// and FIX_VALUE_A; no writer is probed.
func reachFixtureArms() armTable {
	return armTable{
		"DV_TEXT":     {"value": {single: true}},
		"FIX_ROOT":    {"name": {single: true}, "value": {single: true}},
		"FIX_VALUE_A": {"magnitude": {single: true}},
	}
}

// reachFixturePin is the fixture's unreached residual.
func reachFixturePin() map[string]string {
	return map[string]string{
		"FIX_LINKED":  "",
		"FIX_REF":     "",
		"FIX_VALUE_B": "",
	}
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
	"ACTION":                        {"guideline_id", "instruction_details", "other_participations", "workflow_id"},
	"ACTIVITY":                      {},
	"ADDRESS":                       {},
	"ADMIN_ENTRY":                   {"other_participations", "workflow_id"},
	"AGENT":                         {"languages", "roles"},
	"CAPABILITY":                    {},
	"CLUSTER":                       {},
	"CODE_PHRASE":                   {},
	"COMPOSITION":                   {},
	"CONTACT":                       {"name", "time_validity"},
	"DV_BOOLEAN":                    {},
	"DV_CODED_TEXT":                 {"mappings"},
	"DV_COUNT":                      {"other_reference_ranges"},
	"DV_DATE":                       {"normal_range", "normal_status", "other_reference_ranges"},
	"DV_DATE_TIME":                  {"normal_range", "normal_status", "other_reference_ranges"},
	"DV_DURATION":                   {"normal_range", "normal_status", "other_reference_ranges"},
	"DV_EHR_URI":                    {},
	"DV_GENERAL_TIME_SPECIFICATION": {},
	"DV_IDENTIFIER":                 {},
	"DV_INTERVAL":                   {},
	// data and integrity_check are arrays of octets, which no AOM 1.4
	// constraint describes, so no OPT puts a value there to write; the
	// instance generator writes the example data itself.
	"DV_MULTIMEDIA":                  {"data", "integrity_check"},
	"DV_PARAGRAPH":                   {},
	"DV_PARSABLE":                    {},
	"DV_PERIODIC_TIME_SPECIFICATION": {},
	"DV_QUANTITY":                    {"other_reference_ranges"},
	"DV_STATE":                       {},
	"DV_TEXT":                        {"mappings"},
	"DV_TIME":                        {"normal_range", "normal_status", "other_reference_ranges"},
	"DV_URI":                         {},
	"ELEMENT":                        {},
	"EVALUATION":                     {"guideline_id", "other_participations", "workflow_id"},
	"EVENT_CONTEXT":                  {"participations"},
	"GENERIC_ENTRY":                  {},
	"GROUP":                          {"languages", "roles"},
	"HISTORY":                        {},
	"INSTRUCTION":                    {"guideline_id", "other_participations", "workflow_id"},
	"INTERVAL_EVENT":                 {},
	"ISM_TRANSITION":                 {"reason", "transition"},
	"ITEM_LIST":                      {},
	"ITEM_SINGLE":                    {},
	"ITEM_TABLE":                     {},
	"ITEM_TREE":                      {},
	"OBSERVATION":                    {"guideline_id", "other_participations", "workflow_id"},
	"ORGANISATION":                   {"languages", "roles"},
	"PARTY_IDENTITY":                 {},
	"PARTY_RELATED":                  {"external_ref", "identifiers", "name"},
	"PERSON":                         {"languages", "roles"},
	"POINT_EVENT":                    {},
	"ROLE":                           {},
	"SECTION":                        {},
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

// pinnedUnreached lists the registered classes a value below a LOCATABLE can
// be that neither writer has a case for (reachableClasses says how the set is
// found). It is the measured residual, not a judgement: rmwrite has no case to
// write any attribute of an instance of a class here, so where an OPT
// constrains one of its attributes the instance generator cannot write what
// the OPT constrains. The value is the reason a class stays, and is empty for
// a gap a writer is meant to close. Classes reached only through LOCATABLE's
// links, feeder_audit, archetype_details or uid (LINK, FEEDER_AUDIT,
// ARCHETYPED and what only they hold) are not listed, because the closure
// does not follow those attributes.
var pinnedUnreached = map[string]string{
	"ACCESS_GROUP_REF":    "",
	"ARCHETYPE_ID":        "",
	"DV_ORDINAL":          "",
	"DV_PROPORTION":       "",
	"DV_SCALE":            "",
	"EHR_ACCESS":          "settings, its one attribute beside LOCATABLE's, is ACCESS_CONTROL_SETTINGS, which no concrete class extends in the pinned RM",
	"EHR_STATUS":          "",
	"FOLDER":              "",
	"GENERIC_ID":          "",
	"HIER_OBJECT_ID":      "",
	"INSTRUCTION_DETAILS": "",
	"LOCATABLE_REF":       "",
	"OBJECT_REF":          "",
	"OBJECT_VERSION_ID":   "",
	"PARTICIPATION":       "",
	"PARTY_IDENTIFIED":    "",
	"PARTY_REF":           "",
	"PARTY_RELATIONSHIP":  "",
	"PARTY_SELF":          "",
	"REFERENCE_RANGE":     "",
	"TEMPLATE_ID":         "",
	"TERMINOLOGY_ID":      "",
	"TERM_MAPPING":        "",
}

// TestREQ107_WriterReachableClassParity checks that the classes a value below
// a LOCATABLE can be, which neither writer has a case for, are exactly
// pinnedUnreached. TestREQ107_WriterParentAttributeParity sees only classes
// with at least one writer case; this test sees the rest. A BMM bump that
// makes a class reachable, a writer case added for a pinned class, or a
// class's last writer case removed changes the set and fails here.
func TestREQ107_WriterReachableClassParity(t *testing.T) {
	t.Parallel()

	t.Run("fixture control", func(t *testing.T) {
		t.Parallel()
		reachable := reachableClasses(modelOf(t, rminfo.New(reachFixtureData())), reachFixtureRegistry())
		wantReachable := []string{"DV_TEXT", "FIX_LINKED", "FIX_REF", "FIX_ROOT", "FIX_VALUE_A", "FIX_VALUE_B"}
		if !slices.Equal(reachable, wantReachable) {
			t.Fatalf("reachableClasses(fixture) = %v, want %v", reachable, wantReachable)
		}
		if drift := unreachedDrift(reachable, reachFixtureArms(), reachFixturePin()); len(drift) > 0 {
			t.Errorf("unreachedDrift(fixture) drifted from %v:\n  %s", reachFixturePin(), strings.Join(drift, "\n  "))
		}

		// Each case changes the fixture model, registry or arm table once,
		// and the unchanged pin then drifts by exactly the classes it touches.
		addRootAttr := func(data map[string]rminfo.ClassMeta, attr, rmType string) {
			root := data["FIX_ROOT"]
			root.Attributes[attr] = rminfo.AttrMeta{TypeName: rmType, DeclaredIn: "FIX_ROOT"}
			root.AttrOrder = append(root.AttrOrder, attr)
			data["FIX_ROOT"] = root
		}
		changes := []struct {
			name     string
			model    func(map[string]rminfo.ClassMeta)
			registry func(map[string]bool)
			arms     func(armTable)
			want     []string
		}{
			{
				name: "class added under a new attribute",
				model: func(data map[string]rminfo.ClassMeta) {
					data["FIX_NEW"] = rminfo.ClassMeta{}
					addRootAttr(data, "extra", "FIX_NEW")
				},
				registry: func(reg map[string]bool) { reg["FIX_NEW"] = true },
				want:     []string{"FIX_NEW: reachable and no writer has a case for it, but it is not pinned"},
			},
			{
				name: "concrete descendant added to an abstract attribute type",
				model: func(data map[string]rminfo.ClassMeta) {
					data["FIX_VALUE_C"] = rminfo.ClassMeta{Parents: []string{"FIX_VALUE"}}
				},
				registry: func(reg map[string]bool) { reg["FIX_VALUE_C"] = true },
				want:     []string{"FIX_VALUE_C: reachable and no writer has a case for it, but it is not pinned"},
			},
			{
				name: "attribute removed",
				model: func(data map[string]rminfo.ClassMeta) {
					root := data["FIX_ROOT"]
					delete(root.Attributes, "value")
					root.AttrOrder = slices.DeleteFunc(root.AttrOrder, func(a string) bool { return a == "value" })
					data["FIX_ROOT"] = root
				},
				want: []string{"FIX_VALUE_B: pinned, but no concrete LOCATABLE reaches it, or the type registry does not register it"},
			},
			{
				name:     "class dropped from the registry",
				registry: func(reg map[string]bool) { delete(reg, "FIX_REF") },
				want: []string{
					"FIX_LINKED: pinned, but no concrete LOCATABLE reaches it, or the type registry does not register it",
					"FIX_REF: pinned, but no concrete LOCATABLE reaches it, or the type registry does not register it",
				},
			},
			{
				name: "writer case added",
				arms: func(arms armTable) { arms["FIX_VALUE_B"] = map[string]armShape{"magnitude": {single: true}} },
				want: []string{"FIX_VALUE_B: pinned, but a writer now has a case for it"},
			},
			{
				name: "writer case removed",
				arms: func(arms armTable) { delete(arms, "FIX_VALUE_A") },
				want: []string{"FIX_VALUE_A: reachable and no writer has a case for it, but it is not pinned"},
			},
		}
		for _, tc := range changes {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				data, registry, arms := reachFixtureData(), reachFixtureRegistry(), reachFixtureArms()
				if tc.model != nil {
					tc.model(data)
				}
				if tc.registry != nil {
					tc.registry(registry)
				}
				if tc.arms != nil {
					tc.arms(arms)
				}
				reachable := reachableClasses(modelOf(t, rminfo.New(data)), registry)
				if got := unreachedDrift(reachable, arms, reachFixturePin()); !slices.Equal(got, tc.want) {
					t.Errorf("unreachedDrift(changed fixture) =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(tc.want, "\n  "))
				}
			})
		}
	})

	t.Run("registry", func(t *testing.T) {
		t.Parallel()
		model := modelOf(t, rminfo.Default)
		reachable := reachableClasses(model, registrySet())
		if drift := unreachedDrift(reachable, registryArms(t, model), pinnedUnreached); len(drift) > 0 {
			t.Errorf("the reachable RM classes rmwrite has no writer case for drifted from pinnedUnreached:\n  %s\n"+
				"A pinned class that now has a writer case leaves pinnedUnreached, and its unwritten "+
				"attributes go into pinnedUnwritten. A pinned class no concrete LOCATABLE reaches any "+
				"more, or that the type registry no longer registers, leaves pinnedUnreached too. A class "+
				"that became reachable either gets writer cases in write.go, or is added to "+
				"pinnedUnreached on purpose. A class whose last writer case was removed makes the "+
				"instance generator fail on it: restore the case.",
				strings.Join(drift, "\n  "))
		}
	})
}

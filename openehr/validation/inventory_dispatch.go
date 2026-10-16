package validation

// inventory_dispatch.go: REQ-171 *Where the RM rules come from*. The RM rules
// the walk applies are the evaluated entries of the invariant inventory
// together with the mandatory-presence rule. The inventory (inventory_gen.go)
// is generated from the pinned RM and BASE BMMs and classified in
// invariants.json; this file binds each evaluated entry to the Go function
// that evaluates it, or, for an entry another evaluated check already
// decides, to that check's function, and builds from both, once, what the
// walk runs on a node of each class the type registry decodes and which of
// the invariants that class is subject to the walk does not evaluate.

import (
	"maps"
	"math/bits"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// invariantKind is how invariants.json classifies an invariant of the
// inventory.
type invariantKind uint8

const (
	// invariantEvaluated is an invariant the RM rules evaluate, with the Go
	// function that evaluates it or the check that already decides it.
	invariantEvaluated invariantKind = iota + 1
	// invariantNotMechanical is an invariant no check of the instance alone
	// can decide, because it needs a record the instance does not carry: in
	// the pinned BMMs, the versions stored for a versioned object, or the
	// other party of a demographic relationship.
	invariantNotMechanical
	// invariantOutOfScope is an invariant the SDK does not check on an
	// instance, for the reason the classification gives: most are not
	// evaluated yet; others hold by definition on the SDK's types, are
	// declared on a class no registered type is or descends from, or are
	// read otherwise by a documented deviation of the REQ-123 ISO 8601
	// parse.
	invariantOutOfScope
)

// inventoryEntry is the classification of one invariant: its kind and, for
// an evaluated one, the name of the Go function that evaluates it.
type inventoryEntry struct {
	kind     invariantKind
	function string
}

// floorNode is the node an evaluator runs on, as the walk sees it.
type floorNode struct {
	value  any
	rmType string
	path   string
}

// invariantCheck binds an evaluated invariant to the evaluator that
// evaluates it, or to the check that already decides it.
type invariantCheck struct {
	// function is the evaluator's name, as invariants.json names it. The
	// evaluator serves the node once, however many of the node's invariants
	// it is bound to, unless perRule is set.
	function string
	// run calls the evaluator on a node; rule is the invariant that
	// dispatched it.
	run func(into *findings, n floorNode, rule string)
	// perRule is set for an evaluator that reports the invariant that
	// dispatched it, so it serves each of them.
	perRule bool
	// coded is set, in place of run, for a coded invariant, which
	// [findings.checkCodedInvariants] evaluates from its row.
	coded *codedRule
	// covered is set, in place of run and coded, for an invariant that the
	// check function names already decides: the binding dispatches nothing
	// of its own, and the invariant is evaluated on a class only where that
	// check runs.
	covered bool
}

// mandatoryCheck is the function that applies the mandatory-presence rule,
// [findings.checkMandatory]. The walk runs it on the RM-mandatory
// attributes of every node it visits, so [inventorySource.build] counts it as
// run on every class.
const mandatoryCheck = "checkMandatory"

// nodeCheck binds an evaluator that reads the node's value and path.
func nodeCheck(function string, f func(*findings, any, string)) invariantCheck {
	return invariantCheck{function: function, run: func(into *findings, n floorNode, _ string) {
		f(into, n.value, n.path)
	}}
}

// typeCheck binds an evaluator that also reads the node's RM type, to name
// the class it reports on.
func typeCheck(function string, f func(*findings, any, string, string)) invariantCheck {
	return invariantCheck{function: function, run: func(into *findings, n floorNode, _ string) {
		f(into, n.value, n.rmType, n.path)
	}}
}

// ruleCheck binds an evaluator that reads the node's RM type and reports
// the invariant that dispatched it: one Go function serves an invariant of
// several classes (Value_valid), or one invariant declared on an ancestor of
// several (Is_archetype_root), and the rule it reports is always the
// inventory's key.
func ruleCheck(function string, f func(*findings, any, string, string, string)) invariantCheck {
	return invariantCheck{function: function, perRule: true, run: func(into *findings, n floorNode, rule string) {
		f(into, n.value, n.rmType, n.path, rule)
	}}
}

// codedCheck binds a coded invariant to its row of the coded table
// (rmfloor_coded.go).
func codedCheck(row *codedRule) invariantCheck {
	return invariantCheck{function: "checkCodedInvariants", coded: row}
}

// coveredCheck binds an invariant that another evaluated check already
// decides to that check's function, handed in by value beside its name so
// that a renamed or removed function fails to build. It dispatches nothing:
// the check runs on the node already, the mandatory-presence check on every
// node the walk visits and any other where an invariant of the node's class
// dispatches it, and a finding carries the rule of the check that reports it
// (REQ-171 Finding identity).
func coveredCheck(function string, _ any) invariantCheck {
	return invariantCheck{function: function, covered: true}
}

// invariantChecks binds every evaluated invariant of the inventory to the Go
// function that evaluates it, by value, so a renamed or removed evaluator
// fails to build. The names recorded beside the functions are checked against
// the functions and against invariants.json by a test, which also fails when
// an evaluated invariant is missing here or one listed here is not evaluated.
// The last three groups bind the invariants another evaluated check decides:
// the mandatory-presence rule, for a mandatory string or list the invariant
// says is not empty; the REQ-123 parse a temporal Value_valid applies, for
// the parts of the ISO 8601 value it refuses; and the DV_PROPORTION
// precision check, for Is_integral_validity, whose is_integral the RM reads
// as a precision of 0, under which that check reports an operand that is not
// whole as DV_PROPORTION.Precision_validity.
var invariantChecks = map[string]invariantCheck{
	"ARCHETYPED.Rm_version_valid":        nodeCheck("checkArchetyped", (*findings).checkArchetyped),
	"COMPOSITION.Is_archetype_root":      ruleCheck("checkArchetypeRoot", (*findings).checkArchetypeRoot),
	"DV_DATE.Value_valid":                ruleCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"DV_DATE_TIME.Value_valid":           ruleCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"DV_DURATION.Value_valid":            ruleCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"DV_INTERVAL.Limits_consistent":      typeCheck("checkDVInterval", (*findings).checkDVInterval),
	"DV_PROPORTION.Precision_validity":   nodeCheck("checkDVProportion", (*findings).checkDVProportion),
	"DV_TEXT.Mappings_valid":             nodeCheck("checkTermMappings", (*findings).checkTermMappings),
	"DV_TIME.Value_valid":                ruleCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"EHR_ACCESS.Is_archetype_root":       ruleCheck("checkArchetypeRoot", (*findings).checkArchetypeRoot),
	"EHR_STATUS.Is_archetype_root":       ruleCheck("checkArchetypeRoot", (*findings).checkArchetypeRoot),
	"ELEMENT.Inv_null_flavour_indicated": nodeCheck("checkElementNullFlavour", (*findings).checkElementNullFlavour),
	"ENTRY.Is_archetype_root":            ruleCheck("checkArchetypeRoot", (*findings).checkArchetypeRoot),
	"Interval.Lower_included_valid":      typeCheck("checkDVInterval", (*findings).checkDVInterval),
	"Interval.Upper_included_valid":      typeCheck("checkDVInterval", (*findings).checkDVInterval),
	"PARTY.Is_archetype_root":            ruleCheck("checkArchetypeRoot", (*findings).checkArchetypeRoot),
	"PARTY_IDENTIFIED.Basic_validity":    nodeCheck("checkPartyIdentified", (*findings).checkPartyIdentified),
	"PARTY_IDENTIFIED.Identifiers_valid": nodeCheck("checkPartyIdentified", (*findings).checkPartyIdentified),
	"PARTY_IDENTIFIED.Name_valid":        nodeCheck("checkPartyIdentified", (*findings).checkPartyIdentified),
	"TERM_MAPPING.Match_valid":           nodeCheck("checkTermMapping", (*findings).checkTermMapping),

	"COMPOSITION.Category_validity":                    codedCheck(&ruleCompositionCategory),
	"COMPOSITION.Language_valid":                       codedCheck(&ruleCompositionLanguage),
	"COMPOSITION.Territory_valid":                      codedCheck(&ruleCompositionTerritory),
	"DV_ENCAPSULATED.Charset_valid":                    codedCheck(&ruleEncapsulatedCharset),
	"DV_ENCAPSULATED.Language_valid":                   codedCheck(&ruleEncapsulatedLanguage),
	"DV_MULTIMEDIA.Compression_algorithm_validity":     codedCheck(&ruleCompression),
	"DV_MULTIMEDIA.Integrity_check_algorithm_validity": codedCheck(&ruleIntegrityCheck),
	"DV_MULTIMEDIA.Media_type_valid":                   codedCheck(&ruleMediaType),
	"DV_ORDERED.Normal_status_validity":                codedCheck(&ruleNormalStatus),
	"DV_TEXT.Encoding_valid":                           codedCheck(&ruleTextEncoding),
	"DV_TEXT.Language_valid":                           codedCheck(&ruleTextLanguage),
	"ELEMENT.Inv_null_flavour_valid":                   codedCheck(&ruleElementNullFlavour),
	"ENTRY.Encoding_valid":                             codedCheck(&ruleEntryEncoding),
	"ENTRY.Language_valid":                             codedCheck(&ruleEntryLanguage),
	"EVENT_CONTEXT.Setting_valid":                      codedCheck(&ruleEventContextSetting),
	"INTERVAL_EVENT.Math_function_validity":            codedCheck(&ruleEventMathFunction),
	"ISM_TRANSITION.Current_state_valid":               codedCheck(&ruleISMCurrentState),
	"ISM_TRANSITION.Transition_valid":                  codedCheck(&ruleISMTransition),
	"PARTICIPATION.Function_valid":                     codedCheck(&ruleParticipationFunc),
	"PARTICIPATION.Mode_valid":                         codedCheck(&ruleParticipationMode),
	"PARTY_RELATED.Relationship_valid":                 codedCheck(&rulePartyRelationship),
	"TERM_MAPPING.Purpose_valid":                       codedCheck(&ruleTermMappingPurpose),

	"ACTIVITY.Action_archetype_id_valid":         coveredCheck("checkMandatory", (*findings).checkMandatory),
	"AUDIT_DETAILS.System_id_valid":              coveredCheck("checkMandatory", (*findings).checkMandatory),
	"CODE_PHRASE.Code_string_valid":              coveredCheck("checkMandatory", (*findings).checkMandatory),
	"DV_IDENTIFIER.Id_valid":                     coveredCheck("checkMandatory", (*findings).checkMandatory),
	"DV_PARAGRAPH.Items_valid":                   coveredCheck("checkMandatory", (*findings).checkMandatory),
	"DV_PARSABLE.Formalism_valid":                coveredCheck("checkMandatory", (*findings).checkMandatory),
	"DV_TEXT.Valid_value":                        coveredCheck("checkMandatory", (*findings).checkMandatory),
	"DV_URI.Value_valid":                         coveredCheck("checkMandatory", (*findings).checkMandatory),
	"FEEDER_AUDIT_DETAILS.System_id_valid":       coveredCheck("checkMandatory", (*findings).checkMandatory),
	"INSTRUCTION_DETAILS.Activity_path_valid":    coveredCheck("checkMandatory", (*findings).checkMandatory),
	"LOCATABLE.Archetype_node_id_valid":          coveredCheck("checkMandatory", (*findings).checkMandatory),
	"PARTY.Identities_valid":                     coveredCheck("checkMandatory", (*findings).checkMandatory),
	"RESOURCE_DESCRIPTION.Lifecycle_state_valid": coveredCheck("checkMandatory", (*findings).checkMandatory),
	"RESOURCE_DESCRIPTION_ITEM.Purpose_valid":    coveredCheck("checkMandatory", (*findings).checkMandatory),
	"REVISION_HISTORY_ITEM.Audit_valid":          coveredCheck("checkMandatory", (*findings).checkMandatory),
	"UID.Value_valid":                            coveredCheck("checkMandatory", (*findings).checkMandatory),
	"VERSION_TREE_ID.Value_valid":                coveredCheck("checkMandatory", (*findings).checkMandatory),

	"Iso8601_date.Day_valid":                    coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_date.Month_valid":                  coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_date.Partial_validity":             coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_date.Year_valid":                   coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_date_time.Fractional_second_valid": coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_date_time.Hour_valid":              coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_date_time.Minute_valid":            coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_date_time.Partial_validity_minute": coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_date_time.Year_valid":              coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_duration.Days_valid":               coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_duration.Fractional_second_valid":  coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_duration.Hours_valid":              coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_duration.Minutes_valid":            coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_duration.Months_valid":             coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_duration.Seconds_valid":            coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_duration.Weeks_valid":              coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_duration.Years_valid":              coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_time.Fractional_second_valid":      coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_time.Hour_valid":                   coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_time.Minute_valid":                 coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),
	"Iso8601_time.Partial_validity":             coveredCheck("checkTemporalValue", (*findings).checkTemporalValue),

	"DV_PROPORTION.Is_integral_validity": coveredCheck("checkDVProportion", (*findings).checkDVProportion),
}

// attributeCheck is an evaluator the walk runs on a node of class and of
// every class below it.
type attributeCheck struct {
	class string
	check invariantCheck
	// owns names the RM-mandatory attributes the evaluator reports absent
	// itself, so the required-set check leaves them out on every class the
	// evaluator runs on and each absence is reported once.
	owns []string
}

// attributeChecks are the evaluators of the two rules the BMM states
// in an attribute's definition rather than as a named invariant, so the
// inventory does not list them: the mandatory parts of a reference
// ([findings.checkObjectRef], part of the mandatory-presence rule) and
// the precision range of a DV_QUANTITY ([findings.checkDVQuantity]).
// Each runs on the class it names and on every class the rminfo hierarchy
// places below it; the reference check owns the three parts it reports, so
// the required-set check leaves them out on exactly the classes it runs on.
var attributeChecks = []attributeCheck{
	{class: "OBJECT_REF", check: nodeCheck("checkObjectRef", (*findings).checkObjectRef), owns: []string{"id", "namespace", "type"}},
	{class: "DV_QUANTITY", check: nodeCheck("checkDVQuantity", (*findings).checkDVQuantity)},
}

// indexSet is a set of small integers: the classes a walk visited, by their
// [classRules] index, or the invariants a class is subject to and the RM rules
// do not evaluate, by their position in [inventoryRules].unevaluated.
type indexSet []uint64

// with returns s with i added, growing it as needed.
func (s indexSet) with(i int) indexSet {
	w := i / 64
	if w >= len(s) {
		s = append(s, make(indexSet, w+1-len(s))...)
	}
	s[w] |= 1 << (i % 64)
	return s
}

// union adds every member of t to s, growing it as needed, and returns it.
func (s indexSet) union(t indexSet) indexSet {
	if len(t) > len(s) {
		s = append(s, make(indexSet, len(t)-len(s))...)
	}
	for w, word := range t {
		s[w] |= word
	}
	return s
}

// each calls f with every member of s, in ascending order.
func (s indexSet) each(f func(i int)) {
	for w, word := range s {
		for word != 0 {
			f(w*64 + bits.TrailingZeros64(word))
			word &= word - 1
		}
	}
}

// boundCheck is one evaluator the walk runs on a node, with the invariant
// that dispatched it.
type boundCheck struct {
	run  func(into *findings, n floorNode, rule string)
	rule string
}

// classRules is what the walk runs on a node of one class, built once from
// the inventory: the evaluators, in a fixed order, the coded invariants, the
// attributes an evaluator reports absent itself, and the invariants of the
// class the RM rules do not evaluate.
type classRules struct {
	index       int
	checks      []boundCheck
	coded       []*codedRule
	owned       []string
	unevaluated indexSet
}

// leavesOut reports whether the required-set check leaves the RM-mandatory
// attr out on a node of the class, because an attribute check that runs on
// the class reports it absent itself. Nil rules, those of a type no
// registered class has, leave nothing out.
func (r *classRules) leavesOut(attr string) bool {
	return r != nil && slices.Contains(r.owned, attr)
}

// inventoryRules is the RM rules' dispatch, built once from the inventory.
type inventoryRules struct {
	// byClass holds the rules of every class the type registry decodes, by
	// its bare BMM name.
	byClass map[string]*classRules
	// byIndex holds the same rules, each at its index.
	byIndex []*classRules
	// unevaluated holds, sorted, every invariant some class of byClass is
	// subject to that the RM rules do not evaluate; a class's unevaluated set
	// indexes into it.
	unevaluated []string
}

// inventorySource is what the RM rules' dispatch is built from: the classes
// the type registry decodes with the invariants each is subject to, the
// classification of every invariant, the bindings of the evaluated ones, the
// attribute checks, and the class hierarchy the attribute checks follow.
type inventorySource struct {
	classes    map[string][]string
	inventory  map[string]inventoryEntry
	checks     map[string]invariantCheck
	attributes []attributeCheck
	hierarchy  rminfo.Hierarchy
}

// pinnedSource is the generated inventory with the bindings of this file.
var pinnedSource = inventorySource{
	classes:    classInvariants,
	inventory:  invariantInventory,
	checks:     invariantChecks,
	attributes: attributeChecks,
	hierarchy:  hierarchyOf(rminfo.Default),
}

// hierarchyOf returns the class hierarchy l answers, or nil when it answers
// none.
func hierarchyOf(l rminfo.Lookup) rminfo.Hierarchy {
	h, _ := l.(rminfo.Hierarchy)
	return h
}

// floorRules is the RM rules' dispatch.
var floorRules = pinnedSource.build()

// classAttributes holds, for each class the pinned class metadata knows, by
// its bare name, what the metadata says of each attribute it lists for the
// class ([classFacts]) and how the one walk reads each one it visits on a
// node of the class ([walkedAttr]). Built once, when the package is
// initialised, and never changed, so the walk asks the metadata nothing per
// node and builds no attribute per node but the part a node's role decides
// ([engineAttr.inRole]).
var classAttributes = buildClassAttributes(rminfo.Default, floorRules.byClass)

// classAttributeTable is what the class metadata says of the attributes of
// one class: each attribute it lists, in its order; each of those the walk
// visits on a node of the class, those a serialization carries and the
// metadata gives a type, in the same order, as the walk reads them with no
// template declaration ([walkedAttr]); and the position of each of these by
// name. The pinned metadata lists no name twice for a class.
type classAttributeTable struct {
	listed []attributeFacts
	walked []engineAttr
	byName map[string]int
}

// find returns the attribute name the table walks, nil when it walks none by
// that name: one the class does not list, no serialization carries or the
// metadata gives no type. A nil table walks none.
func (t *classAttributeTable) find(name string) *engineAttr {
	i, ok := t.position(name)
	if !ok {
		return nil
	}
	return &t.walked[i]
}

// position returns the position in walked of the attribute name the table
// walks, with ok set; ok is unset when the table walks none by that name, as
// [classAttributeTable.find] reads it.
func (t *classAttributeTable) position(name string) (i int, ok bool) {
	if t == nil {
		return 0, false
	}
	i, ok = t.byName[name]
	return i, ok
}

// buildClassAttributes builds [classAttributes] for every class l knows,
// whose rules, nil for a class the type registry does not decode, rules
// holds.
func buildClassAttributes(l rminfo.Lookup, rules map[string]*classRules) map[string]*classAttributeTable {
	known := l.KnownRMTypes()
	out := make(map[string]*classAttributeTable, len(known))
	for _, class := range known {
		listed := classFacts(class)
		t := &classAttributeTable{listed: listed, byName: make(map[string]int, len(listed))}
		for _, f := range listed {
			if f.NonStorable || !f.Typed {
				continue
			}
			t.byName[f.Name] = len(t.walked)
			t.walked = append(t.walked, walkedAttr(f, rules[class]))
		}
		out[class] = t
	}
	return out
}

// attributeFacts is what the pinned Reference Model says of one attribute of
// one class, as the walk reads it from [rminfo.Default].
type attributeFacts struct {
	// Name is the attribute's name.
	Name string
	// RMType is the RM type the class metadata declares for the attribute,
	// the type of its items for a container, as the metadata writes it: a
	// formal generic parameter stays as written. Empty when Typed is unset.
	RMType string
	// Typed is set when the class metadata gives the attribute a type.
	Typed bool
	// NonStorable is set for an attribute no serialization carries
	// ([rminfo.IsNonStorableAttr]).
	NonStorable bool
	// Container is set for a multi-valued attribute.
	Container bool
	// PrimitiveItems is set when the metadata knows RMType as a primitive
	// type, such as an Octet or a String, which has nothing below it.
	PrimitiveItems bool
	// Hash is set when RMType is the Hash the metadata declares for every
	// Hash attribute.
	Hash bool
	// Mandatory is set when the class metadata marks the attribute
	// mandatory for the class.
	Mandatory bool
}

// hashType is the RM type the class metadata declares for every Hash
// attribute.
const hashType = "Hash"

// factsOf returns the facts [rminfo.Default] gives of the attribute name of
// the bare RM class class, whose mandatory attributes are required.
func factsOf(class, name string, required []string) attributeFacts {
	f := attributeFacts{
		Name:        name,
		NonStorable: rminfo.IsNonStorableAttr(class, name),
		Mandatory:   slices.Contains(required, name),
	}
	f.RMType, f.Typed = rminfo.Default.AttributeRMType(class, name)
	f.Container, _ = rminfo.Default.IsContainer(class, name)
	if ar, ok := rminfo.Default.(rminfo.AbsenceReporter); ok {
		f.PrimitiveItems = ar.AbsenceReason(f.RMType) == rminfo.AbsencePrimitive
	}
	f.Hash = f.RMType == hashType
	return f
}

// classFacts returns the facts [rminfo.Default] gives of each attribute it
// lists for the bare RM class class, in its order; nil for a class it does
// not know. Each call builds them afresh; [classAttributes] holds them, built
// once per class.
func classFacts(class string) []attributeFacts {
	var names []string
	if lister, ok := rminfo.Default.(rminfo.AttributeLister); ok {
		names = lister.AttributeNames(class)
	}
	if len(names) == 0 {
		return nil
	}
	required := rminfo.Default.RequiredAttributes(class)
	out := make([]attributeFacts, len(names))
	for i, name := range names {
		out[i] = factsOf(class, name, required)
	}
	return out
}

// build builds the RM rules' dispatch. On a node of a class it runs, in this
// order: the attribute checks that apply to the class; the evaluator of each
// evaluated invariant the class is subject to, in key order, once per node
// unless it reports the invariant that dispatched it; then the coded
// invariants, which [findings.checkCodedInvariants] checks in the order
// [codedValues] lists the node's coded attributes (an ENTRY's language
// before its encoding), whatever the order of their keys. An invariant the
// classification calls evaluated that has no binding is treated as not
// evaluated, so Coverage still names it; so is one another check decides, on
// a class that check does not run on.
func (src inventorySource) build() inventoryRules {
	classes := slices.Sorted(maps.Keys(src.classes))
	byIndex := make([]*classRules, len(classes))
	notEvaluated := make([][]string, len(classes))
	listed := map[string]bool{}
	for i, class := range classes {
		r := &classRules{index: i}
		runs := map[string]bool{mandatoryCheck: true}
		for _, ac := range src.attributes {
			if conformsTo(src.hierarchy, class, ac.class) {
				r.checks = append(r.checks, boundCheck{run: ac.check.run})
				r.owned = append(r.owned, ac.owns...)
				runs[ac.check.function] = true
			}
		}
		served := map[string]bool{}
		var covered []string
		for _, key := range src.classes[class] {
			if !src.evaluated(key) {
				notEvaluated[i] = append(notEvaluated[i], key)
				continue
			}
			c := src.checks[key]
			switch {
			case c.covered:
				covered = append(covered, key)
				continue
			case c.coded != nil:
				r.coded = append(r.coded, c.coded)
				runs[c.function] = true
				continue
			}
			runs[c.function] = true
			id := c.function
			if c.perRule {
				id += " " + key
			}
			if !served[id] {
				served[id] = true
				r.checks = append(r.checks, boundCheck{run: c.run, rule: key})
			}
		}
		for _, key := range covered {
			if !runs[src.checks[key].function] {
				notEvaluated[i] = append(notEvaluated[i], key)
			}
		}
		for _, key := range notEvaluated[i] {
			listed[key] = true
		}
		byIndex[i] = r
	}
	unevaluated := slices.Sorted(maps.Keys(listed))
	position := make(map[string]int, len(unevaluated))
	for i, key := range unevaluated {
		position[key] = i
	}
	byClass := make(map[string]*classRules, len(classes))
	for i, class := range classes {
		r := byIndex[i]
		for _, key := range notEvaluated[i] {
			r.unevaluated = r.unevaluated.with(position[key])
		}
		byClass[class] = r
	}
	return inventoryRules{byClass: byClass, byIndex: byIndex, unevaluated: unevaluated}
}

// evaluated reports whether the RM rules evaluate the invariant key: the
// classification calls it evaluated, and a binding names its evaluator or the
// check that decides it. For an invariant another check decides, [build]
// also requires that check to run on the class.
func (src inventorySource) evaluated(key string) bool {
	if src.inventory[key].kind != invariantEvaluated {
		return false
	}
	c, ok := src.checks[key]
	return ok && (c.run != nil || c.coded != nil || c.covered)
}

// conformsTo reports whether class is ancestor or descends from it in the
// rminfo hierarchy; without a hierarchy, only the class itself conforms.
func conformsTo(h rminfo.Hierarchy, class, ancestor string) bool {
	if class == ancestor {
		return true
	}
	if h == nil {
		return false
	}
	ok, known := h.ConformsTo(class, ancestor)
	return ok && known
}

// classEntry is what the walk reads of one class at a node, found by the
// class's bare name in one lookup ([classOf]): the class's RM rules
// ([floorRules]), nil for a class the type registry does not decode, and
// the table of the attributes the class metadata lists for it
// ([classAttributes]), nil for a class the metadata does not know.
type classEntry struct {
	rules *classRules
	attrs *classAttributeTable
}

// classEntries holds the entry of every class that has RM rules or an
// attribute table, by its bare name. The two sets need not be the same: in
// the pinned RM every class the type registry decodes has a table, while
// the classes the metadata knows that the registry does not decode, the
// abstract ones such as LOCATABLE and DV_ORDERED among them, have a table
// and no rules. Built once, when the package is initialised.
var classEntries = joinClasses(floorRules.byClass, classAttributes)

// joinClasses returns the entry of every class rules or tables holds, each
// with what the other holds for it, if anything.
func joinClasses(rules map[string]*classRules, tables map[string]*classAttributeTable) map[string]classEntry {
	out := make(map[string]classEntry, max(len(rules), len(tables)))
	for class, r := range rules {
		out[class] = classEntry{rules: r, attrs: tables[class]}
	}
	for class, t := range tables {
		if _, done := out[class]; !done {
			out[class] = classEntry{attrs: t}
		}
	}
	return out
}

// classOf returns the entry of the class of a node of RM type rmType, which
// may carry a generic bound (DV_INTERVAL<DV_QUANTITY>), and the bare class
// name; the zero entry, no rules and no table, for a type no class has.
func classOf(rmType string) (classEntry, string) {
	class := bareClass(rmType)
	return classEntries[class], class
}

// rulesFor returns the RM rules for a node of RM type rmType, which may
// carry a generic bound (DV_INTERVAL<DV_QUANTITY>), and the bare class name;
// nil rules for a type no registered class has. It reads [floorRules]
// itself, so the class tables can ask it while they are built.
func rulesFor(rmType string) (*classRules, string) {
	class := bareClass(rmType)
	return floorRules.byClass[class], class
}

// bareClass returns the RM type rmType without its generic bound, if any.
func bareClass(rmType string) string {
	if strings.IndexByte(rmType, '<') >= 0 {
		return bmmtype.Class(rmType)
	}
	return rmType
}

// unevaluatedIn returns, sorted and once each, the invariants the classes of
// visited are subject to, their ancestors' included, that the RM rules do
// not evaluate, for [Coverage.Unevaluated]. It returns nil when visited
// holds no class subject to one.
func unevaluatedIn(visited indexSet) []string {
	keys := make(indexSet, (len(floorRules.unevaluated)+63)/64)
	visited.each(func(i int) {
		keys = keys.union(floorRules.byIndex[i].unevaluated)
	})
	var out []string
	keys.each(func(i int) {
		out = append(out, floorRules.unevaluated[i])
	})
	return out
}

package validation

// inventory_dispatch.go: REQ-171 *Where the RM rules come from*. The RM rules
// the floor applies are the evaluated entries of the invariant inventory
// together with the mandatory-presence rule. The inventory (inventory_gen.go)
// is generated from the pinned RM and BASE BMMs and classified in
// invariants.json; this file binds each evaluated entry to the Go function
// that evaluates it, and builds from both, once, what the floor runs on a
// node of each class the type registry decodes and which of the invariants
// that class is subject to the floor does not evaluate.

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
	// invariantEvaluated is an invariant the floor evaluates, with the Go
	// function that evaluates it.
	invariantEvaluated invariantKind = iota + 1
	// invariantNotMechanical is an invariant no check of the instance alone
	// can decide: it needs a terminology service, stored history, or a
	// comparison the RM leaves undefined.
	invariantNotMechanical
	// invariantOutOfScope is an invariant the SDK does not check on an
	// instance, for the reason the classification gives.
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
	role   floorRole
}

// invariantCheck binds an evaluated invariant to the evaluator that
// evaluates it.
type invariantCheck struct {
	// function is the evaluator's name, as invariants.json names it. The
	// evaluator serves the node once, however many of the node's invariants
	// it is bound to, unless perRule is set.
	function string
	// run calls the evaluator on a node; rule is the invariant that
	// dispatched it.
	run func(w *rmFloorWalker, n floorNode, rule string)
	// perRule is set for an evaluator that reports the invariant that
	// dispatched it, so it serves each of them.
	perRule bool
	// coded is set, in place of run, for a coded invariant, which
	// [rmFloorWalker.checkCodedInvariants] evaluates from its row.
	coded *codedRule
}

// nodeCheck binds an evaluator that reads the node's value and path.
func nodeCheck(function string, f func(*rmFloorWalker, any, string)) invariantCheck {
	return invariantCheck{function: function, run: func(w *rmFloorWalker, n floorNode, _ string) {
		f(w, n.value, n.path)
	}}
}

// roleCheck binds an evaluator that also reads the node's role.
func roleCheck(function string, f func(*rmFloorWalker, any, string, floorRole)) invariantCheck {
	return invariantCheck{function: function, run: func(w *rmFloorWalker, n floorNode, _ string) {
		f(w, n.value, n.path, n.role)
	}}
}

// typeCheck binds an evaluator that also reads the node's RM type, to name
// the class it reports on.
func typeCheck(function string, f func(*rmFloorWalker, any, string, string)) invariantCheck {
	return invariantCheck{function: function, run: func(w *rmFloorWalker, n floorNode, _ string) {
		f(w, n.value, n.rmType, n.path)
	}}
}

// ruleCheck binds an evaluator that reads the node's RM type and reports
// the invariant that dispatched it: one Go function serves an invariant of
// several classes (Value_valid), or one invariant declared on an ancestor of
// several (Is_archetype_root), and the rule it reports is always the
// inventory's key.
func ruleCheck(function string, f func(*rmFloorWalker, any, string, string, string)) invariantCheck {
	return invariantCheck{function: function, perRule: true, run: func(w *rmFloorWalker, n floorNode, rule string) {
		f(w, n.value, n.rmType, n.path, rule)
	}}
}

// codedCheck binds a coded invariant to its row of the coded table
// (rmfloor_coded.go).
func codedCheck(row *codedRule) invariantCheck {
	return invariantCheck{function: "checkCodedInvariants", coded: row}
}

// invariantChecks binds every evaluated invariant of the inventory to the Go
// function that evaluates it, by value, so a renamed or removed evaluator
// fails to build. The names recorded beside the functions are checked against
// the functions and against invariants.json by a test, which also fails when
// an evaluated invariant is missing here or one listed here is not evaluated.
var invariantChecks = map[string]invariantCheck{
	"ARCHETYPED.Rm_version_valid":        nodeCheck("checkArchetyped", (*rmFloorWalker).checkArchetyped),
	"CODE_PHRASE.Code_string_valid":      roleCheck("checkCodePhrase", (*rmFloorWalker).checkCodePhrase),
	"COMPOSITION.Is_archetype_root":      ruleCheck("checkArchetypeRoot", (*rmFloorWalker).checkArchetypeRoot),
	"DV_DATE.Value_valid":                ruleCheck("checkTemporalValue", (*rmFloorWalker).checkTemporalValue),
	"DV_DATE_TIME.Value_valid":           ruleCheck("checkTemporalValue", (*rmFloorWalker).checkTemporalValue),
	"DV_DURATION.Value_valid":            ruleCheck("checkTemporalValue", (*rmFloorWalker).checkTemporalValue),
	"DV_INTERVAL.Limits_consistent":      typeCheck("checkDVInterval", (*rmFloorWalker).checkDVInterval),
	"DV_PROPORTION.Precision_validity":   nodeCheck("checkDVProportion", (*rmFloorWalker).checkDVProportion),
	"DV_TEXT.Mappings_valid":             nodeCheck("checkTermMappings", (*rmFloorWalker).checkTermMappings),
	"DV_TIME.Value_valid":                ruleCheck("checkTemporalValue", (*rmFloorWalker).checkTemporalValue),
	"EHR_ACCESS.Is_archetype_root":       ruleCheck("checkArchetypeRoot", (*rmFloorWalker).checkArchetypeRoot),
	"EHR_STATUS.Is_archetype_root":       ruleCheck("checkArchetypeRoot", (*rmFloorWalker).checkArchetypeRoot),
	"ELEMENT.Inv_null_flavour_indicated": nodeCheck("checkElementNullFlavour", (*rmFloorWalker).checkElementNullFlavour),
	"ENTRY.Is_archetype_root":            ruleCheck("checkArchetypeRoot", (*rmFloorWalker).checkArchetypeRoot),
	"Interval.Lower_included_valid":      typeCheck("checkDVInterval", (*rmFloorWalker).checkDVInterval),
	"Interval.Upper_included_valid":      typeCheck("checkDVInterval", (*rmFloorWalker).checkDVInterval),
	"PARTY.Is_archetype_root":            ruleCheck("checkArchetypeRoot", (*rmFloorWalker).checkArchetypeRoot),
	"PARTY_IDENTIFIED.Basic_validity":    nodeCheck("checkPartyIdentified", (*rmFloorWalker).checkPartyIdentified),
	"PARTY_IDENTIFIED.Identifiers_valid": nodeCheck("checkPartyIdentified", (*rmFloorWalker).checkPartyIdentified),
	"PARTY_IDENTIFIED.Name_valid":        nodeCheck("checkPartyIdentified", (*rmFloorWalker).checkPartyIdentified),
	"TERM_MAPPING.Match_valid":           nodeCheck("checkTermMapping", (*rmFloorWalker).checkTermMapping),

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
}

// attributeCheck is an evaluator the floor runs on a node of class and of
// every class below it.
type attributeCheck struct {
	class string
	check invariantCheck
}

// attributeChecks are the floor's evaluators of the two rules the BMM states
// in an attribute's definition rather than as a named invariant, so the
// inventory does not list them: the mandatory parts of a reference
// ([rmFloorWalker.checkObjectRef], part of the mandatory-presence rule) and
// the precision range of a DV_QUANTITY ([rmFloorWalker.checkDVQuantity]).
// Each runs on the class it names and on every class the rminfo hierarchy
// places below it.
var attributeChecks = []attributeCheck{
	{class: "OBJECT_REF", check: nodeCheck("checkObjectRef", (*rmFloorWalker).checkObjectRef)},
	{class: "DV_QUANTITY", check: nodeCheck("checkDVQuantity", (*rmFloorWalker).checkDVQuantity)},
}

// indexSet is a set of small integers: the classes a walk visited, by their
// [classRules] index, or the invariants a class is subject to and the floor
// does not evaluate, by their position in [inventoryRules].unevaluated.
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

// boundCheck is one evaluator the floor runs on a node, with the invariant
// that dispatched it.
type boundCheck struct {
	run  func(w *rmFloorWalker, n floorNode, rule string)
	rule string
}

// classRules is what the floor runs on a node of one class, built once from
// the inventory: the evaluators, in a fixed order, the coded invariants, and
// the invariants of the class the floor does not evaluate.
type classRules struct {
	index       int
	checks      []boundCheck
	coded       []*codedRule
	unevaluated indexSet
}

// inventoryRules is the floor's dispatch, built once from the inventory.
type inventoryRules struct {
	// byClass holds the rules of every class the type registry decodes, by
	// its bare BMM name.
	byClass map[string]*classRules
	// byIndex holds the same rules, each at its index.
	byIndex []*classRules
	// unevaluated holds, sorted, every invariant some class of byClass is
	// subject to that the floor does not evaluate; a class's unevaluated set
	// indexes into it.
	unevaluated []string
}

// inventorySource is what the floor's dispatch is built from: the classes
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

// floorRules is the floor's dispatch.
var floorRules = pinnedSource.build()

// build builds the floor's dispatch. On a node of a class it runs, in this
// order: the attribute checks that apply to the class; the evaluator of each
// evaluated invariant the class is subject to, in key order, once per node
// unless it reports the invariant that dispatched it; then the coded
// invariants, in the class's key order. An invariant the classification
// calls evaluated that has no binding is treated as not evaluated, so
// Coverage still names it.
func (src inventorySource) build() inventoryRules {
	classes := slices.Sorted(maps.Keys(src.classes))
	notEvaluated := map[string]bool{}
	for _, class := range classes {
		for _, key := range src.classes[class] {
			if !src.evaluated(key) {
				notEvaluated[key] = true
			}
		}
	}
	unevaluated := slices.Sorted(maps.Keys(notEvaluated))
	position := make(map[string]int, len(unevaluated))
	for i, key := range unevaluated {
		position[key] = i
	}
	byClass := make(map[string]*classRules, len(classes))
	byIndex := make([]*classRules, len(classes))
	for i, class := range classes {
		r := &classRules{index: i}
		for _, ac := range src.attributes {
			if conformsTo(src.hierarchy, class, ac.class) {
				r.checks = append(r.checks, boundCheck{run: ac.check.run})
			}
		}
		served := map[string]bool{}
		for _, key := range src.classes[class] {
			if !src.evaluated(key) {
				r.unevaluated = r.unevaluated.with(position[key])
				continue
			}
			c := src.checks[key]
			if c.coded != nil {
				r.coded = append(r.coded, c.coded)
				continue
			}
			id := c.function
			if c.perRule {
				id += " " + key
			}
			if !served[id] {
				served[id] = true
				r.checks = append(r.checks, boundCheck{run: c.run, rule: key})
			}
		}
		byClass[class] = r
		byIndex[i] = r
	}
	return inventoryRules{byClass: byClass, byIndex: byIndex, unevaluated: unevaluated}
}

// evaluated reports whether the floor evaluates the invariant key: the
// classification calls it evaluated, and a binding names its evaluator.
func (src inventorySource) evaluated(key string) bool {
	if src.inventory[key].kind != invariantEvaluated {
		return false
	}
	c, ok := src.checks[key]
	return ok && (c.run != nil || c.coded != nil)
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

// rulesFor returns the floor's rules for a node of RM type rmType, which may
// carry a generic bound (DV_INTERVAL<DV_QUANTITY>), and the bare class name;
// nil rules for a type no registered class has.
func rulesFor(rmType string) (*classRules, string) {
	class := rmType
	if strings.IndexByte(rmType, '<') >= 0 {
		class = bmmtype.Class(rmType)
	}
	return floorRules.byClass[class], class
}

// recordClass adds the class of value, as the closed RM set names it, to
// visited, and returns the set. A value of no registered class leaves it as
// it is.
func recordClass(visited indexSet, value any) indexSet {
	rmType, _, ok := rmTypeInfo(value)
	if !ok {
		return visited
	}
	if r, _ := rulesFor(rmType); r != nil {
		return visited.with(r.index)
	}
	return visited
}

// unevaluatedIn returns, sorted and once each, the invariants the classes of
// the visited sets are subject to, their ancestors' included, that the floor
// does not evaluate, for [Coverage.Unevaluated]. It returns nil when the sets
// hold no class subject to one.
func unevaluatedIn(visited ...indexSet) []string {
	keys := make(indexSet, (len(floorRules.unevaluated)+63)/64)
	for _, set := range visited {
		set.each(func(i int) {
			keys = keys.union(floorRules.byIndex[i].unevaluated)
		})
	}
	var out []string
	keys.each(func(i int) {
		out = append(out, floorRules.unevaluated[i])
	})
	return out
}

package validation

// rmfloor.go: REQ-112 — the template-less Reference Model validation floor:
// its entries and its rules. The one walk (engine.go, REQ-171) applies the
// rules, with no template at ValidateRM and its typed wrappers, and inside
// the walk that binds the template at the REQ-102/110 entries (ADR 0025,
// ADR 0027). The rules here report into [findings]:
//
//   - RM-mandatory attribute absences (checkMandatory, which the walk calls
//     for each mandatory attribute it reads absent or empty), and an item of
//     a list that holds no RM value (checkNilItem);
//   - the per-RM-type invariant catalogue of § REQ-112 in
//     docs/specifications/clinical-modeling.md, on every node the walk
//     visits. checkInvariants dispatches it through the invariant inventory
//     (REQ-171, inventory_dispatch.go): on a node of a class, the
//     evaluators of the evaluated invariants the class is subject to, the
//     Coded invariants entry (rmfloor_coded.go) among them; invariants.json,
//     not this comment, is the list to keep current.
//
// The closed RM-type set is shared with the template rules through the
// rmTypeInfo and describeRMType helpers (composition.go).

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// ValidateRM validates root against the openEHR Reference Model alone, for
// a root no operational template applies to. It checks every RM-mandatory
// attribute on every node reachable from root, and runs per-RM-type
// invariants on the nodes it visits. It does not consult any operational
// template. The template-driven entries, [Validate] /
// [ValidateComposition] / [ValidateFolder] / [ValidateEHRStatus] /
// [ValidateDemographic], apply the same rules in their own walk, so a
// caller with a compiled OPT needs only one of them. With no template the
// walk knows no template names, so an item whose archetype node id a
// sibling shares is written by its position.
//
// The invariants include the RM's coded ones: an attribute the RM codes from
// a group or a code set of the openEHR terminology, such as a COMPOSITION's
// language and territory, an EVENT_CONTEXT's setting, an ENTRY's language
// and encoding, an ELEMENT's null_flavour, a DV_ORDERED's normal_status, a
// PARTICIPATION's function and mode or a PARTY_RELATED's relationship, must
// hold a member of it. Groups and code sets come from
// [github.com/cadasto/openehr-sdk-go/openehr/terminology]. A group code must
// also be coded in the openEHR terminology itself; a code-set code is
// matched alone, ignoring letter case for the ISO and IANA sets. An optional
// attribute is checked only when present, and a mandatory one always, so an
// empty one is reported too. A breach is reported as `code_not_in_value_set`
// at the CODE_PHRASE read: the attribute itself, or its defining_code for a
// coded text. The detail names the attribute, the RM invariant and the
// group or code set, never the code. The coded rules of the change-control
// classes (VERSION, AUDIT_DETAILS, ATTESTATION) and of the resource-description
// classes (TRANSLATION_DETAILS, RESOURCE_DESCRIPTION_ITEM) are not checked,
// even on such a value passed as root, an ORIGINAL_VERSION included; nor are
// those of EXTRACT_PARTICIPATION, a class the SDK's RM types leave out.
//
// A value built in Go may hold a pointer in two places, or hold itself; a
// decoded document holds neither. A value reached again by another path is
// validated at each place, so a defect in it is reported at each path. A
// value reached again inside itself is reported as `cyclic_value` at each
// path where it recurs and is not entered again. A value held by value can
// hold itself too, through a list that shares its own backing array, and
// the walk cannot recognise it; so every call stops after a number of
// visits, by default far above what a decoded document needs, and reports
// `max_nodes` once, at the path where it stopped. A graph nested deeper than
// the walk's depth limit is not walked below it; the walk reports
// `max_depth` once, at the first path where it stopped. [Result.Coverage]
// lists the path of each of the two, and the [Limits] of a [Validator] set
// both.
//
// ValidateRM returns what a [Validator] built with no options returns from
// Validate(root, nil).
//
// A nil root surfaces a single `nil_root` issue with an empty path and is
// reported as not-OK. An unknown RM root type (a Go value outside the
// closed RM set) surfaces `rm_type_unknown` at "/"; the walk cannot descend
// further but does not panic.
func ValidateRM(root any) Result {
	var v Validator
	return v.Validate(root, nil)
}

// rmRootGuard returns the RM type of root, the type the RM rules alone read
// it as, with ok set. For a nil root, and for one outside the closed RM set,
// it returns instead the result those rules report for it, a single
// `nil_root` or `rm_type_unknown` issue, with ok unset.
func rmRootGuard(root any) (rmType string, guard Result, ok bool) {
	if root == nil || rmaccess.IsTypedNilPointer(root) {
		return "", resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_root",
			Detail:   "ValidateRM: root is nil",
			Severity: Error,
		}}), false
	}
	rmType, _, ok = rmTypeInfo(root)
	if !ok {
		return "", resultFromIssues([]Issue{{
			Path:     "/",
			Code:     "rm_type_unknown",
			Detail:   fmt.Sprintf("ValidateRM: root Go type %T is outside the closed RM set; cannot descend", root),
			Severity: Error,
		}}), false
	}
	return rmType, Result{}, true
}

// ValidateRMFolder is the typed convenience wrapper for FOLDER roots.
// Delegates to [ValidateRM]; a nil folder surfaces `nil_folder`, with an
// empty path.
func ValidateRMFolder(folder *rm.Folder) Result {
	if folder == nil {
		return resultFromIssues([]Issue{{Path: "", Code: "nil_folder", Detail: "ValidateRMFolder: folder is nil", Severity: Error}})
	}
	return ValidateRM(folder)
}

// ValidateRMEHRStatus is the typed convenience wrapper for EHR_STATUS.
// Delegates to [ValidateRM]; a nil status surfaces `nil_ehr_status`, with an
// empty path.
//
// It cannot flag an omitted value-typed mandatory `subject` (typed
// rm.PartySelf, whose zero value is indistinguishable from an absent one):
// use [ValidateRMEHRStatusBytes], which decides subject presence from the
// source JSON key set. The same holds for the root's archetype_details: an
// omitted `rm_version` decodes to an empty one, so this entry reports it as
// `rm_version_valid` where [ValidateRMEHRStatusBytes] reports `required`,
// and an omitted `archetype_id` is reported only at `archetype_id/value`,
// without the Bytes entry's `required` at `archetype_id`.
func ValidateRMEHRStatus(status *rm.EHRStatus) Result {
	if status == nil {
		return resultFromIssues([]Issue{{Path: "", Code: "nil_ehr_status", Detail: "ValidateRMEHRStatus: status is nil", Severity: Error}})
	}
	return ValidateRM(status)
}

// ValidateRMEHRAccess is the typed convenience wrapper for EHR_ACCESS.
// Delegates to [ValidateRM]; a nil access surfaces `nil_ehr_access`, with an
// empty path.
func ValidateRMEHRAccess(access *rm.EHRAccess) Result {
	if access == nil {
		return resultFromIssues([]Issue{{Path: "", Code: "nil_ehr_access", Detail: "ValidateRMEHRAccess: access is nil", Severity: Error}})
	}
	return ValidateRM(access)
}

// ValidateRMDemographic is the typed convenience wrapper for the
// demographic PARTY hierarchy (PERSON / ORGANISATION / GROUP / AGENT /
// ROLE). Delegates to [ValidateRM]; a nil party surfaces `nil_party`, with
// an empty path.
func ValidateRMDemographic(party rm.Party) Result {
	if party == nil || rmaccess.IsTypedNilPointer(party) {
		return resultFromIssues([]Issue{{Path: "", Code: "nil_party", Detail: "ValidateRMDemographic: party is nil", Severity: Error}})
	}
	return ValidateRM(party)
}

// findings is what the Reference Model rules report into during one walk:
// the issues, in the order the walk found them, and the class of every node
// whose rules ran. The rule bodies are its methods, so each reports where the
// walk is; it holds no walk state of its own.
type findings struct {
	issues []Issue
	// visited holds the class of every node whose rules ran, by its index
	// in the inventory's rules ([classRules]), for [Coverage.Unevaluated].
	visited indexSet
}

// floorRole is what the RM rules know of a node from where it sits, beyond
// its RM type. It is carried down the walk, never looked up by path, so two
// nodes that share a path cannot share a role.
type floorRole uint8

const (
	// roleNone is every node the roles below do not name.
	roleNone floorRole = iota
	// roleScaleSymbol is the symbol of a DV_SCALE.
	roleScaleSymbol
	// roleScaleSymbolCode is the defining_code of a DV_SCALE symbol, whose
	// code_string may be blank (see [scaleSymbolMayHaveNoCode]).
	roleScaleSymbolCode
	// roleArchetypedID is the archetype_id of an ARCHETYPED, whose empty
	// value the ARCHETYPED entry reports ([findings.checkArchetyped]).
	roleArchetypedID
)

// childRole returns the role of the node under attr of a node of RM class
// class that has role.
func childRole(class string, role floorRole, attr string) floorRole {
	switch {
	case class == "DV_SCALE" && attr == "symbol":
		return roleScaleSymbol
	case role == roleScaleSymbol && attr == "defining_code":
		return roleScaleSymbolCode
	case class == "ARCHETYPED" && attr == "archetype_id":
		return roleArchetypedID
	}
	return roleNone
}

// isWalkCondition reports whether code is a walk condition: max_depth,
// max_nodes or cyclic_value, an issue that says where the walk stopped or
// turned back rather than judge the value there.
func isWalkCondition(code string) bool {
	switch code {
	case "max_depth", "max_nodes", "cyclic_value":
		return true
	}
	return false
}

// emit appends i to the issues found. Every issue of an RM rule is an
// [Error], the zero Severity, so no site sets it.
func (f *findings) emit(i Issue) {
	f.issues = append(f.issues, i)
}

// checkMandatory applies the mandatory-presence rule to the RM-mandatory
// attribute attr of a node of RM type rmType, as the walk read it at
// attrPath: absent, it is reported as `required`, rule mandatory; a
// multi-valued one that is present and holds no item, as `cardinality`, rule
// cardinality. present and items are the walk's reading through the generated
// access, under which a mandatory string left empty is absent too; items
// counts a multi-valued attribute's items and is read for no other. The walk
// calls it for every RM-mandatory attribute of every node it visits, where
// no template clause reports the same absence first, but a
// part of a reference, which [findings.checkObjectRef] reports, and a
// value the node's role lets be blank ([mayBeBlank]). No invariant the
// inventory says this rule decides, such as LOCATABLE.Archetype_node_id_valid,
// is about one of those, so each is decided on every class subject to it
// ([invariantChecks]).
func (f *findings) checkMandatory(rmType, attr, attrPath string, multiValued, present bool, items int) {
	switch {
	case !present && multiValued:
		f.emit(Issue{
			Path:   attrPath,
			Code:   "required",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: fmt.Sprintf("RM-mandatory multi-valued attribute %q absent on %s", attr, rmType),
		})
	case !present:
		f.emit(Issue{
			Path:   attrPath,
			Code:   "required",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: fmt.Sprintf("RM-mandatory attribute %q absent on %s", attr, rmType),
		})
	case multiValued && items == 0:
		f.emit(Issue{
			Path:   attrPath,
			Code:   "cardinality",
			Layer:  LayerRM,
			Rule:   ruleCardinality,
			Detail: fmt.Sprintf("RM-mandatory multi-valued attribute %q must be non-empty on %s", attr, rmType),
		})
	}
}

// checkNilItem applies the mandatory-presence rule to an item of the list
// attribute attr of a node of RM type rmType that holds no RM value, a nil
// element or a typed-nil pointer ([isNilItem]): it is reported as
// `required` at the item's own path, itemPath, rule mandatory. The walk
// calls it for each such item of a list no template node binds, and does
// not walk the item; the item still counts toward its list, so a list that
// holds only such items is present and not empty. A nil item the template
// refuses is reported by the template's rule alone.
func (f *findings) checkNilItem(rmType, attr, itemPath string) {
	f.emit(Issue{
		Path:   itemPath,
		Code:   "required",
		Layer:  LayerRM,
		Rule:   ruleMandatory,
		Detail: fmt.Sprintf("item of %q on %s holds no RM value", attr, rmType),
	})
}

// hashEntryPath returns the issue path of the entry at index i, in ascending
// key order, of the Hash attribute at attrPath: its 1-based position, never
// its key, which is instance data (REQ-102 Issue paths).
func hashEntryPath(attrPath string, i int) string {
	return attrPath + "[" + strconv.Itoa(i+1) + "]"
}

// runtimeRMType resolves the RM type to descend into: the value's runtime
// type when the closed RM-node set recognises it (so polymorphic subtypes
// — an OBSERVATION inside a CONTENT_ITEM container, a DV_QUANTITY inside a
// DV_ORDERED slot — dispatch their own invariants and required-set),
// otherwise the BMM-declared attribute type. Applied uniformly to single-
// and multi-valued attributes.
func runtimeRMType(val any, declared string) string {
	if rt, _, ok := rmTypeInfo(val); ok {
		return rt
	}
	return declared
}

// checkInvariants runs the RM rules of the class of the node value, of RM
// type rmType, at path, in role role, and records the class as visited. The
// rules are the inventory's ([floorRules]): the evaluators of the evaluated
// invariants the class and its ancestors declare, the coded ones included,
// and the attribute checks that apply to the class. A type no registered
// class has runs nothing (this is a floor, not an exhaustive RM check). It
// returns the class's rules, nil for such a type, and the bare class name of
// rmType, which the walk reads the class's attributes by ([classAttributes]).
func (f *findings) checkInvariants(value any, rmType, path string, role floorRole) (*classRules, string) {
	rules, class := rulesFor(rmType)
	if rules == nil {
		return nil, class
	}
	f.visited = f.visited.with(rules.index)
	n := floorNode{value: value, rmType: rmType, path: path, role: role}
	for _, c := range rules.checks {
		c.run(f, n, c.rule)
	}
	if len(rules.coded) > 0 {
		f.checkCodedInvariants(value, rmType, path, rules.coded)
	}
	return rules, class
}

// checkArchetypeRoot enforces the archetype-root rule on a node whose class
// is always an archetype root: one whose BMM declares Is_archetype_root, or
// descends from one that does, as the inventory records it. The class
// invariant `Is_archetype_root` fixes is_archetype_root true, and
// LOCATABLE's `Archetyped_valid` (is_archetype_root xor archetype_details =
// Void) then makes archetype_details mandatory, although LOCATABLE declares
// it optional. An absent archetype_details, whether omitted or JSON null,
// reports `is_archetype_root` at the node's archetype_details.
//
// The attribute is read through [rm.Locatable], which every LOCATABLE
// concrete implements. The issue's rule is the Is_archetype_root invariant
// that dispatched the check: the one the class declares, or the one its
// ancestor PARTY or ENTRY declares.
func (f *findings) checkArchetypeRoot(value any, rmType, path, rule string) {
	l, ok := value.(rm.Locatable)
	if !ok || rmaccess.IsTypedNilPointer(value) {
		return
	}
	if l.GetArchetypeDetails() == nil {
		f.emit(Issue{
			Path:   joinPath(path, "/archetype_details"),
			Code:   "is_archetype_root",
			Layer:  LayerRM,
			Rule:   rule,
			Detail: rmType + " is an archetype root, so archetype_details must be present (RM Is_archetype_root, LOCATABLE.Archetyped_valid)",
		})
	}
}

// checkArchetyped enforces the floor on an ARCHETYPED node, wherever it sits:
// its archetype_id and rm_version are RM-mandatory. Both are value-typed, so
// an absent attribute, a JSON null and an empty value all decode to the same
// zero value and are reported the same way:
//
//   - an empty archetype_id.value is `required` at archetype_id/value (the
//     RM makes OBJECT_ID.value mandatory but gives it no non-empty invariant;
//     reading empty as absent is SDK policy);
//   - an empty rm_version is `rm_version_valid` at rm_version (the RM's
//     `Rm_version_valid: not rm_version.is_empty`).
//
// The ARCHETYPE_ID grammar is not checked, and template_id is optional.
// Diagnostics name the attribute, never its value.
func (f *findings) checkArchetyped(value any, path string) {
	a, ok := asArchetyped(value)
	if !ok {
		return
	}
	if a.ArchetypeID.Value == "" {
		f.emit(Issue{
			Path:   joinPath(path, "/archetype_id/value"),
			Code:   "required",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: "ARCHETYPED.archetype_id must carry a non-empty value",
		})
	}
	if a.RMVersion == "" {
		f.emit(Issue{
			Path:   joinPath(path, "/rm_version"),
			Code:   "rm_version_valid",
			Layer:  LayerRM,
			Rule:   ruleRMVersionValid,
			Detail: "ARCHETYPED.rm_version must be non-empty (RM Rm_version_valid)",
		})
	}
}

// checkCodePhrase enforces the RM spec floor on CODE_PHRASE: the
// code_string MUST be non-empty when the value is present, except on a
// DV_SCALE symbol's defining_code (see [scaleSymbolMayHaveNoCode]). (The
// terminology_id absence is already RM-required and caught by the
// mandatory-presence rule.)
func (f *findings) checkCodePhrase(value any, path string, role floorRole) {
	cp, ok := asCodePhrase(value)
	if !ok {
		return
	}
	if cp.CodeString == "" && !scaleSymbolMayHaveNoCode(role) {
		f.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleCodeStringValid,
			Detail: "CODE_PHRASE.code_string must be non-empty",
		})
	}
}

// scaleSymbolMayHaveNoCode reports whether a CODE_PHRASE in role may carry a
// blank code_string. The RM's DV_SCALE.symbol allows a scale value that has
// no code: its symbol is then a DV_CODED_TEXT carrying the terminology_id and
// a blank code_string. The CODE_PHRASE at the symbol's defining_code therefore
// reports neither the non-empty code_string invariant nor a `required`
// code_string. The exemption covers that one node only: a blank
// terminology_id, the symbol's own value, a DV_ORDINAL symbol, and every other
// CODE_PHRASE under the scale are checked as usual. The walk hands the role
// down from the DV_SCALE ([childRole]), so the exemption follows the node and
// not its path, which another node may share.
func scaleSymbolMayHaveNoCode(role floorRole) bool {
	return role == roleScaleSymbolCode
}

// mayBeBlank reports whether the RM-mandatory attr on a node in role is
// exempt from the required-set check. Two are: the code_string of a DV_SCALE
// symbol's defining_code (see [scaleSymbolMayHaveNoCode]), and the value of
// an ARCHETYPED's archetype_id, whose emptiness the ARCHETYPED entry reports
// at the same path ([findings.checkArchetyped]), so it is reported once.
func mayBeBlank(role floorRole, attr string) bool {
	switch attr {
	case "code_string":
		return scaleSymbolMayHaveNoCode(role)
	case "value":
		return role == roleArchetypedID
	}
	return false
}

// checkDVQuantity enforces the spec floor on DV_QUANTITY: precision, when
// set, must be ≥ -1. Per the RM, precision is a number of decimal places
// where 0 means integral and -1 means "no limit" (any number of decimal
// places) — so -1 is valid and only precision < -1 is out of range. units
// is RM-required (caught by the mandatory-presence rule); magnitude is
// always a number on the wire so needs no separate presence check.
//
// The Detail names the attribute and the allowed range, never the
// precision the instance carries; that precision is carried in Issue.Value
// (REQ-168).
func (f *findings) checkDVQuantity(value any, path string) {
	q, ok := asDVQuantity(value)
	if !ok {
		return
	}
	if q.Precision != nil && int(*q.Precision) < -1 {
		f.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   rulePrecisionRange,
			Detail: "DV_QUANTITY.precision must be ≥ -1 (-1 = no limit)",
			Value:  constraints.Redact(*q.Precision),
		})
	}
}

// checkDVProportion enforces the two precision arms the RM puts on
// DV_PROPORTION:
//
//   - Range. precision, when set, must be ≥ -1. The BMM property doc
//     string gives it the same reading as DV_QUANTITY.precision — a
//     number of decimal places, where 0 means integral and -1 means "no
//     limit" — so only precision < -1 is out of range. (DV_QUANTITY
//     carries no `invariants` map in the vendored BMM; this arm rests on
//     the property doc string for both carriers.)
//   - Integrality. DV_PROPORTION's own BMM invariant
//     `Precision_validity: precision = 0 implies is_integral`, read
//     through `Is_integral_validity: is_integral implies
//     (numerator.floor = numerator and denominator.floor = denominator)`.
//     A precision of 0 therefore requires whole-number operands, and a
//     fractional numerator or denominator breaches the RM even though the
//     precision value itself is in range.
//
// The remaining DV_PROPORTION invariants are NOT in the floor: the
// type-kind rules (`Type_validity`, `Unitary_validity`, `Percent_validity`,
// `Fraction_validity`), which turn on the `type` proportion-kind code, a
// different axis from precision, and `Valid_denominator`
// (`denominator /= 0.0`), which reads the denominator alone.
//
// Diagnostics name the attribute and the offending operand, never a value
// from the instance: neither the precision nor an operand (REQ-168). The
// value the check read is carried in Issue.Value: the precision on the
// range arm, and the numerator and the denominator, as a [2]float64, on
// the integrality arm.
func (f *findings) checkDVProportion(value any, path string) {
	p, ok := asDVProportion(value)
	if !ok || p.Precision == nil {
		return
	}
	switch prec := int(*p.Precision); {
	case prec < -1:
		f.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   rulePrecisionRange,
			Detail: "DV_PROPORTION.precision must be ≥ -1 (-1 = no limit)",
			Value:  constraints.Redact(*p.Precision),
		})
	case prec == 0:
		var fractional []string
		if !isIntegralReal(p.Numerator) {
			fractional = append(fractional, "numerator")
		}
		if !isIntegralReal(p.Denominator) {
			fractional = append(fractional, "denominator")
		}
		if len(fractional) > 0 {
			f.emit(Issue{
				Path:   path,
				Code:   "rm_invariant",
				Layer:  LayerRM,
				Rule:   ruleProportionPrecision,
				Detail: "DV_PROPORTION.precision is 0, which requires a whole-number numerator and denominator (RM Precision_validity); not integral: " + strings.Join(fractional, ", "),
				Value:  constraints.Redact([2]float64{float64(p.Numerator), float64(p.Denominator)}),
			})
		}
	}
}

// isIntegralReal reports whether v is a whole number — the
// `numerator.floor = numerator` reading of the RM's Is_integral_validity
// invariant. NaN fails the equality on its own, but ±Inf would pass it
// (truncation is the identity there), so finiteness is checked too: an
// infinite operand is not a whole number.
func isIntegralReal(v rm.Real) bool {
	f := float64(v)
	return !math.IsInf(f, 0) && math.Trunc(f) == f
}

// checkTemporalValue enforces the REQ-112 Value_valid rule on the four ISO
// 8601-backed data values: a non-empty `value` must satisfy the type's BASE
// predicate, decided with the REQ-123 parse so the partial forms it admits
// and DV_DURATION's documented deviations stay valid. A placeholder such as
// "example" fails it. An empty `value`, which is also what a JSON null
// decodes to, is an absent RM-mandatory attribute: the mandatory-presence
// rule reports it once, as `required` at …/value ([findings.checkMandatory]),
// so this rule leaves it alone. The diagnostic names the attribute, never
// the offending value (REQ-168); that value string is carried in
// Issue.Value. The issue's rule is the Value_valid invariant that dispatched
// the check, the one the node's own class declares.
func (f *findings) checkTemporalValue(value any, rmType, path, rule string) {
	text, valid, ok := temporalValue(value)
	if !ok || valid || text == "" {
		return
	}
	f.emit(Issue{
		Path:   path,
		Code:   "rm_invariant",
		Layer:  LayerRM,
		Rule:   rule,
		Detail: rmType + ".value must be a valid ISO 8601 value (RM Value_valid)",
		Value:  constraints.Redact(text),
	})
}

// checkElementNullFlavour enforces the RM invariant
// `Inv_null_flavour_indicated: is_null() xor null_flavour = Void` on an
// ELEMENT. is_null() means there is no `value`, so exactly one of `value` and
// `null_flavour` must be present. A typed-nil `value` counts as absent, as it
// does everywhere else in the walk. The rule is reported on the ELEMENT.
func (f *findings) checkElementNullFlavour(value any, path string) {
	e, ok := asElement(value)
	if !ok {
		return
	}
	hasValue := e.Value != nil && !rmaccess.IsTypedNilPointer(e.Value)
	hasNullFlavour := e.NullFlavour != nil
	switch {
	case hasValue && hasNullFlavour:
		f.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleNullFlavourIndicated,
			Detail: "ELEMENT carries both value and null_flavour; exactly one must be present (RM Inv_null_flavour_indicated)",
		})
	case !hasValue && !hasNullFlavour:
		f.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleNullFlavourIndicated,
			Detail: "ELEMENT carries neither value nor null_flavour; exactly one must be present (RM Inv_null_flavour_indicated)",
		})
	}
}

// checkDVInterval enforces the spec floor on DV_INTERVAL. The boundary
// flags are checked on every interval ([findings.checkIntervalFlags]).
// The bound order is checked when neither side is unbounded and the two
// bounds are strictly comparable as the RM defines is_strictly_comparable_to
// for their type ([dvIntervalOrder]): lower MUST be ≤ upper by the RM's
// less_than for that type, except that two times or date-times in different
// time zones are ordered as instants. A pair the RM does not make strictly
// comparable is not ordered, nor is a time or date-time with a zone against
// one without.
//
// The Detail names the rule and the type of the bounds, never the bounds; the
// two compared magnitudes are carried in Issue.Value, lower first, as a
// [2]float64 (REQ-168), each in UTC for two values ordered as instants.
// rmType is the node's RM type, which the flag check names.
func (f *findings) checkDVInterval(value any, rmType, path string) {
	f.checkIntervalFlags(value, rmType, path)
	order, ok := dvIntervalOrder(value)
	if !ok || !order.inverted {
		return
	}
	f.emit(Issue{
		Path:   path,
		Code:   "rm_invariant",
		Layer:  LayerRM,
		Rule:   ruleIntervalLimits,
		Detail: "DV_INTERVAL<" + order.class + ">: lower must be ≤ upper",
		Value:  constraints.Redact(order.magnitudes),
	})
}

// checkIntervalFlags enforces BASE Interval's Lower_included_valid and
// Upper_included_valid (lower_unbounded implies not lower_included, and the
// same for upper). A side marked both unbounded and included is reported at
// the interval's own path, as lower_included_valid or upper_included_valid;
// both sides wrong give both issues. Decode reads such a side as written, and
// a value built in Go can carry it too, so the floor is where it is reported.
//
// The flags are read through [rmaccess.Single], which serves every class
// below BASE Interval: each DV_INTERVAL instantiation, the typed ones and the
// bare one, Point_interval and Proper_interval, as a pointer or a value. The
// Detail names the class of the node, rmType without its generic parameters,
// the flags and the invariant, never a bound. The check reads only the two
// flags, which the code already states, so the issue's Value stays empty.
func (f *findings) checkIntervalFlags(value any, rmType, path string) {
	class := bmmtype.Class(rmType)
	for _, side := range [...]struct{ end, code, invariant, rule string }{
		{end: "lower", code: "lower_included_valid", invariant: "Lower_included_valid", rule: ruleLowerIncludedValid},
		{end: "upper", code: "upper_included_valid", invariant: "Upper_included_valid", rule: ruleUpperIncludedValid},
	} {
		if !intervalFlagSet(value, side.end+"_unbounded") || !intervalFlagSet(value, side.end+"_included") {
			continue
		}
		f.emit(Issue{
			Path:   path,
			Code:   side.code,
			Layer:  LayerRM,
			Rule:   side.rule,
			Detail: class + "." + side.end + "_included must be false when " + side.end + "_unbounded is true (BASE " + side.invariant + ")",
		})
	}
}

// intervalFlagSet reports whether the boundary flag attr of the interval
// value is true. A value the access cannot read the flag from reads as false.
func intervalFlagSet(value any, attr string) bool {
	v, ok := rmaccess.Single(value, attr)
	set, isBool := v.(bool)
	return ok && isBool && set
}

// checkObjectRef enforces the spec floor on OBJECT_REF (and subtypes):
// id, type, and namespace are RM-mandatory. The walk goes below a reference
// like any node, but its mandatory-presence check leaves the three parts out
// on every class this evaluator runs on ([classRules.leavesOut]), with a
// template as without one, so this evaluator is the only check of a missing
// part, which keeps it from being reported twice. It reads the id
// as the walk does, through the generated access, which reads the id the
// class itself declares: a LOCATABLE_REF's own UID_BASED_ID, not the
// OBJECT_REF id it embeds. It reads type and namespace through the
// [rm.ObjectRefLike] interface (REQ-052), so any BMM subtype is covered.
func (f *findings) checkObjectRef(value any, path string) {
	if value == nil || rmaccess.IsTypedNilPointer(value) {
		return
	}
	ref, ok := value.(rm.ObjectRefLike)
	if !ok {
		return
	}
	if _, present := rmaccess.Single(value, "id"); !present {
		f.emit(Issue{
			Path:   joinPath(path, "/id"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: "OBJECT_REF.id must be present",
		})
	}
	if ref.GetType() == "" {
		f.emit(Issue{
			Path:   joinPath(path, "/type"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: "OBJECT_REF.type must be non-empty",
		})
	}
	if ref.GetNamespace() == "" {
		f.emit(Issue{
			Path:   joinPath(path, "/namespace"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: "OBJECT_REF.namespace must be non-empty",
		})
	}
}

// checkPartyIdentified enforces the three REQ-112 invariants PARTY_IDENTIFIED
// declares, and PARTY_RELATED inherits:
//
//   - Basic_validity: at least one of name, identifiers and external_ref is
//     present, reported on the node;
//   - Name_valid: a present name is not empty, reported at name;
//   - Identifiers_valid: a present identifiers is not empty, reported at
//     identifiers.
//
// Presence is read from Go nilness, as for DV_TEXT mappings (see
// [findings.checkTermMappings]): an absent key and an explicit JSON
// `null` both decode to a nil pointer or a nil slice and read as absent, so
// only a decoded `"name":""` or `"identifiers":[]` is present and empty. A
// present external_ref is walked as its own node, where checkObjectRef
// checks its parts. PARTY_RELATED's Relationship_valid, a terminology-group
// rule, is not checked here but with the other coded invariants
// ([findings.checkCodedInvariants]).
//
// Diagnostics name the attribute and the RM rule, never the offending value
// (REQ-168).
func (f *findings) checkPartyIdentified(value any, path string) {
	p, ok := asPartyIdentified(value)
	if !ok {
		return
	}
	if p.Name == nil && p.Identifiers == nil && p.ExternalRef == nil {
		f.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   rulePartyBasicValidity,
			Detail: "PARTY_IDENTIFIED has none of name, identifiers and external_ref; at least one must be present (RM Basic_validity)",
		})
	}
	if p.Name != nil && *p.Name == "" {
		f.emit(Issue{
			Path:   joinPath(path, "/name"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   rulePartyNameValid,
			Detail: "PARTY_IDENTIFIED.name is present but empty (RM Name_valid)",
		})
	}
	if p.Identifiers != nil && len(p.Identifiers) == 0 {
		f.emit(Issue{
			Path:   joinPath(path, "/identifiers"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   rulePartyIdentifiersValid,
			Detail: "PARTY_IDENTIFIED.identifiers is present but empty (RM Identifiers_valid)",
		})
	}
}

// checkTermMappings enforces the REQ-112 Mappings_valid invariant on the
// `mappings` attribute DV_TEXT carries (and DV_CODED_TEXT inherits via its
// embedded DV_TEXT): a *present* mappings MUST be non-empty.
//
// Presence is read from Go slice nilness, which mirrors the wire
// distinction canjson decode preserves: an absent `mappings` key and an
// explicit JSON `null` both decode to a nil slice (pinned by
// canjson's TestDVTextMappingsDecodePresenceAndEncodeCollapse), so they
// are indistinguishable from "not supplied" and both valid; only a decoded
// literal `"mappings":[]` produces the non-nil empty slice this check
// flags. The element-level match check is NOT here — `mappings` is a
// walked container, so each TERM_MAPPING is a node of
// its own and [findings.checkTermMapping] evaluates it there.
//
// Diagnostics name the attribute only, never a value from the instance
// (REQ-168). The check reads no value, only the list's length, so the
// issue's Value stays empty.
func (f *findings) checkTermMappings(value any, path string) {
	ms, ok := asMappings(value)
	if !ok {
		return
	}
	if ms != nil && len(ms) == 0 {
		f.emit(Issue{
			Path:   joinPath(path, "/mappings"),
			Code:   "mappings_valid",
			Layer:  LayerRM,
			Rule:   ruleMappingsValid,
			Detail: "DV_TEXT.mappings present but empty",
		})
	}
}

// checkTermMapping enforces the REQ-112 value-set invariant on a
// TERM_MAPPING node: `match` MUST be one of '>', '=', '<', '?' (openEHR RM
// Data Types). It runs on the node itself, so it fires wherever a mapping
// sits — under a DV_TEXT / DV_CODED_TEXT `mappings` container, nested under
// a mapping's own `purpose` (a DV_CODED_TEXT carrying further mappings), or
// as the validated root.
//
// The diagnostic names the attribute and the allowed set only — never the
// offending value (REQ-168); the match is carried in Issue.Value.
func (f *findings) checkTermMapping(value any, path string) {
	m, ok := asTermMapping(value)
	if !ok {
		return
	}
	switch m.Match {
	case ">", "=", "<", "?":
	default:
		f.emit(Issue{
			Path:   joinPath(path, "/match"),
			Code:   "term_mapping_match",
			Layer:  LayerRM,
			Rule:   ruleTermMappingMatch,
			Detail: "TERM_MAPPING.match must be one of {'>', '=', '<', '?'}",
			Value:  constraints.Redact(m.Match),
		})
	}
}

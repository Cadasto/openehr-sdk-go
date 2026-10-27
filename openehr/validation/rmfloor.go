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
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
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
// visits, and reports `max_nodes` once, at the path where it stopped. A
// large decoded document can reach that limit too. A graph nested deeper than
// the walk's depth limit is not walked below it; the walk reports
// `max_depth` once, at the first path where it stopped. [Result.Coverage]
// lists the path of each of the two, and the [Limits] of a [Validator] set
// both.
//
// A member held by value, a number, a boolean or a value struct such as an
// EHR_STATUS subject or an ARCHETYPED archetype_id, holds its zero value
// whether the document left it out or gave that value, so the value alone
// cannot say it is missing. A caller that decoded root with a codec's report
// of missing members passes it as [WithMissing], and each member the report
// names is read as absent: an omitted magnitude, subject or rm_version is
// then `required` at its path. Without the option, such a member reads as
// present. [ValidateJSON] and [ValidateXML] decode a document and validate
// it with its report in one call.
//
// ValidateRM returns what a [Validator] built with no options returns from
// Validate(root, nil, opts...).
//
// A nil root surfaces a single `nil_root` issue with an empty path and is
// reported as not-OK. An unknown RM root type (a Go value outside the
// closed RM set) surfaces `rm_type_unknown` at "/"; the walk cannot descend
// further but does not panic.
func ValidateRM(root any, opts ...ValidateOption) Result {
	var v Validator
	return v.Validate(root, nil, opts...)
}

// rmRootGuard returns the RM type of root, the type the RM rules alone read
// it as, with ok set. For a nil root, and for one outside the closed RM set,
// it returns instead the result those rules report for it, a single
// `nil_root` or `rm_type_unknown` issue whose detail names the entry, with
// ok unset.
func rmRootGuard(entry string, root any) (rmType string, guard Result, ok bool) {
	if root == nil || rmaccess.IsTypedNilPointer(root) {
		return "", resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_root",
			Detail:   entry + ": root is nil",
			Severity: Error,
		}}), false
	}
	rmType, _, ok = rmTypeInfo(root)
	if !ok {
		return "", resultFromIssues([]Issue{{
			Path:     "/",
			Code:     "rm_type_unknown",
			Detail:   fmt.Sprintf("%s: root Go type %T is outside the closed RM set; cannot descend", entry, root),
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
// It reads the status as the Go value holds it, so it cannot flag an
// omitted `subject` (typed rm.PartySelf, whose zero value is the valid bare
// PARTY_SELF), and it reads an ARCHETYPED's omitted `rm_version` as an empty
// one, `rm_version_valid`, and an omitted `archetype_id` as one with an empty
// value, `required` at `archetype_id/value`. A caller that decodes the status
// validates it with [ValidateJSON] or [ValidateXML], or passes the codec's
// report to [ValidateRM] with [WithMissing]: an omitted subject is then
// `required` at `/subject`, an omitted rm_version `required` at its path,
// and an omitted archetype_id `required` at `archetype_id` alone.
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
func (f *findings) checkMandatory(rmType, attr string, attrPath issuePath, multiValued, present bool, items int) {
	switch {
	case !present && multiValued:
		f.emit(Issue{
			Path:   attrPath.String(),
			Code:   "required",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: fmt.Sprintf("RM-mandatory multi-valued attribute %q absent on %s", attr, rmType),
		})
	case !present:
		f.emit(Issue{
			Path:   attrPath.String(),
			Code:   "required",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: fmt.Sprintf("RM-mandatory attribute %q absent on %s", attr, rmType),
		})
	case multiValued && items == 0:
		f.emit(Issue{
			Path:   attrPath.String(),
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
// calls it for each such item of a list no template node binds that is
// within the visit limit, and does not walk the item; the item still counts
// toward its list, so a list that holds only such items is present and not
// empty, and toward the visit limit as one visit. A nil item the template
// refuses is reported by the template's rule alone.
func (f *findings) checkNilItem(rmType, attr string, itemPath issuePath) {
	f.emit(Issue{
		Path:   itemPath.String(),
		Code:   "required",
		Layer:  LayerRM,
		Rule:   ruleMandatory,
		Detail: fmt.Sprintf("item of %q on %s holds no RM value", attr, rmType),
	})
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

// checkInvariants runs rules, the RM rules of the class of the node value,
// of RM type rmType, on the node at path, and records the class as visited.
// The rules are the inventory's ([floorRules]), which the walk finds with
// the class's attributes in one lookup ([classOf]): the evaluators of the
// evaluated invariants the class and its ancestors declare, the coded ones
// included, and the attribute checks that apply to the class. Nil rules,
// those of a type no registered class has, run nothing (this is a floor, not
// an exhaustive RM check).
func (f *findings) checkInvariants(value any, rmType string, path issuePath, rules *classRules) {
	if rules == nil {
		return
	}
	f.visited = f.visited.with(rules.index)
	n := floorNode{value: value, rmType: rmType, path: path}
	for _, c := range rules.checks {
		c.run(f, n, c.rule)
	}
	if len(rules.coded) > 0 {
		f.checkCodedInvariants(value, rmType, path, rules.coded)
	}
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
func (f *findings) checkArchetypeRoot(value any, rmType string, path issuePath, rule string) {
	l, ok := value.(rm.Locatable)
	if !ok || rmaccess.IsTypedNilPointer(value) {
		return
	}
	if l.GetArchetypeDetails() == nil {
		f.emit(Issue{
			Path:   path.join("/archetype_details"),
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
func (f *findings) checkArchetyped(value any, path issuePath) {
	a, ok := asArchetyped(value)
	if !ok {
		return
	}
	if a.ArchetypeID.Value == "" {
		f.emit(Issue{
			Path:   path.join("/archetype_id/value"),
			Code:   "required",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: "ARCHETYPED.archetype_id must carry a non-empty value",
		})
	}
	if a.RMVersion == "" {
		f.emit(Issue{
			Path:   path.join("/rm_version"),
			Code:   "rm_version_valid",
			Layer:  LayerRM,
			Rule:   ruleRMVersionValid,
			Detail: "ARCHETYPED.rm_version must be non-empty (RM Rm_version_valid)",
		})
	}
}

// scaleSymbolMayHaveNoCode reports whether a CODE_PHRASE in role may carry a
// blank code_string. The RM's DV_SCALE.symbol allows a scale value that has
// no code: its symbol is then a DV_CODED_TEXT carrying the terminology_id and
// a blank code_string. Elsewhere a blank code_string draws one finding, the
// `required` of the mandatory-presence rule, which decides the non-empty
// code_string invariant; the CODE_PHRASE at the symbol's defining_code draws
// none. The exemption covers that one node only: a blank terminology_id, the
// symbol's own value, a DV_ORDINAL symbol, and every other CODE_PHRASE under
// the scale are checked as usual. The walk hands the role down from the
// DV_SCALE ([childRole]), so the exemption follows the node and not its
// path, which another node may share.
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
func (f *findings) checkDVQuantity(value any, path issuePath) {
	q, ok := asDVQuantity(value)
	if !ok {
		return
	}
	if q.Precision != nil && int(*q.Precision) < -1 {
		f.emit(Issue{
			Path:   path.String(),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   rulePrecisionRange,
			Detail: "DV_QUANTITY.precision must be ≥ -1 (-1 = no limit)",
			Value:  constraints.Redact(*q.Precision),
		})
	}
}

// checkDVProportion enforces the RM rules on a DV_PROPORTION, in the order
// the BMM declares its invariants:
//
//   - Type_validity: `valid_proportion_kind (type)`, so type is a
//     PROPORTION_KIND, 0 (pk_ratio) to 4 (pk_integer_fraction).
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
//   - Fraction_validity: `(type = pk_fraction or type =
//     pk_integer_fraction) implies is_integral`, with is_integral read as
//     for Precision_validity, so a fraction or an integer fraction requires
//     whole-number operands, whatever its precision.
//   - Unitary_validity and Percent_validity: a unitary proportion has
//     denominator 1, and a percent denominator 100.
//   - Valid_denominator: `denominator /= 0.0`, on every kind.
//
// Each rule the value breaks is one issue of its own, with the rule as
// Rule, so one value can draw several: a fraction of precision 0 over a
// fractional operand breaks Precision_validity and Fraction_validity, and a
// percent over denominator 0 Percent_validity and Valid_denominator. No rule
// is reported twice: Is_integral_validity is decided by the integrality arm,
// and an integrality rule names every operand that is not whole in its one
// issue. A type that is no proportion kind draws Type_validity alone of the
// rules keyed on the kind.
//
// Diagnostics name the attribute, the rule and the offending operand, never
// a value from the instance (REQ-168). The value the check read is carried
// in Issue.Value: the precision on the range arm; the type on
// Type_validity; the denominator on Unitary_validity, Percent_validity and
// Valid_denominator; and the numerator and the denominator, as a
// [2]float64, on Precision_validity and Fraction_validity.
func (f *findings) checkDVProportion(value any, path issuePath) {
	p, ok := asDVProportion(value)
	if !ok {
		return
	}
	kind := rm.ProportionKind(p.Type)
	if !isProportionKind(kind) {
		f.emit(Issue{
			Path:   path.String(),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleProportionType,
			Detail: "DV_PROPORTION.type must be a PROPORTION_KIND, 0 to 4 (RM Type_validity)",
			Value:  constraints.Redact(p.Type),
		})
	}
	f.checkProportionPrecision(p, path)
	if kind == rm.ProportionKindPkFraction || kind == rm.ProportionKindPkIntegerFraction {
		if fractional := nonIntegralOperands(p); fractional != "" {
			f.emit(Issue{
				Path:   path.String(),
				Code:   "rm_invariant",
				Layer:  LayerRM,
				Rule:   ruleProportionFraction,
				Detail: "DV_PROPORTION.type is a fraction kind, which requires a whole-number numerator and denominator (RM Fraction_validity); not integral: " + fractional,
				Value:  constraints.Redact([2]float64{float64(p.Numerator), float64(p.Denominator)}),
			})
		}
	}
	if kind == rm.ProportionKindPkUnitary && p.Denominator != 1 {
		f.emit(Issue{
			Path:   path.String(),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleProportionUnitary,
			Detail: "DV_PROPORTION.denominator must be 1 on a unitary proportion (RM Unitary_validity)",
			Value:  constraints.Redact(p.Denominator),
		})
	}
	if kind == rm.ProportionKindPkPercent && p.Denominator != 100 {
		f.emit(Issue{
			Path:   path.String(),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleProportionPercent,
			Detail: "DV_PROPORTION.denominator must be 100 on a percent (RM Percent_validity)",
			Value:  constraints.Redact(p.Denominator),
		})
	}
	if p.Denominator == 0 {
		f.emit(Issue{
			Path:   path.String(),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleProportionDenominator,
			Detail: "DV_PROPORTION.denominator must not be 0 (RM Valid_denominator)",
			Value:  constraints.Redact(p.Denominator),
		})
	}
}

// checkProportionPrecision enforces the two precision arms of
// [findings.checkDVProportion] on p, when its precision is set.
func (f *findings) checkProportionPrecision(p rm.DVProportion, path issuePath) {
	if p.Precision == nil {
		return
	}
	switch prec := int(*p.Precision); {
	case prec < -1:
		f.emit(Issue{
			Path:   path.String(),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   rulePrecisionRange,
			Detail: "DV_PROPORTION.precision must be ≥ -1 (-1 = no limit)",
			Value:  constraints.Redact(*p.Precision),
		})
	case prec == 0:
		if fractional := nonIntegralOperands(p); fractional != "" {
			f.emit(Issue{
				Path:   path.String(),
				Code:   "rm_invariant",
				Layer:  LayerRM,
				Rule:   ruleProportionPrecision,
				Detail: "DV_PROPORTION.precision is 0, which requires a whole-number numerator and denominator (RM Precision_validity); not integral: " + fractional,
				Value:  constraints.Redact([2]float64{float64(p.Numerator), float64(p.Denominator)}),
			})
		}
	}
}

// isProportionKind reports whether k is one of the five PROPORTION_KIND
// values, the RM's valid_proportion_kind.
func isProportionKind(k rm.ProportionKind) bool {
	switch k {
	case rm.ProportionKindPkRatio, rm.ProportionKindPkUnitary, rm.ProportionKindPkPercent,
		rm.ProportionKindPkFraction, rm.ProportionKindPkIntegerFraction:
		return true
	}
	return false
}

// nonIntegralOperands names the operands of p that are not whole numbers
// ([isIntegralReal]), "numerator", "denominator" or both joined by a comma,
// or returns "" when both are whole.
func nonIntegralOperands(p rm.DVProportion) string {
	var fractional []string
	if !isIntegralReal(p.Numerator) {
		fractional = append(fractional, "numerator")
	}
	if !isIntegralReal(p.Denominator) {
		fractional = append(fractional, "denominator")
	}
	return strings.Join(fractional, ", ")
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
func (f *findings) checkTemporalValue(value any, rmType string, path issuePath, rule string) {
	text, valid, ok := temporalValue(value)
	if !ok || valid || text == "" {
		return
	}
	f.emit(Issue{
		Path:   path.String(),
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
func (f *findings) checkElementNullFlavour(value any, path issuePath) {
	e, ok := asElement(value)
	if !ok {
		return
	}
	hasValue := e.Value != nil && !rmaccess.IsTypedNilPointer(e.Value)
	hasNullFlavour := e.NullFlavour != nil
	switch {
	case hasValue && hasNullFlavour:
		f.emit(Issue{
			Path:   path.String(),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleNullFlavourIndicated,
			Detail: "ELEMENT carries both value and null_flavour; exactly one must be present (RM Inv_null_flavour_indicated)",
		})
	case !hasValue && !hasNullFlavour:
		f.emit(Issue{
			Path:   path.String(),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleNullFlavourIndicated,
			Detail: "ELEMENT carries neither value nor null_flavour; exactly one must be present (RM Inv_null_flavour_indicated)",
		})
	}
}

// checkDVInterval enforces the spec floor on DV_INTERVAL. The boundary
// flags are checked on every interval ([findings.checkIntervalFlags]).
// When neither side is unbounded and each holds a bound
// ([intervalBounds]), the bounds are checked against RM
// DV_INTERVAL.Limits_consistent, lower.is_strictly_comparable_to(upper) and
// lower <= upper. A pair the floor reads as not strictly comparable (bounds
// of two classes, two DV_QUANTITY in different units, two DV_PROPORTION of
// different types: [notStrictlyComparable]) breaks the rule whatever its
// order, and is reported once, with no Value. A strictly comparable pair
// MUST have lower ≤ upper by the RM's less_than for its type, except that two
// times or date-times in different time zones are ordered as instants
// ([orderBounds]). A strictly comparable pair the floor has no order for,
// such as a time or date-time with a zone against one without, is not
// reported.
//
// Each Detail names the rule and the type of the bounds, never the bounds;
// for an out-of-order pair the two compared magnitudes are carried in
// Issue.Value, lower first, as a [2]float64 (REQ-168), each in UTC for two
// values ordered as instants. rmType is the node's RM type, which the flag
// check names. Interval.Limits_comparable, which BASE declares and
// DV_INTERVAL.Limits_consistent restates, is not evaluated on the BASE
// intervals ([intervalBounds] reads no Proper_interval or Point_interval),
// so Coverage lists it.
func (f *findings) checkDVInterval(value any, rmType string, path issuePath) {
	f.checkIntervalFlags(value, rmType, path)
	lower, upper, bounded := intervalBounds(value)
	if !bounded {
		return
	}
	if detail, apart := notStrictlyComparable(lower, upper); apart {
		f.emit(Issue{
			Path:   path.String(),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleIntervalLimits,
			Detail: detail,
		})
		return
	}
	order, ok := orderBounds(lower, upper)
	if !ok || !order.inverted {
		return
	}
	f.emit(Issue{
		Path:   path.String(),
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
func (f *findings) checkIntervalFlags(value any, rmType string, path issuePath) {
	class := bmmtype.Class(rmType)
	for _, side := range [...]struct{ end, code, invariant, rule string }{
		{end: "lower", code: "lower_included_valid", invariant: "Lower_included_valid", rule: ruleLowerIncludedValid},
		{end: "upper", code: "upper_included_valid", invariant: "Upper_included_valid", rule: ruleUpperIncludedValid},
	} {
		if !intervalFlagSet(value, side.end+"_unbounded") || !intervalFlagSet(value, side.end+"_included") {
			continue
		}
		f.emit(Issue{
			Path:   path.String(),
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
func (f *findings) checkObjectRef(value any, path issuePath) {
	if value == nil || rmaccess.IsTypedNilPointer(value) {
		return
	}
	ref, ok := value.(rm.ObjectRefLike)
	if !ok {
		return
	}
	if _, present := rmaccess.Single(value, "id"); !present {
		f.emit(Issue{
			Path:   path.join("/id"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: "OBJECT_REF.id must be present",
		})
	}
	if ref.GetType() == "" {
		f.emit(Issue{
			Path:   path.join("/type"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: "OBJECT_REF.type must be non-empty",
		})
	}
	if ref.GetNamespace() == "" {
		f.emit(Issue{
			Path:   path.join("/namespace"),
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
func (f *findings) checkPartyIdentified(value any, path issuePath) {
	p, ok := asPartyIdentified(value)
	if !ok {
		return
	}
	if p.Name == nil && p.Identifiers == nil && p.ExternalRef == nil {
		f.emit(Issue{
			Path:   path.String(),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   rulePartyBasicValidity,
			Detail: "PARTY_IDENTIFIED has none of name, identifiers and external_ref; at least one must be present (RM Basic_validity)",
		})
	}
	if p.Name != nil && *p.Name == "" {
		f.emit(Issue{
			Path:   path.join("/name"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   rulePartyNameValid,
			Detail: "PARTY_IDENTIFIED.name is present but empty (RM Name_valid)",
		})
	}
	if p.Identifiers != nil && len(p.Identifiers) == 0 {
		f.emit(Issue{
			Path:   path.join("/identifiers"),
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
func (f *findings) checkTermMappings(value any, path issuePath) {
	ms, ok := asMappings(value)
	if !ok {
		return
	}
	if ms != nil && len(ms) == 0 {
		f.emit(Issue{
			Path:   path.join("/mappings"),
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
func (f *findings) checkTermMapping(value any, path issuePath) {
	m, ok := asTermMapping(value)
	if !ok {
		return
	}
	switch m.Match {
	case ">", "=", "<", "?":
	default:
		f.emit(Issue{
			Path:   path.join("/match"),
			Code:   "term_mapping_match",
			Layer:  LayerRM,
			Rule:   ruleTermMappingMatch,
			Detail: "TERM_MAPPING.match must be one of {'>', '=', '<', '?'}",
			Value:  constraints.Redact(m.Match),
		})
	}
}

// checkLinks enforces the RM invariant `Links_valid: links /= Void implies
// not links.is_empty` (REQ-112) on a LOCATABLE node of any class: a `links`
// that is present and holds no item is reported at the node's `links`.
// Presence is the generated access's reading (REQ-171 Structural access): a
// list is present when it is not nil, so an absent key and JSON null both
// read as absent, and only a decoded `[]` is present and empty. The
// invariant is not one of the REQ-112 Non-empty lists, so a template's
// existence on `links` asks for no member. The diagnostic names the class
// and the attribute, never a value (REQ-168); the check reads only the
// list's length, so the issue's Value stays empty.
func (f *findings) checkLinks(value any, rmType string, path issuePath) {
	if n, present := rmaccess.Len(value, "links"); present && n == 0 {
		f.emit(Issue{
			Path:   path.join("/links"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleLinksValid,
			Detail: bareClass(rmType) + ".links is present but holds no item (RM Links_valid)",
		})
	}
}

// checkEventContextLocation enforces the RM invariant `location_valid:
// location /= Void implies not location.is_empty` (REQ-112) on an
// EVENT_CONTEXT: a `location` that is present and empty is reported at the
// context's `location`. Presence is the generated access's reading: a
// pointer to a string is present when it is not nil, so an absent key and
// JSON null both read as absent, and only a decoded `""` is present and
// empty. The diagnostic names the class and the attribute, never a value
// (REQ-168); an empty string holds none, so the issue's Value stays empty.
func (f *findings) checkEventContextLocation(value any, path issuePath) {
	v, present := rmaccess.Single(value, "location")
	if !present {
		return
	}
	if s, ok := v.(*string); ok && s != nil && *s == "" {
		f.emit(Issue{
			Path:   path.join("/location"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleLocationValid,
			Detail: "EVENT_CONTEXT.location is present but empty (RM location_valid)",
		})
	}
}

// ehrScheme is the scheme a DV_EHR_URI carries, the RM's Ehr_scheme.
const ehrScheme = "ehr"

// checkDVEHRURIScheme enforces the RM invariant `Scheme_valid:
// scheme.is_equal (Ehr_scheme)` (REQ-112) on a DV_EHR_URI: a `value` that
// has an RFC 3986 scheme ([uriScheme]) other than `ehr`, compared without
// regard to letter case, is reported at the node itself, as the ISO 8601
// rule reports at its node. A value with no scheme has none to compare, so
// it is not reported here, and an empty one is the mandatory-presence
// rule's. The diagnostic names the class and the attribute, never a value
// (REQ-168), the scheme being part of the value, so the issue's Value stays
// empty.
func (f *findings) checkDVEHRURIScheme(value any, path issuePath) {
	u, ok := asDVEHRURI(value)
	if !ok {
		return
	}
	scheme, found := uriScheme(u.Value)
	if !found || strings.EqualFold(scheme, ehrScheme) {
		return
	}
	f.emit(Issue{
		Path:   path.String(),
		Code:   "rm_invariant",
		Layer:  LayerRM,
		Rule:   ruleEHRSchemeValid,
		Detail: "DV_EHR_URI.value must use the ehr scheme (RM Scheme_valid)",
	})
}

// uriScheme returns the scheme of the URI reference s as RFC 3986 section
// 3.1 writes it, `ALPHA *( ALPHA / DIGIT / "+" / "-" / "." )`, followed by
// ":", and whether s has one. A reference whose text before the first colon
// is no scheme, such as a relative reference, a path with a colon after a
// slash, or one that starts with a digit, has none.
func uriScheme(s string) (scheme string, found bool) {
	scheme, _, found = strings.Cut(s, ":")
	if !found || scheme == "" {
		return "", false
	}
	for i := range len(scheme) {
		if !isSchemeByte(scheme[i], i == 0) {
			return "", false
		}
	}
	return scheme, true
}

// isSchemeByte reports whether c may stand in an RFC 3986 scheme: a letter
// anywhere, and, past the first byte, a digit, "+", "-" or ".".
func isSchemeByte(c byte, first bool) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		return true
	case first:
		return false
	}
	return '0' <= c && c <= '9' || c == '+' || c == '-' || c == '.'
}

// listMembers is what an RM rule that asks a list for a member reads: the
// list, and, for a rule that a second attribute also keeps, that attribute,
// whose presence keeps the rule while the list holds no item.
type listMembers struct {
	list, alternative string
}

// listMemberRules names the list each RM invariant of the REQ-112 Non-empty
// lists entry reads, keyed CLASS.Invariant as the inventory keys it. The
// lists are the ones [rminfo.NonEmptyLists] names, which the generator reads
// too, so the two cannot drift apart. Five read `x /= Void implies not
// x.is_empty` for their list x, so a list that is present holds a member;
// HISTORY Events_valid reads `(events /= Void and then not events.is_empty)
// or summary /= Void`, so a HISTORY carries an event or a summary. The rules
// are evaluated by [findings.checkListMembers], and a class the rules of one
// of them run on asks the template's existence on its list for a member
// ([classRules.asksForMember]).
var listMemberRules = listMemberRulesOf(rminfo.NonEmptyLists())

// listMemberRulesOf keys each of lists by its class and invariant,
// CLASS.Invariant, with the list it reads and its alternative.
func listMemberRulesOf(lists []rminfo.NonEmptyList) map[string]listMembers {
	rules := make(map[string]listMembers, len(lists))
	for _, l := range lists {
		rules[l.Class+"."+l.Invariant] = listMembers{list: l.List, alternative: l.Alternative}
	}
	return rules
}

// checkListMembers enforces the REQ-112 Non-empty lists rule that
// dispatched it, rule, on the node value at path, of RM type rmType. A rule
// that reads its list alone reports the list when it is present and holds
// no item, at the list's path; an absent list keeps it. A rule that a
// second attribute keeps too, HISTORY Events_valid, reports the node, at
// its own path, when its list is absent or holds no item and the second
// attribute is absent, as the floor reports PARTY_IDENTIFIED Basic_validity
// and ELEMENT Inv_null_flavour_indicated, the other rules over more than one
// attribute. Presence is the generated access's reading (REQ-171 Structural
// access): a list is present when it is not nil, so only a decoded `[]` is
// present and empty, and an absent key and JSON null both read as absent.
//
// Under a template the finding gives way to the template's report of the
// same list, while it holds no item, at the list's path or below it, and,
// for a HISTORY's events, to its report of the summary
// ([engine.withdrawGivenWay]). The diagnostic
// names the class and the attributes, never a value (REQ-168).
func (f *findings) checkListMembers(value any, rmType string, path issuePath, rule string) {
	m, ok := listMemberRules[rule]
	if !ok {
		return
	}
	n, present := rmaccess.Len(value, m.list)
	if m.alternative == "" {
		if present && n == 0 {
			f.emit(Issue{
				Path:   path.join("/" + m.list),
				Code:   "rm_invariant",
				Layer:  LayerRM,
				Rule:   rule,
				Detail: fmt.Sprintf("%s.%s is present but holds no item (RM %s)", bareClass(rmType), m.list, rule),
			})
		}
		return
	}
	if n > 0 {
		return
	}
	if _, kept := rmaccess.Single(value, m.alternative); kept {
		return
	}
	f.emit(Issue{
		Path:   path.String(),
		Code:   "rm_invariant",
		Layer:  LayerRM,
		Rule:   rule,
		Detail: fmt.Sprintf("%s has no item in %s and no %s; one of them must be present (RM %s)", bareClass(rmType), m.list, m.alternative, rule),
	})
}

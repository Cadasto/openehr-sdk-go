package validation

// rmfloor.go: REQ-112 — the template-less Reference Model validation
// floor. ValidateRM walks an RM root using the BMM (via rminfo) as the
// driver — no compiled OPT — and reports:
//
//   - RM-mandatory attribute absences (rminfo.RequiredAttributes per type
//     plus the container "lower bound ≥ 1" reading);
//   - the per-RM-type invariant catalogue of § REQ-112 in
//     docs/specifications/clinical-modeling.md, on every node it reaches.
//     checkInvariants dispatches it, one evaluator per catalogue row, and
//     checkCodedInvariants (rmfloor_coded.go) runs the Coded invariants
//     entry beside it; the catalogue, not this comment, is the list to keep
//     current.
//
// REQ-112 surface. Independent of REQ-102/110 (template-driven); both
// drivers may run against the same root — REQ-110 enforces template
// constraints, REQ-112 enforces the RM-only floor.
//
// Sign-off (2026-06-29): Option A — second driver alongside the
// template-driven walker. Walker + invariant evaluators live in this
// file; the closed RM-type set is shared with the template-driven path
// via the existing rmTypeInfo/describeRMType helpers (composition.go).

import (
	"fmt"
	"math"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/rmroots"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// maxWalkDepth bounds the RM-floor descent. RM graphs decoded from the
// wire are acyclic trees, but ValidateRM accepts any caller-built value;
// a pathological cyclic graph is stopped here rather than overflowing the
// stack. The deepest legitimate RM nesting is far below this bound.
const maxWalkDepth = 256

// ValidateRM validates root against the openEHR Reference Model alone.
// It checks every RM-mandatory attribute on every node
// reachable from root, and runs per-RM-type invariants on the leaves it
// touches. It does not consult any operational template; use
// [Validate] / [ValidateComposition] / [ValidateFolder] / [ValidateEHRStatus]
// / [ValidateDemographic] when a compiled OPT is available.
//
// The invariants include the RM's coded ones: an attribute the RM codes from
// a group or a code set of the openEHR terminology, such as a COMPOSITION's
// language and territory, an EVENT_CONTEXT's setting, an ENTRY's language
// and encoding, an ELEMENT's null_flavour, a DV_ORDERED's normal_status or
// a PARTY_RELATED's relationship, must hold a member of it. The
// participations of an ENTRY or an EVENT_CONTEXT are read too: their
// function and mode, and the relationship of a performer that is a
// PARTY_RELATED. Groups and code sets come from
// [github.com/cadasto/openehr-sdk-go/openehr/terminology]. A group code must
// also be coded in the openEHR terminology itself; a code-set code is
// matched alone, ignoring letter case for the ISO and IANA sets. An optional
// attribute is checked only when present, and a mandatory one always, so an
// empty one is reported too. A breach is reported as `code_not_in_value_set`
// at the CODE_PHRASE read: the attribute itself, or its defining_code for a
// coded text. The detail names the attribute, the RM invariant and the
// group or code set, never the code. The coded rules of the change-control
// classes (VERSION, AUDIT_DETAILS, ATTESTATION) are not checked, nor those of
// EXTRACT_PARTICIPATION, a class the SDK's RM types leave out.
//
// A nil root surfaces a single `nil_root` issue and is reported as
// not-OK. An unknown RM root type (a Go value outside the closed RM
// set) surfaces `rm_type_unknown` at "/"; the floor cannot descend
// further but does not panic.
func ValidateRM(root any) Result {
	if root == nil || rmread.IsTypedNilPointer(root) {
		return resultFromIssues([]Issue{{
			Path:     "/",
			Code:     "nil_root",
			Detail:   "ValidateRM: root is nil",
			Severity: Error,
		}})
	}
	rmType, _, ok := rmTypeInfo(root)
	if !ok {
		return resultFromIssues([]Issue{{
			Path:     "/",
			Code:     "rm_type_unknown",
			Detail:   fmt.Sprintf("ValidateRM: root Go type %T is outside the closed RM set; cannot descend", root),
			Severity: Error,
		}})
	}
	w := &rmFloorWalker{info: rminfo.Default}
	w.walk(root, rmType, "/", 0)
	return resultFromIssues(w.issues)
}

// ValidateRMFolder is the typed convenience wrapper for FOLDER roots.
// Delegates to [ValidateRM]; a nil folder surfaces `nil_folder`.
func ValidateRMFolder(folder *rm.Folder) Result {
	if folder == nil {
		return resultFromIssues([]Issue{{Path: "/", Code: "nil_folder", Detail: "ValidateRMFolder: folder is nil", Severity: Error}})
	}
	return ValidateRM(folder)
}

// ValidateRMEHRStatus is the typed convenience wrapper for EHR_STATUS.
// Delegates to [ValidateRM]; a nil status surfaces `nil_ehr_status`.
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
		return resultFromIssues([]Issue{{Path: "/", Code: "nil_ehr_status", Detail: "ValidateRMEHRStatus: status is nil", Severity: Error}})
	}
	return ValidateRM(status)
}

// ValidateRMEHRAccess is the typed convenience wrapper for EHR_ACCESS.
// Delegates to [ValidateRM]; a nil access surfaces `nil_ehr_access`.
func ValidateRMEHRAccess(access *rm.EHRAccess) Result {
	if access == nil {
		return resultFromIssues([]Issue{{Path: "/", Code: "nil_ehr_access", Detail: "ValidateRMEHRAccess: access is nil", Severity: Error}})
	}
	return ValidateRM(access)
}

// ValidateRMDemographic is the typed convenience wrapper for the
// demographic PARTY hierarchy (PERSON / ORGANISATION / GROUP / AGENT /
// ROLE). Delegates to [ValidateRM]; a nil party surfaces `nil_party`.
func ValidateRMDemographic(party rm.Party) Result {
	if party == nil || rmread.IsTypedNilPointer(party) {
		return resultFromIssues([]Issue{{Path: "/", Code: "nil_party", Detail: "ValidateRMDemographic: party is nil", Severity: Error}})
	}
	return ValidateRM(party)
}

// rmFloorWalker accumulates issues as it descends the RM graph driven
// by rminfo. There is no notion of "OPT-declared attribute" here —
// every BMM-known attribute is a descend candidate. Recursion depth is
// bounded by maxWalkDepth so a pathological cyclic in-memory graph
// terminates instead of overflowing the stack.
type rmFloorWalker struct {
	info   rminfo.Lookup
	issues []Issue
	// blankCodeAllowed holds the paths of the CODE_PHRASE nodes whose
	// code_string may be blank: the defining_code of each DV_SCALE symbol
	// the walk has reached (see [rmFloorWalker.allowScaleSymbolWithoutCode]).
	blankCodeAllowed map[string]bool
}

func (w *rmFloorWalker) emit(i Issue) {
	if i.Severity == 0 {
		i.Severity = Error
	}
	w.issues = append(w.issues, i)
}

// walk descends value (of declared BMM type rmType) at the given AQL
// path. It first runs the per-type invariants on the current node, then
// — for types rmread models — iterates every BMM-known attribute,
// emitting `required` for any RM-mandatory attribute that is absent or
// empty and recursing into every present attribute.
//
// A type rmread does NOT model (OBJECT_REF, PARTICIPATION, LINK, … — see
// [rmread.Handles]) is an opaque leaf here: its members are unreadable, so
// reading them would report every one absent and fabricate `required`.
// Such a node is validated solely by its per-type invariant evaluator
// (run above). The same gate stops descent into a flattened scalar a
// reader surfaces directly — e.g. CODE_PHRASE.terminology_id comes back
// as a Go string, which is not an RM node to walk.
func (w *rmFloorWalker) walk(value any, rmType string, path string, depth int) {
	if value == nil || rmread.IsTypedNilPointer(value) {
		return
	}
	w.checkInvariants(value, rmType, path)
	// The coded invariants (REQ-112) run as their own pass rather than as
	// checkInvariants arms: that switch stops at its first match, and a
	// COMPOSITION or an ENTRY already lands on the archetype-root arm. The
	// pass also reads a leaf such as PARTY_RELATED, so it runs before the
	// Handles gate below.
	w.checkCodedInvariants(value, rmType, path)

	if !rmread.Handles(value) {
		return
	}
	if depth >= maxWalkDepth {
		w.emit(Issue{
			Path:   path,
			Code:   "max_depth",
			Detail: fmt.Sprintf("RM-floor walk exceeded max depth %d at %s (possible cyclic graph)", maxWalkDepth, rmType),
		})
		return
	}

	// AttributeNames is an optional rminfo extension (kept off the stable
	// rminfo.Lookup interface per idiom.md § public-API stability). Default
	// implements it; absence would only mean "cannot enumerate" → no descend.
	lister, ok := w.info.(rminfo.AttributeLister)
	if !ok {
		return
	}
	// rminfo knows each class by its bare BMM name; rmType may carry a
	// generic bound (DV_INTERVAL<DV_QUANTITY>) that it does not.
	class := bmmtype.Class(rmType)
	attrs := lister.AttributeNames(class)
	if attrs == nil {
		return
	}
	requiredSet := setFromSlice(w.info.RequiredAttributes(class))
	for _, attr := range attrs {
		if rminfo.IsNonStorableAttr(class, attr) {
			continue
		}
		attrType, ok := w.info.AttributeRMType(class, attr)
		if !ok {
			continue
		}
		isContainer, _ := w.info.IsContainer(class, attr)
		required := requiredSet[attr]
		attrPath := joinPath(path, "/"+attr)

		if isContainer {
			kids, hadField := rmread.ReadMultiple(value, rmType, attr)
			if !hadField {
				if required {
					w.emit(Issue{
						Path:   attrPath,
						Code:   "required",
						Detail: fmt.Sprintf("RM-mandatory multi-valued attribute %q absent on %s", attr, rmType),
					})
				}
				continue
			}
			if required && len(kids) == 0 {
				w.emit(Issue{
					Path:   attrPath,
					Code:   "cardinality",
					Detail: fmt.Sprintf("RM-mandatory multi-valued attribute %q must be non-empty on %s", attr, rmType),
				})
			}
			for i, k := range kids {
				w.walk(k, runtimeRMType(k, attrType), fmt.Sprintf("%s[%d]", attrPath, i), depth+1)
			}
			continue
		}

		// Single-valued attribute.
		val, hadField := rmread.ReadSingle(value, rmType, attr)
		if !hadField || val == nil || rmread.IsTypedNilPointer(val) {
			if required && !w.mayBeBlank(path, attr) {
				w.emit(Issue{
					Path:   attrPath,
					Code:   "required",
					Detail: fmt.Sprintf("RM-mandatory attribute %q absent on %s", attr, rmType),
				})
			}
			continue
		}
		w.walk(val, runtimeRMType(val, attrType), attrPath, depth+1)
	}
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

// setFromSlice is a small helper that turns a (possibly-nil) slice into
// a set for O(1) attribute-required lookup during the walk.
func setFromSlice(s []string) map[string]bool {
	if len(s) == 0 {
		return nil
	}
	out := make(map[string]bool, len(s))
	for _, x := range s {
		out[x] = true
	}
	return out
}

// checkInvariants runs the per-RM-type invariant evaluators on value.
// The catalogue is intentionally small (REQ-112 first cycle) — additions
// land per the BMM-bump runbook and any spec refresh. Unknown types are
// silently skipped (this is a floor, not an exhaustive RM check).
func (w *rmFloorWalker) checkInvariants(value any, rmType, path string) {
	switch {
	case rmType == "CODE_PHRASE":
		w.checkCodePhrase(value, path)
	case rmType == "DV_QUANTITY":
		w.checkDVQuantity(value, path)
	case rmType == "DV_PROPORTION":
		w.checkDVProportion(value, path)
	case strings.HasPrefix(rmType, "DV_INTERVAL"):
		// rmTypeInfo reports the numeric instantiations as
		// "DV_INTERVAL<DV_QUANTITY>" / "<DV_COUNT>" (and "DV_INTERVAL" for
		// the bare collapsed form); all dispatch to the bounds check.
		w.checkDVInterval(value, path)
	case rmType == "OBJECT_REF", rmType == "PARTY_REF", rmType == "ACCESS_GROUP_REF", rmType == "LOCATABLE_REF":
		w.checkObjectRef(value, path)
	case rmType == "DV_TEXT", rmType == "DV_CODED_TEXT":
		// DV_CODED_TEXT inherits `mappings` from DV_TEXT via embedding —
		// one evaluator, dispatched for both runtime types.
		w.checkTermMappings(value, path)
	case rmType == "TERM_MAPPING":
		w.checkTermMapping(value, path)
	case rmType == "ARCHETYPED":
		w.checkArchetyped(value, path)
	case rmType == "DV_SCALE":
		w.allowScaleSymbolWithoutCode(path)
	case rmType == "DV_DATE_TIME", rmType == "DV_DATE", rmType == "DV_TIME", rmType == "DV_DURATION":
		w.checkTemporalValue(value, rmType, path)
	case rmType == "ELEMENT":
		w.checkElementNullFlavour(value, path)
	case rmroots.IsArchetypeRoot(rmType):
		w.checkArchetypeRoot(value, rmType, path)
	}
}

// checkArchetypeRoot enforces the archetype-root rule on a node whose class
// is always an archetype root (see [rmroots.IsArchetypeRoot]). The class
// invariant `Is_archetype_root` fixes is_archetype_root true, and
// LOCATABLE's `Archetyped_valid` (is_archetype_root xor archetype_details =
// Void) then makes archetype_details mandatory, although LOCATABLE declares
// it optional. An absent archetype_details, whether omitted or JSON null,
// reports `is_archetype_root` at the node's archetype_details.
//
// The attribute is read through [rm.Locatable], which every LOCATABLE
// concrete implements, so the rule needs no rmread reader of its own.
func (w *rmFloorWalker) checkArchetypeRoot(value any, rmType, path string) {
	l, ok := value.(rm.Locatable)
	if !ok || rmread.IsTypedNilPointer(value) {
		return
	}
	if l.GetArchetypeDetails() == nil {
		w.emit(Issue{
			Path:   joinPath(path, "/archetype_details"),
			Code:   "is_archetype_root",
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
func (w *rmFloorWalker) checkArchetyped(value any, path string) {
	a, ok := asArchetyped(value)
	if !ok {
		return
	}
	if a.ArchetypeID.Value == "" {
		w.emit(Issue{
			Path:   joinPath(path, "/archetype_id/value"),
			Code:   "required",
			Detail: "ARCHETYPED.archetype_id must carry a non-empty value",
		})
	}
	if a.RMVersion == "" {
		w.emit(Issue{
			Path:   joinPath(path, "/rm_version"),
			Code:   "rm_version_valid",
			Detail: "ARCHETYPED.rm_version must be non-empty (RM Rm_version_valid)",
		})
	}
}

// checkCodePhrase enforces the RM spec floor on CODE_PHRASE: the
// code_string MUST be non-empty when the value is present, except on a
// DV_SCALE symbol's defining_code (see
// [rmFloorWalker.allowScaleSymbolWithoutCode]). (The terminology_id absence
// is already RM-required and caught by the floor's required-set walk.)
func (w *rmFloorWalker) checkCodePhrase(value any, path string) {
	cp, ok := asCodePhrase(value)
	if !ok {
		return
	}
	if cp.CodeString == "" && !w.blankCodeAllowed[path] {
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Detail: "CODE_PHRASE.code_string must be non-empty",
		})
	}
}

// allowScaleSymbolWithoutCode lets the DV_SCALE at path carry a symbol with
// no code. The RM's DV_SCALE.symbol allows a scale value that has none: its
// symbol is then a DV_CODED_TEXT carrying the terminology_id and a blank
// code_string. The CODE_PHRASE at the symbol's defining_code therefore reports
// neither the non-empty code_string invariant nor a `required` code_string.
// The exemption covers that one node only: a blank terminology_id, the
// symbol's own value, a DV_ORDINAL symbol, and every other CODE_PHRASE under
// the scale are checked as usual. It runs before the walk descends into the
// scale, so the exemption is in place when the defining_code is reached.
func (w *rmFloorWalker) allowScaleSymbolWithoutCode(path string) {
	if w.blankCodeAllowed == nil {
		w.blankCodeAllowed = map[string]bool{}
	}
	w.blankCodeAllowed[joinPath(path, "/symbol/defining_code")] = true
}

// mayBeBlank reports whether the RM-mandatory attr on the node at path is
// exempt from the required-set check. Only the code_string of a DV_SCALE
// symbol's defining_code is (see [rmFloorWalker.allowScaleSymbolWithoutCode]).
func (w *rmFloorWalker) mayBeBlank(path, attr string) bool {
	return attr == "code_string" && w.blankCodeAllowed[path]
}

// checkDVQuantity enforces the spec floor on DV_QUANTITY: precision, when
// set, must be ≥ -1. Per the RM, precision is a number of decimal places
// where 0 means integral and -1 means "no limit" (any number of decimal
// places) — so -1 is valid and only precision < -1 is out of range. units
// is RM-required (caught by the floor's required-set walk); magnitude is
// always a number on the wire so needs no separate presence check.
func (w *rmFloorWalker) checkDVQuantity(value any, path string) {
	q, ok := asDVQuantity(value)
	if !ok {
		return
	}
	if q.Precision != nil && int(*q.Precision) < -1 {
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Detail: fmt.Sprintf("DV_QUANTITY.precision must be ≥ -1 (-1 = no limit); got %d", *q.Precision),
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
// The remaining DV_PROPORTION invariants are type-specific denominator
// rules (`Valid_denominator`, `Unitary_validity`, `Percent_validity`,
// `Fraction_validity`, `Type_validity`) and are NOT in the floor: they
// turn on the `type` proportion-kind code, a different axis from
// precision.
//
// Diagnostics name the attribute and the offending operand, never the
// operand's value (REQ-093); the precision value itself is named because
// it is the constraint being reported.
func (w *rmFloorWalker) checkDVProportion(value any, path string) {
	p, ok := asDVProportion(value)
	if !ok || p.Precision == nil {
		return
	}
	switch prec := int(*p.Precision); {
	case prec < -1:
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Detail: fmt.Sprintf("DV_PROPORTION.precision must be ≥ -1 (-1 = no limit); got %d", prec),
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
			w.emit(Issue{
				Path:   path,
				Code:   "rm_invariant",
				Detail: "DV_PROPORTION.precision is 0, which requires a whole-number numerator and denominator (RM Precision_validity); not integral: " + strings.Join(fractional, ", "),
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
// 8601-backed data values: `value` must satisfy the type's BASE predicate,
// decided with the REQ-123 parse so the partial forms it admits and
// DV_DURATION's documented deviations stay valid. The empty string and a
// placeholder such as "example" fail it. The diagnostic names the attribute,
// never the offending value (REQ-093).
func (w *rmFloorWalker) checkTemporalValue(value any, rmType, path string) {
	valid, ok := temporalValueValid(value)
	if !ok || valid {
		return
	}
	w.emit(Issue{
		Path:   path,
		Code:   "rm_invariant",
		Detail: rmType + ".value must be a valid ISO 8601 value (RM Value_valid)",
	})
}

// checkElementNullFlavour enforces the RM invariant
// `Inv_null_flavour_indicated: is_null() xor null_flavour = Void` on an
// ELEMENT. is_null() means there is no `value`, so exactly one of `value` and
// `null_flavour` must be present. A typed-nil `value` counts as absent, as it
// does everywhere else in the walk. The rule is reported on the ELEMENT.
func (w *rmFloorWalker) checkElementNullFlavour(value any, path string) {
	e, ok := asElement(value)
	if !ok {
		return
	}
	hasValue := e.Value != nil && !rmread.IsTypedNilPointer(e.Value)
	hasNullFlavour := e.NullFlavour != nil
	switch {
	case hasValue && hasNullFlavour:
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Detail: "ELEMENT carries both value and null_flavour; exactly one must be present (RM Inv_null_flavour_indicated)",
		})
	case !hasValue && !hasNullFlavour:
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Detail: "ELEMENT carries neither value nor null_flavour; exactly one must be present (RM Inv_null_flavour_indicated)",
		})
	}
}

// checkDVInterval enforces the spec floor on DV_INTERVAL when both
// bounds are numerically comparable (DV_QUANTITY / DV_COUNT) and
// neither side is unbounded: lower magnitude MUST be ≤ upper magnitude.
// Other DVOrdered bound types (DV_DATE, DV_TIME, …) carry richer
// comparison semantics — those are deferred from the first cycle and
// will land alongside the REQ-123 temporal helpers' interval support.
func (w *rmFloorWalker) checkDVInterval(value any, path string) {
	lower, upper, ok := dvIntervalNumericBounds(value)
	if !ok {
		return
	}
	if lower > upper {
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Detail: fmt.Sprintf("DV_INTERVAL: lower (%v) must be ≤ upper (%v)", lower, upper),
		})
	}
}

// checkObjectRef enforces the spec floor on OBJECT_REF (and subtypes):
// id, type, and namespace are RM-mandatory. rmread models OBJECT_REF as an
// opaque leaf (the walk does not read its members), so this evaluator is
// the floor's sole check for the reference — reading the fields through the
// [rm.ObjectRefLike] interface (REQ-052) so any BMM subtype is covered.
func (w *rmFloorWalker) checkObjectRef(value any, path string) {
	if value == nil || rmread.IsTypedNilPointer(value) {
		return
	}
	ref, ok := value.(rm.ObjectRefLike)
	if !ok {
		return
	}
	if id := ref.GetID(); id == nil || rmread.IsTypedNilPointer(id) {
		w.emit(Issue{
			Path:   joinPath(path, "/id"),
			Code:   "rm_invariant",
			Detail: "OBJECT_REF.id must be present",
		})
	}
	if ref.GetType() == "" {
		w.emit(Issue{
			Path:   joinPath(path, "/type"),
			Code:   "rm_invariant",
			Detail: "OBJECT_REF.type must be non-empty",
		})
	}
	if ref.GetNamespace() == "" {
		w.emit(Issue{
			Path:   joinPath(path, "/namespace"),
			Code:   "rm_invariant",
			Detail: "OBJECT_REF.namespace must be non-empty",
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
// walked container (rmread reads it), so each TERM_MAPPING is a node of
// its own and [rmFloorWalker.checkTermMapping] evaluates it there.
//
// Diagnostics name the attribute only — never the offending value
// (REQ-093's value-free boundary-diagnostic discipline).
func (w *rmFloorWalker) checkTermMappings(value any, path string) {
	ms, ok := asMappings(value)
	if !ok {
		return
	}
	if ms != nil && len(ms) == 0 {
		w.emit(Issue{
			Path:   joinPath(path, "/mappings"),
			Code:   "mappings_valid",
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
// offending value (REQ-093).
func (w *rmFloorWalker) checkTermMapping(value any, path string) {
	m, ok := asTermMapping(value)
	if !ok {
		return
	}
	switch m.Match {
	case ">", "=", "<", "?":
	default:
		w.emit(Issue{
			Path:   joinPath(path, "/match"),
			Code:   "term_mapping_match",
			Detail: "TERM_MAPPING.match must be one of {'>', '=', '<', '?'}",
		})
	}
}

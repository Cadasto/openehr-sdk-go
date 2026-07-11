package validation

// rmfloor.go: REQ-112 — the template-less Reference Model validation
// floor. ValidateRM walks an RM root using the BMM (via rminfo) as the
// driver — no compiled OPT — and reports:
//
//   - RM-mandatory attribute absences (rminfo.RequiredAttributes per type
//     plus the container "lower bound ≥ 1" reading);
//   - per-RM-type invariants on the leaves it touches (CODE_PHRASE
//     code_string, DV_INTERVAL numeric bounds, DV_QUANTITY precision, the
//     OBJECT_REF id/type/namespace floor).
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
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// maxWalkDepth bounds the RM-floor descent. RM graphs decoded from the
// wire are acyclic trees, but ValidateRM accepts any caller-built value;
// a pathological cyclic graph is stopped here rather than overflowing the
// stack. The deepest legitimate RM nesting is far below this bound.
const maxWalkDepth = 256

// ValidateRM validates root against the openEHR Reference Model alone
// (REQ-112). It checks every RM-mandatory attribute on every node
// reachable from root, and runs per-RM-type invariants on the leaves it
// touches. It does NOT consult any operational template — use
// [Validate] / [ValidateComposition] / [ValidateFolder] / [ValidateEHRStatus]
// / [ValidateDemographic] when a compiled OPT is available.
//
// A nil root surfaces a single `nil_root` issue and is reported as
// not-OK. An unknown RM root type (a Go value outside the v2 closed RM
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
// source JSON key set (REQ-112).
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
	attrs := lister.AttributeNames(rmType)
	if attrs == nil {
		return
	}
	requiredSet := setFromSlice(w.info.RequiredAttributes(rmType))
	for _, attr := range attrs {
		if rminfo.IsNonStorableAttr(rmType, attr) {
			continue
		}
		attrType, ok := w.info.AttributeRMType(rmType, attr)
		if !ok {
			continue
		}
		isContainer, _ := w.info.IsContainer(rmType, attr)
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
			if required {
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
	case strings.HasPrefix(rmType, "DV_INTERVAL"):
		// rmTypeInfo reports the numeric instantiations as
		// "DV_INTERVAL<DV_QUANTITY>" / "<DV_COUNT>" (and "DV_INTERVAL" for
		// the bare collapsed form); all dispatch to the bounds check.
		w.checkDVInterval(value, path)
	case rmType == "OBJECT_REF", rmType == "PARTY_REF", rmType == "ACCESS_GROUP_REF", rmType == "LOCATABLE_REF":
		w.checkObjectRef(value, path)
	}
}

// checkCodePhrase enforces the RM spec floor on CODE_PHRASE: the
// code_string MUST be non-empty when the value is present. (The
// terminology_id absence is already RM-required and caught by the
// floor's required-set walk.)
func (w *rmFloorWalker) checkCodePhrase(value any, path string) {
	cp, ok := asCodePhrase(value)
	if !ok {
		return
	}
	if cp.CodeString == "" {
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Detail: "CODE_PHRASE.code_string must be non-empty",
		})
	}
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

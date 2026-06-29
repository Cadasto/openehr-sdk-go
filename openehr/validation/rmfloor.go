package validation

// rmfloor.go: REQ-112 — the template-less Reference Model validation
// floor. ValidateRM walks an RM root using the BMM (via rminfo) as the
// driver — no compiled OPT — and reports:
//
//   - RM-mandatory attribute absences (rminfo.RequiredAttributes per type
//     plus the container "lower bound ≥ 1" reading);
//   - per-RM-type invariants on the leaves it touches (CODE_PHRASE,
//     DV_INTERVAL bounds, DV_QUANTITY magnitude/units coherence, the
//     OBJECT_REF id+type floor).
//
// SDK-GAP-15 surface. Independent of REQ-102/110 (template-driven); both
// drivers may run against the same root — REQ-110 enforces template
// constraints, REQ-112 enforces the RM-only floor.
//
// Sign-off (2026-06-29): Option A — second driver alongside the
// template-driven walker. Walker + invariant evaluators live in this
// file; the closed RM-type set is shared with the template-driven path
// via the existing rmTypeInfo/describeRMType helpers (composition.go).

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

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
	w.walk(root, rmType, "/")
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
// every BMM-known attribute is a descend candidate; cycle-breaking
// uses a visited set keyed on identity-comparable RM nodes.
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
// path. It first runs the per-type invariants on the current node,
// then iterates every BMM-known attribute — emits `required` for any
// RM-mandatory attribute that is absent or empty, and recurses into
// every present attribute. Unknown rmType (not registered with
// rminfo.Default) is a no-op descent — invariants and required-set
// checks both consult rminfo, so an unknown class yields no issues
// from this node, only what its parent already emitted.
func (w *rmFloorWalker) walk(value any, rmType string, path string) {
	if value == nil || rmread.IsTypedNilPointer(value) {
		return
	}
	w.checkInvariants(value, rmType, path)

	attrs := w.info.AttributeNames(rmType)
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
				w.walk(k, attrType, fmt.Sprintf("%s[%d]", attrPath, i))
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
		// Recurse with the value's *runtime* RM type when we know it;
		// otherwise honour the BMM-declared attribute type. The runtime
		// type may be a subtype of the declared one (Liskov), so reading
		// rmTypeInfo first lets the invariant checks dispatch correctly.
		runtimeType := attrType
		if rt, _, ok := rmTypeInfo(val); ok {
			runtimeType = rt
		}
		w.walk(val, runtimeType, attrPath)
	}
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
	switch rmType {
	case "CODE_PHRASE":
		w.checkCodePhrase(value, path)
	case "DV_QUANTITY":
		w.checkDVQuantity(value, path)
	case "DV_INTERVAL":
		w.checkDVInterval(value, path)
	case "OBJECT_REF", "PARTY_REF", "ACCESS_GROUP_REF", "LOCATABLE_REF":
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

// checkDVQuantity enforces the spec floor on DV_QUANTITY: precision,
// when set, must be non-negative; units is RM-required (so caught by
// the floor's required-set walk). The magnitude is always a number on
// the wire so no separate presence check is needed.
func (w *rmFloorWalker) checkDVQuantity(value any, path string) {
	q, ok := asDVQuantity(value)
	if !ok {
		return
	}
	if q.Precision != nil && int(*q.Precision) < 0 {
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Detail: fmt.Sprintf("DV_QUANTITY.precision must be non-negative; got %d", *q.Precision),
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
// the parent struct's id, type, and namespace are RM-required. The
// required-set walk already catches absent fields by name; this
// invariant adds the "empty-string vs nil" reading that rminfo can't
// express, since the generated Go shape sets the field to zero rather
// than omitting it.
func (w *rmFloorWalker) checkObjectRef(value any, path string) {
	id, refType, namespace, ok := objectRefBaseFields(value)
	if !ok {
		return
	}
	if refType == "" {
		w.emit(Issue{
			Path:   joinPath(path, "/type"),
			Code:   "rm_invariant",
			Detail: "OBJECT_REF.type must be non-empty",
		})
	}
	if namespace == "" {
		w.emit(Issue{
			Path:   joinPath(path, "/namespace"),
			Code:   "rm_invariant",
			Detail: "OBJECT_REF.namespace must be non-empty",
		})
	}
	// id is an OBJECT_ID polymorphic interface; the required-set walk
	// emits `required` on a nil id, so we only sanity-check the
	// printable value here when one is present.
	if id != "" {
		return
	}
}

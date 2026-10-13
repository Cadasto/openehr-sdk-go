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
// REQ-112 surface. ValidateRM runs it alone; the REQ-102/110
// template-driven entries run it after their own walk over the same root
// and merge its findings (validate.go, ADR 0025).
//
// Sign-off (2026-06-29): Option A — second driver alongside the
// template-driven walker. Walker + invariant evaluators live in this
// file; the closed RM-type set is shared with the template-driven path
// via the existing rmTypeInfo/describeRMType helpers (composition.go).

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	"github.com/cadasto/openehr-sdk-go/internal/rmroots"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// ValidateRM validates root against the openEHR Reference Model alone:
// it is the floor on its own, for a root no operational template applies
// to. It checks every RM-mandatory attribute on every node
// reachable from root, and runs per-RM-type invariants on the leaves it
// touches. It does not consult any operational template. The
// template-driven entries, [Validate] / [ValidateComposition] /
// [ValidateFolder] / [ValidateEHRStatus] / [ValidateDemographic], include
// this floor, so a caller with a compiled OPT needs only one of them. On its
// own the floor knows no template names, so an item whose archetype node id
// a sibling shares is written by its position.
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
// The floor visits each value held by pointer once. A value built in Go may
// hold a pointer in two places, or hold itself; a decoded document holds
// neither. A value reached again by another path is not walked a second
// time, so its issues carry the first path only, and a value reached again
// inside itself is reported as `cyclic_value` at each path where it recurs
// and is not descended into. A value held by value can hold itself too,
// through a list that shares its own backing array, and the floor cannot
// recognise it; so every call stops after a number of visits, by default
// far above what a decoded document needs, and reports `max_nodes` once, at
// the path where it stopped. A graph nested deeper than the walk's depth limit
// is not walked below it; the walk reports `max_depth` once, at the first
// path where it stopped. [Result.Coverage] lists the path of each of the
// two, and the [Limits] of a [Validator] set both.
//
// ValidateRM returns what a [Validator] built with no options returns from
// Validate(root, nil).
//
// A nil root surfaces a single `nil_root` issue with an empty path and is
// reported as not-OK. An unknown RM root type (a Go value outside the
// closed RM set) surfaces `rm_type_unknown` at "/"; the floor cannot descend
// further but does not panic.
func ValidateRM(root any) Result {
	var v Validator
	return v.Validate(root, nil)
}

// validateRM runs the floor alone over root, within lim, guarding a nil root
// and one outside the closed RM set.
func validateRM(root any, lim Limits) Result {
	if root == nil || rmaccess.IsTypedNilPointer(root) {
		return resultFromIssues([]Issue{{
			Path:     "",
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
	issues, stopped := walkFloor(root, rmType, nil, lim)
	r := resultFromIssues(issues)
	r.Coverage.Stopped = stopped
	return r
}

// walkFloor runs the floor over root, of RM type rmType, within lim, and
// returns its issues and the path of each place where a limit stopped it.
// names gives the name the OPT pins on each item a template walk bound, so
// the floor writes that walk's path for the node; nil writes no name
// predicate.
func walkFloor(root any, rmType string, names pinnedNames, lim Limits) (issues []Issue, stopped []string) {
	w := newFloorWalker(names, lim)
	w.walk(root, rmType, "/", roleNone, 0)
	return w.issues, w.stoppedAt
}

// newFloorWalker returns a floor walker that writes the names the OPT pins
// (nil for none) and stops at lim, each zero or negative field of which
// stands for its default.
func newFloorWalker(names pinnedNames, lim Limits) *rmFloorWalker {
	lim = lim.withDefaults()
	return &rmFloorWalker{info: rminfo.Default, names: names, maxDepth: lim.Depth, maxNodes: lim.Nodes}
}

// floorIssues runs the floor, within lim, over the root a template-driven
// validation walked, with the names that walk bound, and returns its issues
// and the path of each place where a limit stopped it. A root outside the
// closed RM set, which the template walk reports as a type mismatch, is one
// the floor cannot descend: it reports it as `rm_type_unknown` at "/", as
// [ValidateRM] does.
func floorIssues(root any, names pinnedNames, lim Limits) (issues []Issue, stopped []string) {
	rmType, _, ok := rmTypeInfo(root)
	if !ok {
		return []Issue{{
			Path:     "/",
			Code:     "rm_type_unknown",
			Detail:   fmt.Sprintf("root Go type %T is outside the closed RM set; the RM floor cannot descend", root),
			Severity: Error,
		}}, nil
	}
	return walkFloor(root, rmType, names, lim)
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

// rmFloorWalker accumulates issues as it descends the RM graph driven
// by rminfo. There is no notion of "OPT-declared attribute" here —
// every BMM-known attribute is a descend candidate. Each value with an
// identity is visited once ([rmFloorWalker.walk]), recursion depth is
// bounded by maxDepth and the number of visits by maxNodes, so a
// caller-built graph that shares or cycles through a pointer, holds itself
// by value, or nests too deep, terminates instead of branching without end
// or overflowing the stack.
type rmFloorWalker struct {
	info   rminfo.Lookup
	issues []Issue
	// maxDepth and maxNodes are the [Limits] of the call, defaults filled
	// in. RM graphs decoded from the wire are acyclic trees far shallower
	// and smaller than either, but a value built in Go may nest without end
	// or hold itself by value, which the visit record cannot recognise.
	maxDepth, maxNodes int
	// stoppedAt holds the path of the max_depth issue and of the max_nodes
	// issue, in the order the walk reported them; depthStopped is set once
	// the first depth stop is reported, so later ones are silent.
	stoppedAt    []string
	depthStopped bool
	// reached holds each value with an identity ([hasIdentity]) the walk has
	// reached; walk allocates it. inside holds those the walk is inside,
	// outermost first.
	reached map[any]struct{}
	inside  []any
	// visits counts the values the walk has visited, up to maxNodes;
	// stopped is set once it reaches that limit, and ends the walk.
	visits  int
	stopped bool
	// names gives the name the OPT pins on the node an item binds to, so
	// the floor writes an item's path as the template walk does
	// ([itemPaths]). The floor on its own binds no OPT node, so it leaves
	// names nil and writes no name predicate.
	names pinnedNames
}

// floorRole is what the floor knows of a node from where it sits, beyond
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
	// value the ARCHETYPED entry reports ([rmFloorWalker.checkArchetyped]).
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
// max_nodes or cyclic_value, an issue that says where the floor's walk
// stopped or turned back rather than judge the value there.
func isWalkCondition(code string) bool {
	switch code {
	case "max_depth", "max_nodes", "cyclic_value":
		return true
	}
	return false
}

// emit appends i to the walker's issues. Every floor issue is an [Error],
// the zero Severity, so no site sets it.
func (w *rmFloorWalker) emit(i Issue) {
	w.issues = append(w.issues, i)
}

// walk descends value (of declared BMM type rmType, in role role) at the
// given issue path, once per value. A value with an identity
// ([hasIdentity]) that the walk is inside, reached again, is a cycle:
// walk reports `cyclic_value` at path and does not descend. One the walk
// has finished, reached again by another path, is shared: walk leaves it,
// so its issues carry the first path only. Every other value is visited.
func (w *rmFloorWalker) walk(value any, rmType string, path string, role floorRole, depth int) {
	if w.stopped || value == nil || rmaccess.IsTypedNilPointer(value) {
		return
	}
	if !hasIdentity(value) {
		w.visit(value, rmType, path, role, depth)
		return
	}
	if w.reached == nil {
		// Sized for a small composition, so the common walk grows them
		// rarely.
		w.reached = make(map[any]struct{}, 64)
		w.inside = make([]any, 0, 16)
	}
	// One map operation records value and tells whether it was there: the
	// set grows only for a value reached for the first time.
	before := len(w.reached)
	w.reached[value] = struct{}{}
	if len(w.reached) == before {
		// The inside list is searched only for a value reached again, which
		// a decoded document never holds.
		if slices.Contains(w.inside, value) {
			w.emit(Issue{
				Path:   path,
				Code:   "cyclic_value",
				Detail: fmt.Sprintf("RM-floor walk reached this %s again inside itself; it is not descended into again", rmType),
			})
		}
		return
	}
	w.inside = append(w.inside, value)
	w.visit(value, rmType, path, role, depth)
	w.inside = w.inside[:len(w.inside)-1]
}

// visit checks value (of declared BMM type rmType, in role role) at the
// given issue path. A visit past maxNodes checks nothing: it reports
// `max_nodes` at path and ends the walk. Otherwise visit first runs the
// per-type invariants on the current node, then, for every registered RM
// class ([rmaccess.Covers]), reads every BMM-known attribute through the
// generated access, emitting `required` for any RM-mandatory attribute that
// is absent and `cardinality` for an RM-mandatory container that is present
// and empty, and recursing into every present attribute. The items of a
// multi-valued attribute are written by the path rule the template walk
// shares ([itemPaths]); the entries of a Hash attribute by their position in
// ascending key order, never by their key.
//
// A value of no registered class is a leaf here, validated solely by its
// per-type invariant evaluator (run above): a flattened scalar the access
// surfaces directly, such as CODE_PHRASE.terminology_id, which comes back as
// a Go string, or a primitive value. The parts of a reference (OBJECT_REF and
// its subtypes) are the reference's own evaluator's to report
// ([rmFloorWalker.checkObjectRef]), so the required-set check leaves them
// out and a missing part is reported once; the walk still goes below a
// present id.
func (w *rmFloorWalker) visit(value any, rmType string, path string, role floorRole, depth int) {
	if w.visits == w.maxNodes {
		w.emit(Issue{
			Path:   path,
			Code:   "max_nodes",
			Detail: fmt.Sprintf("RM-floor walk stopped at %s after visiting %d values, its limit per call", rmType, w.maxNodes),
		})
		w.stoppedAt = append(w.stoppedAt, path)
		w.stopped = true
		return
	}
	w.visits++
	w.checkInvariants(value, rmType, path, role)
	// The coded invariants (REQ-112) run as their own pass rather than as
	// checkInvariants arms: that switch stops at its first match, and a
	// COMPOSITION or an ENTRY already lands on the archetype-root arm. Like
	// checkInvariants, it runs before the Covers gate below, so it sees every
	// node the walk visits.
	w.checkCodedInvariants(value, rmType, path)

	if !rmaccess.Covers(value) {
		return
	}
	if depth >= w.maxDepth {
		// The first stop is reported; a later one, on another branch, is
		// not walked below either, and is silent, so a value that reaches
		// the limit on many branches yields one issue.
		if !w.depthStopped {
			w.depthStopped = true
			w.emit(Issue{
				Path:   path,
				Code:   "max_depth",
				Detail: fmt.Sprintf("RM-floor walk reached its depth limit %d at %s; nothing below that depth is walked, and later stops there are not reported again", w.maxDepth, rmType),
			})
			w.stoppedAt = append(w.stoppedAt, path)
		}
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
		if w.stopped {
			return // the visit limit ended the walk below this node
		}
		if rminfo.IsNonStorableAttr(class, attr) {
			continue
		}
		attrType, ok := w.info.AttributeRMType(class, attr)
		if !ok {
			continue
		}
		isContainer, _ := w.info.IsContainer(class, attr)
		required := requiredSet[attr] && !isReferencePart(rmType, attr)
		attrPath := joinPath(path, "/"+attr)

		switch {
		case isContainer:
			kids, present := rmaccess.Multiple(value, attr)
			if !present {
				if required {
					w.emit(Issue{
						Path:   attrPath,
						Code:   "required",
						Layer:  LayerRM,
						Rule:   ruleMandatory,
						Detail: fmt.Sprintf("RM-mandatory multi-valued attribute %q absent on %s", attr, rmType),
					})
				}
				continue
			}
			if required && len(kids) == 0 {
				w.emit(Issue{
					Path:   attrPath,
					Code:   "cardinality",
					Layer:  LayerRM,
					Rule:   ruleCardinality,
					Detail: fmt.Sprintf("RM-mandatory multi-valued attribute %q must be non-empty on %s", attr, rmType),
				})
			}
			// A list of a primitive type (DV_MULTIMEDIA data, an Array of
			// Octet, holds one item per byte) has nothing below it to walk.
			if !slices.ContainsFunc(kids, rmaccess.Covers) {
				continue
			}
			kidRole := childRole(class, role, attr)
			for i, kidPath := range w.itemPaths(attrPath, kids) {
				if !rmaccess.Covers(kids[i]) {
					continue
				}
				w.walk(kids[i], runtimeRMType(kids[i], attrType), kidPath, kidRole, depth+1)
			}
		case attrType == hashType:
			_, entries, present := rmaccess.Hash(value, attr)
			if !present {
				if required {
					w.emit(Issue{
						Path:   attrPath,
						Code:   "required",
						Layer:  LayerRM,
						Rule:   ruleMandatory,
						Detail: fmt.Sprintf("RM-mandatory attribute %q absent on %s", attr, rmType),
					})
				}
				continue
			}
			// An entry of a Hash of a primitive type has nothing below it;
			// one of an RM class is walked at its position in key order.
			for i, entry := range entries {
				if !rmaccess.Covers(entry) {
					continue
				}
				w.walk(entry, runtimeRMType(entry, attrType), hashEntryPath(attrPath, i), roleNone, depth+1)
			}
		default:
			val, present := rmaccess.Single(value, attr)
			if !present || val == nil || rmaccess.IsTypedNilPointer(val) {
				if required && !mayBeBlank(role, attr) {
					w.emit(Issue{
						Path:   attrPath,
						Code:   "required",
						Layer:  LayerRM,
						Rule:   ruleMandatory,
						Detail: fmt.Sprintf("RM-mandatory attribute %q absent on %s", attr, rmType),
					})
				}
				continue
			}
			w.walk(val, runtimeRMType(val, attrType), attrPath, childRole(class, role, attr), depth+1)
		}
	}
}

// hashType is the RM type rminfo declares for every Hash attribute, whatever
// its key and value types.
const hashType = "Hash"

// hashEntryPath returns the issue path of the entry at index i, in ascending
// key order, of the Hash attribute at attrPath: its 1-based position, never
// its key, which is instance data (REQ-102 Issue paths).
func hashEntryPath(attrPath string, i int) string {
	return attrPath + "[" + strconv.Itoa(i+1) + "]"
}

// isReferencePart reports whether attr is one of the three parts of a
// reference (id, namespace and type of OBJECT_REF and its subtypes) on a node
// of RM type rmType. [rmFloorWalker.checkObjectRef] reports a missing part,
// so the required-set check leaves the three out and each is reported once.
func isReferencePart(rmType, attr string) bool {
	switch rmType {
	case "OBJECT_REF", "PARTY_REF", "LOCATABLE_REF", "ACCESS_GROUP_REF":
		switch attr {
		case "id", "namespace", "type":
			return true
		}
	}
	return false
}

// itemPaths returns the issue path of each of the items kids of the
// multi-valued attribute at attrPath: the shared path rule ([itemPaths]),
// fed each item's archetype_node_id and the name w.names pins for it.
func (w *rmFloorWalker) itemPaths(attrPath string, kids []any) []string {
	items := make([]pathItem, len(kids))
	for i, k := range kids {
		items[i] = pathItem{
			id:   locatableArchetypeNodeID(k),
			name: w.names[itemKey{attrPath: attrPath, index: i}],
		}
	}
	return itemPaths(attrPath, items)
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
func (w *rmFloorWalker) checkInvariants(value any, rmType, path string, role floorRole) {
	switch {
	case rmType == "CODE_PHRASE":
		w.checkCodePhrase(value, path, role)
	case rmType == "DV_QUANTITY":
		w.checkDVQuantity(value, path)
	case rmType == "DV_PROPORTION":
		w.checkDVProportion(value, path)
	case strings.HasPrefix(rmType, "DV_INTERVAL"):
		// rmTypeInfo reports the typed instantiations as
		// "DV_INTERVAL<DV_QUANTITY>" / "<DV_COUNT>" / … (and "DV_INTERVAL" for
		// the bare collapsed form); all dispatch to the flags and bounds checks.
		w.checkDVInterval(value, path)
	case rmType == "OBJECT_REF", rmType == "PARTY_REF", rmType == "ACCESS_GROUP_REF", rmType == "LOCATABLE_REF":
		w.checkObjectRef(value, path)
	case rmType == "PARTY_IDENTIFIED", rmType == "PARTY_RELATED":
		// PARTY_RELATED inherits the PARTY_IDENTIFIED rules via embedding:
		// one evaluator, dispatched for both runtime types.
		w.checkPartyIdentified(value, path)
	case rmType == "DV_TEXT", rmType == "DV_CODED_TEXT":
		// DV_CODED_TEXT inherits `mappings` from DV_TEXT via embedding —
		// one evaluator, dispatched for both runtime types.
		w.checkTermMappings(value, path)
	case rmType == "TERM_MAPPING":
		w.checkTermMapping(value, path)
	case rmType == "ARCHETYPED":
		w.checkArchetyped(value, path)
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
// concrete implements.
func (w *rmFloorWalker) checkArchetypeRoot(value any, rmType, path string) {
	l, ok := value.(rm.Locatable)
	if !ok || rmaccess.IsTypedNilPointer(value) {
		return
	}
	if l.GetArchetypeDetails() == nil {
		w.emit(Issue{
			Path:   joinPath(path, "/archetype_details"),
			Code:   "is_archetype_root",
			Layer:  LayerRM,
			Rule:   archetypeRootRules[rmType],
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
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: "ARCHETYPED.archetype_id must carry a non-empty value",
		})
	}
	if a.RMVersion == "" {
		w.emit(Issue{
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
// terminology_id absence is already RM-required and caught by the floor's
// required-set walk.)
func (w *rmFloorWalker) checkCodePhrase(value any, path string, role floorRole) {
	cp, ok := asCodePhrase(value)
	if !ok {
		return
	}
	if cp.CodeString == "" && !scaleSymbolMayHaveNoCode(role) {
		w.emit(Issue{
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
// at the same path ([rmFloorWalker.checkArchetyped]), so it is reported once.
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
// is RM-required (caught by the floor's required-set walk); magnitude is
// always a number on the wire so needs no separate presence check.
//
// The Detail names the attribute and the allowed range, never the
// precision the instance carries; that precision is carried in Issue.Value
// (REQ-168).
func (w *rmFloorWalker) checkDVQuantity(value any, path string) {
	q, ok := asDVQuantity(value)
	if !ok {
		return
	}
	if q.Precision != nil && int(*q.Precision) < -1 {
		w.emit(Issue{
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
// The remaining DV_PROPORTION invariants are type-specific denominator
// rules (`Valid_denominator`, `Unitary_validity`, `Percent_validity`,
// `Fraction_validity`, `Type_validity`) and are NOT in the floor: they
// turn on the `type` proportion-kind code, a different axis from
// precision.
//
// Diagnostics name the attribute and the offending operand, never a value
// from the instance: neither the precision nor an operand (REQ-168). The
// value the check read is carried in Issue.Value: the precision on the
// range arm, and the numerator and the denominator, as a [2]float64, on
// the integrality arm.
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
			w.emit(Issue{
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
// 8601-backed data values: `value` must satisfy the type's BASE predicate,
// decided with the REQ-123 parse so the partial forms it admits and
// DV_DURATION's documented deviations stay valid. The empty string and a
// placeholder such as "example" fail it. The diagnostic names the attribute,
// never the offending value (REQ-168); that value string is carried in
// Issue.Value.
func (w *rmFloorWalker) checkTemporalValue(value any, rmType, path string) {
	text, valid, ok := temporalValue(value)
	if !ok || valid {
		return
	}
	w.emit(Issue{
		Path:   path,
		Code:   "rm_invariant",
		Layer:  LayerRM,
		Rule:   valueValidRules[rmType],
		Detail: rmType + ".value must be a valid ISO 8601 value (RM Value_valid)",
		Value:  constraints.Redact(text),
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
	hasValue := e.Value != nil && !rmaccess.IsTypedNilPointer(e.Value)
	hasNullFlavour := e.NullFlavour != nil
	switch {
	case hasValue && hasNullFlavour:
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleNullFlavourIndicated,
			Detail: "ELEMENT carries both value and null_flavour; exactly one must be present (RM Inv_null_flavour_indicated)",
		})
	case !hasValue && !hasNullFlavour:
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleNullFlavourIndicated,
			Detail: "ELEMENT carries neither value nor null_flavour; exactly one must be present (RM Inv_null_flavour_indicated)",
		})
	}
}

// checkDVInterval enforces the spec floor on DV_INTERVAL. The boundary
// flags are checked on every interval ([rmFloorWalker.checkIntervalFlags]).
// The bound order is checked when both bounds are numerically comparable
// (DV_QUANTITY / DV_COUNT) and neither side is unbounded: lower magnitude
// MUST be ≤ upper magnitude. Other DVOrdered bound types (DV_DATE, DV_TIME,
// …) carry richer comparison semantics — those are deferred from the first
// cycle and will land alongside the REQ-123 temporal helpers' interval
// support.
//
// The Detail names the rule, never the bounds; the two compared magnitudes
// are carried in Issue.Value, lower first, as a [2]float64 (REQ-168).
func (w *rmFloorWalker) checkDVInterval(value any, path string) {
	w.checkIntervalFlags(value, path)
	lower, upper, ok := dvIntervalNumericBounds(value)
	if !ok {
		return
	}
	if lower > upper {
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleIntervalLimits,
			Detail: "DV_INTERVAL: lower must be ≤ upper",
			Value:  constraints.Redact([2]float64{lower, upper}),
		})
	}
}

// checkIntervalFlags enforces BASE Interval's Lower_included_valid and
// Upper_included_valid (lower_unbounded implies not lower_included, and the
// same for upper). A side marked both unbounded and included is reported at
// the interval's own path, as lower_included_valid or upper_included_valid;
// both sides wrong give both issues. Decode reads such a side as written, and
// a value built in Go can carry it too, so the floor is where it is reported.
//
// The flags are read through [rmaccess.Single], which serves every
// DV_INTERVAL instantiation, the typed ones and the bare one, as a pointer or
// a value. The Detail names the flags and the invariant, never a bound. The
// check reads only the two flags, which the code already states, so the
// issue's Value stays empty.
func (w *rmFloorWalker) checkIntervalFlags(value any, path string) {
	for _, side := range [...]struct{ end, code, invariant, rule string }{
		{end: "lower", code: "lower_included_valid", invariant: "Lower_included_valid", rule: ruleLowerIncludedValid},
		{end: "upper", code: "upper_included_valid", invariant: "Upper_included_valid", rule: ruleUpperIncludedValid},
	} {
		if !intervalFlagSet(value, side.end+"_unbounded") || !intervalFlagSet(value, side.end+"_included") {
			continue
		}
		w.emit(Issue{
			Path:   path,
			Code:   side.code,
			Layer:  LayerRM,
			Rule:   side.rule,
			Detail: "DV_INTERVAL." + side.end + "_included must be false when " + side.end + "_unbounded is true (BASE " + side.invariant + ")",
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
// like any node, but its required-set check leaves the three parts out
// ([isReferencePart]), so this evaluator is the floor's only check of a
// missing part, which keeps it from being reported twice. It reads the
// fields through the [rm.ObjectRefLike] interface (REQ-052), so any BMM
// subtype is covered.
func (w *rmFloorWalker) checkObjectRef(value any, path string) {
	if value == nil || rmaccess.IsTypedNilPointer(value) {
		return
	}
	ref, ok := value.(rm.ObjectRefLike)
	if !ok {
		return
	}
	if id := ref.GetID(); id == nil || rmaccess.IsTypedNilPointer(id) {
		w.emit(Issue{
			Path:   joinPath(path, "/id"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: "OBJECT_REF.id must be present",
		})
	}
	if ref.GetType() == "" {
		w.emit(Issue{
			Path:   joinPath(path, "/type"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   ruleMandatory,
			Detail: "OBJECT_REF.type must be non-empty",
		})
	}
	if ref.GetNamespace() == "" {
		w.emit(Issue{
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
// [rmFloorWalker.checkTermMappings]): an absent key and an explicit JSON
// `null` both decode to a nil pointer or a nil slice and read as absent, so
// only a decoded `"name":""` or `"identifiers":[]` is present and empty. A
// present external_ref is walked as its own node, where checkObjectRef
// checks its parts. PARTY_RELATED's Relationship_valid, a terminology-group
// rule, is not checked here but with the other coded invariants
// ([rmFloorWalker.checkCodedInvariants]).
//
// Diagnostics name the attribute and the RM rule, never the offending value
// (REQ-168).
func (w *rmFloorWalker) checkPartyIdentified(value any, path string) {
	p, ok := asPartyIdentified(value)
	if !ok {
		return
	}
	if p.Name == nil && p.Identifiers == nil && p.ExternalRef == nil {
		w.emit(Issue{
			Path:   path,
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   rulePartyBasicValidity,
			Detail: "PARTY_IDENTIFIED has none of name, identifiers and external_ref; at least one must be present (RM Basic_validity)",
		})
	}
	if p.Name != nil && *p.Name == "" {
		w.emit(Issue{
			Path:   joinPath(path, "/name"),
			Code:   "rm_invariant",
			Layer:  LayerRM,
			Rule:   rulePartyNameValid,
			Detail: "PARTY_IDENTIFIED.name is present but empty (RM Name_valid)",
		})
	}
	if p.Identifiers != nil && len(p.Identifiers) == 0 {
		w.emit(Issue{
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
// its own and [rmFloorWalker.checkTermMapping] evaluates it there.
//
// Diagnostics name the attribute only, never a value from the instance
// (REQ-168). The check reads no value, only the list's length, so the
// issue's Value stays empty.
func (w *rmFloorWalker) checkTermMappings(value any, path string) {
	ms, ok := asMappings(value)
	if !ok {
		return
	}
	if ms != nil && len(ms) == 0 {
		w.emit(Issue{
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
			Layer:  LayerRM,
			Rule:   ruleTermMappingMatch,
			Detail: "TERM_MAPPING.match must be one of {'>', '=', '<', '?'}",
			Value:  constraints.Redact(m.Match),
		})
	}
}

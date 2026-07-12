package validation

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmnames"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// ValidateComposition validates an in-memory RM Composition
// against a compiled OPT and returns every issue in one pass —
// REQ-102.
//
// The walk is template-driven (see package doc): the compiled OPT
// drives traversal, the composition is the value source. Path
// strings are built incrementally by [joinPath] as the walker
// descends — OPT-side attribute names contribute the segments,
// matched RM children contribute the bracket predicates. The
// composition's at-codes therefore appear in [Issue.Path] only on
// nodes the walker actually bound to an OPT child; missing
// required nodes report at the parent attribute path without a
// composition-side predicate.
//
// Returns a [Result] whose Issues slice is never nil (zero-length
// allocation when no issues fire).
//
// ValidateComposition is the COMPOSITION-typed convenience over the
// generic [Validate]: it guards the nil composition (yielding
// nil_composition), then delegates. Other archetypeable RM roots are
// validated through [Validate] or its siblings [ValidateDemographic],
// [ValidateFolder], [ValidateEHRStatus] (REQ-110).
func ValidateComposition(comp *rm.Composition, c *templatecompile.Compiled) Result {
	if comp == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_composition",
			Detail:   "ValidateComposition called with a nil *rm.Composition argument",
			Severity: Error,
		}})
	}
	return Validate(comp, c)
}

// walker carries the issue accumulator and any per-call state for
// a single ValidateComposition invocation. Constructed via
// newWalker so callers do not see the zero value (which has a nil
// issues slice — never returned to callers).
type walker struct {
	compiled *templatecompile.Compiled
	issues   []Issue
}

func newWalker(c *templatecompile.Compiled) *walker {
	// Pre-allocate a non-nil issues slice so resultFromIssues
	// returns a non-nil slice on the OK path.
	return &walker{compiled: c, issues: []Issue{}}
}

// emit appends one Issue to the walker's accumulator. Small helper
// kept for symmetry with later code that may want to deduplicate
// or sort issues; right now it is a direct append.
func (w *walker) emit(i Issue) {
	w.issues = append(w.issues, i)
}

// joinPath glues a parent AQL path with a child segment. Mirrors
// the compile-side pathSegment convention: root path is "/" and
// child deltas already begin with "/" (e.g. "/category",
// "/content[at0001]"). Centralised here so future tweaks to path
// construction (e.g. predicate normalisation) have one home.
func joinPath(parent, segment string) string {
	if parent == "/" {
		return segment
	}
	return parent + segment
}

// rmTypeInfo is the single source of truth for "what RM class is
// this Go value and (when LOCATABLE) what is its archetype_node_id".
// All v2 walker code routes through this function — describeRMType
// and locatableArchetypeNodeID are thin wrappers.
//
// Since ADR 0013 it decomposes onto the generated surface: the name
// half delegates to rm.RMTypeName (every registered concrete — the
// previous hand-written closed set was a subset, so recognition
// widened to the full registry), the archetype_node_id half reads
// polymorphically through rm.Locatable. Only the validation-facing
// parameterised DV_INTERVAL diagnostic names stay hand-written: they
// are display names for rm_type_mismatch findings, deliberately more
// specific than the bare registry name.
//
// Returns ("", "", false) for nil, typed-nil, and non-RM Go values.
// REQ-024 — no reflection.
func rmTypeInfo(v any) (rmType string, archetypeNodeID string, ok bool) {
	if v == nil || rmread.IsTypedNilPointer(v) {
		return "", "", false
	}
	// Typed DV_INTERVAL instantiations keep their parameterised
	// diagnostic names (bare "DV_INTERVAL" would lose the bound in
	// rm_type_mismatch findings). Single canonical closed set in
	// rmnames — the previous local switch had drifted three
	// instantiations (DV_DURATION/DV_ORDINAL/DV_SCALE) behind the
	// DVOrdered closure.
	if name, ok := rmnames.TypedIntervalName(v); ok {
		return name, "", true
	}
	name, ok := rm.RMTypeName(v)
	if !ok {
		return "", "", false
	}
	if l, isLocatable := v.(rm.Locatable); isLocatable {
		// Guarded above: typed-nils never reach the getter.
		return name, l.GetArchetypeNodeID(), true
	}
	return name, "", true
}

// describeRMType returns a string suitable for inclusion in Issue
// Detail messages identifying the concrete Go type of an RM value.
// Pointer types are unwrapped to their elem type name.
func describeRMType(v any) string {
	if v == nil {
		return "<nil>"
	}
	if name, _, ok := rmTypeInfo(v); ok {
		return name
	}
	return fmt.Sprintf("%T", v)
}

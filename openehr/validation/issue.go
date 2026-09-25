package validation

import (
	"fmt"
	"strings"
)

// Severity is the typed severity attached to every [Issue]. [ValidateComposition]
// emits only [Error]. [ValidateAQL] may also emit [Warning] for lint
// advisories (e.g. aql_from_archetype, aql_select_star; the lint codes pass
// through verbatim, so the full set is the openehr/aql/lint advisory catalogue).
type Severity int

const (
	// Error means the composition violates a constraint.
	// Callers should treat any Error-severity issue as a hard fail.
	Error Severity = iota

	// Warning is an advisory issue that does not make a [Result]
	// not-OK. [ValidateAQL] emits it for lint advisories (e.g.
	// aql_from_archetype, aql_select_star; the full set is the
	// openehr/aql/lint catalogue); [ValidateComposition] emits only [Error].
	Warning
)

// String returns "error" / "warning"; out-of-range values render as
// `severity(N)` with the numeric form, so callers logging issues see
// both the name and the wire value.
func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	}
	return fmt.Sprintf("severity(%d)", int(s))
}

// Issue is one failing clause from a validator. Validators emit one
// Issue per failure (collect-all, not fail-fast). The zero value is
// not useful; construct via the per-validator helpers.
type Issue struct {
	// Path is the AQL path of the offending node. Empty for global
	// issues that do not localise to a specific node (e.g. root
	// archetype-id mismatch).
	Path string

	// Code is a stable programmatic identifier (e.g. "required",
	// "cardinality", "rm_type_mismatch", "slot_fill",
	// "primitive_out_of_range"). Consumers should dispatch on Code
	// rather than parse Detail.
	Code string

	// Detail is a human-readable message describing the failure.
	// Includes the offending value where reasonable; not localised.
	Detail string

	// Severity classifies the issue. [ValidateComposition] emits [Error] only;
	// [ValidateAQL] may emit [Warning] advisories that do not flip [Result.OK].
	Severity Severity
}

// Err returns the typed sentinel matching this Issue's Code, or
// nil when no sentinel maps. The mapping is:
//
//   - "required"                                          → [ErrRequired]
//   - "cardinality"                                       → [ErrCardinality]
//   - "rm_type_mismatch" / "alternative_mismatch" /
//     "archetype_id_mismatch" / "node_id_mismatch"        → [ErrTypeMismatch]
//   - "primitive_*" (any primitive_-prefixed code)        → [ErrPrimitive]
//   - "slot_fill"                                         → [ErrSlotFill]
//   - "aql_syntax" / "aql_empty"                          → [ErrAQLSyntax]
//
// Global guard codes (`nil_composition`, `nil_template`, and the
// root guards `nil_root` / `nil_party` / `nil_folder` /
// `nil_ehr_status`) return nil, because they represent caller-side
// argument errors rather than validation failures. Callers wanting
// `errors.Is` dispatch go through this method:
//
//	for _, i := range r.Issues {
//	    if errors.Is(i.Err(), validation.ErrRequired) { ... }
//	}
func (i Issue) Err() error {
	switch i.Code {
	case "required":
		return ErrRequired
	case "cardinality":
		return ErrCardinality
	case "rm_type_mismatch",
		"alternative_mismatch",
		"archetype_id_mismatch",
		"node_id_mismatch":
		return ErrTypeMismatch
	case "slot_fill":
		return ErrSlotFill
	case "aql_syntax", "aql_empty":
		// Codes minted by openehr/aql/lint (REQ-109) and carried verbatim
		// through ValidateAQL; keep this in sync with the lint code strings.
		return ErrAQLSyntax
	}
	if strings.HasPrefix(i.Code, "primitive_") {
		return ErrPrimitive
	}
	return nil
}

// Result aggregates every [Issue] from a single validator call.
// OK is the convenience boolean: true exactly when no Error-severity
// issue is present.
type Result struct {
	// OK is true when the validator found no [Error]-severity issue.
	// [Warning]-severity advisories (emitted by [ValidateAQL], e.g.
	// aql_from_archetype) do not make OK false. Treat OK as the
	// pass/fail result and inspect Severity for advisories. For
	// [ValidateComposition], which emits only Error issues, OK is
	// equivalent to len(r.Issues) == 0.
	OK bool

	// Issues is the full list of issues in a stable per-validator order
	// (composition walk order for [ValidateComposition]; lint layer order
	// for [ValidateAQL]). Empty when a validator found nothing; never nil
	// after a validator call (zero-length allocation is acceptable). May be
	// non-empty while OK is true when it holds only Warning-severity issues.
	Issues []Issue
}

// resultFromIssues builds a [Result] from a slice of issues. Used by
// validators that accumulate into a local slice and return at the
// end of the walk. OK is true unless an Error-severity issue is
// present (Warnings do not flip it). The Issues field is never nil —
// callers can range over it without a nil guard, matching the doc on
// [Result.Issues].
func resultFromIssues(issues []Issue) Result {
	if issues == nil {
		issues = []Issue{}
	}
	ok := true
	for _, i := range issues {
		if i.Severity == Error {
			ok = false
			break
		}
	}
	return Result{
		OK:     ok,
		Issues: issues,
	}
}

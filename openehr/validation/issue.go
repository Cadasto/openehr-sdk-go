package validation

import (
	"fmt"
	"strings"
)

// Severity is the typed severity attached to every [Issue]. Only
// [Error] is emitted in v1; [Warning] is reserved for follow-up REQs
// that surface advisory checks (e.g. open-list pattern hints).
type Severity int

const (
	// Error means the composition violates a normative constraint.
	// Callers SHOULD treat any Error-severity issue as a hard fail.
	Error Severity = iota

	// Warning is reserved for v1; no validator currently emits it.
	// Future versions MAY use it for advisory checks (open-list
	// hints, deprecated patterns, etc.).
	Warning
)

// String returns "error" / "warning"; out-of-range values render as
// `severity(N)` with the numeric form for diagnostic readability —
// callers logging issues see both the name and the wire value.
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
	// "primitive_out_of_range"). Consumers SHOULD dispatch on Code
	// rather than parse Detail.
	Code string

	// Detail is a human-readable message describing the failure.
	// Includes the offending value where reasonable; not localised.
	Detail string

	// Severity classifies the issue. Always [Error] in v1.
	Severity Severity
}

// Err returns the typed sentinel matching this Issue's Code, or
// nil when no sentinel maps. The mapping is the inverse of the
// REQ-102 § Sentinels table:
//
//   - "required"                                          → [ErrRequired]
//   - "cardinality"                                       → [ErrCardinality]
//   - "rm_type_mismatch" / "alternative_mismatch" /
//     "archetype_id_mismatch" / "node_id_mismatch"        → [ErrTypeMismatch]
//   - "primitive_*" (any primitive_-prefixed code)        → [ErrPrimitive]
//   - "slot_fill"                                         → [ErrSlotFill]
//
// Global guard codes (`nil_composition`, `nil_template`) return
// nil — those represent caller-side argument errors rather than
// validation failures. Callers wanting `errors.Is` dispatch wrap
// via this method:
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
	}
	if strings.HasPrefix(i.Code, "primitive_") {
		return ErrPrimitive
	}
	return nil
}

// Result aggregates every [Issue] from a single validator call.
// OK is the convenience boolean — true exactly when Issues is empty.
type Result struct {
	// OK is true when no issues were found. Defensive: callers MAY
	// also use len(r.Issues) == 0.
	OK bool

	// Issues is the full list of failing clauses in walk order.
	// Empty when OK; never nil after a validator call (zero-length
	// allocation is acceptable).
	Issues []Issue
}

// resultFromIssues builds a [Result] from a slice of issues. Used by
// validators that accumulate into a local slice and return at the
// end of the walk. The Issues field is never nil — callers can range
// over it without a nil guard, matching the doc on [Result.Issues].
func resultFromIssues(issues []Issue) Result {
	if issues == nil {
		issues = []Issue{}
	}
	return Result{
		OK:     len(issues) == 0,
		Issues: issues,
	}
}

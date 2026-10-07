package validation

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
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
// not useful; the validators in this package build issues themselves.
//
// The doc of each field ends with its class, value-free or value-bearing.
// The class of Path and Detail depends on the entry point that returned
// the issue.
//
// The instance validators are [ValidateComposition], [Validate],
// [ValidateDemographic], [ValidateFolder] and [ValidateEHRStatus], and the
// template-less floor [ValidateRM], [ValidateRMFolder],
// [ValidateRMEHRStatus], [ValidateRMEHRAccess], [ValidateRMDemographic]
// and [ValidateRMEHRStatusBytes]. On their issues Path, Code, Detail and
// Severity never contain a value taken from the instance: not the content
// of a data value, a code, units, a precision, an interval bound, nor the
// text of an error raised while decoding it. They may name RM types and
// attributes, archetype and node ids, and the template's own constraint,
// so such an issue can be logged or returned to a client as it is. The
// value the failed check read is kept only in Value.
//
// On an issue from [ValidateAQL] or [ValidateAQLWithTypeRelation], Code
// and Severity never contain text from the query, but Detail and Path
// carry the lint text and may quote the query, its literals included.
// Value is empty.
//
// Two issues compare equal under == when their paths, codes, details and
// severities are equal and their held values are equal under ==, and the
// comparison never panics. A value that == cannot compare, such as a slice
// or a map, is held by reference, so it equals only copies of the same
// [constraints.Redacted].
type Issue struct {
	// Path is the AQL path of the offending node. Empty for global
	// issues that do not localise to a specific node (e.g. root
	// archetype-id mismatch). On an issue from [ValidateAQL] or
	// [ValidateAQLWithTypeRelation] it is the lint path.
	//
	// Value-free on issues from the instance validators: never contains a
	// value from the instance. Value-bearing on issues from [ValidateAQL]
	// and [ValidateAQLWithTypeRelation]: may quote the query, literals
	// included.
	Path string

	// Code is a stable programmatic identifier (e.g. "required",
	// "cardinality", "rm_type_mismatch", "slot_fill",
	// "primitive_out_of_range"). Consumers should dispatch on Code
	// rather than parse Detail.
	//
	// Value-free: never carries a value from the instance or text from
	// the query.
	Code string

	// Detail is a human-readable message describing the failure; it is
	// not localised. From an instance validator it names the check that
	// failed; from [ValidateAQL] or [ValidateAQLWithTypeRelation] it is
	// the lint text.
	//
	// Value-free on issues from the instance validators: never contains a
	// value from the instance, such as the value the check read.
	// Value-bearing on issues from [ValidateAQL] and
	// [ValidateAQLWithTypeRelation]: may quote the query, literals
	// included.
	Detail string

	// Severity classifies the issue. [ValidateComposition] emits [Error] only;
	// [ValidateAQL] may emit [Warning] advisories that do not flip [Result.OK].
	//
	// Value-free: never carries a value from the instance or text from
	// the query.
	Severity Severity

	// Value holds the value from the instance that the failed check
	// read. Value.Reveal() returns it with this Go type:
	//
	//   - on a primitive_* issue, the value of the
	//     [constraints.Violation] it reports, with the type that
	//     violation holds;
	//   - on an rm_invariant issue for a precision below -1, the
	//     precision, an rm.Integer;
	//   - on an rm_invariant issue for a precision of 0 with a
	//     fractional operand, the numerator and the denominator, a
	//     [2]float64;
	//   - on an rm_invariant issue for interval bounds out of order, the
	//     two compared magnitudes, lower first, a [2]float64;
	//   - on an rm_invariant issue for a date, time or duration that is
	//     not valid ISO 8601, its value, a string;
	//   - on a term_mapping_match issue, the match, an rm.Character;
	//   - on an invalid_shape issue from a failed decode, the decode
	//     error, an error whose text may quote the input.
	//
	// Each call that fails to decode creates a new error value, so two
	// calls over the same bytes give invalid_shape issues that are not
	// equal under ==.
	//
	// Value is empty on every other issue (something absent, a count, a
	// type or identity mismatch, a guard).
	//
	// Value-bearing: holds a value from the instance. It prints as
	// "[redacted]", as [constraints.Redacted] describes, and is left out
	// of JSON and gob output. Value.Reveal() returns the value itself, to
	// be called only where showing it is safe. Value is empty on every
	// issue from [ValidateAQL] and [ValidateAQLWithTypeRelation].
	Value constraints.Redacted `json:"-"`
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

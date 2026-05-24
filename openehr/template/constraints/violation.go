package constraints

// Violation is one failed clause of a [PrimitiveConstraint.Validate]
// call. Validators emit a Violation per failing clause (range, list,
// pattern, …), never collapsing several failures into one. Callers
// that need a single-line message can join Detail values.
//
// The zero value is not useful; construct via the helper builders
// (`outOfRange`, `notInList`, etc. — internal) or directly when the
// detail is bespoke.
type Violation struct {
	// Code is the typed reason for the failure. Use the exported
	// `Code*` constants in this package; new codes appear here only
	// when a constraint type cannot be expressed with the existing
	// vocabulary.
	Code ViolationCode

	// Detail is a human-readable message describing the failure,
	// suitable for display in validator output. Implementations
	// include the offending value verbatim where reasonable.
	Detail string
}

// ViolationCode is the typed failure category attached to every
// [Violation]. The set is closed — validators inside this package
// never invent new codes at runtime. Consumers can pattern-match on
// it to surface localised messages or to bucket failures by kind.
type ViolationCode string

const (
	// CodeOutOfRange — the input value lies outside the constraint's
	// numeric range (lower / upper bounds).
	CodeOutOfRange ViolationCode = "out_of_range"

	// CodePatternMismatch — the input string does not match the
	// constraint's pattern (regex for C_STRING; AOM date pattern for
	// C_DATE / C_TIME / C_DATE_TIME / C_DURATION).
	CodePatternMismatch ViolationCode = "pattern_mismatch"

	// CodeNotInList — the input value is not a member of the
	// constraint's closed list (e.g. allowed strings, allowed codes,
	// allowed ordinal values).
	CodeNotInList ViolationCode = "not_in_list"

	// CodeWrongType — the input value's Go type cannot be coerced to
	// the type the constraint expects (e.g. passing a string to
	// CInteger.Validate).
	CodeWrongType ViolationCode = "wrong_type"

	// CodeUnitUnknown — the input quantity's units string is not one
	// of the units enumerated by the DV_QUANTITY constraint.
	CodeUnitUnknown ViolationCode = "unit_unknown"

	// CodeInvalidValue — the input value is malformed in a way the
	// other codes do not cover (e.g. a date string that does not
	// parse, an empty pattern). Validators fall back to this when no
	// more specific code applies.
	CodeInvalidValue ViolationCode = "invalid_value"
)

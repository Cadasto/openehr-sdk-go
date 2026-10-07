package constraints

// Violation is one failed clause of a [PrimitiveConstraint.Validate]
// call. Validators emit a Violation per failing clause (range, list,
// pattern, …), never collapsing several failures into one. Callers
// that need a single-line message can join Detail values.
//
// Code and Detail never contain the value passed to Validate, or any
// part of it, such as a quantity's magnitude or a coded term's code.
// They name the clause that failed, and may quote the constraint
// itself (its list, range, pattern, units or terminology), so a
// Violation can be logged or returned to a client as it is. The value
// is kept only in Value, which always prints as "[redacted]" when it
// holds a value, is left out of JSON and gob output, and is read with
// Value.Reveal().
//
// Violations can be compared with ==, and two compare equal when their
// codes, details and held values are equal. The zero value is not
// useful: validators in this package build violations themselves. A
// caller that builds one keeps the input out of Code and Detail and
// wraps it with [Redact] into Value.
type Violation struct {
	// Code is the typed reason for the failure. Use the exported
	// `Code*` constants in this package; new codes appear here only
	// when a constraint type cannot be expressed with the existing
	// vocabulary. It never contains the value under validation.
	Code ViolationCode

	// Detail is a human-readable message naming the clause that
	// failed, suitable for display in validator output. It never
	// contains the value passed to Validate or any part of it.
	Detail string

	// Value holds the part of the input that the failing clause
	// tested: the argument as passed, or the one field of it the
	// clause read, such as a quantity's magnitude or a coded term's
	// code. Holding a value, it always prints as "[redacted]"; it is
	// left out of JSON and gob output, and Value.Reveal() returns the
	// value itself, to be called only where showing it is safe. Value is
	// empty on CodeWrongType, where no clause tested the input, and
	// when the constraint itself is at fault, such as a pattern that
	// does not parse.
	Value Redacted `json:"-"`
}

// ViolationCode is the typed failure category attached to every
// [Violation]. The set is closed: validators inside this package
// never invent new codes at runtime. Consumers can pattern-match on
// it to surface localised messages or to bucket failures by kind.
type ViolationCode string

const (
	// CodeOutOfRange means the input value lies outside the constraint's
	// numeric range (lower / upper bounds).
	CodeOutOfRange ViolationCode = "out_of_range"

	// CodePatternMismatch means the input string does not match the
	// constraint's pattern (regex for C_STRING; AOM date pattern for
	// C_DATE / C_TIME / C_DATE_TIME / C_DURATION).
	CodePatternMismatch ViolationCode = "pattern_mismatch"

	// CodeNotInList means the input value is not a member of the
	// constraint's closed list (e.g. allowed strings, allowed codes,
	// allowed ordinal values).
	CodeNotInList ViolationCode = "not_in_list"

	// CodeWrongType means the input value's Go type cannot be coerced to
	// the type the constraint expects (e.g. passing a string to
	// CInteger.Validate).
	CodeWrongType ViolationCode = "wrong_type"

	// CodeUnitUnknown means the input quantity's units string is not one
	// of the units enumerated by the DV_QUANTITY constraint.
	CodeUnitUnknown ViolationCode = "unit_unknown"

	// CodeInvalidValue means the input value is malformed in a way the
	// other codes do not cover (e.g. a date string that does not
	// parse, an empty pattern). Validators fall back to this when no
	// more specific code applies.
	CodeInvalidValue ViolationCode = "invalid_value"
)

package constraints

// Violation is one failed clause of a [PrimitiveConstraint.Validate]
// call. Validators emit a Violation per failing clause (range, list,
// pattern, …), never collapsing several failures into one. Callers
// that need a single-line message can join Detail values.
//
// The doc of each field ends with its class. Code and Detail are
// value-free: they name the clause that failed, and may quote the
// constraint itself (its list, range, pattern, units or terminology), so
// a Violation can be logged or returned to a client as it is. Value is
// value-bearing: the input is kept there and nowhere else.
//
// Two violations compare equal under == when their codes and details are
// equal and their held values are equal under ==, and the comparison
// never panics. A value that == cannot compare, such as a slice or a map,
// is held by reference, so it equals only copies of the same [Redacted].
// The zero value is not useful: validators in this package build
// violations themselves. A caller that builds one keeps the input out of
// Code and Detail and wraps it with [Redact] into Value.
type Violation struct {
	// Code is the typed reason for the failure. Use the exported
	// `Code*` constants in this package; new codes appear here only
	// when a constraint type cannot be expressed with the existing
	// vocabulary.
	//
	// Value-free: never contains the value passed to Validate or any
	// part of it.
	Code ViolationCode

	// Detail is a human-readable message naming the clause that
	// failed, suitable for display in validator output. It may quote
	// the constraint itself.
	//
	// Value-free: never contains the value passed to Validate or any
	// part of it, such as a quantity's magnitude or a coded term's code.
	Detail string

	// Value holds the part of the input that the failing clause
	// tested: the argument as passed, or the one field of it the
	// clause read, such as a quantity's magnitude or a coded term's
	// code. Value is empty on CodeWrongType, where no clause tested the
	// input, and when the constraint itself is at fault, such as a
	// pattern that does not parse.
	//
	// Value-bearing: holds the tested part of the input. It prints as
	// "[redacted]", as [Redacted] describes, and is left out of JSON and
	// gob output. Value.Reveal() returns the value itself, to be called
	// only where showing it is safe.
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

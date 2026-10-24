package validation

import "errors"

// Sentinel errors for validation issues. Each [Issue] returned by a
// validator carries a stable [Issue.Code] string that mirrors the
// sentinel name (e.g. an issue with `Code: "required"` corresponds
// to ErrRequired). Callers compare via [errors.Is] only when they
// need to surface a typed Go error; the typical consumer path is
// the [Result.Issues] slice with code-based dispatch.
var (
	// ErrCardinality fires when a multi-valued attribute's child
	// count violates the OPT-declared cardinality (lower / upper).
	// The composition-root invariant "content non-empty when set"
	// also surfaces as ErrCardinality.
	ErrCardinality = errors.New("validation: cardinality")

	// ErrRequired fires when a required attribute is absent or
	// zero-valued: one the Reference Model marks mandatory, or one the
	// template requires. The template requires an attribute by its
	// existence, and a list or a Hash only where its cardinality, or the
	// occurrences of one of its children, also asks for a member.
	ErrRequired = errors.New("validation: required attribute missing")

	// ErrProhibited fires when an attribute the OPT prohibits (its
	// existence upper bound is 0, or it has at least one OPT child and
	// allows none of them) is present: a single-valued
	// attribute holds a value, or a multi-valued one holds at least one
	// item.
	ErrProhibited = errors.New("validation: prohibited attribute present")

	// ErrTypeMismatch fires when the RM type of a composition node
	// disagrees with the template's declared RMTypeName at the
	// matching compiled node.
	ErrTypeMismatch = errors.New("validation: rm type mismatch")

	// ErrPrimitive fires when a template leaf's primitive constraint
	// emits one or more violations on a
	// composition value (out of range, not in code list, pattern mismatch,
	// etc.). The [constraints.ViolationCode] is reflected into [Issue.Code]
	// with a "primitive_" prefix so consumers can dispatch without
	// importing the constraints package.
	ErrPrimitive = errors.New("validation: primitive constraint")

	// ErrSlotFill fires when a Content[i] archetype id does not
	// satisfy the OPT's slot include / exclude rules. The rules are the
	// parsed `archetype_id matches {regex}` assertions, with the
	// RM-type-prefix fallback (openEHR-EHR-<rmType>.) when no
	// includes were parsed.
	ErrSlotFill = errors.New("validation: slot fill")

	// ErrAQLSyntax is the sentinel for a Layer 1 lint failure reported by
	// [ValidateAQL]: an empty query (code "aql_empty") or one that does
	// not parse against the SDK grammar profile (code "aql_syntax").
	// Map via [Issue.Err]; the parse layer's own aql.ErrSyntax
	// is the equivalent in the aql package.
	ErrAQLSyntax = errors.New("validation: aql syntax")
)

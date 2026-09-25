package contain

import "strconv"

// Verdict is the answer to a containment question.
//
// The zero value is [UnknownClass]: without a definite answer the relation
// reports "not a class I know" instead of a false Admissible or Never.
//
// The numeric values and their ordering are not part of the contract:
// compare verdicts with == or switch, never order them.
type Verdict int

const (
	// UnknownClass means the class is not one the relation knows (pinned BMM +
	// overlays). Unknown is not wrong: future RM releases, demographic
	// deployments, and dialects are legitimate sources of unknown names.
	UnknownClass Verdict = iota
	// Never means that, for a pair, no route connects the pair (a CONTAINS over it is always
	// empty, a NOT CONTAINS trivially true); for an operand, it is not a containment target.
	Never
	// ByReference means the pair is resolvable only through a reference hop that
	// some engines implement as containment; portable behaviour is not
	// guaranteed. Arises only on the pair question.
	ByReference
	// Admissible means that, for a pair, a route (by-value or overlay) connects the pair using
	// only non-reference edges; for an operand, it is a legal CONTAINS operand.
	Admissible
)

func (v Verdict) String() string {
	switch v {
	case UnknownClass:
		return "UnknownClass"
	case Never:
		return "Never"
	case ByReference:
		return "ByReference"
	case Admissible:
		return "Admissible"
	default:
		return "Verdict(" + strconv.Itoa(int(v)) + ")"
	}
}

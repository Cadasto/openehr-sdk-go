package contain

import "strconv"

// Verdict is the answer to a containment question — REQ-160 § Verdicts.
//
// The zero value is [UnknownClass]: absent a definite answer the relation
// reports "not a class I know" rather than a false Admissible or Never.
type Verdict int

const (
	// UnknownClass — the class is not one the relation knows (pinned BMM +
	// overlays). Unknown is not wrong: future RM releases, demographic
	// deployments, and dialects are legitimate sources of unknown names.
	UnknownClass Verdict = iota
	// Never — pair: no route connects the pair (a CONTAINS over it is always
	// empty, a NOT CONTAINS trivially true); operand: not a containment target.
	Never
	// ByReference — the pair is resolvable only through a reference hop that
	// some engines implement as containment; portable behaviour is not
	// guaranteed. Arises only on the pair question.
	ByReference
	// Admissible — pair: a route (by-value or overlay) connects the pair using
	// only non-reference edges; operand: a legal CONTAINS operand.
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

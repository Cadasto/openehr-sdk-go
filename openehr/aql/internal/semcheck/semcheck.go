// Package semcheck is the shared rule engine behind the AQL semantic
// containment checks: it turns REQ-160 containment verdicts into REQ-161 issue
// codes, once, for every adapter that needs them.
//
// Two adapters consume it. The read side is the lint pass over a parsed
// document (openehr/aql/lint, REQ-161); the write side is the builder
// verification over the containment algebra (openehr/aql, REQ-162). REQ-162
// § Contract requires the two to report an IDENTICAL code multiset for an
// equivalent query — which is what one engine buys: neither adapter classifies
// a verdict of its own, and neither spells an issue code.
//
// It is internal deliberately. openehr/aql/lint already imports openehr/aql, so
// the builder cannot import lint; a package under openehr/aql/internal/ is
// importable by both (Go's internal-visibility rule) and adds no public API
// surface to keep stable.
//
// The contract is class NAMES in, [contain.Finding]s out — no AST, no spans, no
// source text — so a caller holding only two RM class names gets the full
// decision. Severity, Span and Path stay the adapter's business: the read side
// looks the severity up with [SeverityOf] and pins the Span on the offending
// class expression, while the write side has no source text to point into
// (REQ-162 § Contract).
//
// Building block (REQ-013): this package imports openehr/aql/contain and the
// standard library, nothing else.
package semcheck

import (
	"fmt"
	"strconv"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
)

// The REQ-161 containment issue codes (REQ-161 § Checks). These strings exist
// in exactly one place in the SDK — here — so the two adapters cannot drift
// apart on a spelling.
const (
	// CodeImpossibleContainment marks an adjacent FROM/CONTAINS pair of
	// containable operands whose REQ-160 pair verdict is Never.
	CodeImpossibleContainment = "aql_impossible_containment"
	// CodeNotContainable marks a CONTAINS operand that is a known RM class but
	// not a containment target at all.
	CodeNotContainable = "aql_contains_not_containable"
	// CodeUnknownRMClass marks a class token the relation does not know.
	CodeUnknownRMClass = "aql_unknown_rm_class"
	// CodeContainmentByReference marks a pair whose REQ-160 pair verdict is
	// ByReference — resolvable only across a reference hop.
	CodeContainmentByReference = "aql_containment_by_reference"
)

// Severity is the REQ-161 severity of an issue code.
//
// It mirrors the lint.Severity vocabulary rather than reusing it: openehr/aql/lint
// imports this package, so the arrow cannot be reversed. The read-side adapter
// maps this onto its own Severity; the write side (REQ-162 § Contract) carries
// no severity field at all and reads the table only when it wants one.
type Severity int

const (
	// Error means a static defect provable from the relation.
	Error Severity = iota
	// Warning is advisory: the query may still be exactly what the author
	// meant, and the target CDR is the authority.
	Warning
)

// String renders "error" / "warning"; out-of-range values render numerically.
func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	}
	return "severity(" + strconv.Itoa(int(s)) + ")"
}

// severities is the ONE code→severity table for the feature (REQ-161
// § Checks). Both adapters read it through [SeverityOf]; neither keeps a table
// of its own.
var severities = map[string]Severity{
	CodeImpossibleContainment:  Error,
	CodeNotContainable:         Error,
	CodeUnknownRMClass:         Warning,
	CodeContainmentByReference: Warning,
}

// SeverityOf reports the REQ-161 severity of code, and whether code is in the
// catalogue at all.
//
// A caller MUST treat !ok conservatively: an unrecognised code is not licence to
// manufacture an Error (REQ-161 § Flagging policy — a false Error is worse than
// a missed defect).
func SeverityOf(code string) (Severity, bool) {
	s, ok := severities[code]
	return s, ok
}

// Role is where a class expression sits in the FROM/CONTAINS tree.
//
// It decides one thing only: whether [Operand.Findings] may raise
// CodeNotContainable. REQ-161 § Checks scopes that code to a CONTAINS operand,
// and a FROM root is the containment anchor rather than a containment target.
type Role int

const (
	// RoleRoot is the FROM root — not a CONTAINS operand.
	RoleRoot Role = iota
	// RoleContained is a CONTAINS operand, junction operands included.
	RoleContained
)

// Checker applies the REQ-161 containment rules over one REQ-160 relation.
//
// The zero Checker uses the REQ-160 default relation, as does New(nil)
// (REQ-161 § Relation supply). A Checker is immutable and safe for concurrent
// use.
type Checker struct{ rel *contain.Relation }

// New returns a Checker over r. A nil r means the REQ-160 default relation, so
// a caller with no dialect overlay edges passes nil and gets the pinned RM
// (REQ-161 § Relation supply).
func New(r *contain.Relation) Checker { return Checker{rel: r} }

// relation resolves the nil relation once, here, so neither adapter repeats the
// defaulting rule — and so the zero Checker is usable rather than a nil
// dereference (REQ-025: no panic on caller input).
func (c Checker) relation() *contain.Relation {
	if c.rel == nil {
		return contain.Default()
	}
	return c.rel
}

// Operand is one decided class expression of a FROM/CONTAINS tree — the
// per-operand half of the REQ-161 decision, and the token [Checker.Pair] takes.
//
// Obtain one from [Checker.Operand]. The zero Operand names nothing: it reports
// no finding and suppresses every pair it takes part in, which is what lets an
// adapter model "no enclosing parent" (a junction AT the FROM root has none)
// without a special case.
type Operand struct {
	rmType  string
	role    Role
	verdict contain.Verdict
}

// Operand decides one class expression from its RM class name (REQ-160
// § Containable operands). The relation matches rmType
// ASCII-case-insensitively. An empty rmType names nothing — a junction node
// carries no class of its own — and yields the zero-value decision rather than
// a finding against the empty name.
func (c Checker) Operand(rmType string, role Role) Operand {
	if rmType == "" {
		return Operand{role: role}
	}
	return Operand{rmType: rmType, role: role, verdict: c.relation().Containable(rmType)}
}

// RMType is the class name this operand was decided from; "" for the zero
// Operand.
func (o Operand) RMType() string { return o.rmType }

// Verdict is the REQ-160 containability verdict of the operand's class:
// Admissible, Never, or UnknownClass. ByReference never appears here — it
// arises only on the pair question.
func (o Operand) Verdict() contain.Verdict { return o.verdict }

// Suppresses reports whether this operand suppresses the pair checks for every
// pair it participates in.
//
// REQ-161 § Checks: an operand whose verdict is UnknownClass, or whose
// containability is Never, is reported ONCE through its own operand-level code,
// and no pair code is built on it — "one finding per defect, and no Error built
// on an unknown name". [contain.Relation.CanContain] is total and would answer
// such a pair UnknownClass or Never all by itself; mapping that answer into a
// pair code is exactly the double-report the rule forbids, which is why
// [Checker.Pair] consults this first.
func (o Operand) Suppresses() bool { return o.verdict != contain.Admissible }

// Findings are this operand's own findings — at most one:
//
//   - CodeUnknownRMClass (Warning) when the relation does not know the class.
//     Unknown is not wrong: a future RM release, a demographic deployment or a
//     dialect are legitimate sources of an unknown name, so it is never an Error
//     and never silence (REQ-161 § Flagging policy).
//   - CodeNotContainable (Error) when the class is known but is not a
//     containment target, and the operand is a CONTAINS operand.
//
// A FROM root whose class is not containable yields NO finding: REQ-161
// § Checks scopes CodeNotContainable to a CONTAINS operand, and the catalogue
// authorises no code for the anchor position. That is a documented missed
// defect — the conservative direction — not an Error invented outside the
// catalogue. It still suppresses the pair checks below it, so no Error is built
// on it either.
func (o Operand) Findings() []contain.Finding {
	if o.rmType == "" {
		return nil
	}
	switch o.verdict {
	case contain.UnknownClass:
		return []contain.Finding{{
			Code: CodeUnknownRMClass,
			Detail: fmt.Sprintf("class %s is not known to the containment relation, so its containment is unchecked;"+
				" a future RM release, a demographic deployment, or a dialect may define it", o.rmType),
		}}
	case contain.Never:
		if o.role != RoleContained {
			return nil
		}
		return []contain.Finding{{
			Code: CodeNotContainable,
			Detail: fmt.Sprintf("class %s is a known RM class but not a containment target"+
				" (it conforms to none of LOCATABLE, VERSIONED_OBJECT, VERSION, and is not the EHR root),"+
				" so a CONTAINS naming it can never match", o.rmType),
		}}
	case contain.Admissible:
		return nil // a legal operand — nothing to report about it alone
	case contain.ByReference:
		return nil // unreachable: ByReference arises only on the pair question
	}
	return nil
}

// Pair asks the REQ-160 pair question for ONE adjacent FROM/CONTAINS pair and
// classifies the verdict (REQ-161 § Checks):
//
//   - Never → CodeImpossibleContainment (Error). Provable from the relation: no
//     route connects the pair, so the CONTAINS can never match.
//   - ByReference → CodeContainmentByReference (Warning). The hop is
//     engine-specific, so this is advisory rather than an Error.
//   - Admissible → no finding.
//
// It returns nothing when either operand suppresses (see [Operand.Suppresses]),
// which is where the REQ-161 suppression rule is enforced — structurally, in one
// place, so neither adapter can re-derive it differently.
//
// Adjacency is the caller's business. Reachability composes, so a pairwise
// question is exactly what the query asserts and a transitive pair MUST NOT be
// synthesised. A junction operand is passed with the junction's ENCLOSING
// parent as ancestor, and a NOT CONTAINS pair is passed in like any other — an
// impossible exclusion is trivially true, a dead constraint, and equally a
// defect.
func (c Checker) Pair(ancestor, descendant Operand) []contain.Finding {
	if ancestor.Suppresses() || descendant.Suppresses() {
		return nil
	}
	// Both operands are containable, so CanContain's totality short-circuits
	// (UnknownClass / Never on an operand) cannot fire: what comes back is the
	// route verdict itself.
	switch c.relation().CanContain(ancestor.rmType, descendant.rmType) {
	case contain.Never:
		return []contain.Finding{{
			Code: CodeImpossibleContainment,
			Detail: fmt.Sprintf("no containment route under the pinned RM connects %s to %s,"+
				" so this CONTAINS can never match", ancestor.rmType, descendant.rmType),
		}}
	case contain.ByReference:
		return []contain.Finding{{
			Code: CodeContainmentByReference,
			Detail: fmt.Sprintf("%s reaches %s only across a reference hop; whether that counts as containment"+
				" is engine-specific, so verify this step against the target CDR", ancestor.rmType, descendant.rmType),
		}}
	case contain.Admissible:
		return nil // a route exists using no reference edge — nothing to report
	case contain.UnknownClass:
		return nil // unreachable: an unknown operand suppresses the pair above
	}
	return nil
}

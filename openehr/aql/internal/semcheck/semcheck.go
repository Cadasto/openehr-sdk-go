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
	"strings"

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
	// CodeArchetypeClassMismatch marks a literal archetype predicate whose
	// HRID type segment does not conform to the class it is attached to
	// (REQ-161 § Checks, REQ-160 § Archetype/class conformance).
	CodeArchetypeClassMismatch = "aql_archetype_class_mismatch"

	// The three REQ-161 portability codes below are advisory: the query is
	// legal and well-formed, but its behaviour is left open by the openEHR
	// QUERY specification, so a conformant CDR is free to differ. None of the
	// three may ever become an Error (REQ-161 § Flagging policy) — see
	// [severities].

	// CodeVersionNoPredicate marks a VERSION class expression carrying no
	// version predicate at all: the default tier a bare VERSION resolves to
	// is unspecified (SPECPR-481), so a portable query SHOULD state
	// [LATEST_VERSION] or [ALL_VERSIONS] explicitly.
	CodeVersionNoPredicate = "aql_version_no_predicate"
	// CodeVersionedObjectUnreferenced marks an operand whose class conforms
	// to VERSIONED_OBJECT (a conformance question, REQ-160 § Containable
	// operands — never a `VERSIONED_` name-prefix guess) and whose alias
	// roots no identified path outside FROM/CONTAINS: the step is redundant
	// unless container-level attributes are read (Discourse #14186).
	CodeVersionedObjectUnreferenced = "aql_versioned_object_unreferenced"
	// CodeFanoutRowGrain marks an AND containment junction whose
	// AND-flattened class-expression leaves (never descending into an OR
	// junction) include two or more operands each projected by at least one
	// SELECT column: what a result row IS when sibling containments multiply
	// is an open specification question (SPECQUERY-9), so this is advisory
	// only, and deliberately narrow. Unlike the other two portability codes,
	// this one carries no RM question at all — no relation is consulted — so
	// the firing rule lives entirely in the lint adapter
	// (openehr/aql/lint/semantic.go), which is a pure walk of the parsed
	// containment/SELECT shape; this package only holds its code spelling
	// and severity, for the single reason every other code does (one
	// catalogue, never two).
	CodeFanoutRowGrain = "aql_fanout_row_grain"
)

// Severity is the REQ-161 severity of an issue code.
//
// It mirrors the lint.Severity vocabulary rather than reusing it: openehr/aql/lint
// imports this package, so the arrow cannot be reversed.
//
// Only ONE adapter consumes it. The read side maps it onto its own
// lint.Severity to fill [lint.Issue].Severity; the write-side adapter never
// touches it, because a [contain.Finding] carries no severity field at all
// (REQ-162 § Contract). A CONSUMER of the write side that wants a severity for a
// code it received is a different party from the adapter, and gets one from
// REQ-161's catalogue — restated in prose on [aql.Builder.VerifyContainment] so
// no lookup is needed — never from a second mapping of its own. See [severities].
type Severity int

// Warning is FIRST so the zero Severity is the INERT one. [SeverityOf] returns
// (zero, false) for a code outside the catalogue, and REQ-161 § Flagging policy
// holds that a false Error is worse than a missed defect — an Error zero value
// would make this API's own default the very thing SeverityOf's doc comment
// tells callers not to do. The numeric values are not part of any contract:
// compare with ==, never by order (pinned by TestEnumZeroValuesAreConservative).
const (
	// Warning is advisory: the query may still be exactly what the author
	// meant, and the target CDR is the authority.
	Warning Severity = iota
	// Error means a static defect provable from the relation.
	Error
)

// severities is the ONE code→severity table for the feature (REQ-161
// § Checks). The read-side adapter reads it through [SeverityOf] to fill its
// own [lint.Issue].Severity field. The write-side adapter never calls
// [SeverityOf]: it emits [contain.Finding]s, which carry no severity field, so
// it has nothing to fill — its entry point's doc comment (openehr/aql/verify.go)
// restates this table's five containment rows in prose for a consumer who wants
// the severity without a lookup. Either way, no adapter keeps a SECOND
// code→severity mapping of its own.
var severities = map[string]Severity{
	CodeImpossibleContainment:  Error,
	CodeNotContainable:         Error,
	CodeUnknownRMClass:         Warning,
	CodeContainmentByReference: Warning,
	CodeArchetypeClassMismatch: Error,
	// The three portability codes are Warning ONLY: REQ-161 § Checks holds
	// none of them may ever become an Error — the openEHR QUERY specification
	// leaves the behaviour open, so a conformant CDR is free to differ.
	CodeVersionNoPredicate:          Warning,
	CodeVersionedObjectUnreferenced: Warning,
	CodeFanoutRowGrain:              Warning,
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

// Role is the POSITION a class expression occupies in the query — is it a FROM
// root, or an operand some CONTAINS introduced?
//
// It decides one thing only: whether [Operand.Findings] may raise
// CodeNotContainable. REQ-161 § Checks scopes that code to a CONTAINS operand,
// and a FROM root is the containment anchor rather than a containment target.
//
// Role is a property of the position, never of the node's SHAPE in whatever
// tree an adapter happens to walk. Assign it with [Role.Next] rather than by
// hand — see that method for why the distinction is load-bearing.
//
// RoleRoot is FIRST so the zero Role is the SILENT position — the one that
// raises no containability code at all. Same polarity as [Severity] and [Step];
// pinned by TestEnumZeroValuesAreConservative.
type Role int

const (
	// RoleRoot is a FROM-root operand — the containment anchor, introduced by
	// no CONTAINS keyword.
	RoleRoot Role = iota
	// RoleContained is an operand a CONTAINS / NOT CONTAINS keyword introduced,
	// at any depth.
	RoleContained
)

// Step is how an adapter reached an operand from the position above it.
//
// StepJunction is FIRST so the zero Step is the INERT one: [Role.Next] leaves
// the role unchanged for it, so a zero-valued Step promotes nothing. The acting
// step must not be the default — a zero Step that meant StepContains would turn
// a FROM root into a RoleContained operand and could raise a false
// aql_contains_not_containable on the anchor position REQ-161 § Checks
// explicitly exempts, which REQ-161 § Flagging policy names as the worse of the
// two errors. Same polarity as [Role] and [Severity]; pinned by
// TestEnumZeroValuesAreConservative.
type Step int

const (
	// StepJunction is a boolean junction operand — an arm of an AND / OR, which
	// is not a containment step at all.
	StepJunction Step = iota
	// StepContains is a CONTAINS / NOT CONTAINS keyword.
	StepContains
)

// Next is the role of an operand reached from a position holding role r by step
// s. It is the WHOLE role-assignment rule, and it lives here — not in either
// adapter — because REQ-162 § Contract makes an identical code multiset a MUST
// and a divergent assignment would break that parity silently:
//
//   - StepContains yields RoleContained, at any depth. A CONTAINS keyword
//     introduces a containment target whatever introduced its ancestor.
//   - StepJunction yields r UNCHANGED. A junction is boolean grouping, not a
//     containment step, so its operands occupy the same position the junction
//     itself occupies. `FROM A a OR B b` therefore holds two ROOT operands —
//     neither is introduced by a CONTAINS — while `x CONTAINS (A a OR B b)`
//     holds two contained ones. The inheritance composes, so a junction nested
//     inside a junction keeps propagating the enclosing position.
//
// The rule an adapter must NOT use is tree shape: "this node is a junction's
// child, therefore it is a containment child" makes the two spellings of a root
// position — `FROM DV_TEXT t` and `FROM DV_TEXT t OR COMPOSITION c` — disagree,
// and only one of them can be right. Root positions are silent for
// CodeNotContainable in EVERY spelling: REQ-161 § Checks authorises the code
// only for a CONTAINS operand, and REQ-161 § Flagging policy is explicit that a
// false Error is worse than a missed defect. (An unknown root stays covered
// either way — CodeUnknownRMClass fires on any class token, in any position —
// so the silence is narrowly the known-but-non-containable root.)
func (r Role) Next(s Step) Role {
	if s == StepContains {
		return RoleContained
	}
	return r
}

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
//
// It is deliberately OPAQUE: no accessor for the class name, none for the raw
// verdict, and the suppression test is unexported. This package's own invariant
// is that "neither adapter classifies a verdict of its own" — handing an adapter
// the raw [contain.Verdict] to branch on is exactly what the shared engine
// exists to prevent, and exposing the class name and the suppression test would
// hand it the pieces to re-derive the REQ-161 suppression rule for itself. All
// three are re-exported in export_test.go for the external test package, which
// is the only caller that ever needed them.
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

// suppresses reports whether this operand suppresses the pair checks for every
// pair it participates in.
//
// REQ-161 § Checks: an operand whose verdict is UnknownClass, or whose
// containability is Never, is reported ONCE through its own operand-level code,
// and no pair code is built on it — "one finding per defect, and no Error built
// on an unknown name". [contain.Relation.CanContain] is total and would answer
// such a pair UnknownClass or Never all by itself; mapping that answer into a
// pair code is exactly the double-report the rule forbids, which is why
// [Checker.Pair] consults this first.
//
// This is the FOURTH [contain.Verdict] dispatch in the package, spelled as a
// single inequality rather than a switch. It is fail-safe by construction under
// REQ-160 § Extensibility: a verdict this build does not know is not Admissible,
// so it SUPPRESSES — no pair code is built on an operand nobody could classify,
// which is the conservative direction REQ-161 § Flagging policy asks for. The
// widening still has to be noticed rather than absorbed, and
// TestContainVerdictVocabularyIsPinned is what notices it.
func (o Operand) suppresses() bool { return o.verdict != contain.Admissible }

// Findings are this operand's own findings — at most one:
//
//   - CodeUnknownRMClass (Warning) when the relation does not know the class.
//     Unknown is not wrong: a future RM release, a demographic deployment or a
//     dialect are legitimate sources of an unknown name, so it is never an Error
//     and never silence (REQ-161 § Flagging policy).
//   - CodeNotContainable (Error) when the class is known but is not a
//     containment target, and the operand is a CONTAINS operand.
//
// A ROOT-role operand whose class is not containable yields NO finding, in
// every spelling of the root position — the single `FROM DV_TEXT t` and each
// operand of a root junction `FROM DV_TEXT t OR COMPOSITION c` alike, which is
// what [Role.Next] exists to keep consistent. REQ-161 § Checks scopes
// CodeNotContainable to a CONTAINS operand, and the catalogue authorises no code
// for the anchor position. That is a documented missed defect — the conservative
// direction (REQ-161 § Flagging policy) — not an Error invented outside the
// catalogue. Such an operand still suppresses the pair checks around it, so no
// Error is built on it either.
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
	default:
		// A verdict this build does not know — REQ-160 § Extensibility reserves
		// the right to widen the vocabulary. SILENCE is the decision: REQ-161's
		// catalogue authorises no code for it, and § Flagging policy makes a
		// false Error the worse of the two errors. The widening itself must not
		// pass unnoticed, so TestContainVerdictVocabularyIsPinned fails when a
		// fifth member appears — this arm is the safe landing, not the answer.
		return nil
	}
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
// It returns nothing when either operand suppresses (see Operand.suppresses),
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
	if ancestor.suppresses() || descendant.suppresses() {
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
	default:
		// A widened REQ-160 vocabulary (§ Extensibility): silence, for the same
		// reason as [Operand.Findings]'s default — no catalogue code covers it,
		// and REQ-161 § Flagging policy forbids inventing one. Guarded by
		// TestContainVerdictVocabularyIsPinned.
		return nil
	}
}

// Archetype decides a literal-OR-parameter archetype predicate against the
// class it is attached to (REQ-161 § Checks, REQ-160 § Archetype/class
// conformance): rmType is the declared class's RM type name, archetypeID the
// predicate text exactly as either adapter's carrier already holds it — a
// literal HRID, or a `$param` placeholder. It is orthogonal to [Role] — a FROM
// root's archetype predicate is checked exactly like a CONTAINS operand's —
// so, unlike [Checker.Operand], it takes no role.
//
// The `$param` skip lives HERE, not in either adapter, even though the read
// side additionally holds a typed signal
// ([parse.ClassExpr.ParamArchetype]) it could branch on instead: the write
// side's carrier has no such flag — [aql.Containment] stores only the bare
// archetypeID string — so a write-side caller passing that field straight
// through (the exact usage this string-only signature exists for, REQ-162
// § Contract) would otherwise feed "$arch" to [contain.Relation.ArchetypeMatches],
// fail to parse it as an HRID, and manufacture an aql_unknown_rm_class the
// read side never emits — a code-multiset divergence REQ-162 § Contract makes
// a MUST-not. Detecting the sigil is Archetype's OWN business, by the SAME
// test the builder already applies to the same field
// (`strings.CutPrefix(c.archetypeID, "$")`, openehr/aql/containment.go's
// validateTree) — not HRID decomposition, so it does not breach the
// no-lexing rule. A `$param` carries no scope until execution binds it
// (PROBE-021), so it is simply not this method's business, in either
// direction: no mismatch, no unknown-class warning, nothing.
//
// The whole RM question otherwise belongs to [contain.Relation.ArchetypeMatches]
// — no HRID lexing happens here, or anywhere in this package (REQ-160
// § Archetype/class conformance mandates the single canonical
// [rm.ParseArchetypeID], which ArchetypeMatches already delegates to). This
// method only classifies ArchetypeMatches's verdict into a REQ-161 code and
// enforces the suppression rule (REQ-161 § Checks — "one finding per defect,
// and no Error built on an unknown name"):
//
//   - archetypeID begins with `$` → no finding (see above).
//   - Otherwise, rmType itself already [contain.UnknownClass] under
//     [contain.Relation.Containable] — the SAME verdict [Checker.Operand]'s
//     class-token arm classifies — yields NO finding here. That arm already
//     reported [CodeUnknownRMClass] for this class expression; REQ-161 is
//     explicit that an unknown name is reported once, not twice, so this
//     method must not add a second finding for it.
//   - Otherwise, [contain.Never] (a genuine mismatch between two known
//     classes) → [CodeArchetypeClassMismatch] (Error): the query can never
//     match.
//   - Otherwise, [contain.UnknownClass] from ArchetypeMatches itself. This
//     verdict has THREE live causes, only two of them archetype-side: the
//     HRID's type segment names a class the relation does not know; the
//     HRID is unparseable; or — declared-class-side — rmType passed the
//     suppression check above (it IS [contain.Admissible] under
//     [contain.Relation.Containable]) yet ArchetypeMatches still cannot
//     resolve it in the pinned BMM. That third case is the overlay-only
//     class: one the relation knows solely as an overlay-edge endpoint,
//     which Containable admits but ArchetypeMatches's BMM-only resolution
//     does not consult. (A fourth combination — rmType unknown to BOTH
//     Containable and the BMM — never reaches this branch: it is caught by
//     the suppression check above, and already reported by the class-token
//     arm.) → [CodeUnknownRMClass] (Warning). This is the "second arm" of
//     that code: the same code as the class-token arm, but firing for a
//     different reason, still exactly once per class expression. Which of
//     the three causes applies is deliberately NOT distinguished in the
//     Detail text below: ArchetypeMatches collapses all of them into one
//     verdict, and the Detail is worded to stay true regardless of which
//     one applies.
//   - [contain.Admissible] → no finding.
//
// An empty rmType names nothing (mirrors [Checker.Operand]: a degenerate
// caller with no declared class has nothing to check the archetype against)
// and yields no finding — REQ-025: no panic, no finding manufactured on
// caller input.
func (c Checker) Archetype(rmType, archetypeID string) []contain.Finding {
	if rmType == "" {
		return nil
	}
	if strings.HasPrefix(archetypeID, "$") {
		return nil
	}
	if c.relation().Containable(rmType) == contain.UnknownClass {
		return nil
	}
	switch c.relation().ArchetypeMatches(rmType, archetypeID) {
	case contain.Never:
		return []contain.Finding{{
			Code: CodeArchetypeClassMismatch,
			Detail: fmt.Sprintf("archetype %s does not conform to declared class %s,"+
				" so this class expression can never match", archetypeID, rmType),
		}}
	case contain.UnknownClass:
		return []contain.Finding{{
			Code: CodeUnknownRMClass,
			Detail: fmt.Sprintf("archetype %s's conformance to declared class %s cannot be"+
				" decided from the pinned RM, so it is unchecked; unknown is not wrong — a"+
				" future RM release, a demographic deployment, or a dialect may define the"+
				" names involved, and a malformed archetype id would land here too", archetypeID, rmType),
		}}
	case contain.Admissible, contain.ByReference:
		return nil // ByReference is unreachable: ArchetypeMatches never returns it
	default:
		// A widened REQ-160 vocabulary (§ Extensibility): silence, for the same
		// reason as [Operand.Findings]'s default — no catalogue code covers it,
		// and REQ-161 § Flagging policy forbids inventing one. Guarded by
		// TestContainVerdictVocabularyIsPinned.
		return nil
	}
}

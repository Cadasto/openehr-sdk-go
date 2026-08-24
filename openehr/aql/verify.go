package aql

import (
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/internal/semcheck"
)

// VerifyContainment reports the REQ-161 containment findings of the FROM root
// and containment algebra this builder has accumulated — the OPT-IN RM-semantics
// gate of REQ-162 § Contract, for a caller that wants to check a query it has
// just constructed before submitting it.
//
// It walks the builder's OWN tree: no emission, no re-parse, so it needs neither
// a built [Query] nor the parser. r is the REQ-160 containment relation to judge
// against; nil means the REQ-160 default relation, so a caller with no dialect
// overlay edges passes nil. A relation from [contain.Relation.WithOverlay]
// retires the findings that deployment's extra containment routes make false.
//
// FIVE codes can come back — the containment subset of the REQ-161 catalogue.
// Three are static defects PROVABLE from the relation, and REQ-161 § Checks
// classifies them as Errors:
//
//   - aql_impossible_containment — no containment route connects the pair.
//   - aql_contains_not_containable — a CONTAINS names a class that is no
//     containment target at all.
//   - aql_archetype_class_mismatch — the archetype HRID's type segment does not
//     conform to the class it is attached to.
//
// The other two are advisory, and REQ-161 § Checks classifies them as Warnings —
// unknown is not wrong, and a reference hop is engine-specific:
//
//   - aql_unknown_rm_class — the relation does not know the class.
//   - aql_containment_by_reference — the pair resolves only across a reference
//     hop.
//
// REQ-161 § Checks is the normative home of both the severities and the firing
// rules; the split is repeated here only because a [contain.Finding] carries no
// severity field to look one up from. REQ-161 § Flagging policy governs when they
// change, and this method adds none.
//
// REQ-161's three PORTABILITY advisories
// (aql_version_no_predicate, aql_versioned_object_unreferenced,
// aql_fanout_row_grain) are read-side only and never appear here: they are
// advisories about how a CDR may READ a legal query, scoped by REQ-162
// § Contract to the read side (PROBE-097 § parity).
//
// The returned [contain.Finding]s carry a value-free Code and a value-bearing
// Detail, and deliberately no Span, Path, or severity: a builder tree has no
// source text to point into, and each code's severity is fixed once, in
// REQ-161's catalogue. Dispatch on Code.
//
// Three things this method is NOT:
//
//   - It is not part of [Builder.Build]. Build's validation set, its error
//     texts, and its emitted bytes are unchanged by REQ-162 — a query carrying
//     an RM-impossible containment still builds and still emits the same string
//     it emitted before this method existed. Verification never runs implicitly;
//     the SDK does not decide for a caller that a Never pair is a defect rather
//     than a deliberate probe.
//   - It is not a grammar check. Build answers the SHAPE question (is this
//     representable AQL?); this answers the RM question (can these classes
//     actually contain one another?). The two are independent: a query can pass
//     either and fail the other, and this method judges whatever tree it is
//     given without re-running any of Build's refusals.
//   - It is not a rule of its own. Every verdict→code decision, the suppression
//     rule between the operand codes and the pair codes, the `$param` archetype
//     skip, and the role-assignment rule all live in
//     openehr/aql/internal/semcheck, shared verbatim with the read-side lint
//     adapter. REQ-162 § Contract makes an IDENTICAL code multiset across the
//     two a MUST, and that only holds because neither side classifies anything
//     itself (pinned by TestReadWriteParity).
//
// A nil *Builder reports nothing rather than panicking: there is no tree to
// verify, which is a clean answer, and library code must not panic on caller
// input (REQ-025).
func (b *Builder) VerifyContainment(r *contain.Relation) []contain.Finding {
	if b == nil {
		return nil
	}
	return b.ast.verifyContainment(r)
}

// verifyContainment is [Builder.VerifyContainment] over the shared, unexported
// query tree, so the verb-functions style reaches it through the same *Builder.
//
// The traversal mirrors the shapes [validateContainsChain] and
// [Containment.validateTree] already walk (a.from plus a.contains, then each
// class node's own chain, pre-order, depth-first) so the two walks cannot
// disagree about tree SHAPE — but it re-runs none of their validation: they
// answer the grammar question and this answers the RM one.
func (a *ast) verifyContainment(r *contain.Relation) []contain.Finding {
	v := containVerifier{ck: semcheck.New(r)}
	// The FROM root is the ancestor of the first CONTAINS term. RoleRoot: the
	// anchor of a containment tree is introduced by no CONTAINS keyword, so it
	// raises no aql_contains_not_containable ([semcheck.Role]).
	//
	// A missing FROM (a builder Build would refuse for having no source) leaves
	// the zero Operand, which names nothing and suppresses every pair it takes
	// part in — the contained terms are still checked on their own account. The
	// write-side fromClause carries no archetype field at all, so the root has
	// no archetype predicate to check: the empty archetypeID below is the
	// carrier's shape, not a dropped case.
	var root semcheck.Operand
	if a.from != nil {
		root = v.classNode(a.from.rmType, "", semcheck.RoleRoot)
	}
	v.chain(root, semcheck.RoleRoot, a.contains)
	return v.findings
}

// containVerifier accumulates the containment findings of one builder tree.
type containVerifier struct {
	ck       semcheck.Checker
	findings []contain.Finding
}

// chain walks ONE CONTAINS chain — the top-level a.contains entries under the
// FROM root, or a class node's own children — and returns the operand the chain
// ENDS on, which is what a further term appended after it would follow.
//
// Adjacency advances ALONG the chain rather than fanning out from its head,
// because that is what the emitted text says: [ast.build] writes each entry of
// a.contains after the previous one with a CONTAINS keyword, and
// [Containment.emit] does the same for a class node's children, so
// `Contains(X)` then `Contains(Y)` emits `… CONTAINS X CONTAINS Y` — Y follows
// X, not the root. The pair question is asked of ADJACENT operands only:
// reachability composes, so synthesising a transitive pair would assert
// something the query does not (REQ-161 § Checks).
//
// The tail is the FLATTENED chain's tail, not the entry itself: the builder
// nests (a class node may carry several children, and several top-level
// Contains calls stack) while the emitted chain is flat, so the operand a
// following term is adjacent to is the last class node of the whole preceding
// subtree. `Contains(B.Contains(D))` then `Contains(C)` emits
// `… CONTAINS B CONTAINS D CONTAINS C`, and C's ancestor is D. (The read-side
// extractor nests exactly one level per CONTAINS keyword, so its own walk never
// meets the flattened spelling; the tree shapes differ, the pairs must not.)
//
// role is the position the ENCLOSING node occupies; each entry is reached from
// it by a CONTAINS keyword, so the step is applied here once, through
// [semcheck.Role.Next] rather than by hand.
//
// A junction never becomes a predecessor: it is parenthesised in the emitted
// text and the grammar admits no `(A OR B) CONTAINS C`, so nothing can follow
// one with a CONTAINS keyword ([validateContainsChain] refuses a tree that
// tries). The guard keeps the tail on the last CLASS operand for the trees that
// do — verification judges whatever tree it is handed, including one Build would
// refuse.
func (v *containVerifier) chain(parent semcheck.Operand, role semcheck.Role, chain []Containment) semcheck.Operand {
	prev := parent
	for _, c := range chain {
		if decided := v.walk(prev, role.Next(semcheck.StepContains), c); !c.isJunction() {
			prev = decided
		}
	}
	return prev
}

// walk visits one containment node and returns the operand its chain ends on
// (see [containVerifier.chain]).
//
// parent is the enclosing class operand every pair formed AT this node is asked
// against; it is threaded down because a [Containment] carries no parent
// pointer. A junction is boolean grouping, not a containment step, so it passes
// its own parent through to each operand UNTOUCHED: `A CONTAINS (B OR C)` asks
// A→B and A→C, never junction→B (REQ-161 § Checks).
//
// role is the position this node occupies. A junction's operands INHERIT the
// junction's role — [semcheck.Role.Next] owns that rule so the read-side
// adapter cannot pick a different answer (REQ-162 § Contract); assigning it
// from tree shape here is exactly the parity break the shared engine exists to
// prevent. On today's write side the inheritance is not yet DISTINGUISHABLE
// from labelling a junction's operands contained: the builder has no FROM-root
// junction entry point ([Containment]'s doc comment — the write side keeps a
// single root class), so every junction it can build already sits below a
// CONTAINS keyword and its operands are contained either way. The call goes
// through [semcheck.Role.Next] regardless, because the rule is what must be
// shared, not the coincidence that one of its two arms is currently
// unreachable here — the read side, which CAN see a root junction, exercises
// the other arm.
//
// The `negated` flag is deliberately not consulted. REQ-161 § Checks has a
// NOT CONTAINS pair checked IDENTICALLY to a plain one: an RM-impossible pair is
// impossible whichever sign it carries, and an impossible exclusion is a dead
// constraint — equally a defect, not a licence to stay silent.
func (v *containVerifier) walk(parent semcheck.Operand, role semcheck.Role, n Containment) semcheck.Operand {
	if n.isJunction() {
		for _, ch := range n.children {
			v.walk(parent, role.Next(semcheck.StepJunction), ch)
		}
		// A junction has no class of its own, so it decides nothing and can be
		// nobody's ancestor; the zero Operand says exactly that.
		return semcheck.Operand{}
	}
	self := v.classNode(n.rmType, n.archetypeID, role)
	v.findings = append(v.findings, v.ck.Pair(parent, self)...)
	return v.chain(self, role, n.children)
}

// classNode decides one class expression and records its operand-level
// findings: the REQ-161 class-token check (aql_unknown_rm_class /
// aql_contains_not_containable) and, when the node carries an archetype
// predicate, the REQ-161 archetype/class-conformance check
// (aql_archetype_class_mismatch, or the second arm of aql_unknown_rm_class).
//
// Both run from the SAME call, once per node, which is what makes REQ-161's
// "once per class expression" rule hold structurally: [containVerifier.walk]
// visits a class node exactly once.
//
// archetypeID is passed through RAW. A `$param` predicate must be skipped —
// nothing is bound until execution (PROBE-021) — and that skip lives in
// [semcheck.Checker.Archetype], not here: this carrier has no typed param flag
// (unlike the read side's parse.ClassExpr.ParamArchetype), and a second sigil
// test on this side is precisely how the two adapters would drift.
func (v *containVerifier) classNode(rmType, archetypeID string, role semcheck.Role) semcheck.Operand {
	o := v.ck.Operand(rmType, role)
	v.findings = append(v.findings, o.Findings()...)
	if archetypeID != "" {
		v.findings = append(v.findings, v.ck.Archetype(rmType, archetypeID)...)
	}
	return o
}

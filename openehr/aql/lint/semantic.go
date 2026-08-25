package lint

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/internal/semcheck"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// semanticIssues runs the Layer-2 semantic containment checks (REQ-161
// § Checks): aql_unknown_rm_class and aql_contains_not_containable per class
// expression, aql_impossible_containment and aql_containment_by_reference per
// adjacent FROM/CONTAINS pair, aql_archetype_class_mismatch (plus the second
// arm of aql_unknown_rm_class) per literal archetype predicate, and the three
// REQ-161 portability advisories: aql_version_no_predicate (SPECPR-481),
// aql_versioned_object_unreferenced (Discourse #14186), and
// aql_fanout_row_grain (SPECQUERY-9). rel may be nil (the REQ-160 default
// relation).
//
// This function is an ADAPTER, not a rule engine. Every verdict→code decision,
// and the suppression rule between the operand codes and the pair codes, lives
// in openehr/aql/internal/semcheck — shared verbatim with the write-side
// builder verification, which REQ-162 § Contract holds to an identical code
// multiset (a subset that the three portability advisories are NOT part of —
// REQ-162 § Contract scopes read/write parity to the containment-check codes
// alone). What is added here is what only the read side has: the Span on the
// offending class expression, the Path spelling, and the severity lookup.
//
// The containment-pair checks read the STRUCTURED tree
// ([parse.Document.Query]), not the flat [parse.Document.Classes]: the flat
// view is the FROM/CONTAINS tree collapsed to document order and carries no
// nesting at all, so it cannot say which operands are adjacent, which sit
// under a junction, or what is negated. A non-nil [parse.Document.QueryErr]
// is deliberately NOT fatal here — the structured AST is best-effort by
// contract (REQ-119), and a shape the extraction catalogue dropped simply
// goes unchecked, which is the conservative direction.
//
// aql_version_no_predicate and aql_versioned_object_unreferenced need none of
// that nesting — each is scoped to a single class expression's own fields —
// so they run unconditionally over the flat view, structured extraction
// notwithstanding; only aql_fanout_row_grain, which asks about junction
// SHAPE, is gated on a successful [parse.Document.Query] the same way the
// containment-pair checks are.
func semanticIssues(doc *parse.Document, rel *contain.Relation) []Issue {
	ck := semcheck.New(rel)
	var issues []Issue

	// REQ-161 § Checks: pure per-class-expression checks, independent of
	// containment nesting — see [versionPredicateIssues] and
	// [versionedObjectIssues]. versionedObjectIssues takes no relation: the
	// VERSIONED_OBJECT conformance question it asks is answered straight from
	// the pinned BMM ([rminfo.Default]), not from rel — see that function's
	// doc comment for why that is correct rather than a shortcut.
	issues = append(issues, versionPredicateIssues(doc)...)
	issues = append(issues, versionedObjectIssues(doc)...)

	if q := doc.Query(); q != nil {
		c := containCheck{ck: ck}
		from := q.From

		// The FROM root participates: it is the ancestor of each of its direct
		// CONTAINS children. RoleRoot, because the anchor of a containment tree
		// is not itself a CONTAINS operand (REQ-161 § Checks).
		root := c.operand(from.Root, semcheck.RoleRoot)
		if from.Contains != nil {
			// Reached from the root by a CONTAINS keyword, so the subtree starts
			// in the contained position ([semcheck.Role.Next]).
			c.walk(root, semcheck.RoleRoot.Next(semcheck.StepContains), *from.Contains)
		}
		if from.Junction != nil {
			// A junction AT the FROM root (`FROM A a OR B b`) leaves Root zero.
			// Two consequences, both handled by what is passed in here rather
			// than by a special case:
			//
			//   - its operands have no enclosing parent, so there is no pair to
			//     ask about — the zero Operand says exactly that and suppresses
			//     every pair;
			//   - the junction OCCUPIES the root position, so its operands are
			//     roots too. Passing RoleRoot (not RoleContained) is what makes
			//     `FROM DV_TEXT t OR COMPOSITION c` answer the same as the
			//     single root `FROM DV_TEXT t`: a root is a root in every
			//     spelling.
			c.walk(semcheck.Operand{}, semcheck.RoleRoot, *from.Junction)
		}
		issues = append(issues, c.issues...)
	}

	// aql_fanout_row_grain (REQ-161 § Checks, SPECQUERY-9) needs the
	// structured junction tree, so [fanoutIssues] performs its own
	// [parse.Document.Query] lookup and degrades to silence exactly like the
	// pair walk above when structured extraction did not succeed.
	issues = append(issues, fanoutIssues(doc)...)
	return issues
}

// containCheck accumulates the semantic issues of one document.
type containCheck struct {
	ck     semcheck.Checker
	issues []Issue
}

// walk visits one containment node and returns the operand its OWN downward
// chain ends on (the zero Operand for a junction node, which has no class of
// its own and can be nobody's ancestor) — see the Children comment below for
// why the tail, not the node itself, is what a following sibling must be
// checked against.
//
// TWO facts are threaded down, because a [parse.Containment] carries neither a
// parent pointer nor its own position in the query, and both are unrecoverable
// from the node after the fact:
//
// parent is the enclosing class operand every pair formed AT this node is asked
// against. REQ-161 § Checks requires a junction operand to be checked against
// the junction's ENCLOSING parent rather than against the junction: `A CONTAINS
// (B OR C)` asks A→B and A→C, so the junction node passes its own parent through
// to each operand untouched.
//
// role is the POSITION this node occupies — see [semcheck.Role.Next], which owns
// the assignment rule so the write-side adapter cannot pick a different answer
// (REQ-162 § Contract). A junction's operands INHERIT the junction's role rather
// than being labelled contained for being junction children, so a root junction
// holds roots. Assigning by tree shape here is what made the two spellings of a
// root position disagree.
//
// Both threadings compose the same way at depth: same-operator junctions arrive
// pre-flattened (`A OR B OR C` is ONE node with three operands, REQ-117) while
// mixed AND/OR nesting does not, so a junction operand may itself be a junction.
func (c *containCheck) walk(parent semcheck.Operand, role semcheck.Role, n parse.Containment) semcheck.Operand {
	if isJunction(n) {
		for _, ch := range n.Children {
			c.walk(parent, role.Next(semcheck.StepJunction), ch)
		}
		return semcheck.Operand{}
	}

	self := c.operand(n.Class, role)
	c.pair(parent, self, n.Class)

	// A class node's Children are the CONTAINS chain below it, in order — each
	// child follows the PREVIOUS one with a CONTAINS keyword
	// ([parse.Containment.Children]), so adjacency advances along the chain
	// rather than fanning out from this node. The read-side extractor nests a
	// chain one level per CONTAINS (a class node gets a single child), so the
	// loop normally runs once — but a class node CAN carry several children
	// (the flattened spelling of the same tree, [parse.Containment.Children]'s
	// own doc: the chain shape is not stable under re-parse), and when the
	// first of those children carries its own further chain, prev must track
	// THAT chain's tail, not the child itself, or the next sibling is checked
	// against the wrong predecessor — openehr/aql/verify.go's containVerifier.chain
	// faces the identical fan-in and resolves it the same way. A junction may
	// only END a chain, so it never becomes a predecessor.
	prev := self
	for _, ch := range n.Children {
		if decided := c.walk(prev, role.Next(semcheck.StepContains), ch); !isJunction(ch) {
			prev = decided
		}
	}
	return prev
}

// operand decides one class expression and records its operand-level issues:
// the REQ-161 class-token check (aql_unknown_rm_class / aql_contains_not_containable,
// via [semcheck.Checker.Operand]) and, when ce carries a literal archetype
// predicate, the REQ-161 archetype/class-conformance check
// (aql_archetype_class_mismatch, or the second arm of aql_unknown_rm_class,
// via [semcheck.Checker.Archetype]).
//
// Both checks run from the SAME call, once per class expression — which is
// what makes the "once per class-expression occurrence" rule (REQ-161
// § Checks) hold structurally rather than by bookkeeping: a class node is
// visited exactly once by [containCheck.walk], so operand is called exactly
// once for it, and semcheck's own suppression (an already-unknown declared
// class silences [semcheck.Checker.Archetype] before it runs) keeps a single
// defect from producing two findings.
//
// A `$param` archetype predicate is skipped — the CDR resolves the bound
// scope at execution (PROBE-021), the same skip [templateIssues] applies to
// aql_path_not_in_template (resolve.go).
func (c *containCheck) operand(ce parse.ClassExpr, role semcheck.Role) semcheck.Operand {
	o := c.ck.Operand(ce.RMType, role)
	for _, f := range o.Findings() {
		c.issues = append(c.issues, semanticIssue(f, ce))
	}
	if ce.Archetype != "" {
		for _, f := range c.ck.Archetype(ce.RMType, ce.Archetype) {
			c.issues = append(c.issues, semanticIssue(f, ce))
		}
	}
	return o
}

// pair asks the pair question and records its issues against the DESCENDANT's
// class expression: the descendant is the operand the CONTAINS introduces, so
// that is where a reader looks for the offending step. ce is the descendant's
// class expression, carried separately because semcheck deals in class names and
// holds no position.
func (c *containCheck) pair(ancestor, descendant semcheck.Operand, ce parse.ClassExpr) {
	for _, f := range c.ck.Pair(ancestor, descendant) {
		c.issues = append(c.issues, semanticIssue(f, ce))
	}
}

// isJunction reports whether n is a boolean junction node rather than a class
// node. A junction is distinguished by an EMPTY class RM type plus operands —
// the same discriminator the parse package's own containment validation uses;
// there is no stored kind flag. A node with neither a class nor children is
// degenerate (a dropped operand, REQ-119); it is treated as a class node, where
// the empty RM type decides nothing and reports nothing.
func isJunction(n parse.Containment) bool {
	return n.Class.RMType == "" && len(n.Children) > 0
}

// semanticIssue wraps one semcheck finding into a lint issue, adding the three
// things a [contain.Finding] deliberately does not carry (REQ-161
// § Additivity): the Span on the offending class expression, the Path, and the
// severity.
func semanticIssue(f contain.Finding, ce parse.ClassExpr) Issue {
	return Issue{
		Code:     f.Code,
		Path:     classToken(ce),
		Detail:   f.Detail,
		Severity: severityOf(f.Code),
		// The class expression's Pos starts at its RM type token, so the span
		// covers the class name itself — not the alias or predicate that may
		// follow it. Value-free: line and column numbers only.
		Span: spanOfText(ce.Pos, ce.RMType),
	}
}

// severityOf maps a REQ-161 code to its lint severity. The catalogue is
// semcheck's single table (REQ-161 § Checks), shared with the write side; a code
// that table does not know degrades to Warning rather than manufacturing an
// Error the relation never proved (REQ-161 § Flagging policy).
func severityOf(code string) Severity {
	s, ok := semcheck.SeverityOf(code)
	if !ok || s != semcheck.Error {
		return Warning
	}
	return Error
}

// classToken renders a class expression for the value-bearing [Issue.Path] —
// the RM type plus its alias when it has one (`OBSERVATION o`), mirroring how
// the write side renders one node of a containment tree.
func classToken(ce parse.ClassExpr) string {
	if ce.Alias == "" {
		return ce.RMType
	}
	return ce.RMType + " " + ce.Alias
}

// --- REQ-161 portability advisories -----------------------------------------
//
// The three checks below are portability warnings, not RM-correctness
// checks: the AQL is legal and well-formed, but the openEHR QUERY
// specification leaves its behaviour open, so a conformant CDR is free to
// differ. None of the three may ever become an Error (REQ-161 § Checks).

// versionPredicateIssues raises aql_version_no_predicate (REQ-161 § Checks;
// [SPECPR-481](https://openehr.atlassian.net/browse/SPECPR-481)) for every
// VERSION class expression carrying no version predicate at all: the tier a
// bare VERSION defaults to is unspecified, so a portable query SHOULD state
// [LATEST_VERSION] or [ALL_VERSIONS] explicitly.
//
// It fires on ABSENCE of any predicate ([parse.ClassExpr.HasPredicate]
// false), never on which predicate is present — a VERSION carrying some
// OTHER version predicate (a standing comparison the `versionPredicate`
// grammar rule also admits) has already made an explicit choice, however it
// spelled it, so REQ-161's "no version predicate" condition does not reach
// it.
//
// This is a pure walk of the flat [parse.Document.Classes]: the code is
// scoped to a single class expression's own fields, needs no containment
// nesting, and so needs no [contain.Relation] either.
func versionPredicateIssues(doc *parse.Document) []Issue {
	var issues []Issue
	for _, ce := range doc.Classes {
		if !ce.Version || ce.HasPredicate {
			continue
		}
		issues = append(issues, Issue{
			Code: semcheck.CodeVersionNoPredicate,
			Path: classToken(ce),
			Detail: "VERSION class expression carries no version predicate; the tier it defaults to is " +
				"unspecified (SPECPR-481), so a portable query SHOULD state [LATEST_VERSION] or " +
				"[ALL_VERSIONS] explicitly",
			Severity: severityOf(semcheck.CodeVersionNoPredicate),
			// ce.Pos starts at the VERSION keyword itself (parse/ast.go's
			// EnterVersionClassExpr), so the span covers the class expression,
			// not its alias or predicate.
			Span: spanOfText(ce.Pos, ce.RMType),
		})
	}
	return issues
}

// versionedObjectIssues raises aql_versioned_object_unreferenced (REQ-161
// § Checks; [Discourse #14186](https://discourse.openehr.org/t/aql-versionedobject-grammar/14186))
// for every class expression whose class conforms to VERSIONED_OBJECT — a
// conformance question, never a `VERSIONED_` name-prefix guess — and whose
// alias roots no identified path anywhere outside FROM/CONTAINS.
//
// The conformance question is asked directly of [rminfo.Default]
// ([conformsToVersionedObject]), not through a [*contain.Relation]. That is
// deliberate, not a shortcut: every [*contain.Relation] a caller can obtain —
// [contain.Default] and every [contain.Relation.WithOverlay] copy of it —
// carries the SAME [rminfo.Lookup] underneath (WithOverlay copies it
// unchanged; the package's only other constructor, `build`, is unexported),
// so there is no caller-suppliable relation this function could consult
// instead that would ever answer differently. An overlay edge states a
// containment ROUTE fact (REQ-160 § Extensibility) — never an IS-A fact — so
// it has nothing to say about VERSIONED_OBJECT conformance either way, and a
// relation parameter here would be decorative.
//
// [parse.Document.Paths] already IS "outside FROM/CONTAINS": an identified
// path only ever occurs in SELECT, WHERE, or ORDER BY
// ([parse.clauseOf] recognises exactly those three); a FROM/CONTAINS class
// predicate is a relative, alias-less object path
// ([parse.ClassExpr.PredicateComparison]'s doc comment), never an
// [parse.IdentifiedPath]. So checking whether the alias appears as any
// path's root there IS the whole "outside FROM/CONTAINS" test — no separate
// exclusion of FROM/CONTAINS itself is needed.
//
// An anonymous operand (no alias at all — AQL's `classExprOperand` grammar
// makes the alias optional) is INCLUDED, not skipped: with no alias, no path
// anywhere can possibly reference it, so "roots no identified path" holds
// unconditionally, and the redundancy the code names is if anything more
// certain, not less.
//
// When the class is not one the pinned BMM knows, [conformsToVersionedObject]
// answers false exactly as it would for a genuine non-conformance, and this
// function stays silent either way — REQ-161 § Flagging policy: unknown is
// not wrong, so a class the BMM cannot place must not be treated as proof of
// anything.
func versionedObjectIssues(doc *parse.Document) []Issue {
	referenced := make(map[string]bool, len(doc.Paths))
	for _, p := range doc.Paths {
		referenced[p.Alias] = true
	}
	var issues []Issue
	for _, ce := range doc.Classes {
		if ce.RMType == "" || !conformsToVersionedObject(ce.RMType) {
			continue
		}
		if ce.Alias != "" && referenced[ce.Alias] {
			continue
		}
		issues = append(issues, Issue{
			Code:     semcheck.CodeVersionedObjectUnreferenced,
			Path:     classToken(ce),
			Detail:   versionedObjectDetail(ce),
			Severity: severityOf(semcheck.CodeVersionedObjectUnreferenced),
			Span:     spanOfText(ce.Pos, ce.RMType),
		})
	}
	return issues
}

// conformsToVersionedObject reports whether rmType's class conforms to
// VERSIONED_OBJECT under the pinned RM ([rminfo.Default]) — REQ-160
// § Containable operands names the concept; this is the one place in the
// lint package that asks it directly rather than through
// [*contain.Relation], for the reason [versionedObjectIssues] explains.
// Unknown classes and known-but-non-conforming classes both answer false;
// REQ-161 § Flagging policy forbids treating an unknown name as proof of
// anything, so the caller must not, and does not need to, tell the two
// apart.
//
// rmType is folded ASCII a-z → A-Z before the lookup: [rminfo.Hierarchy]'s
// map is keyed by the BMM's exact spelling, unlike [*contain.Relation] (which
// folds internally), so this function folds locally to keep this check's
// case-insensitivity consistent with every other REQ-161 code.
// strings.ToUpper is deliberately not used — its Unicode fold can map some
// non-ASCII letters INTO the ASCII alphabet (e.g. 'ı' → 'I'), which would let
// a token that is not a valid RM identifier at all masquerade as one; RM
// class names are ASCII-only, so only ASCII needs folding.
//
// The [rminfo.Hierarchy] type assertion cannot fail against [rminfo.Default]
// (compile-time-checked in that package), but is checked anyway rather than
// asserted blindly — REQ-025: library code must not panic on caller input,
// and "caller input" here is broad enough to include however this function
// might be reached.
func conformsToVersionedObject(rmType string) bool {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		return false
	}
	conf, known := h.ConformsTo(asciiUpperRMType(rmType), "VERSIONED_OBJECT")
	return known && conf
}

// asciiUpperRMType folds only ASCII a-z to A-Z, leaving every other byte
// untouched — see [conformsToVersionedObject]'s doc comment for why
// strings.ToUpper is not used here.
func asciiUpperRMType(s string) string {
	b := []byte(s)
	changed := false
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
			changed = true
		}
	}
	if !changed {
		return s
	}
	return string(b)
}

// versionedObjectDetail names the offending alias, spelling an anonymous
// operand explicitly rather than leaving the sentence read as if an alias
// were merely omitted from the message.
func versionedObjectDetail(ce parse.ClassExpr) string {
	alias := ce.Alias
	if alias == "" {
		alias = "(anonymous)"
	}
	return fmt.Sprintf("%s conforms to VERSIONED_OBJECT but its alias %s roots no identified path outside "+
		"FROM/CONTAINS; the step is redundant unless container-level attributes are read (Discourse #14186)",
		ce.RMType, alias)
}

// fanoutIssues raises aql_fanout_row_grain (REQ-161 § Checks;
// [SPECQUERY-9](https://openehr.atlassian.net/browse/SPECQUERY-9)) — the
// SDK's one deliberately narrow row-semantics-ADJACENT advisory. What a
// result row IS when sibling containments multiply has been an open
// specification question for years and engines still diverge; this check
// warns that the shape exists, and never adjudicates, dedupes, zips, or
// refuses on the strength of it (plan § The big picture).
//
// It fires once per AND containment junction whose AND-flattened
// class-expression leaves ([andFrontier]) include two or more operands each
// projected by at least one SELECT column. Per REQ-161 § Checks, verbatim,
// three clauses:
//
//   - only an AND junction can fire it — an OR junction (or any junction
//     that is not AND) never fires, however many of ITS operands are
//     projected: "alternation selects rows, it does not multiply them";
//   - the counted leaves are found by flattening nested AND junctions,
//     WITHOUT descending into an OR junction — an OR subtree contributes NO
//     leaves to its enclosing AND;
//   - "projected" means the leaf's alias roots at least one SELECT column,
//     checked against [parse.Document.Paths] tagged [parse.ClauseSelect] —
//     the same source [versionedObjectIssues] reads for "outside
//     FROM/CONTAINS", here filtered to one clause rather than excluding one.
//
// A NEGATED subtree never fires an AND junction anywhere inside it, however
// deep — this is fix round 2's correction, and it is INHERITED down the
// walk, not read off each node's own [parse.Containment.Negated] bit in
// isolation. `NOT CONTAINS (B OR (D AND E))` sets Negated only on the OUTER
// OR node (the parser's Negated carrier is the CONTAINS-chain TARGET, which
// here is the whole parenthesised junction); the inner `(D AND E)` is an
// ordinary, un-negated junction OPERAND of that OR — [parse.Containment]'s
// own doc comment records that NOT belongs to a CONTAINS keyword, never to a
// junction operand, so no node BELOW the negated one can carry the flag
// itself. Reading only `n.Negated` at each node (fix round 1's fix) caught
// the negated node ITSELF firing, but let a plain, un-flagged AND several
// junction-operand hops further down still fire — same defect class as
// Important 1, one level deeper, because "is this pair impossible" and "does
// a NOT CONTAINS filter reach this deep" are different questions.
//
// Once a `walk` call is under negation, it stays under negation all the way
// down: the boolean threaded through `walk` is set the moment ANY visited
// node's own Negated is true, and never cleared descending further, exactly
// matching the semantics REQ-161's Important-1 rationale actually states —
// "a NOT CONTAINS collapses to a boolean filter on the PARENT ROW" governs
// the WHOLE excluded subtree, not merely its root spelling. Even though
// [parse.Containment.Negated] is not one of the spec's three named clauses,
// this is the opposite of the rule the containment-pair checks apply:
// REQ-161 § Checks has a NOT CONTAINS pair checked IDENTICALLY to a plain
// one, because an RM-impossible pair is impossible whether or not it is
// negated — POSSIBILITY does not care about the sign, anywhere in the tree.
// Row multiplicity does. Whoever next touches this rule should not "fix"
// this to match the pair checks' NOT-CONTAINS-is-identical stance; the two
// checks answer different questions, and only one of them is sign-blind.
// [containCheck.walk]/[containCheck.pair] (the pair-check machinery) take no
// such flag and MUST NOT: this inheritance is local to the advisory.
//
// TWO shapes this inheritance must NOT touch, both because REQ-161's own
// clauses require them:
//
//   - a NON-negated AND nested under a NON-negated OR still fires — an OR is
//     a leaf-collection BOUNDARY (nothing below it becomes a leaf of the
//     enclosing junction), never a firing boundary; only negation, not
//     alternation, suppresses a descendant AND;
//   - the containment-pair codes (aql_impossible_containment and the rest)
//     still fire inside a negated subtree, exactly as before — this
//     function's own `negated` parameter is invisible to
//     [containCheck.walk], which has and needs none of its own.
//
// This is a pure [parse.Document] walk: no [contain.Relation] is consulted —
// whether two operands' aliases are both SELECT-projected is not an RM
// question, and the containment SHAPE this check reads is a structural fact
// of the parsed query, not a containability verdict.
func fanoutIssues(doc *parse.Document) []Issue {
	q := doc.Query()
	if q == nil {
		return nil
	}
	projected := make(map[string]bool)
	for _, p := range doc.Paths {
		if p.Clause == parse.ClauseSelect {
			projected[p.Alias] = true
		}
	}
	var issues []Issue
	// negated is TRUE once any ancestor node (this one included) carried
	// Negated=true; it is passed down, never reset, and never read back up.
	var walk func(n parse.Containment, negated bool)
	walk = func(n parse.Containment, negated bool) {
		negated = negated || n.Negated
		if !isJunction(n) {
			for _, ch := range n.Children {
				walk(ch, negated)
			}
			return
		}
		if n.ChildJoin != parse.ContainsAnd || negated {
			// An OR (or any non-AND) junction never fires itself, and
			// neither does an AND anywhere under an inherited negation (see
			// the doc comment above) — but one of its operands may hide its
			// own AND junction, at any depth, which still needs the SAME
			// negated flag carried into it.
			for _, ch := range n.Children {
				walk(ch, negated)
			}
			return
		}
		leaves, boundary := andFrontier(n)
		if found := fanoutFinding(leaves, projected); found != nil {
			issues = append(issues, *found)
		}
		// andFrontier already consumed every nested-AND descendant in full —
		// re-walking leaves or boundary as a plain child list would double-
		// count the SAME junction. What it did NOT explore is each leaf's own
		// onward CONTAINS chain (a deeper containment step, not a sibling of
		// THIS junction) and each excluded OR subtree (which may hide its own,
		// independent, AND junction) — both are walked here, so no AND
		// junction anywhere in the tree goes unchecked, and none is checked
		// twice. negated is false in both loops below (this branch is only
		// reached when it was already false), so nothing is lost by not
		// re-deriving it from n.
		for _, leaf := range leaves {
			for _, ch := range leaf.Children {
				walk(ch, negated)
			}
		}
		for _, b := range boundary {
			walk(b, negated)
		}
	}
	if q.From.Contains != nil {
		walk(*q.From.Contains, false)
	}
	if q.From.Junction != nil {
		walk(*q.From.Junction, false)
	}
	return issues
}

// andFrontier flattens n's nested-AND structure — n MUST be a non-negated AND
// junction (checked by the caller, [fanoutIssues]'s walk). leaves are the
// class-expression operands reached without crossing a non-AND (or negated)
// junction (REQ-161's flattening rule, applied regardless of how deep the
// AND-only nesting runs — same-operator junctions arrive pre-flattened by the
// extractor already, REQ-117, but this recurses regardless so a hand-built or
// future unflattened AST is handled identically — see
// [TestAndFrontierFlattensHandBuiltNestedAnd]). boundary is every node the
// flatten explicitly did NOT explore: a non-AND (OR) child, or a NEGATED
// child, returned whole so the caller can look for an AND junction nested
// inside it independently — it contributes nothing to THIS junction's
// leaves, but it is not silently dropped either.
//
// The Negated check is defensive, not reachable through today's only caller:
// REQ-161's flattening rule composes through [n.Children], and [parse.Containment]'s
// own doc comment records that NOT belongs to a CONTAINS keyword, never to a
// junction operand — the parser sets Negated only on the node a CONTAINS/NOT
// CONTAINS chain step targets, so a junction's direct Children (its boolean
// operands) can never individually carry it. n ITSELF can, though — a whole
// `NOT CONTAINS (A AND B)` junction is exactly such a target — which is why
// the check has to live here too, mirroring [fanoutIssues]'s walk, rather
// than only at the call site: a future caller of this function that skips
// walk's own guard must not silently flatten a filter into row-multiplying
// leaves.
//
// This is n's OWN bit only — it says nothing about an ANCESTOR further up
// the tree that was negated (`NOT CONTAINS (B OR (D AND E))` sets Negated on
// the OR, not on the un-negated inner `(D AND E)` this function would be
// called on if [fanoutIssues]'s walk did not carry the inherited flag
// itself; fix round 2). Inheriting a negated ancestor's exclusion down
// through an intervening OR is [fanoutIssues]'s `walk` closure's job, via
// its own threaded `negated bool` parameter — deliberately NOT this
// function's, since andFrontier only ever sees the subtree at or below the
// node walk has already decided is fireable.
func andFrontier(n parse.Containment) (leaves, boundary []parse.Containment) {
	if !isJunction(n) {
		return []parse.Containment{n}, nil
	}
	if n.ChildJoin != parse.ContainsAnd || n.Negated {
		return nil, []parse.Containment{n}
	}
	for _, ch := range n.Children {
		l, b := andFrontier(ch)
		leaves = append(leaves, l...)
		boundary = append(boundary, b...)
	}
	return leaves, boundary
}

// fanoutFinding builds the aql_fanout_row_grain [Issue] for one AND
// junction's leaves, or nil when fewer than two are SELECT-projected —
// REQ-161's ">= 2" threshold, the exact boundary the near-miss pin (one
// projected alias) must not cross.
//
// The Span AND Path both land on the FIRST (document-order) projected leaf:
// a fan-out finding is a joint property of two or more operands together, so
// — unlike every other REQ-161 code — there is no single offending class
// expression to blame, and the first one a reader would meet in the source
// is the most useful anchor. [Issue.Path] is documented singular ("the AQL
// path or class the issue concerns") and every other code's Path names
// exactly the thing its Span covers — a comma-joined list here would be the
// one code that disagrees with its own Span, and REQ-161 § Additivity says
// the new codes reuse the issue model verbatim, not a widened one. The full
// set of projected leaves still appears in Detail, where a list belongs.
func fanoutFinding(leaves []parse.Containment, projectedAliases map[string]bool) *Issue {
	var hits []parse.ClassExpr
	for _, leaf := range leaves {
		if leaf.Class.Alias != "" && projectedAliases[leaf.Class.Alias] {
			hits = append(hits, leaf.Class)
		}
	}
	if len(hits) < 2 {
		return nil
	}
	tokens := make([]string, len(hits))
	for i, ce := range hits {
		tokens[i] = classToken(ce)
	}
	joined := strings.Join(tokens, ", ")
	return &Issue{
		Code: semcheck.CodeFanoutRowGrain,
		Path: classToken(hits[0]),
		Detail: fmt.Sprintf("this AND containment step has %d SELECT-projected sibling operands (%s); row "+
			"multiplicity for this shape is engine-defined (SPECQUERY-9) — verify the result shape against "+
			"the target CDR", len(hits), joined),
		Severity: severityOf(semcheck.CodeFanoutRowGrain),
		Span:     spanOfText(hits[0].Pos, hits[0].RMType),
	}
}

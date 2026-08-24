package lint

import (
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/internal/semcheck"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// semanticIssues runs the Layer-2 semantic containment checks (REQ-161
// § Checks): aql_unknown_rm_class and aql_contains_not_containable per class
// expression, aql_impossible_containment and aql_containment_by_reference per
// adjacent FROM/CONTAINS pair. rel may be nil (the REQ-160 default relation).
//
// This function is an ADAPTER, not a rule engine. Every verdict→code decision,
// and the suppression rule between the operand codes and the pair codes, lives
// in openehr/aql/internal/semcheck — shared verbatim with the write-side
// builder verification, which REQ-162 § Contract holds to an identical code
// multiset. What is added here is what only the read side has: the Span on the
// offending class expression, the Path spelling, and the severity lookup.
//
// It reads the STRUCTURED tree ([parse.Document.Query]), not the flat
// [parse.Document.Classes]: the flat view is the FROM/CONTAINS tree collapsed to
// document order and carries no nesting at all, so it cannot say which operands
// are adjacent, which sit under a junction, or what is negated. A non-nil
// [parse.Document.QueryErr] is deliberately NOT fatal here — the structured AST
// is best-effort by contract (REQ-119), and a shape the extraction catalogue
// dropped simply goes unchecked, which is the conservative direction.
func semanticIssues(doc *parse.Document, rel *contain.Relation) []Issue {
	q := doc.Query()
	if q == nil {
		return nil
	}
	c := containCheck{ck: semcheck.New(rel)}
	from := q.From

	// The FROM root participates: it is the ancestor of each of its direct
	// CONTAINS children. RoleRoot, because the anchor of a containment tree is
	// not itself a CONTAINS operand (REQ-161 § Checks).
	root := c.operand(from.Root, semcheck.RoleRoot)
	if from.Contains != nil {
		c.walk(root, *from.Contains)
	}
	if from.Junction != nil {
		// A junction AT the FROM root (`FROM A a OR B b`) leaves Root zero: its
		// operands have no enclosing parent, so there is no pair to ask about.
		// The zero Operand says exactly that, and suppresses every pair, so the
		// walk needs no special case — only the operand checks fire.
		c.walk(semcheck.Operand{}, *from.Junction)
	}
	return c.issues
}

// containCheck accumulates the semantic issues of one document.
type containCheck struct {
	ck     semcheck.Checker
	issues []Issue
}

// walk visits one containment node and returns the node's own decided operand
// (the zero Operand for a junction node, which has no class of its own).
//
// parent is the enclosing class operand every pair formed AT this node is asked
// against. It is threaded down explicitly because a [parse.Containment] carries
// NO parent pointer, and REQ-161 § Checks requires a junction operand to be
// checked against the junction's ENCLOSING parent rather than against the
// junction: `A CONTAINS (B OR C)` asks A→B and A→C, so the junction node passes
// its own parent through to each operand untouched. Same-operator junctions
// arrive pre-flattened (`A OR B OR C` is ONE node with three operands, REQ-117)
// while mixed AND/OR nesting does not, so a junction operand may itself be a
// junction — which the same pass-through handles at any depth.
func (c *containCheck) walk(parent semcheck.Operand, n parse.Containment) semcheck.Operand {
	if isJunction(n) {
		for _, ch := range n.Children {
			c.walk(parent, ch)
		}
		return semcheck.Operand{}
	}

	self := c.operand(n.Class, semcheck.RoleContained)
	c.pair(parent, self, n.Class)

	// A class node's Children are the CONTAINS chain below it, in order — each
	// child follows the PREVIOUS one with a CONTAINS keyword
	// ([parse.Containment.Children]), so adjacency advances along the chain
	// rather than fanning out from this node. The read-side extractor nests a
	// chain one level per CONTAINS (a class node gets a single child), so the
	// loop normally runs once; walking the chain explicitly keeps the flattened
	// spelling of the same tree correct too. A junction may only END a chain, so
	// it never becomes a predecessor.
	prev := self
	for _, ch := range n.Children {
		if decided := c.walk(prev, ch); !isJunction(ch) {
			prev = decided
		}
	}
	return self
}

// operand decides one class expression and records its operand-level issues.
func (c *containCheck) operand(ce parse.ClassExpr, role semcheck.Role) semcheck.Operand {
	o := c.ck.Operand(ce.RMType, role)
	for _, f := range o.Findings() {
		c.issues = append(c.issues, semanticIssue(f, ce))
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

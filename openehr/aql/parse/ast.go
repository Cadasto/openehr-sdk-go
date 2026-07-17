package parse

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse/gen"
)

// Clause identifies which top-level clause an [IdentifiedPath] appears in. It
// localises a path for diagnostics and lets the lint layer scope checks (e.g.
// SELECT vs WHERE).
type Clause int

const (
	// ClauseUnknown is the zero value — a path whose enclosing clause could
	// not be determined (should not occur for a well-formed query).
	ClauseUnknown Clause = iota
	// ClauseSelect is the SELECT projection list.
	ClauseSelect
	// ClauseWhere is the WHERE predicate.
	ClauseWhere
	// ClauseOrderBy is the ORDER BY list.
	ClauseOrderBy
)

// String renders the clause name for diagnostics.
func (c Clause) String() string {
	switch c {
	case ClauseSelect:
		return "select"
	case ClauseWhere:
		return "where"
	case ClauseOrderBy:
		return "order by"
	}
	return "unknown"
}

// ClassExpr is one class expression bound in the FROM / CONTAINS tree
// (e.g. `OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]`). The
// containment tree is flattened to document order; nesting is not retained
// because the lint contract (REQ-109) reasons over the set of bound classes,
// not their containment shape.
type ClassExpr struct {
	// RMType is the reference-model class name (e.g. "OBSERVATION",
	// "COMPOSITION", "EHR"), or "VERSION" for a VERSION class expression.
	RMType string
	// Alias is the binding variable (e.g. "o"), or "" when anonymous.
	Alias string
	// Archetype is the literal archetype HRID from a containment predicate
	// (e.g. "openEHR-EHR-OBSERVATION.blood_pressure.v1"), or "" when the
	// class carries no archetype predicate (or it is a $param — see
	// ParamArchetype).
	Archetype string
	// ParamArchetype is true when the archetype predicate is a $param
	// placeholder (`[$arch]`) rather than a literal HRID — identifiable
	// scope deferred to bind time.
	ParamArchetype bool
	// Version is true for a VERSION class expression (version machinery,
	// distinct from a clinical RM class).
	Version bool
	// HasPredicate is true when the class carries any path predicate
	// (`[...]`) — an archetype, a standing predicate like `[ehr_id/value=$x]`,
	// or a version predicate. Distinguishes an identifiable EHR/VERSION root
	// from a bare one.
	HasPredicate bool
	// Predicate is the raw text inside the class predicate brackets when
	// HasPredicate is true and the predicate is NOT a literal archetype HRID
	// (which lives on [Archetype]) or a `$param` archetype (signalled by
	// [ParamArchetype]). Carries standing predicates such as
	// `ehr_id/value=$x` so the emitter can round-trip them — brackets
	// stripped, content verbatim from the source.
	Predicate string
	// PredicateComparison is the standing class predicate parsed as a
	// `{path, operator, value}` comparison (e.g. `ehr_id/value = $x`),
	// reusing the shared [aql.Comparison] / [aql.Value] vocabulary
	// (REQ-113). Non-nil only when the predicate is a simple comparison;
	// nil for an archetype HRID (see [Archetype]), a version predicate, or
	// a non-scalar / complex standing predicate — the verbatim [Predicate]
	// text stays authoritative there. The comparison's Path is the relative
	// object path as written, and its ParsedPath carries the same path's
	// structured Segments with an empty Alias (a relative predicate path
	// binds no FROM alias) — the WHERE-side symmetry for the class-predicate
	// left-hand side.
	PredicateComparison *aql.Comparison
	// Pos is the source position of the class expression.
	Pos Position
}

// PathSegment is one step of an identified path — re-exported from
// [aql.PathSegment], the shared path vocabulary (REQ-113).
type PathSegment = aql.PathSegment

// IdentifiedPath is an alias-qualified path referenced in SELECT, WHERE, or
// ORDER BY (e.g. `o/data[at0001]/events[at0006]/value/magnitude`). It embeds
// the shared [aql.IdentifiedPath] (Alias / Predicate / Segments / Raw) — the
// same structured type an [aql.Comparison] carries on the WHERE side, without
// a package cycle (REQ-113) — and adds the parse-only Clause and source
// Position. The embedded fields (Alias, Segments, …) are promoted, so
// existing field access is unchanged.
type IdentifiedPath struct {
	aql.IdentifiedPath
	// Clause is the enclosing top-level clause.
	Clause Clause
	// Pos is the source position of the path.
	Pos Position
}

// extractor is an ANTLR listener that decorates the generated parse tree into
// the package's generated-type-free structures. It runs once per Parse.
type extractor struct {
	*gen.BaseAqlParserListener
	classes   []ClassExpr
	paths     []IdentifiedPath
	params    []string
	seenParam map[string]bool
}

func (d *Document) extract() {
	ex := &extractor{
		BaseAqlParserListener: &gen.BaseAqlParserListener{},
		seenParam:             map[string]bool{},
	}
	antlr.NewParseTreeWalker().Walk(ex, d.tree)
	d.Classes = ex.classes
	d.Paths = ex.paths
	d.Params = ex.params
}

// EnterClassExpression populates [Document.Classes]. Keep in lockstep
// with the structured extractor's extractClassExprOperand
// (extract_query.go) so a consumer reading the flat lint view and the
// structured Query sees identical raw [ClassExpr] fields for the same
// source — RMType / Alias / Archetype / Predicate / HasPredicate,
// including the standing-predicate body and the param-archetype
// placeholder name carried verbatim in Archetype. The structured
// extractor additionally populates [ClassExpr.PredicateComparison],
// which this flat lint view intentionally omits.
func (e *extractor) EnterClassExpression(c *gen.ClassExpressionContext) {
	ce := ClassExpr{Pos: posOf(c.GetStart())}
	if ids := c.AllIDENTIFIER(); len(ids) > 0 {
		ce.RMType = ids[0].GetText()
	}
	if v := c.GetVariable(); v != nil {
		ce.Alias = v.GetText()
	}
	if pp := c.PathPredicate(); pp != nil {
		ce.HasPredicate = true
		switch {
		case pp.ArchetypePredicate() != nil:
			ap := pp.ArchetypePredicate()
			if hrid := ap.ARCHETYPE_HRID(); hrid != nil {
				ce.Archetype = hrid.GetText()
			} else if p := ap.PARAMETER(); p != nil {
				ce.Archetype = p.GetText()
				ce.ParamArchetype = true
			}
		default:
			ce.Predicate = trimBrackets(pp.GetText())
		}
	}
	e.classes = append(e.classes, ce)
}

func (e *extractor) EnterVersionClassExpr(c *gen.VersionClassExprContext) {
	ce := ClassExpr{RMType: "VERSION", Version: true, Pos: posOf(c.GetStart())}
	if v := c.GetVariable(); v != nil {
		ce.Alias = v.GetText()
	}
	if vp := c.VersionPredicate(); vp != nil {
		ce.HasPredicate = true
		ce.Predicate = trimBrackets(vp.GetText())
	}
	e.classes = append(e.classes, ce)
}

func (e *extractor) EnterIdentifiedPath(c *gen.IdentifiedPathContext) {
	ip := IdentifiedPath{Pos: posOf(c.GetStart()), Clause: clauseOf(c)}
	ip.Raw = c.GetText()
	if id := c.IDENTIFIER(); id != nil {
		ip.Alias = id.GetText()
	}
	if pp := c.PathPredicate(); pp != nil {
		ip.Predicate = trimBrackets(pp.GetText())
	}
	if op := c.ObjectPath(); op != nil {
		for _, part := range op.AllPathPart() {
			seg := PathSegment{}
			if id := part.IDENTIFIER(); id != nil {
				seg.Name = id.GetText()
			}
			if pp := part.PathPredicate(); pp != nil {
				seg.Predicate = trimBrackets(pp.GetText())
			}
			ip.Segments = append(ip.Segments, seg)
		}
	}
	e.paths = append(e.paths, ip)
}

// VisitTerminal collects $parameter references from anywhere in the tree,
// de-duplicated and in first-seen (document) order, with the leading `$`
// stripped to match [aql.Query.Parameters] keys.
func (e *extractor) VisitTerminal(node antlr.TerminalNode) {
	if node.GetSymbol().GetTokenType() != gen.AqlLexerPARAMETER {
		return
	}
	name := strings.TrimPrefix(node.GetText(), "$")
	if e.seenParam[name] {
		return
	}
	e.seenParam[name] = true
	e.params = append(e.params, name)
}

// clauseOf walks up the parse tree from a node to its enclosing top-level
// clause. Identified paths only ever appear under SELECT, WHERE, or ORDER BY
// (predicates carry relative objectPaths, not identifiedPaths).
func clauseOf(t antlr.Tree) Clause {
	for p := t.GetParent(); p != nil; p = p.GetParent() {
		switch p.(type) {
		case *gen.SelectClauseContext:
			return ClauseSelect
		case *gen.WhereClauseContext:
			return ClauseWhere
		case *gen.OrderByClauseContext:
			return ClauseOrderBy
		}
	}
	return ClauseUnknown
}

func posOf(tok antlr.Token) Position {
	if tok == nil {
		return Position{}
	}
	// ANTLR columns are 0-based; expose 1-based to match SyntaxError.
	return Position{Line: tok.GetLine(), Col: tok.GetColumn() + 1}
}

func trimBrackets(s string) string {
	return strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
}

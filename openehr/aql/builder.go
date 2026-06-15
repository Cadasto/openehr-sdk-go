package aql

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Builder composes an [Query] from typed clauses (the struct-builder style of
// REQ-055). Methods are chainable and mutate the receiver; call [Builder.Build]
// to emit the canonical query. The verb-functions style (Select, From, …)
// shares the same internal emitter, so both produce byte-identical AQL for the
// same logical query (PROBE-020).
//
// Injection: caller-supplied data MUST flow through [Param] (or a literal
// constructor), which the emitter binds or escapes. Path, alias, and archetype
// arguments (to [Col], [Eq], [Archetype], [Builder.OrderBy], …) are openEHR
// identifiers emitted verbatim — author them as constants, never from
// untrusted input.
type Builder struct {
	ast ast
}

// NewBuilder returns an empty struct-builder.
func NewBuilder() *Builder { return &Builder{} }

// Select sets the projection list (SELECT). Later calls replace earlier ones.
func (b *Builder) Select(cols ...SelectField) *Builder {
	b.ast.sel = slices.Clone(cols)
	return b
}

// From sets the FROM clause to an arbitrary RM class with an alias, e.g.
// From("COMPOSITION", "c"). Use [Builder.FromEHR] for the common ehr_id-scoped
// case.
func (b *Builder) From(rmType, alias string) *Builder {
	b.ast.from = &fromClause{rmType: rmType, alias: alias}
	b.ast.ehrFilter = nil // re-scoping the source drops any prior FromEHR filter
	return b
}

// FromEHR sets the FROM clause to an EHR and, when id is non-nil, scopes the
// query to that EHR with a `WHERE <alias>/ehr_id/value = <id>` condition
// (AND-combined with any [Builder.Where] predicate). A nil id emits a bare
// `FROM EHR <alias>`.
//
// The standing-predicate form `EHR <alias>[ehr_id/value=<id>]` is equally valid
// AQL; this builder emits the WHERE form so the EHR scope composes uniformly
// with other conditions in one clause.
func (b *Builder) FromEHR(alias string, id Value) *Builder {
	b.ast.from = &fromClause{rmType: "EHR", alias: alias}
	b.ast.ehrFilter = nil // reset first so FromEHR(alias, nil) clears a prior filter
	if id != nil {
		b.ast.ehrFilter = Eq(alias+"/ehr_id/value", id)
	}
	return b
}

// Contains appends a CONTAINS containment to the FROM clause.
func (b *Builder) Contains(c Containment) *Builder {
	b.ast.contains = append(b.ast.contains, c)
	return b
}

// Where sets the WHERE predicate. Later calls replace earlier ones; combine
// with [And] / [Or].
func (b *Builder) Where(e WhereExpr) *Builder {
	b.ast.where = e
	return b
}

// OrderBy appends an ORDER BY term.
func (b *Builder) OrderBy(path string, dir Direction) *Builder {
	b.ast.orderBy = append(b.ast.orderBy, orderTerm{path: path, dir: dir})
	return b
}

// Offset sets the row offset. It populates [Query.Offset] (the request
// envelope), not the AQL string — paging is one channel, the envelope.
func (b *Builder) Offset(n int) *Builder {
	b.ast.offset = n
	return b
}

// Limit sets the maximum row count. It populates [Query.Fetch] (the request
// envelope), not the AQL string.
func (b *Builder) Limit(n int) *Builder {
	b.ast.limit = n
	return b
}

// Bind supplies a value for a named placeholder introduced via [Param]; it
// populates [Query.Parameters] on the built query. Binding is optional — the
// emitted string carries `$name` regardless.
func (b *Builder) Bind(name string, value any) *Builder {
	if b.ast.params == nil {
		b.ast.params = map[string]any{}
	}
	b.ast.params[name] = value
	return b
}

// Build emits the canonical [Query]. It returns an error wrapping
// [ErrInvalidQuery] if the query has no projection or no source.
func (b *Builder) Build() (Query, error) { return b.ast.build() }

// SelectField is one entry in the SELECT projection list. Construct with [Col].
type SelectField struct{ path string }

// Col is a projected path or alias, e.g. Col("o") or Col("o/data[at0001]").
func Col(path string) SelectField { return SelectField{path: strings.TrimSpace(path)} }

// Containment is a CONTAINS term in the FROM clause. Construct with [Archetype].
type Containment struct {
	rmType      string
	alias       string
	archetypeID string
}

// Archetype is a containment constraint: `<rmType> <alias>[<archetypeID>]`. An
// empty archetypeID emits `<rmType> <alias>` with no predicate.
func Archetype(rmType, alias, archetypeID string) Containment {
	return Containment{rmType: rmType, alias: alias, archetypeID: archetypeID}
}

// Direction is an ORDER BY sort direction.
type Direction int

const (
	// Ascending emits ASC.
	Ascending Direction = iota
	// Descending emits DESC.
	Descending
)

func (d Direction) keyword() string {
	if d == Descending {
		return "DESC"
	}
	return "ASC"
}

type orderTerm struct {
	path string
	dir  Direction
}

type fromClause struct {
	rmType string
	alias  string
}

// ast is the shared, unexported query tree emitted by both builder styles. It
// is the single canonicalisation point (REQ-055).
type ast struct {
	sel       []SelectField
	from      *fromClause
	contains  []Containment
	where     WhereExpr
	ehrFilter WhereExpr // implicit ehr_id condition from FromEHR; AND-ed with where
	orderBy   []orderTerm
	offset    int
	limit     int
	params    map[string]any
}

func (a *ast) build() (Query, error) {
	if len(a.sel) == 0 {
		return Query{}, fmt.Errorf("%w: no SELECT fields", ErrInvalidQuery)
	}
	for _, c := range a.sel {
		if c.path == "" {
			return Query{}, fmt.Errorf("%w: empty SELECT field", ErrInvalidQuery)
		}
	}
	if a.from == nil {
		return Query{}, fmt.Errorf("%w: no FROM source", ErrInvalidQuery)
	}
	if a.from.rmType == "" || a.from.alias == "" {
		return Query{}, fmt.Errorf("%w: FROM requires an RM type and alias", ErrInvalidQuery)
	}
	for _, c := range a.contains {
		if c.rmType == "" || c.alias == "" {
			return Query{}, fmt.Errorf("%w: CONTAINS requires an RM type and alias", ErrInvalidQuery)
		}
	}

	var sb strings.Builder
	sb.WriteString("SELECT ")
	for i, c := range a.sel {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(c.path)
	}

	sb.WriteString(" FROM ")
	sb.WriteString(a.from.rmType)
	sb.WriteByte(' ')
	sb.WriteString(a.from.alias)

	for _, c := range a.contains {
		sb.WriteString(" CONTAINS ")
		sb.WriteString(c.rmType)
		sb.WriteByte(' ')
		sb.WriteString(c.alias)
		if c.archetypeID != "" {
			sb.WriteByte('[')
			sb.WriteString(c.archetypeID)
			sb.WriteByte(']')
		}
	}

	// The implicit ehr_id filter from FromEHR AND-combines with any explicit
	// WHERE predicate so a single canonical WHERE clause results.
	where := a.effectiveWhere()
	if where != nil {
		// Reject malformed predicates (nil values, empty paths) before emitting
		// so the typed builders can never produce invalid AQL (PROBE-021).
		if err := where.validate(); err != nil {
			return Query{}, err
		}
		// A non-nil predicate that emits nothing (e.g. And() with no terms)
		// would yield a trailing, syntactically invalid WHERE.
		pred := where.expr()
		if strings.TrimSpace(pred) == "" {
			return Query{}, fmt.Errorf("%w: empty WHERE predicate", ErrInvalidQuery)
		}
		sb.WriteString(" WHERE ")
		sb.WriteString(pred)
	}

	if len(a.orderBy) > 0 {
		sb.WriteString(" ORDER BY ")
		for i, t := range a.orderBy {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(t.path)
			sb.WriteByte(' ')
			sb.WriteString(t.dir.keyword())
		}
	}

	// OFFSET / LIMIT are carried in the request envelope (Query.Offset /
	// Query.Fetch), not the AQL string — a single paging channel the executor
	// already maps.
	// Clone so the built query does not alias the builder's internal map.
	return Query{Q: sb.String(), Offset: a.offset, Fetch: a.limit, Parameters: maps.Clone(a.params)}, nil
}

// effectiveWhere combines the implicit ehr_id filter (from FromEHR) with any
// explicit WHERE predicate. The ehr_id condition leads so the canonical clause
// reads `WHERE e/ehr_id/value = $x AND <rest>`.
func (a *ast) effectiveWhere() WhereExpr {
	switch {
	case a.ehrFilter != nil && a.where != nil:
		return And(a.ehrFilter, a.where)
	case a.ehrFilter != nil:
		return a.ehrFilter
	default:
		return a.where
	}
}

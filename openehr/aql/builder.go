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
	// Trimmed like [Col]: a blank part must collapse to the empty string so
	// build's presence check refuses it — written verbatim, `From("   ", "c")`
	// emitted `FROM     c`, which RE-PARSES as an alias-less class named `c`:
	// the alias silently became the RM type (REQ-119's substitution class).
	b.ast.from = &fromClause{rmType: strings.TrimSpace(rmType), alias: strings.TrimSpace(alias)}
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
	alias = strings.TrimSpace(alias) // as in From; also keeps the ehr_id path below clean
	b.ast.from = &fromClause{rmType: "EHR", alias: alias}
	b.ast.ehrFilter = nil // reset first so FromEHR(alias, nil) clears a prior filter
	if id != nil {
		b.ast.ehrFilter = Eq(alias+"/ehr_id/value", id)
	}
	return b
}

// Contains appends a CONTAINS containment to the FROM clause. Repeated calls
// chain (`… CONTAINS A CONTAINS B`), matching the grammar's right-greedy
// `CONTAINS containsExpr`.
//
// Since REQ-117 the argument may be a whole containment expression — a nested
// chain ([Containment.Contains] / [Containment.NotContains]) or a sibling
// junction ([ContainsAnd] / [ContainsOr]) — not only a single class. A single
// class emits exactly as it did before.
func (b *Builder) Contains(c Containment) *Builder {
	c.negated = false
	b.ast.contains = append(b.ast.contains, c)
	return b
}

// NotContains appends a containment connected by NOT CONTAINS — the grammar's
// `classExprOperand NOT CONTAINS containsExpr` (REQ-117), i.e. the absence of
// c below the preceding term. Otherwise identical to [Builder.Contains].
func (b *Builder) NotContains(c Containment) *Builder {
	c.negated = true
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
	// Trimmed like [Col] so a blank path collapses to "" and build refuses it
	// — written verbatim it emitted `ORDER BY  ASC`, a syntax error the
	// grammar's `orderByExpr : identifiedPath …` has no reading for.
	b.ast.orderBy = append(b.ast.orderBy, orderTerm{path: strings.TrimSpace(path), dir: dir})
	return b
}

// Offset sets the row offset. It populates [Query.Offset] (the request
// envelope), not the AQL string — the envelope is the default paging channel.
// Use [Builder.OffsetInline] for the opt-in in-text form.
func (b *Builder) Offset(n int) *Builder {
	b.ast.offset = n
	return b
}

// Limit sets the maximum row count. It populates [Query.Fetch] (the request
// envelope), not the AQL string. Use [Builder.LimitInline] for the opt-in
// in-text form.
func (b *Builder) Limit(n int) *Builder {
	b.ast.limit = n
	return b
}

// LimitInline sets an IN-TEXT `LIMIT n`, emitted into the AQL string after
// ORDER BY instead of travelling in the request envelope (REQ-117). Reach for
// it when the bound must survive stored-query registration — the string is
// what the server stores — or when the query text is handed to another
// engine; the envelope channel ([Builder.Limit] / [Builder.Offset]) remains
// the default.
//
// The two channels are mutually exclusive: setting both makes [Builder.Build]
// return an error wrapping [ErrInvalidQuery] rather than silently combining
// them. A negative n is likewise refused (the grammar's `limitValue` is a
// non-negative INTEGER). Later calls replace earlier ones.
func (b *Builder) LimitInline(n int) *Builder {
	b.ast.limitInline = Int(int64(n))
	return b
}

// LimitInlineParam sets an in-text `LIMIT $name` — the grammar's
// parameter-valued limit, bound by the server at execution time (REQ-117).
// A leading `$` in name is stripped, as in [Param]. Same channel-exclusivity
// rule as [Builder.LimitInline].
func (b *Builder) LimitInlineParam(name string) *Builder {
	b.ast.limitInline = Param(name)
	return b
}

// OffsetInline sets an in-text `OFFSET n`, emitted after the in-text LIMIT
// (REQ-117). The grammar admits OFFSET only after LIMIT
// (`LIMIT limitValue (OFFSET limitValue)?`), so an in-text OFFSET without an
// in-text LIMIT is refused by [Builder.Build] rather than emitted as text the
// parser would reject.
func (b *Builder) OffsetInline(n int) *Builder {
	b.ast.offsetInline = Int(int64(n))
	return b
}

// OffsetInlineParam sets an in-text `OFFSET $name` (REQ-117). Same rules as
// [Builder.OffsetInline] and [Builder.LimitInlineParam].
func (b *Builder) OffsetInlineParam(name string) *Builder {
	b.ast.offsetInline = Param(name)
	return b
}

// Top sets the DEPRECATED in-text `SELECT TOP n` row limit (REQ-118),
// emitted between `SELECT`/`DISTINCT` and the projection list.
//
// Prefer [Builder.LimitInline] or the envelope [Builder.Limit]: openEHR QUERY
// Release-1.1.0 § 4.4.3 deprecates `TOP` in favour of `LIMIT` with `ORDER BY`
// and announces its removal in a future major release. It is offered so the
// SDK can author the deprecated shape a client, a stored query, or a
// conformance corpus may still legitimately carry.
//
// § 4.4.3 also forbids `TOP` and `LIMIT` in one query, and a `TOP` is itself
// an in-text row bound — so setting it alongside EITHER row-limit channel
// (in-text or envelope) makes [Builder.Build] return an error wrapping
// [ErrInvalidQuery], never a silently combined emission. A negative n is
// likewise refused (the grammar's `top` production admits no sign). Later
// calls replace earlier ones.
func (b *Builder) Top(n int) *Builder {
	b.ast.top = &TopClause{N: n}
	return b
}

// TopDirected sets the deprecated `SELECT TOP n FORWARD|BACKWARD` row limit
// (REQ-118). [TopDirUnspecified] emits no direction keyword, making it
// equivalent to [Builder.Top]; a direction outside the vocabulary is refused
// at [Builder.Build] rather than emitted as text the parser would reject. Same
// deprecation notice and channel-exclusivity rule as [Builder.Top].
func (b *Builder) TopDirected(n int, dir TopDir) *Builder {
	b.ast.top = &TopClause{N: n, Dir: dir}
	return b
}

// Bind supplies a value for a named placeholder introduced via [Param]; it
// populates [Query.Parameters] on the built query. A leading `$` in name is
// stripped, as in [Param], so `Bind("$id", …)` and `Bind("id", …)` address the
// SAME key — a later call with the same effective name replaces the earlier
// value. Binding is optional — the emitted string carries `$name` regardless —
// but a bind whose stripped name is empty is refused at [Builder.Build]: no
// placeholder can ever match it, so the value could only be shipped dead.
func (b *Builder) Bind(name string, value any) *Builder {
	name = strings.TrimPrefix(name, "$")
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

// known mirrors [BoolOp.known] and the TopDir vocabulary check: keyword()
// spells any other value as ASC, so an out-of-vocabulary Direction must be
// refused before it is silently re-directed (REQ-119's substitution class).
func (d Direction) known() bool { return d == Ascending || d == Descending }

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
	// limitInline / offsetInline are the opt-in IN-TEXT paging operands
	// (REQ-117), emitted after ORDER BY. Nil means the clause is absent —
	// distinct from the envelope's zero, so `LIMIT 0` stays expressible.
	// Concrete shapes are [IntValue] and [ParamValue], the grammar's
	// `limitValue : INTEGER | PARAMETER`.
	limitInline  Value
	offsetInline Value
	// top is the opt-in DEPRECATED `SELECT TOP n` row limit (REQ-118). Nil
	// means the clause is absent — distinct from `TOP 0`, which is a real
	// bound. It joins the in-text row-limit channel for the exclusivity
	// rule in validatePaging.
	top    *TopClause
	params map[string]any
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
	// Both are IDENTIFIER positions emitted verbatim, so they get the same
	// refusal (*parse.Query).Emit applies — Build/Emit parity from day one
	// rather than one write path hardened and the other left open (issue #96).
	// The RM type goes through [validateRMTypeToken] rather than the bare
	// guard: `VERSION` is classExprOperand's other ALTERNATIVE, and this
	// carrier has no flag to hold it, so the spelling itself is the carrier.
	if err := validateRMTypeToken(a.from.rmType); err != nil {
		return Query{}, fmt.Errorf("FROM RM type: %w", err)
	}
	if err := ValidateIdentifier(a.from.alias); err != nil {
		return Query{}, fmt.Errorf("FROM alias: %w", err)
	}
	// ORDER BY was the one clause whose operands build wrote unchecked while
	// parse.Query.Emit refused them — the Build/Emit write-path fork REQ-119
	// closed for WHERE (REQ-055's builder guarantee, PROBE-021).
	for _, t := range a.orderBy {
		if t.path == "" {
			return Query{}, fmt.Errorf("%w: empty ORDER BY path", ErrInvalidQuery)
		}
		if !t.dir.known() {
			return Query{}, fmt.Errorf("%w: ORDER BY direction %d is outside the ASC/DESC vocabulary; "+
				"emitting would silently re-direct it to ASC", ErrInvalidQuery, t.dir)
		}
	}
	// REQ-117: a containment term is a whole expression (chain, negation,
	// junction), so alias uniqueness and the class-completeness rule are
	// checked over the entire tree. Repeated Contains / NotContains calls emit
	// as ONE chain, so the junction-placement rule spans them as well.
	if err := validateContainsChain(a.contains); err != nil {
		return Query{}, err
	}
	seen := map[string]bool{a.from.alias: true}
	for _, c := range a.contains {
		if err := c.validateTree(seen); err != nil {
			return Query{}, err
		}
	}
	if err := a.validatePaging(); err != nil {
		return Query{}, err
	}

	var sb strings.Builder
	sb.WriteString("SELECT ")
	// REQ-118: the deprecated TOP row limit precedes the projection list
	// (grammar: `SELECT DISTINCT? top? selectExpr …`). validatePaging has
	// already refused a negative count, an unknown direction, and any
	// combination with the other row-limit channels.
	if a.top != nil {
		sb.WriteString(FormatTop(a.top))
		sb.WriteByte(' ')
	}
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
		// REQ-117: the connector carries the term's negation, and emit
		// renders the whole containment expression (chain / junction) —
		// a single class emits the pre-REQ-117 bytes unchanged.
		sb.WriteString(c.connector())
		sb.WriteString(c.emit())
	}

	// The implicit ehr_id filter from FromEHR AND-combines with any explicit
	// WHERE predicate so a single canonical WHERE clause results.
	//
	// Rendered through [FormatWhere] — the SAME validate-then-emit sequence
	// parse.Query.Emit uses — so the subsystem has exactly one WHERE emission
	// path. Build and Emit previously each ran their own validate()+expr()
	// pair; they agreed, but a two-copy sequence is how the typed-nil and
	// parenthesisation defects kept splitting between the write paths
	// (REQ-119). effectiveWhere still owns the FromEHR combining and the
	// collapse of a predicate denoting nothing.
	where := a.effectiveWhere()
	pred, err := FormatWhere(where)
	if err != nil {
		return Query{}, err
	}
	switch {
	case strings.TrimSpace(pred) != "":
		sb.WriteString(" WHERE ")
		sb.WriteString(pred)
	case where != nil:
		// A present predicate that renders nothing would yield a trailing,
		// syntactically invalid WHERE. Junction.validate refuses the term-less
		// junction that used to reach this, so it is a backstop.
		return Query{}, fmt.Errorf("%w: empty WHERE predicate", ErrInvalidQuery)
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

	// Paging: by default OFFSET / LIMIT are carried in the request envelope
	// (Query.Offset / Query.Fetch), not the AQL string. REQ-117 adds the
	// opt-in in-text channel, emitted here after ORDER BY in clause order
	// (grammar: `LIMIT limitValue (OFFSET limitValue)?`); validatePaging has
	// already refused a query that sets both channels.
	if a.limitInline != nil {
		sb.WriteString(" LIMIT ")
		sb.WriteString(a.limitInline.token())
	}
	if a.offsetInline != nil {
		sb.WriteString(" OFFSET ")
		sb.WriteString(a.offsetInline.token())
	}

	// A bind whose stripped name is empty matches no PARAMETER (the grammar
	// requires a leading letter), so the value would ride to the CDR keyed by
	// "" — dead weight at best, surfaced only if the caller happens to lint.
	// `Bind("$")` and `Bind("")` are always defects; refuse loudly.
	if _, ok := a.params[""]; ok {
		return Query{}, fmt.Errorf("%w: Bind with an empty parameter name (after stripping a "+
			"leading '$'); no placeholder can match it", ErrInvalidQuery)
	}
	// Clone so the built query does not alias the builder's internal map.
	return Query{Q: sb.String(), Offset: a.offset, Fetch: a.limit, Parameters: maps.Clone(a.params)}, nil
}

// validatePaging enforces the REQ-117 paging rules: the in-text channel and
// the request envelope are never silently combined, the grammar admits OFFSET
// only after LIMIT, and an in-text operand must be a non-negative integer or
// a named parameter (`limitValue : INTEGER | PARAMETER`).
func (a *ast) validatePaging() error {
	inline := a.limitInline != nil || a.offsetInline != nil
	if err := a.validateTop(inline); err != nil {
		return err
	}
	if inline && (a.limit != 0 || a.offset != 0) {
		return fmt.Errorf("%w: paging set on both channels — in-text LIMIT/OFFSET and the request envelope "+
			"(Limit/Offset); pick one", ErrInvalidQuery)
	}
	if a.offsetInline != nil && a.limitInline == nil {
		return fmt.Errorf("%w: in-text OFFSET without LIMIT", ErrInvalidQuery)
	}
	if err := validateLimitValue("LIMIT", a.limitInline); err != nil {
		return err
	}
	return validateLimitValue("OFFSET", a.offsetInline)
}

// validateTop enforces the REQ-118 rules for the deprecated `SELECT TOP`
// clause: openEHR QUERY Release-1.1.0 § 4.4.3 forbids `TOP` together with
// `LIMIT`, and a TOP is itself an in-text row bound, so it is exclusive with
// BOTH row-limit channels. The count must be representable in the grammar's
// unsigned `top : TOP INTEGER …`, and a direction outside the vocabulary
// would render as nothing at all — a silently undirected bound.
func (a *ast) validateTop(inline bool) error {
	if a.top == nil {
		return nil
	}
	if inline {
		return fmt.Errorf("%w: SELECT TOP set together with an in-text LIMIT/OFFSET — openEHR QUERY "+
			"Release-1.1.0 §4.4.3 forbids TOP with LIMIT; pick one", ErrInvalidQuery)
	}
	if a.limit != 0 || a.offset != 0 {
		return fmt.Errorf("%w: SELECT TOP set together with the request envelope's row limit "+
			"(Limit/Offset) — two row bounds on one query; pick one", ErrInvalidQuery)
	}
	if a.top.N < 0 {
		return fmt.Errorf("%w: negative SELECT TOP count %d", ErrInvalidQuery, a.top.N)
	}
	switch a.top.Dir {
	case TopDirUnspecified, TopForward, TopBackward:
	default:
		return fmt.Errorf("%w: unknown SELECT TOP direction %d", ErrInvalidQuery, int(a.top.Dir))
	}
	return nil
}

// validateLimitValue rejects an in-text paging operand the grammar's
// `limitValue : INTEGER | PARAMETER` cannot carry. A nil operand means the
// clause is absent.
//
// The default arm is fail-closed: the [Builder.LimitInline] /
// [Builder.LimitInlineParam] / [Builder.OffsetInline] /
// [Builder.OffsetInlineParam] setters only ever store an [IntValue] or a
// [ParamValue], so no public route reaches it today — it is here so a widened
// setter (or a new [Value] shape) refuses loudly instead of emitting AQL the
// grammar rejects.
func validateLimitValue(keyword string, v Value) error {
	if v == nil {
		return nil // clause absent
	}
	// Normalised like every other dispatch site (REQ-119): the setters only
	// store value shapes today, but this switch must not become the one place
	// a pointer twin is refused as "not an integer or a $parameter".
	inner, ok := derefValue(v)
	if !ok {
		return fmt.Errorf("%w: in-text %s carries a nil %T", ErrInvalidQuery, keyword, v)
	}
	switch t := inner.(type) {
	case IntValue:
		if t.N < 0 {
			return fmt.Errorf("%w: negative in-text %s (%d)", ErrInvalidQuery, keyword, t.N)
		}
	case ParamValue:
		// The same PARAMETER token as any other placeholder position, so the
		// same guard: `LIMIT $a b` is two tokens and does not re-parse.
		if err := validateParamName(t.Name); err != nil {
			return fmt.Errorf("in-text %s: %w", keyword, err)
		}
	default:
		return fmt.Errorf("%w: in-text %s must be an integer or a $parameter, got %T",
			ErrInvalidQuery, keyword, v)
	}
	return nil
}

// effectiveWhere combines the implicit ehr_id filter (from FromEHR) with any
// explicit WHERE predicate. The ehr_id condition leads so the canonical clause
// reads `WHERE e/ehr_id/value = $x AND <rest>`.
func (a *ast) effectiveWhere() WhereExpr {
	// A predicate denoting nothing is collapsed to a genuine nil BEFORE the
	// combination, because absence is positional (REQ-119): at the top level it
	// is simply no clause. Left un-normalised, a typed nil took a third path
	// again — [And] keeps it as a junction TERM, where absence is correctly
	// refused — so one input produced three behaviours depending on whether
	// FromEHR was used.
	where := a.where
	if _, ok := derefWhere(where); !ok {
		where = nil
	}
	switch {
	case a.ehrFilter != nil && where != nil:
		return And(a.ehrFilter, where)
	case a.ehrFilter != nil:
		return a.ehrFilter
	default:
		return where
	}
}

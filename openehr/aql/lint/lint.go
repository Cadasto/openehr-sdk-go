package lint

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// Severity classifies a lint [Issue]. Error means the query is (statically)
// wrong; Warning is advisory — the query may still execute, and the SDK
// grammar profile / CDR may admit it.
type Severity int

const (
	// Error means a static defect a strict consumer SHOULD reject.
	Error Severity = iota
	// Warning is advisory; it does not make a [Result] not-OK.
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
	return fmt.Sprintf("severity(%d)", int(s))
}

// Span is where in the query an [Issue] applies — the SAME type the parser
// records a dropped construct with ([parse.Span]), re-exported rather than
// redefined, so a consumer correlating a lint issue with a
// [parse.DroppedConstruct] compares spans instead of converting between two
// structurally-identical types (REQ-109 § Value-free lint diagnostics).
type Span = parse.Span

// spanOfText is the span a construct of the given text occupies starting at
// start. Text carrying a newline advances the line and restarts the column, so
// a multi-line path does not report a column past the end of its first line.
// A zero start yields the zero Span — an unattributable issue reports that
// rather than an invented position.
func spanOfText(start parse.Position, text string) Span {
	if start == (parse.Position{}) {
		return Span{}
	}
	return Span{Start: start, End: advance(start, text)}
}

// advance is the position reached by reading text from p. Columns count RUNES,
// matching how the parser reports a position, so a multi-byte character does
// not shift every column after it.
//
// It is shared with [segmentSpan], which starts a span PART-WAY along a
// construct rather than at its beginning ([parse.PathSegment] carries no
// position of its own).
func advance(p parse.Position, text string) parse.Position {
	if n := strings.Count(text, "\n"); n > 0 {
		p.Line += n
		p.Col = 1 + len([]rune(text[strings.LastIndex(text, "\n")+1:]))
	} else {
		p.Col += len([]rune(text))
	}
	return p
}

// Issue is one finding from a lint pass. Lint is collect-all (every issue,
// not fail-fast). The zero value is not meaningful.
type Issue struct {
	// Code is a stable programmatic identifier (e.g. "aql_syntax",
	// "aql_archetype_not_in_template"). Consumers SHOULD dispatch on Code.
	//
	// VALUE-FREE: never carries source text.
	Code string
	// Path is the AQL path or class the issue concerns; "" when not
	// localised.
	//
	// VALUE-BEARING: a path spelling carries its own predicates, so
	// `o/data[at0001, 'Systolic']` is a path AND a value. A disclosure
	// boundary MUST treat this as query text despite its looking structural.
	Path string
	// Detail is a human-readable message (carries ANTLR line/col for
	// syntax errors).
	//
	// VALUE-BEARING: may quote any part of the query.
	Detail string
	// Severity classifies the issue.
	//
	// VALUE-FREE: never carries source text.
	Severity Severity
	// Span locates the issue in the query text handed to [LintString] (or to
	// [parse.Parse] for [Lint]). The zero Span means the issue is not
	// attributable to a position; it never falls back to embedding source
	// text (REQ-109 § Value-free lint diagnostics).
	//
	// VALUE-FREE: line and column numbers only.
	Span Span
}

// Result aggregates every [Issue] from one [Lint] / [LintString] call.
type Result struct {
	// Issues is the full list of findings in a stable, deterministic
	// order: by layer, then document order within a layer (aql_unused_param
	// is sorted by parameter key, since unreferenced params have no
	// document position; and Layer 2 orders by CHECK GROUP first — shape,
	// then the REQ-161 semantic group in its own fixed sequence, then the
	// REQ-164 path-shape group — so a later-in-the-query semantic finding
	// can precede an earlier shape one).
	// Never nil after a lint call (zero-length when clean).
	Issues []Issue
}

// OK reports whether the result carries no Error-severity issue. Warnings do
// not make a result not-OK.
func (r Result) OK() bool {
	return !slices.ContainsFunc(r.Issues, func(i Issue) bool { return i.Severity == Error })
}

// Options tunes a lint pass. The zero value (or nil) runs the AST-shape
// checks AND the whole REQ-161 Layer-2 semantic group (which is ungated and
// can raise Error-severity issues that flip [Result.OK] — see Relation);
// Layer 3 needs Compiled, and the Layer-2 checks that judge the query against
// its request envelope need Query.
type Options struct {
	// Compiled, when non-nil, enables Layer 3 (archetype / path checks
	// against a compiled OPT).
	Compiled *templatecompile.Compiled
	// Query, when non-nil, is the request envelope the AQL will execute
	// under. It enables the parameter-binding checks (aql_unbound_param /
	// aql_unused_param) against its Parameters map, and lets the TOP group
	// see the envelope's row limit (aql_top_with_fetch, REQ-118).
	Query *aql.Query
	// Relation is the REQ-160 containment relation the Layer-2 semantic
	// checks judge FROM/CONTAINS shapes against (REQ-161 § Relation supply).
	//
	// Unlike Compiled and Query it does not GATE its checks: nil means the
	// REQ-160 default relation ([contain.Default]), so the semantic group
	// always runs. Supply a relation extended with dialect overlay edges
	// ([contain.TypeRelation.WithOverlay]) to lint a deployment whose containment
	// facts go beyond the pinned RM without drawing false findings.
	//
	// It governs the five containment-pair codes only. The three portability
	// codes — aql_version_no_predicate, aql_versioned_object_unreferenced and
	// aql_fanout_row_grain — ignore it: the first two put a CLASS question to
	// the pinned RM (rminfo.Default) rather than a containment one, and the
	// third reads the query's own SELECT / CONTAINS shape and consults no RM
	// facts at all. An overlay therefore cannot retire those three.
	//
	// Of the five REQ-164 path-shape codes it governs exactly ONE:
	// aql_contains_redundant_step, whose whole question is whether a
	// containment ROUTE goes round the step — precisely the kind of fact an
	// overlay edge states, so the relation in use must answer it or a dialect
	// deployment draws a false finding on a step its own edges make
	// load-bearing (REQ-160 § Extensibility). The other four ignore it, in two
	// pairs: aql_path_repeating_unpredicated and aql_fanout_path_grain — the
	// two codes the segment walk feeds — ask CLASS questions, attribute typing
	// and multiplicity, which no caller-supplied containment relation may
	// answer differently (REQ-164 § The conservative segment walk); while
	// aql_paging_no_order_by and aql_select_no_alias are parse-only and consult
	// no RM fact at all. No REQ-164 code is GATED by this field either way: nil
	// selects the default here as it does everywhere.
	Relation *contain.TypeRelation
}

// LintString parses q against the SDK grammar profile and lints the result.
// It is the entry point for raw AQL (the [validation.ValidateAQL] bridge uses
// it). An empty/whitespace-only query yields aql_empty; a parse failure
// yields aql_syntax; otherwise it runs [Lint].
func LintString(q string, opts *Options) Result {
	if strings.TrimSpace(q) == "" {
		return Result{Issues: []Issue{{
			Code:     "aql_empty",
			Detail:   "query is empty",
			Severity: Error,
		}}}
	}
	doc, err := parse.Parse(q)
	if err != nil {
		return Result{Issues: []Issue{{
			Code:     "aql_syntax",
			Detail:   syntaxDetail(err),
			Severity: Error,
			Span:     syntaxSpan(err),
		}}}
	}
	return Lint(doc, opts)
}

// syntaxSpan is the zero-width span at a parse failure's position, or the zero
// Span when the error carries none. Zero-width because the parser reports where
// it stopped, not how much text was at fault — an invented end would claim more
// than the diagnostic knows.
func syntaxSpan(err error) Span {
	// A failed errors.AsType can leave ok=true with a typed-nil *SyntaxError —
	// the value a caller's own failed match boxes and passes onward (REQ-025
	// nil-receiver axis) — so ok alone is not proof there is a Pos to read.
	se, ok := errors.AsType[*parse.SyntaxError](err)
	if !ok || se == nil {
		return Span{}
	}
	return Span{Start: se.Pos, End: se.Pos}
}

// syntaxDetail formats a parse failure for lint consumers. REQ-109 requires
// line/column in Detail for aql_syntax; [parse.SyntaxError] carries position.
//
// A zero Pos omits the "L:C:" prefix rather than claiming a fabricated "0:0:".
// That is the position-honesty rule, not the nil-receiver one — the zero
// Position is a NON-nil *parse.SyntaxError's "no position" value, and an
// unattributable diagnostic reports no position rather than an invented one
// (REQ-109 § Value-free lint diagnostics). It mirrors what
// [parse.SyntaxError.Error] and parse's own syntaxErrorPosition do with the
// same value.
func syntaxDetail(err error) string {
	// The se != nil arm is the separate REQ-025 nil-receiver guard: see
	// syntaxSpan — ok alone does not prove se is non-nil.
	if se, ok := errors.AsType[*parse.SyntaxError](err); ok && se != nil {
		if se.Pos == (parse.Position{}) {
			return se.Msg
		}
		return fmt.Sprintf("%d:%d: %s", se.Pos.Line, se.Pos.Col, se.Msg)
	}
	return err.Error()
}

// Lint runs Layers 2–3 on a document obtained from a successful
// [parse.Parse] (Layer 1). opts may be nil. As a guard, a nil or unparsed
// (zero-value) document yields a single aql_syntax issue rather than an
// empty result. Lint is collect-all: it returns every issue across every
// enabled layer.
func Lint(doc *parse.Document, opts *Options) Result {
	if doc == nil || !doc.Parsed() {
		return Result{Issues: []Issue{{
			Code:     "aql_syntax",
			Detail:   "not a parsed document",
			Severity: Error,
		}}}
	}
	if opts == nil {
		opts = &Options{}
	}
	md := Extract(doc)
	issues := []Issue{}

	// opts.Query reaches the shape group too: the TOP check needs the request
	// envelope to see the second row-limit channel (REQ-118). A nil Query
	// leaves that channel invisible, exactly as it gates the parameter checks.
	issues = append(issues, shapeIssues(doc, md, opts.Query)...)
	// The Layer-2 semantic group (REQ-161) is unconditional: unlike Compiled
	// and Query, the relation always has a usable default, so a nil
	// Options.Relation selects the pinned RM rather than switching the group
	// off.
	issues = append(issues, semanticIssues(doc, opts.Relation)...)
	// The Layer-2 path-shape group (REQ-164) is ungated too, and for a
	// stronger reason: it has no input to gate on. Every fact it reads is in
	// the query text, the pinned BMM, or a relation that defaults when absent,
	// and every code it raises is Warning, so it can never turn a passing
	// result into a failing one (REQ-164 § Always on, never gated). opts.Query
	// reaches it for the same reason it reaches the shape group:
	// aql_paging_no_order_by has a second row-bound channel in the request
	// envelope, which a nil Query leaves invisible — that is the ENVELOPE ARM
	// being unable to fire, not the group being gated. opts.Relation reaches it
	// for aql_contains_redundant_step alone, and nil there selects the default
	// relation exactly as it does for the semantic group above.
	issues = append(issues, pathShapeIssues(doc, md, opts.Query, opts.Relation)...)
	if opts.Query != nil {
		issues = append(issues, paramIssues(md, opts.Query)...)
	}
	if opts.Compiled != nil {
		issues = append(issues, templateIssues(md, opts.Compiled)...)
	}
	return Result{Issues: issues}
}

// shapeIssues runs the Layer-2 shape checks: alias binding,
// identifiable scope, the SELECT * relaxation warning, and the TOP-clause
// group. SELECT/FROM presence is guaranteed by a successful parse (the
// grammar requires both), so no aql_select / aql_from issue can arise here.
// q is the request envelope from [Options.Query] (nil when the caller supplied
// none); only the TOP group reads it, and only for its row limit.
func shapeIssues(doc *parse.Document, md Metadata, q *aql.Query) []Issue {
	var issues []Issue

	// aql_unknown_alias — every identified path's root alias MUST bind to a
	// class in FROM / CONTAINS. REQ-117: an ORDER BY key that binds nothing
	// there is resolved against the SELECT `AS` aliases before the issue is
	// raised. FROM/CONTAINS is consulted first, so a SELECT alias reusing a
	// bound alias never shadows the class binding.
	selectAliases := make(map[string]bool, len(md.SelectAliases))
	for _, a := range md.SelectAliases {
		selectAliases[a] = true
	}
	for _, p := range doc.Paths {
		if _, ok := md.Aliases[p.Alias]; ok {
			continue
		}
		if namesSelectAlias(p, selectAliases) {
			continue
		}
		issues = append(issues, Issue{
			Code:     "aql_unknown_alias",
			Path:     displayPath(p),
			Detail:   fmt.Sprintf("path alias %q is not bound in FROM/CONTAINS", p.Alias),
			Severity: Error,
			// The alias is the path's first token, so Pos starts it: the span
			// covers the unbound alias itself, not the whole path.
			Span: spanOfText(p.Pos, p.Alias),
		})
	}

	// aql_from_archetype — the query SHOULD identify what it selects: at
	// least one archetype HRID, a $param archetype predicate, a VERSION
	// operand, or an EHR root. A query with none scans broadly; advisory.
	if !hasIdentifiableScope(doc) {
		issues = append(issues, Issue{
			Code:     "aql_from_archetype",
			Detail:   "FROM/CONTAINS names no archetype, $param, VERSION, or EHR scope",
			Severity: Warning,
		})
	}

	// aql_select_star — bare/mixed SELECT * is the SDK-AQL-002 relaxation;
	// official QUERY 1.1.0 requires explicit columns (COUNT(*) is not this).
	if doc.Star {
		issues = append(issues, Issue{
			Code:     "aql_select_star",
			Detail:   "SELECT * is an SDK grammar-profile relaxation (SDK-AQL-002); official QUERY 1.1.0 requires explicit columns",
			Severity: Warning,
		})
	}

	issues = append(issues, topIssues(doc, q)...)
	return issues
}

// topIssues reports the deprecated `SELECT TOP` modifier and the two row-bound
// pairings the openEHR specifications forbid outright (REQ-118): `TOP` with the
// in-text `LIMIT` clause (QUERY Release-1.1.0 § 4.4.3) and `TOP` with the
// request envelope's row limit (the Query API common parameters).
//
// The SDK parses and emits `TOP` faithfully — a query it did not author may
// legitimately carry one until the announced removal — so this is where the
// spec's judgement on the construct is reported, rather than in the parser or
// the emitter. The builder refuses to CONSTRUCT any of these shapes.
//
// q is the request envelope from [Options.Query]. It is read for its row limit
// only: nil (no envelope supplied) leaves that channel invisible to the pass,
// and the in-text pairing is still judged.
func topIssues(doc *parse.Document, q *aql.Query) []Issue {
	// Keyed on PRESENCE, not on the decoded bound: an out-of-range count
	// leaves [parse.Document.Top] nil (nothing is truncated into a bound), and
	// keying on that would silence both findings for exactly the query that
	// carries a deprecated clause AND an unusable count.
	if !doc.HasTop {
		return nil
	}
	// Name the clause when it decoded; an unrepresentable count has no
	// canonical rendering, so the code carries the construct alone.
	clause := "TOP"
	if doc.Top != nil {
		clause = aql.FormatTop(doc.Top)
	}
	issues := []Issue{{
		Code:     "aql_deprecated_top",
		Detail:   fmt.Sprintf("SELECT %s is deprecated from openEHR QUERY Release-1.1.0 and slated for removal; use LIMIT with ORDER BY", clause),
		Severity: Warning,
	}}
	if doc.HasLimit {
		issues = append(issues, Issue{
			Code:     "aql_top_with_limit",
			Detail:   "SELECT TOP is used together with a LIMIT clause, which openEHR QUERY Release-1.1.0 §4.4.3 does not allow",
			Severity: Error,
		})
	}
	// The envelope's row limit is the same bound arriving by the other
	// channel. Keyed on Fetch alone: the Query API common parameters exclude
	// `fetch` from combining with AQL-top and say nothing of `offset`, so an
	// offset-only envelope does not fire this. The parity with the write side
	// is therefore partial by design: Build() ALSO refuses an envelope Offset
	// beside a TOP, because it will not AUTHOR two row bounds — whereas this
	// check reads a query the SDK did not author, so it reports only what the
	// Query API actually forbids, the `fetch` arm.
	if q != nil && q.Fetch > 0 {
		issues = append(issues, Issue{
			Code: "aql_top_with_fetch",
			Detail: "SELECT TOP is used together with the request envelope's row limit, which the openEHR Query API " +
				"common parameters do not allow: `fetch` cannot be combined with AQL-top",
			Severity: Error,
		})
	}
	return issues
}

// namesSelectAlias reports whether p is an ORDER BY key naming one of the
// SELECT `AS` aliases (REQ-117). Only a BARE identifier qualifies: an AS alias
// labels a projected column, not a path root, so `ORDER BY score/magnitude`
// (or a predicated key) stays aql_unknown_alias. The fallback is scoped to
// ORDER BY — a SELECT alias is not a SELECT or WHERE operand root.
func namesSelectAlias(p parse.IdentifiedPath, selectAliases map[string]bool) bool {
	if p.Clause != parse.ClauseOrderBy || len(p.Segments) > 0 || p.Predicate != "" {
		return false
	}
	return selectAliases[p.Alias]
}

func hasIdentifiableScope(doc *parse.Document) bool {
	return slices.ContainsFunc(doc.Classes, func(ce parse.ClassExpr) bool {
		return ce.Archetype != "" || ce.ParamArchetype || ce.Version || ce.RMType == "EHR"
	})
}

// paramIssues runs the Layer-2 parameter-binding checks against a Query's
// Parameters map.
func paramIssues(md Metadata, q *aql.Query) []Issue {
	var issues []Issue

	// aql_unbound_param — every $name referenced MUST have a Parameters key.
	for _, name := range md.Params {
		if _, ok := q.Parameters[name]; !ok {
			issues = append(issues, Issue{
				Code:     "aql_unbound_param",
				Detail:   fmt.Sprintf("$%s is referenced but not bound in Query.Parameters", name),
				Severity: Error,
			})
		}
	}

	// aql_unused_param — a bound parameter not referenced is advisory.
	referenced := make(map[string]bool, len(md.Params))
	for _, name := range md.Params {
		referenced[name] = true
	}
	// Sort keys: map iteration order is random, but Result.Issues is
	// documented to be in deterministic discovery order.
	for _, key := range slices.Sorted(maps.Keys(q.Parameters)) {
		if !referenced[key] {
			issues = append(issues, Issue{
				Code:     "aql_unused_param",
				Detail:   fmt.Sprintf("Query.Parameters[%q] is bound but never referenced", key),
				Severity: Warning,
			})
		}
	}
	return issues
}

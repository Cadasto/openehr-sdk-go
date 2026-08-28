package lint

// pathshape.go: the REQ-164 path-shape check group — Layer 2's third group,
// AST-only (no CDR, no OPT), and the second direct consumer of the pinned BMM
// in this package (REQ-164 § The conservative segment walk; the first is
// [versionedObjectIssues]'s VERSIONED_OBJECT conformance question).
//
// It carries the segment WALKER — the shared machinery that types an
// identified path's segments from the class its alias binds — and the one
// check that reads it today, aql_path_repeating_unpredicated. The walk's
// per-path verdict ([pathShape]) records which segments are multi-valued,
// which carry a predicate, and where the walk stopped, because REQ-164's
// aql_fanout_path_grain asks the divergence question of the same walk.
//
// It also carries the group's two PARSE-ONLY checks, which need no walk and no
// RM fact at all: aql_paging_no_order_by (clause presence plus the request
// envelope) and aql_select_no_alias (the projection list).

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// walkStop names why [segmentWalker.walk] ended before typing every segment.
// Every non-[stopNone] value is SILENT: no issue on the stopping segment and
// none on any segment beyond it (REQ-164 § The conservative segment walk).
// The reason is recorded rather than collapsed to a bare "stopped" flag so a
// test can pin each stop BY NAME, as REQ-164 § Acceptance requires, and so a
// later check reading the same walk can tell "the pin says no" from "the pin
// cannot say".
type walkStop int

const (
	// stopNone means the walk typed every segment of the path.
	stopNone walkStop = iota
	// stopUnknownAliasClass means the path's alias binds no class expression,
	// or binds one whose class the pin does not know.
	stopUnknownAliasClass
	// stopParamArchetype means the alias's archetype scope is a `$param`
	// placeholder, whose extent the CDR resolves at execution.
	stopParamArchetype
	// stopUndeclaredAttribute means the pin declares no such attribute on the
	// current class (own or inherited).
	stopUndeclaredAttribute
	// stopTypeOutsideUniverse means the segment typed, but to a name that is
	// not a class of the pinned universe — a BMM generic parameter (EVENT.data
	// is literally typed `T`) or a foundation type (Real, String) — so nothing
	// below it can be typed. REQ-048 leaves generic-parameter resolution out
	// of scope, and REQ-164 § Out of scope keeps it there.
	stopTypeOutsideUniverse
)

// String names the stop for a test failure message. It is diagnostic only: no
// [Issue] field carries a stop reason, because a stop produces no issue.
func (s walkStop) String() string {
	switch s {
	case stopNone:
		return "none"
	case stopUnknownAliasClass:
		return "unknown alias class"
	case stopParamArchetype:
		return "$param archetype scope"
	case stopUndeclaredAttribute:
		return "undeclared attribute"
	case stopTypeOutsideUniverse:
		return "type outside the class universe"
	}
	return fmt.Sprintf("walkStop(%d)", int(s))
}

// segmentShape is what the pinned BMM says about one typed path segment,
// beside what the query said about it.
type segmentShape struct {
	// Index is the segment's position in the path's Segments.
	Index int
	// Name is the attribute name as written.
	Name string
	// Parent is the RM class the attribute was read from.
	Parent string
	// RMType is the attribute's declared type — for a container, the ELEMENT
	// type (the type inside the list), which is what the walk descends into.
	RMType string
	// Predicated is true when the segment carries predicate text. PRESENCE
	// only: the content is never judged (REQ-164 § Path-shape checks — whether
	// a node id is the RIGHT one is Layer 3's question).
	Predicated bool
	// Container is the pin's multiplicity verdict ([rminfo.Lookup.IsContainer]).
	Container bool
}

// pathShape is the conservative walk's verdict on one identified path.
type pathShape struct {
	// Root is the RM class the alias binds; "" when the walk never started.
	Root string
	// Typed are the segments the walk typed, in document order. It is short of
	// the path's own segment list exactly when Stop is not [stopNone].
	Typed []segmentShape
	// Stop is why the walk ended; [stopNone] when every segment typed.
	Stop walkStop
	// StopAt is the index of the first UNTYPED segment — len(Segments) when
	// Stop is [stopNone], and 0 when the walk never started.
	StopAt int
}

// offending returns the typed segments that are multi-valued and carry no
// predicate, in document order — the aql_path_repeating_unpredicated finding
// set for this path.
func (s pathShape) offending() []segmentShape {
	var out []segmentShape
	for _, seg := range s.Typed {
		if seg.Container && !seg.Predicated {
			out = append(out, seg)
		}
	}
	return out
}

// segmentWalker types identified paths against the pinned BMM. Construct it
// with [newSegmentWalker]; one walker serves a whole lint pass.
type segmentWalker struct {
	look    rminfo.Lookup
	classes map[string]parse.ClassExpr
	// known is the pin's class universe as a set. The membership test is what
	// separates "the pin cannot type this" from "the pin says no", which
	// [rminfo.Lookup.AttributeRMType] alone cannot: asking it about a generic
	// parameter `T` answers exactly as asking it about a real class with no
	// such attribute would.
	known map[string]bool
}

// newSegmentWalker builds a walker over look's class universe, with classes as
// the alias→class-expression binding ([Metadata.Aliases]).
func newSegmentWalker(look rminfo.Lookup, classes map[string]parse.ClassExpr) *segmentWalker {
	names := look.KnownRMTypes()
	known := make(map[string]bool, len(names))
	for _, n := range names {
		known[n] = true
	}
	return &segmentWalker{look: look, classes: classes, known: known}
}

// walk types p's segments from the class its alias binds, stopping SILENTLY at
// the first step the pin cannot type (REQ-164 § The conservative segment
// walk). It never guesses and never descends a sibling: a false Warning on a
// legal path is worse than a missed one, because the consumer running this in
// CI cannot tell them apart — the same policy [pathDivergence] applies at
// Layer 3.
func (w *segmentWalker) walk(p parse.IdentifiedPath) pathShape {
	ce := w.classes[p.Alias]
	if ce.ParamArchetype {
		return pathShape{Stop: stopParamArchetype}
	}
	// ONE test for two spellings of the same fact — the alias must bind a class
	// the pin knows. An alias bound to nothing at all reaches it too, without a
	// second guard: an absent key yields the ZERO class expression, whose empty
	// RMType no class universe contains. So an unbound alias
	// (aql_unknown_alias) and an ORDER BY key naming a SELECT `AS` alias
	// (REQ-117 — an AS alias labels a column, it roots no class) stop here
	// exactly as a class the pin does not know (aql_unknown_rm_class) does. In
	// every case the path stays unjudged rather than judged against a guess:
	// each of those defects has its own code, and this group adds nothing to it.
	root := asciiUpperRMType(ce.RMType)
	if !w.known[root] {
		return pathShape{Stop: stopUnknownAliasClass}
	}
	sh := pathShape{Root: root, StopAt: len(p.Segments)}
	current := root
	for i, seg := range p.Segments {
		rmType, declared := w.look.AttributeRMType(current, seg.Name)
		if !declared {
			sh.Stop, sh.StopAt = stopUndeclaredAttribute, i
			return sh
		}
		// The second return is the SAME "does the pin declare it" question
		// [rminfo.Lookup.AttributeRMType] just answered yes to, so it carries
		// nothing new here; and were a Lookup ever to disagree with itself, a
		// false reads as single-valued, which is the silent direction.
		container, _ := w.look.IsContainer(current, seg.Name)
		sh.Typed = append(sh.Typed, segmentShape{
			Index:  i,
			Name:   seg.Name,
			Parent: current,
			RMType: rmType,
			// VERBATIM predicate text, presence only (REQ-113 carries it, and
			// REQ-119 keeps it byte-exact) — no predicate spelling has to be
			// understood for this check to be sound.
			Predicated: seg.Predicate != "",
			Container:  container,
		})
		if i+1 == len(p.Segments) {
			break // the last segment typed; there is nothing below to descend
		}
		if !w.known[rmType] {
			sh.Stop, sh.StopAt = stopTypeOutsideUniverse, i+1
			return sh
		}
		current = rmType
	}
	return sh
}

// pathShapeIssues runs the REQ-164 path-shape group. The three landed codes
// are emitted in the order REQ-164 § Path-shape checks catalogues them —
// aql_path_repeating_unpredicated, then aql_paging_no_order_by, then
// aql_select_no_alias — because they answer questions of different scopes (a
// path, the whole query, a projection item) that share no document order to
// sort by. That fixed sequence is what [Result.Issues] promises for a check
// group, exactly as the REQ-161 group has one.
//
// q is the request envelope from [Options.Query]; only the paging check reads
// it, and a nil q simply leaves that check's envelope arm unable to fire, the
// same way it leaves the parameter-binding checks unable to.
//
// The group is ungated — every code in it is Warning, so none can flip
// [Result.OK], and [Options.Relation] governs none of them: attribute typing
// is a class fact of the pinned RM, which no caller-supplied containment
// relation may answer differently (REQ-164 § Always on, never gated).
func pathShapeIssues(doc *parse.Document, md Metadata, q *aql.Query) []Issue {
	var issues []Issue
	issues = append(issues, repeatingSegmentIssues(md)...)
	issues = append(issues, pagingIssues(doc, q)...)
	issues = append(issues, selectAliasIssues(doc)...)
	return issues
}

// repeatingSegmentIssues raises aql_path_repeating_unpredicated over every
// identified path the document records — SELECT, WHERE and ORDER BY alike
// ([Metadata.Paths], which is [parse.Document.Paths]). The clause scope is a
// MUST, not an optimisation skipped: a WHERE filter or an ORDER BY key over an
// unconstrained repeating segment carries the same which-occurrence ambiguity
// a projected path does (REQ-164 § Path-shape checks).
//
// It reads the extracted [Metadata] alone and no other part of the document,
// which is the whole of its input: the query's paths, and the class each alias
// binds.
func repeatingSegmentIssues(md Metadata) []Issue {
	if len(md.Paths) == 0 {
		return nil
	}
	w := newSegmentWalker(rminfo.Default, md.Aliases)
	var issues []Issue
	for _, p := range md.Paths {
		for _, seg := range w.walk(p).offending() {
			issues = append(issues, Issue{
				Code: "aql_path_repeating_unpredicated",
				Path: displayPath(p),
				Detail: fmt.Sprintf(
					"segment %q steps through the multi-valued %s.%s with no predicate; "+
						"which occurrence is meant is engine-defined",
					seg.Name, seg.Parent, seg.Name,
				),
				Severity: Warning,
				Span:     segmentSpan(p, seg.Index),
			})
		}
	}
	return issues
}

// pagingIssues raises aql_paging_no_order_by: a row-bounded query with no
// `ORDER BY` leaves its page boundary to the engine, so successive pages MAY
// repeat or drop rows (REQ-164 § Path-shape checks).
//
// AT MOST ONE issue per query, whichever channels carried the bound, because
// the defect is the missing total order and there is only ever one of those.
// Detail names the channel(s), which is the only thing that distinguishes the
// two arms for a reader.
//
// The two channels:
//
//   - IN-TEXT, keyed on [parse.Document.HasLimit]. The SDK grammar profile's
//     limitClause admits no OFFSET without a LIMIT, so keying on LIMIT alone
//     misses no in-text spelling.
//   - ENVELOPE, keyed on a supplied q's Fetch OR Offset. Reading Offset too is
//     deliberately unlike [topIssues]'s fetch-only scope: that code reports an
//     exclusion the Query API *states*, whereas an offset into an unordered
//     result produces an unstable page boundary on its own.
//
// A query whose ONLY row bound is a deprecated `TOP` raises nothing here, and
// needs no guard to: HasLimit is false and, with no envelope bound, no channel
// fires. aql_deprecated_top already carries the ORDER BY remedy for that
// clause, and one defect gets one finding (REQ-164 § No double-reporting).
//
// No Span: neither channel has a position to point at. [parse.Document]
// records the LIMIT clause's presence but not its place, and the envelope is
// not in the query text at all — so REQ-109 § Value-free lint diagnostics'
// zero Span (the issue is not attributable to a position) is the honest
// answer rather than an invented one.
func pagingIssues(doc *parse.Document, q *aql.Query) []Issue {
	// The total order is the remedy, so its presence is the whole silence
	// rule — an ORDER BY makes the page boundary well-defined however many
	// channels bounded the rows.
	if doc.HasOrderBy {
		return nil
	}
	var channels []string
	if doc.HasLimit {
		channels = append(channels, "a LIMIT clause")
	}
	// Non-zero, not positive: an envelope carrying a negative bound is still an
	// envelope that asks for a page, and the transport — not this advisory —
	// owns whether that value is usable.
	if q != nil && (q.Fetch != 0 || q.Offset != 0) {
		channels = append(channels, "the request envelope")
	}
	if len(channels) == 0 {
		return nil
	}
	return []Issue{{
		Code: "aql_paging_no_order_by",
		Detail: fmt.Sprintf(
			"the query is row-bounded by %s but carries no ORDER BY; without a total "+
				"order the page boundary is engine-defined, so successive pages may "+
				"repeat or drop rows",
			strings.Join(channels, " and "),
		),
		Severity: Warning,
	}}
}

// selectAliasIssues raises aql_select_no_alias once per projection item that
// carries no `AS` alias: a stored-query contract and every column-addressed
// result reader depend on stable column names, and an unaliased column's name
// is engine-defined (REQ-164 § Path-shape checks).
//
// It reads the STRUCTURED projection ([parse.Document.Query]) because the flat
// lint view cannot answer the question: [parse.Document.SelectAliases] lists
// the aliases that WERE written, with nothing to say which items wrote them,
// and [Metadata.Paths] would count a path nested inside an aliased function
// call as an item of its own. A non-nil [parse.Document.QueryErr] is not fatal
// here, exactly as it is not for the REQ-161 junction checks: the structured
// AST is best-effort by contract (REQ-119) and a dropped shape simply goes
// unchecked, which is the conservative direction.
//
// TWO exemptions, and only two:
//
//   - a `*` item, which has nothing to alias — that shape is REQ-109's
//     aql_select_star, and REQ-164 § No double-reporting gives it to that code.
//     A BARE `SELECT *` never reaches the loop at all, since it leaves Items
//     empty by construction; the mixed `SELECT *, col` form carries the star as
//     an item, and it is skipped here.
//   - an item that wrote an alias.
//
// A BARE ALIAS item (`SELECT o`) is NOT exempt: it names no column either, and
// the engine picks that column's name as freely as it picks a path's.
func selectAliasIssues(doc *parse.Document) []Issue {
	q := doc.Query()
	if q == nil {
		return nil
	}
	var issues []Issue
	for i, item := range q.Select.Items {
		if _, star := item.Expr.(parse.StarExpr); star {
			continue
		}
		if item.Alias != "" {
			continue
		}
		path, span := selectItemSite(item)
		issues = append(issues, Issue{
			Code: "aql_select_no_alias",
			Path: path,
			// The ORDINAL, not the item's text: it locates the item for a reader
			// counting projections even where the item shape carries no position
			// to span, and it counts every item including a mixed star, so it is
			// the ordinal the reader counts commas to.
			Detail: fmt.Sprintf(
				"SELECT item %d carries no AS alias; the result column's name is then "+
					"engine-defined, and a stored-query contract depends on a stable one",
				i+1,
			),
			Severity: Warning,
			Span:     span,
		})
	}
	return issues
}

// selectItemSite is where a projected item is, for a diagnostic: its path
// spelling and the span its source text occupies.
//
// Only a bare path item has either. [parse.IdentifiedPath] carries a position
// and its verbatim source ([parse.IdentifiedPath.Raw], byte-exact since
// REQ-119), whereas a function call and a literal projection carry neither —
// [parse.SelectItem] has no position of its own, and REQ-113 models a
// projection's structure, not its layout.
//
// Those items therefore report NEITHER rather than a derived guess: spanning a
// function call's first argument would point at a path INSIDE the item instead
// of at the item, and naming that argument in [Issue.Path] would report a path
// the item does not project. REQ-109 § Value-free lint diagnostics has an
// unattributable issue report no position rather than a wrong one, and Detail's
// ordinal locates it either way.
func selectItemSite(item parse.SelectItem) (string, Span) {
	pe, ok := item.Expr.(parse.PathExpr)
	if !ok {
		return "", Span{}
	}
	return displayPath(pe.IdentifiedPath), spanOfText(pe.Pos, pe.Raw)
}

// segmentSpan is the span the idx-th segment's ATTRIBUTE NAME occupies in the
// query text. [parse.PathSegment] carries no position of its own (REQ-113
// models a path's structure, not its layout), so the position is derived from
// the path's own start and its VERBATIM source text — which is why the derived
// answer can be trusted: since REQ-119 [aql.IdentifiedPath.Raw] is byte-exact
// source, not a re-rendering.
//
// The name alone, not the whole segment: an offending segment carries no
// predicate by definition, so there is nothing else of it to cover.
//
// It returns the zero Span rather than a guess when the scan cannot place the
// segment — REQ-109 § Value-free lint diagnostics makes an unattributable
// issue report no position, and a span that pointed at the wrong text would be
// worse than none.
func segmentSpan(p parse.IdentifiedPath, idx int) Span {
	if p.Pos == (parse.Position{}) || idx < 0 || idx >= len(p.Segments) {
		return Span{}
	}
	name := p.Segments[idx].Name
	off, ok := segmentNameOffset(p, idx)
	if !ok {
		return Span{}
	}
	return spanOfText(advance(p.Pos, p.Raw[:off]), name)
}

// segmentNameOffset returns the byte offset within p.Raw at which the idx-th
// segment's name starts.
//
// It MATCHES the parse against the source rather than re-tokenizing it: the
// alias, each segment name and each predicate BODY are already carried
// verbatim (REQ-113, byte-exact since REQ-119), so the scan only has to step
// over the punctuation between them — `[`, `]`, `/` — and the trivia the lexer
// skipped. Nothing inside a predicate is ever read as structure, because the
// predicate's own bytes are matched whole.
//
// That is what makes the answer trustworthy on the spellings a re-tokenizer
// has to model and gets wrong: an escaped quote (`[name/value='it\'s']` — AQL
// escapes with a backslash and has no SQL-style quote doubling, see
// [aql.StringValue]'s token rules), a `]` or a `/` inside a string literal or
// inside a `MATCHES {/…/}` regex, and an AQL comment carrying either.
//
// The second return is false when the source does not read as the parse says
// it does — a hand-built path whose Raw was never source among them. The
// caller reports no span rather than a guess in that case.
func segmentNameOffset(p parse.IdentifiedPath, idx int) (int, bool) {
	raw := p.Raw
	// The alias is required at offset 0 EXACTLY, no leading trivia admitted:
	// p.Pos addresses the alias, so anchoring there is what keeps the two in
	// step — trivia skipped here would be counted twice by the caller's
	// [advance]. A Raw that starts anywhere else yields no span.
	if !strings.HasPrefix(raw, p.Alias) {
		return 0, false
	}
	i, ok := skipPredicate(raw, len(p.Alias), p.Predicate)
	if !ok {
		return 0, false
	}
	for n, seg := range p.Segments {
		if i = skipRawTrivia(raw, i); i >= len(raw) || raw[i] != '/' {
			return 0, false
		}
		i = skipRawTrivia(raw, i+1)
		if !strings.HasPrefix(raw[i:], seg.Name) {
			return 0, false
		}
		if n == idx {
			return i, true
		}
		i += len(seg.Name)
		if i, ok = skipPredicate(raw, i, seg.Predicate); !ok {
			return 0, false
		}
	}
	return 0, false
}

// expect steps over text at or just after i (trivia first), reporting false
// when the source does not carry it there.
func expect(raw string, i int, text string) (int, bool) {
	i = skipRawTrivia(raw, i)
	if !strings.HasPrefix(raw[i:], text) {
		return 0, false
	}
	return i + len(text), true
}

// skipPredicate steps over `[` + predicate + `]` when predicate is non-empty,
// and is a no-op when it is. predicate is the VERBATIM body between the
// brackets — interior padding and any comment included — so matching it whole
// needs no model of what is inside it.
func skipPredicate(raw string, i int, predicate string) (int, bool) {
	if predicate == "" {
		return i, true
	}
	return expect(raw, i, "["+predicate+"]")
}

// skipRawComment returns the offset just past the `--` line comment starting
// at i (the end of the text when it carries no newline).
func skipRawComment(raw string, i int) int {
	if n := strings.IndexByte(raw[i:], '\n'); n >= 0 {
		return i + n + 1
	}
	return len(raw)
}

// skipRawTrivia returns the offset of the first byte at or after i that is
// neither whitespace nor part of a `--` comment.
func skipRawTrivia(raw string, i int) int {
	for i < len(raw) {
		switch {
		case raw[i] == ' ' || raw[i] == '\t' || raw[i] == '\r' || raw[i] == '\n':
			i++
		case raw[i] == '-' && i+1 < len(raw) && raw[i+1] == '-':
			i = skipRawComment(raw, i)
		default:
			return i
		}
	}
	return i
}

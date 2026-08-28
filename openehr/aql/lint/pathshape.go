package lint

// pathshape.go: the REQ-164 path-shape check group — Layer 2's third group,
// AST-only (no CDR, no OPT), and the second direct consumer of the pinned BMM
// in this package (REQ-164 § The conservative segment walk; the first is
// [versionedObjectIssues]'s VERSIONED_OBJECT conformance question).
//
// It carries the segment WALKER — the shared machinery that types an
// identified path's segments from the class its alias binds — and the two
// checks that read it: aql_path_repeating_unpredicated, over every clause's
// paths, and aql_fanout_path_grain, over the projected ones. The walk runs ONCE
// per path ([walkPaths]) and both checks read the same verdicts, which is what
// REQ-164 § The conservative segment walk means by their sharing one walk. That
// per-path verdict ([pathShape]) records which segments are multi-valued, which
// carry a predicate, and where the walk stopped.
//
// It also carries the group's two PARSE-ONLY checks, which need no walk and no
// RM fact at all: aql_paging_no_order_by (clause presence plus the request
// envelope) and aql_select_no_alias (the projection list).
//
// And it carries the group's one RELATION check, aql_contains_redundant_step,
// which reads neither the walk nor the projection but the FROM/CONTAINS tree,
// and asks the REQ-160 relation whether a step provably does nothing.

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
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

// offendingFrom returns the FIRST unpredicated multi-valued segment at or after
// segment index from — what aql_fanout_path_grain asks of each path once the
// divergence index is known.
//
// It reads [pathShape.Typed] through [pathShape.offending], so a segment the
// walk never typed can never answer yes: everything at or beyond a silent stop
// is simply absent, which is the bound REQ-164 § The conservative segment walk
// puts on this check's reach. Segments typed BEFORE a stop do participate,
// which is the same paragraph's explicit ruling — a path that stops at
// `EVENT.data`'s generic `T` still carries its typed `HISTORY.events` into the
// divergence test.
func (s pathShape) offendingFrom(from int) (segmentShape, bool) {
	for _, seg := range s.offending() {
		if seg.Index >= from {
			return seg, true
		}
	}
	return segmentShape{}, false
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

// pathShapeIssues runs the REQ-164 path-shape group. The five codes are
// emitted in the order REQ-164 § Path-shape checks catalogues them —
// aql_path_repeating_unpredicated, then aql_paging_no_order_by, then
// aql_select_no_alias, then aql_fanout_path_grain, then
// aql_contains_redundant_step — because they answer questions of different
// scopes (a path, the whole query, a projection item, a PAIR of projected
// paths, a containment step) that share no document order to sort by. That
// fixed sequence is what [Result.Issues] promises for a check group, exactly as
// the REQ-161 group has one.
//
// q is the request envelope from [Options.Query]; only the paging check reads
// it, and a nil q simply leaves that check's envelope arm unable to fire, the
// same way it leaves the parameter-binding checks unable to.
//
// rel is the REQ-160 containment relation from [Options.Relation]; only
// aql_contains_redundant_step reads it, and nil means the default relation
// rather than a switched-off check (REQ-161 § Relation supply). The other four
// codes consult NO relation: attribute typing and multiplicity are class facts
// of the pinned RM, which no caller-supplied containment relation may answer
// differently (REQ-164 § The conservative segment walk).
//
// The group is ungated — every code in it is Warning, so none can flip
// [Result.OK] (REQ-164 § Always on, never gated).
func pathShapeIssues(doc *parse.Document, md Metadata, q *aql.Query, rel *contain.TypeRelation) []Issue {
	walked := walkPaths(md)
	var issues []Issue
	issues = append(issues, repeatingSegmentIssues(walked)...)
	issues = append(issues, pagingIssues(doc, q)...)
	issues = append(issues, selectAliasIssues(doc)...)
	issues = append(issues, fanoutPathGrainIssues(walked)...)
	issues = append(issues, redundantStepIssues(doc, md, rel)...)
	return issues
}

// walkedPath is one identified path beside the walk's verdict on it. The two
// travel together — rather than as a path list and a parallel shape list — so
// that no check can pair a path with another path's verdict, and so that adding
// a third reader of the walk needs no new index discipline.
type walkedPath struct {
	// Path is the identified path as the document recorded it.
	Path parse.IdentifiedPath
	// Shape is the conservative walk's verdict on Path.
	Shape pathShape
}

// walkPaths is the group's ONE segment walk: it types every identified path the
// document records — SELECT, WHERE and ORDER BY alike — and returns them in
// document order.
//
// Both RM-fact checks in the group read this result rather than walking again.
// REQ-164 § The conservative segment walk says the repeating-segment check and
// aql_fanout_path_grain's divergence test "share one walk", and sharing the
// RESULT is what makes that true by construction: the two checks cannot drift
// into answering two walks that merely look alike, and a path is typed once
// however many checks ask about it.
func walkPaths(md Metadata) []walkedPath {
	if len(md.Paths) == 0 {
		return nil
	}
	w := newSegmentWalker(rminfo.Default, md.Aliases)
	walked := make([]walkedPath, len(md.Paths))
	for i, p := range md.Paths {
		walked[i] = walkedPath{Path: p, Shape: w.walk(p)}
	}
	return walked
}

// repeatingSegmentIssues raises aql_path_repeating_unpredicated over every
// identified path the document records — SELECT, WHERE and ORDER BY alike
// ([Metadata.Paths], which is [parse.Document.Paths]). The clause scope is a
// MUST, not an optimisation skipped: a WHERE filter or an ORDER BY key over an
// unconstrained repeating segment carries the same which-occurrence ambiguity
// a projected path does (REQ-164 § Path-shape checks).
//
// Its whole input is [walkPaths]'s output — the query's paths, already typed
// against the class each alias binds — and no other part of the document.
func repeatingSegmentIssues(walked []walkedPath) []Issue {
	var issues []Issue
	for _, wp := range walked {
		p := wp.Path
		for _, seg := range wp.Shape.offending() {
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

// fanoutPathGrainIssues raises aql_fanout_path_grain: two PROJECTED identified
// paths rooted on the SAME alias that diverge after their longest common
// prefix, EACH passing at least one unpredicated multi-valued segment at or
// after the divergence. That is the Cartesian-product shape — the two branches
// repeat independently, and how many rows the engine returns for it is
// undefined (SPECQUERY-9), which is the fact Detail states exactly as its
// sibling aql_fanout_row_grain does (REQ-164 § Path-shape checks).
//
// PROJECTION ONLY, unlike its group-mate the repeating-segment check, which is
// required to read every clause. The two are asking different questions: an
// unconstrained repeating segment is ambiguous wherever it stands, whereas a
// row PRODUCT needs two columns to multiply — a WHERE filter or an ORDER BY key
// over a second repeating scope returns no column and multiplies nothing. A
// path nested inside a projection expression (a function call's argument) is
// NOT exempted: the parser records it in the SELECT clause, and the rows it
// contributes are fanned out before any function over them is applied.
//
// ONCE PER ALIAS, not once per pair: a wide projection yields one advisory
// rather than a quadratic report. The pair loop is therefore ordered by the
// LATER path (outer) and then by the earlier one (inner), which is what makes
// "the first offending pair in document order" mean what a reader sees — the
// finding is reported AT its later path, so the pair whose later path comes
// first in the query is the one that fixes the Span. Ordering the search by the
// earlier path instead could report a pair at the end of a projection while an
// offending pair sat wholly before it.
//
// The DIVERGENCE index is the length of the two paths' longest common prefix by
// segment NAME ([commonPrefixLen]) — never by predicate text. Two spellings of
// a predicate on one attribute are a content question, and § The conservative
// segment walk keeps this group's reading of a predicate to PRESENCE: a check
// that had to tell `[at0001]` from `[at0002]` to place a divergence would be
// judging content. Reading names alone also errs silent, which is the direction
// REQ-164 requires where a fact is not provable.
//
// Each MUST NOT of the catalogue row falls out of that, with no arm of its own:
//
//   - multi-valued segments all in the COMMON PREFIX — one repeating scope, no
//     product — leaves neither path an offending segment at or after the
//     divergence, so no pair forms;
//   - a path PREDICATED on every multi-valued segment at or after the
//     divergence has no offending segment there either, so a pair needing BOTH
//     halves never forms;
//   - paths on DIFFERENT aliases never pair, because the inner loop matches the
//     alias — that is the junction question, aql_fanout_row_grain (REQ-161),
//     and the two codes are disjoint by construction: this one needs two paths
//     under one alias, that one two projected aliases under one AND junction;
//   - a bare `SELECT *` roots no identified path at all, so there is nothing to
//     pair. (The star's own row-grain question belongs to the junction code,
//     whose rule reads a star as projecting every alias.)
//
// An aliasless path needs no guard either: the walk roots at the class the
// alias binds, so a path that names none has an unstarted walk and no typed
// segment to offend with.
func fanoutPathGrainIssues(walked []walkedPath) []Issue {
	var issues []Issue
	// One finding per alias, and the first pair found for it wins — the loops
	// below reach pairs in exactly the order the rule names.
	reported := make(map[string]bool)
	for j, lp := range walked {
		later := lp.Path
		if later.Clause != parse.ClauseSelect || reported[later.Alias] {
			continue
		}
		for i := range j {
			earlier := walked[i].Path
			// The clause half of this test is UNREACHABLE by construction and
			// kept as defence in depth, so no test witnesses it: the parser
			// fills [parse.Document.Paths] in source order, so nothing before a
			// SELECT path can be a WHERE or ORDER BY one. The alias half is the
			// live guard — it is what makes two aliases never pair.
			if earlier.Clause != parse.ClauseSelect || earlier.Alias != later.Alias {
				continue
			}
			at := commonPrefixLen(earlier, later)
			// Both halves, or no pair: one branch repeating under another's
			// fixed occurrence is a single scope, not a product.
			first, ok := walked[i].Shape.offendingFrom(at)
			if !ok {
				continue
			}
			second, ok := lp.Shape.offendingFrom(at)
			if !ok {
				continue
			}
			reported[later.Alias] = true
			issues = append(issues, Issue{
				Code: "aql_fanout_path_grain",
				// The path the Span covers, so a reader correlating the two
				// reads one location twice rather than two.
				Path: displayPath(later),
				Detail: fmt.Sprintf(
					"projected paths %q and %q are rooted on alias %q and descend into "+
						"different unpredicated repeating scopes after their common prefix "+
						"(%s.%s and %s.%s); row multiplicity for this shape is engine-defined "+
						"(SPECQUERY-9) — verify the result shape against the target CDR",
					displayPath(earlier), displayPath(later), later.Alias,
					first.Parent, first.Name, second.Parent, second.Name,
				),
				Severity: Warning,
				// The LATER path of the pair, whole: the defect is the pairing,
				// so no single segment of it is the offence, and the later path
				// is where a reader meets the pair.
				Span: spanOfText(later.Pos, later.Raw),
			})
			break
		}
	}
	return issues
}

// commonPrefixLen is how many leading segments a and b spell the same way — the
// index at which the two paths diverge, and len of the shorter when one is a
// prefix of the other (a path with nothing beyond the shared part cannot be
// half of a product).
//
// NAMES only; see [fanoutPathGrainIssues] for why a predicate is not read here.
func commonPrefixLen(a, b parse.IdentifiedPath) int {
	n := min(len(a.Segments), len(b.Segments))
	for i := range n {
		if a.Segments[i].Name != b.Segments[i].Name {
			return i
		}
	}
	return n
}

// redundantStepIssues raises aql_contains_redundant_step: a CONTAINS operand
// whose removal provably changes nothing (REQ-164 § Path-shape checks). ALL
// five conditions must hold, and each is a separate arm below:
//
//   - it is NOT the FROM root — a root anchors the tree, it is not a step;
//   - it is NOT a leaf — an unreferenced leaf is an existence filter, and does
//     work;
//   - it carries NO class-position predicate of any kind
//     ([parse.ClassExpr.HasPredicate]: archetype, standing or version alike);
//   - its alias roots NO identified path outside FROM/CONTAINS;
//   - and its class is UNAVOIDABLE under the REQ-160 relation
//     ([contain.TypeRelation.Unavoidable]) — every containment route from its
//     PARENT operand's class to its CHILD operand's class passes through it.
//
// The last is the substance of the rule, not caution about it. The guidance
// sentence this check comes from ("containment is minimal") would flag every
// unreferenced operand and be wrong on two of the three shapes: an unreferenced
// leaf filters, and an unreferenced intermediate NARROWS whenever it is
// AVOIDABLE — dropping `SECTION s` from `… CONTAINS SECTION s CONTAINS
// OBSERVATION o` admits observations that sit outside any section. Only the
// unavoidable intermediate — `EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION
// o`, where every EHR→OBSERVATION route passes a COMPOSITION — does nothing.
//
// rel is [Options.Relation], nil meaning the default (REQ-161 § Relation
// supply). It is the relation IN USE that is asked, not [contain.Default]: a
// consumer overlay edge states a containment route fact, and a route round the
// via is exactly what retires a proof — so consulting the default instead would
// raise a false finding on the dialect deployment REQ-160 § Extensibility
// exists to serve. This is the one REQ-164 code a supplied relation governs;
// the other four ask class questions no relation may answer differently.
//
// It reads the STRUCTURED tree ([parse.Document.Query]) for the reason
// [semanticIssues] does: the flat [parse.Document.Classes] is the FROM/CONTAINS
// tree collapsed to document order and cannot say which operands are adjacent,
// which sit under a junction, or what is negated — and all three decide this
// code. A nil Query (structured extraction did not succeed) leaves the check
// silent, the conservative direction, exactly as it leaves the REQ-161
// containment-pair checks silent.
//
// A VERSIONED_OBJECT-conforming operand is SKIPPED whole: REQ-161's
// aql_versioned_object_unreferenced owns that shape, and this code is the
// general case yielding to the specific (REQ-164 § No double-reporting). The
// conformance question is asked of [conformsToVersionedObject] — the pinned
// BMM, not rel — for the reason [versionedObjectIssues] gives: no relation a
// caller can obtain answers an IS-A question differently.
//
// An ANONYMOUS operand (no alias at all) is INCLUDED, not skipped, exactly as
// it is for aql_versioned_object_unreferenced: with no alias no path can
// reference it, so "roots no identified path" holds unconditionally.
func redundantStepIssues(doc *parse.Document, md Metadata, rel *contain.TypeRelation) []Issue {
	q := doc.Query()
	if q == nil {
		return nil
	}
	from := q.From
	// A junction AT the FROM root (`FROM A a OR B b`) leaves Root and Contains
	// zero, and every one of its operands is inside a junction — silent by the
	// rule below rather than by this early return, which is only what makes the
	// chain walk start from a real class.
	if from.Contains == nil || from.Root.RMType == "" {
		return nil
	}
	chain, _ := containsChain([]parse.ClassExpr{from.Root}, *from.Contains)
	if len(chain) < 3 {
		return nil // a root and a leaf, with no step between them
	}
	referenced := make(map[string]bool, len(md.Paths))
	for _, p := range md.Paths {
		referenced[p.Alias] = true
	}

	var issues []Issue
	// The loop bounds ARE two of the five conditions, and they are the two that
	// cannot be got wrong later: index 0 is the FROM root, and the last index is
	// where the chain ends — a genuine leaf, or the operand whose successor a
	// stop removed ([containsChain]), which is silent either way and for the
	// same reason: neither has a child operand to be redundant between.
	for i := 1; i < len(chain)-1; i++ {
		ce := chain[i]
		if ce.HasPredicate {
			continue // an archetype, standing or version predicate selects; it is not inert
		}
		if conformsToVersionedObject(ce.RMType) {
			continue // REQ-161's code owns this operand
		}
		if ce.Alias != "" && referenced[ce.Alias] {
			continue // a projected / filtered / ordered alias is doing work
		}
		parent, child := chain[i-1], chain[i+1]
		if !rel.Unavoidable(parent.RMType, ce.RMType, child.RMType) {
			continue
		}
		issues = append(issues, Issue{
			Code: "aql_contains_redundant_step",
			Path: classToken(ce),
			Detail: fmt.Sprintf(
				"%s carries no predicate and roots no identified path, and every containment "+
					"route from %s to %s passes through %s; the step narrows nothing, so removing "+
					"it leaves the result unchanged",
				classToken(ce), parent.RMType, child.RMType, ce.RMType,
			),
			Severity: Warning,
			// The class expression's Pos starts at its RM type token, so the span
			// covers the class name itself — the same anchor every REQ-161
			// class-expression code uses (see [semanticIssue]).
			Span: spanOfText(ce.Pos, ce.RMType),
		})
	}
	return issues
}

// containsChain appends the CONTAINS chain below n to chain, in containment
// order — the linear ancestor→descendant sequence
// aql_contains_redundant_step's parent/child questions are asked along.
//
// A [parse.Containment] carries no parent pointer, so the order has to be
// rebuilt. On a CLASS node the Children are the flattened chain below it, each
// following the PREVIOUS one with a CONTAINS keyword
// ([parse.Containment.Children]) — so `n` with children `[c1[c1a], c2]` reads
// `n CONTAINS c1 CONTAINS c1a CONTAINS c2`, which the depth-first append
// produces exactly. The read-side extractor nests a parsed chain one level per
// CONTAINS (a single child each), so the recursion normally goes straight down;
// the flattened spelling is the same tree and is handled identically, which is
// the rule [containCheck.walk] threads through its `prev` for the REQ-161 pair
// checks.
//
// The chain STOPS, contributing nothing further, at the first node that is:
//
//   - a JUNCTION ([isJunction]) — every operand inside a containment junction
//     is silent (removing one changes the junction's arity, which the relation
//     alone cannot prove inert, REQ-164 § Path-shape checks), and so is the
//     operand ABOVE it, whose single child class the pair question needs and
//     which a junction does not supply;
//   - NEGATED — every operand under a negation is silent for the twin reason:
//     removing one changes the excluded set. The flag sits on the CONTAINS
//     chain's target ([parse.Containment]), so stopping AT the flagged node
//     covers the negated operand and everything below it in one test;
//   - a class node with an EMPTY RM type — the degenerate dropped operand
//     (REQ-119). It decides nothing, and neither can a pair built on it.
//
// Truncating rather than skipping is what keeps the surviving prefix honest:
// the last surviving element is never checked (the loop stops one short of it),
// so an operand whose real child was dropped at a stop is never judged against
// the wrong one. The second return says whether the chain ran to its end, which
// is how a stop DEEP inside one child truncates the whole chain rather than
// letting a following sibling be appended as if it followed the stopped node.
func containsChain(chain []parse.ClassExpr, n parse.Containment) ([]parse.ClassExpr, bool) {
	if n.Negated || isJunction(n) || n.Class.RMType == "" {
		return chain, false
	}
	chain = append(chain, n.Class)
	for _, ch := range n.Children {
		var whole bool
		if chain, whole = containsChain(chain, ch); !whole {
			return chain, false
		}
	}
	return chain, true
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

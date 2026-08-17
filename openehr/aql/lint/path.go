package lint

import (
	"errors"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// ErrEmptyPath is returned by [Normalise] for an identified path with no
// alias root. A well-formed parse never yields one (the grammar requires a
// leading IDENTIFIER); it guards against zero-value misuse.
var ErrEmptyPath = errors.New("lint: identified path has no alias")

// Path is an alias-stripped identified path. Suffix is the canonical,
// alias-free path string (`/attr[pred]/attr...`) for diagnostics; Layer 3
// walks Segments against the archetype-scoped compiled subtree.
type Path struct {
	// Alias is the original root binding (e.g. "o").
	Alias string
	// Segments are the path steps after the alias, copied from the source —
	// predicate text VERBATIM, exactly as the read side reports it (REQ-119).
	Segments []parse.PathSegment
	// Suffix is the canonical alias-free path; "" for a bare alias.
	//
	// CANONICAL means the lexer's skipped trivia is normalised out of each
	// segment predicate — one space per interior run, ends dropped — so
	// spellings of one path that differ only in trivia share a suffix, and no
	// skipped newline or AQL comment reaches a line-oriented report. A VALUE's
	// own bytes ride through verbatim (inside a string literal or a term-code
	// display name the lexer skips nothing — see [aql.StripPredicateTrivia]).
	// Since REQ-119 the source text is verbatim, so building the suffix by
	// concatenation put `/items[at0001 -- note\n]` into a line-oriented report.
	// Localised diagnostics reach this form through [displayPath], which is
	// what sets [Issue.Path].
	Suffix string
}

// displayPath renders an identified path for a DIAGNOSTIC: the alias, its
// root predicate (if any) and the segments, in [Path.Suffix]'s canonical form
// with the alias restored. [Issue.Path] is what a line-oriented report prints,
// and since REQ-119 the parsed text is VERBATIM — rendering `p.Raw` put a
// predicate's comment and its newline into a single-line report. The verbatim
// text stays on the AST ([aql.IdentifiedPath.Raw]) for round-trip; a
// diagnostic names the path, it does not re-emit it.
func displayPath(p parse.IdentifiedPath) string {
	norm, err := Normalise(p)
	if err != nil {
		return p.Raw // no alias: nothing structural to render, and Raw is all there is
	}
	var sb strings.Builder
	sb.WriteString(p.Alias)
	if pred := aql.StripPredicateTrivia(p.Predicate); pred != "" {
		sb.WriteByte('[')
		sb.WriteString(pred)
		sb.WriteByte(']')
	}
	sb.WriteString(norm.Suffix)
	return sb.String()
}

// Normalise strips the alias from an identified path and yields the canonical
// alias-free segment list and suffix string. It is purely structural — it
// does not resolve the alias or consult a template (that is Layer 3's job).
func Normalise(p parse.IdentifiedPath) (Path, error) {
	if p.Alias == "" {
		return Path{}, ErrEmptyPath
	}
	var sb strings.Builder
	for _, seg := range p.Segments {
		sb.WriteByte('/')
		sb.WriteString(seg.Name)
		// Trivia-stripped: see [Path.Suffix]. The verbatim text stays on
		// Segments, which is what Layer 3 walks.
		if pred := aql.StripPredicateTrivia(seg.Predicate); pred != "" {
			sb.WriteByte('[')
			sb.WriteString(pred)
			sb.WriteByte(']')
		}
	}
	return Path{Alias: p.Alias, Segments: slices.Clone(p.Segments), Suffix: sb.String()}, nil
}

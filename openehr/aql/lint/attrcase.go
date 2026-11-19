package lint

import (
	"fmt"
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// codeAttributeCase is the REQ-109 § Layer 2 code for an attribute name in a
// path that carries an upper-case letter.
const codeAttributeCase = "aql_path_attribute_case"

// attributeCaseIssues raises aql_path_attribute_case for every attribute name
// in a path that carries an ASCII upper-case letter (REQ-109 § Layer 2). Every
// attribute the pinned Reference Model declares is lower case, so such a step
// names nothing, whereas the keywords, function names and class names around
// it are read in any case.
//
// It reads the steps of every identified path, and the relative paths inside
// the structured predicates on a path's alias root, on its steps and on a
// FROM / CONTAINS class. A predicate the parser does not structure (a nil
// Parsed) is not read: its steps are only text, and re-tokenising it here would
// model the lexer a second time.
//
// The issues come back in document order, ordered by the position of the path
// or class each one belongs to.
func attributeCaseIssues(doc *parse.Document) []Issue {
	type positioned struct {
		at    parse.Position
		issue Issue
	}
	var found []positioned
	add := func(at parse.Position, issues []Issue) {
		for _, is := range issues {
			found = append(found, positioned{at: at, issue: is})
		}
	}
	for _, p := range doc.Paths {
		where := displayPath(p)
		whole := spanOfText(p.Pos, p.Raw)
		var issues []Issue
		issues = predicateAttributeCase(issues, p.ParsedPredicate, where, whole)
		for i, seg := range p.Segments {
			if hasASCIIUpper(seg.Name) {
				issues = append(issues, attributeCaseIssue(seg.Name, where, segmentSpan(p, i)))
			}
			issues = predicateAttributeCase(issues, seg.Parsed, where, whole)
		}
		add(p.Pos, issues)
	}
	for _, ce := range doc.Classes {
		add(ce.Pos, predicateAttributeCase(nil, ce.ParsedPredicate, classToken(ce), spanOfText(ce.Pos, ce.RMType)))
	}
	if len(found) == 0 {
		return nil
	}
	slices.SortStableFunc(found, func(a, b positioned) int {
		if a.at.Line != b.at.Line {
			return a.at.Line - b.at.Line
		}
		return a.at.Col - b.at.Col
	})
	out := make([]Issue, len(found))
	for i, f := range found {
		out[i] = f.issue
	}
	return out
}

// predicateAttributeCase appends a finding for every upper-case attribute name
// in the relative paths of a structured predicate: the left-hand path of a
// comparison or a regex match, through any AND / OR junction. A node id, an
// archetype id and a parameter name no attribute, so they add nothing.
func predicateAttributeCase(issues []Issue, p aql.SegmentPredicate, where string, span Span) []Issue {
	switch p := p.(type) {
	case aql.ComparisonPredicate:
		return relativePathAttributeCase(issues, p.Comparison.ParsedPath, where, span)
	case aql.MatchesPredicate:
		return relativePathAttributeCase(issues, p.ParsedPath, where, span)
	case aql.JunctionPredicate:
		issues = predicateAttributeCase(issues, p.Left, where, span)
		return predicateAttributeCase(issues, p.Right, where, span)
	}
	return issues
}

// relativePathAttributeCase checks the steps of a path inside a predicate. Such
// a path binds no alias: the parser leaves Alias empty and every step,
// the first included, is a segment.
func relativePathAttributeCase(issues []Issue, ip *aql.IdentifiedPath, where string, span Span) []Issue {
	if ip == nil {
		return issues
	}
	issues = predicateAttributeCase(issues, ip.ParsedPredicate, where, span)
	for _, seg := range ip.Segments {
		if hasASCIIUpper(seg.Name) {
			issues = append(issues, attributeCaseIssue(seg.Name, where, span))
		}
		issues = predicateAttributeCase(issues, seg.Parsed, where, span)
	}
	return issues
}

func attributeCaseIssue(name, where string, span Span) Issue {
	return Issue{
		Code: codeAttributeCase,
		Path: where,
		Detail: fmt.Sprintf("attribute %q is not lower case: openEHR attribute names are lower case, so the path names nothing; write %q",
			name, asciiLower(name)),
		Severity: Error,
		Span:     span,
	}
}

// hasASCIIUpper reports whether s carries an ASCII upper-case letter. The
// grammar's IDENTIFIER is ASCII-only, so no other letter can reach a step.
func hasASCIIUpper(s string) bool {
	for i := range len(s) {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			return true
		}
	}
	return false
}

// asciiLower folds ASCII A-Z to a-z and leaves every other byte alone, the
// mirror of [asciiUpperRMType].
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

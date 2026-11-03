package templatecompile

// slots_from_model.go: a slot's rules, its archetype-id patterns and the
// predicate of an unidentified slot's path segment, read from the includes
// and excludes of an ARCHETYPE_SLOT in the operational template model, as
// the deprecated parser reads them from an OPT.

import (
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// slotRulesFromModel returns the rules of the slot s: its RM type, and each
// archetype-id pattern its includes and excludes give that compiles. A pattern
// that does not compile is left out, and RawIncludeCount counts every include
// that is not empty, so a consumer can tell a slot whose includes all failed
// from one that has none.
func slotRulesFromModel(s *aom14.ArchetypeSlot) constraints.SlotRules {
	var raw int
	for _, a := range s.Includes {
		if !assertionIsEmpty(a) {
			raw++
		}
	}
	return constraints.SlotRules{
		RMTypeName:      strings.TrimSpace(s.RMTypeName),
		Includes:        slotAssertions(s.Includes),
		Excludes:        slotAssertions(s.Excludes),
		RawIncludeCount: raw,
	}
}

// slotAssertions compiles every archetype-id pattern the assertions give, in
// order, leaving out one that does not compile, or returns nil when none
// does.
func slotAssertions(as []aom14.Assertion) []constraints.SlotAssertion {
	var out []constraints.SlotAssertion
	for _, a := range as {
		for _, pattern := range assertionSlotPatterns(a) {
			sa, err := constraints.NewSlotAssertion(pattern)
			if err != nil {
				continue
			}
			out = append(out, sa)
		}
	}
	return out
}

// slotPatterns returns the archetype-id pattern of each assertion, the first
// it gives as the slot rules read it, in order, leaving out an assertion that
// gives none. These are a slot's includes and excludes on the compiled node.
func slotPatterns(as []aom14.Assertion) []string {
	var out []string
	for _, a := range as {
		if patterns := assertionSlotPatterns(a); len(patterns) > 0 {
			out = append(out, patterns[0])
		}
	}
	return out
}

// slotPathPredicateFromModel returns the predicate of the AQL path segment of
// a slot with no node_id: the archetype-id pattern of its first include that
// gives one, with its regex escapes stripped, so sibling slots under one
// attribute do not collide; or "" when no include gives a pattern.
func slotPathPredicateFromModel(s *aom14.ArchetypeSlot) string {
	for _, a := range s.Includes {
		if patterns := assertionSlotPatterns(a); len(patterns) > 0 {
			return strings.ReplaceAll(patterns[0], `\`, "")
		}
	}
	return ""
}

// assertionSlotPatterns returns the archetype-id patterns the assertion a
// gives, in this order: the `archetype_id matches {…}` text of its string
// expression, then its expression tree where that matches archetype_id/value
// against a C_STRING. Each is trimmed and stripped of its enclosing slashes,
// and one the assertion has already given is not listed again.
func assertionSlotPatterns(a aom14.Assertion) []string {
	var patterns []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, "/")
		p = strings.TrimSuffix(p, "/")
		if p != "" && !slices.Contains(patterns, p) {
			patterns = append(patterns, p)
		}
	}
	if a.StringExpression != nil {
		if p := archetypeIDMatchesPattern(*a.StringExpression); p != "" {
			add(p)
		}
	}
	if p := expressionSlotPattern(a.Expression); p != "" {
		add(p)
	}
	return patterns
}

// archetypeIDMatchesPattern returns the trimmed text between the braces of
// the first `archetype_id … matches {…}` in text, the case of the two words
// ignored and an escaped brace kept inside, or "" when text holds none.
func archetypeIDMatchesPattern(text string) string {
	const prefix = "archetype_id"
	lower := strings.ToLower(text)
	i := strings.Index(lower, prefix)
	if i < 0 {
		return ""
	}
	matchesAt := strings.Index(lower[i+len(prefix):], "matches")
	if matchesAt < 0 {
		return ""
	}
	openAt := i + len(prefix) + matchesAt + len("matches")
	openRel := strings.IndexByte(text[openAt:], '{')
	if openRel < 0 {
		return ""
	}
	start := openAt + openRel + 1
	depth := 1
	escaped := false
	for pos := start; pos < len(text); pos++ {
		switch ch := text[pos]; {
		case escaped:
			escaped = false
		case ch == '\\':
			escaped = true
		case ch == '{':
			depth++
		case ch == '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(text[start:pos])
			}
		}
	}
	return ""
}

// expressionSlotPattern returns the pattern of the expression tree e where it
// is the match the slot rules read, `archetype_id/value matches {pattern}`:
// a binary matches operator whose left operand is the String attribute
// archetype_id/value and whose right operand is a C_STRING constraint; or ""
// for any other tree.
func expressionSlotPattern(e aom14.ExprItem) string {
	op, ok := e.(*aom14.ExprBinaryOperator)
	if !ok || op.Operator != aom14.OperatorKindMatches {
		return ""
	}
	left, ok := op.LeftOperand.(*aom14.ExprLeaf)
	if !ok ||
		!strings.EqualFold(strings.TrimSpace(left.Type), "String") ||
		!strings.EqualFold(strings.TrimSpace(left.ReferenceType), "attribute") ||
		leafText(left.Item) != "archetype_id/value" {
		return ""
	}
	right, ok := op.RightOperand.(*aom14.ExprLeaf)
	if !ok ||
		!strings.EqualFold(strings.TrimSpace(right.Type), "C_STRING") ||
		!strings.EqualFold(strings.TrimSpace(right.ReferenceType), "constraint") {
		return ""
	}
	cs, ok := right.Item.(*aom14.CString)
	if !ok || cs.Pattern == nil {
		return ""
	}
	return strings.TrimSpace(*cs.Pattern)
}

// leafText returns the trimmed text of an expression leaf's item read as a
// string, or "" for an item of another kind.
func leafText(item any) string {
	switch v := item.(type) {
	case string:
		return strings.TrimSpace(v)
	case *aom14.TypedString:
		if v != nil {
			return strings.TrimSpace(v.Value)
		}
	}
	return ""
}

// assertionIsEmpty reports whether the assertion a holds nothing at all, as
// an `<includes/>` with no content decodes.
func assertionIsEmpty(a aom14.Assertion) bool {
	return a.Tag == nil && a.StringExpression == nil && a.Expression == nil && len(a.Variables) == 0
}

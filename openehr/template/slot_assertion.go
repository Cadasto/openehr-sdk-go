package template

import (
	"encoding/xml"
	"regexp"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

var (
	slotStringExprRE = regexp.MustCompile(`(?is)<string_expression>([^<]*)</string_expression>`)
	slotValueRE      = regexp.MustCompile(`(?is)<value>(.*?)</value>`)
)

// parseSlotAssertions parses raw OPT slot assertion XML / text blobs
// into compiled [constraints.SlotAssertion] values. Unparseable
// entries are skipped (best-effort); callers retain [Slot.Includes]
// for the original wire form.
func parseSlotAssertions(raw []string) []constraints.SlotAssertion {
	if len(raw) == 0 {
		return nil
	}
	out := make([]constraints.SlotAssertion, 0, len(raw))
	for _, blob := range raw {
		for _, pattern := range extractSlotPatterns(blob) {
			a, err := constraints.NewSlotAssertion(pattern)
			if err != nil {
				continue
			}
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func extractSlotPatterns(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var patterns []string
	if p := extractTextSlotPattern(raw); p != "" {
		patterns = append(patterns, normalizeSlotPattern(p))
	}
	// Ocean Template Designer <string_expression> shape.
	if m := slotStringExprRE.FindStringSubmatch(raw); len(m) == 2 {
		if p := extractTextSlotPattern(m[1]); p != "" {
			patterns = append(patterns, normalizeSlotPattern(p))
		}
	}
	for _, p := range extractXMLSlotPatterns(raw) {
		patterns = append(patterns, normalizeSlotPattern(p))
	}
	if len(patterns) == 0 {
		if v := extractXMLChardataValue(raw); v != "" {
			if p := extractTextSlotPattern(v); p != "" {
				patterns = append(patterns, normalizeSlotPattern(p))
			}
		}
	}
	return patterns
}

type slotExpressionWrapper struct {
	Expressions []slotExpression `xml:"expression"`
}

type slotExpression struct {
	Operator string      `xml:"operator"`
	Left     slotOperand `xml:"left_operand"`
	Right    slotOperand `xml:"right_operand"`
}

type slotOperand struct {
	Type          string          `xml:"type"`
	Item          slotOperandItem `xml:"item"`
	ReferenceType string          `xml:"reference_type"`
}

type slotOperandItem struct {
	Text    string `xml:",chardata"`
	Pattern string `xml:"pattern"`
}

func extractXMLSlotPatterns(raw string) []string {
	wrapped := `<root xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">` + raw + `</root>`
	var w slotExpressionWrapper
	if err := xml.Unmarshal([]byte(wrapped), &w); err != nil {
		return nil
	}
	var patterns []string
	for _, expr := range w.Expressions {
		if !expr.isSupportedSlotMatch() {
			continue
		}
		if p := strings.TrimSpace(expr.Right.Item.Pattern); p != "" {
			patterns = append(patterns, p)
		}
	}
	return patterns
}

func (e slotExpression) isSupportedSlotMatch() bool {
	return strings.TrimSpace(e.Operator) == "2007" &&
		strings.EqualFold(strings.TrimSpace(e.Left.Type), "String") &&
		strings.TrimSpace(e.Left.Item.Text) == "archetype_id/value" &&
		strings.EqualFold(strings.TrimSpace(e.Left.ReferenceType), "attribute") &&
		strings.EqualFold(strings.TrimSpace(e.Right.Type), "C_STRING") &&
		strings.EqualFold(strings.TrimSpace(e.Right.ReferenceType), "constraint")
}

func extractTextSlotPattern(raw string) string {
	const prefix = "archetype_id"
	lower := strings.ToLower(raw)
	i := strings.Index(lower, prefix)
	if i < 0 {
		return ""
	}
	rest := lower[i+len(prefix):]
	matchesAt := strings.Index(rest, "matches")
	if matchesAt < 0 {
		return ""
	}
	openAt := i + len(prefix) + matchesAt + len("matches")
	openRel := strings.IndexByte(raw[openAt:], '{')
	if openRel < 0 {
		return ""
	}
	start := openAt + openRel + 1
	depth := 1
	escaped := false
	for pos := start; pos < len(raw); pos++ {
		ch := raw[pos]
		switch {
		case escaped:
			escaped = false
		case ch == '\\':
			escaped = true
		case ch == '{':
			depth++
		case ch == '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(raw[start:pos])
			}
		}
	}
	return ""
}

func normalizeSlotPattern(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	return p
}

func extractXMLChardataValue(raw string) string {
	m := slotValueRE.FindStringSubmatch(raw)
	if len(m) != 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// slotRules builds [constraints.SlotRules] for a wire-side slot.
// RawIncludeCount carries the pre-parse blob count so consumers can
// detect the fail-open case where every include failed to compile.
func (s *Slot) slotRules() constraints.SlotRules {
	return constraints.SlotRules{
		RMTypeName:      s.rmTypeName,
		Includes:        s.parsedIncludes,
		Excludes:        s.parsedExcludes,
		RawIncludeCount: len(s.includes),
	}.Clone()
}

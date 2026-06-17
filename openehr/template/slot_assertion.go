package template

import (
	"regexp"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

var (
	slotXMLPatternRE = regexp.MustCompile(`(?is)<pattern>([^<]*)</pattern>`)
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
	for _, m := range slotXMLPatternRE.FindAllStringSubmatch(raw, -1) {
		if p := strings.TrimSpace(m[1]); p != "" {
			patterns = append(patterns, normalizeSlotPattern(p))
		}
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

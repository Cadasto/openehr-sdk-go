package template

import (
	"regexp"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

var (
	slotTextMatchesRE = regexp.MustCompile(`(?is)archetype_id\s+matches\s*\{([^}]+)\}`)
	slotXMLPatternRE  = regexp.MustCompile(`(?is)<pattern>([^<]*)</pattern>`)
	slotStringExprRE  = regexp.MustCompile(`(?is)<string_expression>([^<]*)</string_expression>`)
)

// parseSlotAssertions parses raw OPT slot assertion XML / text blobs
// into compiled [constraints.SlotAssertion] values. Unparseable
// entries are skipped (best-effort); callers retain [Slot.RawIncludes]
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
	if m := slotTextMatchesRE.FindStringSubmatch(raw); len(m) == 2 {
		patterns = append(patterns, normalizeSlotPattern(m[1]))
	}
	// Ocean Template Designer <string_expression> shape.
	if m := slotStringExprRE.FindStringSubmatch(raw); len(m) == 2 {
		if sub := slotTextMatchesRE.FindStringSubmatch(m[1]); len(sub) == 2 {
			patterns = append(patterns, normalizeSlotPattern(sub[1]))
		}
	}
	for _, m := range slotXMLPatternRE.FindAllStringSubmatch(raw, -1) {
		if p := strings.TrimSpace(m[1]); p != "" {
			patterns = append(patterns, normalizeSlotPattern(p))
		}
	}
	if len(patterns) == 0 {
		if v := extractXMLChardataValue(raw); v != "" {
			if m := slotTextMatchesRE.FindStringSubmatch(v); len(m) == 2 {
				patterns = append(patterns, normalizeSlotPattern(m[1]))
			}
		}
	}
	return patterns
}

func normalizeSlotPattern(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	return p
}

func extractXMLChardataValue(raw string) string {
	const open = "<value>"
	const close = "</value>"
	i := strings.Index(strings.ToLower(raw), open)
	if i < 0 {
		return ""
	}
	start := i + len(open)
	j := strings.Index(strings.ToLower(raw[start:]), close)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(raw[start : start+j])
}

// slotRules builds [constraints.SlotRules] for a wire-side slot.
func (s *Slot) slotRules() constraints.SlotRules {
	return constraints.SlotRules{
		RMTypeName: s.rmTypeName,
		Includes:   s.parsedIncludes,
		Excludes:   s.parsedExcludes,
	}
}

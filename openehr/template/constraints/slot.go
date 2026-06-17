package constraints

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// SlotAssertion is one parsed ARCHETYPE_SLOT include or exclude
// expression. v1 supports the `archetype_id matches {regex}` subset
// and the OPT XML expression shape that carries a C_STRING pattern
// (operator 2007 / "matches").
//
// REQ-104.
type SlotAssertion struct {
	pattern string
	re      *regexp.Regexp
}

// NewSlotAssertion compiles pattern as a Go regexp (POSIX-flavoured,
// as in other REQ-103 string constraints). Returns an error when the
// pattern is empty or does not compile.
func NewSlotAssertion(pattern string) (SlotAssertion, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return SlotAssertion{}, errors.New("slot assertion: empty pattern")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return SlotAssertion{}, fmt.Errorf("slot assertion: %w", err)
	}
	return SlotAssertion{pattern: pattern, re: re}, nil
}

// Pattern returns the compiled regular expression source string.
func (a SlotAssertion) Pattern() string { return a.pattern }

// MatchesArchetypeID reports whether archetypeID satisfies the
// assertion pattern.
func (a SlotAssertion) MatchesArchetypeID(archetypeID string) bool {
	if a.re == nil {
		return false
	}
	return a.re.MatchString(archetypeID)
}

// SlotRules is the parsed include / exclude assertion set for one
// ARCHETYPE_SLOT. When Includes is empty the caller MUST apply the
// RM-type-prefix fallback via [SlotRules.AllowsRMTypePrefix].
type SlotRules struct {
	RMTypeName string
	Includes   []SlotAssertion
	Excludes   []SlotAssertion
}

// AllowsArchetypeID reports whether archetypeID satisfies the slot's
// include / exclude rules. When no include assertions were parsed,
// falls back to the RM-type-prefix heuristic.
func (r SlotRules) AllowsArchetypeID(archetypeID string) bool {
	for _, ex := range r.Excludes {
		if ex.MatchesArchetypeID(archetypeID) {
			return false
		}
	}
	if len(r.Includes) > 0 {
		for _, inc := range r.Includes {
			if inc.MatchesArchetypeID(archetypeID) {
				return true
			}
		}
		return false
	}
	return r.AllowsRMTypePrefix(archetypeID)
}

// HasParsedIncludes reports whether at least one include assertion
// was parsed. When false, [AllowsArchetypeID] uses the prefix
// fallback exclusively.
func (r SlotRules) HasParsedIncludes() bool { return len(r.Includes) > 0 }

// AllowsRMTypePrefix is the v1 pragmatic fallback: archetype ids of
// the form openEHR-EHR-<RMType>.<concept>.v<n> fit a slot constrained
// to RMType.
func (r SlotRules) AllowsRMTypePrefix(archetypeID string) bool {
	if r.RMTypeName == "" {
		return false
	}
	return strings.HasPrefix(archetypeID, "openEHR-EHR-"+r.RMTypeName+".")
}

// ExampleArchetypeID returns a synthetic archetype id guaranteed to
// match the first include assertion, or the prefix-fallback example
// when no includes were parsed. Used by the instance synthesiser.
func (r SlotRules) ExampleArchetypeID() string {
	if len(r.Includes) > 0 {
		if id := exampleFromPattern(r.Includes[0].pattern); id != "" {
			return id
		}
	}
	if r.RMTypeName != "" {
		return "openEHR-EHR-" + r.RMTypeName + ".example.v1"
	}
	return ""
}

// exampleFromPattern derives a literal archetype id that satisfies
// simple OPT patterns (no unbounded repetition). Returns "" when the
// pattern is too complex to synthesise safely.
func exampleFromPattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	// Common OPT shape: openEHR-EHR-CLUSTER.device(-suffix)*\.v1
	if strings.Contains(pattern, "(") || strings.Contains(pattern, "|") ||
		strings.Contains(pattern, "[") || strings.Contains(pattern, "*") ||
		strings.Contains(pattern, "+") || strings.Contains(pattern, "?") {
		simplified := strings.ReplaceAll(pattern, `\.`, ".")
		simplified = strings.ReplaceAll(simplified, `(-[a-zA-Z0-9_]+)*`, "")
		simplified = strings.ReplaceAll(simplified, "(-[a-zA-Z0-9_]+)*", "")
		if re, err := regexp.Compile(pattern); err == nil && re.MatchString(simplified) {
			return simplified
		}
		return ""
	}
	simplified := strings.ReplaceAll(pattern, `\.`, ".")
	if re, err := regexp.Compile(pattern); err == nil && re.MatchString(simplified) {
		return simplified
	}
	return ""
}

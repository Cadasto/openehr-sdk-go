package constraints

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// SlotAssertion is one parsed ARCHETYPE_SLOT include or exclude
// expression. v1 supports the `archetype_id matches {regex}` subset
// and the OPT XML expression shape that carries a C_STRING pattern
// (operator 2007 / "matches").
//
// Construct only via [NewSlotAssertion]; the zero value carries no
// compiled regex and [SlotAssertion.MatchesArchetypeID] reports
// false for it (it is not a valid assertion, not a match-all).
//
// REQ-104.
type SlotAssertion struct {
	pattern string
	re      *regexp.Regexp
}

// NewSlotAssertion compiles pattern as a Go regexp / RE2 expression
// (as in other REQ-103 string constraints). Returns an error when the
// pattern is empty or does not compile.
//
// The pattern is matched against a candidate archetype id in full:
// ADL `archetype_id matches {regex}` semantics are whole-string, so
// the compiled regex is anchored (`\A(?:…)\z`). Storing the raw
// source separately keeps [SlotAssertion.Pattern] (diagnostics) and
// example synthesis honest about what the OPT author wrote.
func NewSlotAssertion(pattern string) (SlotAssertion, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return SlotAssertion{}, errors.New("slot assertion: empty pattern")
	}
	re, err := regexp.Compile(anchorPattern(pattern))
	if err != nil {
		return SlotAssertion{}, fmt.Errorf("slot assertion: %w", err)
	}
	return SlotAssertion{pattern: pattern, re: re}, nil
}

// anchorPattern wraps a raw archetype-id pattern so it matches the
// whole candidate string rather than any substring.
func anchorPattern(pattern string) string {
	return `\A(?:` + pattern + `)\z`
}

// Pattern returns the raw (unanchored) regular expression source
// string as written in the OPT.
func (a SlotAssertion) Pattern() string { return a.pattern }

// MatchesArchetypeID reports whether archetypeID satisfies the
// assertion pattern in full (whole-string match).
func (a SlotAssertion) MatchesArchetypeID(archetypeID string) bool {
	if a.re == nil {
		return false
	}
	return a.re.MatchString(archetypeID)
}

// isUniversal reports whether the assertion matches every archetype
// id — the `.*` (or `.+`) catch-all that template editors emit as
// the auto-generated complement of an includes list. See
// [SlotRules.AllowsArchetypeID] for why it is treated specially.
func (a SlotAssertion) isUniversal() bool {
	switch a.pattern {
	case ".*", "(.*)", ".+", "(.+)":
		return true
	default:
		return false
	}
}

// SlotRules is the parsed include / exclude assertion set for one
// ARCHETYPE_SLOT. When Includes is empty the caller MUST apply the
// RM-type-prefix fallback via [SlotRules.AllowsRMTypePrefix].
//
// RawIncludeCount records how many include assertion blobs the OPT
// carried before parsing. When it is non-zero but Includes is empty
// every include failed to compile and [AllowsArchetypeID] degrades
// to the permissive prefix fallback — a known fail-open limitation
// surfaced via [SlotRules.IncludesDroppedUnparsed].
type SlotRules struct {
	RMTypeName      string
	Includes        []SlotAssertion
	Excludes        []SlotAssertion
	RawIncludeCount int
}

// Clone returns a copy whose exported slices do not alias the
// receiver. SlotRules is a value object, but Includes / Excludes are
// exported for inspection and therefore need defensive copies at API
// boundaries.
func (r SlotRules) Clone() SlotRules {
	r.Includes = slices.Clone(r.Includes)
	r.Excludes = slices.Clone(r.Excludes)
	return r
}

// IncludesDroppedUnparsed reports the fail-open case: the OPT
// declared include assertions but none compiled, so the slot widens
// to the RM-type-prefix fallback instead of the authored constraint.
func (r SlotRules) IncludesDroppedUnparsed() bool {
	return len(r.Includes) == 0 && r.RawIncludeCount > 0
}

// AllowsArchetypeID reports whether archetypeID satisfies the slot's
// include / exclude rules. When no include assertions were parsed,
// falls back to the RM-type-prefix heuristic — including the
// fail-open case flagged by [SlotRules.IncludesDroppedUnparsed].
//
// A catch-all exclude (`.*`) is ignored when includes are present:
// template editors auto-generate such an exclude as the complement
// of an includes list ("fill with only these"), so applying it
// literally would reject the slot's own includes.
func (r SlotRules) AllowsArchetypeID(archetypeID string) bool {
	hasIncludes := len(r.Includes) > 0
	for _, ex := range r.Excludes {
		if hasIncludes && ex.isUniversal() {
			continue
		}
		if ex.MatchesArchetypeID(archetypeID) {
			return false
		}
	}
	if hasIncludes {
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

// AllowsRMTypePrefix is the v1 pragmatic fallback: archetype ids with
// prefix openEHR-EHR-<RMType>. fit a slot constrained to RMType.
func (r SlotRules) AllowsRMTypePrefix(archetypeID string) bool {
	if r.RMTypeName == "" {
		return false
	}
	return strings.HasPrefix(archetypeID, "openEHR-EHR-"+r.RMTypeName+".")
}

// ExampleArchetypeID returns a synthetic archetype id that matches
// the first include assertion when one can be synthesized safely. If
// includes exist but are too complex to synthesize, returns "". When
// no includes were parsed, returns the RM-type-prefix fallback
// example. Used by the instance synthesiser.
func (r SlotRules) ExampleArchetypeID() string {
	if len(r.Includes) > 0 {
		if id := exampleFromPattern(r.Includes[0].pattern); id != "" {
			return id
		}
		return ""
	}
	if r.RMTypeName != "" {
		return "openEHR-EHR-" + r.RMTypeName + ".example.v1"
	}
	return ""
}

// archetypeIDShapeRE is the canonical openEHR archetype-id shape
// (openEHR-<rm>-<class>.<concept>.v<n>). A synthesised example must
// match it, otherwise exampleFromPattern bails out rather than emit
// an id that merely satisfies the source regex (e.g. a literal `.*`).
var archetypeIDShapeRE = regexp.MustCompile(`^openEHR-[A-Za-z]+-[A-Za-z0-9_]+\.[A-Za-z0-9_-]+\.v[0-9]+$`)

// exampleFromPattern derives a literal archetype id that satisfies
// simple OPT patterns (no unbounded repetition). Returns "" when the
// pattern is too complex to synthesise a conforming id from.
func exampleFromPattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	// Flat alternation (A|B|…) is the common closed-includes shape:
	// synthesise from the first alternative.
	if before, _, ok := strings.Cut(pattern, "|"); ok {
		return exampleFromPattern(before)
	}
	// Drop the optional ADL suffix group, then unescape literal dots.
	simplified := strings.ReplaceAll(pattern, `(-[a-zA-Z0-9_]+)*`, "")
	simplified = strings.ReplaceAll(simplified, `\.`, ".")
	// Reject anything that still carries regex structure: the result
	// must be a literal, well-shaped archetype id and must satisfy the
	// source pattern.
	if !archetypeIDShapeRE.MatchString(simplified) {
		return ""
	}
	if re, err := regexp.Compile(anchorPattern(pattern)); err == nil && re.MatchString(simplified) {
		return simplified
	}
	return ""
}

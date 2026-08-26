package aql

// version_internal_test.go reaches the unexported containment carrier to pin
// the one REQ-163 refusal the public API cannot put a query into: a version
// predicate on a class node that is not spelled VERSION. [Version] fixes the RM
// type to the SDK's own spelling, so the state is unreachable from outside —
// the guard exists so a future constructor, or a widened field, cannot regress
// it silently. The public surface is covered from aql_test (version_test.go).
// REQ-163 · PROBE-088

import (
	"errors"
	"strings"
	"testing"
)

// TestVersionPredicateRefusedOffVersionNode pins the REQ-163 rule that
// `versionPredicate` is reachable from classExprOperand's VERSION alternative
// alone: the bracket has no position on any other class expression, so a node
// carrying one MUST be refused rather than emitted as text the parser rejects.
//
// The refusal names the constructor that DOES carry the shape, so the caller is
// pointed at a route rather than left with a grammar citation.
func TestVersionPredicateRefusedOffVersionNode(t *testing.T) {
	for _, rmType := range []string{"COMPOSITION", "OBSERVATION", "VERSIONED_COMPOSITION"} {
		t.Run(rmType, func(t *testing.T) {
			c := Containment{rmType: rmType, alias: "x", versionPred: LatestVersion()}
			err := c.validateTree(map[string]bool{})
			if !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("validateTree = %v, want ErrInvalidQuery", err)
			}
			if !strings.Contains(err.Error(), "aql.Version") {
				t.Errorf("refusal does not name the constructor that carries the shape: %v", err)
			}
		})
	}
}

// TestVersionPredicateSpellingFoldIsASCII pins the FOLD the VERSION-spelling
// test uses, which is not interchangeable with the archetype refusal's beside
// it because the two have opposite polarity.
//
// This refusal fires when the node is NOT VERSION, so a WIDER fold accepts
// more: under [strings.EqualFold], `VERſION` (U+017F, which folds to `s`) would
// count as the keyword and carry a bracket to the wire — a spelling the lexer,
// whose keyword fragments are ASCII, cannot read. [asciiKeyword] is what keeps
// the accept set to the spellings the lexer actually tokenises, in both
// directions: every ASCII casing is accepted, the Unicode fold-equal is not.
func TestVersionPredicateSpellingFoldIsASCII(t *testing.T) {
	accepted := []string{"VERSION", "version", "Version"}
	for _, rmType := range accepted {
		t.Run("accepts "+rmType, func(t *testing.T) {
			c := Containment{rmType: rmType, alias: "v", versionPred: LatestVersion()}
			if err := c.validateVersionPredicate(); err != nil {
				t.Fatalf("validateVersionPredicate(%q) = %v, want nil", rmType, err)
			}
		})
	}
	// The Unicode fold-equal spelling, refused here as well as by
	// [validateRMTypeToken] one check earlier — the redundancy is the point.
	t.Run("refuses the Unicode fold-equal spelling", func(t *testing.T) {
		c := Containment{rmType: "VERſION", alias: "v", versionPred: LatestVersion()}
		if err := c.validateVersionPredicate(); !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("validateVersionPredicate = %v, want ErrInvalidQuery", err)
		}
	})
}

// TestVersionPredicateVocabularyIsThreeShapes pins the sealed sum's size: the
// grammar's `versionPredicate` is a fixed three-way choice that does not
// recurse into its own position, so a fourth shape would be a grammar change.
// The compiler seals the interface (both methods are unexported); this pins
// that the three constructors are three DISTINCT shapes rather than aliases of
// one, which no emission table could distinguish from a correctly-rendering
// single shape.
func TestVersionPredicateVocabularyIsThreeShapes(t *testing.T) {
	shapes := map[string]VersionPredicate{
		"latest":     LatestVersion(),
		"all":        AllVersions(),
		"comparison": VersionCompare("a/b", OpEq, Int(1)),
	}
	seen := map[string]string{}
	for name, p := range shapes {
		key := typeKey(p)
		if other, dup := seen[key]; dup {
			t.Errorf("%s and %s are the same shape (%s); the three alternatives must stay distinct",
				name, other, key)
		}
		seen[key] = name
	}
}

// typeKey renders a predicate's concrete type without reflection (the idiom
// spec bans it): each shape's bracket text is grammar-distinct, and the two
// keyword shapes are singletons, so the rendered text identifies the shape.
func typeKey(p VersionPredicate) string {
	switch p.(type) {
	case latestVersion:
		return "latestVersion"
	case allVersions:
		return "allVersions"
	case versionComparison:
		return "versionComparison"
	default:
		return "unknown"
	}
}

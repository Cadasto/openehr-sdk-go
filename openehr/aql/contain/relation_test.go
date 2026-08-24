package contain_test

import (
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestCanContainAcceptanceTable transcribes the pair rows of REQ-160
// § Acceptance — the representative rows are the relation oracle (the
// operand-question rows live in TestContainable). A failing row means either
// the reachability logic or a BMM-fact assumption is wrong.
func TestCanContainAcceptanceTable(t *testing.T) {
	r := contain.Default()
	cases := []struct {
		ancestor, descendant string
		want                 contain.Verdict
	}{
		// Admissible — overlay + closure, by-value, abstract operands.
		{"EHR", "COMPOSITION", contain.Admissible},
		{"EHR", "OBSERVATION", contain.Admissible},
		{"COMPOSITION", "OBSERVATION", contain.Admissible},
		{"COMPOSITION", "ELEMENT", contain.Admissible}, // depth-skip
		{"SECTION", "SECTION", contain.Admissible},
		{"CLUSTER", "CLUSTER", contain.Admissible},
		{"INSTRUCTION", "ACTIVITY", contain.Admissible},
		{"ENTRY", "CLUSTER", contain.Admissible}, // abstract operand
		{"EHR_STATUS", "ELEMENT", contain.Admissible},
		{"FOLDER", "FOLDER", contain.Admissible},
		// Admissible — version tier (overlay).
		{"EHR", "VERSION", contain.Admissible},
		{"VERSIONED_OBJECT", "VERSION", contain.Admissible},
		{"VERSION", "COMPOSITION", contain.Admissible},
		{"VERSIONED_FOLDER", "VERSION", contain.Admissible},
		{"VERSION", "EHR_ACCESS", contain.Admissible},
		{"VERSION", "PARTY", contain.Admissible},
		// Admissible — deliberate over-admission via the family-agnostic hub.
		{"VERSIONED_FOLDER", "COMPOSITION", contain.Admissible},
		{"EHR", "PERSON", contain.Admissible},
		// ByReference — FOLDER → COMPOSITION reference hop.
		{"FOLDER", "COMPOSITION", contain.ByReference},
		{"FOLDER", "OBSERVATION", contain.ByReference},
		// Never.
		{"EHR", "VERSIONED_PARTY", contain.Never}, // container unreachable
		{"OBSERVATION", "COMPOSITION", contain.Never},
		{"OBSERVATION", "SECTION", contain.Never},
		{"OBSERVATION", "EVALUATION", contain.Never},
		{"ELEMENT", "CLUSTER", contain.Never},
		// Never — depth >= 1: reachability starts at the successors, so a
		// class with no route back to itself never self-contains, while the
		// genuine self-loops above (SECTION, CLUSTER, FOLDER) stay Admissible.
		{"OBSERVATION", "OBSERVATION", contain.Never}, // entries never contain entries
		{"ELEMENT", "ELEMENT", contain.Never},
		{"COMPOSITION", "EHR", contain.Never},           // any CONTAINS EHR
		{"COMPOSITION", "DV_TEXT", contain.Never},       // Never-containability operand
		{"COMPOSITION", "EVENT_CONTEXT", contain.Never}, // non-LOCATABLE operand
		// UnknownClass.
		{"FOO_BAR", "COMPOSITION", contain.UnknownClass},
		{"COMPOSITION", "FOO_BAR", contain.UnknownClass},
		// UnknownClass beats Never (§ Verdicts precedence), both operand orders.
		{"FOO_BAR", "DV_TEXT", contain.UnknownClass},
		{"DV_TEXT", "FOO_BAR", contain.UnknownClass},
	}
	for _, c := range cases {
		t.Run(c.ancestor+" CONTAINS "+c.descendant, func(t *testing.T) {
			if got := r.CanContain(c.ancestor, c.descendant); got != c.want {
				t.Errorf("CanContain(%q, %q) = %v, want %v", c.ancestor, c.descendant, got, c.want)
			}
		})
	}
}

// TestContainable exercises the operand-containability question (REQ-160
// § Containable operands).
func TestContainable(t *testing.T) {
	r := contain.Default()
	cases := []struct {
		class string
		want  contain.Verdict
	}{
		{"OBSERVATION", contain.Admissible},
		{"COMPOSITION", contain.Admissible},
		{"EHR", contain.Admissible},
		{"VERSION", contain.Admissible},
		{"ORIGINAL_VERSION", contain.Admissible},
		{"IMPORTED_VERSION", contain.Admissible},
		{"VERSIONED_FOLDER", contain.Admissible},
		{"VERSIONED_PARTY", contain.Admissible},
		{"DV_TEXT", contain.Never},
		{"DV_QUANTITY", contain.Never},
		{"EVENT_CONTEXT", contain.Never}, // non-LOCATABLE, non-version-tier
		{"FOO_BAR", contain.UnknownClass},
	}
	for _, c := range cases {
		t.Run(c.class, func(t *testing.T) {
			if got := r.Containable(c.class); got != c.want {
				t.Errorf("Containable(%q) = %v, want %v", c.class, got, c.want)
			}
		})
	}
}

// TestCanContainASCIICaseInsensitive — class matching is ASCII-case-insensitive
// against the BMM canonical spelling (REQ-160 § Reachability semantics).
func TestCanContainASCIICaseInsensitive(t *testing.T) {
	r := contain.Default()
	if got := r.CanContain("composition", "observation"); got != contain.Admissible {
		t.Errorf(`CanContain("composition","observation") = %v, want Admissible`, got)
	}
	if got := r.CanContain("Composition", "ELEMENT"); got != contain.Admissible {
		t.Errorf(`CanContain("Composition","ELEMENT") = %v, want Admissible`, got)
	}
	if got := r.Containable("dv_text"); got != contain.Never {
		t.Errorf(`Containable("dv_text") = %v, want Never`, got)
	}
	// The pin ships mixed-case canonical spellings (foundation classes);
	// folding must match those too — a known non-containable class is Never,
	// not UnknownClass.
	if got := r.Containable("POINT_INTERVAL"); got != contain.Never {
		t.Errorf(`Containable("POINT_INTERVAL") = %v, want Never (canonical spelling Point_interval is pin-known)`, got)
	}
}

// TestEHRbaseCompatibilityGuard — an RM-valid pair a conformant engine admits
// MUST NOT verdict Never (REQ-160 § Overlay edges). Loosening only ever adds
// Admissible/ByReference, so no RM-valid pair should regress to Never.
func TestEHRbaseCompatibilityGuard(t *testing.T) {
	r := contain.Default()
	rmValid := [][2]string{
		{"EHR", "COMPOSITION"},
		{"COMPOSITION", "OBSERVATION"},
		{"COMPOSITION", "ELEMENT"},
		{"SECTION", "SECTION"},
		{"CLUSTER", "CLUSTER"},
		{"ENTRY", "CLUSTER"},
		{"INSTRUCTION", "ACTIVITY"},
		{"EHR_STATUS", "ELEMENT"},
	}
	for _, p := range rmValid {
		if got := r.CanContain(p[0], p[1]); got == contain.Never {
			t.Errorf("EHRbase-compat guard: CanContain(%q, %q) = Never (an RM-valid pair MUST NOT verdict Never)", p[0], p[1])
		}
	}
}

// TestEHRbaseDocumentedDifferences asserts this relation's verdict on each
// pair where observed EHRbase behaviour differs, so the boundary is executable
// documentation (REQ-160 § Acceptance, closing note). Differences are neutral
// observations of engine behaviour, never anyone's defect. Survey: maintainer's
// knowledge base, openehr-kb/notes/ecosystem/ehrbase-aql.md §4.1.2 and
// openehr-kb/notes/aql-language-reference.md §6.1a.
func TestEHRbaseDocumentedDifferences(t *testing.T) {
	r := contain.Default()

	// Pairs this relation verdicts Never that EHRbase observably admits:
	// EHRbase validates containment at storage-root granularity, so an
	// RM-impossible pair sharing a storage root is accepted and returns zero
	// rows (ehrbase-aql.md §4.1.2). The relation answers the RM question.
	neverHereEHRbaseAdmits := [][2]string{
		{"OBSERVATION", "EVALUATION"}, // sibling entries under the COMPOSITION root
		{"ELEMENT", "CLUSTER"},        // metadata-only by-value path, same root
	}
	for _, p := range neverHereEHRbaseAdmits {
		if got := r.CanContain(p[0], p[1]); got != contain.Never {
			t.Errorf("documented difference drifted: CanContain(%q, %q) = %v, want Never (EHRbase admits at root granularity; this relation answers the RM)", p[0], p[1], got)
		}
	}

	// Pairs the RM permits that EHRbase observably restricts for
	// engine-specific reasons — root disambiguation for EHR CONTAINS ELEMENT,
	// a standalone CLUSTER source (ehrbase-aql.md §4.1.2). The relation stays
	// with the RM: restricting them would be a false Never for other
	// conformant engines.
	if got := r.CanContain("EHR", "ELEMENT"); got != contain.Admissible {
		t.Errorf("documented difference drifted: CanContain(EHR, ELEMENT) = %v, want Admissible (EHRbase requires root disambiguation; the RM route exists)", got)
	}
	if got := r.Containable("CLUSTER"); got != contain.Admissible {
		t.Errorf("documented difference drifted: Containable(CLUSTER) = %v, want Admissible (EHRbase restricts a standalone CLUSTER source; the RM admits the operand)", got)
	}
}

// TestNothingContainsEHR — under the default relation a pair with EHR as the
// descendant MUST verdict Never (REQ-160 § Overlay edges), for every pinned
// class, not just the sampled acceptance row: an accidental overlay edge
// ending at EHR would slip past instance checks.
func TestNothingContainsEHR(t *testing.T) {
	r := contain.Default()
	for _, c := range rminfo.Default.KnownRMTypes() {
		if got := r.CanContain(c, "EHR"); got != contain.Never {
			t.Errorf("CanContain(%q, EHR) = %v, want Never (nothing contains EHR)", c, got)
		}
	}
}

// TestVerdictString covers every named verdict plus the out-of-range fallback —
// it is the diagnostic every failure message in this suite prints.
func TestVerdictString(t *testing.T) {
	cases := []struct {
		v    contain.Verdict
		want string
	}{
		{contain.UnknownClass, "UnknownClass"},
		{contain.Never, "Never"},
		{contain.ByReference, "ByReference"},
		{contain.Admissible, "Admissible"},
		{contain.Verdict(42), "Verdict(42)"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			if got := c.v.String(); got != c.want {
				t.Errorf("Verdict(%d).String() = %q, want %q", int(c.v), got, c.want)
			}
		})
	}
}

// TestConcurrentUse exercises Default, CanContain, Containable, and
// WithOverlay from concurrent goroutines: the doc contract is "safe for
// concurrent use", and the verdict memo must keep it true under -race
// (exercised on main CI).
func TestConcurrentUse(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			r := contain.Default()
			_ = r.CanContain("COMPOSITION", "ELEMENT")
			_ = r.CanContain("FOLDER", "OBSERVATION")
			_ = r.Containable("DV_TEXT")
			ext := r.WithOverlay(contain.Edge{From: "PERSON", To: "EHR"})
			_ = ext.CanContain("PERSON", "EHR")
		})
	}
	wg.Wait()
}

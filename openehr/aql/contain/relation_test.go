package contain_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
)

// TestCanContainAcceptanceTable transcribes REQ-160 § Acceptance verbatim — the
// representative rows are the relation oracle. A failing row means either the
// reachability logic or a BMM-fact assumption is wrong.
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
		{"COMPOSITION", "EHR", contain.Never},     // any CONTAINS EHR
		{"COMPOSITION", "DV_TEXT", contain.Never}, // Never-containability operand
		// UnknownClass.
		{"FOO_BAR", "COMPOSITION", contain.UnknownClass},
		{"COMPOSITION", "FOO_BAR", contain.UnknownClass},
	}
	for _, c := range cases {
		if got := r.CanContain(c.ancestor, c.descendant); got != c.want {
			t.Errorf("CanContain(%q, %q) = %v, want %v", c.ancestor, c.descendant, got, c.want)
		}
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
		{"VERSIONED_PARTY", contain.Admissible},
		{"DV_TEXT", contain.Never},
		{"DV_QUANTITY", contain.Never},
		{"FOO_BAR", contain.UnknownClass},
	}
	for _, c := range cases {
		if got := r.Containable(c.class); got != c.want {
			t.Errorf("Containable(%q) = %v, want %v", c.class, got, c.want)
		}
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

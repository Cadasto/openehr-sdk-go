package contain_test

// unavoidable_test.go: TypeRelation.Unavoidable — the reachability-avoiding-a-
// vertex query REQ-164 § The redundant-step ruling names, asked in REQ-160
// § Reachability semantics' route vocabulary.
//
// The rows are acceptance-style, like TestCanContainAcceptanceTable's: each is
// a claim about the pinned RM, so a failing row means either the exclusion walk
// or a BMM-fact assumption is wrong. Both directions carry rows — an
// over-answering proof is worse than a missing one, because the lint code that
// reads it (aql_contains_redundant_step) advises deleting a query step on the
// strength of it.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
)

// TestUnavoidableAcceptanceTable is the relation-level oracle for
// aql_contains_redundant_step: `ancestor CONTAINS via CONTAINS descendant` is a
// step that provably does nothing exactly when the row says true.
func TestUnavoidableAcceptanceTable(t *testing.T) {
	r := contain.Default()
	cases := []struct {
		ancestor, via, descendant string
		want                      bool
		why                       string
	}{
		// --- Proved: every route passes the via ---------------------------
		{
			"EHR", "COMPOSITION", "OBSERVATION", true,
			"REQ-164's own witness: every EHR -> OBSERVATION route passes a COMPOSITION",
		},
		{
			"EHR", "COMPOSITION", "INSTRUCTION", true,
			"the same shape for another entry kind",
		},
		{
			"EHR", "COMPOSITION", "ACTIVITY", true,
			"and one level below an entry — ACTIVITY hangs off INSTRUCTION, itself only under a COMPOSITION",
		},
		{
			"VERSIONED_COMPOSITION", "VERSION", "COMPOSITION", true,
			"the version tier is the only route from a container to its payload (REQ-160 § Overlay edges)",
		},
		{
			"FOLDER", "COMPOSITION", "OBSERVATION", true,
			"proof holds across a ByReference hop: FOLDER reaches OBSERVATION only through the FOLDER -> COMPOSITION reference",
		},

		// --- Not proved: a route goes round the via -----------------------
		{
			"EHR", "SECTION", "OBSERVATION", false,
			"REQ-164's own silence witness: an observation directly in a composition's content needs no section",
		},
		{
			"COMPOSITION", "SECTION", "OBSERVATION", false,
			"the same bypass one level down — COMPOSITION.content holds entries directly",
		},
		{
			"EHR", "COMPOSITION", "ELEMENT", false,
			"an ELEMENT is reached under EHR_STATUS.other_details with no composition anywhere on the route",
		},
		{
			"EHR", "COMPOSITION", "CLUSTER", false,
			"same bypass, one more RM step",
		},
		{
			"EHR", "ENTRY", "ELEMENT", false,
			"abstract via: excluding EVERY concrete entry still leaves the EHR_STATUS route open",
		},
		{
			"EHR", "VERSION", "COMPOSITION", false,
			"the EHR overlay reaches its payload classes directly as well as through the version tier",
		},
		{
			"EHR", "VERSIONED_COMPOSITION", "COMPOSITION", false,
			"same: the container is one route among several, not a gate",
		},
		{
			"COMPOSITION", "OBSERVATION", "ELEMENT", false,
			"any other entry kind, and SECTION-nested content, reach an element without an observation",
		},

		// --- Not proved: the question cannot be asked ----------------------
		{
			"OBSERVATION", "COMPOSITION", "SECTION", false,
			"no route connects the pair at all — that is aql_impossible_containment's finding, not a redundant step",
		},
		{
			"SECTION", "SECTION", "OBSERVATION", false,
			"via stands for the ancestor's own kind: excluding it removes the start, which is a different question",
		},
		{
			"COMPOSITION", "SECTION", "SECTION", false,
			"via stands for the descendant's own kind: excluding it removes the target",
		},
		{
			"EHR", "DV_TEXT", "OBSERVATION", false,
			"a class the relation knows but never admits as a CONTAINS operand proves nothing",
		},
		{
			"EHR", "COMPOSITION", "FOO_BAR", false,
			"an unknown descendant — unknown is not wrong, and it is not proof either",
		},
		{
			"FOO_BAR", "COMPOSITION", "OBSERVATION", false,
			"an unknown ancestor, same reason",
		},
		{
			"EHR", "FOO_BAR", "OBSERVATION", false,
			"an unknown via, same reason",
		},
		{
			"", "", "", false,
			"three empty names name nothing at all",
		},
	}
	for _, c := range cases {
		t.Run(c.ancestor+"/"+c.via+"/"+c.descendant, func(t *testing.T) {
			if got := r.Unavoidable(c.ancestor, c.via, c.descendant); got != c.want {
				t.Errorf("Unavoidable(%q, %q, %q) = %v, want %v — %s",
					c.ancestor, c.via, c.descendant, got, c.want, c.why)
			}
		})
	}
}

// TestUnavoidableIsASCIICaseInsensitive — class matching follows REQ-160
// § Reachability semantics here as everywhere: engines and the EHRbase SDK
// round-trip lower-case class spellings, and a case mismatch is not a semantic
// difference.
func TestUnavoidableIsASCIICaseInsensitive(t *testing.T) {
	r := contain.Default()
	if !r.Unavoidable("ehr", "composition", "observation") {
		t.Error("Unavoidable(ehr, composition, observation) = false; lower-case spellings must answer as upper-case ones do")
	}
	if r.Unavoidable("Ehr", "Section", "Observation") {
		t.Error("Unavoidable(Ehr, Section, Observation) = true; mixed-case spellings must answer as upper-case ones do")
	}
}

// TestUnavoidableOverlayEdgeRetiresAProof pins the direction an overlay edge
// moves this answer: a consumer edge ADDS routes (REQ-160 § Extensibility), and
// a route round the via is exactly what retires a proof. So a relation extended
// with dialect facts can only ever answer "unavoidable" on FEWER triples than
// the default does — which is why the lint check consulting the relation IN USE
// rather than the default is the direction that avoids false findings.
func TestUnavoidableOverlayEdgeRetiresAProof(t *testing.T) {
	def := contain.Default()
	if !def.Unavoidable("EHR", "COMPOSITION", "OBSERVATION") {
		t.Fatal("premise gone: the default no longer proves the EHR/COMPOSITION/OBSERVATION witness")
	}
	// A deployment that files observations straight under the EHR status.
	ext := def.WithOverlay(contain.Edge{From: "EHR_STATUS", To: "OBSERVATION"})
	if ext.Unavoidable("EHR", "COMPOSITION", "OBSERVATION") {
		t.Error("the extended relation still proves the step redundant, though its own edge routes round the COMPOSITION")
	}
	if !def.Unavoidable("EHR", "COMPOSITION", "OBSERVATION") {
		t.Error("extending a relation changed the answer the DEFAULT gives; overlays must not leak (REQ-160 § Extensibility)")
	}
}

// TestUnavoidableHonoursAConsumerEdgeEndpoint — an endpoint the pin does not
// know is a known, containable class of the extended relation (REQ-160
// § Containable operands), so it can stand in any of the three positions. Here
// it is the ancestor whose ONLY exit is the edge, which makes the edge's target
// unavoidable on the way to anything below it.
func TestUnavoidableHonoursAConsumerEdgeEndpoint(t *testing.T) {
	r := contain.Default().WithOverlay(contain.Edge{From: "SITE", To: "VERSIONED_COMPOSITION"})
	if !r.Unavoidable("SITE", "VERSIONED_COMPOSITION", "COMPOSITION") {
		t.Error("Unavoidable(SITE, VERSIONED_COMPOSITION, COMPOSITION) = false; SITE has no other exit than the consumer edge")
	}
	// The default knows no SITE at all, so it proves nothing about it.
	if contain.Default().Unavoidable("SITE", "VERSIONED_COMPOSITION", "COMPOSITION") {
		t.Error("the default relation answered true for a class it does not know")
	}
}

// TestUnavoidableLeavesTheVerdictMemoIntact is the OUTSIDE half of the memo
// guarantee (the inside half, which reads the memo directly, is
// TestUnavoidableDoesNotPoisonTheMemo in relation_internal_test.go). Excluding
// a vertex is a different graph from the one CanContain answers over, so the
// exclusion walk must not share the memo with it. If it did, the pair verdicts
// asked AFTER an Unavoidable call would start reporting Never for pairs the
// relation admits.
func TestUnavoidableLeavesTheVerdictMemoIntact(t *testing.T) {
	r := contain.Default()
	pairs := [][2]string{
		{"EHR", "COMPOSITION"},
		{"EHR", "OBSERVATION"},
		{"COMPOSITION", "OBSERVATION"},
		{"VERSIONED_COMPOSITION", "COMPOSITION"},
		{"FOLDER", "OBSERVATION"},
	}
	before := make([]contain.Verdict, len(pairs))
	for i, p := range pairs {
		before[i] = r.CanContain(p[0], p[1])
	}
	for _, tr := range [][3]string{
		{"EHR", "COMPOSITION", "OBSERVATION"},
		{"EHR", "SECTION", "OBSERVATION"},
		{"VERSIONED_COMPOSITION", "VERSION", "COMPOSITION"},
		{"FOLDER", "COMPOSITION", "OBSERVATION"},
	} {
		_ = r.Unavoidable(tr[0], tr[1], tr[2])
	}
	for i, p := range pairs {
		if got := r.CanContain(p[0], p[1]); got != before[i] {
			t.Errorf("CanContain(%q, %q) = %v after Unavoidable calls, was %v before", p[0], p[1], got, before[i])
		}
	}
}

// TestNilRelationUnavoidableAnswersAsTheDefault is the per-method row for
// REQ-160 § Nil and zero relations, beside the ones nilrelation_test.go carries
// for the other four methods. The reflection sweep there proves only that a nil
// receiver does not PANIC; this proves it gives the default's answer, which is
// the half that would otherwise fail silently.
func TestNilRelationUnavoidableAnswersAsTheDefault(t *testing.T) {
	var nilRel *contain.TypeRelation
	def := contain.Default()
	for _, tr := range [][3]string{
		{"EHR", "COMPOSITION", "OBSERVATION"}, // true
		{"EHR", "SECTION", "OBSERVATION"},     // false
		{"FOO_BAR", "COMPOSITION", "OBSERVATION"},
		{"", "", ""},
	} {
		got, want := nilRel.Unavoidable(tr[0], tr[1], tr[2]), def.Unavoidable(tr[0], tr[1], tr[2])
		if got != want {
			t.Errorf("nil.Unavoidable(%q, %q, %q) = %v, default says %v", tr[0], tr[1], tr[2], got, want)
		}
	}
}

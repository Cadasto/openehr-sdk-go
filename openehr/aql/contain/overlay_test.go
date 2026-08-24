package contain_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestWithOverlayIsImmutable — WithOverlay returns an extended copy and MUST NOT
// alter the default relation (REQ-160 § Extensibility). The demographic
// containment example (AQL-C-010): PERSON CONTAINS EHR is Never by default (any
// class CONTAINS EHR is Never), and a consumer edge makes it Admissible on the
// copy only.
func TestWithOverlayIsImmutable(t *testing.T) {
	base := contain.Default()
	ext := base.WithOverlay(contain.Edge{From: "PERSON", To: "EHR"})

	if got := ext.CanContain("PERSON", "EHR"); got != contain.Admissible {
		t.Errorf("ext.CanContain(PERSON, EHR) = %v, want Admissible (consumer edge honoured)", got)
	}
	if got := base.CanContain("PERSON", "EHR"); got != contain.Never {
		t.Errorf("base.CanContain(PERSON, EHR) = %v, want Never (default MUST be unaltered)", got)
	}
}

// TestConsumerEdgeUnknownEndpointExactName — a consumer edge endpoint the pin
// does not know is matched by exact name and becomes a containable class of the
// extended relation, without leaking into the default (REQ-160 § Extensibility).
func TestConsumerEdgeUnknownEndpointExactName(t *testing.T) {
	base := contain.Default()
	ext := base.WithOverlay(contain.Edge{From: "PERSON", To: "MY_DIALECT_NODE"})

	if got := base.Containable("MY_DIALECT_NODE"); got != contain.UnknownClass {
		t.Errorf("base.Containable(MY_DIALECT_NODE) = %v, want UnknownClass", got)
	}
	if got := ext.Containable("MY_DIALECT_NODE"); got != contain.Admissible {
		t.Errorf("ext.Containable(MY_DIALECT_NODE) = %v, want Admissible (named by an edge)", got)
	}
	if got := ext.CanContain("PERSON", "MY_DIALECT_NODE"); got != contain.Admissible {
		t.Errorf("ext.CanContain(PERSON, MY_DIALECT_NODE) = %v, want Admissible", got)
	}
}

// TestConsumerByReferenceEdge — a ByReference consumer edge yields a ByReference
// pair verdict when it is the only route.
func TestConsumerByReferenceEdge(t *testing.T) {
	base := contain.Default()
	ext := base.WithOverlay(contain.Edge{From: "CLUSTER", To: "EHR_STATUS", ByReference: true})
	if got := ext.CanContain("CLUSTER", "EHR_STATUS"); got != contain.ByReference {
		t.Errorf("ext.CanContain(CLUSTER, EHR_STATUS) = %v, want ByReference", got)
	}
}

// TestVersionTierCoversEveryPinnedContainer is the pin-drift guard (REQ-160
// § Overlay edges). For every concrete VERSIONED_* container the pin ships, the
// default relation MUST wire its version-tier family edges. The VERSIONED_<CLASS>
// naming heuristic pairs a container with its payload class HERE (the test) only;
// the relation never derives the family set by stripping the prefix. A pin bump
// that adds a versioned container fails this test until its family is carried.
func TestVersionTierCoversEveryPinnedContainer(t *testing.T) {
	r := contain.Default()
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default does not implement Hierarchy")
	}
	containers, ok := h.ConcreteDescendants("VERSIONED_OBJECT")
	if !ok || len(containers) == 0 {
		t.Fatalf("ConcreteDescendants(VERSIONED_OBJECT) = %v, %v; want a non-empty set", containers, ok)
	}
	for _, vc := range containers {
		if got := r.CanContain(vc, "VERSION"); got != contain.Admissible {
			t.Errorf("pin drift: CanContain(%q, VERSION) = %v, want Admissible (container→VERSION edge missing)", vc, got)
		}
		payload, found := strings.CutPrefix(vc, "VERSIONED_")
		if !found {
			t.Errorf("concrete versioned container %q does not follow the VERSIONED_<CLASS> naming heuristic", vc)
			continue
		}
		// The generic base VERSIONED_OBJECT<T> has payload "OBJECT", not a real
		// containable class — its payload is the generic parameter T. Only the
		// concrete payload families carry a VERSION→payload edge.
		if r.Containable(payload) != contain.Admissible {
			continue
		}
		if got := r.CanContain("VERSION", payload); got != contain.Admissible {
			t.Errorf("pin drift: CanContain(VERSION, %q) = %v, want Admissible (VERSION→payload edge missing)", payload, got)
		}
	}
}

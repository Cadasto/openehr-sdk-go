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

// TestWithOverlaySiblingsDoNotAlias — two WithOverlay extensions of the same
// parent must not see each other's edges (REQ-160 § Extensibility). A
// regression to appending into the parent's backing array would let one
// sibling's edge silently replace the other's while every single-extension
// test still passes.
func TestWithOverlaySiblingsDoNotAlias(t *testing.T) {
	base := contain.Default()
	ext1 := base.WithOverlay(contain.Edge{From: "PERSON", To: "NODE_ONE"})
	ext2 := base.WithOverlay(contain.Edge{From: "PERSON", To: "NODE_TWO"})

	if got := ext1.CanContain("PERSON", "NODE_ONE"); got != contain.Admissible {
		t.Errorf("ext1.CanContain(PERSON, NODE_ONE) = %v, want Admissible (own edge lost)", got)
	}
	if got := ext1.CanContain("PERSON", "NODE_TWO"); got != contain.UnknownClass {
		t.Errorf("ext1.CanContain(PERSON, NODE_TWO) = %v, want UnknownClass (sibling edge leaked)", got)
	}
	if got := ext2.CanContain("PERSON", "NODE_TWO"); got != contain.Admissible {
		t.Errorf("ext2.CanContain(PERSON, NODE_TWO) = %v, want Admissible (own edge lost)", got)
	}
	if got := ext2.CanContain("PERSON", "NODE_ONE"); got != contain.UnknownClass {
		t.Errorf("ext2.CanContain(PERSON, NODE_ONE) = %v, want UnknownClass (sibling edge leaked)", got)
	}
}

// TestConsumerEdgeKnownEndpointConformance — the spec's own worked example
// (REQ-160 § Extensibility): a consumer edge with a BMM-known endpoint matches
// by conformance, like any derived edge, so WithOverlay(VERSIONED_OBJECT → …)
// covers every VERSIONED_* container.
func TestConsumerEdgeKnownEndpointConformance(t *testing.T) {
	base := contain.Default()
	ext := base.WithOverlay(contain.Edge{From: "VERSIONED_OBJECT", To: "MY_DIALECT_NODE"})

	if got := ext.CanContain("VERSIONED_FOLDER", "MY_DIALECT_NODE"); got != contain.Admissible {
		t.Errorf("ext.CanContain(VERSIONED_FOLDER, MY_DIALECT_NODE) = %v, want Admissible (VERSIONED_FOLDER conforms to the edge's VERSIONED_OBJECT endpoint)", got)
	}
	if got := base.CanContain("VERSIONED_FOLDER", "MY_DIALECT_NODE"); got != contain.UnknownClass {
		t.Errorf("base.CanContain(VERSIONED_FOLDER, MY_DIALECT_NODE) = %v, want UnknownClass (default MUST be unaltered)", got)
	}
}

// TestWithOverlayIgnoresEmptyEndpoint — an edge with an empty endpoint names
// nothing and is ignored; "" never becomes a containable class.
func TestWithOverlayIgnoresEmptyEndpoint(t *testing.T) {
	ext := contain.Default().WithOverlay(contain.Edge{From: "", To: "COMPOSITION"})
	if got := ext.Containable(""); got != contain.UnknownClass {
		t.Errorf(`Containable("") = %v, want UnknownClass (empty endpoint must not register)`, got)
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

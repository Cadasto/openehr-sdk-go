package contain

// Edge is one overlay containment fact the BMM cannot express — a From→To pair
// with a by-reference marker (REQ-160 § Overlay edges, § Extensibility). A
// plain edge contributes an Admissible route through it; a ByReference edge
// contributes a ByReference route. Endpoint matching is conformance-aware for
// BMM-known classes and exact-name for classes the pin does not know
// (REQ-160 § Extensibility).
type Edge struct {
	From, To    string
	ByReference bool
}

// versionFamilies are the payload classes of the concrete VERSIONED_* containers
// the pinned BMM ships (REQ-160 § Overlay edges). This is pin-reviewed overlay
// data, deliberately NOT derived by stripping "VERSIONED_" in the relation: a
// guard test (TestVersionTierCoversEveryPinnedContainer) fails when this set and
// the pin's ConcreteDescendants("VERSIONED_OBJECT") diverge, so a pin bump that
// adds a versioned container is caught until its family edges are carried here.
var versionFamilies = []string{"COMPOSITION", "EHR_STATUS", "EHR_ACCESS", "FOLDER", "PARTY"}

// ehrDirectPayloads are the EHR IM's own versioned objects — EHR's direct
// overlay endpoints (REQ-160 § Overlay edges). The demographic family (PARTY)
// is deliberately absent: the EHR IM versions no parties, so a deployment that
// wants demographic containment adds a consumer edge (§ Extensibility). The
// hub-mediated EHR…PARTY over-admission is a documented missed defect, not a
// direct endpoint — see the acceptance rows EHR CONTAINS PERSON (Admissible)
// vs EHR CONTAINS VERSIONED_PARTY (Never).
var ehrDirectPayloads = []string{"COMPOSITION", "EHR_STATUS", "FOLDER", "EHR_ACCESS"}

// defaultOverlays is the default relation's overlay edge set (REQ-160 § Overlay
// edges). All endpoints are canonical BMM spellings.
func defaultOverlays() []Edge {
	var edges []Edge

	// EHR → its four EHR-IM payload families and their version containers.
	for _, x := range ehrDirectPayloads {
		edges = append(edges, Edge{From: "EHR", To: x})
		edges = append(edges, Edge{From: "EHR", To: "VERSIONED_" + x})
	}

	// The version tier, per pin-shipped family: VERSIONED_X → VERSION → X.
	// VERSION is one generic RM class, so the tier is family-agnostic — a bare
	// VERSION operand reaches any family (REQ-160 § Overlay edges).
	for _, x := range versionFamilies {
		edges = append(edges, Edge{From: "VERSIONED_" + x, To: "VERSION"})
		edges = append(edges, Edge{From: "VERSION", To: x})
	}

	// FOLDER references its compositions — a reference hop, not by-value.
	edges = append(edges, Edge{From: "FOLDER", To: "COMPOSITION", ByReference: true})

	return edges
}

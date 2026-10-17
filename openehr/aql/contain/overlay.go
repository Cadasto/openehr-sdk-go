package contain

// Edge is one overlay containment fact the BMM cannot express: a From→To pair
// with a by-reference marker. A plain edge contributes an Admissible route
// through it; a ByReference edge contributes a ByReference route. Endpoint
// matching is conformance-aware for BMM-known classes and exact-name for
// classes the pinned BMM does not know.
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
	// This is the shorthand engines resolve straight to the payload; keeping
	// it beside the version-tier hop below also keeps a route from FOLDER to
	// COMPOSITION that does not pass through VERSION.
	edges = append(edges, Edge{From: "FOLDER", To: "COMPOSITION", ByReference: true})

	// The same reference hop, landing on the version tier: the RM Common IM
	// directory package says a folder's items are references to other
	// (usually versioned) objects, with VERSIONED_COMPOSITION its worked
	// example. Onward through the family-agnostic VERSION node, FOLDER
	// CONTAINS VERSION and every other payload family resolve ByReference
	// (REQ-160 § Overlay edges).
	//
	// The hop also gives the version tier a reference route back into
	// itself. Every VERSIONED_* container and every VERSION class reaches
	// FOLDER through the VERSION → FOLDER edge above, and FOLDER now reaches
	// VERSIONED_COMPOSITION. So version-tier pairs that had no route move from
	// Never to ByReference: a version-tier class CONTAINS
	// VERSIONED_COMPOSITION, and a VERSION class CONTAINS a VERSION class
	// (ORIGINAL_VERSION CONTAINS ORIGINAL_VERSION, for one).
	// VERSIONED_FOLDER CONTAINS VERSIONED_COMPOSITION is a real reference
	// chain. VERSIONED_PARTY CONTAINS VERSIONED_COMPOSITION is a cross-family
	// over-admission, the same bounded looseness as the hub's, never a false
	// Error (REQ-160 § Overlay edges, § Acceptance).
	edges = append(edges, Edge{From: "FOLDER", To: "VERSIONED_COMPOSITION", ByReference: true})

	return edges
}

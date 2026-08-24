package contain

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestReferenceAttributesTerminateReachability pins the REQ-160 § Reachability
// semantics terminator on a synthetic model. On the current pin no
// reference-typed attribute leads onward to a containable class, so only a
// synthetic OBJECT_REF descendant carrying such an attribute can prove the
// terminator fires — without this, disabling it would pass the whole external
// suite and silently loosen verdicts on a future pin.
func TestReferenceAttributesTerminateReachability(t *testing.T) {
	lk := rminfo.New(map[string]rminfo.ClassMeta{
		"LOCATABLE":  {Abstract: true},
		"OBJECT_REF": {},
		"ROOM": {
			Parents: []string{"LOCATABLE"},
			Attributes: map[string]rminfo.AttrMeta{
				"pointer": {TypeName: "DOOR_REF", DeclaredIn: "ROOM"},
			},
			AttrOrder: []string{"pointer"},
		},
		"DOOR_REF": {
			Parents: []string{"OBJECT_REF"},
			Attributes: map[string]rminfo.AttrMeta{
				"target": {TypeName: "HALL", DeclaredIn: "DOOR_REF"},
			},
			AttrOrder: []string{"target"},
		},
		"HALL": {Parents: []string{"LOCATABLE"}},
	})
	r := build(lk, nil)
	if got := r.Containable("ROOM"); got != Admissible {
		t.Fatalf("Containable(ROOM) = %v, want Admissible (synthetic model wiring broken)", got)
	}
	if got := r.CanContain("ROOM", "HALL"); got != Never {
		t.Errorf("CanContain(ROOM, HALL) = %v, want Never (the ROOM → DOOR_REF reference hop must terminate reachability)", got)
	}
}

// TestLocatableDeclaredAttributesExcludedFromReachability pins the REQ-160 §
// Reachability semantics housekeeping rule: an attribute whose DeclaredOn site
// is LOCATABLE or PATHABLE MUST NOT be a containment edge. On the pinned BMM
// the real case is ELEMENT → CLUSTER via feeder_audit; a synthetic model
// isolates the guard so disabling isInfrastructureAttr would admit a spurious
// route and pass the acceptance table while violating the spec MUST.
func TestLocatableDeclaredAttributesExcludedFromReachability(t *testing.T) {
	lk := rminfo.New(map[string]rminfo.ClassMeta{
		"LOCATABLE": {Abstract: true},
		"ELEMENT": {
			Parents: []string{"LOCATABLE"},
			Attributes: map[string]rminfo.AttrMeta{
				"feeder_audit": {TypeName: "CLUSTER", DeclaredIn: "LOCATABLE"},
			},
			AttrOrder: []string{"feeder_audit"},
		},
		"CLUSTER": {Parents: []string{"LOCATABLE"}},
	})
	r := build(lk, nil)
	if got := r.Containable("ELEMENT"); got != Admissible {
		t.Fatalf("Containable(ELEMENT) = %v, want Admissible (synthetic model wiring broken)", got)
	}
	if got := r.CanContain("ELEMENT", "CLUSTER"); got != Never {
		t.Errorf("CanContain(ELEMENT, CLUSTER) = %v, want Never (LOCATABLE-declared attributes must not be containment edges)", got)
	}
}

// lookupOnly hides rminfo.Default's optional capability interfaces: interface
// embedding promotes only the Lookup methods, so the wrapper satisfies neither
// rminfo.Hierarchy nor rminfo.AttributeLister.
type lookupOnly struct{ rminfo.Lookup }

// TestBuildWithoutCapabilitiesDegradesToOverlayOnly — a Lookup without the
// optional rminfo capability interfaces cannot answer the BMM half, and build
// degrades to an overlay-only relation instead of panicking: every BMM class
// answers UnknownClass (the fail-safe verdict) while overlay edges still match
// by exact name.
func TestBuildWithoutCapabilitiesDegradesToOverlayOnly(t *testing.T) {
	r := build(lookupOnly{rminfo.Default}, defaultOverlays())
	if got := r.Containable("OBSERVATION"); got != UnknownClass {
		t.Errorf("Containable(OBSERVATION) = %v, want UnknownClass (no BMM capabilities, not an overlay endpoint)", got)
	}
	if got := r.Containable("COMPOSITION"); got != Admissible {
		t.Errorf("Containable(COMPOSITION) = %v, want Admissible (exact-name overlay endpoint survives the degrade)", got)
	}
	if got := r.CanContain("EHR", "COMPOSITION"); got != Admissible {
		t.Errorf("CanContain(EHR, COMPOSITION) = %v, want Admissible (exact-name overlay route survives the degrade)", got)
	}
}

// TestDefaultOverlayEndpointsPinKnown is the reverse direction of the
// § Overlay edges pin-drift guard (the guard MUST fail when the family
// enumeration and the pin diverge — in either direction): every default
// overlay endpoint must be a class the pin ships, in canonical spelling. A
// stale or typo'd versionFamilies / ehrDirectPayloads entry would otherwise
// degrade to an exact-name endpoint — silently containable in the default
// relation, contradicting § Containable operands.
func TestDefaultOverlayEndpointsPinKnown(t *testing.T) {
	known := make(map[string]bool)
	for _, c := range rminfo.Default.KnownRMTypes() {
		known[c] = true
	}
	for _, e := range defaultOverlays() {
		for _, ep := range []string{e.From, e.To} {
			if !known[ep] {
				t.Errorf("default overlay edge %+v names endpoint %q, which the pin does not ship (stale or typo'd family entry)", e, ep)
			}
			if ep != canon(ep) {
				t.Errorf("default overlay endpoint %q is not in canonical spelling (want %q)", ep, canon(ep))
			}
		}
	}
}

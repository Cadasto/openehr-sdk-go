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

// TestLocatableDeclaredAttributesExcludedFromReachability pins all three arms
// of the REQ-160 § Reachability semantics housekeeping rule: an attribute whose
// DeclaredOn site is LOCATABLE *or* PATHABLE MUST NOT be a containment edge,
// and the rule MUST be declaration-site-derived rather than a hand-kept name
// list. Only a synthetic model can pin them: on the pinned BMM the sole real
// case is ELEMENT → CLUSTER via feeder_audit (which the acceptance table
// already covers, but which a six-name list would satisfy too), and the pin
// declares no PATHABLE property at all, so that arm cannot fire against it.
//
// Each excluded attribute here would open a route to a distinct containable
// class, and `value` is the positive control: it is declared on ELEMENT itself,
// so it must stay an edge. Without it the whole model could be inert and every
// Never below would pass vacuously.
func TestLocatableDeclaredAttributesExcludedFromReachability(t *testing.T) {
	lk := rminfo.New(map[string]rminfo.ClassMeta{
		"PATHABLE":  {Abstract: true},
		"LOCATABLE": {Abstract: true, Parents: []string{"PATHABLE"}},
		"ELEMENT": {
			Parents: []string{"LOCATABLE"},
			Attributes: map[string]rminfo.AttrMeta{
				"feeder_audit": {TypeName: "CLUSTER", DeclaredIn: "LOCATABLE"},
				"provenance":   {TypeName: "SECTION", DeclaredIn: "LOCATABLE"},
				"container":    {TypeName: "FOLDER", DeclaredIn: "PATHABLE"},
				"value":        {TypeName: "ITEM", DeclaredIn: "ELEMENT"},
			},
			AttrOrder: []string{"feeder_audit", "provenance", "container", "value"},
		},
		"CLUSTER": {Parents: []string{"LOCATABLE"}},
		"SECTION": {Parents: []string{"LOCATABLE"}},
		"FOLDER":  {Parents: []string{"LOCATABLE"}},
		"ITEM":    {Parents: []string{"LOCATABLE"}},
	})
	r := build(lk, nil)
	if got := r.Containable("ELEMENT"); got != Admissible {
		t.Fatalf("Containable(ELEMENT) = %v, want Admissible (synthetic model wiring broken)", got)
	}
	cases := []struct {
		descendant string
		via        string
		want       Verdict
		why        string
	}{
		{"ITEM", "value", Admissible, "declared on ELEMENT itself — ordinary content, must stay an edge (positive control)"},
		{"CLUSTER", "feeder_audit", Never, "declared on LOCATABLE — housekeeping, not a containment edge"},
		{"SECTION", "provenance", Never, "declared on LOCATABLE but not one of the pin's six names — the rule is declaration-site-derived, never a name list"},
		{"FOLDER", "container", Never, "declared on PATHABLE — the arm the pin cannot exercise, since it ships no PATHABLE property"},
	}
	for _, c := range cases {
		t.Run(c.descendant, func(t *testing.T) {
			if got := r.CanContain("ELEMENT", c.descendant); got != c.want {
				t.Errorf("CanContain(ELEMENT, %s) = %v, want %v — route runs through %s, %s", c.descendant, got, c.want, c.via, c.why)
			}
		})
	}
}

// lookupOnly hides rminfo.Default's optional capability interfaces: interface
// embedding promotes only the Lookup methods, so the wrapper satisfies neither
// rminfo.Hierarchy nor rminfo.AttributeLister.
type lookupOnly struct{ rminfo.Lookup }

// TestBuildWithoutCapabilitiesDegradesToOverlayOnly — a Lookup without the
// optional rminfo capability interfaces cannot answer the BMM half, and build
// degrades to an overlay-only relation instead of panicking: a name only the
// BMM knows answers UnknownClass (the fail-safe verdict), while an overlay
// endpoint keeps matching by exact folded name and stays containable.
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

// TestUnavoidableDoesNotPoisonTheMemo is the INSIDE half of the guarantee
// [TestUnavoidableLeavesTheVerdictMemoIntact] states from outside: the
// vertex-exclusion walk MUST NOT share the verdict memo, because excluding a
// vertex is a different graph. Only an internal test can see the difference
// before it becomes a wrong verdict, and only on a FRESH relation — the shared
// default has already been walked by every other test in the package.
//
// Two claims, one per direction:
//
//   - what IS memoized after the call is the whole-graph closure. If the
//     exclusion walk stored its own, narrower closure under the same key, EHR
//     would have lost COMPOSITION and everything below it;
//   - what is NOT memoized is mode 0. Unavoidable reads routes at their widest
//     on both halves of its question (its own doc comment), so it never asks
//     the ByReference-excluded closure. This is the ROUTE-EXISTENCE half's
//     share of that decision; the EXCLUSION half's share is witnessed from
//     outside the package by TestUnavoidableCountsAByReferenceBypass.
func TestUnavoidableDoesNotPoisonTheMemo(t *testing.T) {
	r := build(rminfo.Default, defaultOverlays())
	if !r.Unavoidable("EHR", "COMPOSITION", "OBSERVATION") {
		t.Fatal("premise gone: the relation no longer proves the REQ-164 witness, so this test proves nothing")
	}

	v, ok := r.memo[1].Load("EHR")
	if !ok {
		t.Fatal("premise gone: the route-existence half memoizes EHR's closure, and nothing did")
	}
	closure, _ := v.(map[string]bool)
	// Each of these is reachable from EHR in the WHOLE graph and unreachable
	// once COMPOSITION is excluded — exactly the nodes a poisoned memo drops.
	for _, n := range []string{"COMPOSITION", "OBSERVATION", "SECTION"} {
		if !closure[n] {
			t.Errorf("memo[1][EHR] does not contain %q; the exclusion walk wrote its own closure into the shared memo", n)
		}
	}

	if _, ok := r.memo[0].Load("EHR"); ok {
		t.Error("memo[0][EHR] was written; Unavoidable reads routes at their widest and must never ask the ByReference-excluded closure")
	}
}

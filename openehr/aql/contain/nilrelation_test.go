package contain_test

import (
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
)

// TestNilRelationAnswersAsTheDefault pins REQ-160 § Nil and zero relations on
// the verdict methods: a nil receiver answers what
// [contain.Default] answers, rather than panicking on a caller-constructible
// value the SDK documents as meaning "the default".
func TestNilRelationAnswersAsTheDefault(t *testing.T) {
	var nilRel *contain.TypeRelation
	def := contain.Default()

	t.Run("Containable", func(t *testing.T) {
		for _, rmType := range []string{"COMPOSITION", "EHR", "DV_TEXT", "NOT_A_CLASS", ""} {
			if got, want := nilRel.Containable(rmType), def.Containable(rmType); got != want {
				t.Errorf("nil.Containable(%q) = %v, default says %v", rmType, got, want)
			}
		}
	})

	t.Run("CanContain", func(t *testing.T) {
		pairs := [][2]string{
			{"EHR", "COMPOSITION"},   // Admissible
			{"ELEMENT", "CLUSTER"},   // Never
			{"EHR", "VERSIONED_ANY"}, // UnknownClass
			{"FOLDER", "COMPOSITION"},
		}
		for _, p := range pairs {
			if got, want := nilRel.CanContain(p[0], p[1]), def.CanContain(p[0], p[1]); got != want {
				t.Errorf("nil.CanContain(%q, %q) = %v, default says %v", p[0], p[1], got, want)
			}
		}
	})

	t.Run("ArchetypeMatches", func(t *testing.T) {
		cases := [][2]string{
			{"COMPOSITION", "openEHR-EHR-COMPOSITION.encounter.v1"}, // Admissible
			{"OBSERVATION", "openEHR-EHR-COMPOSITION.encounter.v1"}, // Never
			{"COMPOSITION", "not-an-hrid"},                          // UnknownClass
		}
		for _, c := range cases {
			if got, want := nilRel.ArchetypeMatches(c[0], c[1]), def.ArchetypeMatches(c[0], c[1]); got != want {
				t.Errorf("nil.ArchetypeMatches(%q, %q) = %v, default says %v", c[0], c[1], got, want)
			}
		}
	})
}

// TestNilRelationIsNotTheZeroRelation guards the distinction REQ-160 § Nil and
// zero relations draws: nil selects the default, whereas the
// zero TypeRelation is a real relation that knows no classes. If a future
// change made a nil receiver behave like the zero value the panic would be
// gone but the answers would be silently wrong, which is worse.
func TestNilRelationIsNotTheZeroRelation(t *testing.T) {
	var nilRel *contain.TypeRelation
	zero := &contain.TypeRelation{}

	if got := nilRel.Containable("COMPOSITION"); got != contain.Admissible {
		t.Errorf("nil.Containable(COMPOSITION) = %v, want Admissible (the default's answer)", got)
	}
	if got := zero.Containable("COMPOSITION"); got != contain.UnknownClass {
		t.Errorf("zero.Containable(COMPOSITION) = %v, want UnknownClass (knows no classes)", got)
	}
}

// TestNilRelationExtendsTheDefault covers the shape the nil convention invites
// — a consumer conditionally extending a relation it may not be holding:
//
//	var rel *contain.TypeRelation
//	if deploymentHasDemographics { rel = rel.WithOverlay(edge) }
//
// The result must carry the edge AND the default's own verdicts, and must not
// disturb the shared default.
func TestNilRelationExtendsTheDefault(t *testing.T) {
	var rel *contain.TypeRelation
	edge := contain.Edge{From: "EHR", To: "VERSIONED_PARTY"}

	if got := contain.Default().CanContain("EHR", "VERSIONED_PARTY"); got == contain.Admissible {
		t.Fatal("premise gone: the default already admits EHR CONTAINS VERSIONED_PARTY, so the overlay proves nothing")
	}

	got := rel.WithOverlay(edge)
	if got == nil {
		t.Fatal("nil.WithOverlay(edge) returned nil; WithOverlay never returns nil")
	}
	if v := got.CanContain("EHR", "VERSIONED_PARTY"); v != contain.Admissible {
		t.Errorf("extended relation: EHR CONTAINS VERSIONED_PARTY = %v, want Admissible", v)
	}
	if v := got.CanContain("EHR", "COMPOSITION"); v != contain.Admissible {
		t.Errorf("extended relation lost a default verdict: EHR CONTAINS COMPOSITION = %v, want Admissible", v)
	}
	if v := contain.Default().CanContain("EHR", "VERSIONED_PARTY"); v == contain.Admissible {
		t.Error("extending a nil relation leaked the overlay edge into the shared default")
	}

	// No edges at all still yields a usable relation, not the nil it started as.
	if bare := rel.WithOverlay(); bare == nil || bare.Containable("COMPOSITION") != contain.Admissible {
		t.Error("nil.WithOverlay() must yield the default relation, not nil")
	}
}

// TestEveryExportedMethodToleratesANilReceiver is the tripwire for the axis
// rather than for the five methods that exist today: it reflects over the whole
// exported method set of *TypeRelation and calls each on a nil receiver with
// zero-value arguments. A method added later that dereferences the receiver
// without going through orDefault fails here on the day it lands, which is the
// only way a REQ-025 § No panics guarantee stays true as the surface grows.
//
// The floor tracks the surface deliberately: raise it whenever a method is
// added, so the sweep keeps proving it looked at everything rather than
// silently passing over a shrunken method set.
func TestEveryExportedMethodToleratesANilReceiver(t *testing.T) {
	var nilRel *contain.TypeRelation
	rt := reflect.TypeOf(nilRel)

	if n := rt.NumMethod(); n < 5 {
		t.Fatalf("reflected %d exported methods on *TypeRelation; expected at least the 5 known ones — the sweep is not looking at what it thinks it is", n)
	}

	for m := range rt.Methods() {
		t.Run(m.Name, func(t *testing.T) {
			// m.Func takes the receiver as argument 0. For a variadic method
			// pass ONE zero element rather than an empty list: an empty list is
			// the case an early `len(edges) == 0` return can answer without ever
			// touching the receiver, so it would not exercise the guard.
			n := m.Type.NumIn()
			args := make([]reflect.Value, 0, n)
			args = append(args, reflect.ValueOf(nilRel))
			for j := 1; j < n; j++ {
				at := m.Type.In(j)
				if m.Type.IsVariadic() && j == n-1 {
					at = at.Elem()
				}
				args = append(args, reflect.Zero(at))
			}

			out := m.Func.Call(args) // panics here = the finding

			// A method handing back a relation must hand back a usable one.
			for _, o := range out {
				if o.Type() == rt && o.IsNil() {
					t.Errorf("%s returned a nil *TypeRelation", m.Name)
				}
			}
		})
	}
}

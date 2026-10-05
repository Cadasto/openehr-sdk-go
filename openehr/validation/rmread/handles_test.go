package rmread

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmnames"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// handledTypes pins the RM types Handles must report as modelled — the same
// set ReadSingle/ReadMultiple dispatch. It is a golden checklist: when a new
// readXxxSingle/readXxxMultiple reader is added, its type MUST be added here
// AND to Handles. Removing a type from Handles without removing it here trips
// TestHandles_ModelledTypes; the reverse (a reader added but omitted from
// Handles) is caught by a reviewer noticing this list is stale.
//
// Value form only — Handles covers `*rm.T` and `rm.T` identically, and the
// pointer form is spot-checked in TestHandles_PointerForm.
var handledTypes = []any{
	// ENTRY + structural
	rm.Composition{},
	rm.Observation{},
	rm.Evaluation{},
	rm.Instruction{},
	rm.Action{},
	rm.IsmTransition{},
	rm.AdminEntry{},
	rm.GenericEntry{},
	rm.Section{},
	rm.Activity{},
	rm.EventContext{},
	rm.History[rm.ItemStructure]{},
	rm.PointEvent[rm.ItemStructure]{},
	rm.IntervalEvent[rm.ItemStructure]{},
	rm.ItemTree{},
	rm.ItemList{},
	rm.ItemSingle{},
	rm.ItemTable{},
	rm.Cluster{},
	rm.Element{},
	// data values
	rm.DVText{},
	rm.DVCodedText{},
	rm.TermMapping{},
	rm.CodePhrase{},
	rm.DVDate{},
	rm.DVTime{},
	rm.DVDateTime{},
	rm.DVDuration{},
	rm.DVBoolean{},
	rm.DVIdentifier{},
	rm.DVMultimedia{},
	rm.DVCount{},
	rm.DVQuantity{},
	rm.DVProportion{},
	rm.DVURI{},
	rm.DVEHRURI{},
	rm.DVParsable{},
	rm.DVOrdinal{},
	rm.DVScale{},
	rm.ReferenceRange[rm.DVOrdered]{},
	rm.ReferenceRange[rm.DVCount]{},
	rm.ReferenceRange[rm.DVQuantity]{},
	rm.ReferenceRange[rm.DVProportion]{},
	// intervals
	rm.DVInterval[rm.DVQuantity]{},
	rm.DVInterval[rm.DVCount]{},
	rm.DVInterval[rm.DVDateTime]{},
	rm.DVInterval[rm.DVDate]{},
	rm.DVInterval[rm.DVTime]{},
	rm.DVInterval[rm.DVProportion]{},
	rm.DVInterval[rm.DVDuration]{},
	rm.DVInterval[rm.DVOrdinal]{},
	rm.DVInterval[rm.DVScale]{},
	rm.DVInterval[rm.DVOrdered]{},
	// demographic
	rm.Person{},
	rm.Organisation{},
	rm.Group{},
	rm.Agent{},
	rm.Role{},
	rm.Address{},
	rm.Contact{},
	rm.PartyIdentity{},
	rm.PartyRelationship{},
	rm.Capability{},
	rm.PartyRef{},
	// EHR-IM roots
	rm.Folder{},
	rm.EHRStatus{},
	rm.EHRAccess{},
	// archetyping
	rm.Archetyped{},
}

func TestHandles_ModelledTypes(t *testing.T) {
	if got, want := len(handledTypes), 68; got != want {
		t.Errorf("handledTypes has %d entries, want %d — keep it in sync with Handles/ReadSingle", got, want)
	}
	for _, v := range handledTypes {
		if !Handles(v) {
			t.Errorf("Handles(%T) = false, want true (modelled by ReadSingle/ReadMultiple)", v)
		}
	}
}

func TestHandles_PointerForm(t *testing.T) {
	// Handles must accept the pointer form too — readers are reached via both,
	// and the walker passes whatever rmread/the caller boxes.
	ptrs := []any{
		&rm.Composition{}, &rm.DVQuantity{}, &rm.DVInterval[rm.DVQuantity]{},
		&rm.Folder{}, &rm.EHRStatus{}, &rm.Cluster{}, &rm.Archetyped{},
		&rm.DVOrdinal{}, &rm.DVScale{}, &rm.ReferenceRange[rm.DVOrdered]{}, &rm.EHRAccess{},
	}
	for _, v := range ptrs {
		if !Handles(v) {
			t.Errorf("Handles(%T) = false, want true (pointer form)", v)
		}
	}
}

func TestHandles_Unmodelled(t *testing.T) {
	// Types rmread does NOT model must report false so the floor treats them
	// as opaque leaves (validated by their own invariant evaluators) rather
	// than reading their members back as absent and fabricating `required`.
	// Includes a flattened scalar, an OBJECT_REF, and a PARTY proxy concrete
	// (recognised by rmTypeInfo but not modelled here).
	unmodelled := []any{
		"a flattened string",
		42,
		rm.ObjectRef{},
		&rm.ObjectRef{},
		rm.PartySelf{},
		nil,
	}
	for _, v := range unmodelled {
		if Handles(v) {
			t.Errorf("Handles(%T) = true, want false (not modelled by rmread)", v)
		}
	}
}

// typedIntervals lists every typed DV_INTERVAL instantiation, in value and
// pointer form. It is written out by hand because a DVInterval[T] value
// cannot be built from a registry name without reflection;
// TestTypedIntervalReaderParity ties it to the registry and to rmnames.
var typedIntervals = []any{
	rm.DVInterval[rm.DVCount]{},
	&rm.DVInterval[rm.DVCount]{},
	rm.DVInterval[rm.DVDate]{},
	&rm.DVInterval[rm.DVDate]{},
	rm.DVInterval[rm.DVDateTime]{},
	&rm.DVInterval[rm.DVDateTime]{},
	rm.DVInterval[rm.DVDuration]{},
	&rm.DVInterval[rm.DVDuration]{},
	rm.DVInterval[rm.DVOrdinal]{},
	&rm.DVInterval[rm.DVOrdinal]{},
	rm.DVInterval[rm.DVProportion]{},
	&rm.DVInterval[rm.DVProportion]{},
	rm.DVInterval[rm.DVQuantity]{},
	&rm.DVInterval[rm.DVQuantity]{},
	rm.DVInterval[rm.DVScale]{},
	&rm.DVInterval[rm.DVScale]{},
	rm.DVInterval[rm.DVTime]{},
	&rm.DVInterval[rm.DVTime]{},
}

// TestTypedIntervalReaderParity checks that rmread reads every typed
// DV_INTERVAL instantiation (REQ-112). The RM floor walks into a typed
// interval only when Handles accepts it and ReadSingle returns its bounds,
// so a missing instantiation leaves that interval's bounds unchecked. Each
// case must be one rmnames names; the cases must cover, in value and
// pointer form, every DV_ORDERED descendant the live registry derives; and
// each case must be accepted by Handles with both bounds readable. A new
// descendant fails here until the list and the readers both gain it.
func TestTypedIntervalReaderParity(t *testing.T) {
	var fromRegistry []string
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		if _, ok := ctor().(rm.DVOrdered); ok {
			fromRegistry = append(fromRegistry, "DV_INTERVAL<"+name+">")
		}
	}
	if len(fromRegistry) == 0 {
		t.Fatal("registry yields no DVOrdered implementers — registrations missing?")
	}
	slices.Sort(fromRegistry)

	// forms records, per parameterised name, which of the two Go forms the
	// case list holds.
	type listed struct{ value, pointer bool }
	forms := map[string]listed{}
	for _, v := range typedIntervals {
		name, ok := rmnames.TypedIntervalName(v)
		if !ok {
			t.Errorf("rmnames.TypedIntervalName(%T) = (_, false); the case list holds a type rmnames does not name", v)
			continue
		}
		f := forms[name]
		if strings.HasPrefix(fmt.Sprintf("%T", v), "*") {
			f.pointer = true
		} else {
			f.value = true
		}
		forms[name] = f

		if !Handles(v) {
			t.Errorf("Handles(%T) = false, want true: the floor stops at this interval", v)
		}
		bound := strings.TrimSuffix(strings.TrimPrefix(name, "DV_INTERVAL<"), ">")
		for _, attr := range []string{"lower", "upper"} {
			got, ok := ReadSingle(v, "DV_INTERVAL", attr)
			gotName, _ := rm.RMTypeName(got)
			if !ok || gotName != bound {
				t.Errorf("ReadSingle(%T, %q) = (%T, %v), want a %s bound and true", v, attr, got, ok, bound)
			}
		}
	}

	covered := slices.Sorted(maps.Keys(forms))
	if !slices.Equal(fromRegistry, covered) {
		t.Errorf("typed DV_INTERVAL drift:\n  registry-derived: %v\n  rmread cases:     %v", fromRegistry, covered)
	}
	for _, name := range covered {
		if f := forms[name]; !f.value || !f.pointer {
			t.Errorf("%s: value form listed = %v, pointer form listed = %v; want both", name, f.value, f.pointer)
		}
	}
}

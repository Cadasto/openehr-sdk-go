package validation_test

// registry_roots_test.go: REQ-171 Acceptance — every class the registry
// knows is accepted as a root by Validate(root, nil): the RM rules apply to
// it, and none draws rm_type_unknown.

import (
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// registeredRMClasses is the number of RM classes the registry knows: the
// concrete classes of the pinned RM BMM the codec decodes.
const registeredRMClasses = 110

// rmPackage is the import path of the RM types.
var rmPackage = reflect.TypeFor[rm.Composition]().PkgPath()

// REQ-171 Acceptance — the zero value of every RM class the registry the
// codec decodes with knows, built by its registered constructor, is
// accepted as a root by Validate(root, nil): no rm_type_unknown. The
// registry also holds classes of other models registered by packages this
// test links, such as the archetype model; those are not RM values and are
// counted apart, so no RM class is skipped without the count failing.
func TestValidator_REQ171_EveryRegisteredClassIsARoot(t *testing.T) {
	t.Parallel()
	v := validation.New()
	names := typereg.Default.Names()
	if len(names) == 0 {
		t.Fatal("typereg.Default knows no class; the check has gone blind")
	}
	checked, other := 0, 0
	for _, name := range names {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok {
			t.Fatalf("typereg.Default lists %s but has no constructor for it", name)
		}
		root := ctor()
		typ := reflect.TypeOf(root)
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ.PkgPath() != rmPackage {
			other++
			continue
		}
		checked++
		r := v.Validate(root, nil)
		for _, i := range r.Issues {
			if i.Code == "rm_type_unknown" {
				t.Errorf("New().Validate(zero %s, nil) reports rm_type_unknown at %q: every class the registry knows is a root (REQ-171)", name, i.Path)
			}
		}
	}
	t.Logf("checked %d RM classes; %d classes of other models in the registry are not RM values", checked, other)
	if checked != registeredRMClasses {
		t.Errorf("checked %d RM classes of the %d typereg.Default lists (%d of other models), want %d: a class was skipped, or the registry changed and this count with it", checked, len(names), other, registeredRMClasses)
	}
}

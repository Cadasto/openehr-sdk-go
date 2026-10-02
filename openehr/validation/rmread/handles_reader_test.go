package rmread

import (
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// probeValue returns v ready to probe. A zero REFERENCE_RANGE answers every
// attribute with (nil, false), as a type with no reader does, so it gets a
// meaning to read.
func probeValue(v any) any {
	switch r := v.(type) {
	case rm.ReferenceRange[rm.DVOrdered]:
		return withMeaning(r)
	case rm.ReferenceRange[rm.DVCount]:
		return withMeaning(r)
	case rm.ReferenceRange[rm.DVQuantity]:
		return withMeaning(r)
	case rm.ReferenceRange[rm.DVProportion]:
		return withMeaning(r)
	}
	return v
}

func withMeaning[T rm.DVOrdered](r rm.ReferenceRange[T]) rm.ReferenceRange[T] {
	r.Meaning = &rm.DVText{Value: "meaning"}
	return r
}

// TestHandles_EveryTypeHasAReadArm (REQ-112, REQ-107) checks that each type Handles accepts
// has a reader arm. A type with a Handles case but no arm in ReadSingle and
// ReadMultiple reads every attribute as absent, so the RM floor would report
// each of its RM-mandatory attributes as required. The probe takes every
// attribute name from the type's own json tags, so no per-type list needs
// keeping: a zero value answers a served attribute with a non-nil value
// (absent, but typed) or a multiple with ok, and an unserved one with
// (nil, false). The few types whose zero value answers nil throughout get
// one populated attribute first (probeValue).
func TestHandles_EveryTypeHasAReadArm(t *testing.T) {
	for _, v := range handledTypes {
		typ := reflect.TypeOf(v)
		v = probeValue(v)
		if typ.Kind() != reflect.Struct {
			t.Fatalf("handledTypes holds %v, want a struct value", typ)
		}
		served := false
		for _, f := range reflect.VisibleFields(typ) {
			attr, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if attr == "" || attr == "-" || attr == "archetype_details" {
				continue
			}
			if got, _ := ReadSingle(v, "", attr); got != nil {
				served = true
			}
			if _, ok := ReadMultiple(v, "", attr); ok {
				served = true
			}
		}
		if !served {
			t.Errorf("%T: Handles accepts it but ReadSingle and ReadMultiple serve none of its attributes", v)
		}
	}
}

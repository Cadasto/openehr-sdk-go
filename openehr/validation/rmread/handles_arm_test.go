package rmread

import (
	"reflect"
	"strings"
	"testing"
)

// TestHandles_EveryTypeHasAReadArm checks that each type Handles accepts
// has a reader arm. A type with a Handles case but no arm in ReadSingle and
// ReadMultiple reads every attribute as absent, so the RM floor would report
// each of its RM-mandatory attributes as required. The probe takes every
// attribute name from the type's own json tags, so no per-type list needs
// keeping: a zero value answers a served attribute with a non-nil value
// (absent, but typed) or a multiple with ok, and an unserved one with
// (nil, false).
func TestHandles_EveryTypeHasAReadArm(t *testing.T) {
	for _, v := range handledTypes {
		typ := reflect.TypeOf(v)
		if typ.Kind() != reflect.Struct {
			t.Fatalf("handledTypes holds %v, want a struct value", typ)
		}
		served := false
		for f := range typ.Fields() {
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

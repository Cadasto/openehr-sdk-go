package canjson_test

import (
	jsonv2 "encoding/json/v2"
	"reflect"
	"strings"
	"testing"

	_ "github.com/cadasto/openehr-sdk-go/openehr/aom/aom14" // register the AOM 1.4 types
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestRegisteredTypeCensus is ruling R19's item (c): the runtime control that
// contains the two-shape risk. A future BMM bump that adds an embedding edge
// silently moves a class between the zero-copy alias and the flat wire struct,
// and the flat-vs-alias mistake is a mislabelled `_type` with no compile error
// (the promotion trap the hybrid shape exists to dodge). The generator-level
// render tests check the emitted source; this checks the built tree.
//
// For every registered library type it takes the value-in-interface shape
// (`reflect.ValueOf(ctor()).Elem().Interface()` held in an `any`, the shape the
// value-receiver decision turns on) and marshals it twice: through
// canjson.Marshal (now on encoding/json/v2) and through bare
// encoding/json/v2. Each output MUST lead with `_type` equal to the registered
// name, and MUST decode back into a fresh instance without error.
//
// Can-fail control (recorded in the report): reclassify one of the 14 flat
// classes into the alias shape (a one-line change to
// embedsMarshalerBearingConcrete) and regenerate: DVCodedText then marshals as
// `{"_type":"DV_TEXT",…}` and this census reports the mismatch by name.
func TestRegisteredTypeCensus(t *testing.T) {
	const libraryPrefix = "github.com/cadasto/openehr-sdk-go/openehr/"
	names := typereg.Default.Names()
	if len(names) < 100 {
		t.Fatalf("registry holds %d types; the census expects the full RM + AOM 1.4 inventory", len(names))
	}

	checked := 0
	for _, name := range names {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok {
			t.Fatalf("Lookup(%q) = false for a name Names() returned", name)
		}
		rt := reflect.TypeOf(ctor())
		for rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		if !strings.HasPrefix(rt.PkgPath(), libraryPrefix) {
			continue // a test fixture registered elsewhere; not a shipped type
		}
		checked++

		t.Run(name, func(t *testing.T) {
			seed := ctor() // *Concrete
			// TERM_MAPPING.match is a single-character rendition; its zero value
			// is the empty string, which rm.Character refuses (a validity rule,
			// not a codec fault). Seed a valid one-character match so the census
			// exercises the codec rather than the value rule.
			if tm, ok := seed.(*rm.TermMapping); ok {
				tm.Match = rm.Character("=")
			}
			held := reflect.ValueOf(seed).Elem().Interface() // Concrete value in an interface

			outputs := map[string][]byte{}
			if b, err := canjson.Marshal(held); err != nil {
				t.Fatalf("canjson.Marshal(%s value-in-interface) error: %v", name, err)
			} else {
				outputs["canjson"] = b
			}
			if b, err := jsonv2.Marshal(held); err != nil {
				t.Fatalf("json/v2.Marshal(%s value-in-interface) error: %v", name, err)
			} else {
				outputs["v2"] = b
			}

			wantPrefix := `{"_type":"` + name + `"`
			for entry, out := range outputs {
				if !strings.HasPrefix(string(out), wantPrefix) {
					t.Errorf("%s (%s): first member is not the registered `_type`\n got  %s\n want prefix %s\n(a flat/alias-shape mistake mislabels `_type` here)", name, entry, out, wantPrefix)
				}
				if err := canjson.Unmarshal(out, ctor()); err != nil {
					t.Errorf("%s (%s): re-decode failed: %v", name, entry, err)
				}
			}
		})
	}
	if checked < 100 {
		t.Errorf("census covered only %d library types; expected the full RM + AOM 1.4 inventory", checked)
	}
}

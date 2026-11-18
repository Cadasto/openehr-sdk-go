package validation_test

// absent_part_test.go: REQ-102 § Primitive constraints, REQ-112 § Value-typed
// mandatory presence and REQ-171 § One finding per defect. A part of a value
// that a primitive constraint reads, and that the RM rules report absent, is
// reported by the RM's `required` alone: the constraint does not judge the
// zero value the decode leaves in its place.

import (
	json "encoding/json/v2"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// quantityMagnitudes returns the JSON Pointer of the magnitude of every
// DV_QUANTITY in doc, in document order.
func quantityMagnitudes(t *testing.T, doc []byte) []string {
	t.Helper()
	var tree any
	if err := json.Unmarshal(doc, &tree); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	var out []string
	var visit func(v any, pointer string)
	visit = func(v any, pointer string) {
		switch x := v.(type) {
		case map[string]any:
			if _, ok := x["magnitude"]; ok && x["_type"] == "DV_QUANTITY" {
				out = append(out, pointer+"/magnitude")
			}
			for _, k := range slices.Sorted(maps.Keys(x)) {
				visit(x[k], pointer+"/"+k)
			}
		case []any:
			for i, item := range x {
				visit(item, pointer+"/"+strconv.Itoa(i))
			}
		}
	}
	visit(tree, "")
	return out
}

// REQ-102, REQ-112, REQ-171 — the corpus composition whose quantities a
// C_DV_QUANTITY holds to 5 to 10 cm, with the magnitude of every DV_QUANTITY
// left out or sent as null: the decode reports each magnitude missing, so the
// one finding for each is the RM's `required` at the magnitude, and the
// constraint does not judge the zero the decode leaves there. Every other
// finding is the fixture's own.
func TestValidateJSON_REQ102_REQ112_REQ171_CorpusAbsentMagnitudeIsRequiredAlone(t *testing.T) {
	t.Parallel()
	const id = "Test_dv_quantity_property_units_constrained_with_magnitude_range.v0"
	c := mustCompile(t, id)
	doc, err := os.ReadFile(fixtures.CompositionJSON(id))
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	// The quantities are the ELEMENT at0043 of each of the three events.
	pointers := quantityMagnitudes(t, doc)
	if len(pointers) != 3 {
		t.Fatalf("%s holds %d DV_QUANTITY magnitudes %q, want 3 (fixture drift)", id, len(pointers), pointers)
	}
	var want []pinnedIssue
	for i := range pointers {
		want = append(want, rmRequired("/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events["+strconv.Itoa(i+1)+"]/data/items[at0043]/value/magnitude"))
	}
	baseline := pinnedIssues(validation.ValidateJSON(doc, new(rm.Composition), c).Issues)
	nulled := make(map[string]string, len(pointers))
	for _, p := range pointers {
		nulled[p] = "null"
	}
	for name, edits := range map[string]map[string]string{
		"left out": dropped(pointers...),
		"null":     nulled,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := validation.ValidateJSON(rewriteJSON(t, string(doc), edits), new(rm.Composition), c)
			added, removed := diffPinned(baseline, pinnedIssues(r.Issues))
			if !slices.Equal(added, want) || len(removed) != 0 {
				t.Errorf("ValidateJSON(%s, magnitudes %s) against the fixture's own result:\n  added   %+v\n  removed %+v\nwant added %+v and nothing removed", id, name, added, removed, want)
			}
			for _, w := range want {
				quantity := strings.TrimSuffix(w.Path, "/magnitude")
				if got := atOrBelow(r, quantity); !slices.Equal(got, []string{"required " + w.Path}) {
					t.Errorf("ValidateJSON(%s, magnitudes %s) reports %q at or below %s, want the `required` at its magnitude alone", id, name, got, quantity)
				}
			}
		})
	}
}

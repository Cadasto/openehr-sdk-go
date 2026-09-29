package validation_test

// interval_open_side_test.go: the template-driven walker on an interval
// side whose `*_unbounded` flag is set. The walker reads interval bounds
// through the shared rmread readers, so it follows the same open-side rule
// as the RM floor: an empty bound on an open side is absent, and a real
// bound beside the flag is checked against the OPT like any bound.

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

const intervalValuePath = "/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0046]/value"

type pathCode struct{ path, code string }

func pathCodes(issues []validation.Issue) []pathCode {
	var out []pathCode
	for _, i := range issues {
		out = append(out, pathCode{i.Path, i.Code})
	}
	return out
}

// TestValidateComposition_DecodedOpenIntervalSide covers a decoded
// half-open DV_INTERVAL (REQ-102): every interval in the vendored fixture
// has its `upper` dropped and `upper_unbounded` set, which the OPT allows
// (`upper` is 0..1). The open side carries no bound, so the walker reports
// no issue there; it once read the missing bound as a present nil and
// reported `rm_type_mismatch` against "<nil>".
func TestValidateComposition_DecodedOpenIntervalSide(t *testing.T) {
	for _, id := range []string{
		"Test_dv_interval_dv_count_open_constraint.v0",
		"Test_dv_interval_dv_quantity_open_constraint.v0",
	} {
		t.Run(id, func(t *testing.T) {
			c := mustCompile(t, id)
			raw, err := os.ReadFile(fixtures.CompositionJSON(id))
			if err != nil {
				t.Fatal(err)
			}
			edited, n := openUpperSides(t, raw)
			if n == 0 {
				t.Fatal("the fixture holds no DV_INTERVAL to open")
			}
			var comp rm.Composition
			if err := canjson.Unmarshal(edited, &comp); err != nil {
				t.Fatalf("decode composition: %v", err)
			}
			r := validation.ValidateComposition(&comp, c)
			if got := pathCodes(r.Issues); len(got) != 0 {
				t.Errorf("ValidateComposition(%s, %d intervals opened) issues = %v, want none\nfull issues: %+v", id, n, got, r.Issues)
			}
		})
	}
}

// TestValidateComposition_TypedOpenIntervalSide covers a typed half-open
// DV_INTERVAL<DV_QUANTITY> built in memory (REQ-102). Its open upper side
// holds the zero DV_QUANTITY a concrete-typed Go interval cannot avoid;
// the walker reads that as no bound and reports nothing, where it once
// checked the zero value's empty units against the OPT's unit list and
// reported `primitive_unit_unknown`. The bounded lower side is in range.
func TestValidateComposition_TypedOpenIntervalSide(t *testing.T) {
	const id = "Test_dv_interval_dv_quantity_lower_upper_constraint.v0"
	c := mustCompile(t, id)
	raw, err := os.ReadFile(fixtures.CompositionJSON(id))
	if err != nil {
		t.Fatal(err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(raw, &comp); err != nil {
		t.Fatalf("decode composition: %v", err)
	}
	n := setIntervalValues(t, &comp, func() rm.DataValue {
		return &rm.DVInterval[rm.DVQuantity]{
			Lower: rm.DVQuantity{Magnitude: 50, Units: "Cel"}, LowerIncluded: true,
			UpperUnbounded: true,
		}
	})
	if n == 0 {
		t.Fatal("the fixture holds no at0046 ELEMENT to set")
	}
	r := validation.ValidateComposition(&comp, c)
	if got := pathCodes(r.Issues); len(got) != 0 {
		t.Errorf("ValidateComposition(%s, typed half-open interval) issues = %v, want none\nfull issues: %+v", id, got, r.Issues)
	}
}

// TestValidateComposition_BuiltBoundBesideOpenFlag covers a bound the
// composition builder sets on an interval it seeded with both sides open
// (REQ-102). The builder leaves `lower_unbounded` set, so the real lower
// bound stands beside its own flag. It is not empty, so the walker checks
// it against the OPT like any bound: a magnitude of 500 breaks the OPT's
// 0..100 range.
func TestValidateComposition_BuiltBoundBesideOpenFlag(t *testing.T) {
	c := mustCompile(t, "Test_dv_interval_dv_count_lower_upper_constraint.v0")
	name := "Test Composer"
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(&rm.PartyIdentified{Name: &name}),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.Set(intervalValuePath+"/lower", &rm.DVCount{Magnitude: 500}); err != nil {
		t.Fatalf("Set lower: %v", err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	r := validation.ValidateComposition(comp, c)
	want := []pathCode{{intervalValuePath + "/lower/magnitude", "primitive_out_of_range"}}
	if got := pathCodes(r.Issues); !slices.Equal(got, want) {
		t.Errorf("ValidateComposition(built, lower 500) issues = %v, want %v\nfull issues: %+v", got, want, r.Issues)
	}
}

// openUpperSides drops `upper` from every DV_INTERVAL object in raw and
// marks that side open (upper_unbounded true, upper_included false, as
// BASE Interval requires of an open side). It reports how many it edited.
func openUpperSides(t *testing.T, raw []byte) ([]byte, int) {
	t.Helper()
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	n := 0
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if x["_type"] == "DV_INTERVAL" {
				delete(x, "upper")
				x["upper_unbounded"] = true
				x["upper_included"] = false
				n++
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("re-encode fixture: %v", err)
	}
	return out, n
}

// setIntervalValues replaces the value of every at0046 ELEMENT under the
// fixture's single OBSERVATION with a fresh value from mk. It reports how
// many it set.
func setIntervalValues(t *testing.T, comp *rm.Composition, mk func() rm.DataValue) int {
	t.Helper()
	obs, ok := comp.Content[0].(*rm.Observation)
	if !ok {
		t.Fatalf("content[0] is %T, want *rm.Observation", comp.Content[0])
	}
	n := 0
	for _, ev := range obs.Data.Events {
		var data rm.ItemStructure
		switch e := ev.(type) {
		case *rm.PointEvent[rm.ItemStructure]:
			data = e.Data
		case *rm.IntervalEvent[rm.ItemStructure]:
			data = e.Data
		default:
			t.Fatalf("event is %T, want a point or interval event", ev)
		}
		tree, ok := data.(*rm.ItemTree)
		if !ok {
			t.Fatalf("event data is %T, want *rm.ItemTree", data)
		}
		for _, it := range tree.Items {
			if el, ok := it.(*rm.Element); ok && el.ArchetypeNodeID == "at0046" {
				el.Value = mk()
				n++
			}
		}
	}
	return n
}

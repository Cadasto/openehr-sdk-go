package templatecompile

// parity_fixtures_test.go: REQ-111, over the parity fixtures, OPTs the SDK
// writes in testdata/parity for each value the compile step tolerates and
// each default-value placement it names: the default values placed and the
// strict mode's refusals. TestCompiledGolden holds the compiled tree of each
// fixture.

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// parityFixtureDir holds the parity fixtures.
const parityFixtureDir = "testdata/parity"

// TestREQ111_ParityFixtures reads each parity fixture with the model's
// reader and compiles it, both in their default mode: the compile step places
// the default values the fixture names. In its strict mode the compile step
// refuses a fixture of a tolerated value, and a default value that addresses
// no node or that it cannot hold as canonical JSON. The compiled tree of each
// fixture is TestCompiledGolden's.
func TestREQ111_ParityFixtures(t *testing.T) {
	const items = "/context/other_context/items"
	fixtures := []struct {
		file string
		// compileStrictRefuses is true when the compile step refuses it
		// with Options.Strict.
		compileStrictRefuses bool
		// defaults are the default values the compile step places, each
		// as the AQL path and RM type of the node that carries it and the
		// RM type of the value.
		defaults map[string]string
	}{
		{file: "constraint_ref_no_reference.opt", compileStrictRefuses: true},
		{file: "constraint_ref_blank_reference.opt", compileStrictRefuses: true},
		{file: "date_timezone_mandatory.opt", compileStrictRefuses: true},
		{file: "temporal_bound_not_iso8601.opt", compileStrictRefuses: true},
		{file: "duration_bounds_not_iso8601.opt", compileStrictRefuses: true},
		{file: "primitive_object_no_item.opt", compileStrictRefuses: true},
		// Its decimal and NaN integer bounds are read in every mode, so
		// the strict mode does not refuse it.
		{file: "decimal_and_nan_bounds.opt"},
		{
			file: "default_value_placement.opt",
			defaults: map[string]string{
				items + "[at0003]/value DV_TEXT":                              "DV_TEXT",
				items + "[at0004]/value DV_TEXT":                              "DV_TEXT",
				items + "[at0002,'Diastolic']/value DV_QUANTITY":              "DV_QUANTITY",
				items + "[at0002,'Pulse pressure']/value DV_QUANTITY":         "DV_QUANTITY",
				items + "[at0002,'Mean arterial pressure']/value DV_QUANTITY": "DV_QUANTITY",
				items + "[at0005]/value DV_PROPORTION":                        "DV_PROPORTION",
			},
		},
		{
			file:                 "default_value_no_node.opt",
			compileStrictRefuses: true,
			defaults:             map[string]string{items + "[at0003]/value DV_TEXT": "DV_TEXT"},
		},
		{
			file:                 "default_value_not_canonical_json.opt",
			compileStrictRefuses: true,
			defaults:             map[string]string{items + "[at0003]/value DV_TEXT": "DV_TEXT"},
		},
		{
			file: "default_value_generic_child.opt",
			defaults: map[string]string{
				items + "[at0002]/value DV_INTERVAL<DV_COUNT>": "DV_INTERVAL",
				items + "[at0003]/value DV_INTERVAL<DV_COUNT>": "DV_INTERVAL",
				items + "[at0004]/value DV_INTERVAL<DV_COUNT>": "DV_INTERVAL",
			},
		},
	}
	files, err := filepath.Glob(filepath.Join(parityFixtureDir, "*.opt"))
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, f := range fixtures {
		listed = append(listed, filepath.Join(parityFixtureDir, f.file))
	}
	slices.Sort(listed)
	if !slices.Equal(files, listed) {
		t.Fatalf("%s holds %q; the test lists %q", parityFixtureDir, files, listed)
	}

	for _, f := range fixtures {
		t.Run(f.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(parityFixtureDir, f.file))
			if err != nil {
				t.Fatal(err)
			}
			model, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("aom14.ReadOperationalTemplate = %v", err)
			}

			c, err := Compile(model)
			if err != nil {
				t.Fatalf("Compile = %v; want the fixture compiled in the default mode", err)
			}
			got := map[string]string{}
			collectDefaultValues(c.root, got)
			if !maps.Equal(got, f.defaults) {
				t.Errorf("default values placed = %v; want %v", got, f.defaults)
			}

			_, err = Compile(model, Options{Strict: true})
			if refused := errors.Is(err, ErrInvalidInput); refused != f.compileStrictRefuses {
				t.Errorf("Compile(Strict) = %v; want refused with ErrInvalidInput %t", err, f.compileStrictRefuses)
			}
		})
	}
}

// collectDefaultValues adds to got, for n and each node below it that
// carries a default value, the node's AQL path and RM type and the value's RM
// type. Alternatives under one attribute share a path, so the node's RM type
// tells them apart.
func collectDefaultValues(n *CompiledNode, got map[string]string) {
	if dv := n.DefaultValue(); dv != nil {
		rmType, _ := rm.RMTypeName(dv)
		got[n.aqlPath+" "+n.rmTypeName] = rmType
	}
	for _, a := range n.attributes {
		for _, child := range a.children {
			collectDefaultValues(child, got)
		}
	}
}

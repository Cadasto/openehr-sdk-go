package templatecompile

// parity_fixtures_test.go: REQ-111 § Parity with the deprecated parser, over
// the parity fixtures, OPTs the SDK writes in testdata/parity for each value
// the compile step tolerates and each default-value placement it names.

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
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// parityFixtureDir holds the parity fixtures.
const parityFixtureDir = "testdata/parity"

// TestREQ111_ParityFixtures reads each parity fixture with the parser and
// with the model's reader, both in their default mode, and compiles it: on
// each node the compile step builds the primitive constraint the parser
// reads, and it places the default values the fixture names. In their strict
// modes ParseOPTStrict refuses a fixture of a tolerated value, and so does
// the compile step, which also refuses a default value that addresses no
// node, which the parser does not read.
func TestREQ111_ParityFixtures(t *testing.T) {
	const items = "/context/other_context/items"
	fixtures := []struct {
		file string
		// parserStrictRefuses is true when ParseOPTStrict refuses the
		// fixture.
		parserStrictRefuses bool
		// compileStrictRefuses is true when the compile step refuses it
		// with Options.Strict.
		compileStrictRefuses bool
		// defaults are the default values the compile step places, each
		// as the AQL path and RM type of the node that carries it and the
		// RM type of the value.
		defaults map[string]string
	}{
		{file: "constraint_ref_no_reference.opt", parserStrictRefuses: true, compileStrictRefuses: true},
		{file: "constraint_ref_blank_reference.opt", parserStrictRefuses: true, compileStrictRefuses: true},
		{file: "date_timezone_mandatory.opt", parserStrictRefuses: true, compileStrictRefuses: true},
		{file: "temporal_bound_not_iso8601.opt", parserStrictRefuses: true, compileStrictRefuses: true},
		{file: "duration_bounds_not_iso8601.opt", parserStrictRefuses: true, compileStrictRefuses: true},
		{file: "primitive_object_no_item.opt", parserStrictRefuses: true, compileStrictRefuses: true},
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
			parsed, err := template.ParseOPT(bytes.NewReader(raw)) //nolint:staticcheck // the parity check reads the fixture with the deprecated parser
			if err != nil {
				t.Fatalf("template.ParseOPT = %v", err)
			}
			model, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("aom14.ReadOperationalTemplate = %v", err)
			}
			for _, p := range pairModelNodes(t, f.file, parsed.Root(), &model.Definition, nil) { //nolint:staticcheck // the parser's tree is what the check compares with
				var want constraints.PrimitiveConstraint
				if on, ok := p.parsed.(interface {
					PrimitiveConstraint() constraints.PrimitiveConstraint
				}); ok {
					want = on.PrimitiveConstraint()
				}
				got, err := primitiveFromModel(p.model, false)
				if err != nil {
					t.Errorf("%s: primitiveFromModel(%T) = %v; want the parser's %s", p.where, p.model, err, renderPrimitive(want))
					continue
				}
				if g, w := renderPrimitive(got), renderPrimitive(want); g != w {
					t.Errorf("%s: primitiveFromModel(%T) =\n  %s\nwant the parser's\n  %s", p.where, p.model, g, w)
				}
			}

			c, err := Compile(model)
			if err != nil {
				t.Fatalf("Compile = %v; want the fixture read as the parser reads it", err)
			}
			got := map[string]string{}
			collectDefaultValues(c.root, got)
			if !maps.Equal(got, f.defaults) {
				t.Errorf("default values placed = %v; want %v", got, f.defaults)
			}

			_, err = template.ParseOPTStrict(bytes.NewReader(raw)) //nolint:staticcheck // the parity check reads the fixture with the deprecated parser
			if refused := err != nil; refused != f.parserStrictRefuses {
				t.Errorf("template.ParseOPTStrict = %v; want refused %t", err, f.parserStrictRefuses)
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

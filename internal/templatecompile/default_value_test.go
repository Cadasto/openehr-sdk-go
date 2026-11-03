package templatecompile_test

// default_value_test.go: REQ-111 § Reading the model. The compiled template
// carries each default value of the template's <constraints> on the node its
// T_ATTRIBUTE addresses, and Compile refuses a default value that addresses
// no node.

import (
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templatecompile/walk"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// constrainTestModel reads the corpus template whose <constraints> carry
// default values, one per T_ATTRIBUTE.
func constrainTestModel(t *testing.T) *aom14.OperationalTemplate {
	t.Helper()
	return mustReadModelFile(t, filepath.Join(fixtures.CorpusRoot(), "webtemplate", "constrain_test.opt"))
}

// defaultValues returns each node of c that carries a default value, by its
// AQL path.
func defaultValues(t *testing.T, c *templatecompile.Compiled) map[string]rm.DataValue {
	t.Helper()
	got := map[string]rm.DataValue{}
	err := walk.Walk(c, walk.VisitorFunc{Pre: func(ctx *walk.Context) error {
		if dv := ctx.Node().DefaultValue(); dv != nil {
			got[ctx.Node().AQLPath()] = dv
		}
		return nil
	}})
	if err != nil {
		t.Fatalf("walk.Walk: %v", err)
	}
	return got
}

func TestREQ111_DefaultValuesPlacedOnTheirNodes(t *testing.T) {
	c, err := templatecompile.Compile(constrainTestModel(t))
	if err != nil {
		t.Fatalf("Compile(constrain_test) = %v", err)
	}
	want := map[string]rm.DataValue{
		"/context/other_context/items[openEHR-EHR-CLUSTER.address.v0]/items[at0001]/items[at0015]/items[at0007]/value": &rm.DVDateTime{
			Value: "2019-01-28T21:22:49.426Z",
		},
		"/content[openEHR-EHR-OBSERVATION.abbey_pain_scale.v0]/data/events[at0002]/data/items[at0004]/value": &rm.DVOrdinal{
			Value: 1,
			Symbol: rm.DVCodedText{
				DVText:       rm.DVText{Value: "Mild"},
				DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0011"},
			},
		},
		"/content[openEHR-EHR-OBSERVATION.abbey_pain_scale.v0]/data/events[at0002]/data/items[at0014]/value": &rm.DVCount{
			Magnitude: 11,
		},
		"/content[openEHR-EHR-OBSERVATION.blood_pressure.v2]/data/events[at1042]/data/items[at0004]/value": &rm.DVQuantity{
			Magnitude: 17, Units: "mm[Hg]",
		},
		"/content[openEHR-EHR-OBSERVATION.blood_pressure.v2]/data/events[at1042]/data/items[at1059]/value": &rm.DVText{
			Value: "default value",
		},
		"/content[openEHR-EHR-EVALUATION.infant_feeding.v0]/data/items[at0003]/items[at0004]/value": &rm.DVDuration{
			Value: "P6Y",
		},
		"/content[openEHR-EHR-OBSERVATION.affected_body_surface_area.v0]/data/events[at0002]/data/items[at0014]/value": &rm.DVProportion{
			Numerator: 10, Denominator: 100, Type: 2,
		},
	}
	got := defaultValues(t, c)
	for path, w := range want {
		g, ok := got[path]
		if !ok {
			t.Errorf("node %s carries no default value; want %#v", path, w)
			continue
		}
		if !reflect.DeepEqual(g, w) {
			t.Errorf("node %s DefaultValue() = %#v; want %#v", path, g, w)
		}
	}
	for path, g := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("node %s carries the default value %#v; want none", path, g)
		}
	}
}

// A template whose <constraints> are empty, as five corpus templates write
// them, places nothing.
func TestREQ111_EmptyConstraintsPlaceNoDefaultValue(t *testing.T) {
	model := mustReadModelFile(t, filepath.Join(fixtures.CorpusRoot(), "templates", "vital_signs.opt"))
	if model.Constraints == nil {
		t.Fatal("vital_signs.opt reads with no <constraints>; want the empty one it writes")
	}
	c, err := templatecompile.Compile(model)
	if err != nil {
		t.Fatalf("Compile(vital_signs) = %v", err)
	}
	if got := defaultValues(t, c); len(got) != 0 {
		t.Errorf("default values = %v; want none", got)
	}
}

// The compiled template shares no default value with the model, and each
// call returns a value of the caller's own.
func TestREQ111_DefaultValueIsACopy(t *testing.T) {
	model := constrainTestModel(t)
	c, err := templatecompile.Compile(model)
	if err != nil {
		t.Fatalf("Compile(constrain_test) = %v", err)
	}
	const path = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v2]/data/events[at1042]/data/items[at1059]/value"
	n, err := c.NodeAt(path)
	if err != nil {
		t.Fatalf("NodeAt(%s) = %v", path, err)
	}
	first, ok := n.DefaultValue().(*rm.DVText)
	if !ok {
		t.Fatalf("DefaultValue() = %#v; want a *rm.DVText", n.DefaultValue())
	}
	first.Value = "changed by the caller"
	for _, a := range model.Constraints.Attributes {
		for _, child := range a.Children {
			if text, ok := child.DefaultValue.(*rm.DVText); ok {
				text.Value = "changed in the model"
			}
		}
	}
	if got := n.DefaultValue().(*rm.DVText).Value; got != "default value" {
		t.Errorf("DefaultValue().Value = %q after the caller and the model changed theirs; want %q", got, "default value")
	}
}

// Compile refuses a default value whose differential path does not open with
// the definition's archetype id, or that addresses no node, no attribute, or
// no child of its RM type, with an error that names the place and not the
// value. A constraint with no default value places nothing and is not read.
func TestREQ111_CompileRefusesADefaultValueThatAddressesNoNode(t *testing.T) {
	const (
		root    = "[openEHR-EHR-COMPOSITION.encounter.v1]"
		element = root + "/content[openEHR-EHR-OBSERVATION.blood_pressure.v2]/data[at0001]/events[at1042]/data[at0003]/items[at1059]"
		// sentinel is a value no message holds otherwise, so the check
		// that the error does not repeat the value can fail.
		sentinel = "sentinel-value-1f3e"
	)
	cases := []struct {
		name    string
		edit    func(a *aom14.TAttribute)
		wantMsg string // "" when the edit is admitted
		wantAt  string // the node that then carries the default value, if any
	}{
		{
			name: "another archetype's id opens the path",
			edit: func(a *aom14.TAttribute) {
				a.DifferentialPath = strings.Replace(a.DifferentialPath, "COMPOSITION.encounter.v1", "COMPOSITION.report.v1", 1)
			},
			wantMsg: "openEHR-EHR-COMPOSITION.report.v1",
		},
		{
			name:    "no archetype id opens the path",
			edit:    func(a *aom14.TAttribute) { a.DifferentialPath = strings.TrimPrefix(a.DifferentialPath, root) },
			wantMsg: "/content[openEHR-EHR-OBSERVATION.blood_pressure.v2]",
		},
		{
			name: "the path reaches no node",
			edit: func(a *aom14.TAttribute) {
				a.DifferentialPath = strings.Replace(a.DifferentialPath, "items[at1059]", "items[at9999]", 1)
			},
			wantMsg: "items[at9999]",
		},
		{
			name:    "the path does not parse",
			edit:    func(a *aom14.TAttribute) { a.DifferentialPath += "/" },
			wantMsg: "items[at1059]/",
		},
		{
			name:    "the node has no such attribute",
			edit:    func(a *aom14.TAttribute) { a.RMAttributeName = "no_such_attribute" },
			wantMsg: "no_such_attribute",
		},
		{
			name:    "the attribute has no child of the RM type",
			edit:    func(a *aom14.TAttribute) { a.Children[0].RMTypeName = "DV_BOOLEAN" },
			wantMsg: "DV_BOOLEAN",
		},
		{
			name: "the root addressed by the archetype id alone",
			edit: func(a *aom14.TAttribute) {
				a.DifferentialPath = root
				a.RMAttributeName = "category"
				a.Children[0].RMTypeName = "DV_CODED_TEXT"
				a.Children[0].DefaultValue = &rm.DVCodedText{Value: "event"}
			},
			wantAt: "/category",
		},
		{
			name: "a value that does not encode as canonical JSON",
			edit: func(a *aom14.TAttribute) {
				a.Children[0].RMTypeName = "DV_TEXT"
				a.Children[0].DefaultValue = &rm.DVQuantity{Magnitude: rm.Real(math.NaN()), Units: sentinel}
			},
			wantMsg: "*rm.DVQuantity does not encode",
		},
		{
			name: "a constraint with no default value addresses nothing",
			edit: func(a *aom14.TAttribute) {
				a.DifferentialPath = strings.Replace(a.DifferentialPath, "items[at1059]", "items[at9999]", 1)
				a.Children[0].DefaultValue = nil
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := constrainTestModel(t)
			var target *aom14.TAttribute
			for i := range model.Constraints.Attributes {
				if model.Constraints.Attributes[i].DifferentialPath == element {
					target = &model.Constraints.Attributes[i]
				}
			}
			if target == nil {
				t.Fatalf("constrain_test.opt has no constraint at %s", element)
			}
			target.Children[0].DefaultValue = &rm.DVText{Value: sentinel}
			tc.edit(target)
			c, err := templatecompile.Compile(model)
			if tc.wantMsg == "" {
				if err != nil {
					t.Fatalf("Compile = %v; want no error", err)
				}
				if tc.wantAt == "" {
					return
				}
				n, err := c.NodeAt(tc.wantAt)
				if err != nil {
					t.Fatalf("NodeAt(%s) = %v", tc.wantAt, err)
				}
				if n.DefaultValue() == nil {
					t.Errorf("node %s carries no default value; want one", tc.wantAt)
				}
				return
			}
			if !errors.Is(err, templatecompile.ErrInvalidInput) {
				t.Fatalf("Compile = %v; want an error wrapping ErrInvalidInput", err)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("Compile error = %q; want it to name %q", err, tc.wantMsg)
			}
			// The value is not template structure, so the error does not
			// repeat it.
			if strings.Contains(err.Error(), sentinel) {
				t.Errorf("Compile error = %q; want it free of the default value's text", err)
			}
		})
	}
}

// A default value whose T_ATTRIBUTE names an ELEMENT's null_flavor lands on
// the attribute the compile reads that name as: null_flavour, unless the
// ELEMENT declares both names, when each keeps its own.
func TestREQ111_DefaultValueOnNullFlavorLandsWhereTheCompileReadsIt(t *testing.T) {
	const (
		archetypeID = "openEHR-EHR-COMPOSITION.model.v1" // the root modelOf gives
		elementPath = "/context/other_context[at0001]/items[at0002]"
	)
	cases := []struct {
		name     string
		declared []string // the ELEMENT's attributes, each over one DV_CODED_TEXT
		wantAt   string   // the attribute whose child carries the default value
	}{
		{name: "null_flavor alone", declared: []string{"null_flavor"}, wantAt: "null_flavour"},
		{name: "null_flavor beside null_flavour", declared: []string{"null_flavor", "null_flavour"}, wantAt: "null_flavor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var attrs []aom14.CAttribute
			for _, name := range tc.declared {
				attrs = append(attrs, single(name, complexObject("DV_CODED_TEXT", "")))
			}
			model := modelOf(single("context", complexObject("EVENT_CONTEXT", "",
				single("other_context", complexObject("ITEM_TREE", "at0001",
					multiple("items", complexObject("ELEMENT", "at0002", attrs...)))))))
			model.Constraints = &aom14.TConstraint{Attributes: []aom14.TAttribute{{
				DifferentialPath: "[" + archetypeID + "]" + elementPath,
				RMAttributeName:  "null_flavor",
				Children: []aom14.TComplexObject{{
					CComplexObject: *complexObject("DV_CODED_TEXT", ""),
					DefaultValue: &rm.DVCodedText{
						Value:        "unknown",
						DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "253"},
					},
				}},
			}}}

			c, err := templatecompile.Compile(model)
			if err != nil {
				t.Fatalf("Compile = %v; want the default value placed", err)
			}
			path := elementPath + "/" + tc.wantAt
			p, err := template.ParsePath(path)
			if err != nil {
				t.Fatalf("ParsePath(%q): %v", path, err)
			}
			n, err := c.Resolve(p)
			if err != nil {
				t.Fatalf("Resolve(%s) = %v", path, err)
			}
			got, ok := n.DefaultValue().(*rm.DVCodedText)
			if !ok || got.DefiningCode.CodeString != "253" {
				t.Errorf("DefaultValue() at %s = %#v; want the DV_CODED_TEXT with code 253", path, n.DefaultValue())
			}
		})
	}
}

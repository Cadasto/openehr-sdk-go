package templatecompile_test

// default_value_test.go: REQ-111 § Reading the model. The compiled template
// carries each default value of the template's <constraints> on the node its
// T_ATTRIBUTE addresses, and Compile skips a default value that addresses no
// node, or refuses it in its strict mode.

import (
	"errors"
	"fmt"
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

// Compile skips, in its default mode, a default value that addresses no node:
// one whose differential path opens with neither the definition's archetype
// id nor a slash, or does not split into segments, or that reaches no node, no
// attribute of its name or no child of its RM type. With Options.Strict it
// refuses one, with an error wrapping ErrInvalidInput that names the place
// and not the value. It skips, and refuses with Options.Strict, a value that
// does not encode as canonical JSON in the same way, and a constraint with no
// default value places nothing and is not read.
func TestREQ111_CompileSkipsADefaultValueThatAddressesNoNode(t *testing.T) {
	const (
		root    = "[openEHR-EHR-COMPOSITION.encounter.v1]"
		element = root + "/content[openEHR-EHR-OBSERVATION.blood_pressure.v2]/data[at0001]/events[at1042]/data[at0003]/items[at1059]"
		// sentinel is a value no message holds otherwise, so the check
		// that the error does not repeat the value can fail.
		sentinel = "sentinel-value-1f3e"
		// others is how many default values constrain_test.opt places
		// beside the one each case edits.
		others = 6
	)
	cases := []struct {
		name string
		edit func(a *aom14.TAttribute)
		// wantMsg is what the strict mode's error names, "" when both
		// modes place the value or the case has none.
		wantMsg string
		// wantAt is the node that then carries the default value, if any.
		wantAt string
	}{
		{
			name: "another archetype's id opens the path",
			edit: func(a *aom14.TAttribute) {
				a.DifferentialPath = strings.Replace(a.DifferentialPath, "COMPOSITION.encounter.v1", "COMPOSITION.report.v1", 1)
			},
			wantMsg: "openEHR-EHR-COMPOSITION.report.v1",
		},
		{
			name:    "neither an archetype id nor a slash opens the path",
			edit:    func(a *aom14.TAttribute) { a.DifferentialPath = strings.TrimPrefix(a.DifferentialPath, root+"/") },
			wantMsg: "content[openEHR-EHR-OBSERVATION.blood_pressure.v2]",
		},
		{
			name: "the path reaches no node",
			edit: func(a *aom14.TAttribute) {
				a.DifferentialPath = strings.Replace(a.DifferentialPath, "items[at1059]", "items[at9999]", 1)
			},
			wantMsg: "items[at9999]",
		},
		{
			name:    "the path ends with an empty segment",
			edit:    func(a *aom14.TAttribute) { a.DifferentialPath += "/" },
			wantMsg: "items[at1059]/",
		},
		{
			name:    "a predicate of the path does not close",
			edit:    func(a *aom14.TAttribute) { a.DifferentialPath = strings.TrimSuffix(a.DifferentialPath, "]") },
			wantMsg: "items[at1059",
		},
		{
			name: "a name predicate that no node's name matches",
			edit: func(a *aom14.TAttribute) {
				a.DifferentialPath = strings.Replace(a.DifferentialPath, "items[at1059]", "items[at1059, 'Clinical interpretation']", 1)
			},
			wantMsg: "Clinical interpretation",
		},
		{
			name: "a name predicate with no quotes",
			edit: func(a *aom14.TAttribute) {
				a.DifferentialPath = strings.Replace(a.DifferentialPath, "items[at1059]", "items[at1059, Comment]", 1)
			},
			wantMsg: "at1059, Comment",
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
			name: "the attribute has no child of the default value's RM type",
			edit: func(a *aom14.TAttribute) {
				a.Children[0].RMTypeName = ""
				a.Children[0].DefaultValue = &rm.DVCodedText{Value: sentinel}
			},
			wantMsg: "DV_CODED_TEXT",
		},
		{
			name:   "a path that opens with a slash",
			edit:   func(a *aom14.TAttribute) { a.DifferentialPath = strings.TrimPrefix(a.DifferentialPath, root) },
			wantAt: "/content[openEHR-EHR-OBSERVATION.blood_pressure.v2]/data/events[at1042]/data/items[at1059]/value",
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
			name: "a value of an infinite magnitude",
			edit: func(a *aom14.TAttribute) {
				a.Children[0].RMTypeName = "DV_TEXT"
				a.Children[0].DefaultValue = &rm.DVQuantity{Magnitude: rm.Real(math.Inf(-1)), Units: sentinel}
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
		for _, strict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/strict=%t", tc.name, strict), func(t *testing.T) {
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
				c, err := templatecompile.Compile(model, templatecompile.Options{Strict: strict})
				if strict && tc.wantMsg != "" {
					if !errors.Is(err, templatecompile.ErrInvalidInput) {
						t.Fatalf("Compile = %v; want an error wrapping ErrInvalidInput", err)
					}
					if !strings.Contains(err.Error(), tc.wantMsg) {
						t.Errorf("Compile error = %q; want it to name %q", err, tc.wantMsg)
					}
					// The value is not template structure, so the error does
					// not repeat it.
					if strings.Contains(err.Error(), sentinel) {
						t.Errorf("Compile error = %q; want it free of the default value's text", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("Compile = %v; want no error", err)
				}
				got := defaultValues(t, c)
				want := others
				if tc.wantAt != "" {
					want++
					n, err := c.NodeAt(tc.wantAt)
					if err != nil {
						t.Fatalf("NodeAt(%s) = %v", tc.wantAt, err)
					}
					if n.DefaultValue() == nil {
						t.Errorf("node %s carries no default value; want one", tc.wantAt)
					}
				}
				if len(got) != want {
					t.Errorf("Compile placed %d default values; want %d", len(got), want)
				}
				for path, dv := range got {
					var value string
					switch v := dv.(type) {
					case *rm.DVText:
						value = v.Value
					case *rm.DVCodedText:
						value = v.Value
					}
					if value == sentinel && path != tc.wantAt {
						t.Errorf("node %s carries the edited default value; want it skipped", path)
					}
				}
			})
		}
	}
}

// placementModel is a model whose definition holds, under the items of
// /context/other_context[at0001], two CLUSTER archetype roots of one
// archetype id, named Infusion pump and Ventilator, each over an ELEMENT
// at0001; an ELEMENT at0002; two ELEMENTs at0003, named Smith and O'Brien;
// an ELEMENT at0004 whose value has a DV_COUNT and a DV_PROPORTION
// alternative; and an ELEMENT at0006 whose value is a DV_INTERVAL<DV_COUNT>.
// Each ELEMENT's value is a DV_TEXT unless said otherwise, and the
// definition declares a category.
func placementModel() *aom14.OperationalTemplate {
	named := func(name string) aom14.CAttribute {
		return single("name", complexObject("DV_TEXT", "", single("value",
			&aom14.CPrimitiveObject{RMTypeName: "STRING", Occurrences: once, Item: &aom14.CString{List: []string{name}}})))
	}
	element := func(nodeID, name string, valueTypes ...string) *aom14.CComplexObject {
		var attrs []aom14.CAttribute
		if name != "" {
			attrs = append(attrs, named(name))
		}
		var values []aom14.CObject
		for _, rmType := range valueTypes {
			values = append(values, complexObject(rmType, ""))
		}
		return complexObject("ELEMENT", nodeID, append(attrs, single("value", values...))...)
	}
	device := func(name string) *aom14.CArchetypeRoot {
		return &aom14.CArchetypeRoot{
			CComplexObject: *complexObject("CLUSTER", "at0000", named(name), multiple("items", element("at0001", "", "DV_TEXT"))),
			ArchetypeID:    rm.ArchetypeID{Value: "openEHR-EHR-CLUSTER.device.v1"},
		}
	}
	return modelOf(
		single("category", complexObject("DV_CODED_TEXT", "")),
		single("context", complexObject("EVENT_CONTEXT", "",
			single("other_context", complexObject("ITEM_TREE", "at0001",
				multiple("items",
					device("Infusion pump"),
					device("Ventilator"),
					element("at0002", "", "DV_TEXT"),
					element("at0003", "Smith", "DV_TEXT"),
					element("at0003", "O'Brien", "DV_TEXT"),
					element("at0004", "", "DV_COUNT", "DV_PROPORTION"),
					element("at0006", "", "DV_INTERVAL<DV_COUNT>"),
				))))))
}

// countInterval is a default value of RM type DV_INTERVAL, from one to five.
func countInterval() *rm.DVInterval[rm.DVCount] {
	return &rm.DVInterval[rm.DVCount]{
		Lower: rm.DVCount{Magnitude: 1}, LowerIncluded: true,
		Upper: rm.DVCount{Magnitude: 5}, UpperIncluded: true,
	}
}

// TestREQ111_DefaultValuePlacementRules pins the placements of § REQ-111
// Reading the model, in both modes: a differential path that opens with the
// definition's archetype id resolves from the rest, and one that opens with
// a slash from the definition's root; a segment's predicate may carry a
// name, after a comma or in an and name/value clause, which selects the node
// of that node id or archetype id whose template-level name it is, each run
// of white space inside the predicate read as one space; a constraint
// object that names no RM type takes its default value's; and a child's
// generic parameters are set aside when its RM type is matched, so
// DV_INTERVAL matches DV_INTERVAL<DV_COUNT>.
func TestREQ111_DefaultValuePlacementRules(t *testing.T) {
	const (
		root    = "[openEHR-EHR-COMPOSITION.model.v1]"
		items   = "/context/other_context[at0001]/items"
		device  = "openEHR-EHR-CLUSTER.device.v1"
		atItems = "/context/other_context/items"
	)
	text := &rm.DVText{Value: "placed"}
	cases := []struct {
		name     string
		path     string
		attr     string       // the T_ATTRIBUTE's rm_attribute_name, value when ""
		rmType   string       // the T_COMPLEX_OBJECT's rm_type_name
		value    rm.DataValue // its default value
		wantAt   string       // the AQL path of the node that carries it
		wantType string       // that node's RM type, which tells alternatives apart
	}{
		{
			name: "a path that opens with the definition's archetype id", path: root + items + "[at0002]",
			rmType: "DV_TEXT", value: text, wantAt: atItems + "[at0002]/value", wantType: "DV_TEXT",
		},
		{
			name: "a path that opens with a slash", path: items + "[at0002]",
			rmType: "DV_TEXT", value: text, wantAt: atItems + "[at0002]/value", wantType: "DV_TEXT",
		},
		{
			name: "the definition's archetype id alone", path: root, attr: "category",
			rmType: "DV_CODED_TEXT", value: &rm.DVCodedText{Value: "event"}, wantAt: "/category", wantType: "DV_CODED_TEXT",
		},
		{
			name: "a slash alone", path: "/", attr: "category",
			rmType: "DV_CODED_TEXT", value: &rm.DVCodedText{Value: "event"}, wantAt: "/category", wantType: "DV_CODED_TEXT",
		},
		{
			name: "a name after a comma", path: root + items + "[" + device + ", 'Ventilator']/items[at0001]",
			rmType: "DV_TEXT", value: text, wantAt: atItems + "[" + device + ",'Ventilator']/items[at0001]/value", wantType: "DV_TEXT",
		},
		{
			name: "a name after a comma with no space, as the compile writes it", path: root + items + "[" + device + ",'Ventilator']/items[at0001]",
			rmType: "DV_TEXT", value: text, wantAt: atItems + "[" + device + ",'Ventilator']/items[at0001]/value", wantType: "DV_TEXT",
		},
		{
			name: "a name in an and name/value clause", path: root + items + "[" + device + " and name/value='Ventilator']/items[at0001]",
			rmType: "DV_TEXT", value: text, wantAt: atItems + "[" + device + ",'Ventilator']/items[at0001]/value", wantType: "DV_TEXT",
		},
		{
			name: "a name in an and name/value clause with spaces around the equals sign", path: root + items + "[" + device + " and name/value = 'Ventilator']/items[at0001]",
			rmType: "DV_TEXT", value: text, wantAt: atItems + "[" + device + ",'Ventilator']/items[at0001]/value", wantType: "DV_TEXT",
		},
		{
			name:   "runs of white space in a predicate, in a name broken across lines among them",
			path:   "\n    " + root + items + "[" + device + "\n      and\tname/value='Infusion\n      pump']/items[at0001]\n  ",
			rmType: "DV_TEXT", value: text, wantAt: atItems + "[" + device + ",'Infusion pump']/items[at0001]/value", wantType: "DV_TEXT",
		},
		{
			name: "a name with an escaped quote", path: root + items + `[at0003, 'O\'Brien']`,
			rmType: "DV_TEXT", value: text, wantAt: atItems + `[at0003,'O\'Brien']/value`, wantType: "DV_TEXT",
		},
		{
			name: "a constraint object that names no RM type takes its default value's", path: root + items + "[at0004]",
			value: &rm.DVProportion{Numerator: 1, Denominator: 2}, wantAt: atItems + "[at0004]/value", wantType: "DV_PROPORTION",
		},
		{
			name: "a constraint object that names no RM type takes its default value's, a child's generic parameters set aside", path: root + items + "[at0006]",
			value: countInterval(), wantAt: atItems + "[at0006]/value", wantType: "DV_INTERVAL<DV_COUNT>",
		},
		{
			name: "a constraint object's RM type matched with a child's generic parameters set aside", path: root + items + "[at0006]",
			rmType: "DV_INTERVAL", value: countInterval(), wantAt: atItems + "[at0006]/value", wantType: "DV_INTERVAL<DV_COUNT>",
		},
		{
			name: "a constraint object that names the child's generic parameters", path: root + items + "[at0006]",
			rmType: "DV_INTERVAL<DV_COUNT>", value: countInterval(), wantAt: atItems + "[at0006]/value", wantType: "DV_INTERVAL<DV_COUNT>",
		},
	}
	for _, tc := range cases {
		for _, strict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/strict=%t", tc.name, strict), func(t *testing.T) {
				attr := tc.attr
				if attr == "" {
					attr = "value"
				}
				model := placementModel()
				model.Constraints = &aom14.TConstraint{Attributes: []aom14.TAttribute{{
					DifferentialPath: tc.path,
					RMAttributeName:  attr,
					Children:         []aom14.TComplexObject{{RMTypeName: tc.rmType, DefaultValue: tc.value}},
				}}}
				c, err := templatecompile.Compile(model, templatecompile.Options{Strict: strict})
				if err != nil {
					t.Fatalf("Compile = %v; want the default value placed", err)
				}
				var got []string
				err = walk.Walk(c, walk.VisitorFunc{Pre: func(ctx *walk.Context) error {
					if n := ctx.Node(); n.DefaultValue() != nil {
						got = append(got, n.AQLPath()+" "+n.RMTypeName())
					}
					return nil
				}})
				if err != nil {
					t.Fatalf("walk.Walk: %v", err)
				}
				if want := tc.wantAt + " " + tc.wantType; len(got) != 1 || got[0] != want {
					t.Errorf("the default value is carried by %q; want %q alone", got, want)
				}
			})
		}
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

// TestREQ111_DefaultValueOnAMalformedPathAddressesNoNode pins that a
// differential path the placements of § REQ-111 Reading the model do not
// read addresses no node, even where a looser reading would reach one:
// Compile skips its default value in its default mode and refuses it with
// Options.Strict.
func TestREQ111_DefaultValueOnAMalformedPathAddressesNoNode(t *testing.T) {
	const (
		root  = "[openEHR-EHR-COMPOSITION.model.v1]"
		items = "/context/other_context[at0001]/items"
	)
	cases := []struct {
		name, path, attr, rmType string
		value                    rm.DataValue
	}{
		{
			name: "an archetype id that does not close", path: "[openEHR-EHR-COMPOSITION.model.v1",
			attr: "category", rmType: "DV_CODED_TEXT", value: &rm.DVCodedText{Value: "event"},
		},
		{
			name: "a predicate followed by neither a slash nor the end", path: root + "/context/other_context[at0001]_items[at0002]",
			attr: "value", rmType: "DV_TEXT", value: &rm.DVText{Value: "skipped"},
		},
		{
			name: "an empty predicate", path: root + "/context[]/other_context[at0001]/items[at0002]",
			attr: "value", rmType: "DV_TEXT", value: &rm.DVText{Value: "skipped"},
		},
		{
			name: "a name after a space alone", path: root + items + "[at0003 'Smith']",
			attr: "value", rmType: "DV_TEXT", value: &rm.DVText{Value: "skipped"},
		},
		{
			name: "an and name/value clause with no equals sign", path: root + items + "[at0003 and name/value 'Smith']",
			attr: "value", rmType: "DV_TEXT", value: &rm.DVText{Value: "skipped"},
		},
		{
			name: "a name in double quotes", path: root + items + `[at0003, "Smith"]`,
			attr: "value", rmType: "DV_TEXT", value: &rm.DVText{Value: "skipped"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := placementModel()
			model.Constraints = &aom14.TConstraint{Attributes: []aom14.TAttribute{{
				DifferentialPath: tc.path,
				RMAttributeName:  tc.attr,
				Children:         []aom14.TComplexObject{{RMTypeName: tc.rmType, DefaultValue: tc.value}},
			}}}
			c, err := templatecompile.Compile(model)
			if err != nil {
				t.Fatalf("Compile = %v; want the default value skipped", err)
			}
			if got := defaultValues(t, c); len(got) != 0 {
				t.Errorf("default values placed = %v; want none", got)
			}
			if _, err := templatecompile.Compile(model, templatecompile.Options{Strict: true}); !errors.Is(err, templatecompile.ErrInvalidInput) {
				t.Errorf("Compile(Strict) = %v; want an error wrapping ErrInvalidInput", err)
			}
		})
	}
}

// TestREQ111_DefaultValueOfAnotherGenericParameterAddressesNoNode pins that
// only a child's generic parameters are set aside when its RM type is
// matched: a constraint object that names DV_INTERVAL<DV_QUANTITY> does not
// match a DV_INTERVAL<DV_COUNT> child, so Compile skips its default value in
// its default mode and refuses it with Options.Strict.
func TestREQ111_DefaultValueOfAnotherGenericParameterAddressesNoNode(t *testing.T) {
	model := placementModel()
	model.Constraints = &aom14.TConstraint{Attributes: []aom14.TAttribute{{
		DifferentialPath: "[openEHR-EHR-COMPOSITION.model.v1]/context/other_context[at0001]/items[at0006]",
		RMAttributeName:  "value",
		Children:         []aom14.TComplexObject{{RMTypeName: "DV_INTERVAL<DV_QUANTITY>", DefaultValue: countInterval()}},
	}}}
	c, err := templatecompile.Compile(model)
	if err != nil {
		t.Fatalf("Compile = %v; want the default value skipped", err)
	}
	if got := defaultValues(t, c); len(got) != 0 {
		t.Errorf("default values placed = %v; want none", got)
	}
	_, err = templatecompile.Compile(model, templatecompile.Options{Strict: true})
	if !errors.Is(err, templatecompile.ErrInvalidInput) {
		t.Fatalf("Compile(Strict) = %v; want an error wrapping ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "DV_INTERVAL<DV_QUANTITY>") {
		t.Errorf("Compile(Strict) error = %q; want it to name the RM type DV_INTERVAL<DV_QUANTITY>", err)
	}
}

// TestREQ111_DefaultValueOfNoRMTypeAddressesNoNode pins that a default value
// whose constraint object names no RM type and whose value has none, a nil
// pointer, addresses no node, even under an attribute whose child names no
// RM type either: Compile skips it in its default mode and refuses it with
// Options.Strict.
func TestREQ111_DefaultValueOfNoRMTypeAddressesNoNode(t *testing.T) {
	model := modelOf(single("context", complexObject("EVENT_CONTEXT", "",
		single("other_context", complexObject("", "at0001")))))
	model.Constraints = &aom14.TConstraint{Attributes: []aom14.TAttribute{{
		DifferentialPath: "[openEHR-EHR-COMPOSITION.model.v1]/context",
		RMAttributeName:  "other_context",
		Children:         []aom14.TComplexObject{{DefaultValue: (*rm.DVText)(nil)}},
	}}}
	if _, err := templatecompile.Compile(model); err != nil {
		t.Fatalf("Compile = %v; want the default value skipped", err)
	}
	if _, err := templatecompile.Compile(model, templatecompile.Options{Strict: true}); !errors.Is(err, templatecompile.ErrInvalidInput) {
		t.Errorf("Compile(Strict) = %v; want an error wrapping ErrInvalidInput", err)
	}
}

package instance_test

import (
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The helpers below build small synthetic OPTs for the REQ-107 default
// fills. Each OPT names only what its case needs, so the generator's own
// default is the only thing that can fill the field under test.

// optNode is a C_COMPLEX_OBJECT of rmType with the given node id and
// attributes.
func optNode(rmType, nodeID string, attrs ...string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<node_id>` + nodeID + `</node_id>` + strings.Join(attrs, "") + `</children>`
}

// optArchetypeRoot is a C_ARCHETYPE_ROOT of rmType: a child the OPT pins
// as an archetype of its own, so the generator can name its archetype.
func optArchetypeRoot(rmType, archetypeID string, attrs ...string) string {
	return `<children xsi:type="C_ARCHETYPE_ROOT"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<node_id>at0000</node_id>` + strings.Join(attrs, "") +
		`<archetype_id><value>` + archetypeID + `</value></archetype_id></children>`
}

// optPrimitive is a C_PRIMITIVE_OBJECT of the AOM primitive rmType whose
// item is an itemType carrying body.
func optPrimitive(rmType, itemType, body string) string {
	return `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<node_id></node_id><item xsi:type="` + itemType + `">` + body + `</item></children>`
}

// optCodedText is a DV_CODED_TEXT whose defining code the OPT pins to
// codes of terminologyID with a C_CODE_PHRASE. The OPT says nothing about
// the text.
func optCodedText(terminologyID string, codes ...string) string {
	return optNode("DV_CODED_TEXT", "", optSingle("defining_code", optCodePhrase(terminologyID, codes...)))
}

// walkText is the text the walk gives a DV_CODED_TEXT built by
// optCodedText, under every policy and value fill: the OPT constrains no
// text, so it is the open-string example.
const walkText = "example"

// optSingle is a C_SINGLE_ATTRIBUTE called name over children.
func optSingle(name string, children ...string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>1</lower><upper>1</upper></existence>` + strings.Join(children, "") + `</attributes>`
}

// optMultiple is a C_MULTIPLE_ATTRIBUTE called name over children.
func optMultiple(name string, children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		strings.Join(children, "") +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality></attributes>`
}

// optOptionalMultiple is a C_MULTIPLE_ATTRIBUTE called name that the OPT
// names with no children and no lower bound: the attribute is optional.
func optOptionalMultiple(name string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality></attributes>`
}

// optTemplate is an OPT whose definition is an archetype root of rmType.
func optTemplate(rmType string, attrs ...string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>rm_defaults</value></template_id><concept>rm_defaults</concept>
<definition><rm_type_name>` + rmType + `</rm_type_name><node_id>at0000</node_id>` + strings.Join(attrs, "") +
		`<archetype_id><value>openEHR-EHR-` + rmType + `.rm_defaults.v1</value></archetype_id></definition>
</template>`
}

// compileOPTText compiles xml, with or without the implicit RM
// attributes the compile step adds for every mandatory field the OPT
// leaves out.
func compileOPTText(t *testing.T, xml string, implicit bool) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseOPT(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	var opts []templatecompile.Option
	if !implicit {
		opts = append(opts, templatecompile.WithoutImplicitAttributes())
	}
	c, err := templatecompile.Compile(opt, opts...)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

// nodeID is the archetype_node_id of a locatable, or "" for any other value.
func nodeID(v any) string {
	if l, ok := v.(rm.Locatable); ok {
		return l.GetArchetypeNodeID()
	}
	return ""
}

var defaultsNow = time.Date(2021, 3, 4, 5, 6, 7, 0, time.UTC)

// entryAttrs are the ENTRY attributes an ACTION or INSTRUCTION case names
// without children, so the generator's implicit fills supply them.
var entryAttrs = []string{optSingle("language"), optSingle("encoding"), optSingle("subject")}

// emptyTree is an ITEM_TREE the OPT names with no attributes. Its items
// list is optional in the RM, so the generator gives it no member.
var emptyTree = optNode("ITEM_TREE", "at0001")

// TestREQ107_RMDefaultsFillOPTSilentFields is the REQ-107 check that
// "where the OPT leaves an RM attribute open, the generator MUST fill in
// RM-valid defaults". Each case builds an OPT in which nothing upstream
// of the default fills the field, asserts the field the default writes,
// and checks the generated value against the RM floor.
func TestREQ107_RMDefaultsFillOPTSilentFields(t *testing.T) {
	cases := []struct {
		name     string
		opt      string
		implicit bool
		check    func(t *testing.T, out any)
	}{
		{
			name: "ACTION time",
			opt: optTemplate("ACTION", append(entryAttrs,
				optSingle("ism_transition", optNode("ISM_TRANSITION", "")),
				optSingle("description", emptyTree))...),
			check: func(t *testing.T, out any) {
				a := out.(*rm.Action)
				if want := defaultsNow.Format(time.RFC3339); a.Time.Value != want {
					t.Errorf("ACTION.time = %q, want %q", a.Time.Value, want)
				}
			},
		},
		{
			name: "ISM_TRANSITION current_state",
			opt: optTemplate("ACTION", append(entryAttrs,
				optSingle("ism_transition", optNode("ISM_TRANSITION", "")),
				optSingle("description", emptyTree))...),
			check: func(t *testing.T, out any) {
				cs := out.(*rm.Action).IsmTransition.CurrentState
				if cs.DefiningCode.TerminologyID.Value != "openehr" || cs.DefiningCode.CodeString != "524" || cs.Value != "initial" {
					t.Errorf("ISM_TRANSITION.current_state = %+v, want openehr::524|initial|", cs)
				}
			},
		},
		{
			// ITEM_TREE.items is optional in the RM and the OPT does not
			// name it, so the tree gets no member.
			name: "ITEM_TREE items stay empty",
			opt: optTemplate("ACTION", append(entryAttrs,
				optSingle("ism_transition", optNode("ISM_TRANSITION", "")),
				optSingle("description", emptyTree))...),
			check: func(t *testing.T, out any) {
				tree, _ := out.(*rm.Action).Description.(*rm.ItemTree)
				if tree == nil || len(tree.Items) != 0 {
					t.Errorf("ACTION.description = %+v, want an ITEM_TREE with no items", out.(*rm.Action).Description)
				}
			},
		},
		{
			name: "CLUSTER items",
			opt:  optTemplate("CLUSTER"),
			check: func(t *testing.T, out any) {
				c := out.(*rm.Cluster)
				if len(c.Items) != 1 || nodeID(c.Items[0]) != "at0000" {
					t.Errorf("CLUSTER.items = %+v, want one at0000 item", c.Items)
				}
			},
		},
		{
			// ITEM_LIST.items is optional in the RM and the OPT does not
			// name it, so the list gets no member.
			name: "ITEM_LIST items stay empty",
			opt:  optTemplate("ITEM_LIST"),
			check: func(t *testing.T, out any) {
				if l := out.(*rm.ItemList); len(l.Items) != 0 {
					t.Errorf("ITEM_LIST.items = %+v, want none", l.Items)
				}
			},
		},
		{
			// The compiled ACTIVITY carries no action_archetype_id, a BMM
			// String the generator fills as an attribute the OPT leaves
			// silent, so RM Action_archetype_id_valid (not empty) holds.
			name: "ACTIVITY action_archetype_id",
			opt:  optTemplate("ACTIVITY", optSingle("description", emptyTree)),
			check: func(t *testing.T, out any) {
				if got := out.(*rm.Activity).ActionArchetypeID; got == "" {
					t.Errorf("ACTIVITY.action_archetype_id is empty, want a value")
				}
			},
		},
		{
			name: "PARTY_RELATIONSHIP source and target",
			opt:  optTemplate("PARTY_RELATIONSHIP"),
			check: func(t *testing.T, out any) {
				rel := out.(*rm.PartyRelationship)
				for side, ref := range map[string]rm.PartyRef{"source": rel.Source, "target": rel.Target} {
					if ref.ID == nil || ref.Namespace == "" || ref.Type == "" {
						t.Errorf("PARTY_RELATIONSHIP.%s = %+v, want id, namespace and type", side, ref)
					}
				}
			},
		},
		{
			name: "DV_EHR_URI value",
			opt:  optTemplate("DV_EHR_URI"),
			check: func(t *testing.T, out any) {
				if got := out.(*rm.DVEHRURI).Value; !strings.HasPrefix(got, "ehr:") {
					t.Errorf("DV_EHR_URI.value = %q, want the ehr scheme", got)
				}
			},
		},
		{
			name: "DV_ORDINAL symbol",
			opt:  optTemplate("DV_ORDINAL"),
			check: func(t *testing.T, out any) {
				if s := out.(*rm.DVOrdinal).Symbol; s.DefiningCode.CodeString == "" || s.Value == "" {
					t.Errorf("DV_ORDINAL.symbol = %+v, want a coded symbol", s)
				}
			},
		},
		{
			name: "DV_SCALE symbol",
			opt:  optTemplate("DV_SCALE"),
			check: func(t *testing.T, out any) {
				if s := out.(*rm.DVScale).Symbol; s.DefiningCode.CodeString == "" || s.Value == "" {
					t.Errorf("DV_SCALE.symbol = %+v, want a coded symbol", s)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, tc.opt, tc.implicit)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
				if err != nil {
					t.Fatalf("Generate(%v): %v", policy, err)
				}
				tc.check(t, out)
				for _, iss := range validation.ValidateRM(out).Issues {
					if iss.Severity == validation.Error {
						t.Errorf("Generate(%v): ValidateRM: %s @ %s: %s", policy, iss.Code, iss.Path, iss.Detail)
					}
				}
			}
		})
	}
}

// TestREQ107_RMDefaultsFillBMMSynthesisedValues is the REQ-107 check of
// the defaults the generator writes on values it builds from the BMM
// alone, and on values the OPT names but leaves unconstrained. Each case
// asserts the field the default writes and checks the generated value
// against the RM floor, under both policies and both value fills.
func TestREQ107_RMDefaultsFillBMMSynthesisedValues(t *testing.T) {
	cases := []struct {
		name  string
		opt   string
		check func(t *testing.T, out any)
	}{
		{
			// An OBSERVATION the OPT pins by archetype and leaves
			// otherwise open: the generator fills its ENTRY codes from
			// the BMM.
			name: "ENTRY language and encoding of a pinned OBSERVATION",
			opt:  pinnedObservationOPT,
			check: func(t *testing.T, out any) {
				obs := pinnedObservation(t, out)
				if obs.Language.TerminologyID.Value != "ISO_639-1" || obs.Language.CodeString != "en" {
					t.Errorf("OBSERVATION.language = %+v, want ISO_639-1::en", obs.Language)
				}
				if obs.Encoding.TerminologyID.Value != "IANA_character-sets" || obs.Encoding.CodeString != "UTF-8" {
					t.Errorf("OBSERVATION.encoding = %+v, want IANA_character-sets::UTF-8", obs.Encoding)
				}
			},
		},
		{
			// CLUSTER.items named with no children: the generator builds
			// an ELEMENT from the BMM and stamps the identity of the
			// locatable.
			name: "BMM-built locatable identity",
			opt:  optTemplate("CLUSTER", optMultiple("items")),
			check: func(t *testing.T, out any) {
				c := out.(*rm.Cluster)
				if len(c.Items) != 1 || nodeID(c.Items[0]) != "at0000" || c.Items[0].(rm.Locatable).GetName() == nil {
					t.Errorf("CLUSTER.items = %+v, want one item with archetype_node_id at0000 and a name", c.Items)
				}
			},
		},
		{
			// COMPOSITION.content is optional in the RM. With the OPT
			// silent on it the generator builds no entry: one built from
			// the BMM alone would be an archetype root with no archetype.
			name: "OPT-silent optional content stays empty",
			opt:  optTemplate("COMPOSITION", optOptionalMultiple("content")),
			check: func(t *testing.T, out any) {
				comp, err := instance.AsComposition(out)
				if err != nil {
					t.Fatalf("AsComposition: %v", err)
				}
				if len(comp.Content) != 0 {
					t.Errorf("COMPOSITION.content = %+v, want none", comp.Content)
				}
			},
		},
		{
			name: "locatable without a node id",
			opt:  optTemplate("CLUSTER", optMultiple("items", optNode("ELEMENT", ""))),
			check: func(t *testing.T, out any) {
				c := out.(*rm.Cluster)
				if len(c.Items) != 1 || nodeID(c.Items[0]) != "at0000" {
					t.Errorf("CLUSTER.items = %+v, want one item with archetype_node_id at0000", c.Items)
				}
			},
		},
		{
			name: "DV_DATE_TIME value from the clock",
			opt:  optTemplate("DV_DATE_TIME"),
			check: func(t *testing.T, out any) {
				if got, want := out.(*rm.DVDateTime).Value, defaultsNow.Format(time.RFC3339); got != want {
					t.Errorf("DV_DATE_TIME.value = %q, want %q", got, want)
				}
			},
		},
		{
			// REQ-107: a temporal root's value comes from the string pass,
			// which must write a valid ISO 8601 date, not the open-string
			// sentinel (RM Value_valid).
			name: "DV_DATE value is a valid date",
			opt:  optTemplate("DV_DATE"),
			check: func(t *testing.T, out any) {
				if got, want := out.(*rm.DVDate).Value, "2020-01-01"; got != want {
					t.Errorf("DV_DATE.value = %q, want %q", got, want)
				}
			},
		},
		{
			name: "DV_TIME value is a valid time",
			opt:  optTemplate("DV_TIME"),
			check: func(t *testing.T, out any) {
				if got, want := out.(*rm.DVTime).Value, "12:00:00"; got != want {
					t.Errorf("DV_TIME.value = %q, want %q", got, want)
				}
			},
		},
		{
			name: "DV_DURATION value is a valid duration",
			opt:  optTemplate("DV_DURATION"),
			check: func(t *testing.T, out any) {
				if got, want := out.(*rm.DVDuration).Value, "P0D"; got != want {
					t.Errorf("DV_DURATION.value = %q, want %q", got, want)
				}
			},
		},
		{
			name: "DV_EHR_URI under ELEMENT keeps the ehr scheme",
			opt:  optTemplate("ELEMENT", optSingle("value", optNode("DV_EHR_URI", ""))),
			check: func(t *testing.T, out any) {
				uri, _ := out.(*rm.Element).Value.(*rm.DVEHRURI)
				if uri == nil || !strings.HasPrefix(uri.Value, "ehr:") {
					t.Errorf("ELEMENT.value = %+v, want a DV_EHR_URI with the ehr scheme", out.(*rm.Element).Value)
				}
			},
		},
		{
			name: "DV_PROPORTION numerator, denominator, precision and accuracy",
			opt: optTemplate("ELEMENT", optSingle("value", optNode("DV_PROPORTION", "",
				optSingle("numerator", optPrimitive("REAL", "C_REAL", "<list>3</list>")),
				optSingle("denominator", optPrimitive("REAL", "C_REAL", "<list>4</list>")),
				optSingle("type", optPrimitive("INTEGER", "C_INTEGER", "<list>0</list>")),
				optSingle("precision", optPrimitive("INTEGER", "C_INTEGER", "<list>2</list>")),
				optSingle("accuracy", optPrimitive("REAL", "C_REAL", "<list>0.5</list>"))))),
			check: func(t *testing.T, out any) {
				p, _ := out.(*rm.Element).Value.(*rm.DVProportion)
				if p == nil {
					t.Fatalf("ELEMENT.value = %T, want *rm.DVProportion", out.(*rm.Element).Value)
				}
				if p.Numerator != 3 || p.Denominator != 4 {
					t.Errorf("DV_PROPORTION = %v/%v, want the pinned 3/4", p.Numerator, p.Denominator)
				}
				if p.Precision == nil || *p.Precision != 2 {
					t.Errorf("DV_PROPORTION.precision = %v, want 2", p.Precision)
				}
				if p.Accuracy == nil || *p.Accuracy != 0.5 {
					t.Errorf("DV_PROPORTION.accuracy = %v, want 0.5", p.Accuracy)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, tc.opt, true)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
					t.Run(policy.String()+"/"+fill.String(), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, instance.Options{
							Policy:    policy,
							ValueFill: fill,
							Language:  "en",
							Territory: "NL",
							Composer:  testComposer(),
							Now:       defaultsNow,
						})
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						tc.check(t, out)
						for _, iss := range validation.ValidateRM(out).Issues {
							if iss.Severity == validation.Error {
								t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
							}
						}
					})
				}
			}
		})
	}
}

// A COMPOSITION category that the OPT pins to a code of the openEHR
// "composition category" group carries that code's pinned rubric as its
// text, not the synthesiser's placeholder. A code outside the group, or a
// group code in another terminology, keeps the text the walk gave it: the
// generator invents no rubric for it.
func TestREQ034_REQ107_PinnedCategoryCarriesItsRubric(t *testing.T) {
	cases := []struct {
		name          string
		terminologyID string
		code          string
		wantRubric    bool
	}{
		{name: "event", terminologyID: terminology.ID, code: "433", wantRubric: true},
		{name: "persistent", terminologyID: terminology.ID, code: "431", wantRubric: true},
		{name: "openehr code outside the group", terminologyID: terminology.ID, code: "999"},
		{name: "group code in another terminology", terminologyID: "local", code: "433"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, optTemplate("COMPOSITION", optSingle("category", optCodedText(tc.terminologyID, tc.code))), true)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
					t.Run(policy.String()+"/"+fill.String(), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, instance.Options{
							Policy:    policy,
							ValueFill: fill,
							Language:  "en",
							Territory: "NL",
							Composer:  testComposer(),
							Now:       defaultsNow,
						})
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						comp, err := instance.AsComposition(out)
						if err != nil {
							t.Fatalf("AsComposition: %v", err)
						}
						got := comp.Category
						if got.DefiningCode.TerminologyID.Value != tc.terminologyID || got.DefiningCode.CodeString != tc.code {
							t.Fatalf("category pinned to %s::%s generated code %s::%s",
								tc.terminologyID, tc.code, got.DefiningCode.TerminologyID.Value, got.DefiningCode.CodeString)
						}
						if tc.wantRubric {
							want, _ := terminology.CompositionCategory.Rubric(tc.code)
							if got.Value != want {
								t.Errorf("category %s::%s value = %q, want the pin's rubric %q", tc.terminologyID, tc.code, got.Value, want)
							}
							return
						}
						if got.Value != walkText {
							t.Errorf("category %s::%s value = %q, want the walk's text %q, not an invented rubric",
								tc.terminologyID, tc.code, got.Value, walkText)
						}
					})
				}
			}
		})
	}
}

// pinnedObservationOPT is a COMPOSITION whose content is one OBSERVATION
// the OPT pins by archetype, with its history pinned and nothing else.
var pinnedObservationOPT = optTemplate("COMPOSITION", optMultiple("content",
	optArchetypeRoot("OBSERVATION", "openEHR-EHR-OBSERVATION.rm_defaults.v1",
		optSingle("data", optNode("HISTORY", "at0001")))))

// TestREQ107_ImplicitEntryStructureCarriesLocatableIdentity is the REQ-107
// check that a structure the generator builds from the BMM alone, for a
// mandatory single the OPT leaves silent (OBSERVATION.data, EVALUATION.data,
// ACTION.description, ADMIN_ENTRY.data), carries the archetype_node_id and
// name the RM floor requires, under both policies and both value fills.
func TestREQ107_ImplicitEntryStructureCarriesLocatableIdentity(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// structure returns the implicit single under test.
		structure func(t *testing.T, out any) any
	}{
		{
			name:      "OBSERVATION.data of a pinned COMPOSITION entry",
			opt:       dataSilentObservationOPT,
			structure: func(t *testing.T, out any) any { return &pinnedObservation(t, out).Data },
		},
		{
			name: "EVALUATION.data of a pinned COMPOSITION entry",
			opt: optTemplate("COMPOSITION", optMultiple("content",
				optArchetypeRoot("EVALUATION", "openEHR-EHR-EVALUATION.rm_defaults.v1"))),
			structure: func(t *testing.T, out any) any {
				comp, err := instance.AsComposition(out)
				if err != nil {
					t.Fatalf("AsComposition: %v", err)
				}
				if len(comp.Content) != 1 {
					t.Fatalf("COMPOSITION.content has %d items, want 1", len(comp.Content))
				}
				ev, ok := comp.Content[0].(*rm.Evaluation)
				if !ok {
					t.Fatalf("COMPOSITION.content[0] is %T, want *rm.Evaluation", comp.Content[0])
				}
				return ev.Data
			},
		},
		{
			name:      "OBSERVATION root data",
			opt:       optTemplate("OBSERVATION"),
			structure: func(_ *testing.T, out any) any { return &out.(*rm.Observation).Data },
		},
		{
			name:      "EVALUATION root data",
			opt:       optTemplate("EVALUATION"),
			structure: func(_ *testing.T, out any) any { return out.(*rm.Evaluation).Data },
		},
		{
			name:      "ACTION root description",
			opt:       optTemplate("ACTION"),
			structure: func(_ *testing.T, out any) any { return out.(*rm.Action).Description },
		},
		{
			name:      "ADMIN_ENTRY root data",
			opt:       optTemplate("ADMIN_ENTRY"),
			structure: func(_ *testing.T, out any) any { return out.(*rm.AdminEntry).Data },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, tc.opt, true)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
					t.Run(policy.String()+"/"+fill.String(), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, instance.Options{
							Policy:    policy,
							ValueFill: fill,
							Language:  "en",
							Territory: "NL",
							Composer:  testComposer(),
							Now:       defaultsNow,
						})
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						s := tc.structure(t, out)
						loc, ok := s.(rm.Locatable)
						if !ok || rm.IsTypedNil(s) {
							t.Fatalf("implicit structure is %T, want a locatable", s)
						}
						if got := loc.GetArchetypeNodeID(); got != "at0000" {
							t.Errorf("%T archetype_node_id = %q, want at0000", s, got)
						}
						if name := loc.GetName(); name == nil || rm.IsTypedNil(name) || name.GetValue() == "" {
							t.Errorf("%T name = %v, want a non-empty name", s, name)
						}
						for _, iss := range validation.ValidateRM(out).Issues {
							if iss.Severity == validation.Error {
								t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
							}
						}
					})
				}
			}
		})
	}
}

// dataSilentObservationOPT is a COMPOSITION whose content is one OBSERVATION
// the OPT pins by archetype, with its mandatory data left silent.
var dataSilentObservationOPT = optTemplate("COMPOSITION", optMultiple("content",
	optArchetypeRoot("OBSERVATION", "openEHR-EHR-OBSERVATION.rm_defaults.v1")))

// pinnedObservation returns the one OBSERVATION in COMPOSITION.content.
func pinnedObservation(t *testing.T, out any) *rm.Observation {
	t.Helper()
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}
	if len(comp.Content) != 1 {
		t.Fatalf("COMPOSITION.content has %d items, want 1", len(comp.Content))
	}
	obs, ok := comp.Content[0].(*rm.Observation)
	if !ok {
		t.Fatalf("COMPOSITION.content[0] is %T, want *rm.Observation", comp.Content[0])
	}
	return obs
}

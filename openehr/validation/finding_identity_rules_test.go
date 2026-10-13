package validation_test

// finding_identity_rules_test.go: REQ-171 § Finding identity, rule by rule.
// Each row plants one defect and names the issue it must draw, with its
// layer and its rule: every rule the walks can set is asserted on at least
// one issue here, or, for the coded invariants, by the coded rows of
// rmfloor_coded_test.go. The vendored constraint fixtures give the primitive
// rules; a small CLUSTER template gives the structural ones.

import (
	json "encoding/json/v2"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// fiInterval is an AOM interval body from lower to upper.
func fiInterval(lower, upper string) string {
	return `<lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>` + lower + `</lower><upper>` + upper + `</upper>`
}

// fiElement is an ELEMENT child at nodeID, occurring lower..upper times, whose
// value admits the given children; nameExistence, when set, names the
// ELEMENT's name attribute with that existence.
func fiElement(nodeID, lower, upper, nameExistence string, valueChildren ...string) string {
	name := ""
	if nameExistence != "" {
		name = `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>name</rm_attribute_name>` +
			`<existence>` + nameExistence + `</existence></attributes>`
	}
	var value strings.Builder
	value.WriteString(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>`)
	for _, rmType := range valueChildren {
		value.WriteString(`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name><node_id></node_id></children>`)
	}
	value.WriteString(`</attributes>`)
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name>` +
		`<occurrences>` + fiInterval(lower, upper) + `</occurrences>` +
		`<node_id>` + nodeID + `</node_id>` + name + value.String() + `</children>`
}

// findingIdentityArchetype is the archetype of the root of findingIdentityOPT.
const findingIdentityArchetype = "openEHR-EHR-CLUSTER.finding_identity.v1"

// findingIdentityOPT is a CLUSTER whose items the template requires
// (existence 1..1) and holds to 1..3 items: an ELEMENT at0001 whose value is
// a DV_TEXT, an ELEMENT at0002 whose value is a DV_TEXT or a DV_QUANTITY,
// both optional, and an ELEMENT at0003 that occurs exactly once and whose
// name the template requires itself. Every ELEMENT's name is RM-mandatory.
var findingIdentityOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>finding_identity</value></template_id><concept>finding_identity</concept>
<definition><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>
<existence>` + fiInterval("1", "1") + `</existence>` +
	fiElement("at0001", "0", "1", "", "DV_TEXT") +
	fiElement("at0002", "0", "1", "", "DV_TEXT", "DV_QUANTITY") +
	fiElement("at0003", "1", "1", fiInterval("1", "1"), "DV_TEXT") + `
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` + fiInterval("1", "3") + `</interval></cardinality>
</attributes>
<archetype_id><value>` + findingIdentityArchetype + `</value></archetype_id></definition>
</template>`

// fiItem is an ELEMENT at nodeID, named name, holding value.
func fiItem(nodeID, name string, value rm.DataValue) *rm.Element {
	return &rm.Element{ArchetypeNodeID: nodeID, Name: rm.DVText{Value: name}, Value: value}
}

// fiCluster is the root of findingIdentityOPT holding items.
func fiCluster(items ...rm.Item) *rm.Cluster {
	return &rm.Cluster{ArchetypeNodeID: findingIdentityArchetype, Name: &rm.DVText{Value: "cluster"}, Items: items}
}

// assertHasIdentities fails the test unless each of want, written as
// identityKeys writes an issue, is among the issues of r, and unless every
// issue of r has a layer and a rule that fit its code.
func assertHasIdentities(t *testing.T, call string, r validation.Result, want ...string) {
	t.Helper()
	got := identityKeys(r.Issues)
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("%s issues (code path layer rule) = %q, want one %q", call, got, w)
		}
	}
	assertIdentities(t, call, r, mustBMMInvariants(t))
}

// REQ-171 — each template-layer rule, on an issue of the code it explains:
// existence (an attribute the template requires itself), cardinality (the
// items an attribute holds), occurrences (the items bound to one child), type
// (one child or several alternatives), slot, identity, and, at a primitive
// leaf, the violation's own code. An absent attribute only the RM makes
// mandatory is on the RM layer; one the template requires itself is on the
// template layer, the RM's own requirement notwithstanding.
func TestValidate_REQ171_TemplateLayerRules(t *testing.T) {
	t.Parallel()
	c := mustCompileInline(t, findingIdentityOPT)
	text := &rm.DVText{Value: "text"}
	count := &rm.DVCount{Magnitude: 1}
	if r := validation.Validate(fiCluster(fiItem("at0001", "one", text), fiItem("at0003", "three", text)), c); !r.OK {
		t.Fatalf("precondition: the valid CLUSTER draws %q", identityKeys(r.Issues))
	}
	tests := []struct {
		name string
		root *rm.Cluster
		want []string
	}{
		{
			name: "items absent",
			root: fiCluster(),
			want: []string{
				"required /items template existence",
				"cardinality /items template cardinality",
				"cardinality /items[at0003] template occurrences",
			},
		},
		{
			name: "four items, at0003 twice",
			root: fiCluster(fiItem("at0001", "one", text), fiItem("at0002", "two", text), fiItem("at0003", "three", text), fiItem("at0003", "four", text)),
			want: []string{
				"cardinality /items template cardinality",
				"cardinality /items[at0003] template occurrences",
			},
		},
		{
			name: "a DV_COUNT under the one DV_TEXT child",
			root: fiCluster(fiItem("at0001", "one", count), fiItem("at0003", "three", text)),
			want: []string{"rm_type_mismatch /items[at0001]/value template type"},
		},
		{
			name: "a DV_COUNT under the DV_TEXT and DV_QUANTITY alternatives",
			root: fiCluster(fiItem("at0002", "two", count), fiItem("at0003", "three", text)),
			want: []string{"alternative_mismatch /items[at0002]/value template type"},
		},
		{
			name: "an item no child takes",
			root: fiCluster(fiItem("at0099", "stray", text), fiItem("at0003", "three", text)),
			want: []string{"slot_fill /items[at0099] template slot"},
		},
		{
			name: "a root of another archetype",
			root: func() *rm.Cluster {
				r := fiCluster(fiItem("at0003", "three", text))
				r.ArchetypeNodeID = "openEHR-EHR-CLUSTER.other.v1"
				return r
			}(),
			want: []string{"archetype_id_mismatch /archetype_node_id template identity"},
		},
		{
			name: "a name the template requires itself",
			root: fiCluster(fiItem("at0003", "", text)),
			want: []string{"required /items[at0003]/name template existence"},
		},
		{
			name: "a name only the RM makes mandatory",
			root: fiCluster(fiItem("at0001", "", text), fiItem("at0003", "three", text)),
			want: []string{"required /items[at0001]/name rm mandatory"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertHasIdentities(t, "Validate("+tc.name+")", validation.Validate(tc.root, c), tc.want...)
		})
	}

	t.Run("a single-valued slot fill the slot does not admit", func(t *testing.T) {
		t.Parallel()
		comp := validSingleAttributeSlotComposition("openEHR-EHR-ITEM_TREE.rejected.v1")
		assertHasIdentities(t, "ValidateComposition(protocol slot holding ITEM_TREE.rejected)", validation.ValidateComposition(comp, mustCompileSyntheticOPT(t, singleAttributeSlotOPT)),
			"slot_fill /content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/protocol template slot")
	})

	for _, tc := range []struct {
		name, languages, want string
	}{
		{name: "an item that is not a LOCATABLE under one child it does not fit", languages: languagesChild("DV_CODED_TEXT"), want: "rm_type_mismatch /languages[1] template type"},
		{
			name:      "an item that is not a LOCATABLE under two alternatives it does not fit",
			languages: languagesChild("DV_CODED_TEXT") + languagesChild("DV_CODED_TEXT"),
			want:      "alternative_mismatch /languages[1] template type",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertHasIdentities(t, "ValidateDemographic(ACTOR with a DV_TEXT language)", validation.ValidateDemographic(validActor(), mustCompileInline(t, actorOPT(tc.languages))), tc.want)
		})
	}

	t.Run("a node id the template does not pin", func(t *testing.T) {
		t.Parallel()
		comp := validVitalSignsComposition()
		obs, ok := comp.Content[0].(*rm.Observation)
		if !ok {
			t.Fatalf("fixture content[0] is %T, want *rm.Observation", comp.Content[0])
		}
		obs.Data.ArchetypeNodeID = "at9999"
		assertHasIdentities(t, "ValidateComposition(vital_signs, data at9999)", validation.ValidateComposition(comp, mustCompile(t, "vital_signs")),
			"node_id_mismatch /content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/archetype_node_id template identity")
	})

	for _, tc := range []struct {
		template string
		want     string
	}{
		{
			template: "Test_dv_count_range_constraint.v0",
			want:     "primitive_out_of_range /content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[1]/data/items[at0042]/value/magnitude template out_of_range",
		},
		{
			template: "Test_dv_multimedia_open_constraint.v0",
			want:     "primitive_not_in_list /content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[1]/data/items[at0028]/value/media_type template not_in_list",
		},
	} {
		t.Run(tc.template, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(fixtures.CompositionJSON(tc.template))
			if err != nil {
				t.Fatal(err)
			}
			var comp rm.Composition
			if err := canjson.Unmarshal(raw, &comp); err != nil {
				t.Fatalf("decode composition: %v", err)
			}
			assertHasIdentities(t, "ValidateComposition("+tc.template+")", validation.ValidateComposition(&comp, mustCompile(t, tc.template)), tc.want)
		})
	}
}

// bareClusterOPT is a CLUSTER template that declares no attribute: the
// compiled node carries the attributes the RM makes mandatory on a CLUSTER,
// items among them, and nothing of the template's own.
const bareClusterOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>bare_cluster</value></template_id><concept>bare_cluster</concept>
<definition><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id>
<archetype_id><value>openEHR-EHR-CLUSTER.bare.v1</value></archetype_id></definition>
</template>`

// paragraphChild is an ELEMENT value child DV_PARAGRAPH with no attribute:
// the compiled node carries DV_PARAGRAPH's RM-mandatory items, which the
// reader for DV_PARAGRAPH does not read.
const paragraphChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_PARAGRAPH</rm_type_name>
        <node_id></node_id>
      </children>`

// REQ-171 § Finding identity — the template walk's `required` for a
// multi-valued attribute names the rule that requires it. One the template's
// own existence requires and the instance holds empty is template
// existence. One only the RM makes mandatory and the instance holds empty is
// RM cardinality, the rule the floor's `cardinality` at the same path also
// names. One the reader for its parent's class cannot address at all is
// absent as far as the walk sees, so it is RM mandatory: a DV_PARAGRAPH's
// items, though the instance carries them (REQ-110 § Known limitations).
func TestValidate_REQ171_EmptyMandatoryListRule(t *testing.T) {
	t.Parallel()
	empty := &rm.Cluster{ArchetypeNodeID: "openEHR-EHR-CLUSTER.bare.v1", Name: &rm.DVText{Value: "cluster"}, Items: []rm.Item{}}
	paragraph := valueElement(&rm.DVParagraph{Items: []rm.DVTextLike{&rm.DVText{Value: "line"}}})
	tests := []struct {
		name string
		r    validation.Result
		want []string // the whole result, as identityKeys writes it
	}{
		{
			name: "empty items only the RM makes mandatory",
			r:    validation.Validate(empty, mustCompileInline(t, bareClusterOPT)),
			want: []string{"cardinality /items rm cardinality", "required /items rm cardinality"},
		},
		{
			name: "empty items the template requires",
			r: validation.Validate(func() *rm.Cluster {
				c := fiCluster()
				c.Items = []rm.Item{}
				return c
			}(), mustCompileInline(t, findingIdentityOPT)),
			// The floor's cardinality at /items repeats the template
			// walk's code at that path, so it is carried once.
			want: []string{
				"cardinality /items template cardinality",
				"cardinality /items[at0003] template occurrences",
				"required /items template existence",
			},
		},
		{
			name: "items the reader cannot address",
			r:    validation.Validate(paragraph, mustCompileInline(t, elementValueOPT(paragraphChild))),
			want: []string{"required /value/items rm mandatory"},
		},
	}
	for _, tc := range tests {
		if got := identityKeys(tc.r.Issues); !slices.Equal(got, tc.want) {
			t.Errorf("Validate(%s) issues (code path layer rule) = %q, want %q", tc.name, got, tc.want)
		}
		assertIdentities(t, "Validate("+tc.name+")", tc.r, mustBMMInvariants(t))
	}
}

// REQ-171 — each RM-layer rule the floor evaluates outside the coded
// invariants, on the issue it draws: a rule the BMM names as CLASS.Invariant,
// and the three the BMM states in an attribute's definition (mandatory,
// cardinality, precision_range). The rule-specific codes carry their rule's
// name; rm_invariant tells its rules apart by Rule alone.
func TestValidateRM_REQ171_RMLayerRules(t *testing.T) {
	t.Parallel()
	minus2 := rm.Integer(-2)
	zero := rm.Integer(0)
	openEHR271 := &rm.DVCodedText{Value: "no information", DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "271"}}
	interval := func(lower, upper int64) *rm.DVInterval[rm.DVCount] {
		return &rm.DVInterval[rm.DVCount]{
			Lower: rm.DVCount{Magnitude: lower}, LowerIncluded: true,
			Upper: rm.DVCount{Magnitude: upper}, UpperIncluded: true,
		}
	}
	tests := []struct {
		name string
		root any
		want []string
	}{
		{name: "FOLDER without a name", root: &rm.Folder{ArchetypeNodeID: "at0001"}, want: []string{"required /name rm mandatory"}},
		{
			name: "CLUSTER with an empty items list",
			root: &rm.Cluster{ArchetypeNodeID: "at0001", Name: &rm.DVText{Value: "c"}, Items: []rm.Item{}},
			want: []string{"cardinality /items rm cardinality"},
		},
		{
			name: "ARCHETYPED with an empty archetype id",
			root: &rm.Archetyped{RMVersion: "1.1.0"},
			want: []string{"required /archetype_id/value rm mandatory"},
		},
		{
			name: "ARCHETYPED with an empty rm_version",
			root: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-CLUSTER.c.v1"}},
			want: []string{"rm_version_valid /rm_version rm ARCHETYPED.Rm_version_valid"},
		},
		{
			name: "CODE_PHRASE with a blank code_string",
			root: &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}},
			want: []string{"rm_invariant / rm CODE_PHRASE.Code_string_valid"},
		},
		{
			name: "DV_QUANTITY precision -2",
			root: &rm.DVQuantity{Magnitude: 1, Units: "kg", Precision: &minus2},
			want: []string{"rm_invariant / rm precision_range"},
		},
		{
			name: "DV_PROPORTION precision -2",
			root: &rm.DVProportion{Numerator: 1, Denominator: 2, Precision: &minus2},
			want: []string{"rm_invariant / rm precision_range"},
		},
		{
			name: "DV_PROPORTION precision 0 over a fractional numerator",
			root: &rm.DVProportion{Numerator: 1.5, Denominator: 2, Precision: &zero},
			want: []string{"rm_invariant / rm DV_PROPORTION.Precision_validity"},
		},
		{name: "DV_DATE not ISO 8601", root: &rm.DVDate{Value: "example"}, want: []string{"rm_invariant / rm DV_DATE.Value_valid"}},
		{name: "DV_TIME not ISO 8601", root: &rm.DVTime{Value: "example"}, want: []string{"rm_invariant / rm DV_TIME.Value_valid"}},
		{name: "DV_DATE_TIME not ISO 8601", root: &rm.DVDateTime{Value: "example"}, want: []string{"rm_invariant / rm DV_DATE_TIME.Value_valid"}},
		{name: "DV_DURATION not ISO 8601", root: &rm.DVDuration{Value: "example"}, want: []string{"rm_invariant / rm DV_DURATION.Value_valid"}},
		{
			name: "ELEMENT with neither value nor null_flavour",
			root: &rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "e"}},
			want: []string{"rm_invariant / rm ELEMENT.Inv_null_flavour_indicated"},
		},
		{
			name: "ELEMENT with both value and null_flavour",
			root: &rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "e"}, Value: &rm.DVText{Value: "v"}, NullFlavour: openEHR271},
			want: []string{"rm_invariant / rm ELEMENT.Inv_null_flavour_indicated"},
		},
		{
			name: "DV_INTERVAL with its lower bound above its upper",
			root: interval(5, 1),
			want: []string{"rm_invariant / rm DV_INTERVAL.Limits_consistent"},
		},
		{
			name: "DV_INTERVAL with both sides unbounded and included",
			root: func() any {
				iv := interval(1, 5)
				iv.LowerUnbounded, iv.UpperUnbounded = true, true
				return iv
			}(),
			want: []string{
				"lower_included_valid / rm Interval.Lower_included_valid",
				"upper_included_valid / rm Interval.Upper_included_valid",
			},
		},
		{
			name: "PARTY_REF without id, type or namespace",
			root: &rm.PartyRef{},
			want: []string{"rm_invariant /id rm mandatory", "rm_invariant /type rm mandatory", "rm_invariant /namespace rm mandatory"},
		},
		{
			name: "PARTY_IDENTIFIED with nothing",
			root: &rm.PartyIdentified{},
			want: []string{"rm_invariant / rm PARTY_IDENTIFIED.Basic_validity"},
		},
		{
			name: "PARTY_IDENTIFIED with an empty name and empty identifiers",
			root: &rm.PartyIdentified{Name: new(""), Identifiers: []rm.DVIdentifier{}},
			want: []string{
				"rm_invariant /name rm PARTY_IDENTIFIED.Name_valid",
				"rm_invariant /identifiers rm PARTY_IDENTIFIED.Identifiers_valid",
			},
		},
		{
			name: "DV_TEXT with present but empty mappings",
			root: &rm.DVText{Value: "t", Mappings: []rm.TermMapping{}},
			want: []string{"mappings_valid /mappings rm DV_TEXT.Mappings_valid"},
		},
		{
			name: "TERM_MAPPING with a match outside the set",
			root: &rm.TermMapping{Match: rm.Character("x"), Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "271649006"}},
			want: []string{"term_mapping_match /match rm TERM_MAPPING.Match_valid"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertHasIdentities(t, "ValidateRM("+tc.name+")", validation.ValidateRM(tc.root), tc.want...)
		})
	}
}

// REQ-171 — the presence-aware EHR_STATUS entry reports a value-typed
// RM-mandatory attribute whose key is absent as required on the RM layer, by
// the rule mandatory.
func TestValidateRMEHRStatusBytes_REQ171_RMLayerRules(t *testing.T) {
	t.Parallel()
	body := []byte(`{
	"_type": "EHR_STATUS",
	"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
	"name": {"_type": "DV_TEXT", "value": "s"},
	"archetype_details": {"archetype_id": {"value": "openEHR-EHR-EHR_STATUS.generic.v1"}},
	"is_queryable": true,
	"is_modifiable": true
}`)
	assertHasIdentities(t, "ValidateRMEHRStatusBytes", validation.ValidateRMEHRStatusBytes(body),
		"required /subject rm mandatory",
		"required /archetype_details/rm_version rm mandatory")
}

// REQ-171 — is_archetype_root names, as its rule, the class that declares
// Is_archetype_root in the pinned RM BMM for the node's class: the class
// itself, or the ancestor (PARTY, ENTRY) that declares it. Every concrete
// archetype-root class of the BMM is checked, built from the type registry
// with no archetype_details.
func TestValidateRM_REQ171_IsArchetypeRootNamesTheDeclaringClass(t *testing.T) {
	t.Parallel()
	declarers, err := archetypeRootDeclarers()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{
		"ACTION", "ADMIN_ENTRY", "AGENT", "COMPOSITION", "EHR_ACCESS", "EHR_STATUS", "EVALUATION",
		"GROUP", "INSTRUCTION", "OBSERVATION", "ORGANISATION", "PERSON", "ROLE",
	}; !slices.Equal(slices.Sorted(maps.Keys(declarers)), want) {
		t.Fatalf("BMM concrete archetype-root classes = %v, want %v", slices.Sorted(maps.Keys(declarers)), want)
	}
	for class, declarer := range declarers {
		ctor, ok := typereg.Default.Lookup(class)
		if !ok {
			t.Errorf("archetype root %s has no registered Go type", class)
			continue
		}
		r := validation.ValidateRM(ctor())
		want := "is_archetype_root /archetype_details rm " + declarer + ".Is_archetype_root"
		if got := identityKeys(r.Issues); !slices.Contains(got, want) {
			t.Errorf("ValidateRM(%s without archetype_details) issues = %q, want one %q", class, got, want)
		}
	}
}

// archetypeRootDeclarers reads the pinned RM BMM and returns, for each
// concrete class that is an archetype root, the class that declares the
// Is_archetype_root invariant it is bound by: itself or its nearest ancestor
// that declares it.
func archetypeRootDeclarers() (map[string]string, error) {
	raw, err := os.ReadFile("../../resources/bmm/openehr_rm_1.2.0.bmm.json")
	if err != nil {
		return nil, err
	}
	var schema struct {
		ClassDefinitions map[string]struct {
			Ancestors  []string       `json:"ancestors"`
			IsAbstract bool           `json:"is_abstract"`
			Invariants map[string]any `json:"invariants"`
		} `json:"class_definitions"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	defs := schema.ClassDefinitions
	var declarer func(name string, depth int) string
	declarer = func(name string, depth int) string {
		def, ok := defs[name]
		if !ok || depth > 32 {
			return ""
		}
		if _, declares := def.Invariants["Is_archetype_root"]; declares {
			return name
		}
		for _, a := range def.Ancestors {
			if d := declarer(a, depth+1); d != "" {
				return d
			}
		}
		return ""
	}
	out := map[string]string{}
	for name, def := range defs {
		if def.IsAbstract {
			continue
		}
		if d := declarer(name, 0); d != "" {
			out[name] = d
		}
	}
	return out, nil
}

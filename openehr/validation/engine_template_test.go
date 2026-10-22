package validation_test

// engine_template_test.go: REQ-171 § One walk, § Acceptance — the one walk
// with a template, through Validate. A template node the instance lacks
// draws one `required`, on the template layer; content the template does not
// admit draws its one admission finding and nothing from inside; an attribute
// the template does not mention is walked with the RM rules; a value held
// from two places is validated at each; an empty RM-mandatory container
// draws one finding, the template's where the template reports it and the
// RM's `cardinality` where only the RM requires it; an attribute the RM
// marks mandatory, and the part of a reference, are read as the RM rules
// read them; and the attributes the template declares are visited in its
// order.

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/validation"
)

// validateWith runs Validate over root with the compiled template c, failing
// the test when it has not returned within walkDeadline.
func validateWith(t *testing.T, root any, c *templatecompile.Compiled) validation.Result {
	t.Helper()
	return validateWithin(t, "Validate", walkDeadline, func() validation.Result { return validation.Validate(root, c) })
}

// keysAt returns, sorted, "code layer rule" for each issue at path.
func keysAt(issues []validation.Issue, path string) []string {
	var out []string
	for _, i := range issues {
		if i.Path == path {
			out = append(out, i.Code+" "+i.Layer.String()+" "+i.Rule)
		}
	}
	slices.Sort(out)
	return out
}

// bloodPressurePath is the path of the blood-pressure OBSERVATION of the
// vital_signs compositions.
const bloodPressurePath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]"

// personIdentitiesOPT returns a PERSON template whose identities, a list the
// RM marks mandatory, has the existence existence and a child count from
// countLower up, with no child declared.
func personIdentitiesOPT(existence, countLower string) string {
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>person-ident</value></template_id>
  <concept>person-ident</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>PERSON</rm_type_name>
    <node_id>at0000</node_id>
    <archetype_id><value>openEHR-DEMOGRAPHIC-PERSON.person.v1</value></archetype_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>identities</rm_attribute_name>
      <existence>` + existence + `</existence>
      <cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique>
        <interval><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>` + countLower + `</lower></interval></cardinality>
    </attributes>
  </definition>
</template>`
}

// missingNodeCase is a root that lacks a node at path, an attribute both
// the template c and the RM require.
type missingNodeCase struct {
	name string
	root any
	c    *templatecompile.Compiled
	path string
}

// missingNodeCases returns an OBSERVATION without its data under
// vital_signs.opt, and a PERSON without its identities under a template
// whose existence and child count require them.
func missingNodeCases(t *testing.T) []missingNodeCase {
	t.Helper()
	noData := validVitalSignsComposition()
	bpObservation(t, noData).Data = rm.History[rm.ItemStructure]{}
	return []missingNodeCase{
		{name: "an OBSERVATION without its data", root: noData, c: mustCompile(t, "vital_signs"), path: bloodPressurePath + "/data"},
		{name: "a PERSON without its identities", root: probes.PersonAbsentIdentities(), c: mustCompileInline(t, probes.PersonIdentitiesOPT), path: "/identities"},
	}
}

// REQ-171 § One walk (Presence), § Acceptance — a template node the instance
// lacks, at an attribute both the template and the RM require, draws one
// `required`, on the template layer, at the node's path, and no `required` on
// the RM layer there: the template's requirement implies the RM's. The RM
// alone reports the same absence on the RM layer, so the RM requirement is
// real.
func TestREQ171_EngineReportsAMissingTemplateNodeOnce(t *testing.T) {
	t.Parallel()
	for _, tc := range missingNodeCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			if got := keysAt(validation.ValidateRM(tc.root).Issues, tc.path); !slices.Equal(got, []string{"required rm mandatory"}) {
				t.Fatalf("precondition: ValidateRM(%s) at %s = %q, want the RM's required", tc.name, tc.path, got)
			}
			r := validateWith(t, tc.root, tc.c)
			if got, want := keysAt(r.Issues, tc.path), []string{"required template existence"}; !slices.Equal(got, want) {
				t.Errorf("Validate(%s) at %s = %q, want %q: one absence, one finding, on the template layer", tc.name, tc.path, got, want)
			}
		})
	}
}

// refusedContentCase is a root holding content the template c does not
// admit, with an RM breach inside it.
type refusedContentCase struct {
	name string
	root any
	c    *templatecompile.Compiled
	// want is the one finding, as identityKeys writes it; inside is the
	// breach inside, which the RM rules alone report.
	want, inside string
}

// compositionOPT returns a COMPOSITION template, root node at0000, that
// declares attributes, each an <attributes> element, and nothing else.
func compositionOPT(attributes ...string) string {
	return rootOPT("COMPOSITION", "", attributes...)
}

// rootOPT returns a template whose root node, of RM type rmType and node id
// at0000, pins the archetype id archetypeID when it is not empty and
// declares attributes, each an <attributes> element, and nothing else.
func rootOPT(rmType, archetypeID string, attributes ...string) string {
	archetype := ""
	if archetypeID != "" {
		archetype = `<archetype_id><value>` + archetypeID + `</value></archetype_id>`
	}
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>engine-` + strings.ToLower(rmType) + `</value></template_id>
  <concept>engine-` + strings.ToLower(rmType) + `</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>` + rmType + `</rm_type_name>
    <node_id>at0000</node_id>
    ` + strings.Join(attributes, "\n    ") + `
    ` + archetype + `
  </definition>
</template>`
}

// composerAttribute returns a composer attribute the template requires, with
// child, a <children> element, as its one alternative.
func composerAttribute(child string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>composer</rm_attribute_name>` +
		`<existence>` + fiInterval("1", "1") + `</existence>` + child + `</attributes>`
}

// singleAttribute returns a single-valued attribute name with the existence
// lower..upper and children, each a <children> element.
func singleAttribute(name, lower, upper string, children ...string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence>` + fiInterval(lower, upper) + `</existence>` + strings.Join(children, "") + `</attributes>`
}

// complexChild returns a C_COMPLEX_OBJECT child of RM type rmType, with no
// node id, that declares attributes.
func complexChild(rmType string, attributes ...string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name><node_id></node_id>` +
		strings.Join(attributes, "") + `</children>`
}

// forbiddenChildArchetype is the archetype of the root of forbiddenChildOPT.
const forbiddenChildArchetype = "openEHR-EHR-CLUSTER.forbidden_child.v1"

// forbiddenChildOPT is a CLUSTER whose items, open in existence and count,
// allow an ELEMENT at0001 and declare an ELEMENT at0002 that occurs 0..0,
// which the template does not allow. Each ELEMENT's value is a DV_TEXT.
var forbiddenChildOPT = rootOPT("CLUSTER", forbiddenChildArchetype,
	`<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>`+
		`<existence>`+fiInterval("0", "1")+`</existence>`+
		fiElement("at0001", "0", "1", "", "DV_TEXT")+
		fiElement("at0002", "0", "0", "", "DV_TEXT")+
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>`+unboundedFromZero+`</interval></cardinality>`+
		`</attributes>`)

// refusedContentCases returns content the template does not admit, one row
// for each kind the Admission step names, each carrying an RM breach inside:
// an OBSERVATION, an item of a list, whose archetype fits no slot of
// vital_signs.opt, lacking its language; an ELEMENT, an item of a list,
// bound to a child the template does not allow, whose value's mappings are
// present and empty; a PARTY_RELATED composer, the value of a single-valued
// attribute that binds to none of the allowed children, as the template
// allows a PARTY_SELF alone, coded outside its group; an ELEMENT under a
// template that prohibits its null_flavour and its value's mappings,
// carrying a null flavour whose mappings are present and empty, a mapping
// with no valid match, or a null flavour coded outside its group; and a
// composer whose name the template prohibits, present and empty. The last
// two carry a breach that a rule of the node holding the refused attribute
// reports, at or below the attribute's path.
func refusedContentCases(t *testing.T) []refusedContentCase {
	t.Helper()
	unknownArchetype := probes.UnknownArchetypeUnderContent()
	// ENTRY.language is RM-mandatory.
	unknownArchetype.Content[0].(*rm.Observation).Language = rm.CodePhrase{}

	// A present and empty mappings list breaks DV_TEXT.Mappings_valid.
	forbiddenItem := &rm.Cluster{
		ArchetypeNodeID: forbiddenChildArchetype,
		Name:            &rm.DVText{Value: "cluster"},
		Items:           []rm.Item{fiItem("at0002", "two", &rm.DVText{Value: "text", Mappings: []rm.TermMapping{}})},
	}

	relatedComposer := probes.ComposerPartyRelated()
	// 999 is no relationship of the openEHR terminology.
	relatedComposer.Composer.(*rm.PartyRelated).Relationship.DefiningCode.CodeString = "999"

	noInformation := &rm.DVCodedText{
		Value:        "no information",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "271"},
	}
	// A present and empty mappings list breaks DV_TEXT.Mappings_valid.
	noInformation.Mappings = []rm.TermMapping{}
	snomed := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "271649006"}
	// "x" is no TERM_MAPPING match.
	badMatch := &rm.DVText{Value: "text", Mappings: []rm.TermMapping{{Match: rm.Character("x"), Target: snomed}}}
	prohibits := mustCompileInline(t, prohibitedOPT("0", "0"))
	// 999 is no null flavour of the openEHR terminology.
	unknownFlavour := &rm.DVCodedText{
		Value:        "unknown",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "999"},
	}

	blankName := probes.ComposerPartyIdentified()
	// A present and empty name breaks PARTY_IDENTIFIED.Name_valid.
	blankName.Composer.(*rm.PartyIdentified).Name = new("")
	nameProhibited := compositionOPT(composerAttribute(complexChild("PARTY_IDENTIFIED", singleAttribute("name", "0", "0"))))

	return []refusedContentCase{
		{
			name:   "an OBSERVATION whose archetype fits no slot, without its language",
			root:   unknownArchetype,
			c:      mustCompile(t, "vital_signs"),
			want:   "slot_fill /content[openEHR-EHR-OBSERVATION.no_such_archetype.v1] template slot",
			inside: "required /content[openEHR-EHR-OBSERVATION.no_such_archetype.v1]/language rm mandatory",
		},
		{
			name:   "an ELEMENT bound to a child that occurs 0..0, whose value's mappings are present and empty",
			root:   forbiddenItem,
			c:      mustCompileInline(t, forbiddenChildOPT),
			want:   "cardinality /items[at0002] template occurrences",
			inside: "mappings_valid /items[at0002]/value/mappings rm DV_TEXT.Mappings_valid",
		},
		{
			// The single-valued sentence of Admission: a value that binds to
			// none of the allowed children is refused, not entered.
			name:   "a single-valued composer whose PARTY_RELATED value binds to none of the allowed children, coded outside its group",
			root:   relatedComposer,
			c:      mustCompileInline(t, probes.RMTypeMismatchOPT),
			want:   "rm_type_mismatch /composer template type",
			inside: "code_not_in_value_set /composer/relationship/defining_code rm PARTY_RELATED.Relationship_valid",
		},
		{
			name:   "a prohibited null_flavour whose mappings are present and empty",
			root:   prohibitedElement(nil, noInformation),
			c:      prohibits,
			want:   "prohibited /null_flavour template existence",
			inside: "mappings_valid /null_flavour/mappings rm DV_TEXT.Mappings_valid",
		},
		{
			name:   "prohibited mappings whose item has no valid match",
			root:   prohibitedElement(badMatch, nil),
			c:      prohibits,
			want:   "prohibited /value/mappings template existence",
			inside: "term_mapping_match /value/mappings[1]/match rm TERM_MAPPING.Match_valid",
		},
		{
			name:   "a prohibited null_flavour coded outside its group",
			root:   prohibitedElement(nil, unknownFlavour),
			c:      prohibits,
			want:   "prohibited /null_flavour template existence",
			inside: "code_not_in_value_set /null_flavour/defining_code rm ELEMENT.Inv_null_flavour_valid",
		},
		{
			name:   "a prohibited composer name, present and empty",
			root:   blankName,
			c:      mustCompileInline(t, nameProhibited),
			want:   "prohibited /composer/name template existence",
			inside: "rm_invariant /composer/name rm PARTY_IDENTIFIED.Name_valid",
		},
	}
}

// REQ-171 § One walk (Admission), § Acceptance; ADR 0027 — content the
// template does not admit draws its one admission finding and nothing from
// inside: an item that binds to no child, an item bound to a child the
// template does not allow, whose one finding is that child's occurrences, a
// single-valued attribute's value that binds to none of the allowed
// children, and a present attribute the template prohibits, single-valued or
// multi-valued. Each carries an RM breach inside, which the
// RM rules alone report, so a walk that entered the refused content would
// report it too. A rule of the node that holds the refused attribute reports
// nothing at or below the attribute's path either: an ELEMENT's coded rule
// reads its null flavour's defining_code, and a PARTY_IDENTIFIED's rule its
// name, so a finding of theirs there would be an RM rule evaluated on
// content the template did not admit.
func TestREQ171_EngineDoesNotEnterRefusedContent(t *testing.T) {
	t.Parallel()
	tests := refusedContentCases(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := identityKeys(validation.ValidateRM(tc.root).Issues); !slices.Contains(got, tc.inside) {
				t.Fatalf("precondition: ValidateRM(%s) = %q, want it to hold the breach inside, %q", tc.name, got, tc.inside)
			}
			r := validateWith(t, tc.root, tc.c)
			if got := identityKeys(r.Issues); !slices.Equal(got, []string{tc.want}) {
				t.Errorf("Validate(%s) = %q, want %q alone: nothing from inside refused content", tc.name, got, tc.want)
			}
		})
	}
}

// REQ-171 § One walk (the paragraph after step 4), § Acceptance — a rule of
// an admitted node reads the node's attributes as the instance holds them, a
// refused one included, and its finding at the node's own path is reported
// beside the admission finding: an ELEMENT carrying both a value and a
// null_flavour, under a template that prohibits its null_flavour, draws
// `prohibited` at /null_flavour and the ELEMENT's own null-flavour rule at
// "/". Only what the rule would report at or below the refused attribute is
// from inside it; a walk that withdrew the node's own finding too would fail
// here.
func TestREQ171_EngineReportsANodeRuleThatReadsARefusedAttribute(t *testing.T) {
	t.Parallel()
	unknown := &rm.DVCodedText{
		Value:        "unknown",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "253"},
	}
	root := prohibitedElement(&rm.DVText{Value: "x"}, unknown)
	const nodeRule = "rm_invariant / rm ELEMENT.Inv_null_flavour_indicated"
	if got := identityKeys(validation.ValidateRM(root).Issues); !slices.Contains(got, nodeRule) {
		t.Fatalf("precondition: ValidateRM(ELEMENT with a value and a null_flavour) = %q, want it to hold %q", got, nodeRule)
	}
	r := validateWith(t, root, mustCompileInline(t, prohibitedOPT("0", "0")))
	want := []string{"prohibited /null_flavour template existence", nodeRule}
	if got := identityKeys(r.Issues); !slices.Equal(got, want) {
		t.Errorf("Validate(ELEMENT with a value and a prohibited null_flavour) = %q, want %q: the admission finding and the node's own rule at its path", got, want)
	}
}

// guidelineWithoutNamespace returns the valid vital_signs composition whose
// OBSERVATION carries a guideline_id lacking its namespace.
func guidelineWithoutNamespace(t *testing.T) *rm.Composition {
	t.Helper()
	root := validVitalSignsComposition()
	bpObservation(t, root).GuidelineID = &rm.ObjectRef{ID: &rm.HierObjectID{Value: "a3e8b1c2-0d4f-4e5a-9b6c-7d8e9f0a1b2c"}, Type: "GUIDELINE"}
	return root
}

// REQ-171 § One walk (Admission), § Acceptance — an attribute the template
// does not mention is unconstrained and walked with the RM rules alone: an
// OBSERVATION's guideline_id, which vital_signs.opt does not declare, missing
// its namespace draws that breach on the RM layer.
func TestREQ171_EngineWalksAnUnmentionedAttributeWithTheRMRules(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	for _, node := range c.AllByRMType("OBSERVATION") {
		if node.Attribute("guideline_id") != nil {
			t.Fatalf("precondition: vital_signs.opt declares guideline_id on %s; the attribute is not unmentioned", node.AQLPath())
		}
	}
	r := validateWith(t, guidelineWithoutNamespace(t), c)
	want := []string{"rm_invariant " + bloodPressurePath + "/guideline_id/namespace rm mandatory"}
	if got := identityKeys(r.Issues); !slices.Equal(got, want) {
		t.Errorf("Validate(OBSERVATION with a guideline_id lacking its namespace) = %q, want %q: the RM rules walk an attribute the template does not mention", got, want)
	}
}

// sharedPrecision returns the probe input SystolicTwice, whose two systolic
// ELEMENTs hold one DV_QUANTITY, with that DV_QUANTITY's precision set below
// -1, which breaks the RM's precision range.
func sharedPrecision(t *testing.T) *rm.Composition {
	t.Helper()
	root := probes.SystolicTwice()
	items := root.Content[0].(*rm.Observation).Data.Events[0].(*rm.PointEvent[rm.ItemStructure]).Data.(*rm.ItemList).Items
	shared := items[0].Value.(*rm.DVQuantity)
	if items[1].Value != shared {
		t.Fatalf("precondition: the two systolic ELEMENTs hold %p and %p, want one DV_QUANTITY", items[0].Value, items[1].Value)
	}
	shared.Precision = new(rm.Integer(-5))
	return root
}

// REQ-171 § One walk, § Acceptance — a Go value with one defect, held from
// two places, draws two issues, one at each path, with a template as without
// one: one DV_QUANTITY whose precision is below -1, held by two systolic
// ELEMENTs, each bound to the template's systolic node.
func TestREQ171_EngineWithATemplateValidatesASharedValueAtEachPlace(t *testing.T) {
	t.Parallel()
	root := sharedPrecision(t)
	c := mustCompile(t, "vital_signs")
	const list = bloodPressurePath + "/data/events[at0006]/data/items"
	want := []string{list + "[1]/value", list + "[2]/value"}

	precisionAt := func(issues []validation.Issue) []string {
		var out []string
		for _, i := range issues {
			if i.Code == "rm_invariant" && i.Rule == "precision_range" {
				out = append(out, i.Path)
			}
		}
		return out
	}
	if got := precisionAt(validation.ValidateRM(root).Issues); !slices.Equal(got, want) {
		t.Errorf("ValidateRM(two ELEMENTs sharing a DV_QUANTITY) reports precision_range at %q, want at %q: once at each place", got, want)
	}
	if got := precisionAt(validateWith(t, root, c).Issues); !slices.Equal(got, want) {
		t.Errorf("Validate(two ELEMENTs sharing a DV_QUANTITY) reports precision_range at %q, want at %q: once at each place", got, want)
	}
}

// emptyContainerCase is a root holding an RM-mandatory container that is
// present and empty, at path.
type emptyContainerCase struct {
	name string
	root any
	c    *templatecompile.Compiled
	path string
	// want is what Validate reports at path.
	want []string
	// childPath, when set, is the path of the container's child that the
	// template's occurrences report, and wantChild what both report there.
	childPath string
	wantChild []string
}

// personIdentityChildOPT returns a PERSON template whose identities, a list
// the RM marks mandatory, the template leaves optional (existence 0..1), with
// one PARTY_IDENTITY child, at nodeID, that occurs exactly once. A child with
// no node id has its occurrences written at the container's own path.
func personIdentityChildOPT(nodeID string) string {
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>person-identity-child</value></template_id>
  <concept>person-identity-child</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>PERSON</rm_type_name>
    <node_id>at0000</node_id>
    <archetype_id><value>openEHR-DEMOGRAPHIC-PERSON.person.v1</value></archetype_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>identities</rm_attribute_name>
      <existence>` + fiInterval("0", "1") + `</existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>PARTY_IDENTITY</rm_type_name>
        <occurrences>` + fiInterval("1", "1") + `</occurrences>
        <node_id>` + nodeID + `</node_id>
      </children>
      <cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique>
        <interval><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
    </attributes>
  </definition>
</template>`
}

// emptyContainerCases returns an ORGANISATION whose identities are empty
// under a template that declares nothing on them, the vital_signs device
// CLUSTER, which fills a slot, with empty items, and a PERSON whose
// identities are empty under a template whose existence alone requires
// them, under one that requires them by its existence and its child count,
// under one that requires them by its child count, and under two that leave
// them optional but require one child: one whose child has no node id, so
// the template's occurrences report the container's own path, and one whose
// child has a node id, so they report the child's.
func emptyContainerCases(t *testing.T) []emptyContainerCase {
	t.Helper()
	emptyOrganisation := &rm.Organisation{
		ArchetypeNodeID:  "openEHR-DEMOGRAPHIC-ORGANISATION.organisation.v1",
		Name:             rm.DVText{Value: "O"},
		ArchetypeDetails: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-DEMOGRAPHIC-ORGANISATION.organisation.v1"}, RMVersion: "1.1.0"},
		Identities:       []rm.PartyIdentity{},
	}
	emptyDevice := validVitalSignsComposition()
	tree, ok := bpObservation(t, emptyDevice).Protocol.(*rm.ItemTree)
	if !ok || len(tree.Items) != 1 {
		t.Fatalf("fixture protocol is %T, want an ITEM_TREE holding the device CLUSTER", bpObservation(t, emptyDevice).Protocol)
	}
	tree.Items[0].(*rm.Cluster).Items = []rm.Item{}
	required, open := fiInterval("1", "1"), fiInterval("0", "1")

	return []emptyContainerCase{
		{
			name: "under the BMM's mandatory bit alone",
			root: emptyOrganisation,
			c:    mustCompileInline(t, probes.OrganisationOPT),
			path: "/identities",
			want: []string{"cardinality rm cardinality"},
		},
		{
			name: "below a slot",
			root: emptyDevice,
			c:    mustCompile(t, "vital_signs"),
			path: bloodPressurePath + "/protocol/items[openEHR-EHR-CLUSTER.device.v1]/items",
			want: []string{"cardinality rm cardinality"},
		},
		{
			name: "under the template's existence alone",
			root: probes.PersonEmptyIdentities(),
			c:    mustCompileInline(t, personIdentitiesOPT(required, "0")),
			path: "/identities",
			want: []string{"cardinality rm cardinality"},
		},
		{
			name: "under the template's existence and child count",
			root: probes.PersonEmptyIdentities(),
			c:    mustCompileInline(t, personIdentitiesOPT(required, "1")),
			path: "/identities",
			want: []string{"cardinality template cardinality", "required template existence"},
		},
		{
			name: "under the template's child count",
			root: probes.PersonEmptyIdentities(),
			c:    mustCompileInline(t, personIdentitiesOPT(open, "1")),
			path: "/identities",
			want: []string{"cardinality template cardinality"},
		},
		{
			name: "under its child's occurrences, written at the container",
			root: probes.PersonEmptyIdentities(),
			c:    mustCompileInline(t, personIdentityChildOPT("")),
			path: "/identities",
			want: []string{"cardinality template occurrences"},
		},
		{
			name:      "beside its child's occurrences, written at the child",
			root:      probes.PersonEmptyIdentities(),
			c:         mustCompileInline(t, personIdentityChildOPT("at0004")),
			path:      "/identities",
			want:      []string{"cardinality rm cardinality"},
			childPath: "/identities[at0004]",
			wantChild: []string{"cardinality template occurrences"},
		},
	}
}

// REQ-171 § One walk (Presence, Counts), § Acceptance; REQ-112 § Driver —
// an RM-mandatory container that is present and empty, which the RM rules
// alone report as `cardinality`, is reported by the template where a
// template clause covers it and by the RM rules otherwise, never by both.
// Where no template clause covers it, it is the RM's `cardinality`: under a
// template node that carries only the BMM's mandatory bit, below a slot the
// template does not constrain, and under a template whose existence alone
// requires it, since an existence alone asks for no member (REQ-102). Where
// the template's existence and its child count both require it, it is the
// template's `required` and `cardinality`, one per clause; where the
// template's child count alone reports it, the template's `cardinality`
// alone; and so where a child's occurrences report it, at the container's
// own path. Occurrences written at the child's own path report the child,
// not the container: the RM's `cardinality` stays beside them, and the
// empty list's child that occurs once is still counted.
func TestREQ171_EngineReportsAnEmptyMandatoryContainerOnce(t *testing.T) {
	t.Parallel()
	tests := emptyContainerCases(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := keysAt(validation.ValidateRM(tc.root).Issues, tc.path), []string{"cardinality rm cardinality"}; !slices.Equal(got, want) {
				t.Fatalf("precondition: ValidateRM(%s) at %s = %q, want %q", tc.name, tc.path, got, want)
			}
			r := validateWith(t, tc.root, tc.c)
			if got := keysAt(r.Issues, tc.path); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) at %s = %q, want %q: the template's findings or the RM's, never both, for one empty container", tc.name, tc.path, got, tc.want)
			}
			if tc.childPath == "" {
				return
			}
			if got := keysAt(r.Issues, tc.childPath); !slices.Equal(got, tc.wantChild) {
				t.Errorf("Validate(%s) at %s = %q, want %q: the occurrences of the empty list's child", tc.name, tc.childPath, got, tc.wantChild)
			}
		})
	}
}

// resourceDescriptionOPT returns a RESOURCE_DESCRIPTION template that
// declares original_author, a Hash the RM marks mandatory, as multi-valued,
// with the existence lower..upper and a child count from countLower up.
func resourceDescriptionOPT(lower, upper, countLower string) string {
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>resource-description</value></template_id>
  <concept>resource-description</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>RESOURCE_DESCRIPTION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>original_author</rm_attribute_name>
      <existence>` + fiInterval(lower, upper) + `</existence>
      <cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique>
        <interval><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>` + countLower + `</lower></interval></cardinality>
    </attributes>
  </definition>
</template>`
}

// REQ-171 § One walk (Presence), § Structural access — a Hash the RM marks
// mandatory that is present and empty breaks no mandatory rule, as the RM
// rules alone read it, however the template declares it: a template that
// declares RESOURCE_DESCRIPTION original_author multi-valued and leaves its
// existence open draws nothing, though the template carries the BMM's
// mandatory bit for it, and nor does one whose existence alone requires it,
// since an existence alone asks for no member (REQ-102). Where the
// template's existence and its child count both require it, the template's
// `required` and `cardinality` stay.
func TestREQ171_EngineReadsAnEmptyHashAsTheRMRulesDo(t *testing.T) {
	t.Parallel()
	const path = "/original_author"
	tests := []struct {
		name string
		c    *templatecompile.Compiled
		want []string
	}{
		{name: "existence open", c: mustCompileInline(t, resourceDescriptionOPT("0", "1", "0"))},
		{name: "existence required alone", c: mustCompileInline(t, resourceDescriptionOPT("1", "1", "0"))},
		{
			name: "existence and child count required",
			c:    mustCompileInline(t, resourceDescriptionOPT("1", "1", "1")),
			want: []string{"cardinality template cardinality", "required template existence"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := describedResource([]string{"en"}, "")
			root.OriginalAuthor = map[string]string{}
			if got := keysAt(validation.ValidateRM(root).Issues, path); len(got) != 0 {
				t.Fatalf("precondition: ValidateRM(RESOURCE_DESCRIPTION with an empty original_author) at %s = %q, want nothing", path, got)
			}
			if got := keysAt(validateWith(t, root, tc.c).Issues, path); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(RESOURCE_DESCRIPTION with an empty original_author, %s) at %s = %q, want %q", tc.name, path, got, tc.want)
			}
		})
	}
}

// REQ-171 § Surface — the compiled template is built once and shared:
// Validate over every corpus composition, against its compiled template,
// run from several goroutines at once over one compiled template and one
// root, returns what it returns run alone. Everything the compile resolves
// for the walk is set before Compile returns, and the walk only reads it; a
// walk that wrote to the compiled template, an answer filled in on first
// use, would fail this test under the race detector.
func TestREQ171_EngineSharesOneCompiledTemplateBetweenGoroutines(t *testing.T) {
	t.Parallel()
	const goroutines = 4
	for _, doc := range corpusDocuments(t) {
		if doc.c == nil {
			continue
		}
		want := validation.Validate(doc.root, doc.c)
		results := make([]validation.Result, goroutines)
		var wg sync.WaitGroup
		for i := range results {
			wg.Go(func() { results[i] = validation.Validate(doc.root, doc.c) })
		}
		wg.Wait()
		for i, got := range results {
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Validate(%s) in goroutine %d = %+v, want %+v, its result run alone", doc.rel.Rel, i, got, want)
			}
		}
	}
}

// rmMandatoryCase is a root that lacks an attribute the RM marks mandatory
// for the value's class, at path, which a template node of a class above it
// declares and leaves optional.
type rmMandatoryCase struct {
	name string
	root any
	c    *templatecompile.Compiled
	path string
}

// rmMandatoryCases returns a PARTY_RELATED composer with an empty
// relationship, bound to a PARTY_PROXY node that declares relationship with
// the existence 0..1, and a CLUSTER root with no items, bound to an ITEM
// node that declares items, multi-valued, with the existence 0..1. Neither
// node's class declares the attribute, so the template carries no BMM bit
// for it.
func rmMandatoryCases(t *testing.T) []rmMandatoryCase {
	t.Helper()
	related := probes.ComposerPartyRelated()
	related.Composer.(*rm.PartyRelated).Relationship = rm.DVCodedText{}
	itemOPT := `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>item-root</value></template_id>
  <concept>item-root</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>ITEM</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>items</rm_attribute_name>
      <existence>` + fiInterval("0", "1") + `</existence>
      <cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique>
        <interval><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
    </attributes>
  </definition>
</template>`
	return []rmMandatoryCase{
		{
			name: "a PARTY_RELATED composer with no relationship, under a PARTY_PROXY node",
			root: related,
			c:    mustCompileInline(t, compositionOPT(composerAttribute(complexChild("PARTY_PROXY", singleAttribute("relationship", "0", "1"))))),
			path: "/composer/relationship",
		},
		{
			name: "a CLUSTER root with no items, under an ITEM node",
			root: &rm.Cluster{ArchetypeNodeID: "at0000", Name: rm.DVText{Value: "cluster"}},
			c:    mustCompileInline(t, itemOPT),
			path: "/items",
		},
	}
}

// REQ-171 § One walk (Presence), § Acceptance — an attribute a template node
// declares and leaves optional, which the RM marks mandatory for the class of
// the value bound there, is reported once, as the RM's `required`, when the
// instance lacks it, single-valued or multi-valued, as the RM rules alone
// report it. The node's class does not declare the attribute, so the
// template carries no BMM bit for it: the walk reads it from the value's
// class.
func TestREQ171_EngineReportsAnRMMandatoryAttributeATemplateNodeLeavesOptional(t *testing.T) {
	t.Parallel()
	want := []string{"required rm mandatory"}
	for _, tc := range rmMandatoryCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			if got := keysAt(validation.ValidateRM(tc.root).Issues, tc.path); !slices.Equal(got, want) {
				t.Fatalf("precondition: ValidateRM(%s) at %s = %q, want %q", tc.name, tc.path, got, want)
			}
			if got := keysAt(validateWith(t, tc.root, tc.c).Issues, tc.path); !slices.Equal(got, want) {
				t.Errorf("Validate(%s) at %s = %q, want %q", tc.name, tc.path, got, want)
			}
		})
	}
}

// foreignClassCase is a root whose value's class does not list attributes
// the root node of the template c carries, with the findings Validate
// reports, as identityKeys writes them.
type foreignClassCase struct {
	name string
	root any
	c    *templatecompile.Compiled
	want []string
}

// foreignClassCases returns an RM-valid COMPOSITION and a root outside the
// closed RM set, each bound to an OBSERVATION node that declares subject,
// which the RM marks mandatory for an OBSERVATION, with the existence 0..1,
// and data with the existence 1..1. The compile injects the other attributes
// the RM marks mandatory for an OBSERVATION, its encoding among them. A
// COMPOSITION lists none of subject, data and encoding; a value of no RM
// class lists no attribute.
func foreignClassCases(t *testing.T) []foreignClassCase {
	t.Helper()
	type notRM struct{ Name string }
	const observationID = "openEHR-EHR-OBSERVATION.order.v1"
	c := mustCompileInline(t, rootOPT("OBSERVATION", observationID,
		singleAttribute("subject", "0", "1"),
		singleAttribute("data", "1", "1"),
	))
	return []foreignClassCase{
		{
			name: "a COMPOSITION under an OBSERVATION node",
			root: probes.ComposerPartyIdentified(),
			c:    c,
			want: []string{
				"archetype_id_mismatch /archetype_node_id template identity",
				"required /data template existence",
				"rm_type_mismatch / template type",
			},
		},
		{
			name: "a root outside the RM set under an OBSERVATION node",
			root: &notRM{Name: "root"},
			c:    c,
			want: []string{
				"required /data template existence",
				"rm_type_mismatch / template type",
				"rm_type_unknown /  ",
			},
		},
	}
}

// REQ-112 § Driver; REQ-171 § One walk (Presence) — the RM layer reports an
// attribute absent only from the pinned Reference Model's reading of the
// value's own class. A value whose class does not list an attribute the
// template node carries draws no RM-layer finding for it: not for one the
// node declares and leaves optional though the RM marks it mandatory for
// the node's class (subject), nor for one the compile injects because it
// does (encoding, language, name). The template's own existence finding
// stays (data), beside the type mismatch at the root.
func TestREQ112_REQ171_EngineReadsRMPresenceFromTheValuesClassAlone(t *testing.T) {
	t.Parallel()
	for _, tc := range foreignClassCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			r := validateWith(t, tc.root, tc.c)
			if got := identityKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) = %q, want %q: no RM-layer finding for an attribute the value's class does not list", tc.name, got, tc.want)
			}
		})
	}
}

// REQ-171 § One walk (Presence) — a part of a reference the instance lacks,
// where the template binds the reference to a node, is reported once: by the
// reference's own rule, as the RM rules alone report it. The compiled node
// carries the RM's mandatory bit for the part; the walk reads the part as
// the RM rules do, and they leave the parts of a reference to the
// reference's rule.
func TestREQ171_EngineReportsAMissingReferencePartOnce(t *testing.T) {
	t.Parallel()
	root := probes.ComposerPartyIdentified()
	// The reference lacks its namespace.
	root.Composer.(*rm.PartyIdentified).ExternalRef = &rm.PartyRef{ID: &rm.HierObjectID{Value: "a3e8b1c2-0d4f-4e5a-9b6c-7d8e9f0a1b2c"}, Type: "PERSON"}
	c := mustCompileInline(t, compositionOPT(composerAttribute(complexChild("PARTY_IDENTIFIED", singleAttribute("external_ref", "0", "1", complexChild("PARTY_REF"))))))
	const path = "/composer/external_ref/namespace"
	want := []string{"rm_invariant rm mandatory"}
	if got := keysAt(validation.ValidateRM(root).Issues, path); !slices.Equal(got, want) {
		t.Fatalf("precondition: ValidateRM(a composer reference lacking its namespace) at %s = %q, want %q", path, got, want)
	}
	if got := keysAt(validateWith(t, root, c).Issues, path); !slices.Equal(got, want) {
		t.Errorf("Validate(a composer reference lacking its namespace) at %s = %q, want %q: one absence, one finding", path, got, want)
	}
}

// attributeOrderCase is a root whose findings, in the order the walk reports
// them, fall under the paths want, each path's findings together and the
// paths in want's order.
type attributeOrderCase struct {
	name string
	root any
	c    *templatecompile.Compiled
	want []string
}

// attributeOrderCases returns three roots whose findings fall under several
// attributes, with the order of those attributes. A composition lacks the
// composer and the context a template requires, in that order, and carries
// a link with none of its mandatory parts, which the template does not
// mention: the class lists links first, and context before composer. A
// PARTY_RELATED composer, bound to a PARTY_PROXY node that requires its
// relationship, lacks it, and its external_ref lacks its namespace: the
// value is of a class below the node's, whose order lists external_ref
// first. An OBSERVATION, bound to a node that declares its protocol alone,
// lacks its subject and its data, which the compile injects into the node
// because the RM marks them mandatory, and carries a link with none of its
// mandatory parts: no template clause constrains the injected attributes,
// so they come in the class's order, after the link.
func attributeOrderCases(t *testing.T) []attributeOrderCase {
	t.Helper()
	noComposer := probes.ComposerPartyIdentified()
	noComposer.Composer = nil
	noComposer.Links = []rm.Link{{}}

	related := probes.ComposerPartyRelated()
	composer := related.Composer.(*rm.PartyRelated)
	composer.Relationship = rm.DVCodedText{}
	composer.ExternalRef = &rm.PartyRef{ID: &rm.HierObjectID{Value: "a3e8b1c2-0d4f-4e5a-9b6c-7d8e9f0a1b2c"}, Type: "PERSON"} // namespace missing

	const observationID = "openEHR-EHR-OBSERVATION.order.v1"
	observation := &rm.Observation{
		ArchetypeNodeID:  observationID,
		Name:             rm.DVText{Value: "order"},
		ArchetypeDetails: archetypeDetails(observationID),
		Language:         rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
		Encoding:         rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}, CodeString: "UTF-8"},
		Links:            []rm.Link{{}},
	}

	return []attributeOrderCase{
		{
			name: "a composition lacking the composer and the context a template requires",
			root: noComposer,
			c: mustCompileInline(t, compositionOPT(
				composerAttribute(complexChild("PARTY_SELF")),
				singleAttribute("context", "1", "1"),
			)),
			want: []string{"/composer", "/context", "/links[1]"},
		},
		{
			name: "a PARTY_RELATED composer under a PARTY_PROXY node requiring its relationship",
			root: related,
			c:    mustCompileInline(t, compositionOPT(composerAttribute(complexChild("PARTY_PROXY", singleAttribute("relationship", "1", "1"))))),
			want: []string{"/composer/relationship", "/composer/external_ref"},
		},
		{
			name: "an OBSERVATION lacking the subject and the data the compile injects",
			root: observation,
			c:    mustCompileInline(t, rootOPT("OBSERVATION", observationID, singleAttribute("protocol", "0", "1"))),
			want: []string{"/links[1]", "/subject", "/data"},
		},
	}
}

// pathGroups returns, for each of issues in order, the first of under its
// path is at or below, with a run of one path written once; an issue under
// none of them is written by its own path.
func pathGroups(issues []validation.Issue, under []string) []string {
	var out []string
	for _, iss := range issues {
		group := iss.Path
		for _, u := range under {
			if iss.Path == u || strings.HasPrefix(iss.Path, u+"/") || strings.HasPrefix(iss.Path, u+"[") {
				group = u
				break
			}
		}
		if len(out) == 0 || out[len(out)-1] != group {
			out = append(out, group)
		}
	}
	return out
}

// REQ-171 § One walk, § Coverage — at a node the template binds, the engine
// visits the attributes the template declares in the template's order, then
// the other attributes it visits in the order of the value's class, the
// attributes the compile injects because the RM marks them mandatory among
// them: they are no template clause. That holds where the value is of the
// bound node's class and where it is of a class below it.
func TestREQ171_EngineVisitsTheDeclaredAttributesInTheTemplatesOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range attributeOrderCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			r := validateWith(t, tc.root, tc.c)
			if got := pathGroups(r.Issues, tc.want); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) reports its findings under %q, want under %q in that order; issues %q", tc.name, got, tc.want, identityKeys(r.Issues))
			}
		})
	}
}

// REQ-171 § One walk (Admission) — content the template refuses withdraws
// only the findings at or below its own path. A DV_MULTIMEDIA whose template
// prohibits its integrity_check carries one, and an
// integrity_check_algorithm coded outside its code set: the prohibition is
// reported once, and the class's coded rule still reports the algorithm,
// whose path begins with the refused attribute's name but is no path below
// it.
func TestREQ171_EngineKeepsAFindingBesideRefusedContent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		root any
		c    *templatecompile.Compiled
		want []string
	}{
		{
			name: "a DV_MULTIMEDIA whose prohibited integrity_check sits beside an algorithm coded outside its code set",
			root: &rm.DVMultimedia{
				MediaType:               rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}, CodeString: "image/png"},
				Size:                    3,
				IntegrityCheck:          []byte{1, 2, 3},
				IntegrityCheckAlgorithm: &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr_integrity_check_algorithms"}, CodeString: "NOPE"},
			},
			c: mustCompileInline(t, rootOPT("DV_MULTIMEDIA", "", singleAttribute("integrity_check", "0", "0"))),
			want: []string{
				"code_not_in_value_set /integrity_check_algorithm rm DV_MULTIMEDIA.Integrity_check_algorithm_validity",
				"prohibited /integrity_check template existence",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := identityKeys(validateWith(t, tc.root, tc.c).Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

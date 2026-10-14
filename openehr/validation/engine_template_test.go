package validation_test

// engine_template_test.go: REQ-171 § One walk, § Acceptance, "when the engine
// lands" — the one walk with a template. A template node the instance lacks
// draws one `required`, on the template layer; content the template does not
// admit draws its one admission finding and nothing from inside; an attribute
// the template does not mention is walked with the RM rules; a value held
// from two places is validated at each; and an empty RM-mandatory container
// draws one finding, the template's where the template reports it and the
// RM's `cardinality` where only the RM requires it.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/validation"
)

// engineWith runs the one walk over root with the compiled template c,
// failing the test when it has not returned within walkDeadline.
func engineWith(t *testing.T, root any, c *templatecompile.Compiled) validation.Result {
	t.Helper()
	return validateWithin(t, "the engine", walkDeadline, func() validation.Result { return validation.RunEngine(root, c, validation.Limits{}) })
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
// whose existence requires them.
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
			r := engineWith(t, tc.root, tc.c)
			if got, want := keysAt(r.Issues, tc.path), []string{"required template existence"}; !slices.Equal(got, want) {
				t.Errorf("engine(%s) at %s = %q, want %q: one absence, one finding, on the template layer", tc.name, tc.path, got, want)
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
	// breach inside, which the two walks report.
	want, inside string
}

// refusedContentCases returns an OBSERVATION whose archetype fits no slot of
// vital_signs.opt, lacking its language; a PARTY_RELATED composer, coded
// outside its group, where the template allows a PARTY_SELF alone; and an
// ELEMENT under a template that prohibits its null_flavour and its value's
// mappings, carrying a null flavour whose mappings are present and empty, or
// a mapping with no valid match.
func refusedContentCases(t *testing.T) []refusedContentCase {
	t.Helper()
	unknownArchetype := probes.UnknownArchetypeUnderContent()
	// ENTRY.language is RM-mandatory.
	unknownArchetype.Content[0].(*rm.Observation).Language = rm.CodePhrase{}

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

	return []refusedContentCase{
		{
			name:   "an OBSERVATION whose archetype fits no slot, without its language",
			root:   unknownArchetype,
			c:      mustCompile(t, "vital_signs"),
			want:   "slot_fill /content[openEHR-EHR-OBSERVATION.no_such_archetype.v1] template slot",
			inside: "required /content[openEHR-EHR-OBSERVATION.no_such_archetype.v1]/language rm mandatory",
		},
		{
			name:   "a PARTY_RELATED composer under a PARTY_SELF-only composer, coded outside its group",
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
	}
}

// REQ-171 § One walk (Admission), § Acceptance; ADR 0027 — content the
// template does not admit draws its one admission finding and nothing from
// inside: an item that binds to no child, a single value of a type the
// template does not allow, and a present attribute the template prohibits,
// single-valued or multi-valued. Each carries an RM breach inside, which the
// two walks report, so a walk that entered the refused content would report
// it too.
func TestREQ171_EngineDoesNotEnterRefusedContent(t *testing.T) {
	t.Parallel()
	tests := refusedContentCases(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := identityKeys(validation.Validate(tc.root, tc.c).Issues); !slices.Contains(got, tc.inside) || !slices.Contains(got, tc.want) {
				t.Fatalf("precondition: Validate(%s) = %q, want it to hold %q and the breach inside, %q", tc.name, got, tc.want, tc.inside)
			}
			r := engineWith(t, tc.root, tc.c)
			if got := identityKeys(r.Issues); !slices.Equal(got, []string{tc.want}) {
				t.Errorf("engine(%s) = %q, want %q alone: nothing from inside refused content", tc.name, got, tc.want)
			}
		})
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
	r := engineWith(t, guidelineWithoutNamespace(t), c)
	want := []string{"rm_invariant " + bloodPressurePath + "/guideline_id/namespace rm mandatory"}
	if got := identityKeys(r.Issues); !slices.Equal(got, want) {
		t.Errorf("engine(OBSERVATION with a guideline_id lacking its namespace) = %q, want %q: the RM rules walk an attribute the template does not mention", got, want)
	}
}

// REQ-171 § One walk, § Acceptance — a Go value with one defect, held from
// two places, draws two issues, one at each path, with a template as without
// one: one DV_QUANTITY whose precision is below -1, held by two systolic
// ELEMENTs, each bound to the template's systolic node. The two walks report
// it once, since the floor walks a shared value once.
func TestREQ171_EngineWithATemplateValidatesASharedValueAtEachPlace(t *testing.T) {
	t.Parallel()
	root := probes.SystolicTwice()
	items := root.Content[0].(*rm.Observation).Data.Events[0].(*rm.PointEvent[rm.ItemStructure]).Data.(*rm.ItemList).Items
	shared := items[0].Value.(*rm.DVQuantity)
	if items[1].Value != shared {
		t.Fatalf("precondition: the two systolic ELEMENTs hold %p and %p, want one DV_QUANTITY", items[0].Value, items[1].Value)
	}
	shared.Precision = new(rm.Integer(-5))
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
	if got := precisionAt(validation.Validate(root, c).Issues); len(got) != 1 {
		t.Fatalf("precondition: Validate(two ELEMENTs sharing a DV_QUANTITY) reports precision_range at %q, want once", got)
	}
	if got := precisionAt(engineWith(t, root, c).Issues); !slices.Equal(got, want) {
		t.Errorf("engine(two ELEMENTs sharing a DV_QUANTITY) reports precision_range at %q, want at %q: once at each place", got, want)
	}
}

// emptyContainerCase is a root holding an RM-mandatory container that is
// present and empty, at path.
type emptyContainerCase struct {
	name string
	root any
	c    *templatecompile.Compiled
	path string
	// twoWalks is what Validate reports at path; want what the engine
	// reports there.
	twoWalks, want []string
}

// emptyContainerCases returns an ORGANISATION whose identities are empty
// under a template that declares nothing on them, the vital_signs device
// CLUSTER, which fills a slot, with empty items, and a PERSON whose
// identities are empty under a template that requires them by its existence,
// and under one that requires them by its child count.
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
			name:     "under the BMM's mandatory bit alone",
			root:     emptyOrganisation,
			c:        mustCompileInline(t, probes.OrganisationOPT),
			path:     "/identities",
			twoWalks: []string{"cardinality rm cardinality", "required rm cardinality"},
			want:     []string{"cardinality rm cardinality"},
		},
		{
			name:     "below a slot",
			root:     emptyDevice,
			c:        mustCompile(t, "vital_signs"),
			path:     bloodPressurePath + "/protocol/items[openEHR-EHR-CLUSTER.device.v1]/items",
			twoWalks: []string{"cardinality rm cardinality"},
			want:     []string{"cardinality rm cardinality"},
		},
		{
			name:     "under the template's existence",
			root:     probes.PersonEmptyIdentities(),
			c:        mustCompileInline(t, personIdentitiesOPT(required, "0")),
			path:     "/identities",
			twoWalks: []string{"cardinality rm cardinality", "required template existence"},
			want:     []string{"required template existence"},
		},
		{
			name:     "under the template's child count",
			root:     probes.PersonEmptyIdentities(),
			c:        mustCompileInline(t, personIdentitiesOPT(open, "1")),
			path:     "/identities",
			twoWalks: []string{"cardinality template cardinality", "required rm cardinality"},
			want:     []string{"cardinality template cardinality"},
		},
	}
}

// REQ-171 § One walk (Presence), § Acceptance; REQ-112 § Driver — an
// RM-mandatory container that is present and empty draws one finding. Where
// no template clause covers it, it is the RM's `cardinality`: under a
// template node that carries only the BMM's mandatory bit, where the two
// walks report it twice, as `required` and `cardinality`, and below a slot
// the template does not constrain. Where the template's existence requires
// it, it is the template's `required`, which the two walks pair with the RM's
// `cardinality`; where the template's child count reports it, the template's
// `cardinality` alone, which the two walks pair with a `required` read from
// the BMM's mandatory bit.
func TestREQ171_EngineReportsAnEmptyMandatoryContainerOnce(t *testing.T) {
	t.Parallel()
	tests := emptyContainerCases(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := keysAt(validation.Validate(tc.root, tc.c).Issues, tc.path); !slices.Equal(got, tc.twoWalks) {
				t.Fatalf("precondition: Validate(%s) at %s = %q, want %q", tc.name, tc.path, got, tc.twoWalks)
			}
			r := engineWith(t, tc.root, tc.c)
			if got := keysAt(r.Issues, tc.path); !slices.Equal(got, tc.want) {
				t.Errorf("engine(%s) at %s = %q, want %q: one finding for one empty container", tc.name, tc.path, got, tc.want)
			}
		})
	}
}

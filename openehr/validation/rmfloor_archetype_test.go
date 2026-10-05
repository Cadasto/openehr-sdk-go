package validation_test

// rmfloor_archetype_test.go: REQ-112 — the RM floor's archetype-root and
// ARCHETYPED catalogue rows. An object of a class that declares the RM
// Is_archetype_root invariant must carry archetype_details
// (LOCATABLE.Archetyped_valid), and an ARCHETYPED wherever it sits must carry a
// non-empty archetype_id and rm_version. The EHR_STATUS rows through both
// entries, the Bytes entry's key presence included, are PROBE-081's
// (rmfloor_bytes_test.go).

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmroots"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestValidateRM_ArchetypeRootClassesMatchBMM checks that the floor reports
// the archetype-root rule on exactly the classes of the shared closed list
// (REQ-112, ADR 0001), which internal/rmroots pins to the vendored BMM's
// Is_archetype_root declarations. It runs the floor over every registered
// LOCATABLE concrete: a zero value of a root class must report
// `is_archetype_root` at /archetype_details, any other LOCATABLE must not
// (the spec's MUST NOT: FOLDER, PARTY_RELATIONSHIP, GENERIC_ENTRY,
// PARTY_IDENTITY, CONTACT, ADDRESS, CAPABILITY and the data structures), and
// a value of either kind carrying archetype_details must not. A floor that
// stops reporting a root class, or reports a class the list leaves out,
// fails here.
func TestValidateRM_ArchetypeRootClassesMatchBMM(t *testing.T) {
	swept := map[string]bool{}
	var roots []string
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		v := ctor()
		root := rmroots.IsArchetypeRoot(name)
		loc, ok := v.(rm.MutableLocatable)
		if !ok {
			if root {
				t.Errorf("%s is an archetype root but its registered Go type %T is not a LOCATABLE", name, v)
			}
			continue
		}
		swept[name] = true
		if root {
			roots = append(roots, name)
		}
		if got := reportsArchetypeRoot(validation.ValidateRM(v)); got != root {
			t.Errorf("ValidateRM(zero %s) reports is_archetype_root at /archetype_details = %v, want %v", name, got, root)
		}
		loc.SetArchetypeDetails(&rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-" + name + ".x.v1"}, RMVersion: "1.1.0"})
		if reportsArchetypeRoot(validation.ValidateRM(v)) {
			t.Errorf("ValidateRM(%s with archetype_details) reports is_archetype_root; want none", name)
		}
	}
	// internal/rmroots checks that every root class has a registered
	// LOCATABLE Go type; this sweep must have reached at least one.
	if len(roots) == 0 {
		t.Fatalf("swept %d LOCATABLE concretes (%v) and no archetype root, so the rule above is vacuous", len(swept), slices.Sorted(maps.Keys(swept)))
	}
	// The spec's named non-roots the SDK models must have been swept, so the
	// MUST NOT above is not vacuous. EXTRACT is not registered by the SDK.
	for _, name := range []string{
		"FOLDER", "PARTY_RELATIONSHIP", "GENERIC_ENTRY", "PARTY_IDENTITY", "CONTACT", "ADDRESS", "CAPABILITY",
		"SECTION", "ACTIVITY", "HISTORY", "POINT_EVENT", "INTERVAL_EVENT",
		"ITEM_TREE", "ITEM_LIST", "ITEM_SINGLE", "ITEM_TABLE", "CLUSTER", "ELEMENT",
	} {
		if root := rmroots.IsArchetypeRoot(name); !swept[name] || root {
			t.Errorf("non-root %s: swept = %v, root = %v; want swept and not a root", name, swept[name], root)
		}
	}
}

// reportsArchetypeRoot reports whether r holds the root-level
// `is_archetype_root` finding.
func reportsArchetypeRoot(r validation.Result) bool {
	return containsIssue(r.Issues, "/archetype_details", "is_archetype_root")
}

// TestValidateRM_ArchetypeRootTypedEntries checks the typed entries report the
// archetype-root rule on their own root (REQ-112): EHR_ACCESS and a demographic
// PARTY without archetype_details are reported, a FOLDER is not.
func TestValidateRM_ArchetypeRootTypedEntries(t *testing.T) {
	name := rm.DVText{Value: "n"}
	cases := []struct {
		name string
		run  func() validation.Result
		want bool
	}{
		{"ValidateRMEHRAccess", func() validation.Result {
			return validation.ValidateRMEHRAccess(&rm.EHRAccess{ArchetypeNodeID: "openEHR-EHR-EHR_ACCESS.generic.v1", Name: name})
		}, true},
		{"ValidateRMDemographic(PERSON)", func() validation.Result {
			return validation.ValidateRMDemographic(&rm.Person{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PERSON.person.v1", Name: name})
		}, true},
		{"ValidateRMDemographic(ROLE)", func() validation.Result {
			return validation.ValidateRMDemographic(&rm.Role{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-ROLE.role.v1", Name: name})
		}, true},
		{"ValidateRMFolder", func() validation.Result {
			return validation.ValidateRMFolder(&rm.Folder{ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1", Name: name})
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.run()
			if got := reportsArchetypeRoot(r); got != tc.want {
				t.Errorf("%s: is_archetype_root at /archetype_details = %v, want %v; issues %+v", tc.name, got, tc.want, r.Issues)
			}
		})
	}
}

// TestValidateRM_ArchetypedRoot checks the ARCHETYPED rule on an ARCHETYPED
// handed to ValidateRM directly (REQ-112): an empty archetype_id is `required`
// at archetype_id/value, an empty rm_version is `rm_version_valid`, and a
// complete one reports nothing.
func TestValidateRM_ArchetypedRoot(t *testing.T) {
	cases := []struct {
		name string
		root any
		want []string
	}{
		{
			name: "zero, pointer form",
			root: &rm.Archetyped{},
			want: []string{"required /archetype_id/value", "rm_version_valid /rm_version"},
		},
		{
			name: "rm_version empty, value form",
			root: rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-CLUSTER.x.v1"}},
			want: []string{"rm_version_valid /rm_version"},
		},
		{
			name: "complete",
			root: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-CLUSTER.x.v1"}, RMVersion: "1.1.0"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := findingsOf(validation.ValidateRM(tc.root)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) findings = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// archetypedJSON returns a complete ARCHETYPED for archetype id.
func archetypedJSON(id string) string {
	return `{"_type":"ARCHETYPED","archetype_id":{"_type":"ARCHETYPE_ID","value":"` + id + `"},"rm_version":"1.1.0"}`
}

// compositionJSON returns an otherwise valid COMPOSITION body. rootDetails is
// its archetype_details member value (empty leaves it out); content is the
// content array's elements, verbatim.
func compositionJSON(rootDetails string, content ...string) string {
	details := ""
	if rootDetails != "" {
		details = `"archetype_details":` + rootDetails + `,`
	}
	return `{"_type":"COMPOSITION",
		"name":{"_type":"DV_TEXT","value":"Encounter"},
		"archetype_node_id":"openEHR-EHR-COMPOSITION.encounter.v1",` + details + `
		"language":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"},"code_string":"en"},
		"territory":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"},"code_string":"NL"},
		"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"433"}},
		"composer":{"_type":"PARTY_SELF"},
		"content":[` + strings.Join(content, ",") + `]}`
}

// evaluationJSON returns an otherwise valid EVALUATION. details is its
// archetype_details member value (empty leaves it out); items are the ITEM_TREE
// data's items, verbatim.
func evaluationJSON(details string, items ...string) string {
	member := ""
	if details != "" {
		member = `"archetype_details":` + details + `,`
	}
	return `{"_type":"EVALUATION",
		"name":{"_type":"DV_TEXT","value":"Problem"},
		"archetype_node_id":"openEHR-EHR-EVALUATION.problem_diagnosis.v1",` + member + `
		"language":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"},"code_string":"en"},
		"encoding":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"IANA_character-sets"},"code_string":"UTF-8"},
		"subject":{"_type":"PARTY_SELF"},
		"data":{"_type":"ITEM_TREE","name":{"_type":"DV_TEXT","value":"Structure"},"archetype_node_id":"at0001",
			"items":[` + strings.Join(items, ",") + `]}}`
}

// sectionJSON returns a SECTION holding items, verbatim.
func sectionJSON(items ...string) string {
	return `{"_type":"SECTION","name":{"_type":"DV_TEXT","value":"Findings"},
		"archetype_node_id":"openEHR-EHR-SECTION.adhoc.v1",
		"items":[` + strings.Join(items, ",") + `]}`
}

// clusterJSON returns a CLUSTER with one ELEMENT; details is its
// archetype_details member value (empty leaves it out).
func clusterJSON(details string) string {
	member := ""
	if details != "" {
		member = `"archetype_details":` + details + `,`
	}
	return `{"_type":"CLUSTER","name":{"_type":"DV_TEXT","value":"Anatomical location"},
		"archetype_node_id":"openEHR-EHR-CLUSTER.anatomical_location.v1",` + member + `
		"items":[{"_type":"ELEMENT","name":{"_type":"DV_TEXT","value":"Body site"},"archetype_node_id":"at0001",
			"value":{"_type":"DV_TEXT","value":"left arm"}}]}`
}

// TestValidateRM_NestedArchetypeRootsAndArchetyped checks the two rows on
// nodes below the root (REQ-112), each reported at its own path and nothing
// else: an ENTRY nested in a COMPOSITION, directly or in a SECTION, is an
// archetype root and needs archetype_details; a CLUSTER is not a root, so it
// may leave archetype_details out, but one it carries must be a complete
// ARCHETYPED, as must a root's.
func TestValidateRM_NestedArchetypeRootsAndArchetyped(t *testing.T) {
	const (
		compositionID = "openEHR-EHR-COMPOSITION.encounter.v1"
		evaluationID  = "openEHR-EHR-EVALUATION.problem_diagnosis.v1"
		clusterID     = "openEHR-EHR-CLUSTER.anatomical_location.v1"
		incomplete    = `{"_type":"ARCHETYPED","archetype_id":{"_type":"ARCHETYPE_ID","value":""},"rm_version":""}`
	)
	root := archetypedJSON(compositionID)
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "every root carries archetype_details",
			body: compositionJSON(root, evaluationJSON(archetypedJSON(evaluationID), clusterJSON(""))),
		},
		{
			name: "COMPOSITION root without archetype_details",
			body: compositionJSON("", evaluationJSON(archetypedJSON(evaluationID))),
			want: []string{"is_archetype_root /archetype_details"},
		},
		{
			name: "EVALUATION in content without archetype_details",
			body: compositionJSON(root, evaluationJSON("")),
			want: []string{"is_archetype_root /content[0]/archetype_details"},
		},
		{
			name: "EVALUATION in a SECTION without archetype_details",
			body: compositionJSON(root, sectionJSON(evaluationJSON(archetypedJSON(evaluationID))), sectionJSON(evaluationJSON(""))),
			want: []string{"is_archetype_root /content[1]/items[0]/archetype_details"},
		},
		{
			name: "EVALUATION with an incomplete ARCHETYPED",
			body: compositionJSON(root, evaluationJSON(incomplete)),
			want: []string{
				"required /content[0]/archetype_details/archetype_id/value",
				"rm_version_valid /content[0]/archetype_details/rm_version",
			},
		},
		{
			name: "CLUSTER with an incomplete ARCHETYPED",
			body: compositionJSON(root, evaluationJSON(archetypedJSON(evaluationID), clusterJSON(incomplete))),
			want: []string{
				"required /content[0]/data/items[0]/archetype_details/archetype_id/value",
				"rm_version_valid /content[0]/data/items[0]/archetype_details/rm_version",
			},
		},
		{
			name: "CLUSTER with a complete ARCHETYPED",
			body: compositionJSON(root, evaluationJSON(archetypedJSON(evaluationID), clusterJSON(archetypedJSON(clusterID)))),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var comp rm.Composition
			if err := canjson.Unmarshal([]byte(tc.body), &comp); err != nil {
				t.Fatalf("canjson.Unmarshal: %v", err)
			}
			if got := findingsOf(validation.ValidateRM(&comp)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) findings = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

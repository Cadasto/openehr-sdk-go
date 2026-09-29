package validation_test

// rmfloor_archetype_test.go: REQ-112 — the RM floor's archetype-root and
// ARCHETYPED catalogue rows. An object of a class that declares the RM
// Is_archetype_root invariant must carry archetype_details
// (LOCATABLE.Archetyped_valid), and an ARCHETYPED wherever it sits must carry a
// non-empty archetype_id and rm_version. The EHR_STATUS rows through both
// entries, the Bytes entry's key presence included, are PROBE-081's
// (rmfloor_bytes_test.go).

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// bmmArchetypeRoots reads the vendored RM BMM and returns the classes that
// declare the Is_archetype_root invariant, and the concrete classes that are
// archetype roots: a declaring class or any descendant of one, abstract
// classes left out.
func bmmArchetypeRoots(t *testing.T) (declarers, concrete []string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "resources", "bmm", "openehr_rm_1.2.0.bmm.json"))
	if err != nil {
		t.Fatalf("read vendored RM BMM: %v", err)
	}
	var schema struct {
		ClassDefinitions map[string]struct {
			Ancestors  []string          `json:"ancestors"`
			IsAbstract bool              `json:"is_abstract"`
			Invariants map[string]string `json:"invariants"`
		} `json:"class_definitions"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode vendored RM BMM: %v", err)
	}
	classes := schema.ClassDefinitions
	for name, c := range classes {
		expr, ok := c.Invariants["Is_archetype_root"]
		if !ok {
			continue
		}
		if expr != "is_archetype_root" {
			t.Errorf("BMM %s.Is_archetype_root = %q, want %q: the floor reads the invariant as fixing is_archetype_root true", name, expr, "is_archetype_root")
		}
		declarers = append(declarers, name)
	}
	memo := map[string]bool{}
	var isRoot func(string) bool
	isRoot = func(name string) bool {
		if v, seen := memo[name]; seen {
			return v
		}
		memo[name] = false // guards a cyclic ancestry
		c, ok := classes[name]
		if !ok {
			return false
		}
		_, declares := c.Invariants["Is_archetype_root"]
		root := declares || slices.ContainsFunc(c.Ancestors, isRoot)
		memo[name] = root
		return root
	}
	for name, c := range classes {
		if !c.IsAbstract && isRoot(name) {
			concrete = append(concrete, name)
		}
	}
	slices.Sort(declarers)
	slices.Sort(concrete)
	return declarers, concrete
}

// TestValidateRM_ArchetypeRootClassesMatchBMM pins the floor's closed list of
// archetype-root classes to the vendored BMM (REQ-112, ADR 0001). The BMM gives
// the declaring classes and their concrete descendants; this test checks that
// they are the ones the spec names, and then runs the floor over every
// registered LOCATABLE concrete: a zero value of a root class must report
// `is_archetype_root` at /archetype_details, any other LOCATABLE must not
// (the spec's MUST NOT: FOLDER, PARTY_RELATIONSHIP, GENERIC_ENTRY,
// PARTY_IDENTITY, CONTACT, ADDRESS, CAPABILITY and the data structures), and
// a value of either kind carrying archetype_details must not. A BMM bump that
// adds a root class fails here until the floor's list gains it.
func TestValidateRM_ArchetypeRootClassesMatchBMM(t *testing.T) {
	declarers, roots := bmmArchetypeRoots(t)
	if want := []string{"COMPOSITION", "EHR_ACCESS", "EHR_STATUS", "ENTRY", "PARTY"}; !slices.Equal(declarers, want) {
		t.Errorf("BMM Is_archetype_root declarers = %v, want %v: update the floor's closed list and the spec together (ADR 0001)", declarers, want)
	}
	wantRoots := []string{
		"ACTION", "ADMIN_ENTRY", "AGENT", "COMPOSITION", "EHR_ACCESS", "EHR_STATUS", "EVALUATION",
		"GROUP", "INSTRUCTION", "OBSERVATION", "ORGANISATION", "PERSON", "ROLE",
	}
	if !slices.Equal(roots, wantRoots) {
		t.Errorf("BMM concrete archetype-root classes = %v, want %v: update the floor's closed list and the spec together (ADR 0001)", roots, wantRoots)
	}

	isRoot := map[string]bool{}
	for _, name := range roots {
		isRoot[name] = true
	}
	swept := map[string]bool{}
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		v := ctor()
		loc, ok := v.(rm.MutableLocatable)
		if !ok {
			if isRoot[name] {
				t.Errorf("%s is a BMM archetype root but its registered Go type %T is not a LOCATABLE", name, v)
			}
			continue
		}
		swept[name] = true
		if got := reportsArchetypeRoot(validation.ValidateRM(v)); got != isRoot[name] {
			t.Errorf("ValidateRM(zero %s) reports is_archetype_root at /archetype_details = %v, want %v", name, got, isRoot[name])
		}
		loc.SetArchetypeDetails(&rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-" + name + ".x.v1"}, RMVersion: "1.1.0"})
		if reportsArchetypeRoot(validation.ValidateRM(v)) {
			t.Errorf("ValidateRM(%s with archetype_details) reports is_archetype_root; want none", name)
		}
	}
	for _, name := range roots {
		if !swept[name] {
			t.Errorf("BMM archetype root %s has no registered LOCATABLE concrete, so the floor cannot reach it", name)
		}
	}
	// The spec's named non-roots the SDK models must have been swept, so the
	// MUST NOT above is not vacuous. EXTRACT is not registered by the SDK.
	for _, name := range []string{
		"FOLDER", "PARTY_RELATIONSHIP", "GENERIC_ENTRY", "PARTY_IDENTITY", "CONTACT", "ADDRESS", "CAPABILITY",
		"SECTION", "ACTIVITY", "HISTORY", "POINT_EVENT", "INTERVAL_EVENT",
		"ITEM_TREE", "ITEM_LIST", "ITEM_SINGLE", "ITEM_TABLE", "CLUSTER", "ELEMENT",
	} {
		if !swept[name] || isRoot[name] {
			t.Errorf("non-root %s: swept = %v, root = %v; want swept and not a root", name, swept[name], isRoot[name])
		}
	}
	if len(swept) < len(roots) {
		t.Fatalf("swept %d LOCATABLE concretes (%v), fewer than the %d roots", len(swept), slices.Sorted(maps.Keys(swept)), len(roots))
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

package validation_test

// rmfloor_archetype_test.go: REQ-112 — the RM floor's archetype-root and
// ARCHETYPED catalogue rows. An archetype root must carry archetype_details
// (LOCATABLE.Archetyped_valid): an object of a class that declares the RM
// Is_archetype_root invariant, one whose archetype_node_id is an archetype id,
// and one that carries the details. At a root, archetype_details names the
// archetype archetype_node_id names (archetype_root_id). An ARCHETYPED
// wherever it sits must carry a non-empty archetype_id and rm_version. The
// EHR_STATUS rows through both entries, the Bytes entry's key presence
// included, are PROBE-081's (rmfloor_bytes_test.go).

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

// TestValidateRM_ArchetypeRootClassesMatchSharedList checks that the floor
// reports the archetype-root rule by class on exactly the classes of the
// shared closed list (REQ-112, ADR 0001), which internal/rmroots pins to the
// vendored BMM's Is_archetype_root declarations. It runs the floor over
// every registered LOCATABLE concrete: a zero value, whose archetype_node_id
// is empty and so no archetype id, must report `is_archetype_root` at
// /archetype_details on a root class and on no other LOCATABLE (FOLDER,
// PARTY_RELATIONSHIP, GENERIC_ENTRY, PARTY_IDENTITY, CONTACT, ADDRESS,
// CAPABILITY and the data structures), and a value of either kind carrying
// archetype_details must not. A floor that stops reporting a root class, or
// reports by class one the list leaves out, fails here.
func TestValidateRM_ArchetypeRootClassesMatchSharedList(t *testing.T) {
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
// archetype-root rule on their own root (REQ-112): EHR_ACCESS, a demographic
// PARTY and a FOLDER whose archetype_node_id is an archetype id, without
// archetype_details, are reported; a FOLDER whose archetype_node_id is an
// at-code is not.
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
		{"ValidateRMFolder(archetype id)", func() validation.Result {
			return validation.ValidateRMFolder(&rm.Folder{ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1", Name: name})
		}, true},
		{"ValidateRMFolder(at-code)", func() validation.Result {
			return validation.ValidateRMFolder(&rm.Folder{ArchetypeNodeID: "at0001", Name: name})
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

// sectionJSON returns a SECTION holding items, verbatim; details is its
// archetype_details member value (empty leaves it out).
func sectionJSON(details string, items ...string) string {
	member := ""
	if details != "" {
		member = `"archetype_details":` + details + `,`
	}
	return `{"_type":"SECTION","name":{"_type":"DV_TEXT","value":"Findings"},
		"archetype_node_id":"openEHR-EHR-SECTION.adhoc.v1",` + member + `
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
// archetype root by its class and needs archetype_details; a SECTION or a
// CLUSTER whose archetype_node_id is an archetype id is a root by its node id
// and needs it too; and an ARCHETYPED a node carries must be complete.
func TestValidateRM_NestedArchetypeRootsAndArchetyped(t *testing.T) {
	const (
		compositionID = "openEHR-EHR-COMPOSITION.encounter.v1"
		evaluationID  = "openEHR-EHR-EVALUATION.problem_diagnosis.v1"
		sectionID     = "openEHR-EHR-SECTION.adhoc.v1"
		clusterID     = "openEHR-EHR-CLUSTER.anatomical_location.v1"
		incomplete    = `{"_type":"ARCHETYPED","archetype_id":{"_type":"ARCHETYPE_ID","value":""},"rm_version":""}`
	)
	root := archetypedJSON(compositionID)
	section := archetypedJSON(sectionID)
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "every root carries archetype_details",
			body: compositionJSON(root, sectionJSON(section, evaluationJSON(archetypedJSON(evaluationID), clusterJSON(archetypedJSON(clusterID))))),
		},
		{
			name: "SECTION with an archetype id and no archetype_details",
			body: compositionJSON(root, sectionJSON("", evaluationJSON(archetypedJSON(evaluationID)))),
			want: []string{"is_archetype_root /content[openEHR-EHR-SECTION.adhoc.v1]/archetype_details"},
		},
		{
			name: "CLUSTER with an archetype id and no archetype_details",
			body: compositionJSON(root, evaluationJSON(archetypedJSON(evaluationID), clusterJSON(""))),
			want: []string{"is_archetype_root /content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/data/items[openEHR-EHR-CLUSTER.anatomical_location.v1]/archetype_details"},
		},
		{
			name: "COMPOSITION root without archetype_details",
			body: compositionJSON("", evaluationJSON(archetypedJSON(evaluationID))),
			want: []string{"is_archetype_root /archetype_details"},
		},
		{
			name: "EVALUATION in content without archetype_details",
			body: compositionJSON(root, evaluationJSON("")),
			want: []string{"is_archetype_root /content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/archetype_details"},
		},
		{
			name: "EVALUATION in a SECTION without archetype_details",
			body: compositionJSON(root, sectionJSON(section, evaluationJSON(archetypedJSON(evaluationID))), sectionJSON(section, evaluationJSON(""))),
			want: []string{"is_archetype_root /content[2]/items[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/archetype_details"},
		},
		{
			name: "EVALUATION with an incomplete ARCHETYPED",
			body: compositionJSON(root, evaluationJSON(incomplete)),
			want: []string{
				"required /content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/archetype_details/archetype_id/value",
				"rm_version_valid /content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/archetype_details/rm_version",
			},
		},
		{
			name: "CLUSTER with an incomplete ARCHETYPED",
			body: compositionJSON(root, evaluationJSON(archetypedJSON(evaluationID), clusterJSON(incomplete))),
			want: []string{
				"required /content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/data/items[openEHR-EHR-CLUSTER.anatomical_location.v1]/archetype_details/archetype_id/value",
				"rm_version_valid /content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]/data/items[openEHR-EHR-CLUSTER.anatomical_location.v1]/archetype_details/rm_version",
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

// TestValidateJSON_ArchetypedValidPerInstance checks LOCATABLE.Archetyped_valid
// per instance (REQ-112 Archetype roots, REQ-171 Finding identity) on the four
// bodies of issue 466 and their controls, each decoded with its report of
// missing members. A FOLDER, which no class invariant makes a root, is one
// when its archetype_node_id is an archetype id, so it must carry
// archetype_details, at the root and nested; on any root, by class or by node
// id, archetype_details must name the archetype archetype_node_id names, and
// so must the details an at-code node carries. A class root without details
// draws its class rule alone, an archetype_node_id that only looks like an
// archetype id draws nothing, and an empty id draws only its `required`. No
// issue names a submitted id but by the path's node predicate (REQ-168).
func TestValidateJSON_ArchetypedValidPerInstance(t *testing.T) {
	t.Parallel()
	const (
		name    = `"name":{"_type":"DV_TEXT","value":"x"}`
		episode = "openEHR-EHR-FOLDER.episode.v1"
		generic = "openEHR-EHR-FOLDER.generic.v1"
		status  = "openEHR-EHR-EHR_STATUS.generic.v1"
		other   = "openEHR-EHR-EHR_STATUS.other.v1"
	)
	details := func(id string) string { return `"archetype_details":` + archetypedJSON(id) }
	folder := func(nodeID string, members ...string) string {
		return `{"_type":"FOLDER","archetype_node_id":"` + nodeID + `",` + strings.Join(append([]string{name}, members...), ",") + `}`
	}
	ehrStatus := func(members ...string) string {
		return `{"_type":"EHR_STATUS","archetype_node_id":"` + status + `","subject":{"_type":"PARTY_SELF"},"is_queryable":true,"is_modifiable":true,` +
			strings.Join(append([]string{name}, members...), ",") + `}`
	}
	newFolder := func() any { return new(rm.Folder) }
	newStatus := func() any { return new(rm.EHRStatus) }
	cases := []struct {
		name string
		body string
		root func() any
		want []string
	}{
		{
			name: "issue 1: FOLDER root with an archetype id and no archetype_details",
			body: folder(episode),
			root: newFolder,
			want: []string{"is_archetype_root /archetype_details rm LOCATABLE.Archetyped_valid"},
		},
		{
			name: "issue 2: the same FOLDER nested in another",
			body: folder("at0001", `"folders":[`+folder(episode)+`]`),
			root: newFolder,
			want: []string{"is_archetype_root /folders[openEHR-EHR-FOLDER.episode.v1]/archetype_details rm LOCATABLE.Archetyped_valid"},
		},
		{
			name: "issue 3: FOLDER root whose archetype_details names another archetype",
			body: folder(episode, details(generic)),
			root: newFolder,
			want: []string{"archetype_root_id /archetype_details/archetype_id/value rm archetype_root_id"},
		},
		{
			name: "issue 4: EHR_STATUS whose archetype_details names another archetype",
			body: ehrStatus(details(other)),
			root: newStatus,
			want: []string{"archetype_root_id /archetype_details/archetype_id/value rm archetype_root_id"},
		},
		{
			name: "FOLDER with an at-code that carries archetype_details",
			body: folder("at0001", details(generic)),
			root: newFolder,
			want: []string{"archetype_root_id /archetype_details/archetype_id/value rm archetype_root_id"},
		},
		{
			name: "control: EHR_STATUS without archetype_details draws its class rule alone",
			body: ehrStatus(),
			root: newStatus,
			want: []string{"is_archetype_root /archetype_details rm EHR_STATUS.Is_archetype_root"},
		},
		{
			name: "control: EHR_STATUS whose archetype_details names its archetype",
			body: ehrStatus(details(status)),
			root: newStatus,
		},
		{
			name: "control: FOLDER whose archetype_details names its archetype",
			body: folder(episode, details(episode)),
			root: newFolder,
		},
		{
			name: "control: FOLDER with an at-code and no archetype_details",
			body: folder("at0001"),
			root: newFolder,
		},
		{
			name: "control: FOLDER whose node id has no version, so is no archetype id",
			body: folder("openEHR-EHR-FOLDER.episode"),
			root: newFolder,
		},
		{
			name: "control: FOLDER whose archetype_details has an empty archetype id",
			body: folder(episode, `"archetype_details":{"_type":"ARCHETYPED","archetype_id":{"_type":"ARCHETYPE_ID","value":""},"rm_version":"1.1.0"}`),
			root: newFolder,
			want: []string{"required /archetype_details/archetype_id/value rm mandatory"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := validation.ValidateJSON([]byte(tc.body), tc.root(), nil)
			if got := identityKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateJSON(%s) issues (code path layer rule) = %q, want %q", tc.body, got, tc.want)
			}
			for _, i := range r.Issues {
				for _, id := range []string{episode, generic, status, other} {
					if strings.Contains(i.Detail, id) {
						t.Errorf("%s at %q: Detail %q names the submitted id %q", i.Code, i.Path, i.Detail, id)
					}
				}
			}
			assertValueFree(t, r, generic, other)
		})
	}
}

// TestValidateRM_ArchetypeRootPerInstanceOnEveryLocatable sweeps every
// registered LOCATABLE concrete (REQ-112 Archetype roots, REQ-171 Finding
// identity). Whatever its class, a node whose archetype_node_id is an
// archetype id and that carries no archetype_details draws one
// `is_archetype_root` at /archetype_details: under the Is_archetype_root
// invariant its class declares or inherits on a root class, and under
// LOCATABLE.Archetyped_valid on any other. One whose archetype_details names
// another archetype, or that carries details under an at-code, draws one
// `archetype_root_id` at /archetype_details/archetype_id/value; one whose
// details name its archetype draws neither. Under an at-code with no
// details, only a root class draws its rule.
func TestValidateRM_ArchetypeRootPerInstanceOnEveryLocatable(t *testing.T) {
	t.Parallel()
	declarers, err := archetypeRootDeclarers()
	if err != nil {
		t.Fatal(err)
	}
	mismatch := []string{"archetype_root_id /archetype_details/archetype_id/value rm archetype_root_id"}
	swept := 0
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		if _, ok := ctor().(rm.MutableLocatable); !ok {
			continue
		}
		swept++
		id := "openEHR-EHR-" + name + ".x.v1"
		absent := []string{"is_archetype_root /archetype_details rm LOCATABLE.Archetyped_valid"}
		var atCode []string
		if d, root := declarers[name]; root {
			absent = []string{"is_archetype_root /archetype_details rm " + d + ".Is_archetype_root"}
			atCode = absent
		}
		for _, tc := range []struct {
			what    string
			nodeID  string
			details string
			want    []string
		}{
			{what: "an archetype id and no archetype_details", nodeID: id, want: absent},
			{what: "archetype_details that name its archetype", nodeID: id, details: id},
			{what: "archetype_details that name another archetype", nodeID: id, details: "openEHR-EHR-" + name + ".y.v1", want: mismatch},
			{what: "an at-code and archetype_details", nodeID: "at0001", details: id, want: mismatch},
			{what: "an at-code and no archetype_details", nodeID: "at0001", want: atCode},
		} {
			v := ctor()
			loc := v.(rm.MutableLocatable)
			loc.SetArchetypeNodeID(tc.nodeID)
			if tc.details != "" {
				loc.SetArchetypeDetails(&rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: tc.details}, RMVersion: "1.1.0"})
			}
			got := slices.DeleteFunc(identityKeys(validation.ValidateRM(v).Issues), func(k string) bool {
				return !strings.Contains(k, " /archetype_details")
			})
			if !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s with %s) issues at /archetype_details = %q, want %q", name, tc.what, got, tc.want)
			}
		}
	}
	if swept < len(declarers) {
		t.Fatalf("swept %d LOCATABLE concretes, fewer than the %d archetype-root classes; the sweep has gone blind", swept, len(declarers))
	}
}

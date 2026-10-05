package instanceprobes_test

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// socialContentArchetypeIDs are the archetype ids social.opt names on
// the archetype roots of COMPOSITION.content.
var socialContentArchetypeIDs = []string{
	"openEHR-EHR-OBSERVATION.livingsituation.v1",
	"openEHR-EHR-OBSERVATION.languageproficiency.v1",
	"openEHR-EHR-OBSERVATION.participationinsociety.v1",
	"openEHR-EHR-OBSERVATION.helpfromothers.v1",
	"openEHR-EHR-EVALUATION.familysituation.v1",
	"openEHR-EHR-EVALUATION.education_summary.v1",
}

// TestREQ100_REQ107_TArchetypeRootEntriesPassRMFloor regenerates social.opt
// with its archetype roots spelled T_ARCHETYPE_ROOT, as the original export
// wrote them. REQ-100 reads that spelling as an archetype root, so every
// generated entry carries archetype_details and the body passes the REQ-107
// RM floor (validation.ValidateRM).
func TestREQ100_REQ107_TArchetypeRootEntriesPassRMFloor(t *testing.T) {
	raw, err := os.ReadFile(fixtures.TemplateOptForName("social"))
	if err != nil {
		t.Fatalf("ReadFile(social.opt): %v", err)
	}
	from, to := []byte(`xsi:type="C_ARCHETYPE_ROOT"`), []byte(`xsi:type="T_ARCHETYPE_ROOT"`)
	if got, want := bytes.Count(raw, from), 7; got != want {
		t.Fatalf("social.opt holds %d %s, want %d", got, from, want)
	}
	opt, err := template.ParseOPT(bytes.NewReader(bytes.ReplaceAll(raw, from, to)))
	if err != nil {
		t.Fatalf("ParseOPT(social.opt with T_ARCHETYPE_ROOT): %v", err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(social.opt with T_ARCHETYPE_ROOT): %v", err)
	}

	for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
		t.Run(policy.String(), func(t *testing.T) {
			out, err := instance.Generate(t.Context(), compiled, instance.Options{
				Policy:    policy,
				Language:  "en",
				Territory: "NL",
				Composer:  fixedComposer(),
				Now:       ratchetNow,
			})
			if err != nil {
				t.Fatalf("Generate(%s): %v", policy, err)
			}
			comp, err := instance.AsComposition(out)
			if err != nil {
				t.Fatalf("AsComposition(%s): %v", policy, err)
			}
			if res := validation.ValidateRM(comp); !res.OK {
				t.Errorf("ValidateRM(%s) = not OK, want OK; issues: %s", policy, summariseIssueList(res.Issues))
			}

			body, err := canjson.Marshal(comp)
			if err != nil {
				t.Fatalf("canjson.Marshal(%s): %v", policy, err)
			}
			var tree map[string]any
			if err := jsonv2.Unmarshal(body, &tree); err != nil {
				t.Fatalf("decode canonical JSON (%s): %v", policy, err)
			}
			content, _ := tree["content"].([]any)
			if len(content) == 0 {
				t.Fatalf("Generate(%s): composition has no content entries, want at least one", policy)
			}
			for i, entry := range content {
				e, _ := entry.(map[string]any)
				typ, _ := e["_type"].(string)
				nodeID, _ := e["archetype_node_id"].(string)
				details, ok := e["archetype_details"].(map[string]any)
				if !ok {
					t.Errorf("Generate(%s): /content[%d] (%s, archetype_node_id %q) has no archetype_details", policy, i, typ, nodeID)
					continue
				}
				archetypeID, _ := details["archetype_id"].(map[string]any)
				id, _ := archetypeID["value"].(string)
				if !slices.Contains(socialContentArchetypeIDs, id) {
					t.Errorf("Generate(%s): /content[%d] archetype_details.archetype_id = %q, want one of %q", policy, i, id, socialContentArchetypeIDs)
				}
				if nodeID != id {
					t.Errorf("Generate(%s): /content[%d] archetype_node_id = %q, want the archetype id %q", policy, i, nodeID, id)
				}
			}
			if n := countElements(tree); n == 0 {
				t.Errorf("Generate(%s): body holds no ELEMENT, want at least one", policy)
			}
		})
	}
}

func summariseIssueList(issues []validation.Issue) string {
	var buf strings.Builder
	for i, iss := range issues {
		if i == 5 {
			buf.WriteString(" ...")
			break
		}
		if i > 0 {
			buf.WriteString(" | ")
		}
		buf.WriteString(iss.Code)
		buf.WriteByte('@')
		buf.WriteString(iss.Path)
	}
	return buf.String()
}

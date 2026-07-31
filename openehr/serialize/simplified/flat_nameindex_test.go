package simplified

// REQ-053 / REQ-116 — the FLAT decode name index must keep answering to the
// bare (un-predicated) path spelling after REQ-116 Phase 3 started emitting
// name predicates on compiled paths.
//
// Decode composes its lookup key from the incoming FLAT segments, which key
// on archetype id / at-code and never carry a name. Indexing solely by the
// compiled path therefore made every named node's entry unreachable, and
// LOCATABLE.name silently stopped being repopulated for the 9 vendored
// templates that pin names. No FLAT fixture covers a name-pinning template,
// so the whole suite stayed green through the regression — hence this test.

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func TestBareAQLPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no predicate", "/context/other_context", "/context/other_context"},
		{"id only", "/content[openEHR-EHR-SECTION.adhoc.v1]", "/content[openEHR-EHR-SECTION.adhoc.v1]"},
		{
			"named archetype root",
			"/content[openEHR-EHR-SECTION.adhoc.v1,'Symptome']",
			"/content[openEHR-EHR-SECTION.adhoc.v1]",
		},
		{"named at-code", "/data/items[at0002,'Causative agent']", "/data/items[at0002]"},
		{
			"several segments",
			"/content[openEHR-EHR-SECTION.adhoc.v1,'Symptome']/items[at0009,'Reaction details']/value",
			"/content[openEHR-EHR-SECTION.adhoc.v1]/items[at0009]/value",
		},
		{
			// A comma inside the quoted name is literal — the corona golden
			// carries one — and must not end the predicate early.
			"comma inside the name",
			"/content[openEHR-EHR-SECTION.adhoc.v1,'Kontakt zu Menschen, die dort waren']",
			"/content[openEHR-EHR-SECTION.adhoc.v1]",
		},
		{
			"escaped quote inside the name",
			`/items[at0001,'O\'Brien score']/value`,
			"/items[at0001]/value",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := bareAQLPath(tc.in); got != tc.want {
				t.Errorf("bareAQLPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A name-pinning template must expose every rubric under the bare spelling
// decode actually looks up, and additionally under the compiled one.
func TestBuildNameIndex_AnswersBareSpelling(t *testing.T) {
	parsed, err := template.ParseFile(fixtures.TemplateOpt("IDCR -  Adverse Reaction List.v1"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	idx := buildNameIndex(c)

	var predicated int
	for key, want := range idx {
		if !strings.Contains(key, ",'") {
			continue
		}
		predicated++
		bare := bareAQLPath(key)
		got, ok := idx[bare]
		if !ok {
			t.Errorf("bare spelling %q missing — decode's lookup key would miss", bare)
			continue
		}
		if got != want {
			t.Errorf("idx[%q] = %q but idx[%q] = %q — rubric disagrees across spellings",
				key, want, bare, got)
		}
	}
	if predicated == 0 {
		t.Fatal("fixture pins no names any more — pick another name-pinning template")
	}
}

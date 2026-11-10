package simplified_test

// REQ-053 — the defaults a WithTemplate decode gives a body that leaves them
// out. The composition category is the one the template fixes, where it fixes
// one: a persistent template's composition is persistent, not the 433|event
// default.

import (
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The corpus templates the cases decode against. Both hold one OBSERVATION
// with one HISTORY and one EVENT; the first fixes the category 433|event, the
// second 431|persistent.
const (
	minimalObservationID = "minimal_observation.en.v1"
	persistentMinimalID  = "persistent_minimal.en.v1"
	// persistentCategory is the persistent template's category constraint, the
	// one code its C_CODE_PHRASE lists.
	persistentCategory = "<code_list>431</code_list>"
)

// defaultsBody is issue 420's FLAT body for the template whose Web Template
// root is root: the three ctx/ fields and the one text leaf, with no ctx/time
// and no category. extra is merged over it.
func defaultsBody(root string, extra map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":           "en",
		"ctx/territory":          "US",
		"ctx/composer_name":      "Silvia Blake",
		root + "/minimal:0/text": "first value",
	}
	maps.Copy(body, extra)
	return body
}

// editedCorpusTemplate compiles the corpus OPT id with its one occurrence of
// old replaced by repl, and builds its Web Template.
func editedCorpusTemplate(t *testing.T, id, old, repl string) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(replaceExactlyOnce(t, string(raw), old, repl)))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate(edited %s): %v", id, err)
	}
	return compileTemplate(t, id, model)
}

// wantCategory fails the test unless comp's category is code in the openEHR
// terminology, with text as its value.
func wantCategory(t *testing.T, comp *rm.Composition, code, text string) {
	t.Helper()
	got := comp.Category
	if got.DefiningCode.CodeString != code || got.DefiningCode.TerminologyID.Value != "openehr" || got.Value != text {
		t.Errorf("category = %s::%s|%s|, want openehr::%s|%s|",
			got.DefiningCode.TerminologyID.Value, got.DefiningCode.CodeString, got.Value, code, text)
	}
}

// TestREQ053TemplateFixesTheCategory — REQ-053: with the template, a body
// that spells no category takes the one the template fixes, a C_CODE_PHRASE
// listing one openehr code, with that code's openEHR rubric as its text. Where
// the template fixes none, the category stays 433|event.
func TestREQ053TemplateFixesTheCategory(t *testing.T) {
	const startTime = "2026-01-01T00:00:00Z"
	persistentWT, persistent := templateOf(t, persistentMinimalID)
	eventWT, event := templateOf(t, minimalObservationID)
	twoCodesWT, twoCodes := editedCorpusTemplate(t, persistentMinimalID, persistentCategory,
		persistentCategory+"<code_list>433</code_list>")
	localWT, local := editedCorpusTemplate(t, persistentMinimalID,
		"<value>openehr</value>\n            </terminology_id>\n            "+persistentCategory,
		"<value>local</value>\n            </terminology_id>\n            "+persistentCategory)
	cases := []struct {
		name      string
		wt        *webtemplate.WebTemplate
		compiled  *templatecompile.Compiled
		root      string
		code      string
		text      string
		validates bool
	}{
		{"a persistent template", persistentWT, persistent, "persistent_minimal", "431", "persistent", true},
		{"an event template", eventWT, event, "minimal", "433", "event", true},
		{"a template listing two codes", twoCodesWT, twoCodes, "persistent_minimal", "433", "event", true},
		{"a template fixing a local code", localWT, local, "persistent_minimal", "433", "event", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := defaultsBody(tc.root, map[string]any{"ctx/time": startTime})
			for name, decode := range map[string]func([]byte, *webtemplate.WebTemplate, ...simplified.Option) (*rm.Composition, error){
				"FLAT":       simplified.UnmarshalFlat,
				"STRUCTURED": simplified.UnmarshalStructured,
			} {
				data := mustJSON(t, body)
				if name == "STRUCTURED" {
					var err error
					if data, err = simplified.FlatToStructured(data); err != nil {
						t.Fatalf("FlatToStructured: %v", err)
					}
				}
				comp, err := decode(data, tc.wt, simplified.WithTemplate(tc.compiled))
				if err != nil {
					t.Fatalf("%s decode: %v", name, err)
				}
				wantCategory(t, comp, tc.code, tc.text)
				if r := validation.ValidateComposition(comp, tc.compiled); r.OK != tc.validates {
					t.Errorf("%s: ValidateComposition OK = %v, want %v; issues=%v", name, r.OK, tc.validates, r.Issues)
				}
			}
		})
	}
}

// TestREQ053BodyCategoryKeptAsWritten — REQ-053: a category the body spells is
// kept as written, even where the template fixes another; validation judges
// it.
func TestREQ053BodyCategoryKeptAsWritten(t *testing.T) {
	wt, compiled := templateOf(t, persistentMinimalID)
	body := defaultsBody("persistent_minimal", map[string]any{
		"ctx/time":                                "2026-01-01T00:00:00Z",
		"persistent_minimal/category|code":        "433",
		"persistent_minimal/category|value":       "event",
		"persistent_minimal/category|terminology": "openehr",
	})
	comp := decodeWith(t, wt, body, simplified.WithTemplate(compiled))
	wantCategory(t, comp, "433", "event")
}

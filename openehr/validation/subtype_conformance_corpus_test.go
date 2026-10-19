package validation_test

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-102 (Validation dimensions: RM type match) — a DV_CODED_TEXT
// satisfies an OPT child declared as DV_TEXT. These tests take the Large
// instance the generator builds for a vendored template, code one or more of
// its DV_TEXT values on the canonical JSON, as a client holding wire data
// would, and validate the result.

// minCorpusSwept is the number of vendored templates the corpus sweep
// measured: their Large instance validates clean and holds a DV_TEXT
// ELEMENT value. A sweep that covers fewer has lost templates, and passes
// for that reason rather than for the rule it checks. Template validation
// includes the RM floor, so TestPerson.v2 is not counted: its Large
// instance breaks the floor's media type rule, a known generator gap the
// REQ-107 corpus ratchet records.
const minCorpusSwept = 47

// generateExample builds the Large instance of c, with the composer and
// territory a COMPOSITION root needs; any other root ignores them.
func generateExample(t *testing.T, c *templatecompile.Compiled) (any, error) {
	t.Helper()
	return instance.Generate(t.Context(), c, instance.Options{
		Policy:    instance.Large,
		Language:  "en",
		Territory: "NL",
		Composer:  &rm.PartyIdentified{Name: new("Test Composer")},
		Now:       time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC),
	})
}

// validateRoot validates root against c: a COMPOSITION through
// ValidateComposition, any other root through Validate.
func validateRoot(root any, c *templatecompile.Compiled) validation.Result {
	if comp, ok := root.(*rm.Composition); ok {
		return validation.ValidateComposition(comp, c)
	}
	return validation.Validate(root, c)
}

// codeText turns a canonical-JSON DV_TEXT into a DV_CODED_TEXT with the
// same members, coded as at9999 in the local terminology.
func codeText(text map[string]any) {
	text["_type"] = "DV_CODED_TEXT"
	text["defining_code"] = map[string]any{
		"_type":          "CODE_PHRASE",
		"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "local"},
		"code_string":    "at9999",
	}
}

// withNodeID picks the ELEMENTs whose archetype_node_id is id.
func withNodeID(id string) func(element map[string]any) bool {
	return func(element map[string]any) bool { return element["archetype_node_id"] == id }
}

// anyElement picks every ELEMENT.
func anyElement(map[string]any) bool { return true }

// editElementTexts calls edit on the DV_TEXT held under attribute attr of
// every ELEMENT in root's canonical JSON that pick accepts, then decodes the
// edited JSON into a new root of root's type. It returns that root and the
// number of texts it edited; with none edited it returns root itself.
func editElementTexts(t *testing.T, root any, attr string, pick func(element map[string]any) bool, edit func(text map[string]any)) (any, int) {
	t.Helper()
	raw, err := canjson.Marshal(root)
	if err != nil {
		t.Fatalf("canjson.Marshal(%T): %v", root, err)
	}
	var tree any
	if err := jsonv2.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("decode the canonical JSON of %T: %v", root, err)
	}
	n := editTree(tree, attr, pick, edit)
	if n == 0 {
		return root, 0
	}
	edited, err := jsonv2.Marshal(tree)
	if err != nil {
		t.Fatalf("encode the edited JSON of %T: %v", root, err)
	}
	out, err := decodeLike(edited, root)
	if err != nil {
		t.Fatalf("decode the edited JSON of %T: %v", root, err)
	}
	return out, n
}

// editTree walks a decoded canonical-JSON tree for editElementTexts and
// returns the number of texts it edited.
func editTree(v any, attr string, pick func(map[string]any) bool, edit func(map[string]any)) int {
	n := 0
	switch node := v.(type) {
	case map[string]any:
		if node["_type"] == "ELEMENT" && pick(node) {
			if text, ok := node[attr].(map[string]any); ok && text["_type"] == "DV_TEXT" {
				edit(text)
				n++
			}
		}
		for _, child := range node {
			n += editTree(child, attr, pick, edit)
		}
	case []any:
		for _, child := range node {
			n += editTree(child, attr, pick, edit)
		}
	}
	return n
}

// decodeLike decodes canonical JSON into a new value of like's RM type: one
// of the root types the vendored templates declare.
func decodeLike(raw []byte, like any) (any, error) {
	switch like.(type) {
	case *rm.Composition:
		return decodeInto(raw, new(rm.Composition))
	case *rm.Person:
		return decodeInto(raw, new(rm.Person))
	case *rm.Address:
		return decodeInto(raw, new(rm.Address))
	}
	return nil, fmt.Errorf("no decode target for a %T root", like)
}

func decodeInto[T any](raw []byte, dst *T) (any, error) {
	if err := canjson.Unmarshal(raw, dst); err != nil {
		return nil, err
	}
	return dst, nil
}

// REQ-102 — issue #248: the Large instance of BMI.opt validates clean,
// and stays clean once the DV_TEXT value of its protocol ELEMENT at0010 is
// coded, because a DV_CODED_TEXT satisfies the DV_TEXT the OPT declares
// there. It used to be reported as rm_type_mismatch at
// /content[openEHR-EHR-OBSERVATION.body_mass_index.v1]/protocol/items[at0010]/value.
func TestValidateComposition_REQ102_CodedTextElementValueUnderTextChild(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "BMI")
	root, err := generateExample(t, c)
	if err != nil {
		t.Fatalf("Generate(BMI): %v", err)
	}
	if r := validateRoot(root, c); !r.OK {
		t.Fatalf("precondition: the Large instance of BMI has issues %q, want none", issueKeys(r.Issues))
	}
	coded, n := editElementTexts(t, root, "value", withNodeID("at0010"), codeText)
	if n == 0 {
		t.Fatal("the Large instance of BMI holds no DV_TEXT value on ELEMENT at0010; update the node id")
	}
	r := validateRoot(coded, c)
	if got := issueKeys(r.Issues); len(got) != 0 {
		t.Errorf("ValidateComposition(BMI with a DV_CODED_TEXT value on ELEMENT at0010) issues = %q, want none", got)
	}
}

// REQ-102 — an ELEMENT name the OPT declares as DV_TEXT may be a
// DV_CODED_TEXT. The coded name binds to the DV_TEXT child and is walked
// there, so the child's C_STRING still checks its value.
func TestValidateComposition_REQ102_CodedTextNameUnderTextChild(t *testing.T) {
	t.Parallel()
	const textOpen = "Test_dv_text_open_constraint.v0"
	const at0031Name = "/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031]/name/value"
	renamed := func(text map[string]any) {
		codeText(text)
		text["value"] = "renamed"
	}
	tests := []struct {
		name     string
		template string
		nodeID   string
		edit     func(text map[string]any)
		want     []string
	}{
		{name: "coded name", template: textOpen, nodeID: "at0031", edit: codeText, want: []string{}},
		{
			name:     "coded name outside the C_STRING list",
			template: textOpen,
			nodeID:   "at0031",
			edit:     renamed,
			want:     []string{"primitive_not_in_list " + at0031Name},
		},
		{name: "coded names in a clinical template", template: "IDCR -  Adverse Reaction List.v1", nodeID: "at0002", edit: codeText, want: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompile(t, tc.template)
			root, err := generateExample(t, c)
			if err != nil {
				t.Fatalf("Generate(%s): %v", tc.template, err)
			}
			if r := validateRoot(root, c); !r.OK {
				t.Fatalf("precondition: the Large instance of %s has issues %q, want none", tc.template, issueKeys(r.Issues))
			}
			coded, n := editElementTexts(t, root, "name", withNodeID(tc.nodeID), tc.edit)
			if n == 0 {
				t.Fatalf("the Large instance of %s holds no DV_TEXT name on ELEMENT %s; update the node id", tc.template, tc.nodeID)
			}
			r := validateRoot(coded, c)
			got := issueKeys(r.Issues)
			want := make([]string, 0, n*len(tc.want))
			for range n {
				want = append(want, tc.want...)
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("ValidateComposition(%s with %d coded names on ELEMENT %s) issues = %q, want %q", tc.template, n, tc.nodeID, got, want)
			}
		})
	}
}

// REQ-102 — the sweep issue #248 measured, over the templates vendored in
// testkit/corpus/templates: for every one whose Large instance validates
// clean, coding every DV_TEXT ELEMENT value brings no rm_type_mismatch or
// alternative_mismatch. A DV_CODED_TEXT conforms to every child a DV_TEXT
// conforms to; a coded child's own constraints may still report the code.
func TestValidate_REQ102_CorpusCodedTextValues(t *testing.T) {
	t.Parallel()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	// The floor holds only for a full run: a -run pattern that selects
	// some templates enters fewer, and must not fail the parent for it.
	listed, entered, swept := 0, 0, 0
	for _, ref := range refs {
		if !strings.HasPrefix(ref.Name, "templates/") {
			continue
		}
		listed++
		t.Run(ref.Name, func(t *testing.T) {
			entered++
			opt, err := template.ParseFile(ref.Path)
			if err != nil {
				t.Skipf("ParseFile: %v", err)
			}
			c, err := templatecompile.Compile(opt)
			if err != nil {
				t.Skipf("Compile: %v", err)
			}
			root, err := generateExample(t, c)
			if err != nil {
				t.Skipf("Generate: %v", err)
			}
			if r := validateRoot(root, c); !r.OK {
				t.Skipf("the Large instance has issues %q", issueKeys(r.Issues))
			}
			coded, n := editElementTexts(t, root, "value", anyElement, codeText)
			if n == 0 {
				t.Skip("the Large instance holds no DV_TEXT ELEMENT value")
			}
			swept++
			for _, iss := range validateRoot(coded, c).Issues {
				if iss.Code == "rm_type_mismatch" || iss.Code == "alternative_mismatch" {
					t.Errorf("%d DV_TEXT values coded: %s at %s: %s", n, iss.Code, iss.Path, iss.Detail)
				}
			}
		})
	}
	if entered == listed && swept < minCorpusSwept {
		t.Errorf("the sweep covered %d of %d templates, want at least %d", swept, listed, minCorpusSwept)
	}
}

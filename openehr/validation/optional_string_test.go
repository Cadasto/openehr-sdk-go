package validation_test

import (
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// errorIssues returns the error-severity issues of r.
func errorIssues(r validation.Result) []validation.Issue {
	var out []validation.Issue
	for _, iss := range r.Issues {
		if iss.Severity == validation.Error {
			out = append(out, iss)
		}
	}
	return out
}

// TestREQ107_OptionalStringIdentifierSubfields is the REQ-107 / REQ-103
// check that the optional String attributes of DV_IDENTIFIER (issuer, type,
// assigner) are read as values: a filled one satisfies the template's
// STRING constraint instead of being reported as an rm_type_mismatch.
func TestREQ107_OptionalStringIdentifierSubfields(t *testing.T) {
	const id = "Test_dv_identifier_pattern_constraint.v0"
	c := mustCompile(t, id)
	raw, err := os.ReadFile(fixtures.CompositionJSON(id))
	if err != nil {
		t.Fatal(err)
	}
	decode := func(t *testing.T, body string) *rm.Composition {
		t.Helper()
		var comp rm.Composition
		if err := canjson.Unmarshal([]byte(body), &comp); err != nil {
			t.Fatalf("decode composition: %v", err)
		}
		return &comp
	}
	t.Run("matching", func(t *testing.T) {
		for _, iss := range errorIssues(validation.ValidateComposition(decode(t, string(raw)), c)) {
			t.Errorf("%s @ %s: %s", iss.Code, iss.Path, iss.Detail)
		}
	})
	// The template pins issuer, type and assigner to XYZ.*; a filled value
	// that does not match must be reported against the pattern, not read
	// as a pointer the validator cannot type.
	for _, attr := range []string{"issuer", "type", "assigner"} {
		t.Run("violating "+attr, func(t *testing.T) {
			old := `"` + attr + `": "XYZ"`
			if !strings.Contains(string(raw), old) {
				t.Fatalf("composition has no %s", old)
			}
			body := strings.ReplaceAll(string(raw), old, `"`+attr+`": "ABC"`)
			issues := errorIssues(validation.ValidateComposition(decode(t, body), c))
			if !containsCode(issues, "primitive_pattern_mismatch") {
				t.Errorf("issues = %v, want a primitive_pattern_mismatch issue", issues)
			}
		})
	}
}

// TestREQ107_OptionalStringDVTextFormatting is the REQ-107 / REQ-103 check
// that DV_TEXT.formatting is readable: a template listing XYZ and OPQ on
// formatting accepts a filled member, and rejects a filled non-member.
func TestREQ107_OptionalStringDVTextFormatting(t *testing.T) {
	const id = "Test_dv_text_list_constraint.v0"
	opt, err := os.ReadFile(fixtures.TemplateOptForName(id))
	if err != nil {
		t.Fatal(err)
	}
	// Move the one C_STRING list from DV_TEXT.value to DV_TEXT.formatting.
	const anchor = "<list>XYZ</list>"
	const tag = "<rm_attribute_name>value</rm_attribute_name>"
	at := strings.Index(string(opt), anchor)
	i := strings.LastIndex(string(opt)[:at], tag)
	if at < 0 || i < 0 {
		t.Fatalf("%s: no value attribute before %s", id, anchor)
	}
	edited := string(opt)[:i] + "<rm_attribute_name>formatting</rm_attribute_name>" + string(opt)[i+len(tag):]
	c := mustCompileSyntheticOPT(t, edited)

	rawJSON, err := os.ReadFile(fixtures.CompositionJSON(id))
	if err != nil {
		t.Fatal(err)
	}
	const filled = `"value": "OPQ"`
	if !strings.Contains(string(rawJSON), filled) {
		t.Fatalf("%s: composition has no %s", id, filled)
	}
	for _, tc := range []struct {
		name       string
		formatting string
		wantOK     bool
	}{
		{"member", "OPQ", true},
		{"non-member", "nope", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.ReplaceAll(string(rawJSON), filled, filled+`, "formatting": "`+tc.formatting+`"`)
			var comp rm.Composition
			if err := canjson.Unmarshal([]byte(body), &comp); err != nil {
				t.Fatalf("decode composition: %v", err)
			}
			issues := errorIssues(validation.ValidateComposition(&comp, c))
			if tc.wantOK {
				for _, iss := range issues {
					t.Errorf("%s @ %s: %s", iss.Code, iss.Path, iss.Detail)
				}
				return
			}
			if !containsCode(issues, "primitive_not_in_list") {
				t.Errorf("issues = %v, want a primitive_not_in_list issue", issues)
			}
		})
	}
}

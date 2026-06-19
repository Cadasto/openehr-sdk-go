package composition_test

import (
	"bytes"
	"context"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// gap12KnownFailures lists corpus OPTs for which NewSkeleton cannot yet
// produce a template-valid skeleton. Tripwire for SDK-GAP-12 — see
// docs/plans/2026-06-19-sdk-gap-12-newskeleton.md. When the SDK closes
// a gap the fixture starts succeeding and this test fails, prompting
// removal from the set.
var gap12KnownFailures = map[string]bool{
	"Referral Request.v1": true,
	"Demonstration.v1":    true,
	"social":              true,
}

func parseOPTBytes(b []byte) (*template.OperationalTemplate, error) {
	parsed, err := template.ParseOPT(bytes.NewReader(b))
	if err == nil {
		return parsed, nil
	}
	if !bytes.Contains(b, []byte("<OPERATIONAL_TEMPLATE")) {
		return nil, err
	}
	s := string(b)
	s = strings.Replace(s, "<OPERATIONAL_TEMPLATE", "<template", 1)
	if idx := strings.LastIndex(s, "</OPERATIONAL_TEMPLATE>"); idx >= 0 {
		s = s[:idx] + "</template>" + s[idx+len("</OPERATIONAL_TEMPLATE>"):]
	}
	return template.ParseOPT(bytes.NewReader([]byte(s)))
}

func parseGAP12Fixture(t *testing.T, name string) *template.OperationalTemplate {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("ReadFile %s: %v", name, err)
	}
	opt, err := parseOPTBytes(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return opt
}

func testComposerGap12() *rm.PartyIdentified {
	name := "Test Composer"
	return &rm.PartyIdentified{Name: &name}
}

// TestNewSkeleton_GAP12_tripwire records exactly which SDK-GAP-12 corpus
// fixtures still fail NewSkeleton or ValidateComposition today.
func TestNewSkeleton_GAP12_tripwire(t *testing.T) {
	names := make([]string, 0, len(gap12KnownFailures))
	for k := range gap12KnownFailures {
		names = append(names, k)
	}
	sort.Strings(names)

	var got []string
	for _, name := range names {
		opt := parseGAP12Fixture(t, name)
		c, err := templatecompile.Compile(opt)
		if err != nil {
			t.Errorf("%s: Compile: %v", name, err)
			got = append(got, name)
			continue
		}
		comp, err := composition.NewSkeleton(context.Background(), c,
			composition.WithTerritory("NL"),
			composition.WithComposer(testComposerGap12()),
		)
		if err != nil || !validation.ValidateComposition(comp, c).OK {
			got = append(got, name)
		}
	}

	want := make([]string, 0, len(gap12KnownFailures))
	for k := range gap12KnownFailures {
		want = append(want, k)
	}
	sort.Strings(want)

	if !equalStrings(got, want) {
		t.Fatalf("SDK-GAP-12 gap set drifted.\n got: %v\nwant: %v\n"+
			"If a fixture now succeeds, remove it from gap12KnownFailures and update the plan. "+
			"If a new fixture regressed, investigate before allow-listing it.", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

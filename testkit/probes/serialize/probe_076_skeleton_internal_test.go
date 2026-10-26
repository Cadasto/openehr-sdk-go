package serializeprobes

// The skeleton leg of PROBE-076 (REQ-053) judges a round trip only from a
// skeleton that validates: a skeleton the generator writes invalid fails the
// pair and is named, so a decode defect is never confused with a generator one.

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

// TestProbe076SkeletonLegFailsAnInvalidSkeleton pins that the leg refuses to
// judge a round trip from a written skeleton that does not validate: the BMI
// skeleton with its composer taken away fails, naming the template, the
// policy and the issue, before any format is tried.
func TestProbe076SkeletonLegFailsAnInvalidSkeleton(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOpt("BMI"))
	if err != nil {
		t.Fatalf("ParseFile(BMI): %v", err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(BMI): %v", err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatalf("Build(BMI): %v", err)
	}
	name := "Composer"
	comp, err := composition.NewSkeleton(t.Context(), compiled,
		composition.WithPolicy(instance.Medium),
		composition.WithComposer(&rm.PartyIdentified{Name: &name}),
		composition.WithTerritory("NL"),
		composition.WithNow(probe076SkeletonNow))
	if err != nil {
		t.Fatalf("NewSkeleton(BMI): %v", err)
	}
	if r := skeletonRoundTrip(compiled, wt, comp, instance.Medium, nil); r.Status != probe.StatusPass {
		t.Fatalf("the intact BMI skeleton: status = %q (detail: %s); want pass, or the case below proves nothing", r.Status, r.Detail)
	}
	comp.Composer = nil
	r := skeletonRoundTrip(compiled, wt, comp, instance.Medium, nil)
	if r.Status != probe.StatusFail {
		t.Fatalf("status = %q (detail: %s) for a skeleton with no composer; want fail", r.Status, r.Detail)
	}
	for _, w := range []string{"BMI medium: the written skeleton does not validate", "/composer"} {
		if !strings.Contains(r.Detail, w) {
			t.Errorf("detail = %q; want it to name %q", r.Detail, w)
		}
	}
	if strings.Contains(r.Detail, "FLAT") || strings.Contains(r.Detail, "STRUCTURED") {
		t.Errorf("detail = %q; an invalid skeleton must fail before any format is tried", r.Detail)
	}
}

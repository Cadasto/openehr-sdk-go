package templatecompile_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestCompile_ProducesUsableCompiled is the headline REQ-111 assertion:
// a parsed OPT (public template.ParseFile output) compiles to a usable
// *Compiled via the public bridge — no internal/ import required.
func TestCompile_ProducesUsableCompiled(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if c == nil {
		t.Fatal("Compile returned nil *Compiled")
	}
	if got, want := c.TemplateID(), opt.TemplateID(); got != want {
		t.Errorf("TemplateID = %q, want %q", got, want)
	}
	if c.Root() == nil {
		t.Fatal("Compiled.Root() is nil")
	}
	if got := c.Root().RMTypeName(); got != "COMPOSITION" {
		t.Errorf("root RM type = %q, want COMPOSITION", got)
	}
}

// TestCompile_NilOPT_ReturnsErrInvalidInput proves the sentinel is the
// same var as the engine's, so errors.Is works across the package
// boundary for external callers.
func TestCompile_NilOPT_ReturnsErrInvalidInput(t *testing.T) {
	_, err := templatecompile.Compile(nil)
	if !errors.Is(err, templatecompile.ErrInvalidInput) {
		t.Fatalf("Compile(nil) error = %v, want errors.Is ErrInvalidInput", err)
	}
}

// TestCompile_WithoutImplicitAttributes proves the functional option is
// wired to the engine: disabling implicit-attribute injection yields a
// root with strictly fewer attributes than the default compile (the OPT
// omits some BMM-required attributes that default injection restores).
func TestCompile_WithoutImplicitAttributes(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	withImplicit, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile (implicit): %v", err)
	}
	withoutImplicit, err := templatecompile.Compile(opt, templatecompile.WithoutImplicitAttributes())
	if err != nil {
		t.Fatalf("Compile (skip implicit): %v", err)
	}

	got := len(withoutImplicit.Root().Attributes())
	want := len(withImplicit.Root().Attributes())
	if got >= want {
		t.Errorf("root attributes without implicit = %d, want < %d (default)", got, want)
	}
}

// noRequiredLookup wraps the default RM info but advertises no required
// attributes. Because implicit-attribute injection is driven by
// Lookup.RequiredAttributes, compiling with this lookup must inject
// nothing — observably proving Compile routes WithRMInfo's lookup into
// the engine (rather than ignoring it and using the default).
type noRequiredLookup struct {
	rminfo.Lookup
}

func (noRequiredLookup) RequiredAttributes(string) []string { return nil }

func TestCompile_WithRMInfo_DrivesImplicitInjection(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	custom, err := templatecompile.Compile(opt, templatecompile.WithRMInfo(noRequiredLookup{rminfo.Default}))
	if err != nil {
		t.Fatalf("Compile (custom RM info): %v", err)
	}
	skipped, err := templatecompile.Compile(opt, templatecompile.WithoutImplicitAttributes())
	if err != nil {
		t.Fatalf("Compile (skip implicit): %v", err)
	}
	def, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile (default): %v", err)
	}

	// A lookup with no required attributes injects no implicits — so the
	// root attribute count must match the explicit skip-implicit compile,
	// and be strictly fewer than the default lookup's.
	if got, want := len(custom.Root().Attributes()), len(skipped.Root().Attributes()); got != want {
		t.Errorf("WithRMInfo(no-required) root attrs = %d, want %d (== WithoutImplicitAttributes)", got, want)
	}
	if len(custom.Root().Attributes()) >= len(def.Root().Attributes()) {
		t.Error("WithRMInfo(no-required) should inject fewer attributes than the default lookup")
	}
}

// moreRequiredLookup wraps the default RM info but also marks OBSERVATION's
// uid and links mandatory, which the pinned Reference Model leaves optional.
type moreRequiredLookup struct {
	rminfo.Lookup
}

func (l moreRequiredLookup) RequiredAttributes(rmType string) []string {
	required := l.Lookup.RequiredAttributes(rmType)
	if rmType == "OBSERVATION" {
		return append(slices.Clone(required), "uid", "links")
	}
	return required
}

// observationLinksOPT is a template rooted at an OBSERVATION that declares
// its links, optional, and nothing else.
const observationLinksOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>observation-links</value></template_id>
  <concept>observation-links</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>OBSERVATION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>links</rm_attribute_name>
      <existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></existence>
      <cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique>
        <interval><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
    </attributes>
    <archetype_id><value>openEHR-EHR-OBSERVATION.links.v1</value></archetype_id>
  </definition>
</template>`

// REQ-112 § Driver, REQ-111 — WithRMInfo shapes the compiled template, not
// the Reference Model rules the validators apply. A lookup that marks
// OBSERVATION's uid and links mandatory makes the compile inject uid and
// mark the declared links required; an OBSERVATION without a uid, whose
// links are present and empty or absent, still draws nothing at /uid and
// nothing at /links from Validate, neither a required nor a cardinality, as
// ValidateRM, which no lookup reaches, draws nothing there. The one finding
// at /links is the RM's own LOCATABLE Links_valid, on links that are present
// and empty, which ValidateRM and both compiles report alike.
func TestCompile_WithRMInfo_ShapesTheTemplateNotTheRMRules(t *testing.T) {
	opt, err := template.ParseOPT(strings.NewReader(observationLinksOPT))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	def, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile (default): %v", err)
	}
	custom, err := templatecompile.Compile(opt, templatecompile.WithRMInfo(moreRequiredLookup{rminfo.Default}))
	if err != nil {
		t.Fatalf("Compile (custom RM info): %v", err)
	}
	if def.Root().Attribute("uid") != nil || def.Root().Attribute("links").Required() {
		t.Fatal("precondition: the default compile injects uid or marks links required; the pinned RM leaves both optional")
	}
	if uid := custom.Root().Attribute("uid"); uid == nil || !uid.Implicit() || !custom.Root().Attribute("links").Required() {
		t.Fatal("precondition: WithRMInfo(uid and links mandatory) does not inject uid and mark links required; the lookup does not shape the template")
	}

	const id = "openEHR-EHR-OBSERVATION.links.v1"
	observation := func(links []rm.Link) *rm.Observation {
		return &rm.Observation{
			ArchetypeNodeID:  id,
			Name:             rm.DVText{Value: "links"},
			ArchetypeDetails: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: id}, RMVersion: "1.1.0"},
			Language:         rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
			Encoding:         rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}, CodeString: "UTF-8"},
			Subject:          &rm.PartySelf{},
			Links:            links,
		}
	}
	for _, tc := range []struct {
		name  string
		obs   *rm.Observation
		empty bool // links is present and empty, which the RM's Links_valid reports
	}{
		{name: "links empty", obs: observation([]rm.Link{}), empty: true},
		{name: "links absent", obs: observation(nil)},
	} {
		for _, path := range []string{"/uid", "/links"} {
			for call, r := range map[string]validation.Result{
				"ValidateRM":                         validation.ValidateRM(tc.obs),
				"Validate, the default compile":      validation.Validate(tc.obs, def),
				"Validate, compiled with WithRMInfo": validation.Validate(tc.obs, custom),
			} {
				linksValid := 0
				for _, iss := range r.Issues {
					switch {
					case iss.Path != path:
					case path == "/links" && tc.empty && iss.Code == "rm_invariant" && iss.Layer == validation.LayerRM && iss.Rule == "LOCATABLE.Links_valid":
						linksValid++
					default:
						t.Errorf("%s(OBSERVATION without a uid, %s) reports %s %s %s at %s, want nothing there: the RM leaves it optional", call, tc.name, iss.Code, iss.Layer, iss.Rule, path)
					}
				}
				want := 0
				if path == "/links" && tc.empty {
					want = 1
				}
				if linksValid != want {
					t.Errorf("%s(OBSERVATION without a uid, %s) reports LOCATABLE.Links_valid %d times at %s, want %d", call, tc.name, linksValid, path, want)
				}
			}
		}
	}
}

// TestNodeAt_UnknownPath_ReturnsErrPathNotFound pins the other
// re-exported sentinel: an external caller cannot import the engine, so
// matching templatecompile.ErrPathNotFound across the boundary is the
// only way to detect an unknown path from Compiled.NodeAt.
func TestNodeAt_UnknownPath_ReturnsErrPathNotFound(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, err := c.NodeAt("/no_such_attribute"); !errors.Is(err, templatecompile.ErrPathNotFound) {
		t.Fatalf("NodeAt(unknown) error = %v, want errors.Is ErrPathNotFound", err)
	}
}

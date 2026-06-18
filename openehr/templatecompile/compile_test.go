package templatecompile_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
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

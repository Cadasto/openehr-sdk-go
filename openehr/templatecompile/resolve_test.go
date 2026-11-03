package templatecompile_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestREQ111_PublicResolve drives the path lookup through the public names
// alone: Resolve and ValidatePath on a compiled template, WithStrictPaths,
// and an error for a path that reaches no node that this package's
// ErrPathNotFound and template.ErrPathNotFound both match (REQ-111 § Path
// lookup).
func TestREQ111_PublicResolve(t *testing.T) {
	c, err := templatecompile.Compile(mustReadModelFile(t, fixtures.TemplateOptForName("vital_signs")))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	parse := func(s string) template.Path {
		t.Helper()
		p, err := template.ParsePath(s)
		if err != nil {
			t.Fatalf("ParsePath(%q): %v", s, err)
		}
		return p
	}

	const bp = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]"
	n, err := c.Resolve(parse(bp + "/data/events[at0006]/data/items[at0004]/value"))
	if err != nil {
		t.Fatalf("Resolve(systolic) = %v", err)
	}
	if n.RMTypeName() != "DV_QUANTITY" || n.AQLPath() != systolicPath {
		t.Errorf("Resolve(systolic) = %s at %s, want DV_QUANTITY at %s", n.RMTypeName(), n.AQLPath(), systolicPath)
	}

	err = c.ValidatePath(parse(bp + "/no_such_attribute"))
	if !errors.Is(err, templatecompile.ErrPathNotFound) || !errors.Is(err, template.ErrPathNotFound) {
		t.Errorf("ValidatePath(no such attribute) = %v, want an error matching both not-found sentinels", err)
	}

	if err := c.ValidatePath(parse("/content"), templatecompile.WithStrictPaths()); !errors.Is(err, template.ErrAmbiguousPath) {
		t.Errorf("ValidatePath(/content, WithStrictPaths()) = %v, want an error wrapping template.ErrAmbiguousPath", err)
	}
	var opts []templatecompile.ResolveOption
	if err := c.ValidatePath(parse("/content"), opts...); err != nil {
		t.Errorf("ValidatePath(/content) = %v, want the first child", err)
	}
}

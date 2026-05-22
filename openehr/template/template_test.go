package template_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// REQ-100 — parse a real vendored OPT and assert wrapper identity.
func TestParseFile_VitalSigns_Identity(t *testing.T) {
	opt, err := template.ParseFile(filepath.Join("testdata", "vital_signs.opt"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if got, want := opt.TemplateID(), "vital_signs"; got != want {
		t.Errorf("TemplateID() = %q, want %q", got, want)
	}
	if got, want := opt.Concept(), "vital_signs"; got != want {
		t.Errorf("Concept() = %q, want %q", got, want)
	}
	if opt.UID() == "" {
		t.Error("UID() should be non-empty for vital_signs.opt")
	}
	if got, want := opt.Language(), "en"; got != want {
		t.Errorf("Language() = %q, want %q", got, want)
	}
	root := opt.Root()
	if root == nil {
		t.Fatal("Root() returned nil")
	}
	if got, want := root.RMTypeName(), "COMPOSITION"; got != want {
		t.Errorf("Root().RMTypeName() = %q, want %q", got, want)
	}
	// The vital_signs OPT root is an archetype root with an explicit
	// COMPOSITION archetype id.
	ar, ok := root.(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("Root() type = %T, want *template.ArchetypeRoot", root)
	}
	if !strings.HasPrefix(ar.ArchetypeID(), "openEHR-EHR-COMPOSITION.") {
		t.Errorf("Root() archetype id = %q, want openEHR-EHR-COMPOSITION.* prefix", ar.ArchetypeID())
	}
	if len(ar.Attributes()) == 0 {
		t.Error("Root() should have at least one child attribute")
	}
}

// REQ-100 — the second fixture exercises a different concept; basic
// identity check confirms parse succeeds on a structurally distinct OPT.
func TestParseFile_ClinicalNote_Identity(t *testing.T) {
	opt, err := template.ParseFile(filepath.Join("testdata", "clinical_note.opt"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if got := opt.TemplateID(); !strings.Contains(strings.ToLower(got), "clinical") {
		t.Errorf("TemplateID() = %q, expected to contain 'clinical'", got)
	}
	if opt.Root() == nil {
		t.Fatal("Root() returned nil")
	}
}

// REQ-100 — ParseFile MUST reject non-.opt paths with ErrNotOPTFile
// without opening the file.
func TestParseFile_RejectsNonOPTSuffix(t *testing.T) {
	_, err := template.ParseFile(filepath.Join("testdata", "README.md"))
	if !errors.Is(err, template.ErrNotOPTFile) {
		t.Fatalf("got %v, want ErrNotOPTFile", err)
	}
}

// REQ-100 — ParseOPT MUST reject obviously invalid input with
// ErrInvalidOPT.
func TestParseOPT_InvalidXML(t *testing.T) {
	_, err := template.ParseOPT(strings.NewReader("not xml at all"))
	if !errors.Is(err, template.ErrInvalidOPT) {
		t.Fatalf("got %v, want ErrInvalidOPT", err)
	}
}

func TestParseOPT_MissingTemplateID(t *testing.T) {
	xmlNoTID := `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <concept>x</concept>
  <definition><rm_type_name>COMPOSITION</rm_type_name></definition>
</template>`
	_, err := template.ParseOPT(strings.NewReader(xmlNoTID))
	if !errors.Is(err, template.ErrInvalidOPT) {
		t.Fatalf("got %v, want ErrInvalidOPT for missing template_id", err)
	}
}

func TestParseOPT_MissingDefinition(t *testing.T) {
	xmlNoDef := `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>x</value></template_id>
  <concept>x</concept>
</template>`
	_, err := template.ParseOPT(strings.NewReader(xmlNoDef))
	if !errors.Is(err, template.ErrInvalidOPT) {
		t.Fatalf("got %v, want ErrInvalidOPT for missing definition", err)
	}
}

// REQ-100 — ParseOPT MUST accept a UTF-8 BOM prefix (some authoring
// tools emit BOM-prefixed UTF-8).
func TestParseOPT_AcceptsBOM(t *testing.T) {
	bytes, err := os.ReadFile(filepath.Join("testdata", "vital_signs.opt"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	// vital_signs.opt already ships with a BOM — proof BOM handling
	// works end to end is the existing identity test above. As a
	// belt-and-braces unit guard, also accept BOM on a synthetic
	// minimal OPT.
	const bom = "\xEF\xBB\xBF"
	mini := bom + `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>mini</value></template_id>
  <concept>mini</concept>
  <definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id></definition>
</template>`
	if _, err := template.ParseOPT(strings.NewReader(mini)); err != nil {
		t.Fatalf("ParseOPT with BOM: %v", err)
	}
	_ = bytes // capture for diagnostics on failure
}

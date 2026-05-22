package template_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io/fs"
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

// ParseFile MUST wrap the underlying os.Open error so callers can
// classify with errors.Is(err, fs.ErrNotExist). Self-review finding
// #2 from PR #10 multi-agent review.
func TestParseFile_MissingFileWrapsFSError(t *testing.T) {
	_, err := template.ParseFile(filepath.Join("testdata", "does_not_exist.opt"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("got %v, want errors.Is(err, fs.ErrNotExist) to be true", err)
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
	// vital_signs.opt already ships with a BOM. Dual-prove BOM handling:
	// (a) the on-disk fixture parses, and (b) a synthetic minimal OPT
	// with an injected BOM also parses.
	fixturePath := filepath.Join("testdata", "vital_signs.opt")
	body, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if !bytes.HasPrefix(body, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("fixture %s expected to ship with UTF-8 BOM (BOM-handling regression)", fixturePath)
	}
	if _, err := template.ParseOPT(bytes.NewReader(body)); err != nil {
		t.Fatalf("ParseOPT(vital_signs.opt with BOM): %v", err)
	}

	const bom = "\xEF\xBB\xBF"
	mini := bom + `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>mini</value></template_id>
  <concept>mini</concept>
  <definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id></definition>
</template>`
	if _, err := template.ParseOPT(strings.NewReader(mini)); err != nil {
		t.Fatalf("ParseOPT(synthetic mini with BOM): %v", err)
	}
}

// REQ-100 § Error taxonomy — ParseOPT MUST surface an <attributes>
// element with an unknown xsi:type as ErrUnsupportedNode (wrapped via
// ErrInvalidOPT). Unknown <children> xsi:type values are admitted as
// forward-compatible leaves (parse.go default branch); only the
// attribute taxonomy is closed in v1.
func TestParseOPT_UnsupportedAttributeType(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>t</value></template_id>
  <concept>t</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_TUPLE_ATTRIBUTE"
      xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <rm_attribute_name>category</rm_attribute_name>
    </attributes>
  </definition>
</template>`
	_, err := template.ParseOPT(strings.NewReader(body))
	if !errors.Is(err, template.ErrUnsupportedNode) {
		t.Fatalf("got %v, want errors.Is(err, ErrUnsupportedNode) — chain must reach the inner sentinel through ErrInvalidOPT wrap", err)
	}
}

// REQ-100 § Error taxonomy — malformed XML MUST be surfaced via
// ErrInvalidOPT AND the inner *xml.SyntaxError must be reachable via
// errors.As, so callers can render decoder-style positional context.
// Validates the double-%w wrap in parse.go.
func TestParseOPT_InvalidXML_UnwrapsXMLError(t *testing.T) {
	// Unterminated element — guaranteed to trigger encoding/xml's
	// SyntaxError type from the decoder.
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>oops</template_id>
</template>`
	_, err := template.ParseOPT(strings.NewReader(body))
	if !errors.Is(err, template.ErrInvalidOPT) {
		t.Fatalf("got %v, want ErrInvalidOPT", err)
	}
	var se *xml.SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("got %v, want chain to expose *xml.SyntaxError via errors.As", err)
	}
}

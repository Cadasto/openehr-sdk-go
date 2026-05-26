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
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-100 — parse a real vendored OPT and assert wrapper identity.
func TestParseFile_VitalSigns_Identity(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
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
	opt, err := template.ParseFile(fixtures.TemplateOptForName("clinical_note"))
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
	_, err := template.ParseFile(filepath.Join(fixtures.CassettesRoot(), "README.md"))
	if !errors.Is(err, template.ErrNotOPTFile) {
		t.Fatalf("got %v, want ErrNotOPTFile", err)
	}
}

// ParseFile MUST wrap the underlying os.Open error so callers can
// classify with errors.Is(err, fs.ErrNotExist). Self-review finding
// #2 from PR #10 multi-agent review.
func TestParseFile_MissingFileWrapsFSError(t *testing.T) {
	_, err := template.ParseFile(fixtures.TemplateOptForName("does_not_exist"))
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
	fixturePath := fixtures.TemplateOptForName("vital_signs")
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

// REQ-100 — ParseOPT(nil) must short-circuit with ErrInvalidOPT
// rather than panicking on a nil reader.
func TestParseOPT_NilReader(t *testing.T) {
	_, err := template.ParseOPT(nil)
	if !errors.Is(err, template.ErrInvalidOPT) {
		t.Fatalf("got %v, want ErrInvalidOPT for nil reader", err)
	}
}

// REQ-100 — A document whose root element is not <template> must be
// rejected, even if the inner XML parses. Forward-compat guard
// against future XSD wrappers that might decode permissively.
func TestParseOPT_NonTemplateRoot(t *testing.T) {
	const body = `<?xml version="1.0"?>
<archetype xmlns="http://schemas.openehr.org/v1">
  <template_id><value>x</value></template_id>
  <concept>x</concept>
  <definition><rm_type_name>COMPOSITION</rm_type_name></definition>
</archetype>`
	_, err := template.ParseOPT(strings.NewReader(body))
	if !errors.Is(err, template.ErrInvalidOPT) {
		t.Fatalf("got %v, want ErrInvalidOPT for non-<template> root", err)
	}
}

// REQ-100 — ParseFile's .opt suffix check is case-insensitive, so
// authoring tools that emit upper-case extensions are accepted.
func TestParseFile_CaseInsensitiveExtension(t *testing.T) {
	src := fixtures.TemplateOptForName("vital_signs")
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "VITAL_SIGNS.OPT")
	if err := os.WriteFile(dst, body, 0o600); err != nil {
		t.Fatalf("write copy: %v", err)
	}
	if _, err := template.ParseFile(dst); err != nil {
		t.Fatalf("ParseFile(%q): %v", dst, err)
	}
}

// Phase 2: ParseOPT MUST reject trailing content after </template>;
// a well-formed OPT carries exactly one root element. Prevents
// "looks valid but truncated" inputs from passing silently.
func TestParseOPT_RejectsTrailingContent(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>t</value></template_id>
  <concept>t</concept>
  <definition><rm_type_name>COMPOSITION</rm_type_name></definition>
</template>
<template xmlns="http://schemas.openehr.org/v1"/>`
	_, err := template.ParseOPT(strings.NewReader(body))
	if !errors.Is(err, template.ErrInvalidOPT) {
		t.Fatalf("got %v, want ErrInvalidOPT for trailing root element", err)
	}
}

// Phase 2: ParseOPT MUST tolerate trailing whitespace and comments
// after </template> — those are valid XML trivia.
func TestParseOPT_ToleratesTrailingTrivia(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>t</value></template_id>
  <concept>t</concept>
  <definition><rm_type_name>COMPOSITION</rm_type_name></definition>
</template>
<!-- trailing comment is allowed -->
  ` + "\n\t  "
	if _, err := template.ParseOPT(strings.NewReader(body)); err != nil {
		t.Fatalf("trailing whitespace/comment must be tolerated: %v", err)
	}
}

// Phase 2: lenient ParseOPT admits unknown <children> xsi:type as a
// leaf even when nested <attributes> are present (forward-compat
// escape hatch). ParseOPTStrict rejects the same input with
// ErrUnsupportedNode so production validators can fail loudly on
// shapes outside the v1 taxonomy.
func TestParseOPTStrict_RejectsUnknownChildWithAttributes(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>t</value></template_id>
  <concept>t</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE"
      xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <rm_attribute_name>category</rm_attribute_name>
      <children xsi:type="C_TUPLE_OBJECT">
        <rm_type_name>TUPLE</rm_type_name>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>magnitude</rm_attribute_name>
        </attributes>
      </children>
    </attributes>
  </definition>
</template>`
	// Lenient: admits the leaf, drops the inner attributes.
	if _, err := template.ParseOPT(strings.NewReader(body)); err != nil {
		t.Fatalf("lenient ParseOPT rejected admissible forward-compat shape: %v", err)
	}
	// Strict: surfaces ErrUnsupportedNode.
	_, err := template.ParseOPTStrict(strings.NewReader(body))
	if !errors.Is(err, template.ErrUnsupportedNode) {
		t.Fatalf("ParseOPTStrict got %v, want errors.Is(err, ErrUnsupportedNode)", err)
	}
}

// Phase 2: an unknown xsi:type without nested attributes is still a
// leaf even in strict mode — strict mode only fires when the unknown
// type would otherwise silently flatten a non-trivial subtree.
func TestParseOPTStrict_AdmitsBareUnknownChild(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>t</value></template_id>
  <concept>t</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE"
      xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <rm_attribute_name>category</rm_attribute_name>
      <children xsi:type="C_DV_CODED_TEXT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
      </children>
    </attributes>
  </definition>
</template>`
	if _, err := template.ParseOPTStrict(strings.NewReader(body)); err != nil {
		t.Fatalf("ParseOPTStrict rejected bare unknown child (should admit as leaf): %v", err)
	}
}

// Phase 2: Description() exposes the parsed top-level <description>
// block. Both vendored fixtures carry one; assert at least the
// lifecycle_state round-trips.
func TestOperationalTemplate_DescriptionCaptured(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("clinical_note"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	d := opt.Description()
	if d == nil {
		t.Fatal("Description() = nil, want non-nil for clinical_note.opt")
	}
	if d.LifecycleState() == "" {
		t.Error("LifecycleState() empty; want a value from <lifecycle_state>")
	}
	authors := d.OriginalAuthors()
	if authors["name"] == "" {
		t.Errorf("OriginalAuthors()[\"name\"] empty; got %v", authors)
	}
	if d.OtherDetails()["sem_ver"] != "0.1.0" {
		t.Errorf("OtherDetails()[\"sem_ver\"] = %q, want %q", d.OtherDetails()["sem_ver"], "0.1.0")
	}
}

// Phase 2: Annotations() captures <annotations path="..."> blocks.
// Neither vendored fixture carries any, so use a synthetic body to
// exercise the path.
func TestOperationalTemplate_AnnotationsCaptured(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>t</value></template_id>
  <concept>t</concept>
  <definition><rm_type_name>COMPOSITION</rm_type_name></definition>
  <annotations path="/content[at0001]">
    <items id="comment">first child requires manual review</items>
    <items id="ui-hint">collapsible</items>
  </annotations>
  <annotations path="/category">
    <items id="comment">defaults to event</items>
  </annotations>
</template>`
	opt, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	got := opt.Annotations()
	if len(got["/content[at0001]"]) != 2 {
		t.Errorf("len(annotations[/content[at0001]]) = %d, want 2", len(got["/content[at0001]"]))
	}
	if len(got["/category"]) != 1 {
		t.Errorf("len(annotations[/category]) = %d, want 1", len(got["/category"]))
	}
	if got["/category"][0].ID != "comment" {
		t.Errorf("first /category annotation ID = %q, want %q", got["/category"][0].ID, "comment")
	}
}

// Phase 2: getter immutability — overwriting one element of the
// returned slice MUST NOT affect the next caller. Defends against
// accidental aliases in walker / validator code.
func TestComplexObject_AttributesIsImmutable(t *testing.T) {
	opt := mustParseVitalSigns(t)
	root, ok := opt.Root().(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("root = %T, want *template.ArchetypeRoot", opt.Root())
	}
	first := root.Attributes()
	if len(first) == 0 {
		t.Skip("fixture has no root attributes")
	}
	originalName := first[0].Name()
	first[0] = nil // mutate the caller's copy

	second := root.Attributes()
	if second[0] == nil || second[0].Name() != originalName {
		t.Errorf("Attributes() shared backing array: second call element nil-after-mutation or name=%v != %q", second[0], originalName)
	}
}

// Phase 2 follow-up: map getter immutability — mutating the returned
// Annotations / OriginalAuthors / OtherDetails map MUST NOT affect the
// next caller, mirroring the slice-getter contract (godoc claims the
// returned map is a defensive copy).
func TestOperationalTemplate_MapGettersAreCloned(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>t</value></template_id>
  <concept>t</concept>
  <description>
    <original_author id="name">Alice</original_author>
    <original_author id="organisation">Acme</original_author>
    <lifecycle_state>initial</lifecycle_state>
    <other_details id="licence">CC-BY-SA</other_details>
    <other_details id="sem_ver">1.2.3</other_details>
  </description>
  <definition><rm_type_name>COMPOSITION</rm_type_name></definition>
  <annotations path="/content[at0001]">
    <items id="comment">manual review</items>
  </annotations>
</template>`
	opt, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}

	t.Run("Annotations", func(t *testing.T) {
		first := opt.Annotations()
		delete(first, "/content[at0001]")
		first["/poisoned"] = nil
		second := opt.Annotations()
		if _, ok := second["/content[at0001]"]; !ok {
			t.Errorf("Annotations() shared map: /content[at0001] removed by caller delete")
		}
		if _, ok := second["/poisoned"]; ok {
			t.Errorf("Annotations() shared map: /poisoned leaked from caller insert")
		}
	})

	d := opt.Description()
	if d == nil {
		t.Fatal("Description() = nil")
	}

	t.Run("OriginalAuthors", func(t *testing.T) {
		first := d.OriginalAuthors()
		first["name"] = "MUTATED"
		first["poisoned"] = "x"
		second := d.OriginalAuthors()
		if second["name"] != "Alice" {
			t.Errorf("OriginalAuthors() shared map: name=%q, want %q", second["name"], "Alice")
		}
		if _, ok := second["poisoned"]; ok {
			t.Errorf("OriginalAuthors() shared map: poisoned key leaked")
		}
	})

	t.Run("OtherDetails", func(t *testing.T) {
		first := d.OtherDetails()
		first["sem_ver"] = "MUTATED"
		delete(first, "licence")
		second := d.OtherDetails()
		if second["sem_ver"] != "1.2.3" {
			t.Errorf("OtherDetails() shared map: sem_ver=%q, want %q", second["sem_ver"], "1.2.3")
		}
		if second["licence"] != "CC-BY-SA" {
			t.Errorf("OtherDetails() shared map: licence removed by caller, got %q", second["licence"])
		}
	})
}

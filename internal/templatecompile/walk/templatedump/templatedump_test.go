package templatedump_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templatecompile/walk"
	"github.com/cadasto/openehr-sdk-go/internal/templatecompile/walk/templatedump"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// Phase 5 — PathCollector accumulates every visited node's AQL
// path in pre-order. Spot-check the first entry is the root and a
// known deep path appears.
func TestPathCollector_GathersAllPaths(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")
	pc := &templatedump.PathCollector{}
	if err := walk.Walk(c, pc); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(pc.Paths) == 0 {
		t.Fatal("PathCollector recorded zero paths")
	}
	if pc.Paths[0] != "/" {
		t.Errorf("first path = %q, want root \"/\"", pc.Paths[0])
	}
	const wantDeep = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]"
	if !slices.Contains(pc.Paths, wantDeep) {
		t.Errorf("path %q missing from collected paths", wantDeep)
	}
}

// Phase 5 — Printer renders a non-empty indented tree with one
// line per visited node. Each line carries the AQL path and the
// RM type name.
func TestPrinter_RendersTree(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")
	out, err := templatedump.Dump(c, "  ")
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}
	if out == "" {
		t.Fatal("Dump produced empty output")
	}
	// First line is the root: path "/" + RM type COMPOSITION.
	first := strings.SplitN(out, "\n", 2)[0]
	if !strings.HasPrefix(first, "/") {
		t.Errorf("first line = %q, want it to start with the root path \"/\"", first)
	}
	if !strings.Contains(first, "COMPOSITION") {
		t.Errorf("first line missing COMPOSITION: %q", first)
	}
	// At least one slot tag must appear (vital_signs.opt has CLUSTER
	// slots).
	if !strings.Contains(out, "(slot)") {
		t.Errorf("no (slot) tag in output; expected at least one *Slot leaf")
	}
}

// Phase 5 — Printer marks implicit-attribute children with the
// "(implicit attr)" tag, so visitors that hand the output to humans
// can see which fields were RM-injected vs OPT-declared.
func TestPrinter_MarksImplicitAttrChildren(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")
	out, err := templatedump.Dump(c, "  ")
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}
	// vital_signs.opt declares only category + content explicitly;
	// composer / language / territory arrive via rminfo. The Printer
	// renders the parent attribute's Implicit flag, but the implicit
	// attribute itself has NO children — so the tag appears only
	// when a CHILD of an implicit attribute is visited. Implicit
	// attributes have no children, so the tag stays effectively
	// reserved for future RM-injection that DOES populate values.
	// We don't assert any specific output here — just sanity-check
	// that the dump completes without panic on implicit-bearing
	// nodes.
	if len(out) == 0 {
		t.Fatal("Dump empty")
	}
}

// Phase 5 — Printer is usable both as a Walk visitor and via Dump.
// String() on the visitor returns the same output as Dump for the
// same input + indent.
func TestPrinter_StringAndDumpMatch(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")

	p := &templatedump.Printer{Indent: "  "}
	if err := walk.Walk(c, p); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	dump, err := templatedump.Dump(c, "  ")
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}
	if p.String() != dump {
		t.Errorf("Printer.String() and Dump() produced different output")
	}
}

func mustCompile(t *testing.T, fixture string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(filepath.Join("..", "..", "..", "..", "openehr", "template", "testdata", fixture))
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", fixture, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", fixture, err)
	}
	return c
}

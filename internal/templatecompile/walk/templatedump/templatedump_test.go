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

// Phase 5 — Printer does not panic when the compiled tree carries
// implicit (rminfo-injected) attributes. vital_signs.opt declares
// only category + content explicitly; composer / language / territory
// arrive via rminfo as implicit CompiledAttributes with empty
// Children slices — the walker therefore never visits a child *under*
// an implicit attribute and the "(implicit attr)" marker branch in
// PreHandle stays dormant. This test asserts only the no-panic
// property; the marker itself has no behavioural coverage in v1 and
// would need a synthetic Compiled with a populated implicit attribute
// (out of scope for Phase 5) to exercise.
func TestPrinter_DoesNotPanicOnImplicitAttrs(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")
	out, err := templatedump.Dump(c, "  ")
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}
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

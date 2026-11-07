package internal_test

// godoc_ids_guard_test.go: what SDK users read carries no spec-process
// identifiers. AGENTS.md § Spec-driven workflow sets the convention: "What
// pkg.go.dev shows (package docs, doc comments on exported identifiers, the
// values of exported variables and constants, examples, README files)
// carries no spec-process identifiers (REQ, ADR, STRAND, plan or spec paths)
// and no RFC-2119 capitals; a probe's own `PROBE-NNN` and public openEHR
// specification citations are fine. User-facing error text,
// `docs/quick-start.md`, `docs/examples.md` and the site under `pages/`
// carry no REQ, ADR, STRAND or PROBE id either."
//
// A comment does not show whether go doc prints it, so this reads what go
// doc and pkg.go.dev show: the package comment; the doc comment of an
// exported function, of an exported method of an exported type, and of an
// exported method an exported struct type takes from an unexported type it
// embeds, directly or through other unexported types; for a declaration
// group that declares an exported type, constant or variable, the group's
// doc comment and each exported spec's doc comment, and a value spec's
// trailing comment and the string literals and comments in its value (go
// doc drops those comments; pkg.go.dev may print them); the
// doc and trailing comment of each exported or embedded exported field of an
// exported struct type; the doc comment of each exported method or embedded
// exported interface of an exported interface type; and, from the test
// files, each example's doc comment and the comments and string literals in
// its body, or the whole file for a whole-file example. Directive lines
// (//go:generate, //nolint:...) are left out, as go doc leaves them out.
// Function bodies, unexported identifiers and the rest of the test files are
// out of scope.
//
// In that text it reports a requirement, strand or ADR id; the paths
// docs/specifications, docs/adr and docs/plans; the file name of a spec or
// ADR document, read from docs/ when the test runs, so a new document is
// covered without editing this file; an RFC-2119 keyword in capitals; the
// SDD acronym; and a probe id outside godocProbeDirs. Other .md names
// (README.md, examples.md, ...) and SDK-AQL divergence ids are fine. The
// README.md pkg.go.dev shows for the module and for each package directory
// is read with the same rules, and the documents userDocPatterns names with
// the id rules alone.
//
// The walk covers the files libraryGoFiles (reflect_guard_test.go) returns,
// generated ones and internal/ included, and the files under cmd/: the
// examples' package docs are published too. It must see at least
// minGodocScanFiles files, minExampleFuncs examples, the module README,
// minUserDocs user documents and minSpecDocNames spec and ADR file names: a
// green run over an empty or mis-rooted tree proves nothing.
//
// The fixtures in TestGodocGuardReadsWhatGoDocShows build each identifier
// with spid at run time, so this file cites no requirement, probe, strand or
// ADR for the spec gate to read, and stays out of the traceability map.

import (
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// minGodocScanFiles is well below the module's count of non-test Go files,
// cmd/ included, which is over 600.
const minGodocScanFiles = 500

// minSpecDocNames is well below the count of spec and ADR documents, which
// is over 40.
const minSpecDocNames = 30

// minExampleFuncs is well below the module's count of example functions,
// which is 18.
const minExampleFuncs = 10

// minUserDocs is below the count of documents userDocPatterns names, which
// is 9.
const minUserDocs = 7

// godocProbeDirs hold the probe implementations and their runners, whose
// godoc names their own probes.
var godocProbeDirs = []string{"testkit/probes/", "testkit/conformance/"}

// userDocPatterns name the documents written for SDK users beside go doc,
// relative to the module root: the quick start, the examples catalogue and
// the documentation site's pages. userDocSkip names the site's page for
// contributors, which describes the process.
var (
	userDocPatterns = []string{"docs/quick-start.md", "docs/examples.md", "pages/*.md"}
	userDocSkip     = []string{"pages/contributing.md"}
)

func TestGodocCarriesNoSpecProcessIdentifiers(t *testing.T) {
	t.Parallel()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(self)) // module root

	names, err := specDocNames(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < minSpecDocNames {
		t.Fatalf("found %d spec and ADR file name(s) under docs/; the module is known to have at least %d, so the guard has gone blind", len(names), minSpecDocNames)
	}
	hits, scanned, err := godocHits(root, names)
	if err != nil {
		t.Fatal(err)
	}
	exHits, examples, err := exampleHits(root, names)
	if err != nil {
		t.Fatal(err)
	}
	pageHitList, readmes, userDocs, err := pageHits(root, names)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range slices.Concat(hits, exHits, pageHitList) {
		t.Errorf("%s: this is written for SDK users; state the behaviour in plain English and keep the citation in a maintainer comment or a test", h)
	}
	if scanned < minGodocScanFiles {
		t.Fatalf("walk scanned %d non-test Go file(s); the module is known to have at least %d, so the guard has gone blind", scanned, minGodocScanFiles)
	}
	if examples < minExampleFuncs {
		t.Fatalf("walk found %d example function(s); the module is known to have at least %d, so the guard has gone blind", examples, minExampleFuncs)
	}
	if readmes < 1 {
		t.Fatal("walk read no README.md; the module root has one, so the guard has gone blind")
	}
	if userDocs < minUserDocs {
		t.Fatalf("read %d user document(s); the module is known to have at least %d, so the guard has gone blind", userDocs, minUserDocs)
	}
	t.Logf("scanned the godoc of %d non-test Go files, %d examples, %d README files and %d user documents against %d spec and ADR file names", scanned, examples, readmes, userDocs, len(names))
}

// TestGodocGuardReadsWhatGoDocShows runs the guard over a small module. Each
// forbidden form, in each position go doc shows, is reported once, with its
// line and the identifier it documents; nothing is reported from a position
// go doc does not show, or from an allowed citation.
func TestGodocGuardReadsWhatGoDocShows(t *testing.T) {
	t.Parallel()
	forms := []struct {
		name string
		text string
	}{
		{"requirement id", spid("REQ", "-", "052")},
		{"strand id", spid("STRAND", "-", "10")},
		{"ADR id with a space", spid("ADR", " ", "0029")},
		{"ADR id with a hyphen", spid("ADR", "-", "0029")},
		{"ADR id run together", spid("ADR", "", "0029")},
		{"specifications path", "docs/specifications"},
		{"ADR path", "docs/adr"},
		{"plans path", "docs/plans"},
		{"spec file name", "widget.md"},
		{"ADR file name", "0042-widget-choice.md"},
		{"MUST", "MUST"},
		{"MUST NOT", "MUST NOT"},
		{"SHOULD", "SHOULD"},
		{"SHOULD NOT", "SHOULD NOT"},
		{"SHALL", "SHALL"},
		{"SHALL NOT", "SHALL NOT"},
		{"REQUIRED", "REQUIRED"},
		{"RECOMMENDED", "RECOMMENDED"},
		{"MAY", "MAY"},
		{"OPTIONAL", "OPTIONAL"},
		{"SDD acronym", "SDD"},
		{"probe id outside a probe file", spid("PROBE", "-", "001")},
	}
	// Each position's source holds one %s, where the sentence goes; ident
	// is the identifier the report names.
	positions := []struct {
		name  string
		src   string
		ident string
	}{
		{"package comment", "// Package lib %s.\npackage lib\n", "package lib"},
		{"exported function", "package lib\n\n// F %s.\nfunc F() {}\n", "F"},
		{"method on a pointer receiver", "package lib\n\ntype T struct{}\n\n// M %s.\nfunc (*T) M() {}\n", "T.M"},
		{"method on a generic receiver", "package lib\n\ntype G[P any] struct{}\n\n// M %s.\nfunc (g G[P]) M() {}\n", "G.M"},
		{"method on a pointer to a two-parameter generic receiver", "package lib\n\ntype H[P, Q any] struct{}\n\n// M %s.\nfunc (h *H[P, Q]) M() {}\n", "H.M"},
		{"type declaration", "package lib\n\n// T %s.\ntype T int\n", "T"},
		{"type spec in a group", "package lib\n\ntype (\n\t// T %s.\n\tT int\n)\n", "T"},
		{"constant group", "package lib\n\n// Sizes %s.\nconst (\n\tA = 1\n\tb = 2\n)\n", "A"},
		{"variable spec in a group", "package lib\n\nvar (\n\t// V %s.\n\tV = 1\n)\n", "V"},
		{"trailing comment of a constant", "package lib\n\nconst C = 1 // C %s.\n", "C"},
		{"value of an exported variable", "package lib\n\nvar V = \"%s\"\n", "V"},
		{"value of an exported constant in a group", "package lib\n\nconst (\n\tC = \"%s\"\n)\n", "C"},
		{"comment in an exported variable's value", "package lib\n\nvar V = []int{\n\t// %s.\n\t1,\n}\n", "V"},
		{"struct field", "package lib\n\ntype S struct {\n\t// F %s.\n\tF int\n}\n", "S.F"},
		{"trailing comment of a struct field", "package lib\n\ntype S struct {\n\tF int // F %s.\n}\n", "S.F"},
		{"embedded struct field", "package lib\n\ntype E struct{}\n\ntype S struct {\n\t// E %s.\n\t*E\n}\n", "S.E"},
		{"trailing comment of an embedded struct field", "package lib\n\ntype S struct {\n\tfmt.Stringer // Stringer %s.\n}\n", "S.Stringer"},
		{"interface method", "package lib\n\ntype I interface {\n\t// M %s.\n\tM()\n}\n", "I.M"},
		{"embedded interface", "package lib\n\ntype J interface{}\n\ntype I interface {\n\t// J %s.\n\tJ\n}\n", "I.J"},
	}

	// every carries all the forms at once, for the positions go doc does not
	// show.
	var every []string
	for _, f := range forms {
		every = append(every, f.text)
	}
	unseen := "cites " + strings.Join(every, ", ") + " here"
	notShown := []struct {
		name string
		path string
		src  string
	}{
		{"unexported function", "lib/hidden_func.go", "package lib\n\n// f %s.\nfunc f() {}\n"},
		{"exported method of an unexported type", "lib/hidden_recv.go", "package lib\n\ntype t struct{}\n\n// M %s.\nfunc (t) M() {}\n"},
		{"unexported method of an exported type", "lib/hidden_method.go", "package lib\n\ntype T struct{}\n\n// m %s.\nfunc (*T) m() {}\n"},
		{"exported field of an unexported type", "lib/hidden_type_field.go", "package lib\n\ntype s struct {\n\t// F %s.\n\tF int\n}\n"},
		{"unexported field of an exported type", "lib/hidden_field.go", "package lib\n\ntype S struct {\n\t// f %s.\n\tf int\n}\n"},
		{"unexported method of an exported interface", "lib/hidden_iface_method.go", "package lib\n\ntype I interface {\n\t// m %s.\n\tm()\n}\n"},
		{"unexported spec in an exported group", "lib/hidden_spec.go", "package lib\n\nconst (\n\tA = 1\n\t// b %s.\n\tb = 2 // b %s.\n)\n"},
		{"group of unexported variables", "lib/hidden_group.go", "package lib\n\n// vars %s.\nvar (\n\ta = 1\n)\n"},
		{"value of an unexported variable", "lib/hidden_value.go", "package lib\n\nvar v = []string{\n\t// %s.\n\t\"%s\",\n}\n"},
		{"import group", "lib/hidden_import.go", "package lib\n\n// imports %s.\nimport \"fmt\"\n"},
		{"function body", "lib/hidden_body.go", "package lib\n\n// F does nothing.\nfunc F() {\n\t// body %s.\n\t_ = \"%s\"\n}\n"},
		{"comment attached to nothing", "lib/hidden_floating.go", "package lib\n\n// floating %s.\n\n// F does nothing.\nfunc F() {}\n"},
		{"directive line", "lib/hidden_directive.go", "package lib\n\n//go:generate echo %s\n// F does nothing.\nfunc F() {}\n"},
		{"test file with no example", "lib/lib_test.go", "// Package lib %s.\npackage lib\n\n// F %s.\nfunc F() {}\n"},
		{
			"methods of an unexported type no exported struct embeds", "promo2/hidden.go",
			"package promo2\n\ntype inner struct{}\n\n// M %s.\nfunc (*inner) M() {}\n\ntype outer struct {\n\t*inner\n}\n\n" +
				"// Outer does nothing.\ntype Outer struct {\n\tin *inner\n}\n",
		},
		{"probe id in a probe implementation", "testkit/probes/p/p.go", "// Package p runs " + spid("PROBE", "-", "001") + ".\npackage p\n"},
		{"probe id in a probe runner", "testkit/conformance/c/c.go", "// Package c runs " + spid("PROBE", "-", "001") + ".\npackage c\n"},
		{"README outside a package directory", "notes/README.md", "# Notes\n\n%s.\n"},
		{"site page for contributors", "pages/contributing.md", "# Contributing\n\n%s.\n"},
		{
			"allowed citations", "lib/allowed.go",
			"package lib\n\n// F follows README.md, examples.md, quick-start.md, architecture.md, docs/architecture.md,\n" +
				"// DIVERGENCES.md, CENSUS.md, SKIPPED.md and deviations.md, divergence SDK-AQL-001, the\n" +
				"// openEHR EHR Information Model specification, rewidget.md, my-widget.md and widget.mdx; a caller must\n" +
				"// and may, and the field is optional.\nfunc F() {}\n",
		},
	}

	// shown are the cases go doc, pkg.go.dev or a user document shows
	// besides the positions above, with the hits they produce.
	type wantHit struct {
		ident string
		match string
		line  int
	}
	shown := []struct {
		name string
		path string
		src  string
		want []wantHit
	}{
		{
			"package comment of a command under cmd", "cmd/tool/main.go",
			"// Command tool follows " + spid("REQ", "-", "052") + ".\npackage main\n\nfunc main() {}\n",
			[]wantHit{{"package main", spid("REQ", "-", "052"), 1}},
		},
		{
			"other forms in a probe implementation", "testkit/probes/q/q.go",
			"// Package q runs " + spid("PROBE", "-", "002") + " under " + spid("REQ", "-", "052") + " and widget.md.\npackage q\n",
			[]wantHit{{"package q", spid("REQ", "-", "052"), 1}, {"package q", "widget.md", 1}},
		},
		{
			"ADR id wrapped onto the next line", "lib/wrapped.go",
			"package lib\n\n// F follows ADR\n// 0029 and MUST\n// NOT panic.\nfunc F() {}\n",
			[]wantHit{{"F", spid("ADR", " ", "0029"), 3}, {"F", "MUST NOT", 4}},
		},
		{
			"block comment", "lib/block.go",
			"package lib\n\n/*\nF follows " + spid("STRAND", "-", "10") + ".\n*/\nfunc F() {}\n",
			[]wantHit{{"F", spid("STRAND", "-", "10"), 4}},
		},
		{
			"spec file name in a path", "lib/path.go",
			"package lib\n\n// F follows ../docs/specifications/widget.md#anchor.\nfunc F() {}\n",
			[]wantHit{{"F", "docs/specifications", 3}, {"F", "widget.md", 3}},
		},
		{
			"methods an exported struct takes from unexported types it embeds", "promo/promo.go",
			"package promo\n\ntype inner struct {\n\tdeeper\n}\n\n// M cites " + spid("REQ", "-", "052") + ".\nfunc (*inner) M() {}\n\n" +
				"type deeper struct{}\n\n// N cites " + spid("ADR", " ", "0029") + ".\nfunc (deeper) N() {}\n\n" +
				"// Outer does nothing.\ntype Outer struct {\n\t*inner\n}\n",
			[]wantHit{{"inner.M", spid("REQ", "-", "052"), 7}, {"deeper.N", spid("ADR", " ", "0029"), 12}},
		},
		{
			"example doc comment and body, not the test beside it", "lib/example_test.go",
			"package lib_test\n\nimport \"testing\"\n\n// ExampleF cites " + spid("REQ", "-", "052") + ".\nfunc ExampleF() {\n" +
				"\t// body cites " + spid("ADR", " ", "0029") + ".\n\tprintln(\"" + spid("STRAND", "-", "10") + "\")\n}\n\n" +
				"// TestX cites " + spid("REQ", "-", "052") + ".\nfunc TestX(t *testing.T) {}\n",
			[]wantHit{{"ExampleF", spid("REQ", "-", "052"), 5}, {"ExampleF", spid("ADR", " ", "0029"), 7}, {"ExampleF", spid("STRAND", "-", "10"), 8}},
		},
		{
			"whole-file example", "lib/whole_example_test.go",
			"package lib_test\n\n// helper cites " + spid("REQ", "-", "052") + ".\nfunc helper() {}\n\nfunc ExampleG() {\n\thelper()\n}\n",
			[]wantHit{{"ExampleG", spid("REQ", "-", "052"), 3}},
		},
		{
			"module README", "README.md",
			"# m\n\nSee " + spid("REQ", "-", "052") + ".\n",
			[]wantHit{{"", spid("REQ", "-", "052"), 3}},
		},
		{
			"README of a package directory", "lib/README.md",
			"# lib\n\nFollows " + spid("ADR", " ", "0029") + ".\n",
			[]wantHit{{"", spid("ADR", " ", "0029"), 3}},
		},
		{
			"user document, ids only", "docs/quick-start.md",
			"# Start\n\nUses " + spid("REQ", "-", "052") + ", widget.md and MUST.\n",
			[]wantHit{{"", spid("REQ", "-", "052"), 3}},
		},
		{
			"site page", "pages/index.md",
			"# Home\n\nFollows " + spid("ADR", "-", "0029") + ".\n",
			[]wantHit{{"", spid("ADR", "-", "0029"), 3}},
		},
	}

	root := t.TempDir()
	files := map[string]string{
		"go.mod":                            "module example.com/m\n",
		"docs/specifications/widget.md":     "# Widget\n",
		"docs/specifications/README.md":     "# Specifications\n",
		"docs/adr/0042-widget-choice.md":    "# Widget choice\n",
		"docs/adr/README.md":                "# Decisions\n",
		"docs/specifications/not-a-doc.txt": "text\n",
	}
	type positionCase struct {
		name string
		path string
		want wantHit
	}
	var cross []positionCase
	for i, p := range positions {
		line := 1 + strings.Count(p.src[:strings.Index(p.src, "%s")], "\n")
		for j, f := range forms {
			path := fmt.Sprintf("lib/p%02d_f%02d.go", i, j)
			files[path] = fmt.Sprintf(p.src, "cites "+f.text+" here")
			cross = append(cross, positionCase{p.name + "/" + f.name, path, wantHit{p.ident, f.text, line}})
		}
	}
	for _, c := range notShown {
		files[c.path] = strings.ReplaceAll(c.src, "%s", unseen)
	}
	for _, c := range shown {
		files[c.path] = c.src
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	names, err := specDocNames(root)
	if err != nil {
		t.Fatalf("specDocNames: %v", err)
	}
	hits, _, err := godocHits(root, names)
	if err != nil {
		t.Fatalf("godocHits: %v", err)
	}
	exHits, _, err := exampleHits(root, names)
	if err != nil {
		t.Fatalf("exampleHits: %v", err)
	}
	pageHitList, _, _, err := pageHits(root, names)
	if err != nil {
		t.Fatalf("pageHits: %v", err)
	}
	byFile := map[string][]wantHit{}
	for _, h := range slices.Concat(hits, exHits, pageHitList) {
		byFile[h.file] = append(byFile[h.file], wantHit{h.ident, h.match, h.line})
	}

	check := func(t *testing.T, path string, want []wantHit) {
		t.Helper()
		if got := byFile[path]; !slices.Equal(got, want) {
			t.Errorf("the guard reported %+v for %s, want %+v; source:\n%s", got, path, want, files[path])
		}
	}
	for _, c := range cross {
		t.Run(c.name, func(t *testing.T) {
			check(t, c.path, []wantHit{c.want})
		})
	}
	for _, c := range notShown {
		t.Run("not shown/"+c.name, func(t *testing.T) {
			check(t, c.path, nil)
		})
	}
	for _, c := range shown {
		t.Run("shown/"+c.name, func(t *testing.T) {
			check(t, c.path, c.want)
		})
	}
}

// spid joins a spec-process prefix, a separator and a number at run time,
// so the fixtures never put an identifier in this file for the spec gate to
// read as a citation.
func spid(prefix, sep, number string) string {
	return prefix + sep + number
}

// godocHit is one forbidden form in godoc or a page.
type godocHit struct {
	file  string // module-relative, slash form
	line  int
	ident string // the identifier the comment documents; "" on a page
	form  string // what the form is
	match string // the matched text, its white space collapsed
}

func (h godocHit) String() string {
	what := "the godoc of " + h.ident
	if h.ident == "" {
		what = "the page"
	}
	return fmt.Sprintf("%s:%d: %s carries %s %q", h.file, h.line, what, h.form, h.match)
}

// godocRule is one form godoc must not carry. When re has a group, the
// group is the match; the rest only marks its boundaries.
type godocRule struct {
	form     string
	re       *regexp.Regexp
	probeIDs bool // allowed in a file under godocProbeDirs
	id       bool // applies to the user documents too
}

// godocRules returns the forms godoc must not carry, with the bare file
// names in docNames among them.
func godocRules(docNames []string) []godocRule {
	rules := []godocRule{
		{form: "the requirement id", re: regexp.MustCompile(`\bREQ-\d+`), id: true},
		{form: "the research strand id", re: regexp.MustCompile(`\bSTRAND-\d+`), id: true},
		{form: "the ADR id", re: regexp.MustCompile(`\bADR(?:-|\s*)\d+`), id: true},
		{form: "the spec-process path", re: regexp.MustCompile(`\bdocs/(?:specifications|adr|plans)\b`)},
		{form: "the RFC-2119 keyword", re: regexp.MustCompile(`\b(?:(?:MUST|SHOULD|SHALL)(?:\s+NOT)?|REQUIRED|RECOMMENDED|MAY|OPTIONAL)\b`)},
		{form: "the spec-driven process acronym", re: regexp.MustCompile(`\bSDD\b`), id: true},
		{form: "the probe id", re: regexp.MustCompile(`\bPROBE-\d+`), probeIDs: true, id: true},
	}
	if len(docNames) > 0 {
		quoted := make([]string, len(docNames))
		for i, n := range docNames {
			quoted[i] = regexp.QuoteMeta(n)
		}
		// A name is a whole file name: a letter, digit, "_", "." or "-"
		// right before it, or a letter, digit, "_" or "-" right after it,
		// makes another name.
		rules = append(rules, godocRule{
			form: "the spec or ADR file name",
			re:   regexp.MustCompile(`(?:^|[^\w.-])(` + strings.Join(quoted, "|") + `)(?:$|[^\w-])`),
		})
	}
	return rules
}

// specDocNames returns the file names of the .md documents in root's
// docs/specifications and docs/adr, except README.md.
func specDocNames(root string) ([]string, error) {
	var names []string
	for _, dir := range []string{"docs/specifications", "docs/adr"} {
		paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*.md"))
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			if name := filepath.Base(p); name != "README.md" {
				names = append(names, name)
			}
		}
	}
	return names, nil
}

// inProbeDir reports whether rel, module-relative, lies under one of
// godocProbeDirs.
func inProbeDir(rel string) bool {
	return slices.ContainsFunc(godocProbeDirs, func(dir string) bool { return strings.HasPrefix(rel, dir) })
}

// matchRules returns every form rules forbid in text, which documents
// ident in the file rel; lineAt maps an offset in text to its line.
func matchRules(rules []godocRule, text string, lineAt func(int) int, rel, ident string) []godocHit {
	probeFile := inProbeDir(rel)
	var hits []godocHit
	for _, r := range rules {
		if r.probeIDs && probeFile {
			continue
		}
		for _, m := range r.re.FindAllStringSubmatchIndex(text, -1) {
			start, end := m[0], m[1]
			if len(m) > 2 && m[2] >= 0 {
				start, end = m[2], m[3]
			}
			hits = append(hits, godocHit{
				file:  rel,
				line:  lineAt(start),
				ident: ident,
				form:  r.form,
				match: strings.Join(strings.Fields(text[start:end]), " "),
			})
		}
	}
	return hits
}

// godocHits reads the godoc of the files godocFiles returns under root and
// returns every form godocRules(docNames) forbids, with the number of files
// it read.
func godocHits(root string, docNames []string) ([]godocHit, int, error) {
	files, err := godocFiles(root)
	if err != nil {
		return nil, 0, err
	}
	rules := godocRules(docNames)
	fset := token.NewFileSet()
	parsed := make([]*ast.File, len(files))
	byDir := map[string][]*ast.File{}
	for i, rel := range files {
		file, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(rel)), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, 0, err
		}
		parsed[i] = file
		byDir[path.Dir(rel)] = append(byDir[path.Dir(rel)], file)
	}
	promoted := map[string]map[string]bool{}
	for dir, pkgFiles := range byDir {
		promoted[dir] = promotedTypes(pkgFiles)
	}
	var hits []godocHit
	for i, rel := range files {
		for _, doc := range godocComments(parsed[i], promoted[path.Dir(rel)]) {
			text, lineAt := doc.text(fset)
			hits = append(hits, matchRules(rules, text, lineAt, rel, doc.ident)...)
		}
	}
	return hits, len(files), nil
}

// godocFiles returns the files libraryGoFiles returns under root, then
// those it returns under root's cmd/, whose package docs go doc and
// pkg.go.dev publish too. Below cmd/ it also skips a cmd/cmd or cmd/site
// directory; the module has neither.
func godocFiles(root string) ([]string, error) {
	files, err := libraryGoFiles(root)
	if err != nil {
		return nil, err
	}
	cmd := filepath.Join(root, "cmd")
	switch _, err := os.Stat(cmd); {
	case errors.Is(err, fs.ErrNotExist):
		return files, nil
	case err != nil:
		return nil, err
	}
	cmdFiles, err := libraryGoFiles(cmd)
	if err != nil {
		return nil, err
	}
	for _, rel := range cmdFiles {
		files = append(files, "cmd/"+rel)
	}
	return files, nil
}

// docComment is a comment or string literal go doc shows, with the
// identifier it documents. Exactly one of group and lit is set.
type docComment struct {
	group *ast.CommentGroup
	lit   *ast.BasicLit
	ident string
}

// text returns what go doc shows of d, and a function from an offset in it
// to its line in the file.
func (d docComment) text(fset *token.FileSet) (string, func(int) int) {
	if d.lit != nil {
		return litText(fset, d.lit)
	}
	return commentText(fset, d.group)
}

// godocComments returns the comments and string literals of file that go
// doc shows. promoted names the unexported types of file's package whose
// exported methods go doc shows on an exported struct type.
func godocComments(file *ast.File, promoted map[string]bool) []docComment {
	var out []docComment
	add := func(group *ast.CommentGroup, ident string) {
		if group != nil {
			out = append(out, docComment{group: group, ident: ident})
		}
	}
	add(file.Doc, "package "+file.Name.Name)
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}
			if d.Recv == nil {
				add(d.Doc, d.Name.Name)
				continue
			}
			if base := baseTypeName(d.Recv.List[0].Type); ast.IsExported(base) || promoted[base] {
				add(d.Doc, base+"."+d.Name.Name)
			}
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			first := ""
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if !s.Name.IsExported() {
						continue
					}
					first = cmp.Or(first, s.Name.Name)
					add(s.Doc, s.Name.Name)
					addMembers(add, s)
				case *ast.ValueSpec:
					name := ""
					for _, n := range s.Names {
						if n.IsExported() {
							name = cmp.Or(name, n.Name)
						}
					}
					if name == "" {
						continue
					}
					first = cmp.Or(first, name)
					add(s.Doc, name)
					add(s.Comment, name)
					// go doc prints an exported spec whole, its value
					// with the comments and strings in it.
					for _, group := range file.Comments {
						if group.Pos() > s.Pos() && group.End() <= s.End() {
							add(group, name)
						}
					}
					for _, v := range s.Values {
						out = append(out, stringLits(v, name)...)
					}
				}
			}
			if first != "" {
				add(d.Doc, first)
			}
		}
	}
	return out
}

// stringLits returns the string literals in node, each documenting ident.
func stringLits(node ast.Node, ident string) []docComment {
	var out []docComment
	ast.Inspect(node, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			out = append(out, docComment{lit: lit, ident: ident})
		}
		return true
	})
	return out
}

// promotedTypes returns the unexported types of files, one package's,
// whose exported methods go doc shows on an exported struct type: each
// unexported type such a struct embeds, and each unexported type those
// embed in turn.
func promotedTypes(files []*ast.File) map[string]bool {
	embeds := map[string][]string{}
	var queue []string
	for _, file := range files {
		for _, decl := range file.Decls {
			d, ok := decl.(*ast.GenDecl)
			if !ok || d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				s := spec.(*ast.TypeSpec)
				st, ok := s.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, f := range st.Fields.List {
					base := baseTypeName(f.Type)
					if len(f.Names) > 0 || base == "" || ast.IsExported(base) {
						continue
					}
					if s.Name.IsExported() {
						queue = append(queue, base)
					} else {
						embeds[s.Name.Name] = append(embeds[s.Name.Name], base)
					}
				}
			}
		}
	}
	promoted := map[string]bool{}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if !promoted[name] {
			promoted[name] = true
			queue = append(queue, embeds[name]...)
		}
	}
	return promoted
}

// addMembers adds the comments go doc shows on the exported fields of an
// exported struct type, and on the exported methods and embedded
// interfaces of an exported interface type.
func addMembers(add func(*ast.CommentGroup, string), s *ast.TypeSpec) {
	switch t := s.Type.(type) {
	case *ast.StructType:
		for _, f := range t.Fields.List {
			if name := exportedFieldName(f); name != "" {
				add(f.Doc, s.Name.Name+"."+name)
				add(f.Comment, s.Name.Name+"."+name)
			}
		}
	case *ast.InterfaceType:
		for _, f := range t.Methods.List {
			if name := exportedFieldName(f); name != "" {
				add(f.Doc, s.Name.Name+"."+name)
			}
		}
	}
}

// exportedFieldName returns the first exported name f declares, or, for an
// embedded field, the name of its type when that is exported; otherwise "".
func exportedFieldName(f *ast.Field) string {
	if len(f.Names) == 0 {
		if base := baseTypeName(f.Type); ast.IsExported(base) {
			return base
		}
		return ""
	}
	for _, n := range f.Names {
		if n.IsExported() {
			return n.Name
		}
	}
	return ""
}

// baseTypeName returns the name of the type expr names, through a
// pointer, a package qualifier and type arguments (*T, pkg.T, T[P],
// T[P, Q]); "" for any other expression.
func baseTypeName(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.Ident:
			return e.Name
		case *ast.StarExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.SelectorExpr:
			expr = e.Sel
		default:
			return ""
		}
	}
}

// commentText returns the text of group that go doc shows, a line per
// comment line without the comment markers and without directives, and a
// function from an offset in that text to its line in the file.
func commentText(fset *token.FileSet, group *ast.CommentGroup) (string, func(int) int) {
	var b strings.Builder
	var starts, lines []int
	addLine := func(s string, line int) {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		starts = append(starts, b.Len())
		lines = append(lines, line)
		b.WriteString(s)
	}
	for _, c := range group.List {
		line := fset.Position(c.Slash).Line
		if strings.HasPrefix(c.Text, "//") {
			// The text of a one-comment group drops a directive, as go doc
			// does: //go:generate, //nolint:..., //line and the like.
			if s := (&ast.CommentGroup{List: []*ast.Comment{c}}).Text(); s != "" {
				addLine(strings.TrimSuffix(s, "\n"), line)
			}
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(c.Text, "/*"), "*/")
		for i, s := range strings.Split(body, "\n") {
			addLine(s, line+i)
		}
	}
	return b.String(), func(offset int) int {
		i, _ := slices.BinarySearch(starts, offset+1) // the first line that starts after offset
		return lines[i-1]
	}
}

// litText returns the value of the string literal lit, and a function from
// an offset in it to its line in the file: a raw string's own line breaks
// count, an interpreted string sits on the line it opens on.
func litText(fset *token.FileSet, lit *ast.BasicLit) (string, func(int) int) {
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		s = lit.Value
	}
	line := fset.Position(lit.Pos()).Line
	raw := strings.HasPrefix(lit.Value, "`")
	return s, func(offset int) int {
		if raw {
			return line + strings.Count(s[:offset], "\n")
		}
		return line
	}
}

// exampleHits reads the examples go doc and pkg.go.dev show from the test
// files under root: an example function's doc comment and the comments and
// string literals in its body, or, for a whole-file example, every comment
// and string literal in the file. It returns every form godocRules(docNames)
// forbids, with the number of example functions it read.
func exampleHits(root string, docNames []string) ([]godocHit, int, error) {
	files, err := testGoFiles(root)
	if err != nil {
		return nil, 0, err
	}
	rules := godocRules(docNames)
	fset := token.NewFileSet()
	var hits []godocHit
	count := 0
	for _, rel := range files {
		file, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(rel)), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, 0, err
		}
		var examples []*ast.FuncDecl
		tests, others := false, false
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				name := d.Name.Name
				switch {
				case d.Recv == nil && isTestFunc(name, "Example"):
					examples = append(examples, d)
				case d.Recv == nil && (isTestFunc(name, "Test") || isTestFunc(name, "Benchmark") || isTestFunc(name, "Fuzz")):
					tests = true
				default:
					others = true
				}
			case *ast.GenDecl:
				if d.Tok != token.IMPORT {
					others = true
				}
			}
		}
		count += len(examples)
		var docs []docComment
		if len(examples) == 1 && !tests && others {
			// A whole-file example: go doc shows the file as the example.
			ident := examples[0].Name.Name
			for _, group := range file.Comments {
				docs = append(docs, docComment{group: group, ident: ident})
			}
			for _, decl := range file.Decls {
				if d, ok := decl.(*ast.GenDecl); !ok || d.Tok != token.IMPORT {
					docs = append(docs, stringLits(decl, ident)...)
				}
			}
		} else {
			for _, ex := range examples {
				ident := ex.Name.Name
				if ex.Doc != nil {
					docs = append(docs, docComment{group: ex.Doc, ident: ident})
				}
				for _, group := range file.Comments {
					if group.Pos() > ex.Body.Lbrace && group.End() < ex.Body.Rbrace {
						docs = append(docs, docComment{group: group, ident: ident})
					}
				}
				docs = append(docs, stringLits(ex.Body, ident)...)
			}
		}
		for _, doc := range docs {
			text, lineAt := doc.text(fset)
			hits = append(hits, matchRules(rules, text, lineAt, rel, doc.ident)...)
		}
	}
	return hits, count, nil
}

// isTestFunc reports whether name is a test function name of go test's
// kind prefix (Test, Benchmark, Fuzz, Example): the prefix alone, or
// followed by anything but a lower-case letter.
func isTestFunc(name, prefix string) bool {
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return false
	}
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(r)
}

// testGoFiles returns the module-relative paths of the _test.go files
// under root, cmd/ included, skipping what libraryGoFiles skips besides:
// hidden and underscore directories, testdata, vendor, site and nested
// modules.
func testGoFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" || rel == "site" {
				return fs.SkipDir
			}
			switch _, err := os.Stat(filepath.Join(p, "go.mod")); {
			case err == nil:
				return fs.SkipDir
			case !errors.Is(err, fs.ErrNotExist):
				return err
			}
			return nil
		}
		if strings.HasSuffix(name, "_test.go") {
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

// pageHits reads the pages SDK users read beside go doc. The README.md
// pkg.go.dev shows for the module and for each package directory
// godocFiles finds is read with every rule; the documents userDocPatterns
// names, except userDocSkip, with the id rules alone. It returns the hits
// and the number of README files and of user documents it read.
func pageHits(root string, docNames []string) (hits []godocHit, readmes, userDocs int, err error) {
	files, err := godocFiles(root)
	if err != nil {
		return nil, 0, 0, err
	}
	rules := godocRules(docNames)
	var idRules []godocRule
	for _, r := range rules {
		if r.id {
			idRules = append(idRules, r)
		}
	}
	read := func(rel string, rules []godocRule) (bool, error) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		text := string(b)
		lineAt := func(offset int) int { return 1 + strings.Count(text[:offset], "\n") }
		hits = append(hits, matchRules(rules, text, lineAt, rel, "")...)
		return true, nil
	}

	dirs := []string{"."}
	for _, rel := range files {
		dirs = append(dirs, path.Dir(rel))
	}
	slices.Sort(dirs)
	for _, dir := range slices.Compact(dirs) {
		ok, err := read(path.Join(dir, "README.md"), rules)
		if err != nil {
			return nil, 0, 0, err
		}
		if ok {
			readmes++
		}
	}

	var docs []string
	for _, pattern := range userDocPatterns {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			return nil, 0, 0, err
		}
		for _, m := range matches {
			rel, err := filepath.Rel(root, m)
			if err != nil {
				return nil, 0, 0, err
			}
			if rel = filepath.ToSlash(rel); !slices.Contains(userDocSkip, rel) {
				docs = append(docs, rel)
			}
		}
	}
	for _, rel := range docs {
		ok, err := read(rel, idRules)
		if err != nil {
			return nil, 0, 0, err
		}
		if ok {
			userDocs++
		}
	}
	return hits, readmes, userDocs, nil
}

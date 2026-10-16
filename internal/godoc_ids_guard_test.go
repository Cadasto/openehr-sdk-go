package internal_test

// godoc_ids_guard_test.go: godoc is written for SDK users. AGENTS.md
// § Spec-driven workflow sets the convention: "Godoc is for SDK users.
// Package docs and doc comments on exported identifiers carry no
// spec-process identifiers (REQ, ADR, STRAND, plan or spec paths) and no
// RFC-2119 capitals. State the behaviour a caller needs in plain English. A
// probe's own `PROBE-NNN` id and public openEHR specification citations are
// fine." docs/specifications/README.md says the same of the chain's
// identifiers: "Maintainer comments may cite them too; godoc on exported API
// does not (it is written for SDK users; see AGENTS.md)."
//
// A comment does not show whether go doc prints it, so this reads what go
// doc and pkg.go.dev show: the package comment; the doc comment of an
// exported function, and of an exported method of an exported type; for a
// declaration group that declares an exported type, constant or variable,
// the group's doc comment and each exported spec's doc comment (and a value
// spec's trailing comment); the doc and trailing comment of each exported
// or embedded exported field of an exported struct type; and the doc comment
// of each exported method or embedded exported interface of an exported
// interface type. Directive lines (//go:generate, //nolint:...) are left
// out, as go doc leaves them out. Function bodies, unexported identifiers
// and test files are out of scope.
//
// In that text it reports a requirement, strand or ADR id; the paths
// docs/specifications, docs/adr and docs/plans; the file name of a spec or
// ADR document, read from docs/ when the test runs, so a new document is
// covered without editing this file; an RFC-2119 keyword in capitals; and a
// probe id outside godocProbeDirs. Other .md names (README.md, examples.md,
// ...) and SDK-AQL divergence ids are fine.
//
// The walk covers the files libraryGoFiles (reflect_guard_test.go) returns,
// generated ones and internal/ included, and the files under cmd/: the
// examples' package docs are published too. It must see at least
// minGodocScanFiles files and minSpecDocNames spec and ADR file names: a
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
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// minGodocScanFiles is well below the module's count of non-test Go files,
// cmd/ included, which is over 600.
const minGodocScanFiles = 500

// minSpecDocNames is well below the count of spec and ADR documents, which
// is over 40.
const minSpecDocNames = 30

// godocProbeDirs hold the probe implementations and their runners, whose
// godoc names their own probes.
var godocProbeDirs = []string{"testkit/probes/", "testkit/conformance/"}

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
	for _, h := range hits {
		t.Errorf("%s: godoc is written for SDK users; state the behaviour in plain English and keep the citation in a maintainer comment or a test", h)
	}
	if scanned < minGodocScanFiles {
		t.Fatalf("walk scanned %d non-test Go file(s); the module is known to have at least %d, so the guard has gone blind", scanned, minGodocScanFiles)
	}
	t.Logf("scanned the godoc of %d non-test Go files against %d spec and ADR file names", scanned, len(names))
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
		{"import group", "lib/hidden_import.go", "package lib\n\n// imports %s.\nimport \"fmt\"\n"},
		{"function body", "lib/hidden_body.go", "package lib\n\n// F does nothing.\nfunc F() {\n\t// body %s.\n}\n"},
		{"comment attached to nothing", "lib/hidden_floating.go", "package lib\n\n// floating %s.\n\n// F does nothing.\nfunc F() {}\n"},
		{"directive line", "lib/hidden_directive.go", "package lib\n\n//go:generate echo %s\n// F does nothing.\nfunc F() {}\n"},
		{"test file", "lib/lib_test.go", "// Package lib %s.\npackage lib\n\n// F %s.\nfunc F() {}\n"},
		{"probe id in a probe implementation", "testkit/probes/p/p.go", "// Package p runs " + spid("PROBE", "-", "001") + ".\npackage p\n"},
		{"probe id in a probe runner", "testkit/conformance/c/c.go", "// Package c runs " + spid("PROBE", "-", "001") + ".\npackage c\n"},
		{
			"allowed citations", "lib/allowed.go",
			"package lib\n\n// F follows README.md, examples.md, quick-start.md, architecture.md, docs/architecture.md,\n" +
				"// DIVERGENCES.md, CENSUS.md, SKIPPED.md and deviations.md, divergence SDK-AQL-001, the\n" +
				"// openEHR EHR Information Model specification, rewidget.md, my-widget.md and widget.mdx; a caller must\n" +
				"// and may, and the field is optional.\nfunc F() {}\n",
		},
	}

	// shown are the cases go doc shows besides the positions above, with
	// the hits they produce.
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
	byFile := map[string][]wantHit{}
	for _, h := range hits {
		byFile[h.file] = append(byFile[h.file], wantHit{h.ident, h.match, h.line})
	}

	check := func(t *testing.T, path string, want []wantHit) {
		t.Helper()
		if got := byFile[path]; !slices.Equal(got, want) {
			t.Errorf("godocHits reported %+v for %s, want %+v; source:\n%s", got, path, want, files[path])
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

// godocHit is one forbidden form in godoc.
type godocHit struct {
	file  string // module-relative, slash form
	line  int
	ident string // the identifier the comment documents
	form  string // what the form is
	match string // the matched text, its white space collapsed
}

func (h godocHit) String() string {
	return fmt.Sprintf("%s:%d: the godoc of %s carries %s %q", h.file, h.line, h.ident, h.form, h.match)
}

// godocRule is one form godoc must not carry. When re has a group, the
// group is the match; the rest only marks its boundaries.
type godocRule struct {
	form     string
	re       *regexp.Regexp
	probeIDs bool // allowed in a file under godocProbeDirs
}

// godocRules returns the forms godoc must not carry, with the bare file
// names in docNames among them.
func godocRules(docNames []string) []godocRule {
	rules := []godocRule{
		{form: "the requirement id", re: regexp.MustCompile(`\bREQ-\d+`)},
		{form: "the research strand id", re: regexp.MustCompile(`\bSTRAND-\d+`)},
		{form: "the ADR id", re: regexp.MustCompile(`\bADR(?:-|\s*)\d+`)},
		{form: "the spec-process path", re: regexp.MustCompile(`\bdocs/(?:specifications|adr|plans)\b`)},
		{form: "the RFC-2119 keyword", re: regexp.MustCompile(`\b(?:(?:MUST|SHOULD|SHALL)(?:\s+NOT)?|REQUIRED|RECOMMENDED|MAY|OPTIONAL)\b`)},
		{form: "the probe id", re: regexp.MustCompile(`\bPROBE-\d+`), probeIDs: true},
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
	var hits []godocHit
	for _, rel := range files {
		file, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(rel)), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, 0, err
		}
		probeFile := slices.ContainsFunc(godocProbeDirs, func(dir string) bool { return strings.HasPrefix(rel, dir) })
		for _, doc := range godocComments(file) {
			text, lineAt := commentText(fset, doc.group)
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
						ident: doc.ident,
						form:  r.form,
						match: strings.Join(strings.Fields(text[start:end]), " "),
					})
				}
			}
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

// docComment is a comment go doc shows, with the identifier it documents.
type docComment struct {
	group *ast.CommentGroup
	ident string
}

// godocComments returns the comments of file that go doc shows.
func godocComments(file *ast.File) []docComment {
	var out []docComment
	add := func(group *ast.CommentGroup, ident string) {
		if group != nil {
			out = append(out, docComment{group, ident})
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
			if base := baseTypeName(d.Recv.List[0].Type); ast.IsExported(base) {
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
				}
			}
			if first != "" {
				add(d.Doc, first)
			}
		}
	}
	return out
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

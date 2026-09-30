package probes

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// modulePath is this module's import path.
const modulePath = "github.com/cadasto/openehr-sdk-go"

// backendPackages are the packages through which a probe reaches a backend:
// the standard library's listener and HTTP packages, the OAuth 2.0 client, and
// the SDK packages that open a connection or serve one. An entry ending in
// "/..." also takes in every package below it.
//
// testkit/probe is not on the list. Every probe package names its shared
// Result type there, and that alias is all the probe packages use it for.
var backendPackages = []string{
	"net",
	"net/http/...",
	"golang.org/x/oauth2/...",
	modulePath + "/auth/...",
	modulePath + "/cadasto/admin",
	modulePath + "/openehr/client/...",
	modulePath + "/sandbox",
	modulePath + "/smart/...",
	modulePath + "/transport/...",
}

var (
	probeFuncName = regexp.MustCompile(`^Probe(\d{3})`)
	entryHeading  = regexp.MustCompile(`^#### (PROBE-\d{3})\b`)
	anyHeading    = regexp.MustCompile(`^#{1,6}\s`)
	modesField    = regexp.MustCompile(`^- \*\*Modes:\*\*\s*(.*)$`)
)

// TestREQ082ProbeClassMatchesModes checks that a probe which reaches no
// backend declares In-repo in its Modes line, and that a probe which reaches
// one does not.
func TestREQ082ProbeClassMatchesModes(t *testing.T) {
	// § REQ-082 in docs/specifications/conformance.md: an in-repo probe MUST declare In-repo.
	t.Parallel()
	root := filepath.Join("..", "..")

	probes := loadProbeFuncs(t, root)
	if len(probes) == 0 {
		t.Fatal("found no exported ProbeNNN function under testkit/probes/*/; the scan reads nothing")
	}
	modes := loadModes(t, filepath.Join(root, "docs", "specifications", "conformance.md"))
	if len(modes) == 0 {
		t.Fatal("found no '- **Modes:**' line under a '#### PROBE-NNN' heading in conformance.md; the scan reads nothing")
	}

	for _, id := range slices.Sorted(maps.Keys(probes)) {
		funcs := probes[id]
		line, ok := modes[id]
		if !ok {
			t.Errorf("%s is implemented by %s, but conformance.md has no Modes line for it", id, funcNames(funcs))
			continue
		}
		// The census in § REQ-082 counts a Modes line as in-repo when it
		// contains "In-repo" anywhere, and so does this check.
		inRepo := strings.Contains(line, "In-repo")
		reach := firstBackendReach(funcs)
		switch {
		case reach != "" && inRepo:
			t.Errorf("%s declares In-repo but reaches a backend: %s\n\tModes: %s", id, reach, line)
		case reach == "" && !inRepo:
			t.Errorf("%s reaches no backend (%s and what they call reference no backend package), but its Modes line does not declare In-repo\n\tModes: %s",
				id, funcNames(funcs), line)
		}
	}
}

// probeFunc is one exported ProbeNNN function and the first backend reference
// it reaches, if any.
type probeFunc struct {
	name  string
	reach string // "" when the function reaches no backend package
}

func funcNames(funcs []probeFunc) string {
	names := make([]string, len(funcs))
	for i, f := range funcs {
		names[i] = f.name
	}
	return strings.Join(names, ", ")
}

func firstBackendReach(funcs []probeFunc) string {
	for _, f := range funcs {
		if f.reach != "" {
			return f.reach
		}
	}
	return ""
}

// loadProbeFuncs parses the non-test Go files of every package under
// testkit/probes and returns the exported ProbeNNN functions, grouped by the
// PROBE id their name carries.
func loadProbeFuncs(t *testing.T, root string) map[string][]probeFunc {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("os.ReadDir(testkit/probes): %v", err)
	}
	names := importNames{root: root, cache: map[string]string{}}
	probes := map[string][]probeFunc{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		g, err := loadPackage(e.Name(), &names)
		if err != nil {
			t.Fatalf("testkit/probes/%s: %v", e.Name(), err)
		}
		for _, fn := range g.probes {
			id := "PROBE-" + probeFuncName.FindStringSubmatch(fn)[1]
			probes[id] = append(probes[id], probeFunc{name: fn, reach: g.reach(fn)})
		}
	}
	return probes
}

// decl is one package-level declaration: the backend package members it
// names, and the package-level names it uses. A method is its own decl, keyed
// "Type.Method", and its receiver type uses it.
type decl struct {
	backend []string // "net/http/httptest.NewServer", in source order
	uses    []string
}

type pkgGraph struct {
	decls  map[string]*decl
	probes []string // exported ProbeNNN functions, in source order
}

func (g *pkgGraph) node(name string) *decl {
	d, ok := g.decls[name]
	if !ok {
		d = &decl{}
		g.decls[name] = d
	}
	return d
}

// loadPackage builds the declaration graph of one package directory from its
// non-test Go files.
func loadPackage(dir string, names *importNames) (*pkgGraph, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	g := &pkgGraph{decls: map[string]*decl{}}
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		aliases, err := backendAliases(f, names)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				key := d.Name.Name
				if d.Recv != nil && len(d.Recv.List) > 0 {
					recv := receiverType(d.Recv.List[0].Type)
					key = recv + "." + key
					t := g.node(recv)
					t.uses = append(t.uses, key)
				} else if d.Name.IsExported() && probeFuncName.MatchString(d.Name.Name) {
					g.probes = append(g.probes, d.Name.Name)
				}
				scan(d, aliases, g.node(key))
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						scan(s, aliases, g.node(s.Name.Name))
					case *ast.ValueSpec:
						var refs decl
						scan(s, aliases, &refs)
						for _, n := range s.Names {
							v := g.node(n.Name)
							v.backend = append(v.backend, refs.backend...)
							v.uses = append(v.uses, refs.uses...)
						}
					}
				}
			}
		}
	}
	return g, nil
}

// scan records into d every backend package member n names and every
// identifier it uses. The name after a dot is a field, a method or a member of
// another package, never a package-level name of this one, so it is skipped.
func scan(n ast.Node, aliases map[string]string, d *decl) {
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := n.X.(*ast.Ident); ok {
				if path, ok := aliases[id.Name]; ok {
					d.backend = append(d.backend, path+"."+n.Sel.Name)
					return false
				}
			}
			scan(n.X, aliases, d)
			return false
		case *ast.Ident:
			d.uses = append(d.uses, n.Name)
		}
		return true
	})
}

// reach walks the package-level declarations name uses, breadth first, and
// describes the shortest path to a backend package member, or returns "" when
// there is none.
func (g *pkgGraph) reach(name string) string {
	parent := map[string]string{name: ""}
	queue := []string{name}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		d := g.decls[cur]
		if d == nil {
			continue
		}
		if len(d.backend) > 0 {
			chain := []string{cur}
			for p := parent[cur]; p != ""; p = parent[p] {
				chain = append(chain, p)
			}
			slices.Reverse(chain)
			return strings.Join(chain, " -> ") + " uses " + d.backend[0]
		}
		for _, u := range d.uses {
			if _, seen := parent[u]; seen {
				continue
			}
			if _, ok := g.decls[u]; !ok {
				continue
			}
			parent[u] = cur
			queue = append(queue, u)
		}
	}
	return ""
}

func receiverType(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.StarExpr:
		return receiverType(e.X)
	case *ast.IndexExpr:
		return receiverType(e.X)
	case *ast.IndexListExpr:
		return receiverType(e.X)
	case *ast.Ident:
		return e.Name
	}
	return ""
}

// backendAliases maps the name under which f refers to each backend package it
// imports to that package's import path.
func backendAliases(f *ast.File, names *importNames) (map[string]string, error) {
	aliases := map[string]string{}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		if !isBackendPackage(path) {
			continue
		}
		name, err := names.of(imp, path)
		if err != nil {
			return nil, err
		}
		switch name {
		case "_":
			continue
		case ".":
			return nil, fmt.Errorf("dot import of %s: this check cannot see which names come from it", path)
		}
		aliases[name] = path
	}
	return aliases, nil
}

func isBackendPackage(path string) bool {
	for _, p := range backendPackages {
		if base, ok := strings.CutSuffix(p, "/..."); ok {
			if path == base || strings.HasPrefix(path, base+"/") {
				return true
			}
		} else if path == p {
			return true
		}
	}
	return false
}

// importNames resolves the name an unaliased import is referred to by. For a
// package of this module it reads the package clause; for any other it takes
// the last path element that is not a major-version suffix.
type importNames struct {
	root  string
	cache map[string]string
}

var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

func (n *importNames) of(imp *ast.ImportSpec, path string) (string, error) {
	if imp.Name != nil {
		return imp.Name.Name, nil
	}
	if name, ok := n.cache[path]; ok {
		return name, nil
	}
	rel, ok := strings.CutPrefix(path, modulePath+"/")
	if !ok {
		elems := strings.Split(path, "/")
		name := elems[len(elems)-1]
		if len(elems) > 1 && majorVersion.MatchString(name) {
			name = elems[len(elems)-2]
		}
		n.cache[path] = name
		return name, nil
	}
	files, err := filepath.Glob(filepath.Join(n.root, filepath.FromSlash(rel), "*.go"))
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.PackageClauseOnly)
		if err != nil {
			return "", err
		}
		n.cache[path] = f.Name.Name
		return f.Name.Name, nil
	}
	return "", fmt.Errorf("no non-test Go file for %s under %s", path, n.root)
}

// loadModes returns each catalogue entry's Modes line, keyed by PROBE id.
func loadModes(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%s): %v", path, err)
	}
	modes := map[string]string{}
	entry, fenced := "", false
	for i, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if m := entryHeading.FindStringSubmatch(line); m != nil {
			entry = m[1]
			continue
		}
		if anyHeading.MatchString(line) {
			entry = ""
			continue
		}
		m := modesField.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch _, dup := modes[entry]; {
		case entry == "":
			t.Errorf("%s:%d: a Modes line outside any '#### PROBE-NNN' entry", path, i+1)
		case dup:
			t.Errorf("%s:%d: %s has a second Modes line", path, i+1, entry)
		default:
			modes[entry] = m[1]
		}
	}
	return modes
}

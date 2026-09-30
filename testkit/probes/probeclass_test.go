package probes

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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

// harnessPath is the probe runner's package. A probe package may name its
// Result type there and nothing else: the runner's clients, recorders and
// entry points belong to the runner, never to a probe.
const harnessPath = modulePath + "/testkit/probe"

// connector is one package member through which a probe reaches a backend. A
// member ending in "*" matches every member with that prefix.
type connector struct {
	path, member, reason string
}

// connectors are the members that carry a connection: each opens a listener
// or a connection, sends a request, or is a configured client or backend a
// probe talks to. Naming any other member of these packages, such as a
// constant, a request value or a pure helper, does not make a probe
// backend-facing.
//
// The list is short on purpose. A backend-facing probe that connects through
// a member missing from it is classed in-repo, and the class check then fails
// because that probe's Modes line does not declare In-repo. The fix is to add
// the member here, never to relabel the probe.
//
// http.NewRequest and httptest.NewRequest are not listed: they build a request
// value and send nothing. Whatever sends it is a client or a round tripper,
// and those are listed.
var connectors = []connector{
	{"net", "Dial*", "opens a connection"},
	{"net", "Listen*", "opens a listener"},
	{"net/http", "Client", "sends requests to a server"},
	{"net/http", "DefaultClient", "sends requests to a server"},
	{"net/http", "RoundTripper", "carries requests to a server"},
	{"net/http", "Transport", "carries requests to a server"},
	{"net/http", "DefaultTransport", "carries requests to a server"},
	{"net/http", "Get", "sends a request through DefaultClient"},
	{"net/http", "Head", "sends a request through DefaultClient"},
	{"net/http", "Post", "sends a request through DefaultClient"},
	{"net/http", "PostForm", "sends a request through DefaultClient"},
	{"net/http", "ListenAndServe*", "opens a listener"},
	{"net/http", "Serve", "serves on a listener"},
	{"net/http", "ServeTLS", "serves on a listener"},
	{"net/http/httptest", "NewServer", "starts a test server"},
	{"net/http/httptest", "NewTLSServer", "starts a test server"},
	{"net/http/httptest", "NewUnstartedServer", "builds a test server"},
	{"net/http/httptest", "NewTestServer", "starts a test server"},
	{"net/http/httptest", "Server", "is a running test server"},
	{"golang.org/x/oauth2", "Config", "its Exchange, Client and TokenSource methods call the token endpoint"},
	{"golang.org/x/oauth2", "NewClient", "returns a client that fetches tokens"},
	{"golang.org/x/oauth2", "Transport", "fetches tokens as it carries requests"},
	{modulePath + "/transport", "Client", "is a configured client for a backend"},
	{modulePath + "/transport", "New", "builds a configured client"},
	{modulePath + "/sandbox", "Backend", "is the in-memory backend"},
	{modulePath + "/sandbox", "New", "builds the in-memory backend"},
	{modulePath + "/sandbox", "Scripted", "builds the in-memory backend"},
	{modulePath + "/smart/discovery", "Resolver", "fetches the SMART configuration over HTTP"},
	{modulePath + "/smart/discovery", "NewResolver", "builds a resolver"},
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

	probes, harness := loadProbeFuncs(t, root)
	if len(probes) == 0 {
		t.Fatal("found no exported ProbeNNN function under testkit/probes; the scan reads nothing")
	}
	for _, h := range harness {
		t.Errorf("%s: a probe package names %s; it may name only testkit/probe.Result", h.pos, h.member)
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
			t.Errorf("%s reaches no backend: neither %s nor anything they use names a member in connectors. "+
				"Declare In-repo, or, if the probe does connect, add the member it connects through to connectors\n\tModes: %s",
				id, funcNames(funcs), line)
		}
	}
}

// TestREQ082ProbeClassifierOnFixture runs the classifier over the fixture
// packages in testdata, whose answers are known. Some probes there connect:
// through a helper, a method, an aliased import, init, or a package-level
// var. Others do not: one uses only local code, one names only constants of
// connector packages, and one declares a local that shadows a connecting
// helper's name.
func TestREQ082ProbeClassifierOnFixture(t *testing.T) {
	// § REQ-082: the class check above is only as sound as this classifier.
	t.Parallel()
	tests := []struct {
		dir     string
		want    map[string]string
		harness []string
	}{
		{
			dir: "probeclass",
			want: map[string]string{
				"Probe901ThroughHelper": "Probe901ThroughHelper -> startServer uses net/http/httptest.NewServer",
				"Probe902LocalOnly":     "",
				"Probe903ThroughMethod": "Probe903ThroughMethod -> server.start uses net/http/httptest.NewServer",
				"Probe904Aliased":       "Probe904Aliased uses net/http/httptest.Server",
				"Probe907ConstantOnly":  "",
				"Probe908LocalShadow":   "",
			},
			harness: []string{"testkit/probe.Run"},
		},
		{
			dir: "probeclassinit",
			want: map[string]string{
				"Probe910Plain": "Probe910Plain -> init at package load uses net/http/httptest.NewServer",
			},
		},
		{
			dir: "probeclassvar",
			want: map[string]string{
				"Probe911ThroughVar": "Probe911ThroughVar -> testServer uses net/http/httptest.NewServer",
				"Probe912AtLoad":     "Probe912AtLoad -> testServer at package load uses net/http/httptest.NewServer",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.dir, func(t *testing.T) {
			t.Parallel()
			names := importNames{root: filepath.Join("..", ".."), cache: map[string]string{}}
			g, err := loadPackage(filepath.Join("testdata", tc.dir), &names)
			if err != nil {
				t.Fatalf("loadPackage(testdata/%s): %v", tc.dir, err)
			}
			wantProbes := slices.Sorted(maps.Keys(tc.want))
			if got := slices.Sorted(slices.Values(g.probes)); !slices.Equal(got, wantProbes) {
				t.Fatalf("probe functions found = %v, want %v", got, wantProbes)
			}
			for _, fn := range wantProbes {
				if got := g.reach(fn); got != tc.want[fn] {
					t.Errorf("reach(%s) = %q, want %q", fn, got, tc.want[fn])
				}
			}
			var harness []string
			for _, h := range g.harness {
				harness = append(harness, h.member)
			}
			if !slices.Equal(harness, tc.harness) {
				t.Errorf("testkit/probe members other than Result = %v, want %v", harness, tc.harness)
			}
		})
	}
}

// probeFunc is one exported ProbeNNN function and the first backend reference
// it reaches, if any.
type probeFunc struct {
	name  string
	reach string // "" when the function reaches no connector
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
// PROBE id their name carries, plus every testkit/probe member other than
// Result that a probe package names.
func loadProbeFuncs(t *testing.T, root string) (map[string][]probeFunc, []harnessUse) {
	t.Helper()
	names := importNames{root: root, cache: map[string]string{}}
	probes := map[string][]probeFunc{}
	var harness []harnessUse
	err := filepath.WalkDir(".", func(path string, e fs.DirEntry, err error) error {
		if err != nil || !e.IsDir() {
			return err
		}
		// The go tool skips testdata and names starting with "." or "_";
		// testdata holds the classifier's own fixtures, not probe packages.
		if path != "." && (e.Name() == "testdata" || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_")) {
			return filepath.SkipDir
		}
		g, err := loadPackage(path, &names)
		if err != nil {
			return fmt.Errorf("testkit/probes/%s: %w", path, err)
		}
		for _, fn := range g.probes {
			id := "PROBE-" + probeFuncName.FindStringSubmatch(fn)[1]
			probes[id] = append(probes[id], probeFunc{name: fn, reach: g.reach(fn)})
		}
		harness = append(harness, g.harness...)
		return nil
	})
	if err != nil {
		t.Fatalf("walking testkit/probes: %v", err)
	}
	return probes, harness
}

// decl is one package-level declaration: the connectors it names, the
// package-level names it uses, and the names it selects after a dot. A method
// is its own decl, keyed "Type.Method"; a decl uses every method whose name it
// selects, whatever the receiver, since the receiver's type is not known here.
type decl struct {
	backend []string // "net/http/httptest.NewServer", in source order
	uses    []string
	sels    []string
}

// harnessUse is a testkit/probe member other than Result named in a probe
// package.
type harnessUse struct {
	pos    token.Position
	member string // "testkit/probe.NewClient"
}

type pkgGraph struct {
	decls   map[string]*decl
	probes  []string     // exported ProbeNNN functions, in source order
	load    []string     // init and every var with an initialiser: run when the package loads
	harness []harnessUse // testkit/probe members other than Result
}

func (g *pkgGraph) node(name string) *decl {
	d, ok := g.decls[name]
	if !ok {
		d = &decl{}
		g.decls[name] = d
	}
	return d
}

func (g *pkgGraph) runsAtLoad(name string) {
	if !slices.Contains(g.load, name) {
		g.load = append(g.load, name)
	}
}

// fileScope is what scanning one file needs: the file set for positions, the
// names under which the file refers to its imports, which of them are
// connector packages or the runner, and the names declared inside the
// declaration being scanned.
type fileScope struct {
	fset    *token.FileSet
	imports map[string]bool   // local names of every import
	aliases map[string]string // local name -> import path, connector packages only
	harness string            // local name of testkit/probe, or ""
	local   map[string]bool
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
		fs, err := scopeOf(f, names)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		fs.fset = fset
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				key := d.Name.Name
				switch {
				case d.Recv != nil && len(d.Recv.List) > 0:
					key = receiverType(d.Recv.List[0].Type) + "." + key
				case key == "init":
					g.runsAtLoad(key)
				case d.Name.IsExported() && probeFuncName.MatchString(key):
					g.probes = append(g.probes, key)
				}
				fs.local = localNames(d)
				g.scan(d, fs, g.node(key))
			case *ast.GenDecl:
				for _, s := range d.Specs {
					fs.local = localNames(s)
					switch s := s.(type) {
					case *ast.TypeSpec:
						g.scan(s, fs, g.node(s.Name.Name))
					case *ast.ValueSpec:
						var refs decl
						g.scan(s, fs, &refs)
						for _, n := range s.Names {
							v := g.node(n.Name)
							v.backend = append(v.backend, refs.backend...)
							v.uses = append(v.uses, refs.uses...)
							v.sels = append(v.sels, refs.sels...)
							if len(s.Values) > 0 {
								g.runsAtLoad(n.Name)
							}
						}
					}
				}
			}
		}
	}
	methods := map[string][]string{} // method name -> "Type.Method" keys
	for _, key := range slices.Sorted(maps.Keys(g.decls)) {
		if _, m, ok := strings.Cut(key, "."); ok {
			methods[m] = append(methods[m], key)
		}
	}
	for _, d := range g.decls {
		for _, sel := range d.sels {
			d.uses = append(d.uses, methods[sel]...)
		}
	}
	return g, nil
}

// scan records into d every connector n names, every name it uses that may be
// a package-level declaration, and every name it selects after a dot on a
// value. A name declared inside the declaration (a parameter, a result, a
// local) is not a use. Neither is a struct field name or an identifier key in
// a composite literal, which name fields.
func (g *pkgGraph) scan(n ast.Node, fs fileScope, d *decl) {
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := n.X.(*ast.Ident); ok && !fs.local[id.Name] && fs.imports[id.Name] {
				if path, ok := fs.aliases[id.Name]; ok && isConnector(path, n.Sel.Name) {
					d.backend = append(d.backend, path+"."+n.Sel.Name)
				}
				if id.Name == fs.harness && n.Sel.Name != "Result" {
					g.harness = append(g.harness, harnessUse{
						pos:    fs.fset.Position(n.Pos()),
						member: "testkit/probe." + n.Sel.Name,
					})
				}
				return false
			}
			d.sels = append(d.sels, n.Sel.Name)
			g.scan(n.X, fs, d)
			return false
		case *ast.Field:
			// A struct field's or an interface method's name declares a
			// member; a parameter's name is a local. Only the type is a use.
			g.scan(n.Type, fs, d)
			return false
		case *ast.KeyValueExpr:
			if _, ok := n.Key.(*ast.Ident); !ok {
				g.scan(n.Key, fs, d)
			}
			g.scan(n.Value, fs, d)
			return false
		case *ast.Ident:
			if n.Name != "_" && !fs.local[n.Name] {
				d.uses = append(d.uses, n.Name)
			}
		}
		return true
	})
}

// localNames returns every name declared inside n: parameters, results,
// receivers, type parameters, := and var declarations, range variables and
// labels. It ignores block scope, so a name declared anywhere in n counts as
// local everywhere in n. That can only drop a use, which makes a probe look
// in-repo; for a backend-facing probe the class check then fails loudly.
func localNames(n ast.Node) map[string]bool {
	local := map[string]bool{}
	fields := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, id := range f.Names {
				local[id.Name] = true
			}
		}
	}
	idents := func(exprs ...ast.Expr) {
		for _, e := range exprs {
			if id, ok := e.(*ast.Ident); ok {
				local[id.Name] = true
			}
		}
	}
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			fields(n.Recv)
		case *ast.FuncType:
			fields(n.TypeParams)
			fields(n.Params)
			fields(n.Results)
		case *ast.TypeSpec:
			fields(n.TypeParams)
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				idents(n.Lhs...)
			}
		case *ast.RangeStmt:
			if n.Tok == token.DEFINE {
				idents(n.Key, n.Value)
			}
		case *ast.DeclStmt:
			if gd, ok := n.Decl.(*ast.GenDecl); ok {
				for _, s := range gd.Specs {
					switch s := s.(type) {
					case *ast.ValueSpec:
						for _, id := range s.Names {
							local[id.Name] = true
						}
					case *ast.TypeSpec:
						local[s.Name.Name] = true
					}
				}
			}
		case *ast.LabeledStmt:
			local[n.Label.Name] = true
		}
		return true
	})
	return local
}

// reach walks the package-level declarations name uses, breadth first, and
// describes the shortest path to a connector, or returns "" when there is
// none. Everything that runs when the package loads (init, and every
// package-level var with an initialiser) counts as used by the probe.
func (g *pkgGraph) reach(name string) string {
	type step struct{ parent, label string }
	seen := map[string]step{name: {label: name}}
	queue := []string{name}
	visit := func(from, to, label string) {
		if _, ok := seen[to]; ok {
			return
		}
		if _, ok := g.decls[to]; !ok {
			return
		}
		seen[to] = step{parent: from, label: label}
		queue = append(queue, to)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		d := g.decls[cur]
		if d == nil {
			continue
		}
		if len(d.backend) > 0 {
			var chain []string
			for k := cur; ; k = seen[k].parent {
				chain = append(chain, seen[k].label)
				if k == name {
					break
				}
			}
			slices.Reverse(chain)
			return strings.Join(chain, " -> ") + " uses " + d.backend[0]
		}
		for _, u := range d.uses {
			visit(cur, u, u)
		}
		if cur == name {
			for _, u := range g.load {
				visit(cur, u, u+" at package load")
			}
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

// scopeOf finds the name under which f refers to each of its imports, maps
// those of connector packages to their paths, and finds the name it gives
// testkit/probe.
func scopeOf(f *ast.File, names *importNames) (fileScope, error) {
	fs := fileScope{imports: map[string]bool{}, aliases: map[string]string{}}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return fs, err
		}
		name, err := names.of(imp, path)
		if err != nil {
			return fs, err
		}
		watched := path == harnessPath || isConnectorPackage(path)
		switch {
		case name == "_":
			continue
		case name == "." && watched:
			return fs, fmt.Errorf("dot import of %s: this check cannot see which names come from it", path)
		case name == ".":
			continue
		}
		fs.imports[name] = true
		switch {
		case path == harnessPath:
			fs.harness = name
		case watched:
			fs.aliases[name] = path
		}
	}
	return fs, nil
}

func isConnectorPackage(path string) bool {
	return slices.ContainsFunc(connectors, func(c connector) bool { return c.path == path })
}

func isConnector(path, member string) bool {
	return slices.ContainsFunc(connectors, func(c connector) bool {
		if c.path != path {
			return false
		}
		if prefix, ok := strings.CutSuffix(c.member, "*"); ok {
			return strings.HasPrefix(member, prefix)
		}
		return member == c.member
	})
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

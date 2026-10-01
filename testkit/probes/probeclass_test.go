package probes

import (
	"fmt"
	"go/ast"
	"go/build"
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
// backend-facing. The classifier does not look inside these packages: their
// members are judged by this list alone. It does look inside every other
// package of this module that a probe package imports, directly or through
// another package, so a probe that connects through a helper there is
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
// one does not. What a probe reaches is what it uses in its own package and
// in the other packages of this module that its package imports, directly or
// through another package, including whatever runs when those packages load.
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

	backendFacing := 0
	for _, id := range slices.Sorted(maps.Keys(probes)) {
		if firstBackendReach(probes[id]) != "" {
			backendFacing++
		}
		reportClassMismatch(t, classMismatch(id, probes[id], modes))
	}
	t.Logf("classified %d probes: %d in-repo, %d backend-facing", len(probes), len(probes)-backendFacing, backendFacing)
}

// classMismatch compares the class of the probe id, which funcs implement,
// with its line in modes. It returns what disagrees, or "" when the two agree.
func classMismatch(id string, funcs []probeFunc, modes map[string]string) string {
	line, ok := modes[id]
	if !ok {
		return fmt.Sprintf("%s is implemented by %s, but conformance.md has no Modes line for it", id, funcNames(funcs))
	}
	reach := firstBackendReach(funcs)
	// The census in § REQ-082 counts a Modes line as in-repo when it
	// contains "In-repo" anywhere, and so does this check.
	inRepo := strings.Contains(line, "In-repo")
	switch {
	case reach != "" && inRepo:
		return fmt.Sprintf("%s declares In-repo but reaches a backend: %s\n\tModes: %s", id, reach, line)
	case reach == "" && !inRepo:
		return fmt.Sprintf("%s reaches no backend: neither %s nor anything they use names a member in connectors. "+
			"Declare In-repo, or, if the probe does connect, add the member it connects through to connectors\n\tModes: %s",
			id, funcNames(funcs), line)
	}
	return ""
}

// mismatchReporter is the part of testing.TB that reports a class
// disagreement. The catalogue passes *testing.T; the fixture passes a recorder.
type mismatchReporter interface {
	Helper()
	Error(args ...any)
}

// mismatchCapture records reports so a fixture can assert them without failing itself.
type mismatchCapture struct {
	msgs []string
}

func (mismatchCapture) Helper() {}

func (c *mismatchCapture) Error(args ...any) {
	c.msgs = append(c.msgs, fmt.Sprint(args...))
}

// reportClassMismatch reports msg when it disagrees. The catalogue loop and
// the fixture both call it, so a helper that drops msg fails the fixture.
func reportClassMismatch(r mismatchReporter, msg string) {
	r.Helper()
	if msg != "" {
		r.Error(msg)
	}
}

// TestREQ082ProbeClassMismatch runs the comparison the class check makes over
// fixture probes and Modes lines. The catalogue and the classifier agree
// today, so without this test the class check could stop reporting a
// disagreement and stay green.
func TestREQ082ProbeClassMismatch(t *testing.T) {
	// § REQ-082: an in-repo probe MUST declare In-repo; this pins the comparison that enforces it.
	t.Parallel()
	backendFree := []probeFunc{{name: "Probe901Local"}}
	backendFacing := []probeFunc{
		{name: "Probe901Local"},
		{name: "Probe901Server", reach: "Probe901Server uses net/http/httptest.NewServer"},
	}
	tests := []struct {
		name     string
		funcs    []probeFunc
		modes    map[string]string
		want     string // a phrase the reported mismatch contains; "" when none is reported
		wantMore string // a second phrase it must contain, or ""
	}{
		{
			name:  "backend-free declared Sandbox",
			funcs: backendFree,
			modes: map[string]string{"PROBE-901": "Sandbox."},
			want:  "PROBE-901 reaches no backend",
		},
		{
			name:     "backend-facing declared In-repo",
			funcs:    backendFacing,
			modes:    map[string]string{"PROBE-901": "In-repo (unit-level property; no backend)."},
			want:     "PROBE-901 declares In-repo but reaches a backend",
			wantMore: "Probe901Server uses net/http/httptest.NewServer",
		},
		{
			name:  "backend-free declared In-repo",
			funcs: backendFree,
			modes: map[string]string{"PROBE-901": "In-repo (unit-level property; no backend)."},
		},
		{
			name:  "backend-facing declared Sandbox",
			funcs: backendFacing,
			modes: map[string]string{"PROBE-901": "Sandbox, Cassette, Live."},
		},
		{
			name:  "no Modes line",
			funcs: backendFree,
			modes: map[string]string{"PROBE-902": "In-repo."},
			want:  "PROBE-901 is implemented by Probe901Local, but conformance.md has no Modes line for it",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var rec mismatchCapture
			reportClassMismatch(&rec, classMismatch("PROBE-901", tc.funcs, tc.modes))
			got := strings.Join(rec.msgs, "\n")
			switch {
			case tc.want == "" && got != "":
				t.Errorf("reportClassMismatch(classMismatch(PROBE-901, %s, %v)) = %q, want no mismatch", funcNames(tc.funcs), tc.modes, got)
			case tc.want != "" && !strings.Contains(got, tc.want):
				t.Errorf("reportClassMismatch(classMismatch(PROBE-901, %s, %v)) = %q, want a mismatch containing %q", funcNames(tc.funcs), tc.modes, got, tc.want)
			case tc.wantMore != "" && !strings.Contains(got, tc.wantMore):
				t.Errorf("reportClassMismatch(classMismatch(PROBE-901, %s, %v)) = %q, want it to name the backend path %q", funcNames(tc.funcs), tc.modes, got, tc.wantMore)
			}
		})
	}
}

// TestREQ082CatalogueReportsClassMismatch checks that the catalogue loop
// passes every classMismatch result to something that reports it. The live
// catalogue currently mismatches nothing, so replacing that report with
// `_ = msg` would leave TestREQ082ProbeClassMatchesModes green.
func TestREQ082CatalogueReportsClassMismatch(t *testing.T) {
	// § REQ-082: the catalogue must report a class disagreement, not only compute it.
	t.Parallel()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "probeclass_test.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parser.ParseFile(probeclass_test.go): %v", err)
	}
	fn := funcByName(f, "TestREQ082ProbeClassMatchesModes")
	if fn == nil {
		t.Fatal("TestREQ082ProbeClassMatchesModes is not in probeclass_test.go")
	}
	calls, reported := classMismatchResultsReported(fn)
	if calls == 0 || reported != calls {
		t.Errorf("TestREQ082ProbeClassMatchesModes reports %d of %d classMismatch results, want every result reported", reported, calls)
	}

	// A result counts only as a direct argument of the call that reports it.
	// Each of these shapes drops the text and must report nothing.
	discarded := []struct {
		name string
		body string
	}{
		{
			name: "assigned then discarded",
			body: `if msg := classMismatch(id, probes[id], modes); msg != "" { _ = msg }`,
		},
		{
			name: "wrapped in drop",
			body: `reportClassMismatch(t, drop(classMismatch(id, probes[id], modes)))`,
		},
		{
			name: "wrapped in fmt.Errorf",
			body: `_ = fmt.Errorf("%s", classMismatch(id, probes[id], modes))`,
		},
	}
	for _, tc := range discarded {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "package p\nfunc TestWrapped(t *testing.T) {\n" + tc.body + "\n}\n"
			f, err := parser.ParseFile(token.NewFileSet(), "wrapped.go", src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parser.ParseFile: %v", err)
			}
			wrapped := funcByName(f, "TestWrapped")
			if wrapped == nil {
				t.Fatal("TestWrapped is not in the parsed source")
			}
			calls, reported := classMismatchResultsReported(wrapped)
			if calls != 1 || reported != 0 {
				t.Errorf("classMismatchResultsReported() = %d reported of %d calls, want 0 of 1", reported, calls)
			}
		})
	}
}

// TestREQ082DotImportRefused feeds scopeOf a file that dot-imports a package
// the class walk would otherwise follow. No probe in the catalogue does this,
// so deleting the refusal would leave the catalogue test green.
func TestREQ082DotImportRefused(t *testing.T) {
	// § REQ-082: scopeOf refuses a dot import whose names this check cannot see.
	t.Parallel()
	tests := []struct {
		name string
		path string
	}{
		{name: "connector", path: "net/http"},
		{name: "followed module package", path: modulePath + "/openehr/rm"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "package p\n\nimport . \"" + tc.path + "\"\n"
			f, err := parser.ParseFile(token.NewFileSet(), "dot.go", src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parser.ParseFile: %v", err)
			}
			_, err = scopeOf(f, &importNames{cache: map[string]string{}})
			want := "dot import of " + tc.path + ": this check cannot see which names come from it"
			if err == nil || err.Error() != want {
				t.Errorf("scopeOf(dot import of %s) = %v, want %q", tc.path, err, want)
			}
		})
	}
}

func funcByName(f *ast.File, name string) *ast.FuncDecl {
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if ok && fn.Name.Name == name && fn.Recv == nil {
			return fn
		}
	}
	return nil
}

// classMismatchResultsReported counts classMismatch calls in fn and how many
// of their results are a direct argument of reportClassMismatch or of
// t.Error / t.Errorf. A name the call was assigned to counts only when that
// same identifier is a direct argument of one of those calls.
func classMismatchResultsReported(fn *ast.FuncDecl) (calls, reported int) {
	if fn == nil || fn.Body == nil {
		return 0, 0
	}
	ast.PreorderStack(fn.Body, nil, func(n ast.Node, stack []ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isFunIdent(call, "classMismatch") {
			return true
		}
		calls++
		if classMismatchResultReported(fn, call, stack) {
			reported++
		}
		return true
	})
	return calls, reported
}

func classMismatchResultReported(fn *ast.FuncDecl, call *ast.CallExpr, stack []ast.Node) bool {
	if len(stack) == 0 {
		return false
	}
	switch parent := stack[len(stack)-1].(type) {
	case *ast.CallExpr:
		return isReportingCall(parent) && isDirectArg(parent, call)
	case *ast.AssignStmt:
		name := assignedName(parent, call)
		return name != "" && identPassedToReporter(fn, name)
	default:
		return false
	}
}

func isDirectArg(call *ast.CallExpr, arg ast.Expr) bool {
	return slices.Contains(call.Args, arg)
}

func isFunIdent(call *ast.CallExpr, name string) bool {
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == name
}

func isReportingCall(call *ast.CallExpr) bool {
	if id, ok := call.Fun.(*ast.Ident); ok {
		return id.Name == "reportClassMismatch"
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	return ok && recv.Name == "t" && (sel.Sel.Name == "Error" || sel.Sel.Name == "Errorf")
}

func assignedName(as *ast.AssignStmt, rhs ast.Expr) string {
	for i, e := range as.Rhs {
		if e == rhs && i < len(as.Lhs) {
			id, ok := as.Lhs[i].(*ast.Ident)
			if ok && id.Name != "_" {
				return id.Name
			}
		}
	}
	return ""
}

func identPassedToReporter(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok || !isReportingCall(call) {
			return true
		}
		for _, arg := range call.Args {
			id, ok := arg.(*ast.Ident)
			if ok && id.Name == name {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// TestREQ082ProbeClassifierOnFixture runs the classifier over the fixture
// packages in testdata, whose answers are known. Some probes there connect:
// through a helper, a method, an aliased import, init, or a package-level
// var, in their own package or in another package they import. Others do
// not: one uses only local code, one names only constants of connector
// packages, one declares a local that shadows a connecting helper's name,
// and two use only the parts of another package that open no connection.
func TestREQ082ProbeClassifierOnFixture(t *testing.T) {
	// § REQ-082: the class check above is right only when this classifier is.
	t.Parallel()
	const fixtures = "testkit/probes/testdata/"
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
		{
			dir: "probeclassimport",
			want: map[string]string{
				"Probe913ThroughPackage": "Probe913ThroughPackage -> " + fixtures + "probeclasshelper.StartServer uses net/http/httptest.NewServer",
				"Probe914PureHelper":     "",
				"Probe915ThroughForeignMethod": "Probe915ThroughForeignMethod -> " + fixtures + "probeclasshelper.NewServer -> " +
					fixtures + "probeclasshelper.Server -> " + fixtures + "probeclasshelper.Server.Start uses net/http/httptest.NewServer",
				"Probe916ForeignPureMethod": "",
				"Probe918ThroughTwoPackages": "Probe918ThroughTwoPackages -> " + fixtures + "probeclasshelper.Relayed -> " +
					fixtures + "probeclassdeep.Start uses net/http/httptest.NewServer",
			},
		},
		{
			dir: "probeclassimportload",
			want: map[string]string{
				"Probe917AtImportLoad": "Probe917AtImportLoad -> " + fixtures + "probeclassvar.testServer at package load uses net/http/httptest.NewServer",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.dir, func(t *testing.T) {
			t.Parallel()
			p := newProgram(filepath.Join("..", ".."))
			path := modulePath + "/testkit/probes/testdata/" + tc.dir
			g, err := p.load(path)
			if err != nil {
				t.Fatalf("load(%s): %v", path, err)
			}
			wantProbes := slices.Sorted(maps.Keys(tc.want))
			if got := slices.Sorted(slices.Values(g.probes)); !slices.Equal(got, wantProbes) {
				t.Fatalf("probe functions found = %v, want %v", got, wantProbes)
			}
			for _, fn := range wantProbes {
				got, err := p.reach(path, fn)
				if err != nil {
					t.Fatalf("reach(%s): %v", fn, err)
				}
				if got != tc.want[fn] {
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
	p := newProgram(root)
	probes := map[string][]probeFunc{}
	var harness []harnessUse
	err := filepath.WalkDir(".", func(dir string, e fs.DirEntry, err error) error {
		if err != nil || !e.IsDir() {
			return err
		}
		// The go tool skips testdata and names starting with "." or "_";
		// testdata holds the classifier's own fixtures, not probe packages.
		if dir != "." && (e.Name() == "testdata" || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_")) {
			return filepath.SkipDir
		}
		path := modulePath + "/testkit/probes"
		if dir != "." {
			path += "/" + filepath.ToSlash(dir)
		}
		g, err := p.load(path)
		if err != nil {
			return err
		}
		for _, fn := range g.probes {
			reach, err := p.reach(path, fn)
			if err != nil {
				return err
			}
			id := "PROBE-" + probeFuncName.FindStringSubmatch(fn)[1]
			probes[id] = append(probes[id], probeFunc{name: fn, reach: reach})
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
// package-level names it uses, the members of other packages of this module
// it names, and the names it selects after a dot on a value. A method is its
// own decl, keyed "Type.Method".
type decl struct {
	backend []string // "net/http/httptest.NewServer", in source order
	uses    []string
	ext     []member
	sels    []string
}

// member is a package-level name in another package of this module, which
// the classifier follows.
type member struct {
	path, name string
}

// harnessUse is a testkit/probe member other than Result named in a probe
// package.
type harnessUse struct {
	pos    token.Position
	member string // "testkit/probe.NewClient"
}

type pkgGraph struct {
	decls     map[string]*decl
	methodsOf map[string][]string // type name -> its "Type.Method" keys
	probes    []string            // exported ProbeNNN functions, in source order
	load      []string            // init and every var with an initialiser: run when the package loads
	harness   []harnessUse        // testkit/probe members other than Result
	deps      []string            // packages of this module it imports, connector packages excepted
	files     int
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
// connector packages, other packages of this module or the runner, and the
// names declared inside the declaration being scanned.
type fileScope struct {
	fset    *token.FileSet
	imports map[string]bool   // local names of every import
	aliases map[string]string // local name -> import path, connector packages only
	modules map[string]string // local name -> import path, other packages of this module
	deps    []string          // import paths of the other packages of this module, blank imports included
	harness string            // local name of testkit/probe, or ""
	local   map[string]bool
}

// program loads packages of this module by import path, each once, and walks
// declarations across them.
type program struct {
	root     string
	names    importNames
	pkgs     map[string]*pkgGraph
	closures map[string][]string
}

func newProgram(root string) *program {
	return &program{
		root:     root,
		names:    importNames{root: root, cache: map[string]string{}},
		pkgs:     map[string]*pkgGraph{},
		closures: map[string][]string{},
	}
}

// moduleDir returns the directory of a package of this module, under root.
func moduleDir(root, path string) string {
	rel := strings.TrimPrefix(strings.TrimPrefix(path, modulePath), "/")
	return filepath.Join(root, filepath.FromSlash(rel))
}

func inModule(path string) bool {
	return path == modulePath || strings.HasPrefix(path, modulePath+"/")
}

// load returns the declaration graph of the package with the given import
// path, building it on first use.
func (p *program) load(path string) (*pkgGraph, error) {
	if g, ok := p.pkgs[path]; ok {
		return g, nil
	}
	g, err := loadPackage(moduleDir(p.root, path), &p.names)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimPrefix(path, modulePath+"/"), err)
	}
	p.pkgs[path] = g
	return g, nil
}

// closure loads and returns, sorted, the packages of this module that path
// imports, directly or through another package. Connector packages are not
// entered: their members are judged by connectors.
func (p *program) closure(path string) ([]string, error) {
	if c, ok := p.closures[path]; ok {
		return c, nil
	}
	seen := map[string]bool{path: true}
	queue := []string{path}
	var out []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		g, err := p.load(cur)
		if err != nil {
			return nil, err
		}
		if cur != path && g.files == 0 {
			return nil, fmt.Errorf("%s is imported but has no Go file the go tool builds", cur)
		}
		for _, dep := range g.deps {
			if !seen[dep] {
				seen[dep] = true
				out = append(out, dep)
				queue = append(queue, dep)
			}
		}
	}
	slices.Sort(out)
	p.closures[path] = out
	return out, nil
}

// goFiles returns the non-test Go files in dir that the go tool builds on
// this platform, in name order.
func goFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		ok, err := build.Default.MatchFile(dir, name)
		if err != nil {
			return nil, err
		}
		if ok {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files, nil
}

// loadPackage builds the declaration graph of one package directory from its
// non-test Go files.
func loadPackage(dir string, names *importNames) (*pkgGraph, error) {
	files, err := goFiles(dir)
	if err != nil {
		return nil, err
	}
	g := &pkgGraph{decls: map[string]*decl{}, methodsOf: map[string][]string{}, files: len(files)}
	fset := token.NewFileSet()
	for _, path := range files {
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		fs, err := scopeOf(f, names)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		fs.fset = fset
		for _, dep := range fs.deps {
			if !slices.Contains(g.deps, dep) {
				g.deps = append(g.deps, dep)
			}
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				key := d.Name.Name
				switch {
				case d.Recv != nil && len(d.Recv.List) > 0:
					typ := receiverType(d.Recv.List[0].Type)
					key = typ + "." + key
					g.methodsOf[typ] = append(g.methodsOf[typ], key)
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
							v.ext = append(v.ext, refs.ext...)
							v.sels = append(v.sels, refs.sels...)
							if d.Tok == token.VAR && len(s.Values) > 0 {
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
	for _, keys := range g.methodsOf {
		slices.Sort(keys)
	}
	return g, nil
}

// scan records into d every connector n names, every name it uses that may be
// a package-level declaration, every member of another package of this module
// it names, and every name it selects after a dot on a value. A name declared
// inside the declaration (a parameter, a result, a local) is not a use.
// Neither is a struct field name or an identifier key in a composite literal,
// which name fields.
func (g *pkgGraph) scan(n ast.Node, fs fileScope, d *decl) {
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := n.X.(*ast.Ident); ok && !fs.local[id.Name] && fs.imports[id.Name] {
				if path, ok := fs.aliases[id.Name]; ok && isConnector(path, n.Sel.Name) {
					d.backend = append(d.backend, path+"."+n.Sel.Name)
				}
				if path, ok := fs.modules[id.Name]; ok {
					d.ext = append(d.ext, member{path: path, name: n.Sel.Name})
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
// local everywhere in n. A use outside the local's block can then go unseen:
// a package-level name or, when the local shares an import's name, a
// connector or a member of another package of this module. That can make a
// backend-facing probe look in-repo, and the class check then reports the
// probe unless its Modes line wrongly declares In-repo. A selector on such a
// local counts as a method selection instead, which can only add a path.
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

// node is one declaration: the import path of its package and its key there.
type node struct {
	pkg, name string
}

// reach walks, breadth first, the declarations that the probe name in the
// package path uses, and describes the path to the first connector it
// meets, or returns "" when there is none. The walk follows uses within a
// package and members of the other packages of this module. A selected name
// uses every method of that name in the selecting package, whatever the
// receiver, since the receiver's type is not known here, and every method of
// that name on a type of another package that the walk reaches. Everything
// that runs when the probe's package loads counts as used by the probe: init
// and every package-level var with an initialiser, in that package and in
// every package of this module it imports, directly or through another.
func (p *program) reach(path, name string) (string, error) {
	imported, err := p.closure(path)
	if err != nil {
		return "", err
	}
	type step struct {
		parent node
		label  string
	}
	start := node{pkg: path, name: name}
	label := func(n node) string {
		if n.pkg == path {
			return n.name
		}
		return strings.TrimPrefix(n.pkg, modulePath+"/") + "." + n.name
	}
	seen := map[node]step{start: {label: name}}
	queue := []node{start}
	visit := func(from, to node, suffix string) {
		if _, ok := seen[to]; ok {
			return
		}
		if g := p.pkgs[to.pkg]; g == nil || g.decls[to.name] == nil {
			return
		}
		seen[to] = step{parent: from, label: label(to) + suffix}
		queue = append(queue, to)
	}
	// Methods are matched by name, not by type. In the package of a
	// declaration that selects Start, every method named Start on every type
	// of that package counts as a use, whether or not the walk reached the
	// type. In any package, a method of a type the walk reaches is walked once
	// some visited declaration selects its name. So selecting Start on one
	// type also walks Start on others. That can only add a path to a backend,
	// never hide one. A probe it wrongly classes backend-facing fails the
	// class check loudly, as long as its Modes line declares In-repo.
	selected := map[string]bool{}  // names some visited declaration selects
	pending := map[string][]node{} // methods of visited types, by name, not yet selected
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		g := p.pkgs[cur.pkg]
		d := g.decls[cur.name]
		if len(d.backend) > 0 {
			var chain []string
			for k := cur; ; k = seen[k].parent {
				chain = append(chain, seen[k].label)
				if k == start {
					break
				}
			}
			slices.Reverse(chain)
			return strings.Join(chain, " -> ") + " uses " + d.backend[0], nil
		}
		for _, u := range d.uses {
			visit(cur, node{pkg: cur.pkg, name: u}, "")
		}
		for _, m := range d.ext {
			visit(cur, node{pkg: m.path, name: m.name}, "")
		}
		for _, key := range g.methodsOf[cur.name] {
			_, m, _ := strings.Cut(key, ".")
			method := node{pkg: cur.pkg, name: key}
			if selected[m] {
				visit(cur, method, "")
			} else {
				pending[m] = append(pending[m], method)
			}
		}
		for _, s := range d.sels {
			if selected[s] {
				continue
			}
			selected[s] = true
			for _, method := range pending[s] {
				visit(cur, method, "")
			}
			delete(pending, s)
		}
		if cur == start {
			for _, pkg := range append([]string{path}, imported...) {
				for _, u := range p.pkgs[pkg].load {
					visit(cur, node{pkg: pkg, name: u}, " at package load")
				}
			}
		}
	}
	return "", nil
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
// those of connector packages and of the other packages of this module to
// their paths, and finds the name it gives testkit/probe.
func scopeOf(f *ast.File, names *importNames) (fileScope, error) {
	fs := fileScope{imports: map[string]bool{}, aliases: map[string]string{}, modules: map[string]string{}}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return fs, err
		}
		connector := isConnectorPackage(path)
		followed := inModule(path) && !connector
		if followed {
			fs.deps = append(fs.deps, path)
		}
		name, err := names.of(imp, path)
		if err != nil {
			return fs, err
		}
		switch {
		case name == "_":
			continue
		case name == "." && (connector || followed):
			return fs, fmt.Errorf("dot import of %s: this check cannot see which names come from it", path)
		case name == ".":
			continue
		}
		fs.imports[name] = true
		switch {
		case connector:
			fs.aliases[name] = path
		case followed:
			fs.modules[name] = path
		}
		if path == harnessPath {
			fs.harness = name
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
	if !inModule(path) {
		elems := strings.Split(path, "/")
		name := elems[len(elems)-1]
		if len(elems) > 1 && majorVersion.MatchString(name) {
			name = elems[len(elems)-2]
		}
		n.cache[path] = name
		return name, nil
	}
	files, err := goFiles(moduleDir(n.root, path))
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no non-test Go file for %s under %s", path, n.root)
	}
	f, err := parser.ParseFile(token.NewFileSet(), files[0], nil, parser.PackageClauseOnly)
	if err != nil {
		return "", err
	}
	n.cache[path] = f.Name.Name
	return f.Name.Name, nil
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

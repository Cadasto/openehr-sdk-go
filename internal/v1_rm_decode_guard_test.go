package internal_test

// v1_rm_decode_guard_test.go: REQ-052. wire.md § REQ-052 (the paragraph after
// "Decode-side shape sentinel") rules that the SDK's own non-test code MUST NOT
// decode an RM or AOM 1.4 value through v1 encoding/json: a v1 caller inherits v1's
// decode options (duplicate names last-wins, case-insensitive member names,
// invalid UTF-8 accepted), so an SDK package decoding an RM value that way
// would quietly give up the canonical-JSON guarantees. Nothing at the call
// site shows the difference, so this walks the module's non-test, hand-written
// Go files and holds every v1 decode site to the rule. It has no allow-list.
//
// It is an AST check (go/parser and go/ast only; the module has no
// golang.org/x/tools dependency), so it resolves types syntactically, not with
// the type checker:
//
//   - A v1 decode site is a call to the v1 package's Unmarshal (under whatever
//     name the file imports "encoding/json"), or a one-argument .Decode call on
//     a v1 decoder. A .Decode receiver counts as v1 when it is a
//     <v1>.NewDecoder(...) call, or an identifier the enclosing function
//     assigns from one. In a file that imports v1 and not encoding/json/v2,
//     any other one-argument .Decode counts as v1 too, unless the receiver is
//     clearly something else: a NewDecoder call of another package
//     (canjson.NewDecoder(r).Decode), an identifier assigned from one, or a
//     selector rooted at another imported package (typereg.Default.Decode).
//     The heuristic errs towards checking a site: a false v1 match costs a
//     resolvable target, never a silent pass.
//   - The decode target must resolve to a type expression. It resolves
//     through &x or x where x is declared in an enclosing function by
//     `var x T`, `x := T{}`, `x := new(T)`, or as a parameter of type T; a
//     literal new(T) or &T{}; and `x := maps.Clone(y)` or `slices.Clone(y)`,
//     which take y's type, where y may be a field of the method receiver
//     (m.Extras), looked up in the receiver's struct declaration among the
//     package's files. A target that does not resolve fails the test; it is
//     never skipped.
//   - The type fails the guard when it references the file's import of
//     openehr/rm or openehr/aom/aom14 anywhere inside it (rm.X, []rm.X,
//     map[string]*aom14.X, a generic instantiation such as
//     rm.OriginalVersion[jsontext.Value]). Maps, []byte, local structs and
//     other packages' types pass.
//   - A target passed as an `any` (or other interface) value, typically a
//     helper's parameter, passes only when no non-test file of its package
//     imports openehr/rm or openehr/aom/aom14: an `any` helper can be handed
//     an RM value only by code in a package that can name one. In such a
//     package the site fails and asks for a concrete target. &x with x
//     declared `any` is not such a target: v1 decodes into it only generic
//     maps, slices and scalars.
//   - Inside packages openehr/rm and openehr/aom/aom14 their own types are
//     unqualified, so any v1 decode site there fails outright. Neither package
//     has one today; their generated codec is encoding/json/v2.

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	modulePath    = "github.com/cadasto/openehr-sdk-go"
	rmImportPath  = modulePath + "/openehr/rm"
	aomImportPath = modulePath + "/openehr/aom/aom14"
)

func TestNoV1DecodeIntoRMValues(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(self)) // module root

	fset := token.NewFileSet()
	pkgs := map[string]*pkgInfo{} // by directory, parsed once
	sites := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "gen", "testdata", "resources", "vendor", ".git", ".worktrees", ".claude", "site":
				return fs.SkipDir
			}
			return nil
		}
		if !isScannedGoFile(d.Name()) {
			return nil
		}
		dir := filepath.Dir(path)
		pkg, ok := pkgs[dir]
		if !ok {
			if pkg, err = parsePackage(fset, dir); err != nil {
				return err
			}
			pkgs[dir] = pkg
		}
		file := pkg.files[path]
		imp := fileImports(file)
		if imp.v1 == "" {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		relDir, _ := filepath.Rel(root, dir)
		ownRM := filepath.ToSlash(relDir) == "openehr/rm" || filepath.ToSlash(relDir) == "openehr/aom/aom14"
		for _, s := range v1DecodeSites(file, imp, pkg) {
			sites++
			line := fset.Position(s.call.Pos()).Line
			switch {
			case ownRM:
				t.Errorf("%s:%d: v1 encoding/json decode inside the RM package itself, whose types are unqualified here: the SDK's own non-test code MUST NOT decode an RM or AOM 1.4 value through v1 (wire.md § REQ-052); use encoding/json/v2",
					rel, line)
			case s.typ == nil:
				t.Errorf("%s:%d: v1 encoding/json decode target %s cannot be resolved to a type: declare it locally (var x T, x := T{} or x := new(T)) so this guard can check it (REQ-052)",
					rel, line, exprString(fset, s.target))
			case holdsInterface(s.target) && isInterfaceType(s.typ, pkg) && pkg.importsRM:
				t.Errorf("%s:%d: v1 encoding/json decode into %s typed %s, in a package that imports openehr/rm or openehr/aom/aom14: an interface-typed target could hold an RM value, so decode into a concrete target this guard can check (REQ-052)",
					rel, line, exprString(fset, s.target), exprString(fset, s.typ))
			default:
				if p := referencesRM(s.typ, imp); p != "" {
					t.Errorf("%s:%d: decodes %s, which references %s, through v1 encoding/json: the SDK's own non-test code MUST NOT decode an RM or AOM 1.4 value through v1 (wire.md § REQ-052); use encoding/json/v2 or canjson",
						rel, line, exprString(fset, s.typ), p)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Self-check: the module holds many v1 decode sites (Definition, System,
	// AQL, BMM and auth all stay on v1). A count below the floor means the
	// walk or the matching has gone blind and a green run proves nothing.
	const minSites = 50
	if sites < minSites {
		t.Fatalf("walk found %d v1 decode site(s); the module is known to have at least %d, so the guard has gone blind", sites, minSites)
	}
	t.Logf("checked %d v1 encoding/json decode sites", sites)
}

func isScannedGoFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") && !strings.HasSuffix(name, "_gen.go")
}

// pkgInfo is one directory's non-test files, parsed once: whether any imports
// openehr/rm or openehr/aom/aom14, and its type declarations by name.
type pkgInfo struct {
	files     map[string]*ast.File
	importsRM bool
	types     map[string]ast.Expr
}

func parsePackage(fset *token.FileSet, dir string) (*pkgInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	pkg := &pkgInfo{files: map[string]*ast.File{}, types: map[string]ast.Expr{}}
	for _, e := range entries {
		// Generated files count here: they declare types and import rm.
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		pkg.files[path] = file
		for _, spec := range file.Imports {
			if p, _ := strconv.Unquote(spec.Path.Value); p == rmImportPath || p == aomImportPath {
				pkg.importsRM = true
			}
		}
		for _, decl := range file.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					ts := spec.(*ast.TypeSpec)
					pkg.types[ts.Name.Name] = ts.Type
				}
			}
		}
	}
	return pkg, nil
}

// imports are the local names a file gives the packages this guard reads.
type imports struct {
	v1       string          // name of "encoding/json", or ""
	hasV2    bool            // the file imports "encoding/json/v2"
	rm, aom  string          // names of openehr/rm and openehr/aom/aom14, or ""
	packages map[string]bool // every imported package name
}

func fileImports(file *ast.File) imports {
	imp := imports{packages: map[string]bool{}}
	for _, spec := range file.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := p[strings.LastIndex(p, "/")+1:]
		if p == "encoding/json/v2" {
			name = "json" // v2's package name is json, not v2
		}
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "_" || name == "." {
			continue
		}
		switch p {
		case "encoding/json":
			imp.v1 = name
		case "encoding/json/v2":
			imp.hasV2 = true
		case rmImportPath:
			imp.rm = name
		case aomImportPath:
			imp.aom = name
		}
		imp.packages[name] = true
	}
	return imp
}

// funcScope is one enclosing function: its signature (parameters and, for a
// method, the receiver) and its body.
type funcScope struct {
	recv *ast.FieldList
	typ  *ast.FuncType
	body *ast.BlockStmt
}

// decodeSite is one v1 decode call, its target expression and, when the
// target resolves, the type expression it decodes into.
type decodeSite struct {
	call   *ast.CallExpr
	target ast.Expr
	typ    ast.Expr
}

// v1DecodeSites finds every v1 decode call in file, with the enclosing
// functions each call can see, and resolves its target.
func v1DecodeSites(file *ast.File, imp imports, pkg *pkgInfo) []decodeSite {
	var out []decodeSite
	var visit func(n ast.Node, scopes []funcScope)
	visit = func(n ast.Node, scopes []funcScope) {
		ast.Inspect(n, func(c ast.Node) bool {
			switch x := c.(type) {
			case *ast.FuncDecl:
				if x.Body != nil {
					visit(x.Body, append(scopes, funcScope{x.Recv, x.Type, x.Body}))
				}
				return false
			case *ast.FuncLit:
				visit(x.Body, append(scopes, funcScope{nil, x.Type, x.Body}))
				return false
			case *ast.CallExpr:
				if target, ok := v1DecodeTarget(x, imp, scopes); ok {
					r := resolver{scopes: scopes, pkg: pkg}
					out = append(out, decodeSite{call: x, target: target, typ: r.target(target)})
				}
			}
			return true
		})
	}
	visit(file, nil)
	return out
}

// v1DecodeTarget reports whether call is a v1 decode and returns its target.
func v1DecodeTarget(call *ast.CallExpr, imp imports, scopes []funcScope) (ast.Expr, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	switch sel.Sel.Name {
	case "Unmarshal":
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == imp.v1 && len(call.Args) >= 2 {
			return call.Args[1], true
		}
	case "Decode":
		if len(call.Args) != 1 {
			return nil, false
		}
		switch decoderOrigin(sel.X, imp, scopes) {
		case originV1:
			return call.Args[0], true
		case originOther:
			return nil, false
		case originUnknown:
			// Counted as v1 only where v2 is not imported.
			return call.Args[0], !imp.hasV2
		}
	}
	return nil, false
}

type origin int

const (
	originUnknown origin = iota
	originV1
	originOther
)

// decoderOrigin classifies a .Decode receiver: a NewDecoder call (directly or
// through a local assignment) names its package; a selector rooted at an
// imported package other than v1 is clearly not a v1 decoder.
func decoderOrigin(recv ast.Expr, imp imports, scopes []funcScope) origin {
	switch x := recv.(type) {
	case *ast.CallExpr:
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "NewDecoder" {
			if pkg, ok := sel.X.(*ast.Ident); ok {
				if pkg.Name == imp.v1 {
					return originV1
				}
				return originOther
			}
		}
	case *ast.Ident:
		for _, rhs := range definedValues(x.Name, scopes) {
			if o := decoderOrigin(rhs, imp, nil); o != originUnknown {
				return o
			}
		}
	case *ast.SelectorExpr:
		if root := selectorRoot(x); root != nil && root.Name != imp.v1 && imp.packages[root.Name] {
			return originOther
		}
	}
	return originUnknown
}

// selectorRoot returns the leftmost identifier of a selector chain a.b.c.
func selectorRoot(e ast.Expr) *ast.Ident {
	for {
		switch x := e.(type) {
		case *ast.SelectorExpr:
			e = x.X
		case *ast.Ident:
			return x
		default:
			return nil
		}
	}
}

// definedValues returns every value name is defined from with := in the
// enclosing function bodies.
func definedValues(name string, scopes []funcScope) []ast.Expr {
	var out []ast.Expr
	for _, sc := range scopes {
		ast.Inspect(sc.body, func(n ast.Node) bool {
			s, ok := n.(*ast.AssignStmt)
			if !ok || s.Tok != token.DEFINE || len(s.Lhs) != len(s.Rhs) {
				return true
			}
			for j, lhs := range s.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
					out = append(out, s.Rhs[j])
				}
			}
			return true
		})
	}
	return out
}

// resolver resolves a decode target to its type expression syntactically.
type resolver struct {
	scopes []funcScope
	pkg    *pkgInfo
}

// target returns the type a decode target decodes into, or nil.
func (r resolver) target(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.UnaryExpr: // &x or &T{}
		if x.Op != token.AND {
			return nil
		}
		if lit, ok := x.X.(*ast.CompositeLit); ok {
			return lit.Type
		}
		return r.valueType(x.X)
	case *ast.CallExpr: // new(T)
		return newType(x)
	case *ast.Ident: // a parameter or local already holding a pointer or map
		return r.valueType(x)
	}
	return nil
}

// valueType returns the declared type of an identifier, or of a field of the
// method receiver, or nil.
func (r resolver) valueType(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.Ident:
		return r.identType(x.Name)
	case *ast.SelectorExpr: // recv.Field
		base, ok := x.X.(*ast.Ident)
		if !ok {
			return nil
		}
		for _, sc := range slices.Backward(r.scopes) {
			if recv := sc.recv; recv != nil && len(recv.List) == 1 && len(recv.List[0].Names) == 1 && recv.List[0].Names[0].Name == base.Name {
				return r.fieldType(recv.List[0].Type, x.Sel.Name)
			}
		}
	}
	return nil
}

// identType finds name's declaration in the enclosing functions, innermost
// first: a local in one of the accepted forms, or a parameter.
func (r resolver) identType(name string) ast.Expr {
	for _, sc := range slices.Backward(r.scopes) {
		var found ast.Expr
		ast.Inspect(sc.body, func(n ast.Node) bool {
			switch s := n.(type) {
			case *ast.ValueSpec: // var x T
				for _, id := range s.Names {
					if id.Name == name && s.Type != nil {
						found = s.Type
					}
				}
			case *ast.AssignStmt: // x := T{}, new(T), maps.Clone(y), slices.Clone(y)
				if s.Tok != token.DEFINE || len(s.Lhs) != len(s.Rhs) {
					return true
				}
				for j, lhs := range s.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
						found = r.definedType(s.Rhs[j])
					}
				}
			}
			return found == nil
		})
		if found != nil {
			return found
		}
		if sc.typ.Params != nil {
			for _, f := range sc.typ.Params.List {
				for _, id := range f.Names {
					if id.Name == name {
						return f.Type
					}
				}
			}
		}
	}
	return nil
}

// definedType returns T for T{} and new(T), y's type for maps.Clone(y) and
// slices.Clone(y), and nil for anything else.
func (r resolver) definedType(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.CompositeLit:
		return x.Type
	case *ast.CallExpr:
		if t := newType(x); t != nil {
			return t
		}
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Clone" && len(x.Args) == 1 {
			if pkg, ok := sel.X.(*ast.Ident); ok && (pkg.Name == "maps" || pkg.Name == "slices") {
				return r.valueType(x.Args[0])
			}
		}
	}
	return nil
}

// fieldType looks up field in the struct declaration of the receiver type
// recvType (T, *T or *T[K]) among the package's files.
func (r resolver) fieldType(recvType ast.Expr, field string) ast.Expr {
	if star, ok := recvType.(*ast.StarExpr); ok {
		recvType = star.X
	}
	if idx, ok := recvType.(*ast.IndexExpr); ok {
		recvType = idx.X
	}
	id, ok := recvType.(*ast.Ident)
	if !ok {
		return nil
	}
	st, ok := r.pkg.types[id.Name].(*ast.StructType)
	if !ok {
		return nil
	}
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			if n.Name == field {
				return f.Type
			}
		}
	}
	return nil
}

// newType returns T for new(T), nil for anything else.
func newType(call *ast.CallExpr) ast.Expr {
	if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "new" && len(call.Args) == 1 {
		return call.Args[0]
	}
	return nil
}

// holdsInterface reports whether the decode target is passed as a value, not
// as &x: an interface-typed value can carry a caller's pointer to an RM value,
// while &x with x declared `any` decodes into generic maps and slices only.
func holdsInterface(target ast.Expr) bool {
	_, ok := target.(*ast.Ident)
	return ok
}

// isInterfaceType reports whether typ is `any`, an interface literal, or a
// name the package declares as an interface.
func isInterfaceType(typ ast.Expr, pkg *pkgInfo) bool {
	switch x := typ.(type) {
	case *ast.InterfaceType:
		return true
	case *ast.Ident:
		if x.Name == "any" {
			return true
		}
		_, ok := pkg.types[x.Name].(*ast.InterfaceType)
		return ok
	}
	return false
}

// referencesRM returns the import path of openehr/rm or openehr/aom/aom14 when
// typ names one of their types anywhere inside it, or "".
func referencesRM(typ ast.Expr, imp imports) string {
	hit := ""
	ast.Inspect(typ, func(n ast.Node) bool {
		if hit != "" {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); ok {
			switch {
			case imp.rm != "" && pkg.Name == imp.rm:
				hit = rmImportPath
			case imp.aom != "" && pkg.Name == imp.aom:
				hit = aomImportPath
			}
		}
		return true
	})
	return hit
}

// exprString renders e as Go source for a failure message.
func exprString(fset *token.FileSet, e ast.Expr) string {
	if e == nil {
		return "<nil>"
	}
	var b strings.Builder
	if err := printer.Fprint(&b, fset, e); err != nil {
		return "<expr>"
	}
	return b.String()
}

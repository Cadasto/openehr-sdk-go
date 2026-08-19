package client_test

// route_guard_test.go — REQ-150. The arity half of path-segment validation
// (which catches a `/` smuggled INSIDE one interpolated parameter, where every
// resulting segment is individually legal) runs only when transport.Request.Route
// is set. Route reads like an observability field — it also feeds REQ-090 span
// naming — so a new leaf can silently opt out of the smuggling defence just by
// omitting it. This walks the whole client tree so that cannot happen quietly.
//
// The rule enforced here is deliberately stricter than REQ-150's "path-
// interpolating leaf requests MUST set Route": every transport.Request literal
// under openehr/client must set it, interpolating or not. Classifying a path as
// interpolating means reasoning about string concatenation, which is exactly the
// kind of analysis that goes wrong quietly; requiring Route unconditionally is
// trivially satisfiable (every leaf already does it), and Route on a static path
// is harmless — the arity always matches.
//
// This is an AST check, not a grep: openehr/client/ehr/ehr.go builds its literal
// first and assigns req.Route in a following branch, which a textual scan reports
// as a violation. Crediting that form costs precision, so the credit is tied to
// the literal's own binding identifier — a `.Route =` on any OTHER variable (a
// transport.WireError, say) must not excuse a Route-less request literal sitting
// in the same function, and a function holding two literals must not let one
// literal's assignment cover the other.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const transportPkgPath = "github.com/cadasto/openehr-sdk-go/transport"

func TestEveryClientRequestSetsRoute(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(self) // .../openehr/client

	fset := token.NewFileSet()
	checked := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		pkgName, imported := transportImportName(file)
		if !imported {
			return nil
		}
		if pkgName == "." {
			t.Errorf("%s dot-imports %s, which defeats this guard — import it under a name", rel, transportPkgPath)
			return nil
		}

		// Walk every composite literal in the file, not only those inside a
		// FuncDecl, so a package-scope var cannot slip past.
		ast.Inspect(file, func(n ast.Node) bool {
			scope, isScope := n.(*ast.FuncDecl)
			if !isScope || scope.Body == nil {
				return true
			}
			checked += checkScope(t, fset, rel, scope.Name.Name, pkgName, scope.Body)
			return false // checkScope already descended
		})
		// Anything outside a function body: package-level vars, func literals
		// assigned at package scope.
		for _, decl := range file.Decls {
			gd, isGen := decl.(*ast.GenDecl)
			if !isGen {
				continue
			}
			checked += checkScope(t, fset, rel, "package scope", pkgName, gd)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A guard that silently stops matching is worse than none: if the literals
	// move or the import is aliased in a way this misses, fail loudly rather
	// than pass over an empty set.
	if checked == 0 {
		t.Fatal("found no transport.Request literals under openehr/client — the guard has stopped matching")
	}
	t.Logf("checked %d transport.Request literals", checked)
}

// checkScope reports violations among the transport.Request literals inside
// one scope, and returns how many literals it inspected. A literal missing the
// Route key is excused only when the scope assigns Route on that literal's own
// binding identifier.
func checkScope(t *testing.T, fset *token.FileSet, rel, scopeName, pkgName string, scope ast.Node) int {
	t.Helper()

	// receiver name -> assigns `.Route`
	routeAssigned := map[string]bool{}
	ast.Inspect(scope, func(n ast.Node) bool {
		as, isAssign := n.(*ast.AssignStmt)
		if !isAssign {
			return true
		}
		for _, lhs := range as.Lhs {
			sel, isSel := lhs.(*ast.SelectorExpr)
			if !isSel || sel.Sel.Name != "Route" {
				continue
			}
			if id, isID := sel.X.(*ast.Ident); isID {
				routeAssigned[id.Name] = true
			}
		}
		return true
	})

	// literal -> the identifier it is bound to, when it is bound to one.
	boundTo := map[ast.Node]string{}
	bind := func(lhs, rhs []ast.Expr) {
		if len(lhs) != len(rhs) {
			return
		}
		for i, r := range rhs {
			id, isID := lhs[i].(*ast.Ident)
			if !isID {
				continue
			}
			if lit := requestLiteral(r, pkgName); lit != nil {
				boundTo[lit] = id.Name
			}
		}
	}
	ast.Inspect(scope, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.AssignStmt:
			bind(d.Lhs, d.Rhs)
		case *ast.ValueSpec:
			lhs := make([]ast.Expr, len(d.Names))
			for i, nm := range d.Names {
				lhs[i] = nm
			}
			bind(lhs, d.Values)
		}
		return true
	})

	seen := 0
	ast.Inspect(scope, func(n ast.Node) bool {
		lit, isLit := n.(*ast.CompositeLit)
		if !isLit || !isTransportRequest(lit.Type, pkgName) {
			return true
		}
		seen++
		if litHasKey(lit, "Route") {
			return true
		}
		if id, bound := boundTo[ast.Node(lit)]; bound && routeAssigned[id] {
			return true
		}
		t.Errorf("%s:%d: transport.Request in %s does not set Route — the REQ-150 arity check is Route-gated, so omitting it disables the smuggled-separator defence",
			rel, fset.Position(lit.Pos()).Line, scopeName)
		return true
	})
	return seen
}

// requestLiteral unwraps `&transport.Request{…}` / `transport.Request{…}` and
// returns the literal, or nil when e is something else.
func requestLiteral(e ast.Expr, pkgName string) *ast.CompositeLit {
	if u, isUnary := e.(*ast.UnaryExpr); isUnary && u.Op == token.AND {
		e = u.X
	}
	lit, isLit := e.(*ast.CompositeLit)
	if !isLit || !isTransportRequest(lit.Type, pkgName) {
		return nil
	}
	return lit
}

// transportImportName returns the local name the file imports the transport
// package under ("transport" normally, an alias when renamed, "." for a
// dot-import), and whether it imports it at all.
func transportImportName(f *ast.File) (string, bool) {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != transportPkgPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name, true
		}
		return "transport", true
	}
	return "", false
}

// isTransportRequest reports whether e names <pkgName>.Request.
func isTransportRequest(e ast.Expr, pkgName string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Request" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == pkgName
}

func litHasKey(lit *ast.CompositeLit, key string) bool {
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == key {
			return true
		}
	}
	return false
}

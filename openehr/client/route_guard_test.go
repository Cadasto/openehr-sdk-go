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
// as a violation.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEveryClientRequestSetsRoute(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(self) // .../openehr/client

	fset := token.NewFileSet()
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

		// A Route assigned after the literal (req.Route = "…") counts. Collect
		// them per enclosing function so a literal in one function is not
		// excused by an assignment in another.
		ast.Inspect(file, func(n ast.Node) bool {
			fn, isFn := n.(*ast.FuncDecl)
			if !isFn || fn.Body == nil {
				return true
			}
			assignsRoute := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				as, isAssign := n.(*ast.AssignStmt)
				if !isAssign {
					return true
				}
				for _, lhs := range as.Lhs {
					if sel, isSel := lhs.(*ast.SelectorExpr); isSel && sel.Sel.Name == "Route" {
						assignsRoute = true
					}
				}
				return true
			})
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				lit, isLit := n.(*ast.CompositeLit)
				if !isLit || !isTransportRequest(lit.Type) {
					return true
				}
				if litHasKey(lit, "Route") || assignsRoute {
					return true
				}
				t.Errorf("%s:%d: transport.Request in %s does not set Route — the REQ-150 arity check is Route-gated, so omitting it disables the smuggled-separator defence",
					rel, fset.Position(lit.Pos()).Line, fn.Name.Name)
				return true
			})
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// isTransportRequest reports whether e names transport.Request (the literal is
// written &transport.Request{…} or transport.Request{…} in every leaf).
func isTransportRequest(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Request" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "transport"
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

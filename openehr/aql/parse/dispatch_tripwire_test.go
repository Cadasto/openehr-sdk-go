package parse_test

// REQ-119 · PROBE-090
//
// dispatch_tripwire_test.go is the STRUCTURAL closure of the pointer-twin
// rule. Four review rounds each found another type assertion or type switch
// that decided behaviour from a concrete shape of a sealed vocabulary without
// normalising first — and a mutation test on a known site cannot see a site
// that does not exist yet. This test enumerates the sites mechanically: it
// parses every non-test source file of `openehr/aql` and `openehr/aql/parse`
// and fails on any type assertion or type switch over a sealed-vocabulary
// shape whose enclosing function never calls a deref helper.
//
// The shape list is DERIVED, not maintained: every receiver of the sealed
// interfaces' marker methods (token / expr / validate / isSelectExpr /
// isLimitExpr) is collected from the same sources, so a shape added next
// quarter is covered the day it lands.
//
// Known limits, stated so they are not rediscovered: the check is syntactic
// (no type information), so it keys on TYPE NAMES — a gen./antlr type sharing
// a shape name would false-positive (none does; the names are distinctive) —
// and it is function-granular, so it proves "this function normalises
// somewhere", not "this operand was normalised". The per-position behaviour
// is what the position×kind×carrier sweep in value_position_parity_test.go
// pins; the two guards are designed as a pair.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sealedIfaces are the four sealed vocabularies. The tripwire fails if one of
// these interface declarations disappears (renamed without updating this
// list) so the derivation below cannot silently go blind.
var sealedIfaces = map[string]bool{
	"Value": true, "WhereExpr": true, "SelectExpr": true, "LimitExpr": true,
}

// markerMethods are the sealed interfaces' method names; a type declaring one
// is a vocabulary shape.
var markerMethods = map[string]bool{
	"token": true, "expr": true, "validate": true,
	"isSelectExpr": true, "isLimitExpr": true,
}

func TestSealedVocabularyDispatchSitesNormalise(t *testing.T) {
	dirs := []string{"../../aql", "."}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			files = append(files, f)
		}
	}

	// Pass 1 — derive the shape set from marker-method receivers, and confirm
	// the four interfaces still exist under the expected names.
	shapes := map[string]bool{}
	ifacesSeen := map[string]bool{}
	for _, f := range files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil || len(d.Recv.List) == 0 || !markerMethods[d.Name.Name] {
					continue
				}
				rt := d.Recv.List[0].Type
				if star, ok := rt.(*ast.StarExpr); ok {
					rt = star.X
				}
				if id, ok := rt.(*ast.Ident); ok {
					shapes[id.Name] = true
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if _, isIface := ts.Type.(*ast.InterfaceType); isIface && sealedIfaces[ts.Name.Name] {
						ifacesSeen[ts.Name.Name] = true
					}
				}
			}
		}
	}
	for name := range sealedIfaces {
		if !ifacesSeen[name] {
			t.Fatalf("sealed interface %s not found — if it was renamed, update sealedIfaces "+
				"so this tripwire keeps watching it", name)
		}
	}
	if len(shapes) < 12 {
		t.Fatalf("only %d vocabulary shapes derived — the marker-method derivation has gone blind", len(shapes))
	}
	// The interfaces themselves are dispatchable-on too (`x.(aql.Value)`).
	watched := map[string]bool{}
	for s := range shapes {
		watched[s] = true
	}
	for s := range sealedIfaces {
		watched[s] = true
	}

	// typeName reduces an asserted/case type expression to its bare name:
	// `*aql.FuncCall` → FuncCall, `LiteralExpr` → LiteralExpr.
	var typeName func(e ast.Expr) string
	typeName = func(e ast.Expr) string {
		switch v := e.(type) {
		case *ast.StarExpr:
			return typeName(v.X)
		case *ast.SelectorExpr:
			return v.Sel.Name
		case *ast.Ident:
			return v.Name
		}
		return ""
	}

	// Pass 2 — every function with a watched assert/switch must deref.
	var violations []string
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				return true
			}
			if strings.HasPrefix(strings.ToLower(fd.Name.Name), "deref") {
				return true // the helpers themselves
			}
			derefs := false
			var hits []string
			ast.Inspect(fd.Body, func(m ast.Node) bool {
				switch v := m.(type) {
				case *ast.CallExpr:
					name := ""
					switch fun := v.Fun.(type) {
					case *ast.Ident:
						name = fun.Name
					case *ast.SelectorExpr:
						name = fun.Sel.Name
					}
					if strings.HasPrefix(strings.ToLower(name), "deref") {
						derefs = true
					}
				case *ast.TypeAssertExpr:
					if v.Type != nil && watched[typeName(v.Type)] {
						hits = append(hits, fset.Position(v.Pos()).String()+" asserts "+types.ExprString(v.Type))
					}
				case *ast.TypeSwitchStmt:
					for _, stmt := range v.Body.List {
						cc, ok := stmt.(*ast.CaseClause)
						if !ok {
							continue
						}
						for _, ce := range cc.List {
							if watched[typeName(ce)] {
								hits = append(hits, fset.Position(v.Pos()).String()+" switches over "+types.ExprString(ce))
							}
						}
					}
				}
				return true
			})
			if len(hits) > 0 && !derefs {
				violations = append(violations,
					"func "+fd.Name.Name+" dispatches on a sealed vocabulary without normalising:\n    "+
						strings.Join(hits, "\n    "))
			}
			return true
		})
	}
	for _, v := range violations {
		t.Errorf("%s\n  every dispatch site MUST route through DerefValue / DerefWhere / DerefSelectExpr / "+
			"DerefLimitExpr (or the package-local deref*) before deciding behaviour from a concrete shape (REQ-119)", v)
	}
}

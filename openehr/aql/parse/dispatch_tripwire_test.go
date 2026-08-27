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
// isLimitExpr / versionBracket / versionValidate) is collected from the same
// sources, so a shape added next quarter is covered the day it lands. What IS
// maintained is the two maps below — the vocabularies and their marker names —
// so a whole new sealed interface has to be registered by hand, which is the
// one seam this file cannot derive its way out of.
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

// sealedIfaces are the sealed vocabularies. The tripwire fails if one of these
// interface declarations disappears (renamed without updating this list) so
// the derivation below cannot silently go blind.
//
// VersionPredicate and selectOperand (REQ-163) are registered here on the SAME
// footing as the other four, not because either has a dispatch site to police
// today — production code calls their interface methods and never type-switches
// on them — but because this file exists on the premise that a mutation test
// cannot see a site that does not exist yet. A vocabulary outside the sweep is a
// vocabulary whose first dispatch site lands unwatched.
//
// selectOperand is the openehr/aql projection vocabulary (the write-side mirror
// of [parse.SelectExpr]). It needs its OWN marker name rather than reusing
// isSelectExpr, because the derivation below keys on type names alone and would
// otherwise fold the two packages' shape sets into one — making
// [parse.DerefSelectExpr] answerable for shapes that are not in its vocabulary.
var sealedIfaces = map[string]bool{
	"Value": true, "WhereExpr": true, "SelectExpr": true, "LimitExpr": true,
	"VersionPredicate": true, "selectOperand": true,
}

// markerMethods are the sealed interfaces' method names; a type declaring one
// is a vocabulary shape.
var markerMethods = map[string]bool{
	"token": true, "expr": true, "validate": true,
	"isSelectExpr": true, "isLimitExpr": true,
	"versionBracket": true, "versionValidate": true,
	"selectToken": true,
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
	// every registered interface still exists under the expected name.
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
	if len(shapes) < 15 {
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

// TestDerefSwitchesCoverEveryShape holds the normalisers themselves closed.
//
// The deref helpers are plain type switches — the idiom spec bans reflection —
// which reopens the objection reflection answered: a shape added later is
// normalised only if someone remembers to extend the switch, and a missed case
// falls to the fail-closed default, refusing the new shape's pointer twin on
// one carrier only. This sweep removes the "remembers": it derives each
// vocabulary's shape set from the marker-method receivers (as the tripwire
// above does) and fails when a deref switch is missing a shape's case in
// EITHER carrier form. [aql.EqualValues]'s shape comparison is held the same
// way, value form only (it runs on normalised values).
func TestDerefSwitchesCoverEveryShape(t *testing.T) {
	const aqlDir = "../../aql"
	fset := token.NewFileSet()
	fileDirs := map[*ast.File]string{}
	var files []*ast.File
	for _, dir := range []string{aqlDir, "."} {
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
			fileDirs[f] = dir
		}
	}

	// Derive each vocabulary's shapes from its own marker method. `token` is
	// shared by Value and LimitExpr shapes, so it counts only in the aql
	// package, where LimitExpr's types do not live; `versionBracket` is unique
	// to VersionPredicate and needs no such gate.
	vocab := map[string]map[string]bool{
		"Value": {}, "WhereExpr": {}, "SelectExpr": {}, "LimitExpr": {},
		"VersionPredicate": {}, "selectOperand": {},
	}
	for _, f := range files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
				continue
			}
			rt := fd.Recv.List[0].Type
			if star, ok := rt.(*ast.StarExpr); ok {
				rt = star.X
			}
			id, ok := rt.(*ast.Ident)
			if !ok {
				continue
			}
			switch fd.Name.Name {
			case "isSelectExpr":
				vocab["SelectExpr"][id.Name] = true
			case "isLimitExpr":
				vocab["LimitExpr"][id.Name] = true
			case "validate":
				vocab["WhereExpr"][id.Name] = true
			case "token":
				if fileDirs[f] == aqlDir {
					vocab["Value"][id.Name] = true
				}
			case "versionBracket":
				vocab["VersionPredicate"][id.Name] = true
			case "selectToken":
				vocab["selectOperand"][id.Name] = true
			}
		}
	}
	for name, min := range map[string]int{
		"Value": 8, "WhereExpr": 6, "SelectExpr": 4, "LimitExpr": 2, "VersionPredicate": 3,
		"selectOperand": 5,
	} {
		if len(vocab[name]) < min {
			t.Fatalf("derived only %d %s shapes (floor %d) — the marker derivation has gone blind",
				len(vocab[name]), name, min)
		}
	}

	// The switches under coverage: function name → the vocabulary it must
	// exhaust, and whether the pointer twin case is required too.
	//
	// VersionPredicate and selectOperand (REQ-163) have no row and no
	// normaliser, deliberately: their shapes are UNEXPORTED — selectOperand's
	// interface is unexported too — so no caller outside openehr/aql can form a
	// pointer twin and there is nothing to normalise; the failure mode this
	// sweep exists for is unreachable rather than unhandled. Both are still
	// derived above, with their own floors, so the day a
	// `derefVersionPredicate` or `derefSelectOperand` is written, adding one row
	// here holds it closed over shapes this sweep already knows. Registering a
	// vocabulary without registering a normaliser is the honest state, not an
	// omission.
	targets := map[string]struct {
		vocab    string
		pointers bool
	}{
		"derefValue":      {"Value", true},
		"derefWhere":      {"WhereExpr", true},
		"DerefSelectExpr": {"SelectExpr", true},
		"DerefLimitExpr":  {"LimitExpr", true},
		"sameShape":       {"Value", false},
	}
	seen := map[string]bool{}
	for _, f := range files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Body == nil {
				continue
			}
			want, watched := targets[fd.Name.Name]
			if !watched {
				continue
			}
			seen[fd.Name.Name] = true
			valueCases, pointerCases := map[string]bool{}, map[string]bool{}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSwitchStmt)
				if !ok {
					return true
				}
				for _, stmt := range ts.Body.List {
					cc, ok := stmt.(*ast.CaseClause)
					if !ok {
						continue
					}
					for _, ce := range cc.List {
						if star, ok := ce.(*ast.StarExpr); ok {
							if id, ok := star.X.(*ast.Ident); ok {
								pointerCases[id.Name] = true
							}
							continue
						}
						if id, ok := ce.(*ast.Ident); ok {
							valueCases[id.Name] = true
						}
					}
				}
				return true
			})
			for shape := range vocab[want.vocab] {
				if !valueCases[shape] {
					t.Errorf("%s is missing `case %s:` — a %s shape it no longer normalises",
						fd.Name.Name, shape, want.vocab)
				}
				if want.pointers && !pointerCases[shape] {
					t.Errorf("%s is missing `case *%s:` — the pointer twin would fall to the fail-closed "+
						"default and bind the rule to one carrier (REQ-119)", fd.Name.Name, shape)
				}
			}
		}
	}
	for name := range targets {
		if !seen[name] {
			t.Errorf("deref switch %s not found — if it was renamed, update this sweep so the "+
				"case-coverage hold keeps watching it", name)
		}
	}
}

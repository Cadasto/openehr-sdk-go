package internal_test

// option_guard_test.go — REQ-025. The variadic-option pattern
// (`func F(..., opts ...Option)` then `for _, o := range opts { o(&cfg) }`)
// dereferences the option without checking it. A caller that builds one
// conditionally — `var o WriteOption; if cond { o = WithX() }; F(..., o)` —
// hands over a nil func, and the loop panics on input the exported
// signature invites.
//
// Five sites already guarded this correctly and twenty-two did not, which
// is how the convention drifted in the first place: the guard is invisible
// at the call site and nothing held new code to it. This walks the whole
// module so site twenty-three cannot regress quietly.
//
// It is an AST check, not a grep: the guard may be an `if o != nil` around
// the call or an `if o == nil { continue }` before it, and both must count.

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

func TestEveryOptionLoopGuardsNil(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(self)) // module root

	fset := token.NewFileSet()
	optionLoops := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Generated parsers and vendored corpora are not hand-maintained API.
			if n := d.Name(); n == "gen" || n == "testdata" || n == "resources" || n == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") ||
			strings.HasSuffix(d.Name(), "_gen.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)

		ast.Inspect(file, func(n ast.Node) bool {
			rng, isRange := n.(*ast.RangeStmt)
			if !isRange || rng.Value == nil {
				return true
			}
			v, isIdent := rng.Value.(*ast.Ident)
			if !isIdent || v.Name == "_" {
				return true
			}
			guarded := false // a prior `if o == nil { continue }` in this body
			loopCalls := false
			for _, stmt := range rng.Body.List {
				if ifs, isIf := stmt.(*ast.IfStmt); isIf {
					// Guarded form A: if o != nil { o(&cfg) } — credited
					// only when the call is inside the then-branch.
					if comparesToNil(ifs.Cond, v.Name, token.NEQ) && callsIdent(ifs.Body, v.Name) {
						loopCalls = true
						continue
					}
					// Guarded form B: if o == nil { continue } — guards
					// every later call in this body. Any other `== nil`
					// body (e.g. one that calls the option) earns no
					// credit and falls through to the call check.
					if comparesToNil(ifs.Cond, v.Name, token.EQL) && bodyIsContinue(ifs.Body) {
						guarded = true
						continue
					}
				}
				if callsIdent(stmt, v.Name) {
					loopCalls = true
					if !guarded {
						pos := fset.Position(stmt.Pos())
						t.Errorf("%s:%d: calls option %q without a nil guard — a caller can pass a nil option, and REQ-025 forbids panicking on that; wrap it in `if %s != nil { ... }`",
							rel, pos.Line, v.Name, v.Name)
					}
				}
			}
			if loopCalls {
				optionLoops++
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Self-check: the module is known to hold at least this many
	// value-calling range loops. A count below the floor means the walk
	// or the pattern matching has gone blind and a green run proves
	// nothing — lower the floor only for a deliberate API removal.
	const minOptionLoops = 30
	if optionLoops < minOptionLoops {
		t.Fatalf("walk found %d option loop(s); the module is known to have at least %d — the tripwire has gone blind", optionLoops, minOptionLoops)
	}
}

// bodyIsContinue reports whether body is exactly `{ continue }`.
func bodyIsContinue(body *ast.BlockStmt) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	br, ok := body.List[0].(*ast.BranchStmt)
	return ok && br.Tok == token.CONTINUE
}

// comparesToNil reports whether cond is `name <op> nil`.
func comparesToNil(cond ast.Expr, name string, op token.Token) bool {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op != op {
		return false
	}
	x, isIdent := bin.X.(*ast.Ident)
	y, isNil := bin.Y.(*ast.Ident)
	return isIdent && isNil && x.Name == name && y.Name == "nil"
}

// callsIdent reports whether stmt invokes name as a function.
func callsIdent(stmt ast.Stmt, name string) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == name {
			found = true
		}
		return true
	})
	return found
}

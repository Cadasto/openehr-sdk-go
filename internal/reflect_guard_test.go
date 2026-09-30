package internal_test

// reflect_guard_test.go: REQ-024. idiom.md § Generics policy (REQ-024) rules
// that library code MUST NOT use reflection to dispatch on the `_type`
// discriminator: the type registry (openehr/rm/typereg) is the only
// mechanism that projects `_type` onto a concrete Go type. A reflect call does not show at
// the call site whether it chooses a type, so this walks the module's
// non-test Go files, generated ones included, and holds every "reflect"
// import to a reviewed list. Each entry names the file, the reflect
// identifiers it may use, and why that use chooses no Go type from `_type`.
//
// The test fails on a file that imports reflect and is not on the list, on a
// listed file that starts using another reflect identifier, on a dot import
// of reflect (its uses would be unqualified and unseen), and on a list entry
// whose file no longer imports reflect, so every change to reflection use
// meets a reviewer and the list cannot rot.
//
// Out of scope: cmd/ (example programs), testkit/ (test support) and test
// files. The walk must see at least minReflectScanFiles files: a green run
// over an empty or mis-rooted tree proves nothing.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// reflectReviewed is the reviewed list, keyed by module-relative path. None
// of these uses is struct-tag field mapping, the case the section names;
// each is other reflection that chooses no Go type from `_type`.
var reflectReviewed = map[string]struct {
	members []string
	reason  string
}{
	"openehr/aql/parse/ast.go": {
		[]string{"Pointer", "ValueOf"},
		"typed-nil guard before a type switch on parser nodes; chooses no type",
	},
	"openehr/bmm/loadall.go": {
		[]string{"DeepEqual"},
		"value equality of two BMM class definitions reached through a shared ancestor",
	},
	"openehr/rm/typereg/streaming.go": {
		[]string{"TypeFor"},
		"names the target interface in error messages; the concrete type comes from the registry lookup",
	},
	"openehr/serialize/canxml/marshal.go": {
		[]string{"Chan", "Func", "Interface", "Map", "New", "Pointer", "Slice", "Struct", "ValueOf"},
		"addressable copy to reach pointer-receiver methods, and a typed-nil check; xsi:type dispatch stays with the registry",
	},
	"openehr/serialize/simplified/rmattr_value_encode.go": {
		[]string{"Pointer", "ValueOf"},
		"pointer-or-zero test on an interval bound; chooses no type",
	},
}

// minReflectScanFiles is well below the module's non-test Go file count
// (about 500 outside cmd/ and testkit/).
const minReflectScanFiles = 400

func TestREQ024NoReflectTypeDispatch(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(self)) // module root

	fset := token.NewFileSet()
	scanned := 0
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "cmd" || rel == "testkit" {
				return fs.SkipDir
			}
			switch d.Name() {
			case "testdata", "vendor", ".git", ".worktrees", ".claude", "site":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		scanned++
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		name, imported := reflectImportName(file)
		if !imported {
			return nil
		}
		seen[rel] = true
		entry, listed := reflectReviewed[rel]
		if !listed {
			t.Errorf("%s imports reflect and is not on the reviewed list: library code MUST NOT use reflection to dispatch on `_type` (idiom.md § Generics policy (REQ-024)); use the type registry, or add the file to reflectReviewed with the identifiers it uses and why they choose no type", rel)
			return nil
		}
		if name == "." {
			t.Errorf("%s dot-imports reflect, which hides its uses from this guard (REQ-024); import it by name", rel)
			return nil
		}
		full, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, m := range reflectMembers(full, name) {
			if !slices.Contains(entry.members, m) {
				t.Errorf("%s uses reflect.%s, which its reviewed entry (%s) does not list: check it chooses no Go type from `_type` (REQ-024), then add it to the entry", rel, m, entry.reason)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < minReflectScanFiles {
		t.Fatalf("walk scanned %d non-test Go file(s); the module is known to have at least %d, so the guard has gone blind", scanned, minReflectScanFiles)
	}
	for rel := range reflectReviewed {
		if !seen[rel] {
			t.Errorf("reviewed entry %s no longer imports reflect (or the file moved): remove the entry so the list cannot admit a later import unreviewed (REQ-024)", rel)
		}
	}
	t.Logf("scanned %d non-test Go files; %d import reflect", scanned, len(seen))
}

// reflectImportName returns the name a file gives the "reflect" import, and
// whether it imports it at all. A blank import counts as an import.
func reflectImportName(file *ast.File) (string, bool) {
	for _, spec := range file.Imports {
		if p, err := strconv.Unquote(spec.Path.Value); err != nil || p != "reflect" {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name, true
		}
		return "reflect", true
	}
	return "", false
}

// reflectMembers returns the sorted, distinct identifiers the file selects
// from the reflect import named name (reflect.ValueOf, reflect.Pointer, ...).
func reflectMembers(file *ast.File, name string) []string {
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == name && !slices.Contains(out, sel.Sel.Name) {
			out = append(out, sel.Sel.Name)
		}
		return true
	})
	slices.Sort(out)
	return out
}

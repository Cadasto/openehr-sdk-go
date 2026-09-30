package internal_test

// reflect_guard_test.go: REQ-024. idiom.md § Generics policy (REQ-024) rules
// that library code MUST NOT use reflection, with two exceptions: ordinary
// field mapping over struct tags, and uses that never choose a Go type from
// `_type`. The second exception has four classes: a typed-nil or zero-value
// check, a value comparison (reflect.DeepEqual), a type name in an error
// message, and an addressable copy that reaches pointer-receiver methods. The
// type registry (openehr/rm/typereg) stays the only mechanism that projects
// `_type` onto a concrete Go type.
//
// A reflect call does not show at the call site which class it belongs to,
// so this walks the module's non-test Go files, generated ones included, and
// holds every "reflect" import to a reviewed list. Each entry names the file,
// the reflect identifiers it may use, the classes those uses fall in and why.
// The test fails on a file that imports reflect and is not on the list, on a
// listed file that starts using another reflect identifier, on an entry
// without a named class, on a dot import of reflect (its uses would be
// unqualified and unseen), and on an entry whose file no longer imports
// reflect, so every change to reflection use meets a reviewer and the list
// cannot rot.
//
// testkit/ is in scope: it is published for SDK consumers. Out of scope:
// cmd/ (example programs, not importable library code) and test files. The
// walk must see at least minReflectScanFiles files: a green run over an empty
// or mis-rooted tree proves nothing.

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

// The classes of reflection the section allows.
const (
	reflectFieldMapping    = "field mapping over struct tags"
	reflectNilOrZeroCheck  = "typed-nil or zero-value check"
	reflectValueComparison = "value comparison"
	reflectTypeName        = "type name in an error message"
	reflectAddressableCopy = "addressable copy to reach pointer-receiver methods"
)

var reflectClasses = []string{reflectFieldMapping, reflectNilOrZeroCheck, reflectValueComparison, reflectTypeName, reflectAddressableCopy}

// reflectReviewed is the reviewed list, keyed by module-relative path.
var reflectReviewed = map[string]struct {
	members []string
	classes []string
	reason  string
}{
	"openehr/aql/parse/ast.go": {
		[]string{"Pointer", "ValueOf"},
		[]string{reflectNilOrZeroCheck},
		"typed-nil guard before a type switch on parser nodes",
	},
	"openehr/bmm/loadall.go": {
		[]string{"DeepEqual"},
		[]string{reflectValueComparison},
		"equality of two BMM class definitions reached through a shared ancestor",
	},
	"openehr/rm/typereg/streaming.go": {
		[]string{"TypeFor"},
		[]string{reflectTypeName},
		"names the target interface in error messages; the concrete type comes from the registry lookup",
	},
	"openehr/serialize/canxml/marshal.go": {
		[]string{"Chan", "Func", "Interface", "Map", "New", "Pointer", "Slice", "Struct", "ValueOf"},
		[]string{reflectAddressableCopy, reflectNilOrZeroCheck},
		"reaches BMMName and MarshalXML on a value-typed field, and omits absent polymorphic fields; xsi:type dispatch stays with the registry",
	},
	"openehr/serialize/simplified/rmattr_value_encode.go": {
		[]string{"Pointer", "ValueOf"},
		[]string{reflectNilOrZeroCheck},
		"tells a set interval bound from an unset one",
	},
	"testkit/probes/serialize/probe_030_canjson_round_trip.go": {
		[]string{"DeepEqual"},
		[]string{reflectValueComparison},
		"compares the two decoded values of a round trip",
	},
	"testkit/wireequiv/wireequiv.go": {
		[]string{"DeepEqual"},
		[]string{reflectValueComparison},
		"compares two decoded wire documents",
	},
}

// minReflectScanFiles is well below the module's non-test Go file count
// outside cmd/ (about 580).
const minReflectScanFiles = 400

func TestREQ024ReflectOnlyForReviewedUses(t *testing.T) {
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
			if rel == "cmd" {
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
			t.Errorf("%s imports reflect and is not on the reviewed list: library code MUST NOT use reflection outside the classes idiom.md § Generics policy (REQ-024) names; use the type registry or a type switch, or, for a use in a named class, add the file to reflectReviewed with its identifiers and class", rel)
			return nil
		}
		if len(entry.classes) == 0 {
			t.Errorf("%s: reviewed entry names no class; every reflect use must fall in a class idiom.md § Generics policy (REQ-024) names", rel)
		}
		for _, c := range entry.classes {
			if !slices.Contains(reflectClasses, c) {
				t.Errorf("%s: reviewed entry names class %q, which idiom.md § Generics policy (REQ-024) does not name", rel, c)
			}
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
				t.Errorf("%s uses reflect.%s, which its reviewed entry (%s) does not list: check the use falls in a class idiom.md § Generics policy (REQ-024) names, then add it to the entry", rel, m, entry.reason)
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

package constraints_test

// pure_imports_test.go: REQ-103. clinical-modeling.md § Validate contract
// (under § REQ-103) rules that validators MUST be pure functions with no
// I/O. Nothing at a call site shows that a validator stays in memory, so
// this reads the package's non-test Go files and holds every import to a
// list of packages that do no I/O. "os", "io", "net", "reflect" and the rest
// of the standard library are not on it. Two listed packages also have
// members that do I/O or read the clock; a use of one of those fails too, and
// so does a dot import of either package, whose uses would be unqualified and
// unseen. A scan that reads no file fails: a green run over nothing proves
// nothing.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// pureImports is every package the constraints package may import, with the
// reason it does no I/O and the members, if any, that do I/O or read the
// clock and so stay out.
var pureImports = map[string]struct {
	reason string
	banned []string
}{
	"errors":  {reason: "builds error values in memory"},
	"fmt":     {reason: "formats strings and errors in memory; Print* write to standard output and Scan* read standard input", banned: []string{"Print", "Printf", "Println", "Scan", "Scanf", "Scanln"}},
	"regexp":  {reason: "compiles and matches patterns in memory"},
	"slices":  {reason: "works on slices in memory"},
	"strings": {reason: "works on strings in memory"},
	"time":    {reason: "parses and compares time values in memory; the banned members read or wait on the clock, or read the time zone database", banned: []string{"After", "AfterFunc", "LoadLocation", "NewTicker", "NewTimer", "Now", "Since", "Sleep", "Tick", "Until"}},
}

func TestREQ103ValidatorsImportNoIO(t *testing.T) {
	t.Parallel()
	// go test runs in the package's source directory.
	scanned, problems, err := impureImports(".")
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("scanned no non-test Go file in the package directory, so the guard has gone blind")
	}
	for _, p := range problems {
		t.Errorf("%s: validators MUST be pure functions with no I/O (clinical-modeling.md § Validate contract, REQ-103)", p)
	}
	t.Logf("scanned %d non-test Go files", scanned)
}

// TestREQ103ImpureImportsFlagsIO runs the scan over one-file packages, so the
// guard above is shown to fail on each kind of import or use it exists to
// catch, and to pass a file that stays in memory.
func TestREQ103ImpureImportsFlagsIO(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		file        string
		src         string
		wantScanned int
		wantProblem string // a substring of the one problem expected, or "" for none
	}{
		{
			name:        "os import",
			file:        "v.go",
			src:         "package p\n\nimport \"os\"\n\nvar _ = os.ReadFile\n",
			wantScanned: 1,
			wantProblem: `imports "os"`,
		},
		{
			name:        "reflect import",
			file:        "v.go",
			src:         "package p\n\nimport \"reflect\"\n\nvar _ = reflect.TypeOf\n",
			wantScanned: 1,
			wantProblem: `imports "reflect"`,
		},
		{
			name:        "fmt member that writes to standard output",
			file:        "v.go",
			src:         "package p\n\nimport \"fmt\"\n\nfunc f() { fmt.Println(\"x\") }\n",
			wantScanned: 1,
			wantProblem: "fmt.Println",
		},
		{
			name:        "renamed fmt import",
			file:        "v.go",
			src:         "package p\n\nimport f \"fmt\"\n\nfunc g() { f.Println(\"x\") }\n",
			wantScanned: 1,
			wantProblem: "fmt.Println",
		},
		{
			name:        "dot import of fmt",
			file:        "v.go",
			src:         "package p\n\nimport . \"fmt\"\n\nvar _ = Sprintf\n",
			wantScanned: 1,
			wantProblem: `dot-imports "fmt"`,
		},
		{
			name:        "time member that reads the clock",
			file:        "v.go",
			src:         "package p\n\nimport \"time\"\n\nvar _ = time.Now\n",
			wantScanned: 1,
			wantProblem: "time.Now",
		},
		{
			name:        "file that stays in memory",
			file:        "v.go",
			src:         "package p\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n\t\"time\"\n)\n\nvar (\n\t_ = fmt.Sprintf\n\t_ = strings.Cut\n\t_ = time.Parse\n)\n",
			wantScanned: 1,
		},
		{
			name: "test file",
			file: "v_test.go",
			src:  "package p\n\nimport \"os\"\n\nvar _ = os.ReadFile\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.src), 0o600); err != nil {
				t.Fatal(err)
			}
			scanned, problems, err := impureImports(dir)
			if err != nil {
				t.Fatalf("impureImports: %v", err)
			}
			if scanned != tc.wantScanned {
				t.Errorf("impureImports scanned %d file(s), want %d", scanned, tc.wantScanned)
			}
			switch {
			case tc.wantProblem == "" && len(problems) != 0:
				t.Errorf("impureImports(%q) = %q, want no problem", tc.src, problems)
			case tc.wantProblem != "" && (len(problems) != 1 || !strings.Contains(problems[0], tc.wantProblem)):
				t.Errorf("impureImports(%q) = %q, want one problem naming %s", tc.src, problems, tc.wantProblem)
			}
		})
	}

	t.Run("empty directory", func(t *testing.T) {
		t.Parallel()
		scanned, problems, err := impureImports(t.TempDir())
		if err != nil || scanned != 0 || len(problems) != 0 {
			t.Errorf("impureImports(empty dir) = %d, %q, %v, want 0 files, no problem, no error", scanned, problems, err)
		}
	})
}

// impureImports parses the non-test Go files in dir and returns how many it
// read, and one line for each import, dot import or member use that
// pureImports does not allow.
func impureImports(dir string) (int, []string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, nil, err
	}
	fset := token.NewFileSet()
	scanned := 0
	var problems []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return 0, nil, err
		}
		scanned++
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return 0, nil, err
			}
			allowed, ok := pureImports[path]
			if !ok {
				problems = append(problems, fmt.Sprintf("%s imports %q, which is not on the list of packages that do no I/O", name, path))
				continue
			}
			if len(allowed.banned) == 0 {
				continue
			}
			local := path[strings.LastIndex(path, "/")+1:]
			if spec.Name != nil {
				local = spec.Name.Name
			}
			switch local {
			case "_":
				continue
			case ".":
				problems = append(problems, fmt.Sprintf("%s dot-imports %q, which hides any use of %s", name, path, strings.Join(allowed.banned, ", ")))
				continue
			}
			for _, m := range selectedMembers(file, local) {
				if slices.Contains(allowed.banned, m) {
					problems = append(problems, fmt.Sprintf("%s uses %s.%s, which does I/O or reads the clock", name, path, m))
				}
			}
		}
	}
	return scanned, problems, nil
}

// selectedMembers returns the sorted, distinct names the file selects from
// the import it calls local (local.Name).
func selectedMembers(file *ast.File, local string) []string {
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == local && !slices.Contains(out, sel.Sel.Name) {
			out = append(out, sel.Sel.Name)
		}
		return true
	})
	slices.Sort(out)
	return out
}

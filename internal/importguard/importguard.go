// Package importguard finds forbidden imports in the packages a Go package
// pulls in from its own module.
//
// [Scan] reads the non-test files of one package, then follows every import
// that belongs to the same module, so a forbidden import one or more packages
// down is found as well as a direct one. [ScanStd] follows the standard
// library's packages too. [Imports] reads one package only, for a rule that
// holds on a package's own imports but not on everything below it. All three
// read source files only and run no subprocess.
//
// # Which files count
//
// A file counts when some build may compile it, not only the build on this
// machine. A file that this machine leaves out because of a GOOS or GOARCH
// suffix in its name, a //go:build line, or a cgo import while cgo is off
// still counts. This is the rule go mod tidy and go mod vendor use to collect
// a package's imports: every build tag is taken as set or unset, whichever the
// file needs, except "ignore", which is never set. So a //go:build ignore
// file, the conventional way to keep a tool program in the directory, does not
// count. Like the go command, the guard does not read the package clause, so a
// file that names another package still counts. Test files never count.
//
// Only //go:build lines are read, not the older // +build form. A file whose
// only constraint is a // +build ignore line therefore counts. That is the
// stricter mistake, and a harmless one: gofmt has written a //go:build line
// beside every // +build line since Go 1.17.
package importguard

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// WireLayers returns the wire-layer packages a standalone building block must
// not pull in: transport, auth and openehr/client. Each call returns a new
// slice, so a caller may change it without affecting other callers.
func WireLayers() []string {
	// REQ-013 (module-layout.md § REQ-013): the one list the building-block
	// guards share. TestWireLayers pins it entry by entry.
	return []string{
		"github.com/cadasto/openehr-sdk-go/transport",
		"github.com/cadasto/openehr-sdk-go/auth",
		"github.com/cadasto/openehr-sdk-go/openehr/client",
	}
}

// Violation is one forbidden import found in the closure.
type Violation struct {
	Importer string // import path of the package whose non-test files import it
	Import   string // the forbidden import path
	Prefix   string // the forbidden entry it matched
}

// Matches reports the forbidden entry imp is, or is a sub-package of. It
// returns "" and false when imp matches no entry. An entry matches only at a
// path boundary: "example.com/auth" matches "example.com/auth" and
// "example.com/auth/basic", not "example.com/authoring".
func Matches(imp string, forbidden []string) (string, bool) {
	for _, p := range forbidden {
		if imp == p || strings.HasPrefix(imp, p+"/") {
			return p, true
		}
	}
	return "", false
}

// Standard reports whether imp is the import path of a standard-library
// package. It uses the go command's rule: a standard-library path has no dot
// in its first element. The rule assumes every module path has a dot there,
// which this module's path, github.com/cadasto/openehr-sdk-go, does. The cgo
// pseudo-package "C" is not the standard library, so Standard reports false
// for it.
func Standard(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return imp != "C" && !strings.Contains(first, ".")
}

// Imports returns the imports of the non-test files of the package in dir,
// sorted and without duplicates. It does not follow them: a rule that holds
// for a package's own imports but not for what they pull in is checked on
// this list.
//
// Imports returns an error when dir holds no non-test Go file that any build
// compiles, since a guard built on it would then check nothing, and when the
// package cannot be read.
func Imports(dir string) ([]string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("importguard: resolve %s: %w", dir, err)
	}
	imports, err := readImports(abs)
	if err != nil {
		return nil, startError(abs, err)
	}
	return imports, nil
}

// Scan walks the non-test imports of the package in dir and, transitively, of
// every package of this module that it reaches, and returns every import that
// Matches a forbidden entry.
//
// The module is the one whose go.mod is in dir or its nearest parent. Imports
// from outside the module, the standard library included, are checked but not
// walked. A package that is itself forbidden is reported where it is imported
// and not walked further. Each package is read once, so a package reached by
// two routes reports its violations once. Violations come in walk order, which
// is stable for a given tree.
//
// Scan returns an error when dir holds no non-test Go file that any build
// compiles, since a guard built on it would then check nothing, and when a
// package of the module that the walk reaches cannot be read.
func Scan(dir string, forbidden []string) ([]Violation, error) {
	return scan(dir, forbidden, false)
}

// ScanStd is Scan, except that it also walks the standard-library packages
// the walk reaches, read from the GOROOT the default go/build context names.
// A forbidden standard-library package pulled in through another one, such as
// net/http through expvar or net/rpc, is then found as well. The standard
// library's own vendored packages, and modules other than this one, are
// checked but not walked.
func ScanStd(dir string, forbidden []string) ([]Violation, error) {
	return scan(dir, forbidden, true)
}

// scan is Scan, and with std set, ScanStd.
func scan(dir string, forbidden []string, std bool) ([]Violation, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("importguard: resolve %s: %w", dir, err)
	}
	root, modPath, err := findModule(abs)
	if err != nil {
		return nil, err
	}
	start, err := importPath(abs, root, modPath)
	if err != nil {
		return nil, err
	}
	goroot := build.Default.GOROOT
	if std && goroot == "" {
		return nil, errors.New("importguard: GOROOT is unknown, so the standard library cannot be walked")
	}
	imports, err := readImports(abs)
	if err != nil {
		return nil, startError(abs, err)
	}

	type node struct {
		path    string
		imports []string
	}
	var violations []Violation
	seen := map[string]bool{start: true}
	queue := []node{{path: start, imports: imports}}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, imp := range n.imports {
			if p, ok := Matches(imp, forbidden); ok {
				violations = append(violations, Violation{Importer: n.path, Import: imp, Prefix: p})
				continue
			}
			if seen[imp] {
				continue
			}
			var pkgDir string
			if rel, inModule := relToModule(imp, modPath); inModule {
				pkgDir = filepath.Join(root, filepath.FromSlash(rel))
			} else if std && Standard(imp) {
				pkgDir = filepath.Join(goroot, "src", filepath.FromSlash(imp))
			} else {
				continue
			}
			seen[imp] = true
			next, err := readImports(pkgDir)
			if err != nil {
				return nil, fmt.Errorf("importguard: read %s, imported by %s: %w", imp, n.path, err)
			}
			queue = append(queue, node{path: imp, imports: next})
		}
	}
	return violations, nil
}

// errNoFiles reports a directory with no non-test Go file that counts.
var errNoFiles = errors.New("no non-test Go file that any build compiles")

// startError words an error from reading the package a guard starts at.
func startError(dir string, err error) error {
	if errors.Is(err, errNoFiles) {
		return fmt.Errorf("importguard: %s holds no non-test Go file that any build compiles; the guard would be vacuous", dir)
	}
	return fmt.Errorf("importguard: %w", err)
}

// readImports returns the imports of the files of the package in dir that
// count (see the package documentation), sorted and without duplicates. It
// returns errNoFiles when no file counts. Its other errors name the directory
// or file already, as go/build and go/parser word them.
func readImports(dir string) ([]string, error) {
	pkg, err := build.Default.ImportDir(dir, 0)
	// A directory whose every Go file this machine leaves out is a NoGoError,
	// but its files may still count, so it is read on.
	if _, noGo := errors.AsType[*build.NoGoError](err); err != nil && !noGo {
		return nil, err
	}
	// With cgo on, a cgo file is in CgoFiles and its imports are in Imports;
	// with cgo off, it is among the IgnoredGoFiles read below.
	imports := slices.Clone(pkg.Imports)
	files := len(pkg.GoFiles) + len(pkg.CgoFiles)
	fset := token.NewFileSet()
	for _, file := range pkg.IgnoredGoFiles {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		path := filepath.Join(pkg.Dir, file)
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return nil, err
		}
		expr, err := buildConstraint(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if expr != nil && !anyBuild(expr, true) {
			continue
		}
		files++
		for _, spec := range f.Imports {
			imp, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: import %s: %w", path, spec.Path.Value, err)
			}
			imports = append(imports, imp)
		}
	}
	if files == 0 {
		return nil, errNoFiles
	}
	slices.Sort(imports)
	return slices.Compact(imports), nil
}

// buildConstraint returns the expression of the //go:build line above the
// package clause of f, or nil when there is none.
func buildConstraint(f *ast.File) (constraint.Expr, error) {
	for _, group := range f.Comments {
		if group.Pos() >= f.Package {
			break
		}
		for _, c := range group.List {
			if constraint.IsGoBuild(c.Text) {
				return constraint.Parse(c.Text)
			}
		}
	}
	return nil, nil
}

// anyBuild reports whether some build may satisfy x, taking each tag except
// "ignore" as set or unset, whichever makes x true at that point; want is the
// value the caller needs x to have. Each occurrence of a tag is decided on its
// own, so this over-approximates: "linux && !linux" counts. It is the go
// command's rule for gathering every import a package could have
// (cmd/go/internal/imports, eval with the "*" tag set).
func anyBuild(x constraint.Expr, want bool) bool {
	switch x := x.(type) {
	case *constraint.TagExpr:
		return x.Tag != "ignore" && want
	case *constraint.NotExpr:
		return !anyBuild(x.X, !want)
	case *constraint.AndExpr:
		return anyBuild(x.X, want) && anyBuild(x.Y, want)
	case *constraint.OrExpr:
		return anyBuild(x.X, want) || anyBuild(x.Y, want)
	}
	// constraint.Expr has only the four kinds above. Were a new one added, the
	// file would count: reading one file too many is the stricter mistake.
	return true
}

// findModule walks up from dir to the directory holding go.mod and returns
// that directory and the module path its module line declares.
func findModule(dir string) (root, modPath string, err error) {
	for d := dir; ; {
		data, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil {
			modPath, ok := moduleLine(data)
			if !ok {
				return "", "", fmt.Errorf("importguard: %s has no module line", filepath.Join(d, "go.mod"))
			}
			return d, modPath, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("importguard: read go.mod: %w", err)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", "", fmt.Errorf("importguard: no go.mod in %s or any parent", dir)
		}
		d = parent
	}
}

// moduleLine returns the module path from the contents of a go.mod file.
func moduleLine(gomod []byte) (string, bool) {
	for line := range strings.Lines(string(gomod)) {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "module" {
			continue
		}
		if unq, err := strconv.Unquote(f[1]); err == nil {
			return unq, unq != ""
		}
		return f[1], true
	}
	return "", false
}

// importPath returns the import path of the package in dir, a directory
// inside the module rooted at root.
func importPath(dir, root, modPath string) (string, error) {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", fmt.Errorf("importguard: locate %s in module %s: %w", dir, modPath, err)
	}
	if rel == "." {
		return modPath, nil
	}
	return modPath + "/" + filepath.ToSlash(rel), nil
}

// relToModule reports whether imp is modPath or a package below it, and
// returns its slash-separated path relative to the module root.
func relToModule(imp, modPath string) (string, bool) {
	if imp == modPath {
		return ".", true
	}
	rel, ok := strings.CutPrefix(imp, modPath+"/")
	return rel, ok
}

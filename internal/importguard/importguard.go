// Package importguard finds forbidden imports in the packages a Go package
// pulls in from its own module.
//
// [Scan] reads the non-test files of one package, then follows every import
// that belongs to the same module, so a forbidden import one or more packages
// down is found as well as a direct one. It reads source files only and runs
// no subprocess.
package importguard

import (
	"errors"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
// Scan returns an error when dir holds no non-test Go files, since a guard
// built on it would then check nothing, and when a package of the module that
// the walk reaches cannot be read.
func Scan(dir string, forbidden []string) ([]Violation, error) {
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

	pkg, err := build.Default.ImportDir(abs, 0)
	_, noGo := errors.AsType[*build.NoGoError](err)
	switch {
	case noGo, err == nil && len(pkg.GoFiles) == 0:
		return nil, fmt.Errorf("importguard: no non-test Go files in %s; the guard would be vacuous", abs)
	case err != nil:
		return nil, fmt.Errorf("importguard: read %s: %w", abs, err)
	}

	type node struct {
		path string
		pkg  *build.Package
	}
	var violations []Violation
	seen := map[string]bool{start: true}
	queue := []node{{path: start, pkg: pkg}}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, imp := range n.pkg.Imports {
			if p, ok := Matches(imp, forbidden); ok {
				violations = append(violations, Violation{Importer: n.path, Import: imp, Prefix: p})
				continue
			}
			rel, inModule := relToModule(imp, modPath)
			if !inModule || seen[imp] {
				continue
			}
			seen[imp] = true
			next, err := build.Default.ImportDir(filepath.Join(root, filepath.FromSlash(rel)), 0)
			if err != nil {
				return nil, fmt.Errorf("importguard: read %s, imported by %s: %w", imp, n.path, err)
			}
			queue = append(queue, node{path: imp, pkg: next})
		}
	}
	return violations, nil
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

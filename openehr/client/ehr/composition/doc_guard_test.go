package composition_test

// doc_guard_test.go — REQ-173 § The format query parameters. The format
// query form is outside the ITS-REST pin, so the documentation of both
// format-query options must name it a deployment extension. No behaviour
// test reads a doc comment, so this parses the package's non-test source and
// holds each option's doc comment to that phrase. A line wrap between the
// two words still counts.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// formatQueryDocPhrase is the phrase each format-query option's doc comment
// must carry.
const formatQueryDocPhrase = "deployment extension"

func TestFormatQueryOptionDocsNameDeploymentExtension(t *testing.T) { // REQ-173
	docs := packageFuncDocs(t)
	for _, name := range []string{"WithFormatQuery", "WithGetFormatQuery"} {
		t.Run(name, func(t *testing.T) {
			doc, found := docs[name]
			if !found {
				t.Fatalf("no top-level func %s in package composition: the guard has stopped matching", name)
			}
			if !strings.Contains(doc, formatQueryDocPhrase) {
				t.Errorf("the doc comment of %s does not say %q; got:\n%s", name, formatQueryDocPhrase, doc)
			}
		})
	}
}

// packageFuncDocs parses the non-test Go files beside this test and returns
// the doc comment of each top-level function, its whitespace collapsed to
// single spaces.
func packageFuncDocs(t *testing.T) map[string]string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(self)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	docs := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Recv != nil {
				continue
			}
			docs[fn.Name.Name] = strings.Join(strings.Fields(fn.Doc.Text()), " ")
		}
	}
	return docs
}

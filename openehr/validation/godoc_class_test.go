package validation_test

// godoc_class_test.go: REQ-168. clinical-modeling.md § Field classes rules
// that the godoc of Issue states the class of each of its fields, value-free
// or value-bearing, so a consumer reads it on the type it holds. Each exported
// field's doc carries one paragraph that opens with its class, as the fields
// of lint.Issue do. Path and Detail are value-free on issues from the instance
// validators and value-bearing on issues from ValidateAQL, so their paragraph
// opens with one class and names the other. This reads the package's non-test
// Go files and holds each paragraph to the class the table below gives, so
// deleting it, changing it, or adding an exported field without one fails.
import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The two class words a class paragraph opens with.
const (
	valueFree    = "Value-free"
	valueBearing = "Value-bearing"
)

// fieldClass is what the class paragraph of a field's doc must say: the class
// word it opens with and, for a field whose class depends on the entry point,
// the other class word it must also name.
type fieldClass struct {
	opens string
	names string
}

// issueFieldClasses is the class of every exported field of Issue. Path and
// Detail open with the class they have on issues from the instance
// validators and name the one they have on issues from ValidateAQL.
var issueFieldClasses = map[string]fieldClass{
	"Path":     {opens: valueFree, names: valueBearing},
	"Code":     {opens: valueFree},
	"Detail":   {opens: valueFree, names: valueBearing},
	"Severity": {opens: valueFree},
	"Value":    {opens: valueBearing},
}

func TestREQ168_GodocStatesEachFieldClass(t *testing.T) {
	t.Parallel()
	// go test runs in the package's source directory.
	problems, err := fieldClassProblems(".", "Issue", issueFieldClasses)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Errorf("%s: the godoc MUST state each field's class (clinical-modeling.md § Field classes, REQ-168)", p)
	}
}

// TestREQ168_FieldClassCheckFailsOnEachGap runs the check over one-file
// packages, so the test above is shown to fail on each gap it exists to catch,
// and to pass a struct whose fields all state their class.
func TestREQ168_FieldClassCheckFailsOnEachGap(t *testing.T) {
	t.Parallel()
	const header = "package p\n\n// T is a diagnostic.\ntype T struct {\n"
	twoFields := map[string]fieldClass{"A": {opens: valueFree}, "B": {opens: valueBearing}}
	tests := []struct {
		name        string
		file        string // defaults to "t.go"
		src         string
		want        map[string]fieldClass
		wantProblem string // a substring of the one problem expected, or "" for none
		wantErr     string // a substring of the error expected, or "" for none
	}{
		{
			name: "every field states its class",
			src:  header + "\t// A is a code.\n\t//\n\t// Value-free: never holds the value.\n\tA string\n\n\t// B is the value.\n\t//\n\t// Value-bearing: holds the value.\n\tB any\n\n\tc int\n}\n",
			want: twoFields,
		},
		{
			name:        "field whose doc has no class paragraph",
			src:         header + "\t// A is a code.\n\tA string\n\n\t// B is the value.\n\t//\n\t// Value-bearing: holds the value.\n\tB any\n}\n",
			want:        twoFields,
			wantProblem: "T.A: its doc has no paragraph opening with",
		},
		{
			name:        "field without a doc comment",
			src:         header + "\tA string\n\n\t// B is the value.\n\t//\n\t// Value-bearing: holds the value.\n\tB any\n}\n",
			want:        twoFields,
			wantProblem: "T.A has no doc comment",
		},
		{
			name:        "class word inside a paragraph rather than opening it",
			src:         header + "\t// A is a code. Value-free: never holds the value.\n\tA string\n\n\t// B is the value.\n\t//\n\t// Value-bearing: holds the value.\n\tB any\n}\n",
			want:        twoFields,
			wantProblem: "T.A: its doc has no paragraph opening with",
		},
		{
			name:        "wrong class",
			src:         header + "\t// A is a code.\n\t//\n\t// Value-bearing: holds the value.\n\tA string\n\n\t// B is the value.\n\t//\n\t// Value-bearing: holds the value.\n\tB any\n}\n",
			want:        twoFields,
			wantProblem: `T.A: its class paragraph "Value-bearing: holds the value." opens with the wrong class, want "Value-free"`,
		},
		{
			name:        "two class paragraphs",
			src:         header + "\t// A is a code.\n\t//\n\t// Value-free: never holds the value.\n\t//\n\t// Value-bearing: holds the value.\n\tA string\n\n\t// B is the value.\n\t//\n\t// Value-bearing: holds the value.\n\tB any\n}\n",
			want:        twoFields,
			wantProblem: "T.A: its doc has 2 class paragraphs, want one",
		},
		{
			name:        "field whose class depends on the entry point names only one class",
			src:         header + "\t// A is a path.\n\t//\n\t// Value-free: never holds the value.\n\tA string\n}\n",
			want:        map[string]fieldClass{"A": {opens: valueFree, names: valueBearing}},
			wantProblem: `does not name "Value-bearing"`,
		},
		{
			name: "field whose class depends on the entry point names both classes",
			src:  header + "\t// A is a path.\n\t//\n\t// Value-free on issues from one entry point. Value-bearing on\n\t// issues from the other.\n\tA string\n}\n",
			want: map[string]fieldClass{"A": {opens: valueFree, names: valueBearing}},
		},
		{
			name:        "exported field the table does not list",
			src:         header + "\t// A is a code.\n\t//\n\t// Value-free: never holds the value.\n\tA string\n\n\t// B is the value.\n\t//\n\t// Value-bearing: holds the value.\n\tB any\n\n\t// C is new.\n\t//\n\t// Value-free: never holds the value.\n\tC string\n}\n",
			want:        twoFields,
			wantProblem: "T.C is exported but the table gives it no class",
		},
		{
			name:        "exported field among several names on one line",
			src:         header + "\t// A is a code.\n\t//\n\t// Value-free: never holds the value.\n\tA, C string\n\n\t// B is the value.\n\t//\n\t// Value-bearing: holds the value.\n\tB any\n}\n",
			want:        twoFields,
			wantProblem: "T.C is exported but the table gives it no class",
		},
		{
			name:        "embedded exported type the table does not list",
			src:         header + "\t// A is a code.\n\t//\n\t// Value-free: never holds the value.\n\tA string\n\n\t// B is the value.\n\t//\n\t// Value-bearing: holds the value.\n\tB any\n\n\t*strings.Builder\n}\n",
			want:        twoFields,
			wantProblem: "T.Builder is exported but the table gives it no class",
		},
		{
			name:        "table lists a field the struct does not have",
			src:         header + "\t// A is a code.\n\t//\n\t// Value-free: never holds the value.\n\tA string\n}\n",
			want:        twoFields,
			wantProblem: "the table lists T.B, which is not an exported field of T",
		},
		{
			name:    "struct with no fields",
			src:     "package p\n\n// T is empty.\ntype T struct{}\n",
			want:    twoFields,
			wantErr: "has no fields",
		},
		{
			name:    "type that is not a struct",
			src:     "package p\n\n// T is a name.\ntype T string\n",
			want:    twoFields,
			wantErr: "is not a struct type",
		},
		{
			name:    "type declared only in a test file",
			file:    "t_test.go",
			src:     header + "\t// A is a code.\n\t//\n\t// Value-free: never holds the value.\n\tA string\n}\n",
			want:    twoFields,
			wantErr: "found no type T",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file := tc.file
			if file == "" {
				file = "t.go"
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, file), []byte(tc.src), 0o600); err != nil {
				t.Fatal(err)
			}
			problems, err := fieldClassProblems(dir, "T", tc.want)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("fieldClassProblems(%q) error = %v, want none", tc.src, err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("fieldClassProblems(%q) error = %v, want one naming %q", tc.src, err, tc.wantErr)
			}
			switch {
			case tc.wantProblem == "" && len(problems) != 0:
				t.Errorf("fieldClassProblems(%q) = %q, want no problem", tc.src, problems)
			case tc.wantProblem != "" && (len(problems) != 1 || !strings.Contains(problems[0], tc.wantProblem)):
				t.Errorf("fieldClassProblems(%q) = %q, want one problem naming %q", tc.src, problems, tc.wantProblem)
			}
		})
	}
}

// fieldClassProblems parses the non-test Go files in dir, finds the struct
// type named typeName, and returns one line for each exported field whose doc
// does not state the class that want gives it, for each exported field that
// want does not list, and for each field in want that the struct lacks. It
// returns an error when it finds no such struct, or one with no fields, so
// the check cannot pass over nothing.
func fieldClassProblems(dir, typeName string, want map[string]fieldClass) ([]string, error) {
	st, err := findStruct(dir, typeName)
	if err != nil {
		return nil, err
	}
	if len(st.Fields.List) == 0 {
		return nil, fmt.Errorf("struct %s has no fields, so the check has gone blind", typeName)
	}
	var problems []string
	seen := map[string]bool{}
	for _, f := range st.Fields.List {
		for _, name := range fieldNames(f) {
			if !ast.IsExported(name) {
				continue
			}
			seen[name] = true
			class, ok := want[name]
			if !ok {
				problems = append(problems, fmt.Sprintf("%s.%s is exported but the table gives it no class; add it with the class its godoc states", typeName, name))
				continue
			}
			if p := classProblem(f.Doc, class); p != "" {
				problems = append(problems, typeName+"."+name+p)
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(want)) {
		if !seen[name] {
			problems = append(problems, fmt.Sprintf("the table lists %s.%s, which is not an exported field of %s", typeName, name, typeName))
		}
	}
	return problems, nil
}

// findStruct returns the struct type named typeName that the non-test Go
// files in dir declare.
func findStruct(dir, typeName string) (*ast.StructType, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name.Name != typeName {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					return nil, fmt.Errorf("%s in %s is not a struct type", typeName, name)
				}
				return st, nil
			}
		}
	}
	return nil, errors.New("found no type " + typeName + " in the non-test Go files, so the check has gone blind")
}

// fieldNames returns the names a struct field declares: its own names, or,
// for an embedded field, the name of its type.
func fieldNames(f *ast.Field) []string {
	if len(f.Names) > 0 {
		names := make([]string, 0, len(f.Names))
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
		return names
	}
	t := f.Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
		case *ast.IndexExpr:
			t = x.X
		case *ast.IndexListExpr:
			t = x.X
		case *ast.SelectorExpr:
			return []string{x.Sel.Name}
		case *ast.Ident:
			return []string{x.Name}
		default:
			return nil
		}
	}
}

// classProblem returns what is wrong with the class paragraph of a field's
// doc, starting with the text that follows the field's name, or "" when the
// doc states class: exactly one paragraph opens with a class word, that word
// is class.opens, and the paragraph also names class.names.
func classProblem(doc *ast.CommentGroup, class fieldClass) string {
	if doc == nil {
		return fmt.Sprintf(" has no doc comment, so it states no class; want a paragraph opening with %q", class.opens)
	}
	var found []string
	for p := range strings.SplitSeq(doc.Text(), "\n\n") {
		p = strings.TrimSpace(p)
		if opensWith(p, valueFree) || opensWith(p, valueBearing) {
			found = append(found, p)
		}
	}
	switch {
	case len(found) == 0:
		return fmt.Sprintf(": its doc has no paragraph opening with %q or %q, so it states no class; want one opening with %q", valueFree, valueBearing, class.opens)
	case len(found) > 1:
		return fmt.Sprintf(": its doc has %d class paragraphs, want one", len(found))
	case !opensWith(found[0], class.opens):
		return fmt.Sprintf(": its class paragraph %q opens with the wrong class, want %q", found[0], class.opens)
	case class.names != "" && !strings.Contains(found[0], class.names):
		return fmt.Sprintf(": its class paragraph %q does not name %q; a field whose class depends on the entry point names both classes", found[0], class.names)
	}
	return ""
}

// opensWith reports whether paragraph p opens with the class word w, followed
// by a colon or a space.
func opensWith(p, w string) bool {
	rest, ok := strings.CutPrefix(p, w)
	return ok && (strings.HasPrefix(rest, ":") || strings.HasPrefix(rest, " "))
}

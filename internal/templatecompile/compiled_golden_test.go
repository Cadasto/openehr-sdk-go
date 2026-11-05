package templatecompile_test

// REQ-111 — the compiled-tree golden, the parity check ADR 0035 names.
//
// For every operational template the corpus vendors, walked as PROBE-111
// walks them, for every parity fixture (testdata/parity) and for every
// fixture of the model's reader (openehr/aom/aom14/testdata/reader), the
// test renders the compiled tree as line-oriented text and holds it equal
// to testdata/compiled/<corpus path>.txt, parity/<fixture>.txt or
// reader/<fixture>.txt. Each node is one block in walk order: its path, RM
// type, node id, archetype id, node name, occurrences, slot marks and
// rules, primitive constraint, default value, terms and bindings, then one
// line per attribute. A primitive constraint renders as its Go type and
// exported fields, a default value as its Go type and canonical JSON. A
// change to the compile step that moves any of it shows as a diff on the
// golden, reviewed rather than assumed. Regenerate with:
//
//	go test ./internal/templatecompile/ -run TestCompiledGolden -update

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templatecompile/walk"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	templateprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/template"
)

const compiledGoldenDir = "testdata/compiled"

// readerFixturesRoot holds the folder of the model reader's fixtures.
const readerFixturesRoot = "../../openehr/aom/aom14/testdata"

// goldenSource is a folder of operational templates TestCompiledGolden
// holds to goldens. Each name is a slash-separated path below root, whose
// golden is that path below compiledGoldenDir with .txt for .opt; label
// stands for root in the text of a refusal.
type goldenSource struct {
	root, label string
	names       []string
}

func TestCompiledGolden(t *testing.T) {
	names, err := templateprobes.Probe111OperationalTemplates()
	if err != nil {
		t.Fatalf("Probe111OperationalTemplates: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("Probe111OperationalTemplates found no operational template: corpus layout changed?")
	}
	// The goldens of the parity fixtures and of the reader's fixtures were
	// recorded on the v0.45.0 code, in the commit that added them, while
	// TestREQ111_ParityWithDeprecatedParser held each of these fixtures, and
	// each corpus template, to the deprecated parser's reading; that run left
	// the corpus goldens unchanged. Once the parser is gone they stand for its
	// reading, so a diff on them changes what the compile step makes of a
	// template the parser read: regenerate them only for a reason the change
	// under review gives.
	sources := []goldenSource{
		{root: fixtures.CorpusRoot(), label: "<corpus>", names: names},
		fixtureGoldenSource(t, "testdata", "parity"),
		fixtureGoldenSource(t, readerFixturesRoot, "reader"),
	}

	expected := map[string]bool{}
	for _, src := range sources {
		for _, name := range src.names {
			golden := filepath.Join(compiledGoldenDir, filepath.FromSlash(strings.TrimSuffix(name, ".opt")+".txt"))
			if expected[golden] {
				t.Fatalf("%s is the golden of two operational templates named %s", golden, name)
			}
			expected[golden] = true
			t.Run(name, func(t *testing.T) {
				got := dumpCompiledTemplate(src.root, src.label, name)
				if *updateSnapshot {
					if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(golden, got, 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatalf("read golden (run with -update): %v", err)
				}
				if line, g, w, differ := firstDifference(got, want); differ {
					t.Errorf("compiled tree of %s differs from %s at line %d:\n got: %s\nwant: %s\n"+
						"Regenerate with: go test ./internal/templatecompile/ -run TestCompiledGolden -update",
						name, golden, line, g, w)
				}
			})
		}
	}

	// A golden left behind by a template no source holds any more is
	// removed by -update and refused otherwise.
	err = filepath.WalkDir(compiledGoldenDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || expected[path] {
			return nil
		}
		if *updateSnapshot {
			return os.Remove(path)
		}
		t.Errorf("%s matches no operational template the test walks; regenerate with -update", path)
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("walk %s: %v", compiledGoldenDir, err)
	}
}

// fixtureGoldenSource returns the operational templates of the folder dir
// below root, each named dir/<file>. A *.read.opt file is the reading a
// reader fixture is compared with, not a template to compile, and is left
// out. A folder that holds no template stops the test, so that a fixture
// folder that moves or empties cannot pass unseen.
func fixtureGoldenSource(t *testing.T, root, dir string) goldenSource {
	t.Helper()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(abs, dir, "*.opt"))
	if err != nil {
		t.Fatal(err)
	}
	src := goldenSource{root: abs, label: "<" + filepath.ToSlash(root) + ">"}
	for _, f := range files {
		if strings.HasSuffix(f, ".read.opt") {
			continue
		}
		src.names = append(src.names, dir+"/"+filepath.Base(f))
	}
	if len(src.names) == 0 {
		t.Fatalf("%s holds no operational template", filepath.Join(root, dir))
	}
	return src
}

// firstDifference reports the first line, counted from 1, at which got and
// want differ, with both lines; a missing line reads as "<end of file>".
func firstDifference(got, want []byte) (line int, g, w string, differ bool) {
	if bytes.Equal(got, want) {
		return 0, "", "", false
	}
	gl := strings.Split(string(got), "\n")
	wl := strings.Split(string(want), "\n")
	for i := range max(len(gl), len(wl)) {
		g, w = "<end of file>", "<end of file>"
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			return i + 1, g, w, true
		}
	}
	return 0, "", "", false
}

// dumpCompiledTemplate reads the operational template name, a
// slash-separated path below root, into the model, compiles it and renders
// the result. A template that does not read or compile renders as that
// error, label standing for root in it, so a change in which templates
// compile shows on the golden too.
func dumpCompiledTemplate(root, label, name string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# compiled tree of %s\n", name)
	model, err := readModelFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		fmt.Fprintf(&b, "!! does not read: %s\n", strings.ReplaceAll(err.Error(), root, label))
		return b.Bytes()
	}
	c, err := templatecompile.Compile(model)
	if err != nil {
		fmt.Fprintf(&b, "!! does not compile: %s\n", strings.ReplaceAll(err.Error(), root, label))
		return b.Bytes()
	}
	lang := c.Language()
	fmt.Fprintf(&b, "template_id=%q concept=%q language=%q uid=%q num_nodes=%d\n",
		c.TemplateID(), c.Concept(), lang, c.UID(), c.NumNodes())
	for _, tb := range c.TermBindings() {
		writeBinding(&b, "template_binding", tb)
	}

	var nodes int
	err = walk.Walk(c, walk.VisitorFunc{Pre: func(ctx *walk.Context) error {
		nodes++
		writeNode(&b, c, ctx.Node(), lang)
		return nil
	}})
	if err != nil {
		fmt.Fprintf(&b, "!! walk failed: %v\n", err)
	}
	fmt.Fprintf(&b, "\n# walked nodes=%d\n", nodes)
	return b.Bytes()
}

// writeNode renders one compiled node and its attributes as a block.
func writeNode(b *bytes.Buffer, c *templatecompile.Compiled, n *templatecompile.CompiledNode, lang string) {
	fmt.Fprintf(b, "\nnode %s\n", n.AQLPath())
	fmt.Fprintf(b, "  rm_type=%q node_id=%q archetype_id=%q node_name=%q occurrences=%s slot=%t\n",
		n.RMTypeName(), n.NodeID(), n.ArchetypeID(), n.NodeName(), multiplicity(n.Occurrences()), n.IsSlot())

	includes, excludes, rules := n.SlotIncludes(), n.SlotExcludes(), n.SlotRules()
	if n.IsSlot() || len(includes) > 0 || len(excludes) > 0 || !isZeroSlotRules(rules) {
		fmt.Fprintf(b, "  slot_includes=%s slot_excludes=%s\n", quoteAll(includes), quoteAll(excludes))
		fmt.Fprintf(b, "  slot_rules rm_type=%q includes=%s excludes=%s raw_include_count=%d\n",
			rules.RMTypeName, assertionPatterns(rules.Includes), assertionPatterns(rules.Excludes), rules.RawIncludeCount)
	}

	pc := n.PrimitiveConstraint()
	if pc != nil {
		fmt.Fprintf(b, "  primitive=%s\n", renderConstraint(pc))
	}
	if dv := n.DefaultValue(); dv != nil {
		fmt.Fprintf(b, "  default_value=%T %s\n", dv, renderDefaultValue(dv))
	}

	if id := n.NodeID(); id != "" {
		writeTerm(b, n, "term", id, lang)
		for _, tb := range c.TermBindingsForNode(id) {
			writeBinding(b, "  binding", tb)
		}
	}
	for _, code := range localCodes(pc) {
		writeTerm(b, n, "code_term", code, lang)
	}

	for _, a := range n.Attributes() {
		fmt.Fprintf(b, "  attr %q rm_type=%q cardinality=%s existence=%s child_multiplicity=%s implicit=%t not_in_rm=%t required=%t children=%d\n",
			a.Name(), a.RMTypeName(), a.Cardinality(), multiplicity(a.Existence()), multiplicity(a.ChildMultiplicity()),
			a.Implicit(), a.NotInRM(), a.Required(), len(a.Children()))
	}
}

// writeTerm renders the term n resolves for code, its items sorted by key.
func writeTerm(b *bytes.Buffer, n *templatecompile.CompiledNode, label, code, lang string) {
	term, ok := n.Term(code, lang)
	if !ok {
		fmt.Fprintf(b, "  %s[%s] %q none\n", label, lang, code)
		return
	}
	fmt.Fprintf(b, "  %s[%s] %q code=%q", label, lang, code, term.Code)
	for _, k := range slices.Sorted(maps.Keys(term.Items)) {
		fmt.Fprintf(b, " %s=%q", k, term.Items[k])
	}
	b.WriteByte('\n')
}

func writeBinding(b *bytes.Buffer, label string, tb template.TermBinding) {
	fmt.Fprintf(b, "%s terminology=%q node_or_path=%q target_terminology=%q target_code=%q\n",
		label, tb.Terminology, tb.NodeOrPath, tb.Target.TerminologyID, tb.Target.CodeString)
}

// renderDefaultValue renders a default value as its canonical JSON, or as
// the error that refuses to encode it.
func renderDefaultValue(dv rm.DataValue) string {
	j, err := canjson.Marshal(dv)
	if err != nil {
		return "!! " + err.Error()
	}
	return string(j)
}

// localCodes returns the local at-codes a coded constraint lists, whose
// terms the compiled tree resolves on the node's archetype root.
func localCodes(pc constraints.PrimitiveConstraint) []string {
	var codes []string
	if p, ok := constraints.CodePhraseOf(pc); ok {
		if p.Terminology == "local" {
			codes = append(codes, p.CodeList...)
		}
		return codes
	}
	switch p := pc.(type) {
	case constraints.CDvOrdinal:
		for _, v := range p.Values {
			if v.Symbol.Terminology == "local" {
				codes = append(codes, v.Symbol.CodeString)
			}
		}
	}
	return codes
}

func multiplicity(m *template.Multiplicity) string {
	if m == nil {
		return "none"
	}
	lower, upper := "*", "*"
	if !m.LowerUnbounded() {
		lower = strconv.Itoa(m.Lower())
	}
	if !m.UpperUnbounded() {
		upper = strconv.Itoa(m.Upper())
	}
	return lower + ".." + upper
}

func isZeroSlotRules(r constraints.SlotRules) bool {
	return r.RMTypeName == "" && len(r.Includes) == 0 && len(r.Excludes) == 0 && r.RawIncludeCount == 0
}

func assertionPatterns(as []constraints.SlotAssertion) string {
	patterns := make([]string, len(as))
	for i, a := range as {
		patterns[i] = a.Pattern()
	}
	return quoteAll(patterns)
}

func quoteAll(ss []string) string {
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = strconv.Quote(s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// renderConstraint renders a primitive constraint as its Go type and its
// exported fields, recursively: pointers by what they point to, a nil one
// as nil, slices in order, maps sorted by key, floats in their shortest
// exact form. Unexported fields, such as a compiled pattern, are derived
// from exported ones and left out.
func renderConstraint(pc constraints.PrimitiveConstraint) string {
	var b strings.Builder
	renderValue(&b, reflect.ValueOf(pc))
	return b.String()
}

func renderValue(b *strings.Builder, v reflect.Value) {
	switch v.Kind() {
	case reflect.Invalid:
		b.WriteString("nil")
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			b.WriteString("nil")
			return
		}
		renderValue(b, v.Elem())
	case reflect.Struct:
		b.WriteString(v.Type().String())
		b.WriteByte('{')
		sep := ""
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			b.WriteString(sep)
			b.WriteString(f.Name)
			b.WriteByte(':')
			renderValue(b, v.Field(i))
			sep = " "
		}
		b.WriteByte('}')
	case reflect.Slice, reflect.Array:
		b.WriteByte('[')
		for i := range v.Len() {
			if i > 0 {
				b.WriteByte(' ')
			}
			renderValue(b, v.Index(i))
		}
		b.WriteByte(']')
	case reflect.Map:
		entries := make([]string, 0, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			var e strings.Builder
			renderValue(&e, iter.Key())
			e.WriteByte(':')
			renderValue(&e, iter.Value())
			entries = append(entries, e.String())
		}
		slices.Sort(entries)
		b.WriteString("map[")
		b.WriteString(strings.Join(entries, " "))
		b.WriteByte(']')
	case reflect.String:
		b.WriteString(strconv.Quote(v.String()))
	case reflect.Bool:
		b.WriteString(strconv.FormatBool(v.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.WriteString(strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		b.WriteString(strconv.FormatUint(v.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		b.WriteString(strconv.FormatFloat(v.Float(), 'g', -1, v.Type().Bits()))
	default:
		fmt.Fprintf(b, "<%s>", v.Kind())
	}
}

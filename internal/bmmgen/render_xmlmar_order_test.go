package bmmgen

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// emittedXMLChildren parses every *_xmlmar_gen.go file in dir and returns,
// per BMM class, the child element names its MarshalXML writes, in source
// order. The class is read from the method's canxml.ElementName default.
func emittedXMLChildren(t *testing.T, dir string) map[string][]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*_xmlmar_gen.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no XML encoder files in %s (err %v)", dir, err)
	}
	out := make(map[string][]string)
	fset := token.NewFileSet()
	for _, file := range files {
		f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "MarshalXML" {
				continue
			}
			class, children := marshalXMLChildren(fn.Body)
			if class == "" {
				t.Fatalf("%s: a MarshalXML without a canxml.ElementName default", file)
			}
			out[class] = children
		}
	}
	return out
}

// marshalXMLChildren reads one generated MarshalXML body: the BMM name its
// canxml.ElementName call carries, and the element name of each
// EncodeElement and canxml.EncodePoly call, in source order.
func marshalXMLChildren(body *ast.BlockStmt) (class string, children []string) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "ElementName":
			class = stringLit(call.Args[0])
		case "EncodePoly":
			children = append(children, stringLit(call.Args[1]))
		case "EncodeElement":
			ast.Inspect(call.Args[1], func(m ast.Node) bool {
				kv, ok := m.(*ast.KeyValueExpr)
				if !ok {
					return true
				}
				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Local" {
					children = append(children, stringLit(kv.Value))
				}
				return true
			})
		}
		return true
	})
	return class, children
}

// stringLit returns the value of a string literal, or "" for anything else.
func stringLit(e ast.Expr) string {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return s
}

// nearestFirst returns class and every class it descends from, nearest
// first: its parents in declaration order, then theirs.
func nearestFirst(s *bmm.Schema, class string) []string {
	out := []string{class}
	for i := 0; i < len(out); i++ {
		c, ok := schemaClass(s, out[i])
		if !ok {
			continue
		}
		for _, anc := range c.Ancestors() {
			anc, _, _ = strings.Cut(anc, "<")
			if !slices.Contains(out, anc) {
				out = append(out, anc)
			}
		}
	}
	return out
}

// schemaOrderProblems states, for one class, how the children its encoder
// writes break the order the schema sets give, or nil when they keep it.
// sets is the lookup order: the class takes the schema type of its schema
// name (Target.XMLTypeNames) when it has one, else of its own name, or of its
// nearest ancestor, from the first set that has one. From that type's
// sequence (own):
//
//   - the children own declares come in own's order;
//   - a child own does not declare but another set's type of the same name
//     does sits right after the nearest earlier element both declare in that
//     other sequence, so it follows every element up to that one and
//     precedes every later one;
//   - every other child comes after all of those, in the generator's order.
func schemaOrderProblems(plan *Plan, sets []*xsdSet, class string, children []string, fields []emittedField) ([]string, error) {
	var own, other []string
	found := false
	lineage := nearestFirst(plan.Schema, class)
	if schema, ok := plan.Target.XMLTypeNames[class]; ok {
		lineage = append([]string{schema}, lineage...)
	}
	for i, set := range sets {
		for _, name := range lineage {
			if !set.has(name) {
				continue
			}
			seq, err := set.sequence(name)
			if err != nil {
				return nil, err
			}
			own = seq
			for j, o := range sets {
				if j != i && o.has(name) {
					if other, err = o.sequence(name); err != nil {
						return nil, err
					}
				}
			}
			found = true
			break
		}
		if found {
			break
		}
	}

	var problems []string
	ownIdx := func(name string) int { return slices.Index(own, name) }
	otherOnly := func(name string) bool { return ownIdx(name) < 0 && slices.Contains(other, name) }
	declared := func(name string) bool { return ownIdx(name) >= 0 || otherOnly(name) }

	// Declared children in own's order.
	last := -1
	for _, c := range children {
		if i := ownIdx(c); i >= 0 {
			if i < last {
				problems = append(problems, fmt.Sprintf("%s comes after %s, against the schema sequence", c, own[last]))
			}
			last = max(last, i)
		}
	}
	// Undeclared children last, in the generator's order.
	firstUndeclared := slices.IndexFunc(children, func(c string) bool { return !declared(c) })
	if firstUndeclared >= 0 {
		for _, c := range children[firstUndeclared:] {
			if declared(c) {
				problems = append(problems, fmt.Sprintf("%s, which the schema declares, follows %s, which it does not", c, children[firstUndeclared]))
			}
		}
	}
	var undeclared, generatorOrder []string
	for _, c := range children {
		if !declared(c) {
			undeclared = append(undeclared, c)
		}
	}
	for _, f := range fields {
		if name := f.Prop.PropertyName(); slices.Contains(undeclared, name) {
			generatorOrder = append(generatorOrder, name)
		}
	}
	if !slices.Equal(undeclared, generatorOrder) {
		problems = append(problems, fmt.Sprintf("undeclared children %q are not in the generator's order %q", undeclared, generatorOrder))
	}
	// A child only the other set declares sits right after its anchor.
	anchorOf := func(name string) int {
		for k := slices.Index(other, name) - 1; k >= 0; k-- {
			if i := ownIdx(other[k]); i >= 0 {
				return i
			}
		}
		return -1
	}
	for xi, x := range children {
		if !otherOnly(x) {
			continue
		}
		anchor := anchorOf(x)
		for yi, y := range children {
			switch i := ownIdx(y); {
			case i >= 0 && i <= anchor && yi > xi:
				problems = append(problems, fmt.Sprintf("%s, declared only by the other schema set, precedes %s", x, y))
			case i > anchor && yi < xi:
				problems = append(problems, fmt.Sprintf("%s, declared only by the other schema set, follows %s", x, y))
			case i < 0 && otherOnly(y) && anchorOf(y) == anchor && yi < xi && slices.Index(other, y) > slices.Index(other, x):
				problems = append(problems, fmt.Sprintf("%s and %s are not in the other schema set's order", y, x))
			}
		}
	}
	return problems, nil
}

// TestXMLEncodersFollowSchemaOrder runs the generator over both targets and
// holds every XML encoder it emits to the vendored ITS-XML schemas: an RM
// class looked up in the RM and BASE schemas, an AOM 1.4 class in the AM
// schemas first, each by its schema name, its own name or its nearest
// ancestor's, and a property only the other set declares placed where that
// set puts it.
//
// REQ-056: the encoder writes a class's children in the order of its ITS-XML
// schema sequence.
func TestXMLEncodersFollowSchemaOrder(t *testing.T) {
	out := t.TempDir()
	if _, err := Run(Options{ResourcesDir: testResources, OutDir: out, SkipInventory: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	rmSet, amSet := loadTestXSDSets(t)

	checked := map[string]bool{}
	var bad []string
	for _, tc := range []struct {
		target Target
		sets   []*xsdSet
	}{
		{TargetRM, []*xsdSet{rmSet, amSet}},
		{TargetAOM14, []*xsdSet{amSet, rmSet}},
	} {
		plan, err := BuildPlanForTarget(context.Background(), tc.target, bmm.FSResolver{Root: testResources})
		if err != nil {
			t.Fatalf("BuildPlanForTarget(%s): %v", tc.target.GoPackage, err)
		}
		encoders := emittedXMLChildren(t, filepath.Join(out, filepath.FromSlash(tc.target.OutSubDir)))
		for _, class := range slices.Sorted(maps.Keys(encoders)) {
			pc, ok := plan.Classes[class]
			if !ok {
				t.Fatalf("%s: encoder for %s, which the plan does not hold", tc.target.GoPackage, class)
			}
			fields, err := effectiveFields(plan, pc)
			if err != nil {
				t.Fatalf("effectiveFields(%s): %v", class, err)
			}
			problems, err := schemaOrderProblems(plan, tc.sets, class, encoders[class], fields)
			if err != nil {
				t.Fatalf("%s: %v", class, err)
			}
			checked[tc.target.GoPackage+"."+class] = true
			if len(problems) > 0 {
				bad = append(bad, fmt.Sprintf("%s.%s writes %q:\n\t\t%s",
					tc.target.GoPackage, class, encoders[class], strings.Join(problems, "\n\t\t")))
			}
		}
	}
	for _, want := range []string{"rm.ORIGINAL_VERSION", "rm.OBSERVATION", "rm.RESOURCE_DESCRIPTION_ITEM", "rm.Interval", "aom14.ARCHETYPE"} {
		if !checked[want] {
			t.Errorf("no XML encoder checked for %s", want)
		}
	}
	if len(bad) > 0 {
		t.Errorf("%d of %d XML encoders leave the ITS-XML schema order:\n\t%s",
			len(bad), len(checked), strings.Join(bad, "\n\t"))
	}
}

// TestSchemaOrderProblemsCatches pins the checker on hand-made orders, so a
// pass of TestXMLEncodersFollowSchemaOrder means the order holds, not that
// the checker looked away.
func TestSchemaOrderProblemsCatches(t *testing.T) {
	rmSet, amSet := loadTestXSDSets(t)
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(rm): %v", err)
	}
	sets := []*xsdSet{rmSet, amSet}
	fieldsOf := func(class string) []emittedField {
		fields, err := effectiveFields(plan, plan.Classes[class])
		if err != nil {
			t.Fatalf("effectiveFields(%s): %v", class, err)
		}
		return fields
	}
	cases := []struct {
		name     string
		class    string
		children []string
		wantBad  bool
	}{
		{
			"original version in schema order", "ORIGINAL_VERSION",
			[]string{"contribution", "commit_audit", "signature", "uid", "data", "lifecycle_state"},
			false,
		},
		{
			"original version with signature before commit_audit", "ORIGINAL_VERSION",
			[]string{"contribution", "signature", "commit_audit", "uid", "lifecycle_state"},
			true,
		},
		{
			"copyright right after misuse", "RESOURCE_DESCRIPTION_ITEM",
			[]string{"language", "purpose", "use", "misuse", "copyright", "original_resource_uri"},
			false,
		},
		{
			"copyright before misuse", "RESOURCE_DESCRIPTION_ITEM",
			[]string{"language", "purpose", "use", "copyright", "misuse", "original_resource_uri"},
			true,
		},
		{
			"copyright after original_resource_uri", "RESOURCE_DESCRIPTION_ITEM",
			[]string{"language", "purpose", "misuse", "original_resource_uri", "copyright"},
			true,
		},
		{
			"interval bounds after the flags", "Interval",
			[]string{"lower_included", "upper_included", "lower_unbounded", "upper_unbounded", "lower", "upper"},
			false,
		},
		{
			"interval bound before a flag", "Interval",
			[]string{"lower_included", "upper_included", "lower", "lower_unbounded", "upper_unbounded", "upper"},
			true,
		},
		{
			"interval bounds out of the generator's order", "Interval",
			[]string{"lower_included", "upper_included", "lower_unbounded", "upper_unbounded", "upper", "lower"},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems, err := schemaOrderProblems(plan, sets, tc.class, tc.children, fieldsOf(tc.class))
			if err != nil {
				t.Fatalf("schemaOrderProblems: %v", err)
			}
			if got := len(problems) > 0; got != tc.wantBad {
				t.Errorf("schemaOrderProblems(%s, %q) = %q, want problems: %v", tc.class, tc.children, problems, tc.wantBad)
			}
		})
	}
}

package template_test

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-100 § Strict parse mode: both modes read xsi:type="T_ARCHETYPE_ROOT"
// as C_ARCHETYPE_ROOT, with its archetype id, term definitions and
// subtree. Any other unknown type keeps the leaf / ErrUnsupportedNode
// behaviour, so only the named alias is recognised.
func TestREQ100_TArchetypeRootAlias(t *testing.T) {
	t.Run("social.opt rewritten to T_ARCHETYPE_ROOT parses to the same tree", func(t *testing.T) {
		original, rewritten := socialWithTArchetypeRoot(t)

		want, err := template.ParseOPT(bytes.NewReader(original))
		if err != nil {
			t.Fatalf("ParseOPT(social.opt as vendored): %v", err)
		}
		// The vendored definition is an archetype root, and so are its
		// seven C_ARCHETYPE_ROOT children. Without this floor the walk
		// below could compare two trees that both lost their roots.
		if got, wantRoots := countArchetypeRoots(want.Root()), 8; got != wantRoots {
			t.Fatalf("ParseOPT(social.opt as vendored): %d archetype roots, want %d", got, wantRoots)
		}

		lenient, err := template.ParseOPT(bytes.NewReader(rewritten))
		if err != nil {
			t.Fatalf("ParseOPT(social.opt with T_ARCHETYPE_ROOT): %v", err)
		}
		if diffs := diffNodes("", want.Root(), lenient.Root(), nil); len(diffs) > 0 {
			t.Errorf("ParseOPT(social.opt with T_ARCHETYPE_ROOT) differs from the vendored tree:\n%s", summariseDiffs(diffs))
		}

		strict, err := template.ParseOPTStrict(bytes.NewReader(rewritten))
		if err != nil {
			t.Fatalf("ParseOPTStrict(social.opt with T_ARCHETYPE_ROOT) = %v, want nil error", err)
		}
		if diffs := diffNodes("", want.Root(), strict.Root(), nil); len(diffs) > 0 {
			t.Errorf("ParseOPTStrict(social.opt with T_ARCHETYPE_ROOT) differs from the vendored tree:\n%s", summariseDiffs(diffs))
		}
	})

	// Every row carries an archetype_id and a nested attribute, so the
	// unknown-type row also pins that the archetype_id promotion of an
	// untyped definition was not widened to arbitrary types.
	tests := []struct {
		name     string
		xsiType  string
		wantRoot bool
	}{
		{name: "C_ARCHETYPE_ROOT is an archetype root", xsiType: "C_ARCHETYPE_ROOT", wantRoot: true},
		{name: "T_ARCHETYPE_ROOT is an archetype root", xsiType: "T_ARCHETYPE_ROOT", wantRoot: true},
		{name: "unknown type stays a leaf", xsiType: "X_UNKNOWN", wantRoot: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(inlineArchetypeRootOPT, tc.xsiType)

			lenient, err := template.ParseOPT(strings.NewReader(body))
			if err != nil {
				t.Fatalf("ParseOPT(%s child) = %v, want nil error", tc.xsiType, err)
			}
			child := firstContentChild(t, lenient)
			if tc.wantRoot {
				assertInlineArchetypeRoot(t, "ParseOPT", tc.xsiType, child)
			} else {
				co, ok := child.(*template.ComplexObject)
				if !ok {
					t.Fatalf("ParseOPT(%s child): child is %T, want *template.ComplexObject leaf", tc.xsiType, child)
				}
				if n := len(co.Attributes()); n != 0 {
					t.Errorf("ParseOPT(%s child): leaf has %d attributes, want 0", tc.xsiType, n)
				}
			}

			strict, err := template.ParseOPTStrict(strings.NewReader(body))
			if !tc.wantRoot {
				if !errors.Is(err, template.ErrUnsupportedNode) {
					t.Fatalf("ParseOPTStrict(%s child) = %v, want errors.Is(err, ErrUnsupportedNode)", tc.xsiType, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseOPTStrict(%s child) = %v, want nil error", tc.xsiType, err)
			}
			assertInlineArchetypeRoot(t, "ParseOPTStrict", tc.xsiType, firstContentChild(t, strict))
		})
	}
}

// inlineArchetypeRootOPT is a COMPOSITION whose single content child has
// the xsi:type given by the %s verb, an archetype id, one nested
// attribute and one term definition.
const inlineArchetypeRootOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>t</value></template_id>
  <concept>t</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <children xsi:type="%s">
        <rm_type_name>OBSERVATION</rm_type_name>
        <node_id>at0000</node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>data</rm_attribute_name>
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>HISTORY</rm_type_name>
            <node_id>at0001</node_id>
          </children>
        </attributes>
        <archetype_id><value>openEHR-EHR-OBSERVATION.alias_probe.v1</value></archetype_id>
        <term_definitions code="at0000">
          <items id="text">Alias probe</items>
        </term_definitions>
      </children>
    </attributes>
  </definition>
</template>`

// assertInlineArchetypeRoot checks the content child of
// inlineArchetypeRootOPT kept its archetype id, its data attribute and
// its term definition.
func assertInlineArchetypeRoot(t *testing.T, call, xsiType string, child template.Node) {
	t.Helper()
	ar, ok := child.(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("%s(%s child): child is %T, want *template.ArchetypeRoot", call, xsiType, child)
	}
	if got, want := ar.ArchetypeID(), "openEHR-EHR-OBSERVATION.alias_probe.v1"; got != want {
		t.Errorf("%s(%s child): ArchetypeID() = %q, want %q", call, xsiType, got, want)
	}
	attrs := ar.Attributes()
	if len(attrs) != 1 || attrs[0].Name() != "data" || len(attrs[0].Children()) != 1 {
		t.Errorf("%s(%s child): attributes = %s, want one data attribute with one child", call, xsiType, attributeNames(attrs))
	}
	term, ok := ar.Term("at0000")
	if !ok || term.Items["text"] != "Alias probe" {
		t.Errorf("%s(%s child): Term(at0000) = %+v, %v, want text %q", call, xsiType, term, ok, "Alias probe")
	}
}

func firstContentChild(t *testing.T, opt *template.OperationalTemplate) template.Node {
	t.Helper()
	root, ok := opt.Root().(template.ObjectNode)
	if !ok {
		t.Fatalf("Root() is %T, want a template.ObjectNode", opt.Root())
	}
	attrs := root.Attributes()
	if len(attrs) != 1 || len(attrs[0].Children()) != 1 {
		t.Fatalf("Root() attributes = %s, want one content attribute with one child", attributeNames(attrs))
	}
	return attrs[0].Children()[0]
}

func attributeNames(attrs []*template.Attribute) string {
	names := make([]string, 0, len(attrs))
	for _, a := range attrs {
		names = append(names, fmt.Sprintf("%s(%d children)", a.Name(), len(a.Children())))
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// socialWithTArchetypeRoot returns social.opt as vendored, and the same
// bytes with every C_ARCHETYPE_ROOT spelled T_ARCHETYPE_ROOT, which is
// how the original export wrote them.
func socialWithTArchetypeRoot(t *testing.T) (original, rewritten []byte) {
	t.Helper()
	path := fixtures.TemplateOptForName("social")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(social.opt): %v", err)
	}
	from, to := []byte(`xsi:type="C_ARCHETYPE_ROOT"`), []byte(`xsi:type="T_ARCHETYPE_ROOT"`)
	if got, want := bytes.Count(original, from), 7; got != want {
		t.Fatalf("social.opt holds %d %s, want %d", got, from, want)
	}
	return original, bytes.ReplaceAll(original, from, to)
}

func countArchetypeRoots(n template.Node) int {
	count := 0
	if _, ok := n.(*template.ArchetypeRoot); ok {
		count++
	}
	if obj, ok := n.(template.ObjectNode); ok {
		for _, a := range obj.Attributes() {
			for _, c := range a.Children() {
				count += countArchetypeRoots(c)
			}
		}
	}
	return count
}

// diffNodes walks want and got in step and appends one line per
// difference: the node type, RM type, node id, node name, occurrences,
// archetype id, term definitions and bindings, and per attribute its
// name, cardinality, existence and children.
func diffNodes(path string, want, got template.Node, diffs []string) []string {
	if path == "" {
		path = "/"
	}
	if wt, gt := fmt.Sprintf("%T", want), fmt.Sprintf("%T", got); wt != gt {
		return append(diffs, fmt.Sprintf("%s: node type %s, want %s", path, gt, wt))
	}
	if w, g := want.RMTypeName(), got.RMTypeName(); w != g {
		diffs = append(diffs, fmt.Sprintf("%s: RMTypeName %q, want %q", path, g, w))
	}
	if w, g := want.NodeID(), got.NodeID(); w != g {
		diffs = append(diffs, fmt.Sprintf("%s: NodeID %q, want %q", path, g, w))
	}
	switch w := want.(type) {
	case *template.Slot:
		g := got.(*template.Slot)
		if !slices.Equal(w.Includes(), g.Includes()) || !slices.Equal(w.Excludes(), g.Excludes()) {
			diffs = append(diffs, path+": slot assertions differ")
		}
		return diffs
	case *template.ArchetypeRoot:
		g := got.(*template.ArchetypeRoot)
		if w.ArchetypeID() != g.ArchetypeID() {
			diffs = append(diffs, fmt.Sprintf("%s: ArchetypeID %q, want %q", path, g.ArchetypeID(), w.ArchetypeID()))
		}
		sameTerm := func(a, b template.ArchetypeTerm) bool { return a.Code == b.Code && maps.Equal(a.Items, b.Items) }
		if !maps.EqualFunc(w.Terms(), g.Terms(), sameTerm) {
			diffs = append(diffs, fmt.Sprintf("%s: %d term definitions, want %d with the same content", path, len(g.Terms()), len(w.Terms())))
		}
		if !slices.Equal(w.TermBindings(), g.TermBindings()) {
			diffs = append(diffs, fmt.Sprintf("%s: %d term bindings, want %d with the same content", path, len(g.TermBindings()), len(w.TermBindings())))
		}
	case *template.ComplexObject:
		g := got.(*template.ComplexObject)
		if wp, gp := fmt.Sprintf("%T", w.PrimitiveConstraint()), fmt.Sprintf("%T", g.PrimitiveConstraint()); wp != gp {
			diffs = append(diffs, fmt.Sprintf("%s: primitive constraint %s, want %s", path, gp, wp))
		}
	}
	wo, ok := want.(template.ObjectNode)
	if !ok {
		return diffs
	}
	gotObj := got.(template.ObjectNode)
	if w, g := wo.NodeName(), gotObj.NodeName(); w != g {
		diffs = append(diffs, fmt.Sprintf("%s: NodeName %q, want %q", path, g, w))
	}
	if !sameMultiplicity(wo.Occurrences(), gotObj.Occurrences()) {
		diffs = append(diffs, path+": occurrences differ")
	}
	wa, ga := wo.Attributes(), gotObj.Attributes()
	if len(wa) != len(ga) {
		return append(diffs, fmt.Sprintf("%s: attributes %s, want %s", path, attributeNames(ga), attributeNames(wa)))
	}
	for i := range wa {
		at := strings.TrimSuffix(path, "/") + "/" + wa[i].Name()
		if wa[i].Name() != ga[i].Name() || wa[i].Cardinality() != ga[i].Cardinality() {
			diffs = append(diffs, fmt.Sprintf("%s: attribute %d is %s %s, want %s %s",
				path, i, ga[i].Name(), ga[i].Cardinality(), wa[i].Name(), wa[i].Cardinality()))
			continue
		}
		if !sameMultiplicity(wa[i].Existence(), ga[i].Existence()) || !sameMultiplicity(wa[i].ChildMultiplicity(), ga[i].ChildMultiplicity()) {
			diffs = append(diffs, at+": existence or cardinality differs")
		}
		wc, gc := wa[i].Children(), ga[i].Children()
		if len(wc) != len(gc) {
			diffs = append(diffs, fmt.Sprintf("%s: %d children, want %d", at, len(gc), len(wc)))
			continue
		}
		for j := range wc {
			diffs = diffNodes(fmt.Sprintf("%s[%d]", at, j), wc[j], gc[j], diffs)
		}
	}
	return diffs
}

func sameMultiplicity(a, b *template.Multiplicity) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func summariseDiffs(diffs []string) string {
	const maxShown = 10
	shown := diffs[:min(len(diffs), maxShown)]
	out := strings.Join(shown, "\n")
	if len(diffs) > maxShown {
		out += fmt.Sprintf("\n...and %d more", len(diffs)-maxShown)
	}
	return out
}

package instance_test

// pinned_name_test.go: REQ-107 and REQ-111 § Node names and name predicates
// on the name the generator writes where the template fixes one: it is the
// node's template-level name, so a generated skeleton and its compiled paths
// agree (issue 455).

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// conceptTerm is the term definition of at0000, an archetype's concept,
// with the text Concept.
const conceptTerm = `<term_definitions code="at0000"><items id="text">Concept</items></term_definitions>`

// TestREQ107_REQ111_NameFixedByOneCodeIsTheNodeName is the REQ-107 term-text
// check on a locatable's name the template fixes by one code (REQ-111 § Node
// names and name predicates): the generator writes as its text the node's
// template-level name, the term text of that code, trimmed. at0000 names the
// archetype's concept, a real code where the OPT lists it, while the walk's
// placeholder at0000, for a code phrase that lists no code, keeps the walk's
// text and fixes no name. A nested archetype's name takes its own concept,
// not the enclosing one's. It holds under every policy and both value fills.
func TestREQ107_REQ111_NameFixedByOneCodeIsTheNodeName(t *testing.T) {
	cases := []struct {
		name     string
		opt      string
		coded    func(t *testing.T, out any) rm.DVCodedText
		want     string
		nodeName func(c *templatecompile.Compiled) string
	}{
		{
			name:  "the concept code at0000 the OPT lists",
			opt:   optTemplate("ELEMENT", conceptTerm, optTerms, optSingle("name", optCodedText("local", "at0000"))),
			coded: elementCodedName,
			want:  "Concept",
		},
		{
			name: "a code whose term text is padded",
			opt: optTemplate("ELEMENT", `<term_definitions code="at0004"><items id="text">  Padded name
    </items></term_definitions>`, optSingle("name", optCodedText("local", "at0004"))),
			coded: elementCodedName,
			want:  "Padded name",
		},
		{
			name:  "the placeholder at0000 of a code phrase that lists no code",
			opt:   optTemplate("ELEMENT", conceptTerm, optTerms, optSingle("name", optCodedText("local"))),
			coded: elementCodedName,
			want:  walkText,
		},
		{
			name:     "a nested archetype's concept code",
			opt:      nestedConceptOPT(),
			coded:    innerClusterName,
			want:     "Inner concept",
			nodeName: innerClusterNodeName,
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		nodeName := c.Root().NodeName()
		if tc.nodeName != nil {
			nodeName = tc.nodeName(c)
		}
		if tc.want != walkText && nodeName != tc.want {
			t.Fatalf("%s: NodeName() = %q, want %q — the compile step no longer fixes this name", tc.name, nodeName, tc.want)
		}
		if tc.want == walkText && nodeName != "" {
			t.Fatalf("%s: NodeName() = %q, want none — the placeholder fixes no name", tc.name, nodeName)
		}
		for _, opts := range defaultsOptions() {
			t.Run(fmt.Sprintf("%s/%v/%v", tc.name, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				if got := tc.coded(t, out); got.Value != tc.want {
					t.Errorf("name = %q %+v, want %q", got.Value, got.DefiningCode, tc.want)
				}
				checked := 0
				for _, m := range pinnedNameMismatches(c.Root(), out, "", &checked) {
					t.Error(m)
				}
				checkValid(t, out, c)
			})
		}
	}
}

// nestedConceptOPT is a CLUSTER template whose archetype's concept is Outer
// concept, holding an inner CLUSTER archetype whose concept is Inner concept
// and whose name the OPT fixes to its concept code, local::at0000.
func nestedConceptOPT() string {
	element := optNode("ELEMENT", "at0001", optSingle("value", optNode("DV_TEXT", "")))
	inner := optArchetypeRoot("CLUSTER", "openEHR-EHR-CLUSTER.inner.v1",
		`<term_definitions code="at0000"><items id="text">Inner concept</items></term_definitions>`,
		`<term_definitions code="at0001"><items id="text">Inner element</items></term_definitions>`,
		optSingle("name", optCodedText("local", "at0000")),
		optMultiple("items", element))
	return optTemplate("CLUSTER", `<term_definitions code="at0000"><items id="text">Outer concept</items></term_definitions>`,
		optMultiple("items", inner))
}

// innerClusterName is the name of the inner CLUSTER of nestedConceptOPT in a
// generated root, which must be a DV_CODED_TEXT.
func innerClusterName(t *testing.T, out any) rm.DVCodedText {
	t.Helper()
	root, ok := out.(*rm.Cluster)
	if !ok || len(root.Items) != 1 {
		t.Fatalf("root = %T, want a CLUSTER of one item", out)
	}
	inner, ok := root.Items[0].(*rm.Cluster)
	if !ok {
		t.Fatalf("item = %T, want the inner CLUSTER", root.Items[0])
	}
	switch name := inner.Name.(type) {
	case rm.DVCodedText:
		return name
	case *rm.DVCodedText:
		return *name
	default:
		t.Fatalf("inner CLUSTER name is %T, want a DV_CODED_TEXT", inner.Name)
		return rm.DVCodedText{}
	}
}

// innerClusterNodeName is the template-level name of the inner CLUSTER of
// nestedConceptOPT.
func innerClusterNodeName(c *templatecompile.Compiled) string {
	nodes := c.AllByArchetypeID("openEHR-EHR-CLUSTER.inner.v1")
	if len(nodes) != 1 {
		return ""
	}
	return nodes[0].NodeName()
}

// TestREQ107_REQ111_CorpusNamesAreTheNodeNames is the REQ-107 check, over
// every vendored template, that each locatable the generator writes whose
// OPT node fixes a name (REQ-111 § Node names and name predicates) carries
// that name, under every policy, both value fills and several seeds, so a
// compiled path's name predicate reaches the generated node. It fails when
// no generated locatable's node fixes a name, so nothing was checked.
func TestREQ107_REQ111_CorpusNamesAreTheNodeNames(t *testing.T) {
	checked := 0
	eachCorpusRun(t, func(run corpusRun) {
		for _, m := range pinnedNameMismatches(run.compiled.Root(), run.out, "", &checked) {
			t.Errorf("%s: %s", run, m)
		}
	})
	if checked == 0 {
		t.Fatal("no generated locatable whose OPT node fixes a name, so nothing was checked")
	}
	t.Logf("%d generated locatables checked against the name their OPT node fixes", checked)
}

// TestREQ107_REQ111_NodeNamesHoldInAnotherLanguage is the same check with
// Options.Language other than the template's own language, on the vendored
// templates that carry an archetype's terms in another language besides
// their own: the name a template fixes is read in the one language whose
// terms the compile reads, and so is the name the generator writes, so the
// two agree whatever Options.Language asks for.
func TestREQ107_REQ111_NodeNamesHoldInAnotherLanguage(t *testing.T) {
	checked := 0
	for _, tc := range []struct{ template, language string }{
		{template: "IDCR - Vital Signs Encounter.v1", language: "de"},
		{template: "family_history.v.1.2.3", language: "de"},
		{template: "body_weight", language: "nl"},
	} {
		c := compileFixture(t, tc.template)
		for _, opts := range defaultsOptions() {
			opts.Language = tc.language
			opts.Territory, opts.Composer = "NL", testComposer()
			out, err := instance.Generate(t.Context(), c, opts)
			if err != nil {
				t.Fatalf("%s/%v/%v: Generate: %v", tc.template, opts.Policy, opts.ValueFill, err)
			}
			for _, m := range pinnedNameMismatches(c.Root(), out, "", &checked) {
				t.Errorf("%s/lang=%s/%v/%v: %s", tc.template, tc.language, opts.Policy, opts.ValueFill, m)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no generated locatable whose OPT node fixes a name, so nothing was checked")
	}
}

// pinnedNameMismatches walks the RM value v beside n, the OPT node it was
// built from, and returns one message for each locatable under v, v itself
// excluded, whose name is none of the names its candidate OPT nodes fix,
// where each of them fixes one. A child's candidates are the children of
// the attribute that holds it that carry its archetype_node_id, as an
// archetype id or a node id, and whose class it conforms to. The walk goes
// on below the candidate that fixes the child's name, else below the first
// that fixes none, as the validator binds a child among siblings that share
// an id; it goes below a value that is no locatable where one OPT child of
// its class holds it. checked counts the locatables whose node fixes a name.
func pinnedNameMismatches(n *templatecompile.CompiledNode, v any, path string, checked *int) []string {
	var out []string
	for _, attr := range templatecompile.NodeAttributes(n) {
		kids := templatecompile.AllowedChildNodes(attr)
		if len(kids) == 0 {
			continue
		}
		var values []any
		if attr.Cardinality() == template.Multiple {
			values, _ = rmaccess.Multiple(v, attr.Name())
		} else if value, ok := rmaccess.Single(v, attr.Name()); ok {
			values = []any{value}
		}
		for i, value := range values {
			at := fmt.Sprintf("%s/%s", path, attr.Name())
			if attr.Cardinality() == template.Multiple {
				at = fmt.Sprintf("%s[%d]", at, i)
			}
			class, _ := rm.RMTypeName(value)
			kin := slices.DeleteFunc(slices.Clone(kids), func(k *templatecompile.CompiledNode) bool {
				conforms, known := templatecompile.ClassConformsTo(class, k.RMTypeName())
				return k.RMTypeName() != "" && known && !conforms
			})
			l, isLocatable := value.(rm.Locatable)
			if !isLocatable || rm.IsTypedNil(value) {
				if len(kin) == 1 {
					out = append(out, pinnedNameMismatches(kin[0], value, at, checked)...)
				}
				continue
			}
			id := l.GetArchetypeNodeID()
			candidates := slices.DeleteFunc(kin, func(k *templatecompile.CompiledNode) bool {
				key := k.ArchetypeID()
				if key == "" {
					key = k.NodeID()
				}
				return key != id
			})
			if len(candidates) == 0 {
				continue
			}
			name := ""
			if dv := l.GetName(); dv != nil && !rm.IsTypedNil(dv) {
				name = dv.GetValue()
			}
			var bound *templatecompile.CompiledNode
			var fixed []string
			for _, k := range candidates {
				switch k.NodeName() {
				case name:
					if bound == nil || bound.NodeName() == "" {
						bound = k
					}
				case "":
					if bound == nil {
						bound = k
					}
				default:
					fixed = append(fixed, k.NodeName())
				}
			}
			if bound == nil {
				out = append(out, fmt.Sprintf("%s[%s] is named %q, want the name its OPT node fixes, one of %q", at, id, name, fixed))
				continue
			}
			if bound.NodeName() != "" {
				*checked++
			}
			out = append(out, pinnedNameMismatches(bound, value, at, checked)...)
		}
	}
	return out
}

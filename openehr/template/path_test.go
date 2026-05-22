package template_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// REQ-100 § Path syntax — accept valid forms.
func TestParsePath_ValidForms(t *testing.T) {
	opt := mustParseVitalSigns(t)
	cases := []struct {
		in   string
		want string // String() round-trip
	}{
		{"/", "/"},
		{"/content", "/content"},
		{"/category/defining_code", "/category/defining_code"},
		{"/content[at0001]", "/content[at0001]"},
		{"/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]", "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]"},
		{"/content[at0001]/data", "/content[at0001]/data"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			p, err := opt.ParsePath(tc.in)
			if err != nil {
				t.Fatalf("ParsePath(%q): %v", tc.in, err)
			}
			if got := p.String(); got != tc.want {
				t.Errorf("round-trip = %q, want %q", got, tc.want)
			}
		})
	}
}

// REQ-100 § Path syntax — reject malformed grammar.
func TestParsePath_RejectsMalformed(t *testing.T) {
	opt := mustParseVitalSigns(t)
	cases := []struct {
		in     string
		reason string
	}{
		{"", "empty"},
		{"content", "missing leading slash"},
		{"/content/", "trailing slash"},
		{"//content", "empty segment"},
		{"/content[", "unclosed predicate"},
		{"/content[at0001", "unclosed predicate"},
		{"/content]", "unbalanced bracket"},
		{"/content[]", "empty predicate"},
		{"/[at0001]", "predicate without name"},
		// REQ-100 explicitly rejects AQL-style predicates.
		{"/content[name='Systolic']", "AQL predicate"},
		{"/content[at0001,name='x']", "multi-predicate"},
		{"/content[@id=x]", "@ marker"},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			_, err := opt.ParsePath(tc.in)
			if !errors.Is(err, template.ErrPathSyntax) {
				t.Fatalf("ParsePath(%q) = %v, want ErrPathSyntax", tc.in, err)
			}
		})
	}
}

// REQ-100 § Resolution semantics — root path returns the OPT root.
func TestNodeAt_Root(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, err := opt.ParsePath("/")
	if err != nil {
		t.Fatalf("ParsePath(/): %v", err)
	}
	n, err := opt.NodeAt(p)
	if err != nil {
		t.Fatalf("NodeAt(/): %v", err)
	}
	if n != opt.Root() {
		t.Errorf("NodeAt(/) did not return the template root")
	}
}

// REQ-100 § Resolution semantics — walk into a single attribute.
func TestNodeAt_SingleAttribute(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, _ := opt.ParsePath("/content")
	n, err := opt.NodeAt(p)
	if err != nil {
		t.Fatalf("NodeAt(/content): %v", err)
	}
	// First content child is an ArchetypeRoot for the first
	// vital_signs observation slot fill.
	ar, ok := n.(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("NodeAt(/content) type = %T, want *template.ArchetypeRoot", n)
	}
	if ar.RMTypeName() != "OBSERVATION" {
		t.Errorf("RMTypeName = %q, want OBSERVATION", ar.RMTypeName())
	}
	if !strings.HasPrefix(ar.ArchetypeID(), "openEHR-EHR-OBSERVATION.") {
		t.Errorf("ArchetypeID = %q, want openEHR-EHR-OBSERVATION.* prefix", ar.ArchetypeID())
	}
}

// REQ-100 § Resolution semantics — predicate selects a specific
// archetype-root sibling (not just the first child).
func TestNodeAt_PredicateArchetypeID(t *testing.T) {
	opt := mustParseVitalSigns(t)
	// The vital_signs OPT has multiple OBSERVATION archetype roots
	// under /content. Walk through each first to pick the second
	// archetype id deterministically.
	first, _ := opt.NodeAt(mustParse(t, opt, "/content"))
	firstAR, ok := first.(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("first /content child is %T, want *template.ArchetypeRoot", first)
	}

	co, ok := opt.Root().(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("root not an *ArchetypeRoot: %T", opt.Root())
	}
	var contentAttr *template.Attribute
	for _, a := range co.Attributes() {
		if a.Name() == "content" {
			contentAttr = a
			break
		}
	}
	if contentAttr == nil || len(contentAttr.Children()) < 2 {
		t.Skip("fixture changed: need at least 2 children under /content for predicate test")
	}

	// Pick the archetype id of the second content child and look it
	// up via predicate.
	var secondAR *template.ArchetypeRoot
	for i, c := range contentAttr.Children() {
		if i == 0 {
			continue
		}
		if ar, ok := c.(*template.ArchetypeRoot); ok {
			secondAR = ar
			break
		}
	}
	if secondAR == nil {
		t.Skip("fixture changed: need another ArchetypeRoot under /content")
	}
	if secondAR.ArchetypeID() == firstAR.ArchetypeID() {
		t.Skip("fixture changed: second child has same archetype id as first")
	}

	path := "/content[" + secondAR.ArchetypeID() + "]"
	p, err := opt.ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", path, err)
	}
	n, err := opt.NodeAt(p)
	if err != nil {
		t.Fatalf("NodeAt(%q): %v", path, err)
	}
	gotAR, ok := n.(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("NodeAt(%q) = %T, want *template.ArchetypeRoot", path, n)
	}
	if gotAR.ArchetypeID() != secondAR.ArchetypeID() {
		t.Errorf("predicate selected %q, want %q", gotAR.ArchetypeID(), secondAR.ArchetypeID())
	}
}

// REQ-100 § Resolution semantics — unknown attribute → ErrPathNotFound.
func TestNodeAt_UnknownAttribute(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, _ := opt.ParsePath("/this_attribute_does_not_exist")
	_, err := opt.NodeAt(p)
	if !errors.Is(err, template.ErrPathNotFound) {
		t.Fatalf("got %v, want ErrPathNotFound", err)
	}
}

// REQ-100 § Resolution semantics — unmatched predicate → ErrPathNotFound.
func TestNodeAt_UnmatchedPredicate(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, _ := opt.ParsePath("/content[at9999]")
	_, err := opt.NodeAt(p)
	if !errors.Is(err, template.ErrPathNotFound) {
		t.Fatalf("got %v, want ErrPathNotFound", err)
	}
}

// REQ-100 § Resolution semantics — descending into a leaf node returns
// ErrPathNotFound when the segment cannot be honoured.
func TestNodeAt_DeepNonexistent(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, _ := opt.ParsePath("/category/defining_code/no_such_attr")
	_, err := opt.NodeAt(p)
	if !errors.Is(err, template.ErrPathNotFound) {
		t.Fatalf("got %v, want ErrPathNotFound", err)
	}
}

// --- helpers ------------------------------------------------------------

func mustParseVitalSigns(t *testing.T) *template.OperationalTemplate {
	t.Helper()
	opt, err := template.ParseFile(filepath.Join("testdata", "vital_signs.opt"))
	if err != nil {
		t.Fatalf("load vital_signs.opt: %v", err)
	}
	return opt
}

func mustParse(t *testing.T, opt *template.OperationalTemplate, path string) template.Path {
	t.Helper()
	p, err := opt.ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", path, err)
	}
	return p
}

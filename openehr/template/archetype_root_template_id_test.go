package template_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestREQ100_ArchetypeRootTemplateID is the REQ-100 check that the parser
// reads the optional <template_id> openEHR Template.xsd allows on an
// archetype root, in both parse modes. Corona_Anamnese.opt names the
// template Corona_Anamnese and carries Corona_Anamnese_MDM_V2 on its
// definition root, so ArchetypeRoot.TemplateID and
// OperationalTemplate.TemplateID differ there. A nested archetype root
// with no template id returns "". The answers are the same when the
// nested roots are spelled T_ARCHETYPE_ROOT.
func TestREQ100_ArchetypeRootTemplateID(t *testing.T) {
	vendored, err := os.ReadFile(fixtures.WebTemplateOpt("Corona_Anamnese"))
	if err != nil {
		t.Fatalf("ReadFile(Corona_Anamnese.opt): %v", err)
	}
	from, to := []byte(`xsi:type="C_ARCHETYPE_ROOT"`), []byte(`xsi:type="T_ARCHETYPE_ROOT"`)
	if bytes.Count(vendored, from) == 0 {
		t.Fatalf("Corona_Anamnese.opt holds no %s, want nested archetype roots", from)
	}
	sources := []struct {
		name string
		body []byte
	}{
		{name: "as vendored", body: vendored},
		{name: "with T_ARCHETYPE_ROOT", body: bytes.ReplaceAll(vendored, from, to)},
	}
	parsers := []struct {
		name  string
		parse func([]byte) (*template.OperationalTemplate, error)
	}{
		{name: "ParseOPT", parse: func(b []byte) (*template.OperationalTemplate, error) { return template.ParseOPT(bytes.NewReader(b)) }},
		{name: "ParseOPTStrict", parse: func(b []byte) (*template.OperationalTemplate, error) {
			return template.ParseOPTStrict(bytes.NewReader(b))
		}},
	}
	for _, src := range sources {
		for _, p := range parsers {
			t.Run(src.name+"/"+p.name, func(t *testing.T) {
				opt, err := p.parse(src.body)
				if err != nil {
					t.Fatalf("%s(Corona_Anamnese.opt %s): %v", p.name, src.name, err)
				}
				if got, want := opt.TemplateID(), "Corona_Anamnese"; got != want {
					t.Errorf("OperationalTemplate.TemplateID() = %q, want %q", got, want)
				}
				root, ok := opt.Root().(*template.ArchetypeRoot)
				if !ok {
					t.Fatalf("Root() is %T, want *template.ArchetypeRoot", opt.Root())
				}
				if got, want := root.TemplateID(), "Corona_Anamnese_MDM_V2"; got != want {
					t.Errorf("Root().TemplateID() = %q, want %q", got, want)
				}
				nested := firstNestedArchetypeRoot(root)
				if nested == nil {
					t.Fatal("no archetype root below the definition, want the nested roots")
				}
				if got := nested.TemplateID(); got != "" {
					t.Errorf("nested root %s: TemplateID() = %q, want \"\"", nested.ArchetypeID(), got)
				}
			})
		}
	}
}

// firstNestedArchetypeRoot returns the first archetype root below n, in
// document order, or nil when there is none.
func firstNestedArchetypeRoot(n template.ObjectNode) *template.ArchetypeRoot {
	for _, a := range n.Attributes() {
		for _, c := range a.Children() {
			if ar, ok := c.(*template.ArchetypeRoot); ok {
				return ar
			}
			if obj, ok := c.(template.ObjectNode); ok {
				if ar := firstNestedArchetypeRoot(obj); ar != nil {
					return ar
				}
			}
		}
	}
	return nil
}

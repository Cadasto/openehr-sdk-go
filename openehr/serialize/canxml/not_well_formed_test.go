package canxml_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestREQ056_NotWellFormedIsItsOwnClass pins that the decode's own refusal of
// input that is not well-formed XML can be told apart from a document of the
// wrong shape (REQ-056, REQ-111): it wraps canxml.ErrNotWellFormed beside
// canxml.ErrInvalidShape, for canxml.Unmarshal in every mode and for the
// operational template reader, while a primitive failure wraps
// canxml.ErrInvalidShape alone. Its text names the rule the input breaks, not
// "cause withheld", and none of the input. Markup encoding/xml cannot read
// before the root, with nothing refused before it, stays its *xml.SyntaxError
// in both readers. The inputs are the corpus BMI documents with one defect
// each.
//
// Can-fail control: drop the sentinel from a refusal's cause and its row
// fails the errors.Is check; drop its text from the value-free renderer's
// forms and its row reads "cause withheld"; refuse a syntax error the prolog
// check reads before anything it refused, and the syntax-error rows fail.
func TestREQ056_NotWellFormedIsItsOwnClass(t *testing.T) {
	composition := readCorpus(t, "compositions/BMI.xml")
	template := readCorpus(t, "templates/BMI.opt")
	rootStart := func(doc []byte, root string) int {
		t.Helper()
		i := bytes.Index(doc, []byte("<"+root))
		if i < 0 {
			t.Fatalf("no <%s> in the corpus document", root)
		}
		return i
	}
	// withAttr adds attrs to the start tag of root.
	withAttr := func(doc []byte, root, attrs string) []byte {
		at := rootStart(doc, root) + len("<"+root)
		return joined(doc[:at], []byte(" "+attrs), doc[at:])
	}
	// before inserts s before the first child element of the composition.
	before := func(doc []byte, s string) []byte {
		i := bytes.Index(doc, []byte("<name>"))
		if i < 0 {
			t.Fatal("no <name> in the corpus composition")
		}
		return joined(doc[:i], []byte(s), doc[i:])
	}

	// Each document is built here, before the subtests, so a corpus that
	// no longer holds what a helper looks for fails the test once, here.
	compositionTwice := withAttr(composition, "COMPOSITION", `archetype_node_id="PLANTED"`)
	declarationInside := before(composition, `<?xml version="1.0"?>`)
	declarationAfterSpace := joined([]byte(" "+`<?xml version="1.0" encoding="UTF-8"?>`), composition)
	declarationAfterSpaceThenSyntax := joined([]byte(" "+`<?xml version="1.0"?><1>`), composition)
	compositionTwoRoots := joined(composition, []byte("<PLANTED/>"))
	templateTwice := withAttr(template, "template", `concept="PLANTED" concept="PLANTED"`)
	templateTwoRoots := joined(template, []byte("<PLANTED/>"))

	cases := []struct {
		name, rule string
		decode     func() error
	}{
		{
			name: "composition with archetype_node_id twice",
			rule: "the element carries one attribute twice",
			decode: func() error {
				return canxml.Unmarshal(compositionTwice, new(rm.Composition))
			},
		},
		{
			name: "composition with an XML declaration before its name",
			rule: "the element holds a processing instruction whose target is xml",
			decode: func() error {
				return canxml.Unmarshal(declarationInside, new(rm.Composition), canxml.WithStrict())
			},
		},
		{
			name: "composition after white space and a declaration",
			rule: "content before the root element",
			decode: func() error {
				return canxml.Unmarshal(declarationAfterSpace, new(rm.Composition))
			},
		},
		{
			// The misplaced declaration is refused before the decode reaches
			// the markup it cannot read.
			name: "composition after white space, a declaration and markup it cannot read",
			rule: "content before the root element",
			decode: func() error {
				return canxml.Unmarshal(declarationAfterSpaceThenSyntax, new(rm.Composition))
			},
		},
		{
			name: "composition followed by a second root",
			rule: "content after the root element",
			decode: func() error {
				return canxml.Unmarshal(compositionTwoRoots, new(rm.Composition))
			},
		},
		{
			name: "template with concept twice",
			rule: "the element carries one attribute twice",
			decode: func() error {
				_, err := aom14.ReadOperationalTemplate(bytes.NewReader(templateTwice))
				return err
			},
		},
		{
			name: "template followed by a second root",
			rule: "content after the root element",
			decode: func() error {
				_, err := aom14.ReadOperationalTemplate(bytes.NewReader(templateTwoRoots))
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.decode()
			if !errors.Is(err, canxml.ErrNotWellFormed) || !errors.Is(err, canxml.ErrInvalidShape) {
				t.Fatalf("decode = %v; want an error that wraps canxml.ErrNotWellFormed and canxml.ErrInvalidShape", err)
			}
			got := err.Error()
			if !strings.Contains(got, tc.rule) || strings.Contains(got, "cause withheld") {
				t.Errorf("Error() = %q; want it to name the rule %q", got, tc.rule)
			}
			if strings.Contains(got, "PLANTED") {
				t.Errorf("Error() = %q; it carries the input", got)
			}
		})
	}

	// Markup encoding/xml cannot read before the root, which may be the
	// root's own start tag, is its syntax error when nothing refused comes
	// before it: first, after a comment, and after the XML declaration, so
	// that the prolog check reads up to it.
	syntaxErrors := map[string]func() error{}
	for head, name := range map[string]string{
		"<1>":                           "first",
		"<!-- c --><1>":                 "after a comment",
		`<?xml version="1.0"?>` + "<1>": "after the XML declaration",
	} {
		syntaxErrors["composition, markup it cannot read "+name] = func() error {
			return canxml.Unmarshal(joined([]byte(head), composition), new(rm.Composition), canxml.WithStrict())
		}
		for mode, opts := range map[string][]canxml.DecoderOption{"default": nil, "strict": {canxml.WithStrict()}} {
			syntaxErrors[mode+" template, markup it cannot read "+name] = func() error {
				_, err := aom14.ReadOperationalTemplate(bytes.NewReader(joined([]byte(head), template)), opts...)
				return err
			}
		}
	}
	for name, decode := range syntaxErrors {
		t.Run(name, func(t *testing.T) {
			err := decode()
			if _, ok := errors.AsType[*xml.SyntaxError](err); !ok || errors.Is(err, canxml.ErrNotWellFormed) {
				t.Fatalf("decode = %v; want an *xml.SyntaxError that does not wrap canxml.ErrNotWellFormed", err)
			}
		})
	}

	t.Run("a primitive failure is not one", func(t *testing.T) {
		const doc = `<ehr_status xmlns="http://schemas.openehr.org/v1"><is_queryable>maybe</is_queryable></ehr_status>`
		err := canxml.Unmarshal([]byte(doc), new(rm.EHRStatus))
		if !errors.Is(err, canxml.ErrInvalidShape) || errors.Is(err, canxml.ErrNotWellFormed) {
			t.Fatalf("canxml.Unmarshal(is_queryable maybe) = %v; want an error that wraps canxml.ErrInvalidShape and not canxml.ErrNotWellFormed", err)
		}
	})
}

// readCorpus returns the corpus file at rel under testkit/corpus, without a
// leading byte order mark.
func readCorpus(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimPrefix(b, []byte("\ufeff"))
}

// joined returns a new slice holding the parts one after the other.
func joined(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

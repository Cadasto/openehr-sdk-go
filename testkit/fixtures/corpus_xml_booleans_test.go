package fixtures_test

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// xsBooleanElements lists the element names that the openEHR XML schemas type
// as xs:boolean. An xs:boolean allows only true, false, 1 and 0, so an empty
// element is invalid schema data.
//
// Sources: the openEHR ITS-XML AM Release-1.4 Archetype.xsd (false_valid,
// true_valid, is_controlled, is_ordered, is_unique, list_open,
// precedence_overridden) and the RM BaseTypes.xsd (the four Interval flags
// lower_included, lower_unbounded, upper_included, upper_unbounded).
//
// "value" and "assumed_value" are left out on purpose: in OPTs they are also
// used with non-boolean types, so a name match alone would misfire.
var xsBooleanElements = []string{
	"false_valid",
	"true_valid",
	"is_controlled",
	"is_ordered",
	"is_unique",
	"list_open",
	"precedence_overridden",
	"lower_included",
	"lower_unbounded",
	"upper_included",
	"upper_unbounded",
}

// booleanFindings returns one message per element in xsBooleanElements whose
// trimmed content is not one of true, false, 1 or 0. Each message names the
// line of the element's start tag.
func booleanFindings(r io.Reader) ([]string, error) {
	dec := xml.NewDecoder(r)
	var (
		out   []string
		open  string // boolean element being read, "" when none
		line  int
		value strings.Builder
	)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			open = ""
			if slices.Contains(xsBooleanElements, t.Name.Local) {
				open = t.Name.Local
				line, _ = dec.InputPos()
				value.Reset()
			}
		case xml.CharData:
			if open != "" {
				value.Write(t)
			}
		case xml.EndElement:
			if open == "" || t.Name.Local != open {
				continue
			}
			switch v := strings.TrimSpace(value.String()); v {
			case "true", "false", "1", "0":
			case "":
				out = append(out, fmt.Sprintf("line %d: <%s> is empty", line, open))
			default:
				out = append(out, fmt.Sprintf("line %d: <%s> holds %q", line, open, v))
			}
			open = ""
		}
	}
}

// TestCorpusOPTBooleansAreLexicallyValid guards REQ-100 fixture hygiene: no
// vendored OPT may spell an xs:boolean element as an empty element.
func TestCorpusOPTBooleansAreLexicallyValid(t *testing.T) {
	var files []string
	err := filepath.WalkDir(fixtures.CorpusRoot(), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".opt") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", fixtures.CorpusRoot(), err)
	}
	if len(files) == 0 {
		t.Fatalf("no .opt files under %s", fixtures.CorpusRoot())
	}
	for _, p := range files {
		rel, _ := filepath.Rel(fixtures.CorpusRoot(), p)
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(p)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			found, err := booleanFindings(f)
			if err != nil {
				t.Fatalf("decode %s: %v", rel, err)
			}
			for _, msg := range found {
				t.Errorf("%s: %s", rel, msg)
			}
		})
	}
}

// TestBooleanFindingsDetectsEmptyForms is the can-fail control for REQ-100:
// the scanner must flag every empty spelling and accept valid values.
func TestBooleanFindingsDetectsEmptyForms(t *testing.T) {
	const doc = "<r>\n<lower_unbounded/>\n<upper_unbounded />\n<lower_included></lower_included>\n" +
		"<upper_included> </upper_included>\n<is_unique>maybe</is_unique>\n" +
		"<true_valid>true</true_valid>\n<false_valid> 0 </false_valid>\n<value/>\n</r>"
	got, err := booleanFindings(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("booleanFindings = %q, want 5 findings", got)
	}
}

// TestVitalSignsParsesWithOpenMagnitudeBounds pins REQ-100 behaviour: the
// template still parses, leniently and strictly, with its explicit open flags.
func TestVitalSignsParsesWithOpenMagnitudeBounds(t *testing.T) {
	p := fixtures.TemplateOpt("vital_signs")
	if _, err := template.ParseFile(p); err != nil {
		t.Errorf("ParseFile(%s) = %v, want nil", p, err)
	}
	if _, err := template.ParseFileStrict(p); err != nil {
		t.Errorf("ParseFileStrict(%s) = %v, want nil", p, err)
	}
}

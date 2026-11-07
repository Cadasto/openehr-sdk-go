package aom14_test

// REQ-176, REQ-052: canonical JSON writes the operational template model's
// classes under their registered type names and a `Hash` as a JSON object,
// and reads back what it writes for every operational template the corpus
// holds.

import (
	json "encoding/json/v2"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestREQ176_REQ052_OperationalTemplateCanonicalJSON pins canonical JSON for
// the operational template model: social.opt, read from canonical XML, writes
// its classes under the type names the registry holds, OPERATIONAL_TEMPLATE
// at the root and C_ARCHETYPE_ROOT for its definition, and a `Hash` as a JSON
// object, an ARCHETYPE_TERM's items by key and RESOURCE_DESCRIPTION's details
// by language; and the JSON decodes back to the same value.
func TestREQ176_REQ052_OperationalTemplateCanonicalJSON(t *testing.T) {
	raw, err := os.ReadFile("../../../testkit/corpus/templates/social.opt")
	if err != nil {
		t.Fatalf("read social.opt: %v", err)
	}
	var ot aom14.OperationalTemplate
	if err := canxml.Unmarshal(raw, &ot); err != nil {
		t.Fatalf("canxml.Unmarshal(social.opt) = %v", err)
	}
	b, err := canjson.Marshal(&ot)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var doc struct {
		Type       string `json:"_type"`
		Definition struct {
			Type            string `json:"_type"`
			TermDefinitions []struct {
				Items map[string]string `json:"items"`
			} `json:"term_definitions"`
		} `json:"definition"`
		Description struct {
			Details map[string]struct {
				Type string `json:"_type"`
			} `json:"details"`
		} `json:"description"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("reading the canonical JSON: %v", err)
	}
	if doc.Type != "OPERATIONAL_TEMPLATE" || doc.Definition.Type != "C_ARCHETYPE_ROOT" {
		t.Errorf("_type of the root and its definition = %q, %q; want OPERATIONAL_TEMPLATE, C_ARCHETYPE_ROOT", doc.Type, doc.Definition.Type)
	}
	if len(doc.Definition.TermDefinitions) == 0 || doc.Definition.TermDefinitions[0].Items["text"] == "" {
		t.Errorf("the first term's items are not a JSON object holding its text: %+v", doc.Definition.TermDefinitions)
	}
	if doc.Description.Details["nl"].Type != "RESOURCE_DESCRIPTION_ITEM" {
		t.Errorf("description.details = %+v; want a JSON object keyed by language holding a RESOURCE_DESCRIPTION_ITEM", doc.Description.Details)
	}
	var back aom14.OperationalTemplate
	if err := canjson.Unmarshal(b, &back, canjson.WithStrict()); err != nil {
		t.Fatalf("canjson.Unmarshal = %v", err)
	}
	if !reflect.DeepEqual(back, ot) {
		t.Error("social.opt's canonical JSON decodes to another value than the one written")
	}
}

// TestREQ176_REQ052_CorpusOperationalTemplatesCanonicalJSON pins the canonical
// JSON round trip of every operational template under testkit/corpus: each,
// read from canonical XML, writes canonical JSON that decodes, strictly, to
// the value read. A typed EXPR_LEAF item compares by its text and the
// primitive its Type reads as, since canonical JSON keeps neither the prefix
// nor which of the primitive's built-ins Type names.
func TestREQ176_REQ052_CorpusOperationalTemplatesCanonicalJSON(t *testing.T) {
	const corpus = "../../../testkit/corpus"
	var paths []string
	err := filepath.WalkDir(corpus, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".opt") {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		t.Fatalf("walking %s: %v", corpus, err)
	}
	// The corpus holds 84 operational templates; fewer means the walk lost some.
	if len(paths) < 84 {
		t.Fatalf("found %d operational templates under %s; want at least 84", len(paths), corpus)
	}
	for _, path := range paths {
		name, _ := filepath.Rel(corpus, path)
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			var ot aom14.OperationalTemplate
			if err := canxml.Unmarshal(raw, &ot); err != nil {
				t.Fatalf("canxml.Unmarshal = %v", err)
			}
			b, err := canjson.Marshal(&ot)
			if err != nil {
				t.Fatalf("canjson.Marshal = %v", err)
			}
			var back aom14.OperationalTemplate
			if err := canjson.Unmarshal(b, &back, canjson.WithStrict()); err != nil {
				t.Fatalf("canjson.Unmarshal = %v", err)
			}
			if where := firstDifference(t, "", reflect.ValueOf(&ot), reflect.ValueOf(&back)); where != "" {
				t.Errorf("the canonical JSON decodes to another value than the one written, first at %s", where)
			}
		})
	}
}

var typedStringType = reflect.TypeFor[*aom14.TypedString]()

// firstDifference returns the Go path of the first place a and b, two values
// of one type, differ, or "" when they do not: a *aom14.TypedString compares by
// its text and the primitive its Type reads as ([typedPrimitive]), any other
// value as reflect.DeepEqual compares it.
func firstDifference(t *testing.T, path string, a, b reflect.Value) string {
	t.Helper()
	if a.Type() != b.Type() {
		return path + fmt.Sprintf(" (%s against %s)", a.Type(), b.Type())
	}
	switch a.Kind() {
	case reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				return path
			}
			return ""
		}
		if a.Type() == typedStringType {
			x, y := a.Interface().(*aom14.TypedString), b.Interface().(*aom14.TypedString)
			if x.Value != y.Value || typedPrimitive(t, x) != typedPrimitive(t, y) {
				return path + fmt.Sprintf(" (%#v against %#v)", *x, *y)
			}
			return ""
		}
		return firstDifference(t, path, a.Elem(), b.Elem())
	case reflect.Interface:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				return path
			}
			return ""
		}
		return firstDifference(t, path, a.Elem(), b.Elem())
	case reflect.Struct:
		for i := range a.NumField() {
			if where := firstDifference(t, path+"."+a.Type().Field(i).Name, a.Field(i), b.Field(i)); where != "" {
				return where
			}
		}
		return ""
	case reflect.Slice, reflect.Array:
		if a.Kind() == reflect.Slice && (a.IsNil() != b.IsNil() || a.Len() != b.Len()) {
			return path + fmt.Sprintf(" (length %d against %d)", a.Len(), b.Len())
		}
		for i := range a.Len() {
			if where := firstDifference(t, fmt.Sprintf("%s[%d]", path, i), a.Index(i), b.Index(i)); where != "" {
				return where
			}
		}
		return ""
	case reflect.Map:
		if a.IsNil() != b.IsNil() || a.Len() != b.Len() {
			return path + fmt.Sprintf(" (%d entries against %d)", a.Len(), b.Len())
		}
		for _, k := range a.MapKeys() {
			bv := b.MapIndex(k)
			if !bv.IsValid() {
				return fmt.Sprintf("%s[%v] (missing)", path, k)
			}
			if where := firstDifference(t, fmt.Sprintf("%s[%v]", path, k), a.MapIndex(k), bv); where != "" {
				return where
			}
		}
		return ""
	}
	if !reflect.DeepEqual(a.Interface(), b.Interface()) {
		return path + fmt.Sprintf(" (%v against %v)", a.Interface(), b.Interface())
	}
	return ""
}

// typedPrimitive returns the BMM primitive the Type of s reads as, the
// `_type` canonical JSON writes for it.
func typedPrimitive(t *testing.T, s *aom14.TypedString) string {
	t.Helper()
	b, err := canjson.Marshal(s)
	if err != nil {
		t.Fatalf("canjson.Marshal(%#v) = %v", *s, err)
	}
	var head struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		t.Fatalf("reading %s: %v", b, err)
	}
	return head.Type
}

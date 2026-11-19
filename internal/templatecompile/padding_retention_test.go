package templatecompile_test

// padding_retention_test.go: a compiled template, and a Web Template built
// from it, keep none of the white space an OPT writes around a text, once the
// parsed model is dropped. The model keeps each text as the OPT writes it;
// the compile trims it, and a trimmed string that shares its bytes with the
// model's would keep the whole padded text alive for as long as the template
// is kept.

import (
	"bytes"
	"maps"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unsafe"
	"weak"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// pad is the run of spaces written on each side of a padded text. Its size
// does not decide the test, which follows each padded string by a weak
// pointer rather than by the size of the heap.
var pad = strings.Repeat(" ", 1024)

var (
	// paddedElements matches an element whose content is text alone, for the
	// elements the compile reads as strings: a term's texts (items), an
	// object's RM type and node id, an attribute's name, codes, units,
	// patterns, a slot's expression and the parts of its tree, a constraint
	// reference, the concept, and every value, the template id, uid,
	// archetype ids and terminology ids among them.
	paddedElements = regexp.MustCompile(`(<(?:code_list|code_string|concept|item|items|node_id|pattern|reference|reference_type|rm_attribute_name|rm_type_name|string_expression|type|units|value)(?:\s[^>]*)?>)([^<]+)(</)`)
	// paddedAttributes matches the attributes the compile reads: a term's
	// code and item keys, and a term binding's terminology and code.
	paddedAttributes = regexp.MustCompile(`( (?:code|id|terminology)=")([^"]*)(")`)
)

// padTexts returns the OPT opt with pad written on each side of each text
// paddedElements and paddedAttributes match.
func padTexts(opt []byte) []byte {
	repl := []byte("${1}" + pad + "${2}" + pad + "${3}")
	return paddedAttributes.ReplaceAll(paddedElements.ReplaceAll(opt, repl), repl)
}

// paddedText is one padded string of the model: the name of the field or map
// that holds it and its text without the padding, both for a failure message,
// and a weak pointer to its bytes, cleared once nothing keeps them.
type paddedText struct {
	field string
	text  string
	bytes weak.Pointer[byte]
}

// collectPadded appends to out each string that v holds, through pointers,
// interfaces, slices, arrays, maps and struct fields, and that opens with
// pad. field names the field or map v is held in. seen holds the pointers
// already followed.
func collectPadded(v reflect.Value, field string, seen map[uintptr]bool, out *[]paddedText) {
	switch v.Kind() {
	case reflect.String:
		if s := v.String(); strings.HasPrefix(s, pad) {
			// The text is a copy: a slice of s would keep s alive.
			*out = append(*out, paddedText{field: field, text: strings.Clone(strings.TrimSpace(s)), bytes: weak.Make(unsafe.StringData(s))})
		}
	case reflect.Pointer:
		if v.IsNil() || seen[v.Pointer()] {
			return
		}
		seen[v.Pointer()] = true
		collectPadded(v.Elem(), field, seen, out)
	case reflect.Interface:
		if !v.IsNil() {
			collectPadded(v.Elem(), field, seen, out)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			collectPadded(v.Index(i), field, seen, out)
		}
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			collectPadded(it.Key(), field+" key", seen, out)
			collectPadded(it.Value(), field, seen, out)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			collectPadded(v.Field(i), v.Type().Field(i).Name, seen, out)
		}
	}
}

// compilePadded reads the corpus template id with its texts padded
// ([padTexts]) and compiles it. It returns the compiled template and the
// padded strings of the model, which is unreachable once it returns.
func compilePadded(t *testing.T, id string) (*templatecompile.Compiled, []paddedText) {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	model, err := aom14.ReadOperationalTemplate(bytes.NewReader(padTexts(raw)))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate(%s, texts padded): %v", id, err)
	}
	var texts []paddedText
	collectPadded(reflect.ValueOf(model), "OperationalTemplate", map[uintptr]bool{}, &texts)
	if len(texts) == 0 {
		t.Fatalf("the model of %s holds no padded text: the padding reached no text", id)
	}
	c, err := templatecompile.Compile(model)
	if err != nil {
		t.Fatalf("Compile(%s, texts padded): %v", id, err)
	}
	if c.TemplateID() != id {
		t.Errorf("Compile(%s, texts padded).TemplateID() = %q; want %q, the id without its padding", id, c.TemplateID(), id)
	}
	return c, texts
}

// reportKept fails t for each field of which holder keeps a padded string
// alive, after a collection.
func reportKept(t *testing.T, holder string, texts []paddedText) {
	t.Helper()
	runtime.GC()
	kept := map[string][]string{}
	for _, p := range texts {
		if p.bytes.Value() != nil {
			kept[p.field] = append(kept[p.field], p.text)
		}
	}
	for _, field := range slices.Sorted(maps.Keys(kept)) {
		t.Errorf("%s keeps the padded model text of %d of %s, %q the first: want a trimmed copy that does not keep the padding", holder, len(kept[field]), field, kept[field][0])
	}
}

// TestCompiledTemplateKeepsNoPadding pins that a compiled template, and a Web
// Template built from it and kept on its own, keep no padded text of the
// model alive once the model is dropped: each text the compile keeps
// trimmed is a copy, not a slice of the model's padded string.
func TestCompiledTemplateKeepsNoPadding(t *testing.T) {
	for _, id := range []string{"BMI", "Demonstration.v1", "clinical_notes.v0"} {
		t.Run(id, func(t *testing.T) {
			wt, texts := func() (*webtemplate.WebTemplate, []paddedText) {
				c, texts := compilePadded(t, id)
				reportKept(t, "the compiled template", texts)
				wt, err := webtemplate.Build(c)
				if err != nil {
					t.Fatalf("webtemplate.Build(%s, texts padded): %v", id, err)
				}
				return wt, texts
			}()
			reportKept(t, "a Web Template kept without its compiled template", texts)
			runtime.KeepAlive(wt)
		})
	}
}

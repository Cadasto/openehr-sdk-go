package instance_test

import (
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The vendored OPTs whose one ELEMENT value is a DV_TEXT or DV_PARSABLE
// carrying a C_STRING on one of its String attributes.
const (
	textPatternOPT   = "Test_dv_text_pattern_constraint.v0"
	textListOPT      = "Test_dv_text_list_constraint.v0"
	parsableOpenOPT  = "Test_dv_parsable_open_constraint.v0"
	textValueListXYZ = "<list>XYZ</list>"
)

// editOPT reads the vendored OPT called name and replaces the one
// occurrence of old with repl.
func editOPT(t *testing.T, name, old, repl string) string {
	t.Helper()
	opt := instance.ReadVendoredOPT(t, name)
	if n := strings.Count(opt, old); n != 1 {
		t.Fatalf("%s: %q occurs %d times, want once", name, old, n)
	}
	return strings.Replace(opt, old, repl, 1)
}

// renameAttrBefore renames the last <rm_attribute_name>from</...> that
// precedes anchor in opt to to.
func renameAttrBefore(t *testing.T, opt, anchor, from, to string) string {
	t.Helper()
	at := strings.Index(opt, anchor)
	if at < 0 {
		t.Fatalf("OPT has no %q", anchor)
	}
	tag := "<rm_attribute_name>" + from + "</rm_attribute_name>"
	i := strings.LastIndex(opt[:at], tag)
	if i < 0 {
		t.Fatalf("OPT has no %s attribute before %q", from, anchor)
	}
	return opt[:i] + "<rm_attribute_name>" + to + "</rm_attribute_name>" + opt[i+len(tag):]
}

// elementValue returns the canonical-JSON value of the first ELEMENT
// whose archetype_node_id is nodeID.
func elementValue(t *testing.T, doc any, nodeID string) map[string]any {
	t.Helper()
	var found map[string]any
	var walk func(v any)
	walk = func(v any) {
		if found != nil {
			return
		}
		switch n := v.(type) {
		case map[string]any:
			if n["_type"] == "ELEMENT" && n["archetype_node_id"] == nodeID {
				found, _ = n["value"].(map[string]any)
				return
			}
			for _, child := range n {
				walk(child)
			}
		case []any:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(doc)
	if found == nil {
		t.Fatalf("no ELEMENT %s with a value in the generated composition", nodeID)
	}
	return found
}

// validateClean fails t when the template-driven validator or the RM
// floor reports an error on root.
func validateClean(t *testing.T, root any, c *templatecompile.Compiled) {
	t.Helper()
	comp, err := instance.AsComposition(root)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}
	for _, r := range []struct {
		name string
		res  validation.Result
	}{
		{"ValidateComposition", validation.ValidateComposition(comp, c)},
		{"ValidateRM", validation.ValidateRM(comp)},
	} {
		for _, iss := range r.res.Issues {
			if iss.Severity == validation.Error {
				t.Errorf("%s: %s @ %s: %s", r.name, iss.Code, iss.Path, iss.Detail)
			}
		}
	}
}

// stringLeafFills are the value fills each STRING leaf test runs under:
// ExampleFill, and RandomFill on a few fixed seeds.
func stringLeafFills() []instance.Options {
	fills := []instance.Options{intervalOptions(instance.Example)}
	for seed := range uint64(5) {
		o := intervalOptions(instance.Example)
		o.ValueFill = instance.RandomFill
		o.ValueSource = mrand.NewPCG(seed, 3)
		fills = append(fills, o)
	}
	return fills
}

// TestREQ107_StringLeafPatternOnDVTextValue is the REQ-107 / REQ-103 check
// that a C_STRING pattern on DV_TEXT.value is satisfied: the generator
// writes a value the pattern matches, not the open-string "example".
func TestREQ107_StringLeafPatternOnDVTextValue(t *testing.T) {
	c := compileSyntheticOPT(t, editOPT(t, textPatternOPT, textValueListXYZ, "<pattern>XYZ[0-9]+</pattern>"))
	full := regexp.MustCompile(`^XYZ[0-9]+$`)
	for _, opts := range stringLeafFills() {
		root, doc := generateWithJSON(t, c, opts)
		got, _ := elementValue(t, doc, "at0031")["value"].(string)
		if !full.MatchString(got) {
			t.Errorf("ValueFill %v: DV_TEXT.value = %q, want a full match of XYZ[0-9]+", opts.ValueFill, got)
		}
		validateClean(t, root, c)
	}
}

// TestREQ107_StringLeafListOnDVParsableFormalism is the REQ-107 / REQ-103
// check that a C_STRING list on DV_PARSABLE.formalism is honoured instead
// of the "text/plain" default.
func TestREQ107_StringLeafListOnDVParsableFormalism(t *testing.T) {
	c := compileSyntheticOPT(t, editOPT(t, parsableOpenOPT, "<list>text/plain</list>", "<list>application/json</list>"))
	for _, opts := range stringLeafFills() {
		root, doc := generateWithJSON(t, c, opts)
		v := elementValue(t, doc, "at0028")
		if v["_type"] != "DV_PARSABLE" {
			t.Fatalf("ELEMENT at0028 value is %v, want DV_PARSABLE", v["_type"])
		}
		if got := v["formalism"]; got != "application/json" {
			t.Errorf("ValueFill %v: DV_PARSABLE.formalism = %v, want application/json", opts.ValueFill, got)
		}
		validateClean(t, root, c)
	}
}

// TestREQ107_StringLeafDispatchesOnAttribute is the REQ-107 check that a
// C_STRING lands only on the attribute the OPT names: a list on
// DV_TEXT.formatting never reaches value, and formatting, an optional
// attribute, is filled with a member of the list.
func TestREQ107_StringLeafDispatchesOnAttribute(t *testing.T) {
	opt := renameAttrBefore(t, instance.ReadVendoredOPT(t, textListOPT), textValueListXYZ, "value", "formatting")
	c := compileSyntheticOPT(t, opt)
	for _, opts := range stringLeafFills() {
		root, doc := generateWithJSON(t, c, opts)
		v := elementValue(t, doc, "at0031")
		if got := v["formatting"]; got != "XYZ" && got != "OPQ" {
			t.Errorf("ValueFill %v: DV_TEXT.formatting = %v, want a member of [XYZ OPQ]", opts.ValueFill, got)
		}
		if value := v["value"]; value == "XYZ" || value == "OPQ" {
			t.Errorf("ValueFill %v: DV_TEXT.value = %v, want the formatting list kept off value", opts.ValueFill, value)
		}
		validateClean(t, root, c)
	}
}

// stringLeafPath returns the OPT path of the one C_STRING leaf in c.
func stringLeafPath(t *testing.T, c *templatecompile.Compiled) string {
	t.Helper()
	var path string
	var walk func(n *templatecompile.CompiledNode)
	walk = func(n *templatecompile.CompiledNode) {
		if _, ok := n.PrimitiveConstraint().(constraints.CString); ok && path == "" {
			path = n.AQLPath()
		}
		for _, attr := range n.Attributes() {
			for _, child := range attr.Children() {
				walk(child)
			}
		}
	}
	walk(c.Root())
	if path == "" {
		t.Fatal("compiled OPT has no C_STRING leaf")
	}
	return path
}

// checkUnsatisfiable fails t unless err is the REQ-107 error for an
// unsatisfiable primitive constraint on DV_TEXT.value at path: it wraps
// ErrConstraintUnsatisfiable, not ErrSlotFillUnsupported, and names the
// RM type, the attribute and the path.
func checkUnsatisfiable(t *testing.T, call string, err error, path string) {
	t.Helper()
	if !errors.Is(err, instance.ErrConstraintUnsatisfiable) {
		t.Errorf("%s error = %v, want one wrapping ErrConstraintUnsatisfiable", call, err)
		return
	}
	if errors.Is(err, instance.ErrSlotFillUnsupported) {
		t.Errorf("%s error = %v, want it not to wrap ErrSlotFillUnsupported", call, err)
	}
	for _, part := range []string{"DV_TEXT", ".value", path} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("%s error = %q, want it to name %q", call, err, part)
		}
	}
}

// TestREQ107_StringLeafUnsatisfiableErrors is the REQ-107 check that the
// generator returns ErrConstraintUnsatisfiable, naming the RM type, the
// attribute and the OPT path, and writes nothing, when no string
// satisfies a C_STRING. The composition builder passes the error on.
func TestREQ107_StringLeafUnsatisfiableErrors(t *testing.T) {
	cases := []struct {
		name string
		repl string
	}{
		{name: "no list member matches the pattern", repl: "<list>XYZ</list><pattern>[0-9]+</pattern>"},
		{name: "pattern matches nothing", repl: `<pattern>[^\x00-\x{10FFFF}]</pattern>`},
		{name: "pattern is not a valid regex", repl: "<pattern>XYZ(</pattern>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileSyntheticOPT(t, editOPT(t, textPatternOPT, textValueListXYZ, tc.repl))
			path := stringLeafPath(t, c)
			for _, opts := range stringLeafFills() {
				out, err := instance.Generate(t.Context(), c, opts)
				checkUnsatisfiable(t, fmt.Sprintf("Generate(%v)", opts.ValueFill), err, path)
				if out != nil {
					t.Errorf("Generate(%v) returned %T with the error, want no value", opts.ValueFill, out)
				}
			}
			b, err := composition.NewBuilder(t.Context(), c,
				composition.WithTerritory("NL"), composition.WithComposer(testComposer()))
			checkUnsatisfiable(t, "composition.NewBuilder", err, path)
			if b != nil {
				t.Error("composition.NewBuilder returned a builder with the error, want nil")
			}
		})
	}
}

// TestREQ107_StringLeafFillsOptionalIdentifierSubfields is the REQ-107 /
// REQ-103 check that the optional DV_IDENTIFIER attributes issuer, type and
// assigner are filled with a value their XYZ.* pattern accepts, and that
// the result passes both the template validator and the RM floor.
func TestREQ107_StringLeafFillsOptionalIdentifierSubfields(t *testing.T) {
	c := compileSyntheticOPT(t, instance.ReadVendoredOPT(t, "Test_dv_identifier_pattern_constraint.v0"))
	match := regexp.MustCompile(`^XYZ.*$`)
	for _, opts := range stringLeafFills() {
		root, doc := generateWithJSON(t, c, opts)
		v := elementValue(t, doc, "at0030")
		for _, attr := range []string{"issuer", "type", "assigner"} {
			if got, _ := v[attr].(string); !match.MatchString(got) {
				t.Errorf("ValueFill %v: DV_IDENTIFIER.%s = %v, want a match of XYZ.*", opts.ValueFill, attr, v[attr])
			}
		}
		validateClean(t, root, c)
	}
}

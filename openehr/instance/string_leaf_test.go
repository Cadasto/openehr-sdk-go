package instance_test

import (
	"errors"
	mrand "math/rand/v2"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
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
// C_STRING lands on the attribute the OPT names: a list on
// DV_TEXT.formatting fills formatting and leaves value alone.
func TestREQ107_StringLeafDispatchesOnAttribute(t *testing.T) {
	opt := renameAttrBefore(t, instance.ReadVendoredOPT(t, textListOPT), textValueListXYZ, "value", "formatting")
	c := compileSyntheticOPT(t, opt)
	for _, opts := range stringLeafFills() {
		root, doc := generateWithJSON(t, c, opts)
		v := elementValue(t, doc, "at0031")
		got, _ := v["formatting"].(string)
		if got != "XYZ" && got != "OPQ" {
			t.Errorf("ValueFill %v: DV_TEXT.formatting = %v, want a member of [XYZ OPQ]", opts.ValueFill, v["formatting"])
		}
		if value := v["value"]; value == "XYZ" || value == "OPQ" {
			t.Errorf("ValueFill %v: DV_TEXT.value = %v, want the formatting list kept off value", opts.ValueFill, value)
		}
		comp, err := instance.AsComposition(root)
		if err != nil {
			t.Fatalf("AsComposition: %v", err)
		}
		for _, iss := range validation.ValidateRM(comp).Issues {
			if iss.Severity == validation.Error {
				t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
			}
		}
	}
}

// TestREQ107_StringLeafUnsatisfiableErrors is the REQ-107 check that the
// generator returns an error, and does not write a value the constraint
// rejects, when no string satisfies a C_STRING.
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
			for _, opts := range stringLeafFills() {
				_, err := instance.Generate(t.Context(), c, opts)
				if !errors.Is(err, instance.ErrSlotFillUnsupported) {
					t.Errorf("ValueFill %v: Generate error = %v, want one wrapping ErrSlotFillUnsupported", opts.ValueFill, err)
				}
			}
		})
	}
}

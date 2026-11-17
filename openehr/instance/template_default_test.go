package instance_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// commentPath is the differential path of the ELEMENT at0001, named
// Comment, that optCommentValue's CLUSTER holds.
const commentPath = "[openEHR-EHR-CLUSTER.rm_defaults.v1]/items[at0001]"

// withDefault adds to opt a <constraints> section whose one T_ATTRIBUTE
// sets on the value of the ELEMENT at commentPath a default value of
// rmType, written as body, the children of its default_value.
func withDefault(opt, rmType, body string) string {
	constraint := `<constraints><attributes><rm_attribute_name>value</rm_attribute_name><children>` +
		`<rm_type_name>` + rmType + `</rm_type_name><node_id></node_id>` +
		`<default_value xsi:type="` + rmType + `">` + body + `</default_value></children>` +
		`<differential_path>` + commentPath + `</differential_path></attributes></constraints>`
	return strings.Replace(opt, "</definition>", "</definition>"+constraint, 1)
}

// codedBody is the body of a DV_CODED_TEXT default value of text, coded
// code in local.
func codedBody(text, code string) string {
	return `<value>` + text + `</value><defining_code><terminology_id><value>local</value></terminology_id>` +
		`<code_string>` + code + `</code_string></defining_code>`
}

// ordinalBody is the body of a DV_ORDINAL default value of value, whose
// symbol is text, coded code in local.
func ordinalBody(value, text, code string) string {
	return `<value>` + value + `</value><symbol>` + codedBody(text, code) + `</symbol>`
}

// TestREQ107_TemplateDefaultValueStandsInForTheFill is the REQ-107 Trust
// model check on the default value a template's <constraints> set on a
// node: under every policy and both value fills the generator writes it in
// place of the value the fill gives, where it validates under the node's
// constraints, and keeps the fill's value where it does not: a C_STRING,
// C_INTEGER or C_CODE_PHRASE on an attribute, or a C_DV_ORDINAL or a
// C_CODE_PHRASE on the node itself, that refuses it, an attribute the node
// requires that it leaves out, or one the node prohibits that it carries.
// A default that keeps the RM rules the walk settles its own value by is
// written too: a text whose language is an ISO 639-1 code, a percent over
// 100 and a fraction of whole numbers at precision 0. The RM floor and the
// template validator pass.
func TestREQ107_TemplateDefaultValueStandsInForTheFill(t *testing.T) {
	text := func(attrs ...string) string { return optNode("DV_TEXT", "", attrs...) }
	list := optSingle("value", optPrimitive("STRING", "C_STRING", "<list>Mild</list><list>Severe</list>"))
	count := optNode("DV_COUNT", "", optSingle("magnitude", integerFrom(0, 10)))
	coded := optCodedText("local", "at0002", "at0003")
	language := optSingle("language", optCodePhrase("ISO_639-1", "en"))
	cases := []struct {
		name  string
		value string
		// rmType and body give the default value.
		rmType, body string
		// want reports whether got, the value written as commentText
		// renders it, is the one the case asks for, which about names.
		want  func(fill instance.ValueFill, got string) bool
		about string
	}{
		{
			name: "DV_TEXT left open", value: text(), rmType: "DV_TEXT", body: "<value>Leeds EPCCS</value>",
			want: func(_ instance.ValueFill, got string) bool { return got == "Leeds EPCCS" }, about: "the default Leeds EPCCS",
		},
		{
			name: "DV_TEXT whose C_STRING lists the default", value: text(list), rmType: "DV_TEXT", body: "<value>Severe</value>",
			want: func(_ instance.ValueFill, got string) bool { return got == "Severe" }, about: "the default Severe",
		},
		{
			name: "DV_TEXT whose C_STRING refuses the default", value: text(list), rmType: "DV_TEXT", body: "<value>Moderate</value>",
			want: func(fill instance.ValueFill, got string) bool {
				return got == "Mild" || (fill == instance.RandomFill && got == "Severe")
			},
			about: "a member of the list, not the default Moderate",
		},
		{
			name: "DV_COUNT whose C_INTEGER admits the default", value: count, rmType: "DV_COUNT", body: "<magnitude>7</magnitude>",
			want: func(_ instance.ValueFill, got string) bool { return got == "7" }, about: "the default 7",
		},
		{
			name: "DV_COUNT whose C_INTEGER refuses the default", value: count, rmType: "DV_COUNT", body: "<magnitude>70</magnitude>",
			want: func(_ instance.ValueFill, got string) bool { return got != "70" }, about: "a magnitude from 0 to 10, not the default 70",
		},
		{
			name: "DV_CODED_TEXT whose C_CODE_PHRASE lists the default's code", value: coded, rmType: "DV_CODED_TEXT", body: codedBody("Second", "at0003"),
			want: func(_ instance.ValueFill, got string) bool { return got == "Second local::at0003" }, about: "the default Second local::at0003",
		},
		{
			name: "DV_CODED_TEXT whose C_CODE_PHRASE refuses the default's code", value: coded, rmType: "DV_CODED_TEXT", body: codedBody("Ninth", "at0009"),
			want: func(_ instance.ValueFill, got string) bool { return !strings.Contains(got, "at0009") }, about: "a listed code, not the default's at0009",
		},
		{
			name: "DV_ORDINAL whose C_DV_ORDINAL lists the default", value: ordinalPairs("1/at0002", "2/at0003"),
			rmType: "DV_ORDINAL", body: ordinalBody("2", "Second", "at0003"),
			want: func(_ instance.ValueFill, got string) bool { return got == "2 Second local::at0003" }, about: "the default 2 Second local::at0003",
		},
		{
			name: "DV_ORDINAL whose C_DV_ORDINAL refuses the default", value: ordinalPairs("1/at0002", "2/at0003"),
			rmType: "DV_ORDINAL", body: ordinalBody("2", "Second", "at0002"),
			want: func(_ instance.ValueFill, got string) bool { return !strings.HasPrefix(got, "2 Second local::at0002") }, about: "a listed pair, not the default's 2 coded at0002",
		},
		{
			name: "DV_CODED_TEXT a C_CODE_PHRASE constrains as a whole, listing the default's code", value: optCodedTextWhole("local", "at0002", "at0003"),
			rmType: "DV_CODED_TEXT", body: codedBody("Second", "at0003"),
			want: func(_ instance.ValueFill, got string) bool { return got == "Second local::at0003" }, about: "the default Second local::at0003",
		},
		{
			name: "DV_CODED_TEXT a C_CODE_PHRASE constrains as a whole, refusing the default's code", value: optCodedTextWhole("local", "at0002", "at0003"),
			rmType: "DV_CODED_TEXT", body: codedBody("Ninth", "at0009"),
			want: func(_ instance.ValueFill, got string) bool { return !strings.Contains(got, "at0009") }, about: "a listed code, not the default's at0009",
		},
		{
			name: "DV_TEXT whose template requires a language the default leaves out", value: text(language), rmType: "DV_TEXT", body: "<value>Leeds EPCCS</value>",
			want: func(_ instance.ValueFill, got string) bool { return got != "Leeds EPCCS" }, about: "the fill's text, not the default",
		},
		{
			name: "DV_TEXT whose template prohibits a language the default carries", value: text(optProhibitedSingle("language")), rmType: "DV_TEXT",
			body: `<value>Leeds EPCCS</value><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>`,
			want: func(_ instance.ValueFill, got string) bool { return got != "Leeds EPCCS" }, about: "the fill's text, not the default",
		},
		{
			name: "DV_TEXT left open whose language is an ISO 639-1 code", value: text(), rmType: "DV_TEXT",
			body: `<value>Leeds EPCCS</value><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>`,
			want: func(_ instance.ValueFill, got string) bool { return got == "Leeds EPCCS" }, about: "the default Leeds EPCCS",
		},
		{
			name: "DV_PROPORTION left open, a percent over 100", value: optNode("DV_PROPORTION", ""), rmType: "DV_PROPORTION",
			body:  "<numerator>10.0</numerator><denominator>100.0</denominator><type>2</type>",
			want:  func(_ instance.ValueFill, got string) bool { return got == "10/100 type 2" },
			about: "the default 10/100 type 2",
		},
		{
			name: "DV_PROPORTION left open, a fraction of whole numbers at precision 0", value: optNode("DV_PROPORTION", ""), rmType: "DV_PROPORTION",
			body:  "<numerator>1.0</numerator><denominator>3.0</denominator><type>3</type><precision>0</precision>",
			want:  func(_ instance.ValueFill, got string) bool { return got == "1/3 type 3" },
			about: "the default 1/3 type 3",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eachSetting(t, withDefault(optCommentValue(tc.value), tc.rmType, tc.body), func(t *testing.T, opts instance.Options, out any) {
				if got := commentText(t, out); !tc.want(opts.ValueFill, got) {
					t.Errorf("ELEMENT.value = %q, want %s", got, tc.about)
				}
			})
		})
	}
}

// TestREQ107_TemplateDefaultThatBreaksAnRMRuleLeavesTheFill is the REQ-107
// Output check on a template default that validates under its node's
// constraints but breaks an RM rule the RM floor reads: under every policy
// and both value fills the generator keeps the value the walk built, so the
// root passes the RM floor and the template validator. The defaults leave
// an attribute the BMM marks mandatory empty (a DV_TEXT's value, the code
// string of a DV_ORDINAL's symbol), or break a rule the walk settles its
// own value by (a DV_PROPORTION's Valid_denominator and Percent_validity, a
// DV_TEXT's Language_valid).
func TestREQ107_TemplateDefaultThatBreaksAnRMRuleLeavesTheFill(t *testing.T) {
	text := optNode("DV_TEXT", "")
	proportion := optNode("DV_PROPORTION", "")
	cases := []struct {
		name  string
		value string
		// rmType and body give the default value.
		rmType, body string
		// fault is the default as commentText renders it, which the
		// generator must not write.
		fault string
	}{
		{name: "DV_TEXT whose value is empty", value: text, rmType: "DV_TEXT", body: "<value></value>", fault: ""},
		{
			name: "DV_PROPORTION whose denominator is 0", value: proportion, rmType: "DV_PROPORTION",
			body: "<numerator>1.0</numerator><denominator>0.0</denominator><type>0</type>", fault: "1/0 type 0",
		},
		{
			name: "DV_PROPORTION, a percent whose denominator is not 100", value: proportion, rmType: "DV_PROPORTION",
			body: "<numerator>10.0</numerator><denominator>50.0</denominator><type>2</type>", fault: "10/50 type 2",
		},
		{
			name: "DV_ORDINAL whose symbol has no code string", value: optNode("DV_ORDINAL", ""), rmType: "DV_ORDINAL",
			body: ordinalBody("1", "First", ""), fault: "1 First local::",
		},
		{
			name: "DV_TEXT whose language is no ISO 639-1 code", value: text, rmType: "DV_TEXT",
			body:  `<value>Leeds EPCCS</value><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>xx</code_string></language>`,
			fault: "Leeds EPCCS",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eachSetting(t, withDefault(optCommentValue(tc.value), tc.rmType, tc.body), func(t *testing.T, _ instance.Options, out any) {
				if got := commentText(t, out); got == tc.fault {
					t.Errorf("ELEMENT.value = %q, the default that breaks an RM rule; want the value the walk built", got)
				}
			})
		})
	}
}

// commentText renders the value of the one ELEMENT under the CLUSTER root
// out as the check above compares it: a text's value, a coded text's value
// and code, a count's magnitude, an ordinal's value, symbol text and code,
// or a proportion's numerator, denominator and type.
func commentText(t *testing.T, out any) string {
	t.Helper()
	cluster, ok := out.(*rm.Cluster)
	if !ok || len(cluster.Items) != 1 {
		t.Fatalf("generated root is %T, want a CLUSTER holding one item", out)
	}
	element, ok := cluster.Items[0].(*rm.Element)
	if !ok {
		t.Fatalf("CLUSTER.items[0] is %T, want *rm.Element", cluster.Items[0])
	}
	switch v := element.Value.(type) {
	case *rm.DVCodedText:
		return v.Value + " " + v.DefiningCode.TerminologyID.Value + "::" + v.DefiningCode.CodeString
	case *rm.DVText:
		return v.Value
	case *rm.DVCount:
		return fmt.Sprint(v.Magnitude)
	case *rm.DVOrdinal:
		code := v.Symbol.DefiningCode
		return fmt.Sprintf("%d %s %s::%s", v.Value, v.Symbol.Value, code.TerminologyID.Value, code.CodeString)
	case *rm.DVProportion:
		return fmt.Sprintf("%v/%v type %d", v.Numerator, v.Denominator, v.Type)
	}
	t.Fatalf("ELEMENT.value is %T, want a DV_TEXT, a DV_CODED_TEXT, a DV_COUNT, a DV_ORDINAL or a DV_PROPORTION", element.Value)
	return ""
}

// TestREQ107_CorpusTemplateDefaultValuesAreWritten is the REQ-107 Trust
// model check on a corpus template whose <constraints> set default values
// of seven data types, each of which validates under its node: under Large,
// which builds every node, and both value fills, each value at the path of
// a node that carries a default is that default, and the root passes the RM
// floor and the template validator.
func TestREQ107_CorpusTemplateDefaultValuesAreWritten(t *testing.T) {
	opt, err := readOPTFile(filepath.Join(fixtures.CorpusRoot(), "webtemplate", "constrain_test.opt"))
	if err != nil {
		t.Fatalf("readOPTFile: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	var nodes []*templatecompile.CompiledNode
	var collect func(n *templatecompile.CompiledNode)
	collect = func(n *templatecompile.CompiledNode) {
		if n.DefaultValue() != nil {
			nodes = append(nodes, n)
		}
		for _, attr := range n.Attributes() {
			for _, child := range attr.Children() {
				collect(child)
			}
		}
	}
	collect(c.Root())
	if len(nodes) != 7 {
		t.Fatalf("constrain_test.opt has %d nodes with a default value, want 7", len(nodes))
	}
	for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
		t.Run(fmt.Sprint(fill), func(t *testing.T) {
			out, err := instance.Generate(t.Context(), c, instance.Options{
				Policy: instance.Large, ValueFill: fill, Composer: testComposer(), Territory: "NL", Now: defaultsNow,
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			for _, n := range nodes {
				items, err := rmpath.ItemsAtPath(out.(rm.Locatable), n.AQLPath())
				if err != nil || len(items) == 0 {
					t.Errorf("%s: ItemsAtPath = %d items, %v; want the values Large builds", n.AQLPath(), len(items), err)
					continue
				}
				want := fmt.Sprintf("%+v", n.DefaultValue())
				for _, item := range items {
					if got := fmt.Sprintf("%+v", item); got != want {
						t.Errorf("%s = %s, want the default %s", n.AQLPath(), got, want)
					}
				}
			}
			checkValid(t, out, c)
		})
	}
}

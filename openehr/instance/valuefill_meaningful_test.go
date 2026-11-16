package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// commentTerm is the term definition of at0001, the ELEMENT the
// meaningful-value templates hold.
const commentTerm = `<term_definitions code="at0001"><items id="text">Comment</items></term_definitions>`

// optCommentValue is a CLUSTER template holding the ELEMENT at0001,
// named Comment, whose value is value.
func optCommentValue(value string) string {
	return optTemplate("CLUSTER", commentTerm, optMultiple("items", optNode("ELEMENT", "at0001", optSingle("value", value))))
}

// commentValue returns the value of the one ELEMENT under the CLUSTER root
// out, as T.
func commentValue[T any](t *testing.T, out any) T {
	t.Helper()
	cluster, ok := out.(*rm.Cluster)
	if !ok || len(cluster.Items) != 1 {
		t.Fatalf("generated root is %T, want a CLUSTER holding one item", out)
	}
	element, ok := cluster.Items[0].(*rm.Element)
	if !ok {
		t.Fatalf("CLUSTER.items[0] is %T, want *rm.Element", cluster.Items[0])
	}
	v, ok := element.Value.(T)
	if !ok {
		var zero T
		t.Fatalf("ELEMENT.value is %T, want %T", element.Value, zero)
	}
	return v
}

// eachSetting compiles opt, generates it under every policy and both value
// fills, and calls check with the settings and each root.
func eachSetting(t *testing.T, opt string, check func(t *testing.T, opts instance.Options, out any)) {
	t.Helper()
	c := compileOPTText(t, opt, true)
	for _, opts := range defaultsOptions() {
		t.Run(fmt.Sprintf("%v/%v", opts.Policy, opts.ValueFill), func(t *testing.T) {
			out, err := instance.Generate(t.Context(), c, opts)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			check(t, opts, out)
			checkValid(t, out, c)
		})
	}
}

// TestREQ107_PlainTextTakesTheNameOfItsLocatable is the REQ-107 Leaf
// values check on a plain DV_TEXT whose template leaves its value open:
// under both value fills it takes the name of its nearest enclosing
// locatable, an ELEMENT's name, rather than the open-string example, which
// it takes where that name is only the RM type of an ELEMENT the OPT gives
// no term. A C_STRING with a list or a pattern gives its own example,
// which wins; a C_STRING with neither leaves the value open. A
// DV_CODED_TEXT keeps the text its code gives. The RM floor and the
// template validator pass.
func TestREQ107_PlainTextTakesTheNameOfItsLocatable(t *testing.T) {
	text := func(attrs ...string) string { return optNode("DV_TEXT", "", attrs...) }
	value := func(item string) string {
		return optSingle("value", optPrimitive("STRING", "C_STRING", item))
	}
	cases := []struct {
		name  string
		value string
		want  func(fill instance.ValueFill, got string) bool
		about string
	}{
		{
			name:  "value left silent",
			value: text(),
			want:  func(_ instance.ValueFill, got string) bool { return got == "Comment" },
			about: "the ELEMENT's name Comment",
		},
		{
			name:  "C_STRING with neither a list nor a pattern",
			value: text(value("")),
			want:  func(_ instance.ValueFill, got string) bool { return got == "Comment" },
			about: "the ELEMENT's name Comment",
		},
		{
			name:  "C_STRING with a list",
			value: text(value("<list>Mild</list><list>Severe</list>")),
			want: func(fill instance.ValueFill, got string) bool {
				return got == "Mild" || (fill == instance.RandomFill && got == "Severe")
			},
			about: "a member of the list, the first under ExampleFill",
		},
		{
			name:  "C_STRING with a pattern",
			value: text(value("<pattern>[0-9]+</pattern>")),
			want:  func(_ instance.ValueFill, got string) bool { return got != "Comment" && got != "example" && got != "" },
			about: "the string the pattern builds",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eachSetting(t, optCommentValue(tc.value), func(t *testing.T, opts instance.Options, out any) {
				if got := commentValue[*rm.DVText](t, out).Value; !tc.want(opts.ValueFill, got) {
					t.Errorf("DV_TEXT.value = %q, want %s", got, tc.about)
				}
			})
		})
	}
	t.Run("ELEMENT with no term", func(t *testing.T) {
		// The OPT gives at0002 no term, so the ELEMENT is named after its
		// RM type, which names no concept: the text is the example.
		opt := optTemplate("CLUSTER", commentTerm, optMultiple("items", optNode("ELEMENT", "at0002", optSingle("value", text()))))
		eachSetting(t, opt, func(t *testing.T, _ instance.Options, out any) {
			if got := commentValue[*rm.DVText](t, out).Value; got != "example" {
				t.Errorf("DV_TEXT.value = %q, want example", got)
			}
		})
	})
	t.Run("coded text", func(t *testing.T) {
		eachSetting(t, optCommentValue(optCodedText("local", "at0001")), func(t *testing.T, _ instance.Options, out any) {
			if got := commentValue[*rm.DVCodedText](t, out).Value; got != "Comment" {
				// at0001's term text is Comment: the coded text takes its
				// code's term, not its ELEMENT's name.
				t.Errorf("DV_CODED_TEXT.value = %q, want its term text Comment", got)
			}
		})
		eachSetting(t, optCommentValue(optCodedText("local", "at0009")), func(t *testing.T, _ instance.Options, out any) {
			if got := commentValue[*rm.DVCodedText](t, out).Value; got == "Comment" {
				t.Errorf("DV_CODED_TEXT.value = %q, want the text the walk wrote for a code with no term, not the ELEMENT's name", got)
			}
		})
	})
}

// optParsableValue is an ELEMENT template whose value is a DV_PARSABLE
// with the given attributes.
func optParsableValue(attrs ...string) string {
	return optTemplate("ELEMENT", optSingle("value", optNode("DV_PARSABLE", "", attrs...)))
}

// stringAttr is the attribute name carrying a C_STRING item with body.
func stringAttr(name, body string) string {
	return optSingle(name, optPrimitive("STRING", "C_STRING", body))
}

// TestREQ107_ParsableContentFitsItsFormalism is the REQ-107 Leaf values
// check on a DV_PARSABLE whose template leaves its value open: under both
// value fills its value fits its formalism, <example/> for an XML one
// (text/xml, application/xml or a +xml type), {} for a JSON one
// (application/json or a +json type), and the example otherwise. A
// C_STRING on the value with a list or a pattern gives its own example,
// which wins. The RM floor and the template validator pass.
func TestREQ107_ParsableContentFitsItsFormalism(t *testing.T) {
	formalism := func(f string) string { return stringAttr("formalism", "<list>"+f+"</list>") }
	cases := []struct {
		name  string
		attrs []string
		want  string
	}{
		{name: "text/xml", attrs: []string{formalism("text/xml")}, want: "<example/>"},
		{name: "application/xml", attrs: []string{formalism("application/xml")}, want: "<example/>"},
		{name: "a +xml type", attrs: []string{formalism("application/fhir+xml")}, want: "<example/>"},
		{name: "application/json", attrs: []string{formalism("application/json")}, want: "{}"},
		{name: "a +json type", attrs: []string{formalism("application/fhir+json")}, want: "{}"},
		{name: "text/plain", attrs: []string{formalism("text/plain")}, want: "example"},
		{name: "formalism left silent", want: "example"},
		{name: "XML under a value C_STRING with neither a list nor a pattern", attrs: []string{formalism("text/xml"), stringAttr("value", "")}, want: "<example/>"},
		{name: "XML under a value C_STRING with a list", attrs: []string{formalism("text/xml"), stringAttr("value", "<list>&lt;note/&gt;</list>")}, want: "<note/>"},
		{name: "XML under a value C_STRING with a pattern", attrs: []string{formalism("text/xml"), stringAttr("value", "<pattern>[a-z]+</pattern>")}, want: "example"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eachSetting(t, optParsableValue(tc.attrs...), func(t *testing.T, _ instance.Options, out any) {
				if got := rootElementValue[*rm.DVParsable](t, out); got.Value != tc.want {
					t.Errorf("DV_PARSABLE of formalism %q has value %q, want %q", got.Formalism, got.Value, tc.want)
				}
			})
		})
	}
}

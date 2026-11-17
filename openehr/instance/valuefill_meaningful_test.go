package instance_test

import (
	"fmt"
	mrand "math/rand/v2"
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
// values check on a plain DV_TEXT that is an ELEMENT's value and whose
// template leaves its value open: under both value fills it takes the
// ELEMENT's name rather than the open-string example, which it takes where
// that name is only the RM type of an ELEMENT the OPT gives no term. A
// value attribute the template names with no child is built as such a
// DV_TEXT too. A C_STRING with a list or a pattern gives its own example,
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
			name:  "value attribute with no child",
			value: "",
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
	t.Run("name the template pins", func(t *testing.T) {
		// The template renames the ELEMENT Pinned. Where it lists the
		// value before the name, as most renamed corpus ELEMENTs do, the
		// open value takes the name the walk writes after it, not the term
		// Comment; a C_STRING's own example and a template default stay.
		pinned := optSingle("name", text(value("<list>Pinned</list>")))
		renamed := func(valueFirst bool, attrs ...string) string {
			if valueFirst {
				attrs = append(attrs, pinned)
			} else {
				attrs = append([]string{pinned}, attrs...)
			}
			return optTemplate("CLUSTER", commentTerm, optMultiple("items", optNode("ELEMENT", "at0001", attrs...)))
		}
		renamedCases := []struct {
			name string
			opt  string
			want func(fill instance.ValueFill, got string) bool
		}{
			{
				name: "name before value left silent",
				opt:  renamed(false, optSingle("value", text())),
				want: func(_ instance.ValueFill, got string) bool { return got == "Pinned" },
			},
			{
				name: "value left silent before name",
				opt:  renamed(true, optSingle("value", text())),
				want: func(_ instance.ValueFill, got string) bool { return got == "Pinned" },
			},
			{
				name: "value attribute with no child before name",
				opt:  renamed(true, optSingle("value")),
				want: func(_ instance.ValueFill, got string) bool { return got == "Pinned" },
			},
			{
				name: "open C_STRING before name",
				opt:  renamed(true, optSingle("value", text(value("")))),
				want: func(_ instance.ValueFill, got string) bool { return got == "Pinned" },
			},
			{
				name: "C_STRING with a list before name",
				opt:  renamed(true, optSingle("value", text(value("<list>Comment</list><list>Severe</list>")))),
				want: func(fill instance.ValueFill, got string) bool {
					return got == "Comment" || (fill == instance.RandomFill && got == "Severe")
				},
			},
			{
				name: "template default before name",
				opt:  withDefault(renamed(true, optSingle("value", text())), "DV_TEXT", "<value>Leeds EPCCS</value>"),
				want: func(_ instance.ValueFill, got string) bool { return got == "Leeds EPCCS" },
			},
		}
		for _, rc := range renamedCases {
			t.Run(rc.name, func(t *testing.T) {
				eachSetting(t, rc.opt, func(t *testing.T, opts instance.Options, out any) {
					if got := commentValue[*rm.DVText](t, out).Value; !rc.want(opts.ValueFill, got) {
						t.Errorf("DV_TEXT.value = %q, want the case's %s", got, rc.name)
					}
				})
			})
		}
	})
	t.Run("value the RM rule needs where the template prohibits null_flavour", func(t *testing.T) {
		// The template names no value and prohibits the null flavour, so
		// the value Inv_null_flavour_indicated needs is a plain DV_TEXT
		// built from its BMM type, which takes the ELEMENT's name too.
		opt := optTemplate("CLUSTER", commentTerm, optMultiple("items", optNode("ELEMENT", "at0001", optProhibitedSingle("null_flavour"))))
		eachSetting(t, opt, func(t *testing.T, _ instance.Options, out any) {
			if got := commentValue[*rm.DVText](t, out).Value; got != "Comment" {
				t.Errorf("DV_TEXT.value = %q, want the ELEMENT's name Comment", got)
			}
		})
	})
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

// TestREQ107_OtherPlainTextKeepsTheExample is the REQ-107 Leaf values
// check that only a plain DV_TEXT that is an ELEMENT's value takes the
// name of a locatable: a plain DV_TEXT elsewhere, whose template leaves its
// value open, keeps the open-string example under both value fills, though
// a locatable with a name of its own encloses it. Here that is a
// PARTICIPATION's function in an OBSERVATION, an ISM_TRANSITION's reason in
// an ACTION, and a REFERENCE_RANGE's meaning on the value of an ELEMENT.
func TestREQ107_OtherPlainTextKeepsTheExample(t *testing.T) {
	rootTerm := `<term_definitions code="at0000"><items id="text">Pulse</items></term_definitions>`
	text := optNode("DV_TEXT", "")
	cases := []struct {
		name string
		opt  string
		got  func(t *testing.T, out any) any
	}{
		{
			name: "PARTICIPATION.function in a named OBSERVATION",
			opt:  optTemplate("OBSERVATION", rootTerm, optMultiple("other_participations", optParticipation(optSingle("function", text)))),
			got: func(t *testing.T, out any) any {
				obs, ok := out.(*rm.Observation)
				if !ok || len(obs.OtherParticipations) != 1 {
					t.Fatalf("generated root is %T, want an OBSERVATION with one other participation", out)
				}
				return obs.OtherParticipations[0].Function
			},
		},
		{
			name: "ISM_TRANSITION.reason in a named ACTION",
			opt: optTemplate("ACTION", rootTerm, optSingle("language"), optSingle("encoding"), optSingle("subject"),
				optSingle("ism_transition", optNode("ISM_TRANSITION", "", optMultiple("reason", text))),
				optSingle("description", emptyTree)),
			got: func(t *testing.T, out any) any {
				action, ok := out.(*rm.Action)
				if !ok || len(action.IsmTransition.Reason) != 1 {
					t.Fatalf("generated root is %T, want an ACTION whose transition has one reason", out)
				}
				return action.IsmTransition.Reason[0]
			},
		},
		{
			name: "REFERENCE_RANGE.meaning on the value of a named ELEMENT",
			opt: optCommentValue(countValue("3", optMultiple("other_reference_ranges", optNode("REFERENCE_RANGE", "",
				optSingle("meaning", text),
				optSingle("range", ordInterval("DV_COUNT", countValue("1"), countValue("5"))))))),
			got: func(t *testing.T, out any) any {
				count := commentValue[*rm.DVCount](t, out)
				if len(count.OtherReferenceRanges) != 1 {
					t.Fatalf("DV_COUNT has %d other reference ranges, want 1", len(count.OtherReferenceRanges))
				}
				return count.OtherReferenceRanges[0].Meaning
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, tc.opt, true)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%v/%v", opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					var got string
					switch v := tc.got(t, out).(type) {
					case *rm.DVText:
						got = v.Value
					case rm.DVText:
						got = v.Value
					default:
						t.Fatalf("value is %T, want a plain DV_TEXT", v)
					}
					if got != "example" {
						t.Errorf("DV_TEXT.value = %q, want example, not the name of the locatable that encloses it", got)
					}
				})
			}
		})
	}
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

// TestREQ107_IdentifierHasAnIDShapeAndAnIssuer is the REQ-107 Leaf values
// check on a DV_IDENTIFIER: under both value fills, where the template
// leaves its id open the id is ID-0001, and where the template allows an
// issuer it carries one, Example issuer where the template leaves the
// issuer open. A C_STRING with a list or a pattern gives its own example,
// which wins, for an issuer the policy does not visit too; an issuer the
// template prohibits is left out. The RM floor and the template validator
// pass.
func TestREQ107_IdentifierHasAnIDShapeAndAnIssuer(t *testing.T) {
	identifier := func(attrs ...string) string {
		return optTemplate("ELEMENT", optSingle("value", optNode("DV_IDENTIFIER", "", attrs...)))
	}
	// An optional issuer whose child is optional too, so Required does not
	// visit it.
	optionalIssuer := func(body string) string {
		return optOptionalSingleOver("issuer", optPrimitiveOccurring(0, 1, "STRING", "C_STRING", body))
	}
	issuer := func(s string) *string { return &s }
	cases := []struct {
		name   string
		opt    string
		id     func(fill instance.ValueFill, got string) bool
		issuer *string
	}{
		{
			name:   "silent",
			opt:    identifier(),
			id:     func(_ instance.ValueFill, got string) bool { return got == "ID-0001" },
			issuer: issuer("Example issuer"),
		},
		{
			name:   "id under a C_STRING with neither a list nor a pattern",
			opt:    identifier(stringAttr("id", "")),
			id:     func(_ instance.ValueFill, got string) bool { return got == "ID-0001" },
			issuer: issuer("Example issuer"),
		},
		{
			name: "id under a C_STRING with a list",
			opt:  identifier(stringAttr("id", "<list>A123</list><list>B456</list>")),
			id: func(fill instance.ValueFill, got string) bool {
				return got == "A123" || (fill == instance.RandomFill && got == "B456")
			},
			issuer: issuer("Example issuer"),
		},
		{
			name:   "id under a C_STRING with a pattern",
			opt:    identifier(stringAttr("id", "<pattern>[0-9]{3}</pattern>")),
			id:     func(_ instance.ValueFill, got string) bool { return len(got) == 3 },
			issuer: issuer("Example issuer"),
		},
		{
			name:   "issuer under a C_STRING with neither a list nor a pattern",
			opt:    identifier(optionalIssuer("")),
			id:     func(_ instance.ValueFill, got string) bool { return got == "ID-0001" },
			issuer: issuer("Example issuer"),
		},
		{
			name:   "optional issuer under a C_STRING with a list",
			opt:    identifier(optionalIssuer("<list>NHS</list>")),
			id:     func(_ instance.ValueFill, got string) bool { return got == "ID-0001" },
			issuer: issuer("NHS"),
		},
		{
			name: "issuer prohibited",
			opt:  identifier(optProhibitedSingle("issuer")),
			id:   func(_ instance.ValueFill, got string) bool { return got == "ID-0001" },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eachSetting(t, tc.opt, func(t *testing.T, opts instance.Options, out any) {
				got := rootElementValue[*rm.DVIdentifier](t, out)
				if !tc.id(opts.ValueFill, got.ID) {
					t.Errorf("DV_IDENTIFIER.id = %q, not the id this case wants", got.ID)
				}
				switch {
				case tc.issuer == nil && got.Issuer != nil:
					t.Errorf("DV_IDENTIFIER.issuer = %q, want none", *got.Issuer)
				case tc.issuer != nil && got.Issuer == nil:
					t.Errorf("DV_IDENTIFIER.issuer absent, want %q", *tc.issuer)
				case tc.issuer != nil && *got.Issuer != *tc.issuer:
					t.Errorf("DV_IDENTIFIER.issuer = %q, want %q", *got.Issuer, *tc.issuer)
				}
			})
		})
	}
}

// TestREQ107_DurationIsNotZeroUnderRandomFill is the REQ-107 Primitive-leaf
// value fill check on a DV_DURATION whose value a C_DURATION constrains:
// under RandomFill the value is a duration that is not zero and fits the
// pattern, a count of one designator it names, from 1 to 12 where the
// constraint has no range and among the counts inside its range where it
// has one; under ExampleFill it is the constraint's example, P0D for an
// open C_DURATION. The RM floor and the template validator pass.
func TestREQ107_DurationIsNotZeroUnderRandomFill(t *testing.T) {
	years := "<pattern>PY</pattern><range><lower_included>true</lower_included><upper_included>true</upper_included>" +
		"<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>P18Y</lower><upper>P120Y</upper></range>"
	cases := []struct {
		name            string
		body            string
		example         string
		letters         []string
		lowest, highest int
	}{
		{name: "open C_DURATION", body: "", example: "P0D", letters: []string{"Y", "M", "W", "D", "TH", "TM", "TS"}, lowest: 1, highest: 12},
		{name: "hours and minutes", body: "<pattern>PTHM</pattern>", example: "PT0H", letters: []string{"TH", "TM"}, lowest: 1, highest: 12},
		{name: "years, months and days", body: "<pattern>PYMD</pattern>", example: "P0D", letters: []string{"Y", "M", "D"}, lowest: 1, highest: 12},
		{name: "years from 18 to 120", body: years, example: "P18Y", letters: []string{"Y"}, lowest: 18, highest: 120},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt := optTemplate("ELEMENT", optSingle("value", optNode("DV_DURATION", "",
				optSingle("value", optPrimitive("DURATION", "C_DURATION", tc.body)))))
			eachSetting(t, opt, func(t *testing.T, opts instance.Options, out any) {
				got := rootElementValue[*rm.DVDuration](t, out).Value
				switch {
				case opts.ValueFill == instance.RandomFill && !randomDuration(got, tc.letters, tc.lowest, tc.highest):
					t.Errorf("DV_DURATION.value = %q, want a count from %d to %d of one of %v", got, tc.lowest, tc.highest, tc.letters)
				case opts.ValueFill != instance.RandomFill && got != tc.example:
					t.Errorf("DV_DURATION.value = %q, want the example %q", got, tc.example)
				}
			})
		})
	}
}

// TestREQ107_UnconstrainedDurationIsNotZeroUnderRandomFill is the REQ-107
// Primitive-leaf value fill check on a DV_DURATION whose value the template
// leaves unconstrained, by naming no value or by a C_PRIMITIVE_OBJECT with
// no item: under RandomFill the value is drawn as under a C_DURATION that
// is empty, a count from 1 to 12 of one designator; under ExampleFill it
// is the sentinel P0D. The RM floor passes, and the template validator
// too where the template names no value: it refuses a string under a
// C_PRIMITIVE_OBJECT with no item, which AOM 1.4 does not allow. Two such
// durations that bound an interval are drawn and then put in order.
func TestREQ107_UnconstrainedDurationIsNotZeroUnderRandomFill(t *testing.T) {
	check := func(t *testing.T, opts instance.Options, out any) {
		t.Helper()
		got := rootElementValue[*rm.DVDuration](t, out).Value
		switch {
		case opts.ValueFill == instance.RandomFill && !randomDuration(got, everyDesignator, 1, 12):
			t.Errorf("DV_DURATION.value = %q, want a count from 1 to 12 of one of %v", got, everyDesignator)
		case opts.ValueFill != instance.RandomFill && got != "P0D":
			t.Errorf("DV_DURATION.value = %q, want the sentinel P0D", got)
		}
	}
	t.Run("value left silent", func(t *testing.T) {
		eachSetting(t, optTemplate("ELEMENT", optSingle("value", optNode("DV_DURATION", ""))), check)
	})
	t.Run("C_PRIMITIVE_OBJECT with no item", func(t *testing.T) {
		anyDuration := `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>DURATION</rm_type_name><node_id></node_id></children>`
		c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", optNode("DV_DURATION", "", optSingle("value", anyDuration)))), true)
		for _, opts := range defaultsOptions() {
			t.Run(fmt.Sprintf("%v/%v", opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				check(t, opts, out)
				noFloorErrors(t, out)
			})
		}
	})
	// Each bound is drawn before the interval's walk puts the two in order,
	// so no seed leaves them inverted, which the RM floor refuses.
	t.Run("bounds of an interval", func(t *testing.T) {
		c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", optNode("DV_INTERVAL&lt;DV_DURATION&gt;", "",
			optSingle("lower", optNode("DV_DURATION", "")), optSingle("upper", optNode("DV_DURATION", ""))))), true)
		for seed := range uint64(20) {
			opts := instance.Options{ValueFill: instance.RandomFill, ValueSource: mrand.NewPCG(seed, 2), Now: defaultsNow}
			out, err := instance.Generate(t.Context(), c, opts)
			if err != nil {
				t.Fatalf("Generate with seed %d: %v", seed, err)
			}
			iv := rootElementValue[*rm.DVInterval[rm.DVDuration]](t, out)
			for _, got := range []string{iv.Lower.Value, iv.Upper.Value} {
				if !randomDuration(got, everyDesignator, 1, 12) {
					t.Errorf("seed %d: bound %q, want a count from 1 to 12 of one of %v", seed, got, everyDesignator)
				}
			}
			checkValid(t, out, c)
		}
	})
}

// everyDesignator lists every designator of a duration, each written as it
// follows P, for randomDuration.
var everyDesignator = []string{"Y", "M", "W", "D", "TH", "TM", "TS"}

// randomDuration reports whether s is a count from lowest to highest of one
// of the designators letters, each written as it follows P.
func randomDuration(s string, letters []string, lowest, highest int) bool {
	for _, l := range letters {
		for n := lowest; n <= highest; n++ {
			if s == fmt.Sprintf("P%s%d%s", l[:len(l)-1], n, l[len(l)-1:]) {
				return true
			}
		}
	}
	return false
}

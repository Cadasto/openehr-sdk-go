package instance_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// optLanguageElement is the language element optTemplate writes.
const optLanguageElement = `<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>`

// TestREQ107_EmptyLanguageReadsTheTemplateThenEn is the REQ-107 check that
// an empty Options.Language is read as Compiled.Language(), and as en when
// the template has no language either: the ENTRY's language and a
// COMPOSITION's language take it. It holds under both policies and both
// compile modes.
func TestREQ107_EmptyLanguageReadsTheTemplateThenEn(t *testing.T) {
	cases := []struct {
		name, from, to, want string
	}{
		{name: "template language nl", from: "<code_string>en</code_string>", to: "<code_string>nl</code_string>", want: "nl"},
		{name: "no template language", from: optLanguageElement, to: "", want: "en"},
	}
	for _, tc := range cases {
		for _, class := range []string{"OBSERVATION", "COMPOSITION"} {
			opt := strings.Replace(optTemplate(class), tc.from, tc.to, 1)
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, opt, implicit)
				if tc.want == "en" && c.Language() != "" {
					t.Fatalf("Compiled.Language() = %q, want empty", c.Language())
				}
				for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
					t.Run(fmt.Sprintf("%s/%s/implicit=%t/%v", tc.name, class, implicit, policy), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, instance.Options{
							Policy: policy, Now: defaultsNow, Territory: "NL", Composer: testComposer(),
						})
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						var got rm.CodePhrase
						if comp, ok := out.(*rm.Composition); ok {
							got = comp.Language
						} else {
							got, _, _ = entryLanguageEncoding(out)
						}
						want := rm.CodePhrase{CodeString: tc.want, TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
						checkCode(t, class+".language", got, want)
					})
				}
			}
		}
	}
}

// TestREQ107_AssumedValueIsNotRead is the REQ-107 check that the generator
// does not read an OPT's assumed_value, which compile captures into the
// constraint's Default: under ExampleFill a leaf whose assumed_value
// differs from its constraint's example value takes the example value.
// It holds for a C_BOOLEAN and a C_INTEGER, under both policies and both
// compile modes.
func TestREQ107_AssumedValueIsNotRead(t *testing.T) {
	cases := []struct {
		name  string
		value string
		check func(t *testing.T, v rm.DataValue)
	}{
		{
			// The example value of a C_BOOLEAN that admits true is true.
			name: "C_BOOLEAN",
			value: optNode("DV_BOOLEAN", "", optSingle("value", optPrimitive("BOOLEAN", "C_BOOLEAN",
				"<true_valid>true</true_valid><false_valid>true</false_valid><assumed_value>false</assumed_value>"))),
			check: func(t *testing.T, v rm.DataValue) {
				if b, ok := v.(*rm.DVBoolean); !ok || !b.Value {
					t.Errorf("DV_BOOLEAN = %#v, want true, the example value, not the assumed false", v)
				}
			},
		},
		{
			// The example value of a C_INTEGER list is its first member.
			name: "C_INTEGER",
			value: optNode("DV_COUNT", "", optSingle("magnitude", optPrimitive("INTEGER", "C_INTEGER",
				"<list>3</list><list>7</list><assumed_value>7</assumed_value>"))),
			check: func(t *testing.T, v rm.DataValue) {
				if c, ok := v.(*rm.DVCount); !ok || c.Magnitude != 3 {
					t.Errorf("DV_COUNT = %#v, want 3, the example value, not the assumed 7", v)
				}
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value)), implicit)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v", tc.name, implicit, policy), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					tc.check(t, out.(*rm.Element).Value)
				})
			}
		}
	}
}

// TestREQ107_OtherGenericTypeIsRefused is the REQ-107 check of the known
// gap for generic types: an OPT node whose rm_type_name is a generic type
// other than a DV_INTERVAL instantiation, here POINT_EVENT<ITEM_TREE>,
// cannot be built, and Generate returns an error and no root, under both
// policies and both compile modes.
func TestREQ107_OtherGenericTypeIsRefused(t *testing.T) {
	opt := optTemplate("OBSERVATION", optSingle("data", optNode("HISTORY", "at0001",
		optMultiple("events", optNode("POINT_EVENT&lt;ITEM_TREE&gt;", "at0002")))))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(fmt.Sprintf("implicit=%t/%v", implicit, policy), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
				if !errors.Is(err, rmwrite.ErrUnknownRMType) {
					t.Errorf("Generate error = %v, want one wrapping rmwrite.ErrUnknownRMType", err)
				}
				if out != nil {
					t.Errorf("Generate returned %T with the error, want no root", out)
				}
			})
		}
	}
}

package instance_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// localPlaceholder is the code REQ-107 makes the generator write on a code
// phrase it has no OPT value for.
var localPlaceholder = rm.CodePhrase{
	CodeString:    "at0000",
	TerminologyID: rm.TerminologyID{Value: "local"},
}

// TestREQ107_RootPlaceholders is the REQ-107 check that a generation root
// gets the same placeholders as a nested value: the URI ehr://example on a
// DV_EHR_URI and the code at0000 in terminology local on a CODE_PHRASE.
// The root is compiled with the implicit attributes, so its String
// attributes are filled by the pass that writes the open-string example.
func TestREQ107_RootPlaceholders(t *testing.T) {
	cases := []struct {
		name  string
		root  string
		check func(t *testing.T, out any)
	}{
		{
			name: "DV_EHR_URI value",
			root: "DV_EHR_URI",
			check: func(t *testing.T, out any) {
				if got := out.(*rm.DVEHRURI).Value; got != "ehr://example" {
					t.Errorf("DV_EHR_URI.value = %q, want %q", got, "ehr://example")
				}
			},
		},
		{
			name: "CODE_PHRASE code",
			root: "CODE_PHRASE",
			check: func(t *testing.T, out any) {
				if got := *out.(*rm.CodePhrase); got != localPlaceholder {
					t.Errorf("CODE_PHRASE = %+v, want %+v", got, localPlaceholder)
				}
			},
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, optTemplate(tc.root), true)
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(tc.name+"/"+policy.String(), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
				if err != nil {
					t.Fatalf("Generate(%v): %v", policy, err)
				}
				tc.check(t, out)
			})
		}
	}
}

package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_ComposerIsWrittenAsGiven is the REQ-107 check that a
// COMPOSITION's composer is Options.Composer, the very value the caller
// gave, whatever the OPT constrains on composer: an attribute it names
// with no child, a bare PARTY_IDENTIFIED node, a PARTY_IDENTIFIED whose
// name a C_STRING pins, a PARTY_SELF node, and a prohibition. The caller's
// value is written even where the OPT's own constraint rejects it, here a
// PARTY_SELF where the OPT pins a PARTY_IDENTIFIED. It holds under both
// policies, both value fills and both compile modes.
func TestREQ107_ComposerIsWrittenAsGiven(t *testing.T) {
	shapes := []struct {
		name     string
		composer string
	}{
		{name: "named with no child", composer: optSingle("composer")},
		{name: "bare PARTY_IDENTIFIED", composer: optSingle("composer", optNode("PARTY_IDENTIFIED", ""))},
		{
			name: "PARTY_IDENTIFIED, name C_STRING",
			composer: optSingle("composer", optNode("PARTY_IDENTIFIED", "",
				optStringAttr("name", "<list>Dr Who</list>"))),
		},
		{name: "PARTY_SELF", composer: optSingle("composer", optNode("PARTY_SELF", ""))},
		{name: "prohibited", composer: optProhibitedSingle("composer")},
	}
	given := []struct {
		name     string
		composer func() rm.PartyProxy
	}{
		{name: "PARTY_IDENTIFIED", composer: func() rm.PartyProxy { return testComposer() }},
		{name: "PARTY_SELF", composer: func() rm.PartyProxy { return &rm.PartySelf{} }},
	}
	for _, shape := range shapes {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("COMPOSITION", shape.composer), implicit)
			for _, g := range given {
				for _, opts := range defaultsOptions() {
					opts.Territory, opts.Composer = "NL", g.composer()
					t.Run(fmt.Sprintf("%s/given %s/implicit=%t/%v/%v", shape.name, g.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, opts)
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						if got := generatedComposition(t, out).Composer; got != opts.Composer {
							t.Errorf("COMPOSITION.composer = %#v, want the Options composer %#v as given", got, opts.Composer)
						}
					})
				}
			}
		}
	}
}

package instance_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestREQ107_AttributeNotInRMIsNotVisited is the REQ-107 Visit rule check
// that an attribute the compile marks as one the node's RM class does not
// have (templatecompile NotInRM) is not visited, whatever existence the OPT
// gives it: an ACTION whose ISM_TRANSITION constrains name at 1..1, and an
// ELEMENT that declares null_flavor beside null_flavour. Generate returns a
// root under every policy, both value fills and both compile modes, the
// root passes the RM floor and the template validator, and the validator
// leaves the attribute out and lists its path in Coverage.NotInRM.
func TestREQ107_AttributeNotInRMIsNotVisited(t *testing.T) {
	cases := []struct {
		name    string
		opt     string
		notInRM []string
	}{
		{
			name: "ISM_TRANSITION name at 1..1",
			opt: optTemplate("ACTION", optSingle("ism_transition",
				optNode("ISM_TRANSITION", "", optSingle("name", optNode("DV_TEXT", ""))))),
			notInRM: []string{"/ism_transition/name"},
		},
		{
			name: "ELEMENT null_flavor at 1..1 beside null_flavour",
			opt: optTemplate("ELEMENT",
				optSingle("null_flavour", optCodedText("openehr", "253")),
				optSingle("null_flavor", optCodedText("openehr", "271"))),
			notInRM: []string{"/null_flavor"},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v, want a root", err)
					}
					checkValid(t, out, c)
					if got := validation.Validate(out, c).Coverage.NotInRM; !slices.Equal(got, tc.notInRM) {
						t.Errorf("Validate: Coverage.NotInRM = %q, want %q", got, tc.notInRM)
					}
				})
			}
		}
	}
}

// optLink is a LINK child whose occurrences are lower..upper.
func optLink(lower, upper int) string {
	return optOccurring("C_COMPLEX_OBJECT", "LINK", "", lower, upper)
}

// TestREQ107_AttributeWithoutWriterIsVisitedOnlyWhereRequired is the REQ-107
// Visit rule and Errors check on an attribute the generator has no writer
// for: a LOCATABLE's feeder_audit and links, and a DV_MULTIMEDIA's
// integrity_check. Where the OPT leaves it optional, whatever it puts under
// it, it is not visited, and Generate returns a root that passes the RM
// floor and the template validator; so it is for a list only its existence
// or only its cardinality asks a member of, which may stay absent. Where the
// OPT requires it, by its existence, a child's occurrences, or an existence
// with a cardinality that asks for a member, Generate returns an error
// wrapping ErrConstraintUnsatisfiable that names the RM type, the attribute
// and the OPT path, and no root. Both hold under every policy, both value
// fills and both compile modes.
func TestREQ107_AttributeWithoutWriterIsVisitedOnlyWhereRequired(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// detail is what the error says after the sentinel's own text; ""
		// marks a control row, which must generate.
		detail string
	}{
		{
			name: "optional feeder_audit over a FEEDER_AUDIT",
			opt:  optTemplate("CLUSTER", optOptionalSingleOver("feeder_audit", optNode("FEEDER_AUDIT", ""))),
		},
		{
			name: "optional feeder_audit of a nested ELEMENT",
			opt: optTemplate("CLUSTER", optMultiple("items", optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 1, 1,
				optOptionalSingleOver("feeder_audit", optNode("FEEDER_AUDIT", ""))))),
		},
		{
			name: "optional links over an optional LINK",
			opt:  optTemplate("CLUSTER", optListWithin("links", 0, 1, optLink(0, 1))),
		},
		{
			name: "links only their existence requires",
			opt:  optTemplate("CLUSTER", optListWithin("links", 1, 1, optLink(0, 1))),
		},
		{
			name: "links only their cardinality asks a member of",
			opt:  optTemplate("CLUSTER", optCardinal("links", 1, -1, optLink(0, 1))),
		},
		{
			name: "optional integrity_check over an OCTET",
			opt:  optTemplate("DV_MULTIMEDIA", optListWithin("integrity_check", 0, 1, optOccurring("C_COMPLEX_OBJECT", "OCTET", "at0001", 1, 1))),
		},
		{
			name:   "feeder_audit at existence 1..1 over a FEEDER_AUDIT",
			opt:    optTemplate("CLUSTER", optSingle("feeder_audit", optNode("FEEDER_AUDIT", ""))),
			detail: "CLUSTER.feeder_audit at / (required, but the generator has no writer for it)",
		},
		{
			name:   "feeder_audit at existence 1..1 with no child",
			opt:    optTemplate("CLUSTER", guardSingle("feeder_audit", guardExistence11)),
			detail: "CLUSTER.feeder_audit at / (required, but the generator has no writer for it)",
		},
		{
			name: "feeder_audit at existence 1..1 of a nested ELEMENT",
			opt: optTemplate("CLUSTER", optMultiple("items", optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 1, 1,
				optSingle("feeder_audit", optNode("FEEDER_AUDIT", ""))))),
			detail: "ELEMENT.feeder_audit at /items[at0001] (required, but the generator has no writer for it)",
		},
		{
			name:   "links over a 1..1 LINK",
			opt:    optTemplate("CLUSTER", optListWithin("links", 0, 1, optLink(1, 1))),
			detail: "CLUSTER.links at / (required, but the generator has no writer for it)",
		},
		{
			name:   "links at existence 1..1 and cardinality 1..*",
			opt:    optTemplate("CLUSTER", optMultiple("links", optLink(0, 1))),
			detail: "CLUSTER.links at / (required, but the generator has no writer for it)",
		},
		{
			name:   "integrity_check at existence 1..1 and cardinality 1..*",
			opt:    optTemplate("DV_MULTIMEDIA", optMultiple("integrity_check", optOccurring("C_COMPLEX_OBJECT", "OCTET", "at0001", 0, 1))),
			detail: "DV_MULTIMEDIA.integrity_check at / (required, but the generator has no writer for it)",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if tc.detail == "" {
						if err != nil {
							t.Fatalf("Generate: %v, want a root", err)
						}
						checkValid(t, out, c)
						return
					}
					checkUnsatisfiableDetail(t, "Generate", err, tc.detail)
					if out != nil {
						t.Errorf("Generate returned %T with the error, want no root", out)
					}
				})
			}
		}
	}
}

// TestREQ107_OctetListIsNotVisited is the REQ-107 Visit rule check that a
// DV_MULTIMEDIA's data, a list of octets no OPT child describes, is not
// visited under any policy, so Generate builds no member of the OCTET child
// the OPT puts under it, and the DV_MULTIMEDIA carries the inline example
// payload RM defaults give, sized. It holds for data at existence 0..1 and
// 1..1 over an OCTET child at occurrences 1..1, at the root and as an
// ELEMENT's value, under every policy, both value fills and both compile
// modes, and each root passes the RM floor and the template validator.
func TestREQ107_OctetListIsNotVisited(t *testing.T) {
	data := []struct {
		name string
		attr string
	}{
		{name: "data at existence 0..1", attr: optOctetData()},
		{name: "data at existence 1..1", attr: optListWithin("data", 1, 1, optOccurring("C_COMPLEX_OBJECT", "OCTET", "at0001", 1, 1))},
	}
	for _, d := range data {
		for _, place := range multimediaPlacements {
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, place.opt([]string{d.attr}), implicit)
				for _, opts := range guardOptions() {
					t.Run(fmt.Sprintf("%s/%s/implicit=%t/%v/%v", d.name, place.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, opts)
						if err != nil {
							t.Fatalf("Generate: %v, want a root", err)
						}
						checkSizedData(t, place.multimedia(t, out))
						checkValid(t, out, c)
					})
				}
			}
		}
	}
}

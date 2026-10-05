package instance_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestREQ107_StringAttributeIsWritten is the REQ-107 check that a String
// attribute the generator visits gets a value, also one the
// template-instance writer has no field for: a DV_TEXT's or a
// DV_CODED_TEXT's formatting, a CODE_PHRASE's preferred_term and a
// DV_QUANTITY's magnitude_status. A required one is written under both
// policies and the template validator then finds it; an optional one the
// OPT names is written under Example only. A magnitude_status takes "=",
// a value its RM invariant (Magnitude_status_valid) accepts. It holds
// under both value fills and both compile modes.
func TestREQ107_StringAttributeIsWritten(t *testing.T) {
	cases := []struct {
		name  string
		value func(attr string) string
		path  string
		read  func(v rm.DataValue) string
		want  string
	}{
		{
			name:  "DV_TEXT formatting",
			value: func(attr string) string { return optNode("DV_TEXT", "", attr) },
			path:  "/value/formatting",
			read:  func(v rm.DataValue) string { return derefString(v.(*rm.DVText).Formatting) },
		},
		{
			name:  "DV_CODED_TEXT formatting",
			value: func(attr string) string { return optNode("DV_CODED_TEXT", "", attr) },
			path:  "/value/formatting",
			read:  func(v rm.DataValue) string { return derefString(v.(*rm.DVCodedText).Formatting) },
		},
		{
			name: "CODE_PHRASE preferred_term",
			value: func(attr string) string {
				return optNode("DV_CODED_TEXT", "", optSingle("defining_code", optNode("CODE_PHRASE", "", attr)))
			},
			path: "/value/defining_code/preferred_term",
			read: func(v rm.DataValue) string { return derefString(v.(*rm.DVCodedText).DefiningCode.PreferredTerm) },
		},
		{
			name:  "DV_QUANTITY magnitude_status",
			value: func(attr string) string { return optNode("DV_QUANTITY", "", attr) },
			path:  "/value/magnitude_status",
			read:  func(v rm.DataValue) string { return derefString(v.(*rm.DVQuantity).MagnitudeStatus) },
			want:  "=",
		},
	}
	for _, tc := range cases {
		attrName := tc.path[strings.LastIndex(tc.path, "/")+1:]
		for _, required := range []bool{true, false} {
			attr := optOptionalSingle(attrName)
			if required {
				attr = optSingle(attrName)
			}
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value(attr))), implicit)
				for _, opts := range defaultsOptions() {
					t.Run(fmt.Sprintf("%s/required=%t/implicit=%t/%v/%v", tc.name, required, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, opts)
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						got := tc.read(out.(*rm.Element).Value)
						wantWritten := required || opts.Policy == instance.Example
						switch {
						case wantWritten && got == "":
							t.Errorf("%s is empty, want a value", tc.name)
						case wantWritten && tc.want != "" && got != tc.want:
							t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
						case !wantWritten && got != "":
							t.Errorf("%s = %q, want none: Minimal does not visit an optional attribute", tc.name, got)
						}
						for _, iss := range templateErrors(out, c) {
							if strings.HasPrefix(iss.Path, tc.path) {
								t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
							}
						}
						for _, iss := range validation.ValidateRM(out).Issues {
							if iss.Severity == validation.Error && strings.HasPrefix(iss.Path, tc.path) {
								t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
							}
						}
					})
				}
			}
		}
	}
}

// partyName returns the name of a PARTY_IDENTIFIED or a PARTY_RELATED, ""
// when it has none, and whether v is one of the two.
func partyName(v any) (string, bool) {
	switch p := v.(type) {
	case *rm.PartyIdentified:
		return derefString(p.Name), true
	case *rm.PartyRelated:
		return derefString(p.Name), true
	}
	return "", false
}

// TestREQ107_PartyIdentifiedGetsAName is the REQ-107 check that a
// PARTY_IDENTIFIED or a PARTY_RELATED the generator writes with no name,
// identifiers or external_ref takes the name "example", so RM
// Basic_validity holds: an EVENT_CONTEXT's health_care_facility the OPT
// names without a child (under Example, which visits it) or as a bare
// PARTY_IDENTIFIED node, and an ENTRY subject the OPT pins to a bare
// PARTY_IDENTIFIED or PARTY_RELATED node. The RM rule wins over a
// prohibited name. A name the OPT constrains with a C_STRING keeps the
// value that constraint admits. It holds under both policies, both value
// fills and both compile modes.
func TestREQ107_PartyIdentifiedGetsAName(t *testing.T) {
	type place struct {
		name     string
		opt      string
		policies []instance.Policy
		party    func(out any) any
		want     string
	}
	facility := func(attr string) string {
		return optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "", attr)))
	}
	fromFacility := func(out any) any { return out.(*rm.Composition).Context.HealthCareFacility }
	fromSubject := func(out any) any { return out.(*rm.Observation).Subject }
	both := []instance.Policy{instance.Minimal, instance.Example}
	cases := []place{
		{
			name: "health_care_facility named without a child", opt: facility(optOptionalSingle("health_care_facility")),
			policies: []instance.Policy{instance.Example}, party: fromFacility, want: "example",
		},
		{
			name: "health_care_facility bare PARTY_IDENTIFIED", opt: facility(optOptionalSingleOver("health_care_facility", optNode("PARTY_IDENTIFIED", ""))),
			policies: both, party: fromFacility, want: "example",
		},
		{
			name: "subject bare PARTY_IDENTIFIED", opt: optTemplate("OBSERVATION", optSingle("subject", optNode("PARTY_IDENTIFIED", ""))),
			policies: both, party: fromSubject, want: "example",
		},
		{
			name: "subject bare PARTY_RELATED", opt: optTemplate("OBSERVATION", optSingle("subject", optNode("PARTY_RELATED", ""))),
			policies: both, party: fromSubject, want: "example",
		},
		{
			name: "subject PARTY_IDENTIFIED, name prohibited", opt: optTemplate("OBSERVATION", optSingle("subject",
				optNode("PARTY_IDENTIFIED", "", optProhibitedSingle("name")))),
			policies: both, party: fromSubject, want: "example",
		},
		{
			name: "subject PARTY_IDENTIFIED, name C_STRING", opt: optTemplate("OBSERVATION", optSingle("subject",
				optNode("PARTY_IDENTIFIED", "", optStringAttr("name", "<list>Dr Who</list>")))),
			policies: both, party: fromSubject, want: "Dr Who",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				if !slices.Contains(tc.policies, opts.Policy) {
					continue
				}
				opts.Territory, opts.Composer = "NL", testComposer()
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					got, ok := partyName(tc.party(out))
					if !ok {
						t.Fatalf("party is %T, want a PARTY_IDENTIFIED or a PARTY_RELATED", tc.party(out))
					}
					if got != tc.want {
						t.Errorf("name = %q, want %q", got, tc.want)
					}
				})
			}
		}
	}
}

// derefString is the string s points to, or "" when s is nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

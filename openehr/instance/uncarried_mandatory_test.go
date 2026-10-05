package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// entryConcretes are the concrete ENTRY classes.
var entryConcretes = []string{"OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION", "ADMIN_ENTRY"}

// TestREQ107_UncarriedMandatoryAttributesAreFilled is the REQ-107 check that
// an attribute the BMM marks mandatory, which a node compiled
// WithoutImplicitAttributes does not carry, is filled as an attribute the
// OPT leaves silent is: an ENTRY's language, encoding and subject (as
// PARTY_SELF), its own mandatory structure, a HISTORY's origin and an
// event's time from the clock. Each ENTRY concrete is generated as the
// template root with nothing else named, and nested in a COMPOSITION, and
// an OBSERVATION names a HISTORY and its events and nothing under them.
// The output passes the RM floor under both policies and both value fills.
func TestREQ107_UncarriedMandatoryAttributesAreFilled(t *testing.T) {
	type tree struct {
		name  string
		opt   string
		entry func(t *testing.T, out any) any
	}
	var cases []tree
	for _, class := range entryConcretes {
		cases = append(cases,
			tree{
				name:  class + " root",
				opt:   optTemplate(class),
				entry: func(_ *testing.T, out any) any { return out },
			},
			tree{
				name: class + " in COMPOSITION.content",
				opt: optTemplate("COMPOSITION", optMultiple("content",
					optArchetypeRoot(class, "openEHR-EHR-"+class+".rm_defaults.v1"))),
				entry: func(t *testing.T, out any) any {
					comp := generatedComposition(t, out)
					if len(comp.Content) != 1 {
						t.Fatalf("COMPOSITION.content has %d members, want 1", len(comp.Content))
					}
					return comp.Content[0]
				},
			})
	}
	cases = append(cases, tree{
		name: "OBSERVATION with a sparse HISTORY and events",
		opt: optTemplate("OBSERVATION", optSingle("data", optNode("HISTORY", "at0001",
			optMultiple("events", optNode("POINT_EVENT", "at0002"), optNode("INTERVAL_EVENT", "at0003"))))),
		entry: func(_ *testing.T, out any) any { return out },
	})
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, false)
		for _, opts := range defaultsOptions() {
			opts.Language, opts.Territory, opts.Composer = "en", "NL", testComposer()
			t.Run(fmt.Sprintf("%s/%v/%v", tc.name, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				entry := tc.entry(t, out)
				subject := entrySubject(entry)
				if _, ok := subject.(*rm.PartySelf); !ok {
					t.Errorf("%T.subject = %#v, want PARTY_SELF", entry, subject)
				}
				lang, enc, _ := entryLanguageEncoding(entry)
				checkCode(t, "ENTRY.language", lang, rm.CodePhrase{CodeString: "en", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}})
				checkCode(t, "ENTRY.encoding", enc, rm.CodePhrase{CodeString: "UTF-8", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}})
				noFloorErrors(t, out)
			})
		}
	}
}

// entrySubject returns the subject of an ENTRY concrete, or nil for any
// other value.
func entrySubject(v any) rm.PartyProxy {
	switch e := v.(type) {
	case *rm.Observation:
		return e.Subject
	case *rm.Evaluation:
		return e.Subject
	case *rm.Instruction:
		return e.Subject
	case *rm.Action:
		return e.Subject
	case *rm.AdminEntry:
		return e.Subject
	}
	return nil
}

// entryLanguageEncoding returns the language and encoding of an ENTRY
// concrete; ok is false for any other value.
func entryLanguageEncoding(v any) (language, encoding rm.CodePhrase, ok bool) {
	switch e := v.(type) {
	case *rm.Observation:
		return e.Language, e.Encoding, true
	case *rm.Evaluation:
		return e.Language, e.Encoding, true
	case *rm.Instruction:
		return e.Language, e.Encoding, true
	case *rm.Action:
		return e.Language, e.Encoding, true
	case *rm.AdminEntry:
		return e.Language, e.Encoding, true
	}
	return rm.CodePhrase{}, rm.CodePhrase{}, false
}

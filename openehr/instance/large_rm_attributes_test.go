package instance_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/builderseed"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// rmAttributeRuns are the calls a Large RM-attribute case compares: Large,
// which writes the optional RM attributes the template does not name, and
// Medium and the composition builder's seed, which write none of them.
var rmAttributeRuns = []struct {
	name   string
	ctx    func(context.Context) context.Context
	policy instance.Policy
	writes bool
}{
	{name: "Large", ctx: plainContext, policy: instance.Large, writes: true},
	{name: "Medium", ctx: plainContext, policy: instance.Medium},
	{name: "builder seed", ctx: builderseed.With, policy: instance.Large},
}

// nowText is opts.Now as the generator writes a date-time from the clock.
func nowText(opts instance.Options) string {
	return opts.Now.UTC().Format(time.RFC3339)
}

// checkExampleParticipation reports where p is not the participation Large
// writes where the template names none: a function that is a plain DV_TEXT
// holding the rubric of the RM default openehr 253, unknown, which the FLAT
// format carries, a mode coded openehr 193 not specified with its rubric,
// and a PARTY_SELF performer (REQ-107 RM defaults).
func checkExampleParticipation(t *testing.T, where string, p rm.Participation) {
	t.Helper()
	function, ok := p.Function.(*rm.DVText)
	if !ok {
		t.Errorf("%s function is %T, want *rm.DVText", where, p.Function)
	} else if rubric, _ := terminology.ParticipationFunction.Rubric("253"); function.Value != rubric {
		t.Errorf("%s function = %q, want %q", where, function.Value, rubric)
	}
	if p.Mode == nil {
		t.Errorf("%s mode is absent, want openehr 193", where)
	} else if rubric, _ := terminology.ParticipationMode.Rubric("193"); p.Mode.DefiningCode.CodeString != "193" ||
		p.Mode.DefiningCode.TerminologyID.Value != terminology.ID || p.Mode.Value != rubric {
		t.Errorf("%s mode = %s|%s|%s, want %s|193|%s", where, p.Mode.DefiningCode.TerminologyID.Value,
			p.Mode.DefiningCode.CodeString, p.Mode.Value, terminology.ID, rubric)
	}
	if _, ok := p.Performer.(*rm.PartySelf); !ok {
		t.Errorf("%s performer is %T, want *rm.PartySelf", where, p.Performer)
	}
}

// TestREQ107_LargeWritesOptionalRMAttributes is the REQ-107 check of What
// each policy visits and RM defaults that, under Large, a template that
// names none of them gets a fixed set of optional RM attributes: the
// COMPOSITION's feeder_audit, with an originating_system_audit whose
// system_id is set and whose time is the clock; the EVENT_CONTEXT's
// end_time, from the clock, its health_care_facility, a PARTY_IDENTIFIED
// named example, and one participations member; each ENTRY's provider, a
// PARTY_SELF, and one other_participations member. Medium writes none of
// them, nor does the composition builder's seed (REQ-101). Every root
// passes the template validator and the RM floor, under both value fills.
func TestREQ107_LargeWritesOptionalRMAttributes(t *testing.T) {
	c := compileOPTText(t, optUsesTemplate(optObservationUse("", 0, 1)), true)
	for _, run := range rmAttributeRuns {
		for _, opts := range corpusOptions(run.policy) {
			t.Run(fmt.Sprintf("%s/%v", run.name, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(run.ctx(t.Context()), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				comp, err := instance.AsComposition(out)
				if err != nil {
					t.Fatalf("AsComposition: %v", err)
				}
				if len(comp.Content) != 1 {
					t.Fatalf("content holds %d members, want 1", len(comp.Content))
				}
				obs, ok := comp.Content[0].(*rm.Observation)
				if !ok {
					t.Fatalf("content member is %T, want *rm.Observation", comp.Content[0])
				}
				ctx := comp.Context
				if ctx == nil {
					t.Fatal("context is absent")
				}
				if !run.writes {
					if comp.FeederAudit != nil || ctx.EndTime != nil || ctx.HealthCareFacility != nil ||
						len(ctx.Participations) != 0 || obs.Provider != nil || len(obs.OtherParticipations) != 0 {
						t.Errorf("Generate(%s) wrote an optional RM attribute: feeder_audit=%v end_time=%v health_care_facility=%v participations=%d provider=%v other_participations=%d",
							run.name, comp.FeederAudit, ctx.EndTime, ctx.HealthCareFacility, len(ctx.Participations), obs.Provider, len(obs.OtherParticipations))
					}
				} else {
					now := nowText(opts)
					if fa := comp.FeederAudit; fa == nil {
						t.Error("feeder_audit is absent")
					} else {
						if fa.OriginatingSystemAudit.SystemID == "" {
							t.Error("feeder_audit originating_system_audit system_id is empty")
						}
						if tm := fa.OriginatingSystemAudit.Time; tm == nil || tm.Value != now {
							t.Errorf("feeder_audit originating_system_audit time = %v, want %s", tm, now)
						}
					}
					if ctx.EndTime == nil || ctx.EndTime.Value != now {
						t.Errorf("context end_time = %v, want %s", ctx.EndTime, now)
					}
					if hcf, ok := ctx.HealthCareFacility.(*rm.PartyIdentified); !ok || hcf.Name == nil || *hcf.Name != "example" {
						t.Errorf("context health_care_facility = %#v, want a PARTY_IDENTIFIED named example", ctx.HealthCareFacility)
					}
					if len(ctx.Participations) != 1 {
						t.Errorf("context participations holds %d members, want 1", len(ctx.Participations))
					} else {
						checkExampleParticipation(t, "context participation", ctx.Participations[0])
					}
					if _, ok := obs.Provider.(*rm.PartySelf); !ok {
						t.Errorf("OBSERVATION provider is %T, want *rm.PartySelf", obs.Provider)
					}
					if len(obs.OtherParticipations) != 1 {
						t.Errorf("OBSERVATION other_participations holds %d members, want 1", len(obs.OtherParticipations))
					} else {
						checkExampleParticipation(t, "OBSERVATION other participation", obs.OtherParticipations[0])
					}
				}
				noValidationErrors(t, comp, c)
				noFloorErrors(t, comp)
			})
		}
	}
}

// TestREQ107_LargeLeavesProhibitedRMAttributesOut is the REQ-107 check that
// Large writes none of its optional RM attributes where the template
// prohibits it: the COMPOSITION's feeder_audit, the EVENT_CONTEXT's
// end_time, health_care_facility and participations, and the ENTRY's
// provider and other_participations. Every root passes the template
// validator and the RM floor, under both value fills.
func TestREQ107_LargeLeavesProhibitedRMAttributesOut(t *testing.T) {
	observation := optRootOccurring("OBSERVATION", usedObservationID, 0, 1,
		optSingle("data", optNode("HISTORY", "at0001")),
		optProhibitedSingle("provider", optNode("PARTY_SELF", "")),
		optProhibitedMultiple("other_participations", optNode("PARTICIPATION", "")))
	context := optNode("EVENT_CONTEXT", "",
		optProhibitedSingle("end_time", optNode("DV_DATE_TIME", "")),
		optProhibitedSingle("health_care_facility", optNode("PARTY_IDENTIFIED", "")),
		optProhibitedMultiple("participations", optNode("PARTICIPATION", "")))
	c := compileOPTText(t, optTemplate("COMPOSITION",
		optSingle("context", context),
		optProhibitedSingle("feeder_audit", optNode("FEEDER_AUDIT", "")),
		optCardinal("content", 0, -1, observation)), true)
	for _, opts := range corpusOptions(instance.Large) {
		t.Run(opts.ValueFill.String(), func(t *testing.T) {
			comp := generateComposition(t, c, opts)
			if len(comp.Content) != 1 {
				t.Fatalf("content holds %d members, want 1", len(comp.Content))
			}
			obs, ok := comp.Content[0].(*rm.Observation)
			if !ok {
				t.Fatalf("content member is %T, want *rm.Observation", comp.Content[0])
			}
			ctx := comp.Context
			if ctx == nil {
				t.Fatal("context is absent")
			}
			if comp.FeederAudit != nil || ctx.EndTime != nil || ctx.HealthCareFacility != nil ||
				len(ctx.Participations) != 0 || obs.Provider != nil || len(obs.OtherParticipations) != 0 {
				t.Errorf("Generate(Large) wrote an RM attribute the template prohibits: feeder_audit=%v end_time=%v health_care_facility=%v participations=%d provider=%v other_participations=%d",
					comp.FeederAudit, ctx.EndTime, ctx.HealthCareFacility, len(ctx.Participations), obs.Provider, len(obs.OtherParticipations))
			}
			noValidationErrors(t, comp, c)
			noFloorErrors(t, comp)
		})
	}
}

// TestREQ107_LargeBuildsANamedRMAttributeAsTheVisitDoes is the REQ-107
// check that, where the template names one of Large's optional RM
// attributes and allows it, Large builds it as the visit builds any
// attribute the template names, under the template's constraints: a
// health_care_facility whose name the template fixes takes that name, not
// the example name Large gives one the template does not name.
func TestREQ107_LargeBuildsANamedRMAttributeAsTheVisitDoes(t *testing.T) {
	const clinic = "Clinic"
	facility := optNode("PARTY_IDENTIFIED", "", optSingle("name", optPrimitive("STRING", "C_STRING", "<list>"+clinic+"</list>")))
	context := optNode("EVENT_CONTEXT", "", optOptionalSingleOver("health_care_facility", facility))
	c := compileOPTText(t, optTemplate("COMPOSITION", optSingle("context", context),
		optCardinal("content", 0, -1, optObservationUse("", 0, 1))), true)
	for _, opts := range corpusOptions(instance.Large) {
		t.Run(opts.ValueFill.String(), func(t *testing.T) {
			comp := generateComposition(t, c, opts)
			if comp.Context == nil {
				t.Fatal("context is absent")
			}
			hcf, ok := comp.Context.HealthCareFacility.(*rm.PartyIdentified)
			if !ok || hcf.Name == nil || *hcf.Name != clinic {
				t.Errorf("context health_care_facility = %#v, want a PARTY_IDENTIFIED named %q", comp.Context.HealthCareFacility, clinic)
			}
			noValidationErrors(t, comp, c)
			noFloorErrors(t, comp)
		})
	}
}

// TestREQ107_LargeWritesActivityTiming is the REQ-107 check that, under
// Large, an ACTIVITY whose template does not name its timing gets one: a
// DV_PARSABLE in the ISO 8601 recurring-interval form the RM prefers for
// it, every day from the clock, with the formalism ISO8601. An ACTIVITY
// whose template prohibits timing gets none, and so does one under Medium
// or in the composition builder's seed (REQ-101). The INSTRUCTION, an
// ENTRY, gets its provider and other participation under Large as well.
// Every root passes the template validator and the RM floor, under both
// value fills.
func TestREQ107_LargeWritesActivityTiming(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// timed says Large writes the timing.
		timed bool
	}{
		{name: "timing not named", opt: activityInstructionOPT(), timed: true},
		{name: "timing prohibited", opt: activityInstructionOPT(optProhibitedSingle("timing", optNode("DV_PARSABLE", "")))},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		for _, run := range rmAttributeRuns {
			for _, opts := range optionsFor([]instance.Policy{run.policy}) {
				t.Run(fmt.Sprintf("%s/%s/%v", tc.name, run.name, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(run.ctx(t.Context()), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					timing := onlyActivity(t, out).Timing
					switch {
					case !run.writes || !tc.timed:
						if timing != nil {
							t.Errorf("Generate(%s) timing = %+v, want none", run.name, *timing)
						}
					case timing == nil:
						t.Errorf("Generate(%s) timing is absent, want the ISO 8601 recurring interval", run.name)
					default:
						if want := "R/" + nowText(opts) + "/P1D"; timing.Value != want || timing.Formalism != "ISO8601" {
							t.Errorf("Generate(%s) timing = %q in %q, want %q in ISO8601", run.name, timing.Value, timing.Formalism, want)
						}
					}
					in := out.(*rm.Instruction)
					if wrote := in.Provider != nil || len(in.OtherParticipations) > 0; wrote != run.writes {
						t.Errorf("Generate(%s) INSTRUCTION provider=%v other_participations=%d, want them written: %t",
							run.name, in.Provider, len(in.OtherParticipations), run.writes)
					}
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
					noFloorErrors(t, out)
				})
			}
		}
	}
}

// TestREQ107_LargeWritesOptionalRMAttributesWhereNoOPTNodeDescribes is the
// REQ-107 check of Optional RM attributes under Large that Large writes them
// on an object no OPT node describes, as on one whose OPT node does not name
// them: the OBSERVATION that fills a required slot gets its provider and
// one other participation, the EVENT_CONTEXT built for a context the
// template names with no child gets its end_time, health_care_facility and
// one participation, and the ACTIVITY built from
// the BMM type of activities the template requires with no child gets its
// timing. Medium writes none of them, nor does the composition builder's
// seed (REQ-101). Every root passes the template validator and the RM
// floor, under both value fills.
func TestREQ107_LargeWritesOptionalRMAttributesWhereNoOPTNodeDescribes(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// written reports, for each optional RM attribute the case checks,
		// whether the generated root holds it.
		written func(t *testing.T, out any) map[string]bool
	}{
		{
			name: "slot-filled OBSERVATION",
			opt:  optTemplate("COMPOSITION", optMultiple("content", optSlot("OBSERVATION", "at0001"))),
			written: func(t *testing.T, out any) map[string]bool {
				t.Helper()
				comp, err := instance.AsComposition(out)
				if err != nil {
					t.Fatalf("AsComposition: %v", err)
				}
				if len(comp.Content) != 1 {
					t.Fatalf("content holds %d members, want 1", len(comp.Content))
				}
				obs, ok := comp.Content[0].(*rm.Observation)
				if !ok {
					t.Fatalf("content member is %T, want *rm.Observation", comp.Content[0])
				}
				if len(obs.OtherParticipations) == 1 {
					checkExampleParticipation(t, "OBSERVATION other participation", obs.OtherParticipations[0])
				}
				_, self := obs.Provider.(*rm.PartySelf)
				return map[string]bool{"provider": self, "other_participations": len(obs.OtherParticipations) == 1}
			},
		},
		{
			name: "EVENT_CONTEXT of a context named with no child",
			opt:  optTemplate("COMPOSITION", optSingle("context")),
			written: func(t *testing.T, out any) map[string]bool {
				t.Helper()
				comp, err := instance.AsComposition(out)
				if err != nil {
					t.Fatalf("AsComposition: %v", err)
				}
				ctx := comp.Context
				if ctx == nil {
					t.Fatal("context is absent")
				}
				if len(ctx.Participations) == 1 {
					checkExampleParticipation(t, "context participation", ctx.Participations[0])
				}
				hcf, named := ctx.HealthCareFacility.(*rm.PartyIdentified)
				return map[string]bool{
					"end_time":             ctx.EndTime != nil,
					"health_care_facility": named && hcf.Name != nil && *hcf.Name == "example",
					"participations":       len(ctx.Participations) == 1,
				}
			},
		},
		{
			name: "ACTIVITY built from its BMM type",
			opt: optTemplate("INSTRUCTION", append(entryAttrs,
				optSingle("narrative", optNode("DV_TEXT", "")), optMultiple("activities"))...),
			written: func(t *testing.T, out any) map[string]bool {
				t.Helper()
				timing := onlyActivity(t, out).Timing
				return map[string]bool{"timing": timing != nil && timing.Formalism == "ISO8601"}
			},
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		for _, run := range rmAttributeRuns {
			for _, opts := range corpusOptions(run.policy) {
				t.Run(fmt.Sprintf("%s/%s/%v", tc.name, run.name, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(run.ctx(t.Context()), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					for attr, wrote := range tc.written(t, out) {
						if wrote != run.writes {
							t.Errorf("Generate(%s) wrote %s: %t, want %t", run.name, attr, wrote, run.writes)
						}
					}
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
					noFloorErrors(t, out)
				})
			}
		}
	}
}

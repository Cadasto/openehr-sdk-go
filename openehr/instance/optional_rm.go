package instance

import (
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// participationFunctionDefault is the code of the RM default of a
// PARTICIPATION's function, openehr 253|unknown| (ruleParticipationFunc).
const participationFunctionDefault = "253"

// exampleSystemID is the system_id of the originating system audit Large
// writes in a COMPOSITION's feeder_audit: a placeholder, as the template
// gives no value for it.
const exampleSystemID = "example"

// timingFormalism and timingPeriod make the timing Large writes on an
// ACTIVITY: an ISO 8601 recurring interval, the form RM ACTIVITY.timing
// prefers ("the preferred syntax is ISO8601 'R' format"), that repeats every
// day from the clock with no end.
const (
	timingFormalism = "ISO8601"
	timingPeriod    = "P1D"
)

// writeOptionalRMAttributes writes, where the generator builds full Large
// (fullLarge), the optional RM attributes of rmValue that Large adds and
// that opt, the OPT node of rmValue, does not name: opt is nil for a value
// no OPT node describes, such as the EVENT_CONTEXT applyCompositionDefaults
// builds where the template names no context, an ENTRY that fills a slot,
// or an ACTIVITY built from its BMM type (settleBuilt). They are:
//
//   - a COMPOSITION's feeder_audit, whose originating_system_audit has the
//     system_id exampleSystemID and the clock as its time;
//   - an EVENT_CONTEXT's end_time, from the clock, its health_care_facility,
//     a PARTY_IDENTIFIED named example, and one participations member;
//   - an ENTRY's provider, a PARTY_SELF, and one other_participations member;
//   - an ACTIVITY's timing, an ISO 8601 recurring interval.
//
// Each participation is the one exampleParticipation builds. An attribute
// opt names is the walk's, which visits it under Large where the template
// allows it and leaves it out where the template prohibits it, so nothing
// here writes over what the template constrains. An attribute that already
// holds a value is left as it is.
func (g *generator) writeOptionalRMAttributes(opt *tcimpl.CompiledNode, rmValue any) {
	if !g.fullLarge {
		return
	}
	switch v := rmValue.(type) {
	case *rm.Composition:
		if unnamed(opt, "feeder_audit") && v.FeederAudit == nil {
			v.FeederAudit = &rm.FeederAudit{OriginatingSystemAudit: rm.FeederAuditDetails{
				SystemID: exampleSystemID,
				Time:     &rm.DVDateTime{Value: g.dateTimeDefault()},
			}}
		}
	case *rm.EventContext:
		if unnamed(opt, "end_time") && v.EndTime == nil {
			v.EndTime = &rm.DVDateTime{Value: g.dateTimeDefault()}
		}
		if unnamed(opt, "health_care_facility") && (v.HealthCareFacility == nil || rm.IsTypedNil(v.HealthCareFacility)) {
			facility := &rm.PartyIdentified{}
			fillPartyName(&facility.Name, 0, nil)
			v.HealthCareFacility = facility
		}
		if unnamed(opt, "participations") && len(v.Participations) == 0 {
			v.Participations = []rm.Participation{g.exampleParticipation()}
		}
	case *rm.Activity:
		if unnamed(opt, "timing") && v.Timing == nil {
			v.Timing = &rm.DVParsable{
				Value:     "R/" + g.dateTimeDefault() + "/" + timingPeriod,
				Formalism: timingFormalism,
			}
		}
	case *rm.Observation:
		g.writeEntryParties(opt, &v.Provider, &v.OtherParticipations)
	case *rm.Evaluation:
		g.writeEntryParties(opt, &v.Provider, &v.OtherParticipations)
	case *rm.Instruction:
		g.writeEntryParties(opt, &v.Provider, &v.OtherParticipations)
	case *rm.Action:
		g.writeEntryParties(opt, &v.Provider, &v.OtherParticipations)
	case *rm.AdminEntry:
		g.writeEntryParties(opt, &v.Provider, &v.OtherParticipations)
	}
}

// writeEntryParties writes an ENTRY's provider, a PARTY_SELF, and one member
// of its other_participations, where opt, the ENTRY's OPT node, does not
// name the attribute and it holds no value yet.
func (g *generator) writeEntryParties(opt *tcimpl.CompiledNode, provider *rm.PartyProxy, others *[]rm.Participation) {
	if unnamed(opt, "provider") && (*provider == nil || rm.IsTypedNil(*provider)) {
		*provider = &rm.PartySelf{}
	}
	if unnamed(opt, "other_participations") && len(*others) == 0 {
		*others = []rm.Participation{g.exampleParticipation()}
	}
}

// exampleParticipation is the PARTICIPATION Large writes where the template
// names none: a PARTY_SELF performer, the RM default of an abstract
// PARTY_PROXY; a mode coded with its RM default, openehr 193|not
// specified|, with its rubric (settleCoded), so Mode_valid holds; and a
// function written as a DV_TEXT, the type the BMM gives it, whose text is
// the rubric of the function's RM default, openehr 253|unknown|. The
// function is not coded because the FLAT format carries a participation's
// function as plain text only, so a coded one would make the root one
// FLAT cannot carry; Function_valid binds a coded function alone.
func (g *generator) exampleParticipation() rm.Participation {
	function, _ := terminology.ParticipationFunction.Rubric(participationFunctionDefault)
	p := rm.Participation{
		Function:  &rm.DVText{Value: function},
		Mode:      &rm.DVCodedText{},
		Performer: &rm.PartySelf{},
	}
	g.settleCoded(nil, &p)
	return p
}

// unnamed reports whether opt, an OPT node or nil, does not name the
// attribute attrName.
func unnamed(opt *tcimpl.CompiledNode, attrName string) bool {
	return opt == nil || opt.Attribute(attrName) == nil
}

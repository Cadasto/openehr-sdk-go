package instance_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// historyEvent is a POINT_EVENT child at0002 of a HISTORY's events, at
// occurrences 0..*, with the attributes the RM requires.
var historyEvent = optOccurring("C_COMPLEX_OBJECT", "POINT_EVENT", "at0002", 0, -1,
	optSingle("time", optNode("DV_DATE_TIME", "")),
	optSingle("data", optNode("ITEM_TREE", "at0003")))

// historySummary is a HISTORY's summary at existence 0..1 over an
// ITEM_TREE at0004.
var historySummary = guardSingle("summary", guardExistence01, optNode("ITEM_TREE", "at0004"))

// historyExistence00 is an existence of 0..0: the OPT prohibits the
// attribute.
var historyExistence00 = `<existence>` + optInterval(0, 0) + `</existence>`

// historyShape renders what a generated HISTORY carries: the archetype node
// ids of its events, then "summary=" and its summary's, "" for none.
func historyShape(t *testing.T, out any) string {
	t.Helper()
	h, ok := out.(*rm.History[rm.ItemStructure])
	if !ok {
		t.Fatalf("generated root is %T, want *rm.History[rm.ItemStructure]", out)
	}
	ids := make([]string, 0, len(h.Events))
	for _, ev := range h.Events {
		ids = append(ids, nodeID(ev))
	}
	summary := ""
	if h.Summary != nil && !rm.IsTypedNil(h.Summary) {
		summary = nodeID(h.Summary)
	}
	return fmt.Sprintf("events=%v summary=%s", ids, summary)
}

// TestREQ107_HistoryGetsAnEventOrASummary is the REQ-107 check of
// Precedence 2 for HISTORY Events_valid, (events /= Void and then not
// events.is_empty) or summary /= Void (REQ-112 Non-empty lists). A HISTORY
// the walk leaves with neither, as Required leaves an optional events
// alone, gets the first of these the OPT allows: a member of the first
// allowed child of events that is not a slot; the summary the first allowed
// child of summary that is not a slot builds; a member of events built from
// its BMM type, where the OPT names no child under events; a summary built
// from its BMM type, where it names no child under summary. An attribute the
// OPT prohibits allows none, and where the OPT allows none of them the
// HISTORY gets neither, the one case whose root breaks the rule
// (Exceptions and known gaps, An OPT that contradicts itself). Under Large and Medium the walk writes an event
// already, and a summary the OPT allows beside it. Every root but the last passes the RM floor and the template
// validator, under every policy, both value fills and both compile modes.
func TestREQ107_HistoryGetsAnEventOrASummary(t *testing.T) {
	origin := optSingle("origin", optNode("DV_DATE_TIME", ""))
	cases := []struct {
		name string
		opt  string
		// want is what Required builds, and what Large and Medium build
		// where visited is empty.
		want, visited string
		// breaks names the one floor rule the root breaks, empty for none.
		breaks string
	}{
		{
			name: "optional events",
			opt:  optTemplate("HISTORY", origin, guardMultiple("events", guardExistence01, historyEvent)),
			want: "events=[at0002] summary=",
		},
		{
			name:    "optional events beside an optional summary",
			opt:     optTemplate("HISTORY", origin, guardMultiple("events", guardExistence01, historyEvent), historySummary),
			want:    "events=[at0002] summary=",
			visited: "events=[at0002] summary=at0004",
		},
		{
			name: "an optional summary, events not named",
			opt:  optTemplate("HISTORY", origin, historySummary),
			want: "events=[] summary=at0004",
		},
		{
			name: "an optional summary, events prohibited",
			opt:  optTemplate("HISTORY", origin, guardMultiple("events", historyExistence00, historyEvent), historySummary),
			want: "events=[] summary=at0004",
		},
		{
			name: "events and summary not named",
			opt:  optTemplate("HISTORY", origin),
			want: "events=[at0000] summary=",
		},
		{
			name: "events named with no child, summary not named",
			opt:  optTemplate("HISTORY", origin, guardMultiple("events", guardExistence01)),
			want: "events=[at0000] summary=",
		},
		{
			name: "events prohibited, summary not named",
			opt:  optTemplate("HISTORY", origin, guardMultiple("events", historyExistence00, historyEvent)),
			want: "events=[] summary=at0000",
		},
		{
			name: "neither allowed",
			opt: optTemplate("HISTORY", origin, guardMultiple("events", historyExistence00, historyEvent),
				guardSingle("summary", historyExistence00, optNode("ITEM_TREE", "at0004"))),
			want:   "events=[] summary=",
			breaks: "HISTORY.Events_valid",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					want := tc.want
					if tc.visited != "" && opts.Policy != instance.Required {
						want = tc.visited
					}
					if got := historyShape(t, out); got != want {
						t.Errorf("Generate built %s, want %s", got, want)
					}
					if tc.breaks == "" {
						checkValid(t, out, c)
						return
					}
					var rules []string
					for _, iss := range validation.ValidateRM(out).Issues {
						if iss.Severity == validation.Error {
							rules = append(rules, iss.Rule+" @ "+iss.Path)
						}
					}
					if want := []string{tc.breaks + " @ /"}; !slices.Equal(rules, want) {
						t.Errorf("ValidateRM errors = %q, want %q", rules, want)
					}
				})
			}
		}
	}
}

// TestREQ107_HistoryBuiltFromItsBMMTypeGetsAnEvent is the REQ-107 check of
// Precedence 2 for a HISTORY no OPT node describes: an OBSERVATION whose
// OPT leaves its data silent gets a HISTORY built from its BMM type, and
// that HISTORY gets one event built from its BMM type, since the OPT names
// no child under its events, so RM Events_valid holds (REQ-112 Non-empty
// lists). The root passes the RM floor and the template validator, under
// every policy, both value fills and both compile modes.
func TestREQ107_HistoryBuiltFromItsBMMTypeGetsAnEvent(t *testing.T) {
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, optTemplate("OBSERVATION"), implicit)
		for _, opts := range guardOptions() {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				obs, ok := out.(*rm.Observation)
				if !ok {
					t.Fatalf("generated root is %T, want *rm.Observation", out)
				}
				if n := len(obs.Data.Events); n != 1 {
					t.Errorf("OBSERVATION.data.events has %d members, want 1", n)
				}
				checkValid(t, out, c)
			})
		}
	}
}

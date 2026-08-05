package webtemplate_test

import (
	"cmp"
	"flag"
	"fmt"
	"slices"
	"strings"
	"testing"

	conformance "github.com/cadasto/openehr-sdk-go/testkit/conformance/webtemplate"
)

var census = flag.Bool("census", false, "print the corpus conformance census instead of asserting")

// TestUpstreamFlatParity — PROBE-086. For every body in the pinned upstream
// EHRbase FLAT corpus, decode it through the REQ-053 codec and re-encode it;
// the modelled subset must come back byte-for-byte on key and value.
//
// "Modelled subset" is not a hand-drawn line: it is whatever the codec did
// not refuse (see the package doc). Each fixture's excluded count is pinned
// below, so a gap closing shows up as a deliberate edit here and a gap
// *opening* fails the test.
func TestUpstreamFlatParity(t *testing.T) {
	target, err := conformance.NewTarget()
	if err != nil {
		t.Fatalf("build target: %v", err)
	}
	cases, err := conformance.Cases()
	if err != nil {
		t.Fatalf("enumerate cases: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no fixtures found — corpus layout changed?")
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			rep, err := conformance.Run(target, c)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			// The modelled subset must round-trip exactly. An undiagnosed
			// drop, a key we spell differently, or a changed value is a
			// failure — never a skip.
			for _, m := range rep.Missing {
				t.Errorf("upstream key decoded but not re-emitted: %s", m)
			}
			for _, e := range rep.Extra {
				t.Errorf("key emitted that upstream does not have: %s", e)
			}
			for _, m := range rep.Mismatched {
				t.Errorf("value differs: %s", m)
			}

			want, ok := pinned[c.Name]
			if !ok {
				t.Fatalf("fixture %q is not pinned — add it to `pinned` (see -census)", c.Name)
			}
			if rep.Excluded != want.excluded {
				t.Errorf("excluded %d upstream keys, pinned at %d — the unmodelled surface moved; "+
					"re-run with -census, update the pin, and record the change in SKIPPED.md",
					rep.Excluded, want.excluded)
			}
			if rep.Compared != want.compared {
				t.Errorf("compared %d upstream keys, pinned at %d — coverage moved; "+
					"re-run with -census and update the pin", rep.Compared, want.compared)
			}
		})
	}
}

// TestPinnedCoversCorpus keeps the ratchet and the corpus in step in the
// direction the per-fixture loop cannot see. A fixture *added* upstream fails
// there ("not pinned"); a fixture *removed* would just stop being asserted,
// silently shrinking the suite, and a pin left behind after a rename would sit
// there looking like coverage. Both are caught here.
func TestPinnedCoversCorpus(t *testing.T) {
	cases, err := conformance.Cases()
	if err != nil {
		t.Fatalf("enumerate cases: %v", err)
	}
	present := make(map[string]bool, len(cases))
	for _, c := range cases {
		present[c.Name] = true
	}
	for name := range pinned {
		if !present[name] {
			t.Errorf("pinned fixture %q is not in the corpus — stale pin, or the fixture was renamed", name)
		}
	}
	if len(cases) != len(pinned) {
		t.Errorf("corpus has %d fixtures, %d are pinned", len(cases), len(pinned))
	}
}

// TestCensus prints the corpus-wide conformance picture. Not an assertion —
// it is how SKIPPED.md is regenerated:
//
//	go test ./testkit/conformance/webtemplate/ -run TestCensus -census -v
func TestCensus(t *testing.T) {
	if !*census {
		t.Skip("pass -census to print the conformance census")
	}
	target, err := conformance.NewTarget()
	if err != nil {
		t.Fatalf("build target: %v", err)
	}
	cases, err := conformance.Cases()
	if err != nil {
		t.Fatalf("enumerate cases: %v", err)
	}

	var total, excluded, compared, meta int
	byReason := map[string]int{}
	keysByReason := map[string]int{}
	var lines []string
	for _, c := range cases {
		rep, err := conformance.Run(target, c)
		if err != nil {
			t.Fatalf("run %s: %v", c.Name, err)
		}
		total += rep.Total
		excluded += rep.Excluded
		compared += rep.Compared
		meta += rep.Meta
		for _, r := range rep.Refusals {
			byReason[r.Reason]++
			keysByReason[r.Reason] += r.Keys
		}
		status := "OK"
		if !rep.Clean() {
			status = fmt.Sprintf("DIRTY missing=%d extra=%d mismatch=%d",
				len(rep.Missing), len(rep.Extra), len(rep.Mismatched))
		}
		lines = append(lines, fmt.Sprintf("%-56s total=%-5d excluded=%-5d compared=%-4d %s",
			rep.Case, rep.Total, rep.Excluded, rep.Compared, status))
		for _, m := range rep.Missing {
			lines = append(lines, "      MISSING  "+m)
		}
		for _, e := range rep.Extra {
			lines = append(lines, "      EXTRA    "+e)
		}
		for _, m := range rep.Mismatched {
			lines = append(lines, "      MISMATCH "+m)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\ncorpus: %d fixtures, %d keys — %d compared, %d excluded, %d metadata held out\n",
		len(cases), total, compared, excluded, meta)
	fmt.Fprintf(&b, "coverage: %.1f%% of upstream keys are in the modelled subset\n\n",
		100*float64(compared)/float64(total))
	fmt.Fprintf(&b, "%-6s %-8s %s\n", "FAMS", "KEYS", "REASON")
	reasons := make([]string, 0, len(byReason))
	for r := range byReason {
		reasons = append(reasons, r)
	}
	// Total order: keys descending, then the reason text ascending, so the
	// census is byte-identical run to run and diffable across commits.
	// cmp.Compare rather than a subtraction: the counts are bounded here, but a
	// difference-as-comparator is only ever accidentally correct.
	slices.SortFunc(reasons, func(a, b string) int {
		return cmp.Or(cmp.Compare(keysByReason[b], keysByReason[a]), strings.Compare(a, b))
	})
	for _, r := range reasons {
		fmt.Fprintf(&b, "%-6d %-8d %s\n", byReason[r], keysByReason[r], r)
	}
	b.WriteString("\nper fixture:\n")
	for _, l := range lines {
		b.WriteString("  " + l + "\n")
	}
	t.Log(b.String())
}

// expectation is the pinned conformance position for one fixture.
type expectation struct {
	// excluded is how many upstream keys the codec refused (the unmodelled
	// surface). Shrinks as REQ-053 gaps close.
	excluded int
	// compared is how many upstream keys reached the comparison. Grows as
	// gaps close — the number that matters.
	compared int
}

// pinned is the corpus conformance position, per fixture. It is a ratchet,
// not a waiver: every number here is expected to move only when a REQ-053 or
// REQ-121 gap is deliberately closed, and the accompanying SKIPPED.md entry
// updated. Regenerate with:
//
//	go test ./testkit/conformance/webtemplate/ -run TestCensus -census -v
var pinned = map[string]expectation{
	"ehrbase_conformance_Element_feeder_audit":                {excluded: 52, compared: 15},
	"ehrbase_conformance_Element_null_flavor":                 {excluded: 4, compared: 13},
	"ehrbase_conformance_action":                              {excluded: 63, compared: 22},
	"ehrbase_conformance_admin_entry":                         {excluded: 49, compared: 17},
	"ehrbase_conformance_cluster":                             {excluded: 38, compared: 13},
	"ehrbase_conformance_composition":                         {excluded: 13, compared: 16},
	"ehrbase_conformance_data_types_dv_boolean":               {excluded: 4, compared: 10},
	"ehrbase_conformance_data_types_dv_coded_text":            {excluded: 9, compared: 22},
	"ehrbase_conformance_data_types_dv_coded_text_as_dv_text": {excluded: 9, compared: 22},
	"ehrbase_conformance_data_types_dv_count":                 {excluded: 4, compared: 19},
	"ehrbase_conformance_data_types_dv_date":                  {excluded: 5, compared: 17},
	"ehrbase_conformance_data_types_dv_date_time":             {excluded: 5, compared: 17},
	"ehrbase_conformance_data_types_dv_duration":              {excluded: 4, compared: 19},
	"ehrbase_conformance_data_types_dv_ehr_uri":               {excluded: 4, compared: 10},
	"ehrbase_conformance_data_types_dv_identifier":            {excluded: 4, compared: 13},
	"ehrbase_conformance_data_types_dv_multimedia":            {excluded: 18, compared: 9},
	"ehrbase_conformance_data_types_dv_ordinal":               {excluded: 4, compared: 24},
	"ehrbase_conformance_data_types_dv_parsable":              {excluded: 10, compared: 9},
	"ehrbase_conformance_data_types_dv_proportion":            {excluded: 9, compared: 30},
	"ehrbase_conformance_data_types_dv_quantity":              {excluded: 4, compared: 38},
	"ehrbase_conformance_data_types_dv_text":                  {excluded: 9, compared: 20},
	"ehrbase_conformance_data_types_dv_time":                  {excluded: 5, compared: 17},
	"ehrbase_conformance_data_types_dv_uri":                   {excluded: 4, compared: 10},
	"ehrbase_conformance_data_types_interval_dv_quantity":     {excluded: 14, compared: 10},
	"ehrbase_conformance_evaluation":                          {excluded: 49, compared: 22},
	"ehrbase_conformance_feeder_audit_multimedia":             {excluded: 50, compared: 25},
	"ehrbase_conformance_instruction":                         {excluded: 54, compared: 24},
	"ehrbase_conformance_interval_event":                      {excluded: 42, compared: 11},
	"ehrbase_conformance_observation":                         {excluded: 51, compared: 25},
	"ehrbase_conformance_party_identified":                    {excluded: 106, compared: 16},
	"ehrbase_conformance_party_related":                       {excluded: 130, compared: 16},
	"ehrbase_conformance_party_self":                          {excluded: 23, compared: 16},
	"ehrbase_conformance_point_event":                         {excluded: 37, compared: 11},
	"ehrbase_conformance_section":                             {excluded: 37, compared: 17},
}

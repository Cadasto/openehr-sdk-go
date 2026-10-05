package serializeprobes

// PROBE-105: upstream cross-format parity (REQ-080; exercises REQ-052,
// REQ-053 and REQ-056). For one set of the vendored cross-format corpus, carry
// the composition from each format the set gives to each other through the
// SDK codecs, compare with the upstream sibling, and require every leg's
// outcome to equal the outcome recorded for it.
//
// What this adds over PROBE-086 is the pairing. PROBE-086 round-trips upstream
// FLAT through the FLAT codec alone, so it cannot see a value the canonical
// codecs and the FLAT codec disagree on, or a canonical XML document the JSON
// codec reads and the XML codec does not. Here one upstream composition is
// read in every format it is given in.
//
// This is a thin wrapper: the engine, the recorded outcomes and the census
// live in testkit/conformance/crossformat, so the same runner backs both this
// probe and the package's own tests. CENSUS.md there publishes every outcome
// and why it is what it is.
//
// Modes: In-repo (parity property against vendored fixtures; no backend).

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/testkit/conformance/crossformat"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Probe105CrossFormatParity runs every leg the set carries and compares each
// leg's outcome with the outcome recorded for it in
// [crossformat.Recorded]. The caller owns corpus I/O: enumerate with
// [fixtures.ListCrossFormatSets] once and pass each set in.
//
// Status is "pass" when every leg the set runs matches its record and the
// set's records obey the record rules: a reason for every refusal,
// difference and non-zero excluded count, and at least one key compared by
// every leg that is not refused. Otherwise it is "fail", with a Detail
// naming each set and leg whose recorded and measured outcomes differ: a
// gap that opened or closed without its record changing with it. A harness
// fault, such as an OPT that does not compile or an upstream document that
// is not JSON, is a "fail" too.
//
// Framework misuse (a set with no name, no OPT, or fewer than two formats
// that share a leg) returns a non-nil error.
func Probe105CrossFormatParity(set fixtures.CrossFormatSet) (Result, error) { // PROBE-105 (REQ-080, REQ-052, REQ-053, REQ-056)
	r := Result{Probe: "PROBE-105"}
	switch {
	case set.Name == "":
		return r, errors.New("PROBE-105: set has no name")
	case set.OPT == "":
		return r, fmt.Errorf("PROBE-105: set %q has no OPT", set.Name)
	case len(crossformat.Legs(set)) == 0:
		return r, fmt.Errorf("PROBE-105: set %q carries fewer than two formats that share a leg", set.Name)
	}

	recs, ok := crossformat.Recorded[set.Name]
	if !ok {
		r.Status, r.Detail = "fail", fmt.Sprintf("set %s has no recorded outcomes: record every leg it runs in crossformat.Recorded", set.Name)
		return r, nil
	}
	if err := crossformat.CheckSetRecords(set, recs); err != nil {
		r.Status, r.Detail = "fail", "recorded outcomes break the record rules: "+err.Error()
		return r, nil
	}

	res, err := crossformat.Run(set)
	if err != nil {
		// A harness fault is a probe failure, not a skip: the corpus or the
		// codecs moved in a way the runner cannot characterise.
		r.Status, r.Detail = "fail", "run: "+err.Error()
		return r, nil
	}
	if mismatches := crossformat.Verify(res, recs); len(mismatches) > 0 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("%d of %d legs differ from their records (change the record in the same commit as the gap): %s",
			len(mismatches), len(res.Legs), strings.Join(mismatches, "; "))
		return r, nil
	}

	legs := make([]string, 0, len(res.Legs))
	for _, lr := range res.Legs {
		outcome := lr.Outcome.String()
		if lr.Outcome.Refused != "" {
			outcome = "refused as recorded"
		}
		legs = append(legs, fmt.Sprintf("%s (%s)", lr.Leg, outcome))
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("%d legs match their records: %s", len(res.Legs), strings.Join(legs, "; "))
	return r, nil
}

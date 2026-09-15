package instance_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/wireequiv"
)

// TestCorpusRoundTripValidates is the REQ-107 dossier acceptance
// criterion exercised on the real-world OPT corpus: a generated
// composition survives a canonical-JSON round-trip (marshal → decode)
// intact. Two complementary assertions, because the two REQ-052
// sub-gaps fail differently:
//
//   - value-stability: re-decoding the re-marshalled tree must reproduce the
//     same typed value. This catches sub-gap A (a value-in-interface field
//     dropping its `_type` on the wire) even when the drop never escalates
//     into a validator issue, which it doesn't on most fixtures, so the
//     delta check alone would miss it.
//   - validation delta: the decoded tree must validate with no *new* issue
//     vs the freshly generated tree. This catches sub-gap B (a
//     round-tripped DV_INTERVAL<T> collapsing to the bare
//     DVInterval[DVOrdered] and spuriously failing rm_type_mismatch).
//     Comparing against a baseline keeps it robust to policy-driven
//     findings unrelated to the round-trip (e.g. social.opt's
//     colliding-at0000 content alternatives).
func TestCorpusRoundTripValidates(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"vital_signs", "social", "Referral Request.v1", "Demonstration.v1"} {
		t.Run(name, func(t *testing.T) {
			c := compileRealWorldFixture(t, name)
			composer := "Test Composer"
			out, err := instance.Generate(context.Background(), c, instance.Options{
				Policy:    instance.Example,
				Territory: "NL",
				Composer:  &rm.PartyIdentified{Name: &composer},
				Now:       now,
				UIDSource: counterUID(),
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			comp, err := instance.AsComposition(out)
			if err != nil {
				t.Fatalf("AsComposition: %v", err)
			}

			// Baseline issues on the freshly generated tree, for the delta
			// assertion below.
			before := issueCounts(validation.ValidateComposition(comp, c).Issues)

			data, err := canjson.Marshal(comp)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var rt rm.Composition // A
			if err := canjson.Unmarshal(data, &rt); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			// Value-stability (sub-gap A): re-marshalling the decoded tree and
			// decoding it again must reproduce the same typed value. A dropped
			// subtype/bound `_type` surfaces here (B would hold a different
			// dynamic type in the slot) even when it never reaches the
			// validator. Byte equality is not asserted (member order is not a
			// contract, REQ-052).
			again, err := canjson.Marshal(&rt)
			if err != nil {
				t.Fatalf("re-marshal: %v", err)
			}
			var rt2 rm.Composition // B, straddling the re-encode
			if err := canjson.Unmarshal(again, &rt2); err != nil {
				t.Fatalf("re-decode: %v", err)
			}
			if !reflect.DeepEqual(rt, rt2) {
				if _, diff := wireequiv.Equivalent(data, again); diff != "" {
					t.Errorf("round trip not value-stable (a subtype/bound _type likely dropped): %s", diff)
				} else {
					// Wire-equivalent yet the typed values differ: the difference
					// is below the wire (a dropped subtype field that re-encodes
					// the same), so the diff is empty. Print both encodes so the
					// failure still diagnoses.
					t.Errorf("round trip not value-stable (a subtype/bound _type likely dropped); documents wire-equivalent but typed values differ\nfirst encode=%s\nsecond encode=%s", data, again)
				}
			}

			// Validation delta (sub-gap B): the round-trip must introduce no
			// new issue of any code — rm_type_mismatch in particular.
			after := validation.ValidateComposition(&rt, c)
			for code, n := range issueCounts(after.Issues) {
				if n > before[code] {
					t.Errorf("round-trip introduced %d new %q issue(s) (baseline %d)", n-before[code], code, before[code])
				}
			}
		})
	}
}

func issueCounts(issues []validation.Issue) map[string]int {
	m := make(map[string]int, len(issues))
	for _, iss := range issues {
		m[iss.Code]++
	}
	return m
}

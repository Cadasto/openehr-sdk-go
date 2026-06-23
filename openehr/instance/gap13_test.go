package instance_test

import (
	"context"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestGAP13_CorpusRoundTripValidates is the SDK-GAP-13 dossier acceptance
// criterion exercised on the real-world OPT corpus: a generated
// composition marshalled to canonical JSON and decoded back must still
// validate with no *new* issues — and specifically no rm_type_mismatch.
//
// That spurious mismatch was the ~13% failure mode both sub-gaps fix:
// sub-gap A (a value-in-interface field dropping its `_type` and decoding
// as the parent type) and sub-gap B (a round-tripped DV_INTERVAL<T>
// collapsing to the bare DVInterval[DVOrdered]). Comparing the
// pre-round-trip issue set as a baseline keeps the test robust to
// policy-driven findings unrelated to the round-trip (e.g. a corpus OPT's
// own cardinality pins).
func TestGAP13_CorpusRoundTripValidates(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"vital_signs", "social", "Referral Request.v1", "Demonstration.v1"} {
		t.Run(name, func(t *testing.T) {
			c := compileGAP12Fixture(t, name)
			composer := "Test Composer"
			out, err := instance.Generate(context.Background(), c, instance.Options{
				Policy:    instance.Example,
				Territory: "NL",
				Composer:  &rm.PartyIdentified{Name: &composer},
				Now:       now,
				UIDSource: gap14CounterUID(),
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			comp, err := instance.AsComposition(out)
			if err != nil {
				t.Fatalf("AsComposition: %v", err)
			}

			// Baseline issues on the freshly generated tree. Some corpus
			// OPTs carry policy-driven findings unrelated to the round-trip
			// (e.g. social.opt's colliding-at0000 content alternatives), so
			// GAP-13 is asserted strictly as the *delta*: the round-trip must
			// introduce no new issue of any code — rm_type_mismatch in
			// particular, the ~13% failure mode both sub-gaps fix.
			before := issueCounts(validation.ValidateComposition(comp, c).Issues)

			data, err := canjson.Marshal(comp)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var rt rm.Composition
			if err := canjson.Unmarshal(data, &rt); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
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

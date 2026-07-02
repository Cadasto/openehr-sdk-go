package instance_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestCorpusRoundTripValidates is the SDK-GAP-13 dossier acceptance
// criterion exercised on the real-world OPT corpus: a generated
// composition survives a canonical-JSON round-trip (marshal → decode)
// intact. Two complementary assertions, because the two sub-gaps fail
// differently:
//
//   - byte-stability: re-marshalling the decoded tree must reproduce the
//     original bytes. This catches sub-gap A (a value-in-interface field
//     dropping its `_type` on the wire) even when the drop never escalates
//     into a validator issue — which it doesn't on most fixtures, so the
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
			var rt rm.Composition
			if err := canjson.Unmarshal(data, &rt); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			// Byte-stability (sub-gap A): re-marshalling the decoded tree must
			// reproduce the original bytes. A dropped subtype/bound `_type`
			// surfaces here even when it never reaches the validator.
			again, err := canjson.Marshal(&rt)
			if err != nil {
				t.Fatalf("re-marshal: %v", err)
			}
			if !bytes.Equal(data, again) {
				t.Errorf("round-trip not byte-stable (a subtype/bound _type likely dropped) %s", firstDiff(data, again))
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

// firstDiff returns a bounded, human-readable description of where two
// byte slices first diverge, so a byte-stability failure points at the
// offending key instead of dumping the whole (multi-KB) payload.
func firstDiff(a, b []byte) string {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	window := func(s []byte) string {
		return string(s[max(i-40, 0):min(i+40, len(s))])
	}
	return fmt.Sprintf("at byte %d (len %d vs %d):\n  first:  …%s…\n  second: …%s…", i, len(a), len(b), window(a), window(b))
}

func issueCounts(issues []validation.Issue) map[string]int {
	m := make(map[string]int, len(issues))
	for _, iss := range issues {
		m[iss.Code]++
	}
	return m
}

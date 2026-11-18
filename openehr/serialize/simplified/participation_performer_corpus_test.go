package simplified_test

// REQ-107, REQ-140 — a PARTICIPATION the generator writes, where the template
// names one or Large adds one, carries a performer the formats spell with a
// party key. FLAT and STRUCTURED spell a PARTY_SELF performer by no party key,
// which a reader that knows only the PARTY_IDENTIFIED keys the Simplified
// Formats specification shows for a performer reads as an empty
// PARTY_IDENTIFIED, one RM Basic_validity refuses.

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

// participationBase matches a FLAT key that addresses a participation, an
// EVENT_CONTEXT's `_participation:N` or an ENTRY's `_other_participation:N`,
// and captures the key base the participation's own suffixes and its
// performer's party suffixes hang off.
var participationBase = regexp.MustCompile(`^(.*?/_(?:other_)?participation:\d+)(?:[|/].*)?$`)

// performerKeyed reports whether flat carries a performer party key on the
// participation at base: its name, its external_ref's id or an identifier's
// id.
func performerKeyed(flat map[string]any, base string) bool {
	if _, ok := flat[base+"|name"]; ok {
		return true
	}
	if _, ok := flat[base+"|id"]; ok {
		return true
	}
	for k := range flat {
		if strings.HasPrefix(k, base+"|identifiers_id:") {
			return true
		}
	}
	return false
}

// participationBases returns the key bases of the participations flat
// writes, sorted.
func participationBases(flat map[string]any) []string {
	seen := make(map[string]bool)
	for k := range flat {
		if m := participationBase.FindStringSubmatch(k); m != nil {
			seen[m[1]] = true
		}
	}
	bases := make([]string, 0, len(seen))
	for b := range seen {
		bases = append(bases, b)
	}
	slices.Sort(bases)
	return bases
}

// TestREQ107_REQ140_GeneratedParticipationsCarryTheirPerformer — REQ-107,
// REQ-140: for every COMPOSITION OPT of the corpus, under instance.Required,
// instance.Medium and instance.Large, every participation the generated
// composition's FLAT body, and the FLAT keys of its STRUCTURED body, write
// carries a performer party key: `|name`, `|id` or `|identifiers_id:N`.
func TestREQ107_REQ140_GeneratedParticipationsCarryTheirPerformer(t *testing.T) {
	opts := corpusCompositionOPTs(t)
	if len(opts) < codedNameCorpusFloor {
		t.Errorf("the corpus gives %d COMPOSITION templates, want at least %d; did the corpus shrink?", len(opts), codedNameCorpusFloor)
	}
	checked := 0
	for _, o := range opts {
		for _, policy := range []instance.Policy{instance.Required, instance.Medium, instance.Large} {
			t.Run(o.id+"/"+policy.String(), func(t *testing.T) {
				comp, wt := genCompositionUnder(t, o.path, policy)
				for _, format := range []string{"FLAT", "STRUCTURED"} {
					encode := simplified.MarshalFlat
					if format == "STRUCTURED" {
						encode = simplified.MarshalStructured
					}
					body, err := encode(comp, wt)
					if err != nil {
						if slices.Contains(reusedSiblingTemplates, o.id) && errors.Is(err, rmpath.ErrPathAmbiguous) {
							continue
						}
						t.Fatalf("Marshal %s: %v", format, err)
					}
					if format == "STRUCTURED" {
						if body, err = simplified.StructuredToFlat(body); err != nil {
							t.Fatalf("StructuredToFlat: %v", err)
						}
					}
					var flat map[string]any
					if err := json.Unmarshal(body, &flat); err != nil {
						t.Fatalf("%s: the body is no JSON object: %v", format, err)
					}
					for _, base := range participationBases(flat) {
						checked++
						if !performerKeyed(flat, base) {
							t.Errorf("%s: %s carries no performer party key (|name, |id or |identifiers_id:N)", format, base)
						}
					}
				}
			})
		}
	}
	if checked == 0 {
		t.Error("no generated body writes a participation; the guard compares nothing")
	}
	t.Logf("%d COMPOSITION templates under three policies, %d participations checked", len(opts), checked)
}

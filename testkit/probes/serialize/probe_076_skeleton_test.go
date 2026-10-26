package serializeprobes_test

// The skeleton leg of PROBE-076 (REQ-053): every COMPOSITION OPT the corpus
// vendors, under instance.Required and instance.Medium, writes a skeleton that
// comes back through FLAT and STRUCTURED, decoded with WithTemplate and
// WithStrict, valid against its OPT, apart from the findings
// Probe076SkeletonExclusions names.

import (
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	serializeprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/serialize"
)

// probe076SkeletonPolicies are the generator policies the leg runs under.
var probe076SkeletonPolicies = []instance.Policy{instance.Required, instance.Medium}

// probe076SkeletonPassFloor is the number of (template, policy) pairs that
// pass today: 64 COMPOSITION templates under two policies. A corpus that
// silently empties, or a COMPOSITION OPT that stops parsing and so drops out
// of the corpus, falls below it. Raise it when the corpus grows.
const probe076SkeletonPassFloor = 128

// compositionOPT is one COMPOSITION OPT of the corpus: its template id and its
// body.
type compositionOPT struct {
	id   string
	body []byte
}

// compositionOPTs returns every OPT fixtures.ListAllOPTs lists whose root is
// COMPOSITION, one per template id, in the order the listing gives. An OPT
// that does not parse is left out: the PROBE-027 corpus ratchet
// (corpusCompileFailures in testkit/probes/instance) names the ones that do
// not, and the pass floor catches a COMPOSITION OPT that stops parsing.
func compositionOPTs(t *testing.T) []compositionOPT {
	t.Helper()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	seen := make(map[string]bool)
	var out []compositionOPT
	for _, ref := range refs {
		body, err := os.ReadFile(ref.Path)
		if err != nil {
			t.Fatalf("read %s: %v", ref.Name, err)
		}
		opt, err := fixtures.ParseOPTBytes(body)
		if err != nil {
			continue
		}
		if root := opt.Root(); root == nil || root.RMTypeName() != "COMPOSITION" {
			continue
		}
		id := opt.TemplateID()
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, compositionOPT{id: id, body: body})
	}
	return out
}

// corpusOPT returns the body of the corpus COMPOSITION OPT whose template id
// is id.
func corpusOPT(t *testing.T, id string) []byte {
	t.Helper()
	for _, o := range compositionOPTs(t) {
		if o.id == id {
			return o.body
		}
	}
	t.Fatalf("no corpus COMPOSITION OPT has the template id %q", id)
	return nil
}

type probe076Pair struct {
	template string
	policy   instance.Policy
}

// TestProbe076SkeletonLeg runs the skeleton leg of PROBE-076 (REQ-053) over
// the corpus. Every (template, policy) pair must pass: its written skeleton
// validates, and both round trips validate with nothing but a registered
// exclusion reported. An exclusion that no longer fires fails its pair as
// stale, and one naming a pair the corpus does not run fails here.
func TestProbe076SkeletonLeg(t *testing.T) {
	opts := compositionOPTs(t)
	if len(opts) == 0 {
		t.Fatal("no COMPOSITION OPT found in the corpus; check testkit/fixtures discovery")
	}
	ran := make(map[probe076Pair]bool)
	passes := 0
	for _, o := range opts {
		for _, policy := range probe076SkeletonPolicies {
			ran[probe076Pair{o.id, policy}] = true
			t.Run(o.id+"/"+policy.String(), func(t *testing.T) {
				r, err := serializeprobes.Probe076SkeletonRoundTrip(t.Context(), o.body, policy, serializeprobes.Probe076SkeletonExclusions)
				if err != nil {
					t.Fatalf("probe framework error: %v", err)
				}
				if r.Status != probe.StatusPass {
					t.Errorf("status = %q (detail: %s); want pass", r.Status, r.Detail)
					return
				}
				passes++
			})
		}
	}
	for _, ex := range serializeprobes.Probe076SkeletonExclusions {
		if ex.Reason == "" || ex.Registered == "" {
			t.Errorf("exclusion %s carries no reason or no registration: every exclusion says why it is expected and where the gap is recorded", ex.Finding)
		}
		if !ran[probe076Pair{ex.Finding.Template, ex.Finding.Policy}] {
			t.Errorf("exclusion %s is stale: the corpus runs no template %q under %s", ex.Finding, ex.Finding.Template, ex.Finding.Policy)
		}
	}
	if passes < probe076SkeletonPassFloor {
		t.Errorf("%d (template, policy) pairs pass, want at least %d; did the corpus shrink?", passes, probe076SkeletonPassFloor)
	}
	t.Logf("%d COMPOSITION templates, %d (template, policy) pairs pass, %d registered exclusions", len(opts), passes, len(serializeprobes.Probe076SkeletonExclusions))
}

// TestProbe076SkeletonLegFailsAnUnexcludedFinding pins that a finding no
// exclusion names fails the pair, and that the failure names the template,
// the policy, the format and the finding: an encode refusal by its error text,
// an issue by its code and path.
func TestProbe076SkeletonLegFailsAnUnexcludedFinding(t *testing.T) {
	cases := []struct {
		name string
		id   string
		want []string
	}{
		{
			name: "encode refusal",
			id:   "sdk_crosscheck_siblings.v1",
			want: []string{"sdk_crosscheck_siblings.v1 medium FLAT: encode:", "sdk_crosscheck_siblings.v1 medium STRUCTURED: encode:", `FLAT id "twin" is one of several reused siblings`},
		},
		{
			name: "validation issue",
			id:   "Referral Request.v1",
			want: []string{"Referral Request.v1 medium FLAT: rm_type_mismatch at /context/other_context/items[openEHR-EHR-CLUSTER.annotations.v1]/items[at0001]/name", "Referral Request.v1 medium STRUCTURED: rm_type_mismatch at"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := serializeprobes.Probe076SkeletonRoundTrip(t.Context(), corpusOPT(t, tc.id), instance.Medium, nil)
			if err != nil {
				t.Fatalf("probe framework error: %v", err)
			}
			if r.Status != probe.StatusFail {
				t.Fatalf("status = %q (detail: %s) with no exclusion; want fail", r.Status, r.Detail)
			}
			for _, w := range tc.want {
				if !strings.Contains(r.Detail, w) {
					t.Errorf("detail = %q; want it to name %q", r.Detail, w)
				}
			}
		})
	}
}

// TestProbe076SkeletonLegFailsAStaleExclusion pins that an exclusion which no
// longer fires fails its pair as stale, naming it, and that the finding it
// stopped matching is reported as unexcluded.
func TestProbe076SkeletonLegFailsAStaleExclusion(t *testing.T) {
	const id = "Referral Request.v1"
	var exclusions []serializeprobes.Probe076Exclusion
	moved := false
	for _, ex := range serializeprobes.Probe076SkeletonExclusions {
		if ex.Finding.Template == id && ex.Finding.Format == "FLAT" {
			ex.Finding.Path = "/context/other_context/items[openEHR-EHR-CLUSTER.annotations.v1]/items[at0002]/name"
			moved = true
		}
		exclusions = append(exclusions, ex)
	}
	if !moved {
		t.Fatalf("Probe076SkeletonExclusions holds no FLAT exclusion for %s; the case asserts nothing", id)
	}
	r, err := serializeprobes.Probe076SkeletonRoundTrip(t.Context(), corpusOPT(t, id), instance.Medium, exclusions)
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != probe.StatusFail {
		t.Fatalf("status = %q (detail: %s) with a stale exclusion; want fail", r.Status, r.Detail)
	}
	for _, w := range []string{
		"stale exclusion " + id + " medium FLAT: rm_type_mismatch at /context/other_context/items[openEHR-EHR-CLUSTER.annotations.v1]/items[at0002]/name",
		id + " medium FLAT: rm_type_mismatch at /context/other_context/items[openEHR-EHR-CLUSTER.annotations.v1]/items[at0001]/name",
	} {
		if !strings.Contains(r.Detail, w) {
			t.Errorf("detail = %q; want it to name %q", r.Detail, w)
		}
	}
	if strings.Contains(r.Detail, "STRUCTURED") {
		t.Errorf("detail = %q; the STRUCTURED exclusion still fires and must not be reported", r.Detail)
	}
}

package serializeprobes_test

// The skeleton leg of PROBE-076 (REQ-053): every COMPOSITION OPT the corpus
// vendors, under instance.Required and instance.Medium, writes a skeleton that
// comes back through FLAT and STRUCTURED, decoded with WithTemplate and
// WithStrict, valid against its OPT, apart from the findings
// Probe076SkeletonExclusions names.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	serializeprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/serialize"
)

// probe076SkeletonPolicies are the generator policies the leg runs under.
var probe076SkeletonPolicies = []instance.Policy{instance.Required, instance.Medium}

// probe076SkeletonPairFloor is the number of (template, policy) pairs the
// corpus gives today: 80 COMPOSITION templates under two policies. A corpus
// that silently empties, or a COMPOSITION OPT that stops parsing and so drops
// out of the corpus, falls below it. It counts the pairs the corpus lists, not
// the ones a -run filter selects, so running one pair does not trip it; every
// pair that runs must pass on its own. Raise it when the corpus grows.
const probe076SkeletonPairFloor = 160

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
// not, and the pair floor catches a COMPOSITION OPT that stops parsing.
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
		opt, err := fixtures.ReadOperationalTemplate(body)
		if err != nil {
			continue
		}
		if strings.TrimSpace(opt.Definition.RMTypeName) != "COMPOSITION" {
			continue
		}
		id := strings.TrimSpace(opt.TemplateID.Value)
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
	if len(ran) < probe076SkeletonPairFloor {
		t.Errorf("the corpus gives %d (template, policy) pairs, want at least %d; did the corpus shrink?", len(ran), probe076SkeletonPairFloor)
	}
	t.Logf("%d COMPOSITION templates, %d (template, policy) pairs pass, %d registered exclusions", len(opts), passes, len(serializeprobes.Probe076SkeletonExclusions))
}

// probe076ValuesTemplate is the corpus template whose ELEMENT at0016 has a
// coded name: its text pinned to "Second option", its code one of the local
// at0017 ("First option") and at0018 ("Second option"). Medium fills it.
const probe076ValuesTemplate = "sdk_crosscheck_values.v1"

// probe076ValuesNamePath is the path at which the round trip of
// disagreeingNameAlternatives reports the name of the ELEMENT at0016.
const probe076ValuesNamePath = "/content[openEHR-EHR-OBSERVATION.sdk_values.v1]/data/events[at0002]/data/items[at0016]/name"

// probe076CodedNameEnd matches the end of the DV_CODED_TEXT that constrains
// the name of the ELEMENT at0016: its last listed code, then the closing tags
// of the code phrase, of the defining_code attribute and of the DV_CODED_TEXT.
var probe076CodedNameEnd = regexp.MustCompile(`</code_list>\s*</children>\s*</attributes>\s*</children>`)

// disagreeingNameAlternatives returns the OPT body of probe076ValuesTemplate,
// edited in memory: the name of its ELEMENT at0016 gets a second DV_CODED_TEXT
// alternative, a copy of the first whose pinned text "First option" gives the
// code at0017 where the first gives at0018. The skeleton fills the first
// alternative and validates; decode gives a name whose alternatives disagree
// no coded name, so each round trip reports the DV_TEXT it decodes there as
// matching neither alternative.
func disagreeingNameAlternatives(t *testing.T) []byte {
	t.Helper()
	opt := string(corpusOPT(t, probe076ValuesTemplate))
	element := strings.Index(opt, "<node_id>at0016</node_id>")
	if element < 0 {
		t.Fatalf("%s has no ELEMENT at0016; fixture changed?", probe076ValuesTemplate)
	}
	coded := strings.Index(opt[element:], "<rm_type_name>DV_CODED_TEXT</rm_type_name>")
	if coded < 0 {
		t.Fatalf("the ELEMENT at0016 of %s has no coded name; fixture changed?", probe076ValuesTemplate)
	}
	start := strings.LastIndex(opt[:element+coded], `<children xsi:type="C_COMPLEX_OBJECT">`)
	if start < element {
		t.Fatalf("cannot find the start of the coded name of the ELEMENT at0016 in %s; fixture changed?", probe076ValuesTemplate)
	}
	loc := probe076CodedNameEnd.FindStringIndex(opt[start:])
	if loc == nil {
		t.Fatalf("cannot find the end of the coded name of the ELEMENT at0016 in %s; fixture changed?", probe076ValuesTemplate)
	}
	end := start + loc[1]
	first := opt[start:end]
	second := strings.Replace(first, "<list>Second option</list>", "<list>First option</list>", 1)
	if second == first {
		t.Fatalf("the coded name of the ELEMENT at0016 of %s pins no text \"Second option\"; fixture changed?", probe076ValuesTemplate)
	}
	return []byte(opt[:end] + second + opt[end:])
}

// TestProbe076SkeletonLegFailsAnUnexcludedFinding pins that a finding no
// exclusion names fails the pair, and that the failure names the template,
// the policy, the format and the finding: an encode refusal by its error text,
// an issue by its code and path.
func TestProbe076SkeletonLegFailsAnUnexcludedFinding(t *testing.T) {
	cases := []struct {
		name string
		opt  func(t *testing.T) []byte
		want []string
	}{
		{
			name: "encode refusal",
			opt:  func(t *testing.T) []byte { return corpusOPT(t, "sdk_crosscheck_siblings.v1") },
			want: []string{"sdk_crosscheck_siblings.v1 medium FLAT: encode:", "sdk_crosscheck_siblings.v1 medium STRUCTURED: encode:", `FLAT id "twin" is one of several reused siblings`},
		},
		{
			name: "validation issue",
			opt:  disagreeingNameAlternatives,
			want: []string{
				probe076ValuesTemplate + " medium FLAT: alternative_mismatch at " + probe076ValuesNamePath,
				probe076ValuesTemplate + " medium STRUCTURED: alternative_mismatch at " + probe076ValuesNamePath,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := serializeprobes.Probe076SkeletonRoundTrip(t.Context(), tc.opt(t), instance.Medium, nil)
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
	const (
		id    = probe076ValuesTemplate
		moved = "/content[openEHR-EHR-OBSERVATION.sdk_values.v1]/data/events[at0002]/data/items[at0015]/name"
	)
	exclusion := func(format, path string) serializeprobes.Probe076Exclusion {
		return serializeprobes.Probe076Exclusion{
			Finding: serializeprobes.Probe076Finding{
				Template: id, Policy: instance.Medium, Format: format,
				Stage: "validate", Code: "alternative_mismatch", Path: path,
			},
			Reason:     "The test's own edit gives the name of the ELEMENT at0016 two coded alternatives that disagree.",
			Registered: "testkit/probes/serialize/probe_076_skeleton_test.go",
		}
	}
	opt := disagreeingNameAlternatives(t)
	// The control: the same exclusions, the FLAT one where the finding fires,
	// pass the pair, so the failure below is the stale FLAT exclusion alone.
	control, err := serializeprobes.Probe076SkeletonRoundTrip(t.Context(), opt, instance.Medium, []serializeprobes.Probe076Exclusion{
		exclusion("FLAT", probe076ValuesNamePath), exclusion("STRUCTURED", probe076ValuesNamePath),
	})
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if control.Status != probe.StatusPass {
		t.Fatalf("status = %q (detail: %s) with both exclusions where the findings fire; want pass", control.Status, control.Detail)
	}
	exclusions := []serializeprobes.Probe076Exclusion{exclusion("FLAT", moved), exclusion("STRUCTURED", probe076ValuesNamePath)}
	r, err := serializeprobes.Probe076SkeletonRoundTrip(t.Context(), opt, instance.Medium, exclusions)
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != probe.StatusFail {
		t.Fatalf("status = %q (detail: %s) with a stale exclusion; want fail", r.Status, r.Detail)
	}
	for _, w := range []string{
		"stale exclusion " + id + " medium FLAT: alternative_mismatch at " + moved,
		id + " medium FLAT: alternative_mismatch at " + probe076ValuesNamePath,
	} {
		if !strings.Contains(r.Detail, w) {
			t.Errorf("detail = %q; want it to name %q", r.Detail, w)
		}
	}
	if strings.Contains(r.Detail, "STRUCTURED") {
		t.Errorf("detail = %q; the STRUCTURED exclusion still fires and must not be reported", r.Detail)
	}
}

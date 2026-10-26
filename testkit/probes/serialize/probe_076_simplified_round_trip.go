package serializeprobes

// PROBE-076 — FLAT / STRUCTURED simplified-format composition round-trip
// (REQ-053). Given a vendored upstream OPT + canonical COMPOSITION, the probe
// builds the Web Template (REQ-106), decodes the canonical composition, and
// asserts the simplified codecs round-trip it without losing the data the
// formats carry:
//
//   - FLAT idempotence:      MarshalFlat -> UnmarshalFlat -> MarshalFlat is stable;
//   - STRUCTURED round-trip: MarshalStructured -> UnmarshalStructured re-encodes
//     to the same FLAT;
//   - interconversion:       FlatToStructured -> StructuredToFlat is the identity.
//
// Scope: round-trip idempotence PLUS an OPT-conformance check — when the source
// composition is itself OPT-valid, a WithTemplate decode (names + RM-mandatory
// completion) must also validate against the OPT. That conformance leg catches
// dropped or mistyped leaves that pure FLAT idempotence (a symmetric omission on
// both encode and decode) would miss. It does not compare the emitted
// FLAT/STRUCTURED byte-for-byte against vendored upstream simplified output — a
// documented follow-up needing those fixtures (see
// openehr/serialize/simplified/deviations.md § Conformance).
//
// The skeleton leg (Probe076SkeletonRoundTrip) takes the composition the
// generator writes instead of a vendored one, so it reaches every COMPOSITION
// OPT of the corpus, not only those vendored with a composition: under one
// policy, the skeleton composition.NewSkeleton writes validates, and its FLAT
// and STRUCTURED round trips, decoded with WithTemplate and WithStrict,
// validate against the OPT. A finding the codec registers as a gap is named in
// Probe076SkeletonExclusions with the deviations.md entry that registers it.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/wireequiv"
)

// Probe076SimplifiedRoundTrip runs the round-trip conformance checks for one
// (OPT, canonical composition) pair. A template the Web Template builder or the
// OPT compiler cannot yet handle yields Status "skip" (not "fail") so the probe
// distinguishes an un-modelled template from a codec defect. Framework misuse
// (nil inputs) returns a non-nil error.
func Probe076SimplifiedRoundTrip(optBody, compBody []byte) (Result, error) { // PROBE-076 (REQ-053)
	r := Result{Probe: "PROBE-076"}
	if optBody == nil || compBody == nil {
		return r, errors.New("PROBE-076: nil opt/composition body")
	}
	opt, err := fixtures.ParseOPTBytes(optBody)
	if err != nil {
		r.Status, r.Detail = "fail", "parse opt: "+err.Error()
		return r, nil
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		r.Status, r.Detail = "skip", "compile: "+err.Error()
		return r, nil
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		r.Status, r.Detail = "skip", "web template: "+err.Error()
		return r, nil
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(compBody, &comp); err != nil {
		r.Status, r.Detail = "fail", "canjson decode: "+err.Error()
		return r, nil
	}

	f1, err := simplified.MarshalFlat(&comp, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat: "+err.Error()
		return r, nil
	}
	// FLAT idempotence.
	comp2, err := simplified.UnmarshalFlat(f1, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "UnmarshalFlat: "+err.Error()
		return r, nil
	}
	f2, err := simplified.MarshalFlat(comp2, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat (re-encode): "+err.Error()
		return r, nil
	}
	if !flatMapsEqual(f1, f2) {
		r.Status, r.Detail = "fail", "FLAT round-trip not idempotent"
		return r, nil
	}
	// STRUCTURED round-trip via the OPT.
	s, err := simplified.MarshalStructured(&comp, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalStructured: "+err.Error()
		return r, nil
	}
	comp3, err := simplified.UnmarshalStructured(s, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "UnmarshalStructured: "+err.Error()
		return r, nil
	}
	f3, err := simplified.MarshalFlat(comp3, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat (from structured): "+err.Error()
		return r, nil
	}
	if !flatMapsEqual(f1, f3) {
		r.Status, r.Detail = "fail", "STRUCTURED round-trip diverges from FLAT"
		return r, nil
	}
	// OPT-free interconversion preserves the data. STRUCTURED is arrays-always
	// (spec), so the back-conversion cannot know a single-cardinality leaf had no
	// :index and emits a redundant :0 — valid-but-verbose FLAT, not data loss.
	// Assert semantic (canonical) equivalence: the interconverted FLAT decodes
	// and re-encodes to the same canonical FLAT. See deviations.md.
	sBytes, err := simplified.FlatToStructured(f1)
	if err != nil {
		r.Status, r.Detail = "fail", "FlatToStructured: "+err.Error()
		return r, nil
	}
	back, err := simplified.StructuredToFlat(sBytes)
	if err != nil {
		r.Status, r.Detail = "fail", "StructuredToFlat: "+err.Error()
		return r, nil
	}
	compIC, err := simplified.UnmarshalFlat(back, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "UnmarshalFlat (interconverted): "+err.Error()
		return r, nil
	}
	fIC, err := simplified.MarshalFlat(compIC, wt)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat (interconverted): "+err.Error()
		return r, nil
	}
	if !flatMapsEqual(f1, fIC) {
		r.Status, r.Detail = "fail", "FLAT<->STRUCTURED interconversion loses data"
		return r, nil
	}
	// Conformance: when the source composition is itself OPT-valid, a WithTemplate
	// decode (names + RM-mandatory completion) must also validate — this catches
	// dropped/mistyped leaves that FLAT idempotence alone (a symmetric omission)
	// would miss.
	if validation.Validate(&comp, compiled).OK {
		named, err := simplified.UnmarshalFlat(f1, wt, simplified.WithTemplate(compiled))
		if err != nil {
			r.Status, r.Detail = "fail", "UnmarshalFlat (WithTemplate): "+err.Error()
			return r, nil
		}
		if vr := validation.Validate(named, compiled); !vr.OK {
			r.Status, r.Detail = "fail", "WithTemplate decode of a valid composition does not validate: "+firstIssue(vr)
			return r, nil
		}
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("%d FLAT keys round-tripped", flatKeyCount(f1))
	return r, nil
}

func firstIssue(r validation.Result) string {
	if len(r.Issues) == 0 {
		return ""
	}
	return r.Issues[0].Code + " " + r.Issues[0].Path
}

// probe076SkeletonNow is the clock the skeleton leg writes its skeletons on,
// so that a run is reproducible.
var probe076SkeletonNow = time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)

// The stages of the skeleton leg at which a finding is reported.
const (
	probe076StageEncode   = "encode"
	probe076StageDecode   = "decode"
	probe076StageValidate = "validate"
)

// probe076Formats are the simplified formats the skeleton leg carries a
// skeleton through.
var probe076Formats = []string{"FLAT", "STRUCTURED"}

// Probe076Finding is one finding of the skeleton leg of PROBE-076. Template
// (the template id), Policy and Format ("FLAT" or "STRUCTURED") say where it
// was found, and Stage ("encode", "decode" or "validate") which step reported
// it. An encode or decode finding carries the error text in Error; a validate
// finding carries the issue's Code and Path. Two findings are the same when
// every field is equal.
type Probe076Finding struct {
	Template string
	Policy   instance.Policy
	Format   string
	Stage    string
	Error    string
	Code     string
	Path     string
}

// String names the finding on one line: the template id, the policy and the
// format, then the error text of an encode or decode finding, or the code and
// path of an issue.
func (f Probe076Finding) String() string {
	if f.Stage == probe076StageValidate {
		return fmt.Sprintf("%s %s %s: %s at %s", f.Template, f.Policy, f.Format, f.Code, f.Path)
	}
	return fmt.Sprintf("%s %s %s: %s: %s", f.Template, f.Policy, f.Format, f.Stage, f.Error)
}

// Probe076Exclusion is a finding the skeleton leg expects: a gap the codec
// documents. While the finding fires, the leg does not count it as a failure;
// once it stops firing, the leg fails the exclusion as stale. Reason says why
// the finding is expected, and Registered where the gap is recorded.
type Probe076Exclusion struct {
	Finding    Probe076Finding
	Reason     string
	Registered string
}

// probe076ReusedSiblingTwin is the encode refusal of the sdk_crosscheck_siblings
// skeleton under Medium, word for word.
const probe076ReusedSiblingTwin = `simplified: FLAT id "twin" is one of several reused siblings sharing the path "/context/other_context[at0001]/items[at0002]/value" — not yet encodable (see deviations.md): rmpath: path resolves to multiple items`

// Probe076SkeletonExclusions are the findings the skeleton leg expects over
// the vendored corpus, each matched exactly and each in both formats.
var Probe076SkeletonExclusions = slices.Concat(
	probe076InBothFormats(
		Probe076Finding{
			Template: "sdk_crosscheck_siblings.v1",
			Policy:   instance.Medium,
			Stage:    probe076StageEncode,
			Error:    probe076ReusedSiblingTwin,
		},
		"Medium fills the two ELEMENTs of node at0002 in the context's other_context, which pin no name and so share one bare path; "+
			"encode refuses data at a reused sibling rather than write it under every sibling's FLAT id. "+
			"Under Required the skeleton holds no data there, and the pair passes.",
		"openehr/serialize/simplified/deviations.md § Conformance, Archetype-reuse siblings, Encode",
	),
	probe076InBothFormats(
		Probe076Finding{
			Template: "Referral Request.v1",
			Policy:   instance.Medium,
			Stage:    probe076StageValidate,
			Code:     "rm_type_mismatch",
			Path:     "/context/other_context/items[openEHR-EHR-CLUSTER.annotations.v1]/items[at0001]/name",
		},
		"The annotation's name is a DV_CODED_TEXT whose code comes from an external reference set the template lists no code of. "+
			"FLAT and STRUCTURED have no key for a node's name below the root, and decode invents no code, "+
			"so the name comes back as DV_TEXT.",
		"openehr/serialize/simplified/deviations.md § Deviations, LOCATABLE.name on decode, Registered format limit",
	),
)

// probe076InBothFormats returns the exclusion of f, with reason and
// registered, in each simplified format.
func probe076InBothFormats(f Probe076Finding, reason, registered string) []Probe076Exclusion {
	out := make([]Probe076Exclusion, 0, len(probe076Formats))
	for _, format := range probe076Formats {
		f.Format = format
		out = append(out, Probe076Exclusion{Finding: f, Reason: reason, Registered: registered})
	}
	return out
}

// Probe076SkeletonRoundTrip runs the skeleton leg of PROBE-076 for one
// COMPOSITION OPT under one generator policy. It writes the skeleton
// [composition.NewSkeleton] builds for the OPT under policy, on a fixed clock,
// and requires it to validate against the OPT. It then encodes the skeleton
// as FLAT and as STRUCTURED, decodes each with [simplified.WithTemplate] and
// [simplified.WithStrict], and validates the decoded composition against the
// OPT.
//
// Every encode error, decode error and validation issue is a finding. Status
// is "pass" when every finding matches one of exclusions and every exclusion
// for this template and policy matches a finding; Detail then names the
// exclusions that matched. Otherwise Status is "fail", and Detail names each
// finding that no exclusion matches and each exclusion that matches nothing.
// An OPT that does not parse or compile, a Web Template that does not build,
// and a skeleton the generator refuses or writes invalid are a "fail" too.
//
// Framework misuse (a nil body, or an OPT whose root is not COMPOSITION)
// returns a non-nil error.
func Probe076SkeletonRoundTrip(ctx context.Context, optBody []byte, policy instance.Policy, exclusions []Probe076Exclusion) (Result, error) { // PROBE-076 (REQ-053)
	r := Result{Probe: "PROBE-076"}
	if optBody == nil {
		return r, errors.New("PROBE-076: nil opt body")
	}
	opt, err := fixtures.ParseOPTBytes(optBody)
	if err != nil {
		r.Status, r.Detail = "fail", "parse opt: "+err.Error()
		return r, nil
	}
	id := opt.TemplateID()
	if root := opt.Root(); root == nil || root.RMTypeName() != "COMPOSITION" {
		return r, fmt.Errorf("PROBE-076: the skeleton leg takes a COMPOSITION OPT, and the root of %s is not one", id)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		r.Status, r.Detail = "fail", id+": compile: "+err.Error()
		return r, nil
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		r.Status, r.Detail = "fail", id+": web template: "+err.Error()
		return r, nil
	}
	name := "Composer"
	comp, err := composition.NewSkeleton(ctx, compiled,
		composition.WithPolicy(policy),
		composition.WithComposer(&rm.PartyIdentified{Name: &name}),
		composition.WithTerritory("NL"),
		composition.WithNow(probe076SkeletonNow))
	if err != nil {
		r.Status, r.Detail = "fail", fmt.Sprintf("%s %s: skeleton: %v", id, policy, err)
		return r, nil
	}
	return skeletonRoundTrip(compiled, wt, comp, policy, exclusions), nil
}

// probe076Observed is a finding the skeleton leg observed, with the issue's
// detail text where it has one. The detail is shown, never matched.
type probe076Observed struct {
	finding Probe076Finding
	detail  string
}

func (o probe076Observed) String() string {
	if o.detail == "" {
		return o.finding.String()
	}
	return o.finding.String() + " (" + o.detail + ")"
}

// skeletonRoundTrip judges comp, the skeleton written for compiled under
// policy: comp must validate, and then each format's round trip may report
// only what exclusions names, and every exclusion for this template and
// policy must fire.
func skeletonRoundTrip(compiled *templatecompile.Compiled, wt *webtemplate.WebTemplate, comp *rm.Composition, policy instance.Policy, exclusions []Probe076Exclusion) Result {
	r := Result{Probe: "PROBE-076"}
	id := compiled.TemplateID()
	if vr := validation.ValidateComposition(comp, compiled); !vr.OK {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("%s %s: the written skeleton does not validate, so no round trip is judged: %s", id, policy, issueList(vr.Issues))
		return r
	}
	var found []probe076Observed
	for _, format := range probe076Formats {
		found = append(found, skeletonFindings(compiled, wt, comp, policy, format)...)
	}
	fired := make([]bool, len(exclusions))
	var problems []string
	for _, o := range found {
		i := slices.IndexFunc(exclusions, func(ex Probe076Exclusion) bool { return ex.Finding == o.finding })
		if i < 0 {
			problems = append(problems, o.String())
			continue
		}
		fired[i] = true
	}
	var hit []string
	for i, ex := range exclusions {
		if ex.Finding.Template != id || ex.Finding.Policy != policy {
			continue
		}
		if !fired[i] {
			problems = append(problems, "stale exclusion "+ex.Finding.String()+" no longer fires; remove it")
			continue
		}
		hit = append(hit, ex.Finding.String())
	}
	if len(problems) > 0 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("%d problem(s): %s", len(problems), strings.Join(problems, "; "))
		return r
	}
	r.Status = "pass"
	r.Detail = "the FLAT and STRUCTURED round trips validate"
	if len(hit) > 0 {
		r.Detail += "; registered exclusions that fired: " + strings.Join(hit, "; ")
	}
	return r
}

// skeletonFindings carries comp through format with the compiled template's
// Web Template wt, decoding with WithTemplate and WithStrict, and returns what
// the round trip reports: the encode error, the decode error, or the issues
// of the decoded composition.
func skeletonFindings(compiled *templatecompile.Compiled, wt *webtemplate.WebTemplate, comp *rm.Composition, policy instance.Policy, format string) []probe076Observed {
	at := Probe076Finding{Template: compiled.TemplateID(), Policy: policy, Format: format}
	encode, decode := simplified.MarshalFlat, simplified.UnmarshalFlat
	if format == "STRUCTURED" {
		encode, decode = simplified.MarshalStructured, simplified.UnmarshalStructured
	}
	body, err := encode(comp, wt)
	if err != nil {
		at.Stage, at.Error = probe076StageEncode, err.Error()
		return []probe076Observed{{finding: at}}
	}
	back, err := decode(body, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
	if err != nil {
		at.Stage, at.Error = probe076StageDecode, err.Error()
		return []probe076Observed{{finding: at}}
	}
	issues := validation.ValidateComposition(back, compiled).Issues
	out := make([]probe076Observed, 0, len(issues))
	for _, is := range issues {
		f := at
		f.Stage, f.Code, f.Path = probe076StageValidate, is.Code, is.Path
		out = append(out, probe076Observed{finding: f, detail: is.Detail})
	}
	return out
}

// probe076MaxListedIssues bounds the issues issueList spells out.
const probe076MaxListedIssues = 5

// issueList names the first issues on one line, and how many more there are.
func issueList(issues []validation.Issue) string {
	parts := make([]string, 0, min(len(issues), probe076MaxListedIssues)+1)
	for _, is := range issues[:min(len(issues), probe076MaxListedIssues)] {
		parts = append(parts, is.Code+" at "+is.Path+" ("+is.Detail+")")
	}
	if n := len(issues) - probe076MaxListedIssues; n > 0 {
		parts = append(parts, fmt.Sprintf("and %d more", n))
	}
	return strings.Join(parts, "; ")
}

// flatMapsEqual reports whether two FLAT payloads are wire-equivalent. It
// defers to the shared [wireequiv.Equivalent] oracle, which parses both sides
// with json.Number so integers above 2^53 compare as text rather than through a
// lossy float64, the precision guarantee the codec documents.
func flatMapsEqual(a, b []byte) bool {
	ok, _ := wireequiv.Equivalent(a, b)
	return ok
}

func decodeNumberMap(b []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func flatKeyCount(b []byte) int {
	m, err := decodeNumberMap(b)
	if err != nil {
		return 0
	}
	return len(m)
}

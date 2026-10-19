package probe_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"uuid"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// liveGeneratedTemplate is one vendored OPT the generated-commit snapshot
// commits under every policy. name resolves the file through
// fixtures.TemplateOptForName, and id is the template id that file carries.
// short names the template in its per-run template id and in the probe ids.
// short never contains id, so a rewritten OPT that still holds id is a
// rewrite that missed an occurrence.
type liveGeneratedTemplate struct {
	name  string
	id    string
	short string
}

var liveGeneratedTemplates = []liveGeneratedTemplate{
	{name: "vital_signs", id: "vital_signs", short: "vitals"},
	{name: "social", id: "SocialeAnamnese.v1", short: "social"},
}

// liveGeneratedPolicies are the generator's three policies, smallest
// output first.
var liveGeneratedPolicies = []instance.Policy{instance.Required, instance.Medium, instance.Large}

// liveGeneratedOPT rewrites every occurrence of tmpl.id in the vendored OPT
// to templateID, as livePerRunComposition does, and refuses a rewrite that
// left the vendored id in place. It then parses the rewritten bytes
// strictly and compiles them, as generate-example does.
func liveGeneratedOPT(t *testing.T, tmpl liveGeneratedTemplate, templateID string) ([]byte, *templatecompile.Compiled) {
	t.Helper()
	src := []byte(tmpl.id)
	dst := []byte(templateID)
	optBody := bytes.ReplaceAll(mustReadFile(t, fixtures.TemplateOptForName(tmpl.name)), src, dst)
	if !bytes.Contains(optBody, dst) || bytes.Contains(optBody, src) {
		t.Fatalf("OPT %s does not carry the per-run template id %s in place of %s", tmpl.name, templateID, tmpl.id)
	}
	opt, err := template.ParseOPTStrict(bytes.NewReader(optBody))
	if err != nil {
		t.Fatalf("ParseOPTStrict(%s rewritten to %s): %v", tmpl.name, templateID, err)
	}
	if got := opt.TemplateID(); got != templateID {
		t.Fatalf("ParseOPTStrict(%s rewritten) template id = %q, want %q", tmpl.name, got, templateID)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", templateID, err)
	}
	return optBody, compiled
}

// liveGeneratedComposition generates one composition from compiled under
// policy, with the options generate-example uses. It fails the test unless
// the composition names templateID, the template validator finds no issue,
// and the RM floor finds no issue. A Live failure is then the CDR refusing
// what the SDK holds valid, never an SDK-side validity failure.
func liveGeneratedComposition(t *testing.T, compiled *templatecompile.Compiled, policy instance.Policy, templateID, composer string) *rm.Composition {
	t.Helper()
	generated, err := instance.Generate(t.Context(), compiled, instance.Options{
		Policy:    policy,
		Territory: "NL",
		Composer:  &rm.PartyIdentified{Name: new(composer)},
	})
	if err != nil {
		t.Fatalf("Generate(%s, %v): %v", templateID, policy, err)
	}
	comp, err := instance.AsComposition(generated)
	if err != nil {
		t.Fatalf("AsComposition(Generate(%s, %v)): %v", templateID, policy, err)
	}
	if comp.ArchetypeDetails == nil || comp.ArchetypeDetails.TemplateID == nil || comp.ArchetypeDetails.TemplateID.Value != templateID {
		t.Fatalf("Generate(%s, %v) archetype_details = %+v, want template id %s", templateID, policy, comp.ArchetypeDetails, templateID)
	}
	if res := validation.ValidateComposition(comp, compiled); !res.OK || len(res.Issues) != 0 {
		t.Fatalf("ValidateComposition(Generate(%s, %v)) OK = %v, issues: %v", templateID, policy, res.OK, res.Issues)
	}
	if res := validation.ValidateRM(comp); !res.OK || len(res.Issues) != 0 {
		t.Fatalf("ValidateRM(Generate(%s, %v)) OK = %v, issues: %v", templateID, policy, res.OK, res.Issues)
	}
	return comp
}

// generatedUploadEntry uploads the per-run OPT. Its template id embeds the
// run identifier, so the write is self-scoping (REQ-082 Live): it collides
// with no other run and depends on no pre-seeded catalog entry.
func generatedUploadEntry(short string, optBody []byte) probe.Entry {
	return probe.Entry{
		ID:     "LIVE-GENERATED-UPLOAD-" + strings.ToUpper(short),
		Effect: probe.EffectMutating,
		Modes:  []probe.Mode{probe.ModeLive},
		Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
			meta, _, err := definition.UploadTemplate(ctx, c, definition.FormatADL14, bytes.NewReader(optBody))
			if err != nil {
				return liveFail(err), nil
			}
			if meta == nil || meta.TemplateID == "" {
				return liveFailf("upload returned no template id"), nil
			}
			return probe.Result{Status: probe.StatusPass, Detail: meta.TemplateID}, nil
		},
	}
}

// generatedCommitEntries commits comp, as the generator built it, under the
// per-run EHR, then reads the committed version back. The read-back skips
// when the save committed nothing, and passes only when the CDR returns the
// composition under templateID.
func generatedCommitEntries(id openehrclient.EHRID, short, templateID string, policy instance.Policy, comp *rm.Composition) []probe.Entry {
	live := []probe.Mode{probe.ModeLive}
	suffix := strings.ToUpper(short) + "-" + strings.ToUpper(policy.String())
	out := &committedVersion{}
	return []probe.Entry{
		{
			ID:     "LIVE-GENERATED-SAVE-" + suffix,
			Effect: probe.EffectMutating,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				_, meta, err := composition.Save(ctx, c, id, comp, composition.WithTemplateID(templateID))
				if err != nil {
					return liveFail(err), nil
				}
				if meta == nil || meta.VersionUID == "" {
					return liveFailf("save returned no version uid"), nil
				}
				out.uid = meta.VersionUID
				return probe.Result{Status: probe.StatusPass, Detail: string(meta.VersionUID)}, nil
			},
		},
		{
			ID:     "LIVE-GENERATED-GET-" + suffix,
			Effect: probe.EffectReadOnly,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				if out.uid == "" {
					return probe.Result{Status: probe.StatusSkip, Detail: "no composition was committed to read back"}, nil
				}
				got, _, err := composition.Get(ctx, c, id, openehrclient.VersionOf(out.uid))
				if err != nil {
					return liveFail(err), nil
				}
				if got == nil {
					return liveFailf("get returned a nil composition"), nil
				}
				if got.ArchetypeDetails == nil || got.ArchetypeDetails.TemplateID == nil || got.ArchetypeDetails.TemplateID.Value != templateID {
					return liveFailf("get returned template_id %+v, want %s", got.ArchetypeDetails, templateID), nil
				}
				return probe.Result{Status: probe.StatusPass, Detail: templateID}, nil
			},
		},
	}
}

// TestLiveGeneratedCommit_REQ107_REQ082 commits the generator's output to a
// live deployment (REQ-082 Live): create a per-run EHR, upload each vendored
// OPT under a per-run template id, commit one composition per policy
// (Required, Medium and Large), and read each committed version back. It
// checks that a CDR accepts each policy's output as generated. The ITS-REST
// template example says its detail_level=required output is "expected to be
// committable without adjustment", and Required builds that level (REQ-107).
// It is not a catalog PROBE. Opt-in and skipped in CI, sharing the
// runLiveSnapshot harness with the other Live snapshots: OPENEHR_LIVE_EHRBASE
// names the deployment, OPENEHR_LIVE_ALLOW_MUTATING lets the run write, and
// OPENEHR_LIVE_EHRBASE_BASIC, when set, carries user:pass credentials.
//
// Every resource the run writes carries the run identifier (REQ-082
// self-scoping): the EHR id is the identifier, each template id embeds it,
// and each composition names its per-run template and sits in that EHR.
// Generation and the offline validation run before the harness, so they
// also run in CI. Each composition must pass the template validator and the
// RM floor (REQ-107 Output) before anything goes live, so a Live failure is
// the CDR refusing what the SDK holds valid.
func TestLiveGeneratedCommit_REQ107_REQ082(t *testing.T) {
	id := openehrclient.EHRID(uuid.NewV4().String())
	runID := strings.ReplaceAll(string(id), "-", "")
	t.Logf("per-run EHR id %s", id)

	entries := []probe.Entry{createEHRProbe(id)}
	for _, tmpl := range liveGeneratedTemplates {
		templateID := "sdk_generated_" + runID + "_" + tmpl.short + ".v1"
		t.Logf("template %s, per-run id %s", tmpl.id, templateID)
		optBody, compiled := liveGeneratedOPT(t, tmpl, templateID)
		entries = append(entries, generatedUploadEntry(tmpl.short, optBody))
		for _, policy := range liveGeneratedPolicies {
			comp := liveGeneratedComposition(t, compiled, policy, templateID, "SDK live generated "+runID)
			entries = append(entries, generatedCommitEntries(id, tmpl.short, templateID, policy, comp)...)
		}
	}
	runLiveSnapshot(t, "generated", entries)
}

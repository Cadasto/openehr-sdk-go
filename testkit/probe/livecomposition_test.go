package probe_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"uuid"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// liveTemplateID is the EHRbase-origin fixture the composition snapshot uses:
// its OPT and canonical composition are vendored from EHRbase's own test data
// (testkit/cassettes/), so a conformant EHRbase accepts both — a failure on
// commit is then the SDK's wire encoding, not an unfamiliar template.
const liveTemplateID = "terminology_test.ehrbase.org.v1"

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// committedVersion carries the version uid a save probe produced to the get
// probe that reads it back. probe.Run executes entries in order, so the
// closure hand-off is safe.
type committedVersion struct {
	uid openehrclient.VersionUID
}

// compositionLiveEntries is the clinical write/read path: upload the per-run
// OPT, commit the composition under the per-run EHR id, then read the committed
// version back. templateID embeds the run identifier, so the template write is
// self-scoping (REQ-082 Live): it collides with no other run and depends on no
// pre-seeded catalog entry — a genuine 201, never a foreign 409 counted as a
// pass. The composition reuses the EHR the caller created earlier in the run.
func compositionLiveEntries(id openehrclient.EHRID, templateID string, optBody []byte, comp *rm.Composition, out *committedVersion) []probe.Entry {
	live := []probe.Mode{probe.ModeLive}
	return []probe.Entry{
		{
			ID:     "LIVE-TEMPLATE-UPLOAD",
			Effect: probe.EffectMutating,
			Modes:  live,
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
		},
		{
			ID:     "LIVE-COMPOSITION-SAVE",
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
			ID:     "LIVE-COMPOSITION-GET",
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
				// A light identity check (not a deep compare): the CDR returned
				// the composition committed against the per-run template.
				if got.ArchetypeDetails == nil || got.ArchetypeDetails.TemplateID == nil || got.ArchetypeDetails.TemplateID.Value != templateID {
					return liveFailf("get returned template_id %+v, want %s", got.ArchetypeDetails, templateID), nil
				}
				return probe.Result{Status: probe.StatusPass, Detail: templateID}, nil
			},
		},
	}
}

// TestLiveCompositionSnapshot runs the clinical write/read path against a live
// deployment (REQ-082 Live): create a per-run EHR, upload the per-run OPT,
// commit its canonical composition, and read the committed version back. It
// exercises the REQ-080 wire — the SDK's own canonical composition encoding has
// to be accepted by a real CDR — without being a catalog PROBE. Opt-in and
// skipped in CI, sharing the runLiveSnapshot harness with TestLiveCoreSnapshot.
//
// The EHRbase-origin fixture template and composition are rewritten to a per-run
// template id so every resource the run creates carries the run identifier
// (REQ-082 self-scoping); the structure is unchanged, so a conformant CDR
// accepts both and a failure is the SDK's wire encoding.
func TestLiveCompositionSnapshot(t *testing.T) {
	id := openehrclient.EHRID(uuid.NewV4().String())
	// Per-run template id: embeds the run identifier so the mutating template
	// upload is self-scoping — no collision with another run, no dependency on a
	// pre-seeded catalog template (REQ-082 Live).
	templateID := "sdk_snapshot_" + strings.ReplaceAll(string(id), "-", "") + ".v1"
	t.Logf("per-run EHR id %s, template %s", id, templateID)

	src := []byte(liveTemplateID)
	dst := []byte(templateID)
	optBody := bytes.ReplaceAll(mustReadFile(t, fixtures.TemplateOpt(liveTemplateID)), src, dst)
	var comp rm.Composition
	if err := canjson.Unmarshal(bytes.ReplaceAll(mustReadFile(t, fixtures.CompositionJSON(liveTemplateID)), src, dst), &comp); err != nil {
		t.Fatalf("decode composition fixture %s: %v", liveTemplateID, err)
	}

	out := &committedVersion{}
	entries := append([]probe.Entry{createEHRProbe(id)}, compositionLiveEntries(id, templateID, optBody, &comp, out)...)
	runLiveSnapshot(t, "composition", entries)
}

package probe_test

import (
	"bytes"
	"context"
	"errors"
	"os"
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

// compositionLiveEntries is the clinical write/read path: upload the OPT
// (idempotent — a template already on the deployment is the precondition met,
// not a failure), commit the vendored composition under the per-run EHR id,
// then read the committed version back. The composition reuses the EHR the
// caller created earlier in the same run.
func compositionLiveEntries(id openehrclient.EHRID, optBody []byte, comp *rm.Composition, out *committedVersion) []probe.Entry {
	live := []probe.Mode{probe.ModeLive}
	return []probe.Entry{
		{
			ID:     "LIVE-TEMPLATE-UPLOAD",
			Effect: probe.EffectMutating,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				_, _, err := definition.UploadTemplate(ctx, c, definition.FormatADL14, bytes.NewReader(optBody))
				if err != nil {
					// A template already present answers 409; that is the
					// precondition met, not a failure.
					if errors.Is(err, transport.ErrVersionConflict) {
						return probe.Result{Status: probe.StatusPass, Detail: "template already present"}, nil
					}
					return liveFail(err), nil
				}
				return probe.Result{Status: probe.StatusPass}, nil
			},
		},
		{
			ID:     "LIVE-COMPOSITION-SAVE",
			Effect: probe.EffectMutating,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				_, meta, err := composition.Save(ctx, c, id, comp, composition.WithTemplateID(liveTemplateID))
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
				return probe.Result{Status: probe.StatusPass}, nil
			},
		},
	}
}

// TestLiveCompositionSnapshot runs the clinical write/read path against a live
// deployment: create a per-run EHR, upload the EHRbase-origin OPT, commit its
// canonical composition, and read the committed version back. It is the
// highest-value REQ-080 wire-conformance signal — the SDK's own canonical
// encoding has to be accepted by a real CDR. Opt-in and skipped in CI, sharing
// the runLiveSnapshot harness with TestLiveCoreSnapshot.
func TestLiveCompositionSnapshot(t *testing.T) {
	id := openehrclient.EHRID(uuid.NewV4().String())
	t.Logf("per-run EHR id %s", id)

	optBody := mustReadFile(t, fixtures.TemplateOpt(liveTemplateID))
	var comp rm.Composition
	if err := canjson.Unmarshal(mustReadFile(t, fixtures.CompositionJSON(liveTemplateID)), &comp); err != nil {
		t.Fatalf("decode composition fixture %s: %v", liveTemplateID, err)
	}

	out := &committedVersion{}
	entries := append([]probe.Entry{createEHRProbe(id)}, compositionLiveEntries(id, optBody, &comp, out)...)
	runLiveSnapshot(t, "composition", entries)
}

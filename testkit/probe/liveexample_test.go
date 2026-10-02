package probe_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"uuid"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// definitionExampleLiveEntries witnesses PROBE-104 (REQ-095). The upload
// is the self-scoped setup (REQ-082): templateID embeds the per-run
// identifier. The example GET is read-only and does not capture the
// request. It passes when the composition is non-nil and
// ArchetypeNodeID is non-empty.
func definitionExampleLiveEntries(templateID string, optBody []byte) []probe.Entry {
	live := []probe.Mode{probe.ModeLive}
	uploaded := false
	return []probe.Entry{
		{
			ID:     "LIVE-DEFINITION-EXAMPLE-UPLOAD",
			Effect: probe.EffectMutating,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				if !bytes.Contains(optBody, []byte(templateID)) {
					return liveFailf("template body does not carry the per-run id %s", templateID), nil
				}
				meta, _, err := definition.UploadTemplate(ctx, c, definition.FormatADL14, bytes.NewReader(optBody))
				if err != nil {
					return liveFail(err), nil
				}
				if meta == nil || meta.TemplateID == "" {
					return liveFailf("upload returned no template id"), nil
				}
				uploaded = true
				return probe.Result{Status: probe.StatusPass, Detail: meta.TemplateID}, nil
			},
		},
		{
			ID:     "LIVE-DEFINITION-EXAMPLE",
			Effect: probe.EffectReadOnly,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				if !uploaded {
					return probe.Result{Status: probe.StatusSkip, Detail: "no template was uploaded in this run to ask an example for"}, nil
				}
				comp, _, err := definition.ExampleComposition(ctx, c, templateID, definition.FormatADL14)
				if err != nil {
					return liveFail(err), nil
				}
				if comp == nil {
					return liveFailf("example returned a nil composition"), nil
				}
				if comp.ArchetypeNodeID == "" {
					return liveFailf("example archetype_node_id is empty"), nil
				}
				return probe.Result{Status: probe.StatusPass, Detail: comp.ArchetypeNodeID}, nil
			},
		},
	}
}

// TestLiveDefinitionExample_PROBE104_REQ095_REQ082 runs the definition
// example read-back against a live deployment (PROBE-104, REQ-095,
// REQ-082). Opt-in and skipped in CI, sharing runLiveSnapshot with the
// other Live snapshots. The per-run OPT is the EHRbase terminology_test
// fixture rewritten so the template id carries the run identifier.
func TestLiveDefinitionExample_PROBE104_REQ095_REQ082(t *testing.T) {
	runID := uuid.NewV4().String()
	templateID := "sdk_snapshot_" + strings.ReplaceAll(runID, "-", "") + ".v1"
	t.Logf("per-run id %s, template %s", runID, templateID)

	optBody, _ := livePerRunComposition(t, templateID)
	runLiveSnapshot(t, "definition-example", definitionExampleLiveEntries(templateID, optBody))
}

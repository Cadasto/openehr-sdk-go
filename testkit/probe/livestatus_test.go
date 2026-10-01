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
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// liveEHRStatusJSON is the sandbox PROBE-060 fixture (ehrStatusJSON in
// testkit/probes/rest/probes_test.go): archetype
// openEHR-EHR-EHR_STATUS.generic.v1, subject PARTY_SELF, is_queryable
// true, is_modifiable false. is_modifiable false is the distinctive
// flag, so a server default of both-true cannot pass the read-back.
const liveEHRStatusJSON = `{"_type":"EHR_STATUS","name":{"_type":"DV_TEXT","value":"EHR Status"},"archetype_node_id":"openEHR-EHR-EHR_STATUS.generic.v1","subject":{"_type":"PARTY_SELF"},"is_queryable":true,"is_modifiable":false}`

// createdEHR carries the server-assigned ehr_id from the status create
// to the status read-back. probe.Run executes entries in order, so the
// closure hand-off is safe. The composition write uses a different EHR.
type createdEHR struct {
	id openehrclient.EHRID
}

// liveInitialEHRStatus decodes the sandbox fixture and writes runID into
// name.value, so the EHR_STATUS this run leaves behind is attributable
// (REQ-082).
func liveInitialEHRStatus(t *testing.T, runID string) *rm.EHRStatus {
	t.Helper()
	body := strings.Replace(liveEHRStatusJSON, `"value":"EHR Status"`, `"value":"`+runID+`"`, 1)
	var st rm.EHRStatus
	if err := canjson.Unmarshal([]byte(body), &st); err != nil {
		t.Fatalf("decode EHR_STATUS fixture: %v", err)
	}
	gotName := ""
	if st.Name != nil {
		gotName = st.Name.GetValue()
	}
	if gotName != runID {
		t.Fatalf("EHR_STATUS name = %q, want the per-run id %q", gotName, runID)
	}
	if st.ArchetypeNodeID != "openEHR-EHR-EHR_STATUS.generic.v1" || !st.IsQueryable || st.IsModifiable {
		t.Fatalf("EHR_STATUS fixture archetype %q queryable %v modifiable %v, want openEHR-EHR-EHR_STATUS.generic.v1 true false",
			st.ArchetypeNodeID, st.IsQueryable, st.IsModifiable)
	}
	return &st
}

// livePerRunComposition rewrites the EHRbase-origin OPT and canonical
// composition to templateID, the same way TestLiveCompositionSnapshot
// does, and refuses a rewrite that left the shared template id in place.
func livePerRunComposition(t *testing.T, templateID string) ([]byte, *rm.Composition) {
	t.Helper()
	src := []byte(liveTemplateID)
	dst := []byte(templateID)
	optBody := bytes.ReplaceAll(mustReadFile(t, fixtures.TemplateOpt(liveTemplateID)), src, dst)
	raw := bytes.ReplaceAll(mustReadFile(t, fixtures.CompositionJSON(liveTemplateID)), src, dst)
	if !bytes.Contains(optBody, dst) || bytes.Contains(optBody, src) {
		t.Fatalf("OPT does not carry the per-run template id %s", templateID)
	}
	if !bytes.Contains(raw, dst) || bytes.Contains(raw, src) {
		t.Fatalf("composition fixture does not carry the per-run template id %s", templateID)
	}
	comp := &rm.Composition{}
	if err := canjson.Unmarshal(raw, comp); err != nil {
		t.Fatalf("decode composition fixture %s: %v", liveTemplateID, err)
	}
	return optBody, comp
}

// ehrStatusLiveEntries witnesses PROBE-060 (REQ-095). The create is
// server-assigned (WithInitialStatus, no WithEHRID). The composition
// arm does not use this EHR: is_modifiable false means a conformant
// server refuses later writes to it.
func ehrStatusLiveEntries(runID string, status *rm.EHRStatus, created *createdEHR) []probe.Entry {
	live := []probe.Mode{probe.ModeLive}
	return []probe.Entry{
		{
			ID:     "LIVE-EHR-STATUS-CREATE",
			Effect: probe.EffectMutating,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				if status.Name == nil || status.Name.GetValue() != runID {
					return liveFailf("initial EHR_STATUS name does not carry the per-run id %s", runID), nil
				}
				rec, meta, err := openehrclient.Create(ctx, c, openehrclient.WithInitialStatus(status))
				if err != nil {
					return liveFail(err), nil
				}
				if rec == nil || rec.EHRID.Value == "" {
					return liveFailf("Create surfaced no ehr_id"), nil
				}
				loc := ""
				if meta != nil {
					loc = meta.Location
				}
				wantTail := "/ehr/" + rec.EHRID.Value
				if !strings.HasSuffix(strings.TrimSuffix(loc, "/"), wantTail) {
					return liveFailf("Create Location %q, want a %q tail", loc, wantTail), nil
				}
				created.id = openehrclient.EHRID(rec.EHRID.Value)
				return probe.Result{Status: probe.StatusPass, Detail: rec.EHRID.Value}, nil
			},
		},
		{
			ID:     "LIVE-EHR-STATUS-GET",
			Effect: probe.EffectReadOnly,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				if created.id == "" {
					return probe.Result{Status: probe.StatusSkip, Detail: "no EHR was created in this run to read back"}, nil
				}
				st, _, err := ehrstatus.Get(ctx, c, created.id)
				if err != nil {
					return liveFail(err), nil
				}
				if st == nil {
					return liveFailf("ehrstatus.Get returned nil"), nil
				}
				if st.ArchetypeNodeID != status.ArchetypeNodeID || st.IsQueryable != status.IsQueryable || st.IsModifiable != status.IsModifiable {
					return liveFailf("ehrstatus.Get archetype %q queryable %v modifiable %v, want %q %v %v",
						st.ArchetypeNodeID, st.IsQueryable, st.IsModifiable,
						status.ArchetypeNodeID, status.IsQueryable, status.IsModifiable), nil
				}
				return probe.Result{Status: probe.StatusPass, Detail: st.ArchetypeNodeID}, nil
			},
		},
	}
}

// minimalCompositionLiveEntries witnesses PROBE-065 (REQ-094) on ehrID,
// an EHR this run created and that allows modification. Save uses the
// default Prefer (return=minimal): a nil composition and a VersionUID.
// Get passes when the composition is non-nil and ArchetypeNodeID is set.
func minimalCompositionLiveEntries(ehrID openehrclient.EHRID, templateID string, optBody []byte, comp *rm.Composition, committed *committedVersion) []probe.Entry {
	live := []probe.Mode{probe.ModeLive}
	return []probe.Entry{
		{
			ID:     "LIVE-MINIMAL-TEMPLATE-UPLOAD",
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
				return probe.Result{Status: probe.StatusPass, Detail: meta.TemplateID}, nil
			},
		},
		{
			ID:     "LIVE-MINIMAL-COMPOSITION-SAVE",
			Effect: probe.EffectMutating,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				got, meta, err := composition.Save(ctx, c, ehrID, comp, composition.WithTemplateID(templateID))
				if err != nil {
					return liveFail(err), nil
				}
				if got != nil {
					return liveFailf("composition.Save returned a composition, want nil under Prefer return=minimal"), nil
				}
				if meta == nil || meta.VersionUID == "" {
					return liveFailf("composition.Save returned no version uid"), nil
				}
				committed.uid = meta.VersionUID
				return probe.Result{Status: probe.StatusPass, Detail: string(meta.VersionUID)}, nil
			},
		},
		{
			ID:     "LIVE-MINIMAL-COMPOSITION-GET",
			Effect: probe.EffectReadOnly,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				if committed.uid == "" {
					return probe.Result{Status: probe.StatusSkip, Detail: "no composition was committed to read back"}, nil
				}
				got, _, err := composition.Get(ctx, c, ehrID, openehrclient.VersionOf(committed.uid))
				if err != nil {
					return liveFail(err), nil
				}
				if got == nil {
					return liveFailf("composition.Get returned a nil composition"), nil
				}
				if got.ArchetypeNodeID == "" {
					return liveFailf("composition.Get archetype_node_id is empty"), nil
				}
				return probe.Result{Status: probe.StatusPass, Detail: got.ArchetypeNodeID}, nil
			},
		},
	}
}

// TestLiveEHRStatusAndMinimalSnapshot witnesses PROBE-060 and PROBE-065
// against a live deployment (REQ-082 Live, REQ-095, REQ-094). Opt-in and
// skipped in CI, sharing runLiveSnapshot with the other Live snapshots.
//
// The two probes use two EHRs. PROBE-060 creates one with
// WithInitialStatus and without WithEHRID: is_queryable true,
// is_modifiable false, and the per-run id in name.value. Location must
// name /ehr/{id}, and ehrstatus.Get must return those three fields.
// That EHR is not modified again.
//
// PROBE-065 creates a second EHR with createEHRProbe (client-supplied
// per-run id, server-default status, so the EHR allows modification),
// uploads a per-run OPT, and calls composition.Save with no Prefer
// override. Save passes only when the composition is nil and VersionUID
// is non-empty. composition.Get passes when the composition is non-nil
// and ArchetypeNodeID is non-empty.
func TestLiveEHRStatusAndMinimalSnapshot(t *testing.T) {
	runID := uuid.NewV4().String()
	// The composition EHR's id is the per-run identifier. The status
	// name and the template id carry the same identifier (REQ-082).
	compositionEHR := openehrclient.EHRID(runID)
	templateID := "sdk_snapshot_" + strings.ReplaceAll(runID, "-", "") + ".v1"
	t.Logf("per-run id %s, template %s", runID, templateID)

	status := liveInitialEHRStatus(t, runID)
	optBody, comp := livePerRunComposition(t, templateID)

	created := &createdEHR{}
	committed := &committedVersion{}
	entries := ehrStatusLiveEntries(runID, status, created)
	entries = append(entries, createEHRProbe(compositionEHR))
	entries = append(entries, minimalCompositionLiveEntries(compositionEHR, templateID, optBody, comp, committed)...)
	runLiveSnapshot(t, "ehr-status-minimal", entries)
}

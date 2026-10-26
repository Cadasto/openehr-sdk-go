package probe_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	versionedprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/versioned"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// compositionUpdateRecordings are the composition-update captures, one per
// deployment that accepts the quoted If-Match the ITS-REST pin requires.
var compositionUpdateRecordings = []string{
	"composition-update.har",
	"ferroehr/composition-update.har",
}

// cassetteReplayTemplateID names the template the replay uploads and saves
// under. The replay matches on method and path only, so the id the capture
// minted need not be reused.
const cassetteReplayTemplateID = "sdk_cassette_replay.v1"

// replayCompositionUpdate drives a composition-update recording through the
// versioned-write probes, in the order the capture sent their requests, and
// returns an error naming the first probe the recorded responses fail
// (PROBE-061, PROBE-012, PROBE-011, PROBE-071): a save with
// return=representation answers a bare COMPOSITION and the new version's
// id; a read of the latest version and an update with its ETag succeed; the
// same update with the original, now stale, version id is refused with 412;
// and a save and an update with return=representation answer bare
// COMPOSITION bodies.
func replayCompositionUpdate(ctx context.Context, c *transport.Client) error {
	rec, _, err := ehr.Create(ctx, c)
	if err != nil {
		return fmt.Errorf("ehr.Create: %w", err)
	}
	if rec == nil || rec.EHRID.Value == "" {
		return errors.New("create ehr_id: ehr.Create recovered no EHR id")
	}
	ehrID := ehr.EHRID(rec.EHRID.Value)
	opt, comp, err := cassetteComposition(cassetteReplayTemplateID)
	if err != nil {
		return err
	}
	if _, _, err := definition.UploadTemplate(ctx, c, definition.FormatADL14, bytes.NewReader(opt)); err != nil {
		return fmt.Errorf("definition.UploadTemplate: %w", err)
	}

	saved, meta, err := composition.Save(ctx, c, ehrID, comp, composition.WithPrefer(transport.PreferRepresentation))
	if err != nil {
		return fmt.Errorf("PROBE-061 save: %w", err)
	}
	if saved == nil || saved.ArchetypeNodeID == "" {
		return errors.New("PROBE-061 body: the save with return=representation decoded no bare COMPOSITION")
	}
	if meta == nil || meta.VersionUID == "" {
		return errors.New("PROBE-061 version uid: the save recovered no version id")
	}
	created := meta.VersionUID
	voID := created.VersionedObjectID()

	steps := []struct {
		probe string
		run   func() (probe.Result, error)
	}{
		{"PROBE-012", func() (probe.Result, error) {
			return versionedprobes.Probe012ETagRoundTrip(ctx, c, ehrID, voID, comp)
		}},
		{"PROBE-011", func() (probe.Result, error) {
			return versionedprobes.Probe011PutStaleIfMatch(ctx, c, ehrID, voID, string(created), comp)
		}},
	}
	for _, s := range steps {
		if err := passed(s.probe, s.run); err != nil {
			return err
		}
	}

	_, latest, err := composition.Get(ctx, c, ehrID, ehr.LatestOf(voID))
	if err != nil {
		return fmt.Errorf("PROBE-071 read latest: %w", err)
	}
	if latest == nil || latest.VersionUID == "" {
		return errors.New("PROBE-071 read latest: the read recovered no version id")
	}
	return passed("PROBE-071", func() (probe.Result, error) {
		return versionedprobes.Probe071CompositionWriteResponseShape(ctx, c, ehrID, voID, string(latest.VersionUID), comp)
	})
}

// passed runs one probe and returns an error naming it unless it passed.
func passed(name string, run func() (probe.Result, error)) error {
	r, err := run()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if r.Status != "pass" {
		return fmt.Errorf("%s %s: %s", name, r.Status, r.Detail)
	}
	return nil
}

// TestCassette_ReplaysCompositionUpdate_PROBE061_PROBE012_PROBE011_PROBE071_REQ054_REQ094_REQ082
// replays each composition-update recording (PROBE-061, PROBE-012,
// PROBE-011, PROBE-071, REQ-054, REQ-094, REQ-082) through the versioned
// probes. A further update is unmatched.
func TestCassette_ReplaysCompositionUpdate_PROBE061_PROBE012_PROBE011_PROBE071_REQ054_REQ094_REQ082(t *testing.T) {
	t.Parallel()
	for _, name := range compositionUpdateRecordings {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := replayClientFor(t, loadRecording(t, name))
			if err := replayCompositionUpdate(t.Context(), c); err != nil {
				t.Fatal(err)
			}
			_, err := ehr.Exists(t.Context(), c, ehr.EHRID("7d44b88c-4199-4bad-97dc-d78268e01398"))
			wantUnmatched(t, err)
		})
	}
}

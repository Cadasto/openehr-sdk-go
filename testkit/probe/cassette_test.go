package probe_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// ehrCreateRecording is the vendored EHRbase POST /ehr capture
// (ADR 0020 / STRAND-11). The path is resolved from this file so the
// test does not depend on the process working directory.
func ehrCreateRecording(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "recordings", "ehr-create.har")
}

// ehrLifecycleRecording is the vendored EHRbase capture of the create-then-read
// path (POST /ehr, then GET and HEAD the created id).
func ehrLifecycleRecording(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "recordings", "ehr-lifecycle.har")
}

// TestCassette_ReplaysEHRLifecycle replays the three-exchange capture and drives
// the same create-then-confirm sequence a probe would: create, then GET and
// HEAD the id the create returned. It witnesses that the recorded reader path
// resolves the id from the recorded create response — coverage the
// single-exchange POST /ehr recording cannot give, since GET and HEAD share a
// path and are told apart only by method.
func TestCassette_ReplaysEHRLifecycle(t *testing.T) {
	t.Parallel()
	har, err := probe.ValidateHAR(ehrLifecycleRecording(t))
	if err != nil {
		t.Fatal(err)
	}
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", probe.NewReplayer(har).HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}
	const want = "5fdb1b6a-fd89-4610-973e-e6a4d20b2cb5"

	rec, meta, err := ehr.Create(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil || rec.EHRID.Value != want {
		t.Fatalf("ehr.Create EHRID = %v, want %s", rec, want)
	}
	if meta == nil || meta.ETag == "" {
		t.Fatalf("ehr.Create metadata ETag = %v, want the recorded ETag", meta)
	}

	id := ehr.EHRID(rec.EHRID.Value)
	got, _, err := ehr.Get(t.Context(), c, id)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.EHRID.Value != want {
		t.Fatalf("ehr.Get EHRID = %v, want %s", got, want)
	}

	exists, err := ehr.Exists(t.Context(), c, id)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("ehr.Exists on the recorded id = false, want true")
	}
}

func TestCassette_ReplaysVendoredEHRCreate(t *testing.T) {
	t.Parallel()
	har, err := probe.ValidateHAR(ehrCreateRecording(t))
	if err != nil {
		t.Fatal(err)
	}
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", probe.NewReplayer(har).HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rec, meta, err := ehr.Create(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	const want = "dedb09f5-15ab-45e4-a8fe-8a9a961d5f21"
	if rec == nil || rec.EHRID.Value != want {
		t.Fatalf("ehr.Create EHRID = %v, want %s", rec, want)
	}
	if meta == nil || meta.ETag == "" {
		t.Fatalf("ehr.Create metadata ETag = %v, want the recorded ETag", meta)
	}
}

// REQ-082: the runner runs a probe in Cassette mode against a replayed
// recording.
func TestRun_CassetteReplaysVendoredEHRCreate(t *testing.T) {
	t.Parallel()
	har, err := probe.ValidateHAR(ehrCreateRecording(t))
	if err != nil {
		t.Fatal(err)
	}
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", probe.NewReplayer(har).HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}

	entry := probe.Entry{
		ID:     "ehr-create",
		Effect: probe.EffectMutating,
		Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
			rec, _, err := ehr.Create(ctx, c)
			if err != nil {
				return probe.Result{Status: probe.StatusFail, Detail: err.Error()}, nil
			}
			if rec == nil || rec.EHRID.Value == "" {
				return probe.Result{Status: probe.StatusFail, Detail: "empty EHR id"}, nil
			}
			return probe.Result{Status: probe.StatusPass}, nil
		},
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "recordings")

	sum, err := probe.Run(t.Context(), probe.Config{
		Mode:         probe.ModeCassette,
		Client:       c,
		RecordingDir: dir,
	}, []probe.Entry{entry})
	if err != nil {
		t.Fatalf("Run(cassette, ehr-create) = %v", err)
	}
	if !sum.Green() || sum.Passed != 1 {
		t.Fatalf("Run(cassette, ehr-create) green=%v passed=%d, want one pass", sum.Green(), sum.Passed)
	}
}

// REQ-082: a request the recording cannot answer fails loudly instead of
// falling back.
func TestCassette_UnmatchedCreateFailsClosed(t *testing.T) {
	t.Parallel()
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", probe.NewReplayer(probe.HAR{Log: probe.HARLog{
		Version: "1.2",
		Entries: []probe.HAREntry{{
			Request:  probe.HARRequest{Method: "GET", URL: "https://sandbox.local/openehr/v1/ehr/x"},
			Response: probe.HARResponse{Status: 200, Content: probe.HARContent{Text: `{}`}},
		}},
	}}).HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ehr.Create(t.Context(), c)
	if !errors.Is(err, probe.ErrUnmatchedRecording) {
		t.Fatalf("ehr.Create against a GET-only recording error = %v, want %v", err, probe.ErrUnmatchedRecording)
	}
}

// recordingFile resolves a vendored HAR next to this package. The path
// does not depend on the process working directory.
func recordingFile(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "recordings", name)
}

func replayClient(t *testing.T, harPath string) *transport.Client {
	t.Helper()
	har, err := probe.ValidateHAR(harPath)
	if err != nil {
		t.Fatal(err)
	}
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", probe.NewReplayer(har).HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// wantUnmatched fails the test unless err is the replayer's
// unmatched-recording error (REQ-082). An extra call the HAR does not
// contain must not pass through to a network.
func wantUnmatched(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, probe.ErrUnmatchedRecording) {
		t.Fatalf("extra request error = %v, want %v", err, probe.ErrUnmatchedRecording)
	}
}

// cassetteEHRStatusJSON matches the ehr-status scenario: a fixed status
// with is_modifiable true, and no client-supplied EHR id.
const cassetteEHRStatusJSON = `{"_type":"EHR_STATUS","name":{"_type":"DV_TEXT","value":"cassette EHR status"},"archetype_node_id":"openEHR-EHR-EHR_STATUS.generic.v1","subject":{"_type":"PARTY_SELF"},"is_queryable":true,"is_modifiable":true}`

// TestCassette_ReplaysEHRStatus_PROBE060_REQ095_REQ082 replays the EHR-status
// recording (PROBE-060, REQ-082, REQ-095): create with a fixed initial
// status, then read that status back. An extra HEAD is unmatched.
func TestCassette_ReplaysEHRStatus_PROBE060_REQ095_REQ082(t *testing.T) {
	t.Parallel()
	c := replayClient(t, recordingFile(t, "ehr-status.har"))
	var st rm.EHRStatus
	if err := canjson.Unmarshal([]byte(cassetteEHRStatusJSON), &st); err != nil {
		t.Fatal(err)
	}
	rec, _, err := ehr.Create(t.Context(), c, ehr.WithInitialStatus(&st))
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil || rec.EHRID.Value == "" {
		t.Fatalf("ehr.Create returned %#v, want an ehr id", rec)
	}
	got, _, err := ehrstatus.Get(t.Context(), c, ehr.EHRID(rec.EHRID.Value))
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("ehrstatus.Get returned nil")
	}
	_, err = ehr.Exists(t.Context(), c, ehr.EHRID(rec.EHRID.Value))
	wantUnmatched(t, err)
}

// cassetteCompositionForReplay rewrites the terminology_test fixture to
// the fixed template id the composition-minimal scenario uploaded.
func cassetteCompositionForReplay(t *testing.T) ([]byte, *rm.Composition) {
	t.Helper()
	const srcID = "terminology_test.ehrbase.org.v1"
	const dstID = "sdk_cassette_comp.v1"
	src, dst := []byte(srcID), []byte(dstID)
	opt, err := os.ReadFile(fixtures.TemplateOpt(srcID))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(fixtures.CompositionJSON(srcID))
	if err != nil {
		t.Fatal(err)
	}
	opt = bytes.ReplaceAll(opt, src, dst)
	raw = bytes.ReplaceAll(raw, src, dst)
	comp := &rm.Composition{}
	if err := canjson.Unmarshal(raw, comp); err != nil {
		t.Fatal(err)
	}
	return opt, comp
}

// TestCassette_ReplaysCompositionMinimal_PROBE065_REQ094_REQ082 replays the
// minimal composition recording (PROBE-065, REQ-082, REQ-094): create an
// EHR, upload the fixed OPT, save, then get the version the save returned.
// An extra HEAD is unmatched.
func TestCassette_ReplaysCompositionMinimal_PROBE065_REQ094_REQ082(t *testing.T) {
	t.Parallel()
	c := replayClient(t, recordingFile(t, "composition-minimal.har"))
	rec, _, err := ehr.Create(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil || rec.EHRID.Value == "" {
		t.Fatalf("ehr.Create returned %#v, want an ehr id", rec)
	}
	id := ehr.EHRID(rec.EHRID.Value)
	opt, comp := cassetteCompositionForReplay(t)
	if _, _, err := definition.UploadTemplate(t.Context(), c, definition.FormatADL14, bytes.NewReader(opt)); err != nil {
		t.Fatal(err)
	}
	_, meta, err := composition.Save(t.Context(), c, id, comp, composition.WithTemplateID("sdk_cassette_comp.v1"))
	if err != nil {
		t.Fatal(err)
	}
	if meta == nil || meta.VersionUID == "" {
		t.Fatalf("composition.Save metadata = %#v, want a version uid", meta)
	}
	got, _, err := composition.Get(t.Context(), c, id, ehr.VersionOf(meta.VersionUID))
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("composition.Get returned nil")
	}
	_, err = ehr.Exists(t.Context(), c, id)
	wantUnmatched(t, err)
}

// TestCassette_ReplaysStoredQuery_PROBE066_PROBE079_REQ057_REQ082 replays the
// stored-query recording (PROBE-066, PROBE-079, REQ-082, REQ-057): create
// an EHR, store one fixed query, then execute it. An extra HEAD is unmatched.
func TestCassette_ReplaysStoredQuery_PROBE066_PROBE079_REQ057_REQ082(t *testing.T) {
	t.Parallel()
	c := replayClient(t, recordingFile(t, "stored-query.har"))
	rec, _, err := ehr.Create(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil || rec.EHRID.Value == "" {
		t.Fatalf("ehr.Create returned %#v, want an ehr id", rec)
	}
	const name = "org.cadasto.sdk::cassette_stored"
	const aql = "SELECT e/ehr_id/value FROM EHR e WHERE e/ehr_id/value = $target_ehr"
	if _, _, err := definition.PutStoredQuery(t.Context(), c, name, aql); err != nil {
		t.Fatal(err)
	}
	rs, _, err := query.RunStored(t.Context(), c, name, map[string]any{"target_ehr": rec.EHRID.Value}, query.WithFetch(100))
	if err != nil {
		t.Fatal(err)
	}
	if rs == nil {
		t.Fatal("query.RunStored returned nil")
	}
	_, err = ehr.Exists(t.Context(), c, ehr.EHRID(rec.EHRID.Value))
	wantUnmatched(t, err)
}

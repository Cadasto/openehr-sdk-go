package probe_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// loadRecording reads and validates one vendored HAR. Each call reads the
// file again, so a caller that mutates the result changes only its own copy.
func loadRecording(t *testing.T, name string) probe.HAR {
	t.Helper()
	har, err := probe.ValidateHAR(recordingFile(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return har
}

// replayClientFor returns a client that replays har and reaches no network.
func replayClientFor(t *testing.T, har probe.HAR) *transport.Client {
	t.Helper()
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

// replayEHRStatus drives the ehr-status recording and checks what it holds:
// the create recovers an EHR id and its Location names that EHR, and the
// status read back carries the fields the create submitted. It returns the
// created id so the caller can make one more request against it.
func replayEHRStatus(ctx context.Context, c *transport.Client) (ehr.EHRID, error) {
	var st rm.EHRStatus
	if err := canjson.Unmarshal([]byte(cassetteEHRStatusJSON), &st); err != nil {
		return "", fmt.Errorf("decode the submitted EHR_STATUS: %w", err)
	}
	rec, meta, err := ehr.Create(ctx, c, ehr.WithInitialStatus(&st))
	if err != nil {
		return "", fmt.Errorf("ehr.Create: %w", err)
	}
	if rec == nil || rec.EHRID.Value == "" {
		return "", errors.New("create ehr_id: ehr.Create recovered no EHR id")
	}
	id := ehr.EHRID(rec.EHRID.Value)
	loc := ""
	if meta != nil && meta.Metadata != nil {
		loc = meta.Location
	}
	if wantTail := "/ehr/" + string(id); !strings.HasSuffix(strings.TrimSuffix(loc, "/"), wantTail) {
		return "", fmt.Errorf("create Location: %q does not end in %q", loc, wantTail)
	}
	got, _, err := ehrstatus.Get(ctx, c, id)
	if err != nil {
		return "", fmt.Errorf("ehrstatus.Get: %w", err)
	}
	if got == nil {
		return "", errors.New("ehrstatus.Get returned nil")
	}
	if got.ArchetypeNodeID != st.ArchetypeNodeID {
		return "", fmt.Errorf("read-back archetype_node_id: got %q, want %q", got.ArchetypeNodeID, st.ArchetypeNodeID)
	}
	if gotName, wantName := textValue(got.Name), textValue(st.Name); gotName != wantName {
		return "", fmt.Errorf("read-back name.value: got %q, want %q", gotName, wantName)
	}
	if got.IsQueryable != st.IsQueryable {
		return "", fmt.Errorf("read-back is_queryable: got %v, want %v", got.IsQueryable, st.IsQueryable)
	}
	if got.IsModifiable != st.IsModifiable {
		return "", fmt.Errorf("read-back is_modifiable: got %v, want %v", got.IsModifiable, st.IsModifiable)
	}
	return id, nil
}

// textValue returns the value of name, or "" when there is none.
func textValue(name rm.DVTextLike) string {
	if name == nil {
		return ""
	}
	return name.GetValue()
}

// TestCassette_ReplaysEHRStatus_PROBE060_REQ095_REQ082 replays the EHR-status
// recording (PROBE-060, REQ-082, REQ-095): create with a fixed initial
// status, then read that status back, checking the wire contract the
// recording holds. An extra HEAD is unmatched.
func TestCassette_ReplaysEHRStatus_PROBE060_REQ095_REQ082(t *testing.T) {
	t.Parallel()
	c := replayClientFor(t, loadRecording(t, "ehr-status.har"))
	id, err := replayEHRStatus(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ehr.Exists(t.Context(), c, id)
	wantUnmatched(t, err)
}

// cassetteTemplateID is the template id the composition-minimal recording
// was captured under.
const cassetteTemplateID = "sdk_cassette_comp.v1"

// cassetteComposition rewrites the terminology_test fixture OPT and
// composition to templateID, the id the composition-minimal recording was
// captured under.
func cassetteComposition(templateID string) ([]byte, *rm.Composition, error) {
	const srcID = "terminology_test.ehrbase.org.v1"
	src, dst := []byte(srcID), []byte(templateID)
	opt, err := os.ReadFile(fixtures.TemplateOpt(srcID))
	if err != nil {
		return nil, nil, err
	}
	raw, err := os.ReadFile(fixtures.CompositionJSON(srcID))
	if err != nil {
		return nil, nil, err
	}
	opt = bytes.ReplaceAll(opt, src, dst)
	raw = bytes.ReplaceAll(raw, src, dst)
	comp := &rm.Composition{}
	if err := canjson.Unmarshal(raw, comp); err != nil {
		return nil, nil, fmt.Errorf("decode the composition fixture: %w", err)
	}
	return opt, comp, nil
}

// templateIDOf returns the archetype_details.template_id of comp, or "".
func templateIDOf(comp *rm.Composition) string {
	if comp == nil || comp.ArchetypeDetails == nil || comp.ArchetypeDetails.TemplateID == nil {
		return ""
	}
	return comp.ArchetypeDetails.TemplateID.Value
}

// replayCompositionMinimal drives the composition-minimal recording under
// templateID and checks what it holds: the default save (Prefer
// return=minimal) returns no composition and a version uid, and the
// version read back carries the saved composition's archetype_node_id and
// template id. saveOpts are appended to the save's options; the scenario
// passes none, and only a can-fail row passes one, to show the
// no-composition check can fire. It returns the created EHR id.
func replayCompositionMinimal(ctx context.Context, c *transport.Client, templateID string, saveOpts ...composition.WriteOption) (ehr.EHRID, error) {
	rec, _, err := ehr.Create(ctx, c)
	if err != nil {
		return "", fmt.Errorf("ehr.Create: %w", err)
	}
	if rec == nil || rec.EHRID.Value == "" {
		return "", errors.New("create ehr_id: ehr.Create recovered no EHR id")
	}
	id := ehr.EHRID(rec.EHRID.Value)
	opt, comp, err := cassetteComposition(templateID)
	if err != nil {
		return "", err
	}
	if _, _, err := definition.UploadTemplate(ctx, c, definition.FormatADL14, bytes.NewReader(opt)); err != nil {
		return "", fmt.Errorf("definition.UploadTemplate: %w", err)
	}
	opts := append([]composition.WriteOption{composition.WithTemplateID(templateID)}, saveOpts...)
	saved, meta, err := composition.Save(ctx, c, id, comp, opts...)
	if err != nil {
		return "", fmt.Errorf("composition.Save: %w", err)
	}
	if saved != nil {
		return "", errors.New("save returned a composition: want none under the default Prefer return=minimal")
	}
	if meta == nil || meta.VersionUID == "" {
		return "", fmt.Errorf("save version uid: composition.Save metadata %#v carries no version uid", meta)
	}
	got, _, err := composition.Get(ctx, c, id, ehr.VersionOf(meta.VersionUID))
	if err != nil {
		return "", fmt.Errorf("composition.Get: %w", err)
	}
	if got == nil {
		return "", errors.New("composition.Get returned nil")
	}
	if got.ArchetypeNodeID != comp.ArchetypeNodeID {
		return "", fmt.Errorf("read-back archetype_node_id: got %q, want the saved %q", got.ArchetypeNodeID, comp.ArchetypeNodeID)
	}
	if gotID, wantID := templateIDOf(got), templateIDOf(comp); gotID != wantID {
		return "", fmt.Errorf("read-back template_id: got %q, want the saved %q", gotID, wantID)
	}
	return id, nil
}

// TestCassette_ReplaysCompositionMinimal_PROBE065_REQ094_REQ082 replays the
// minimal composition recording (PROBE-065, REQ-082, REQ-094): create an
// EHR, upload the OPT, save, then get the version the save returned,
// checking the wire contract the recording holds. An extra HEAD is
// unmatched.
func TestCassette_ReplaysCompositionMinimal_PROBE065_REQ094_REQ082(t *testing.T) {
	t.Parallel()
	c := replayClientFor(t, loadRecording(t, "composition-minimal.har"))
	id, err := replayCompositionMinimal(t.Context(), c, cassetteTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ehr.Exists(t.Context(), c, id)
	wantUnmatched(t, err)
}

// The stored-query scenario stores one fixed query under one fixed name.
const (
	cassetteStoredQueryName = "org.cadasto.sdk::cassette_stored"
	cassetteStoredQueryAQL  = "SELECT e/ehr_id/value FROM EHR e WHERE e/ehr_id/value = $target_ehr"
)

// replayStoredQuery drives the stored-query recording and checks what it
// holds: the store's metadata recovers the stored name and a version, and
// the execution returns a result set whose one row is the EHR the create
// returned, which the query filters on. It returns the created EHR id.
func replayStoredQuery(ctx context.Context, c *transport.Client) (ehr.EHRID, error) {
	rec, _, err := ehr.Create(ctx, c)
	if err != nil {
		return "", fmt.Errorf("ehr.Create: %w", err)
	}
	if rec == nil || rec.EHRID.Value == "" {
		return "", errors.New("create ehr_id: ehr.Create recovered no EHR id")
	}
	id := rec.EHRID.Value
	stored, _, err := definition.PutStoredQuery(ctx, c, cassetteStoredQueryName, cassetteStoredQueryAQL)
	if err != nil {
		return "", fmt.Errorf("definition.PutStoredQuery: %w", err)
	}
	if stored == nil {
		return "", errors.New("store name: definition.PutStoredQuery returned no metadata")
	}
	if stored.Name != cassetteStoredQueryName {
		return "", fmt.Errorf("store name: got %q, want %q", stored.Name, cassetteStoredQueryName)
	}
	// An unversioned store sends no version, so a non-empty one is what
	// the reply's Location gave back (REQ-057).
	if stored.Version == "" {
		return "", errors.New("store version: definition.PutStoredQuery recovered no version")
	}
	rs, _, err := query.RunStored(ctx, c, cassetteStoredQueryName, map[string]any{"target_ehr": id}, query.WithFetch(100))
	if err != nil {
		return "", fmt.Errorf("query.RunStored: %w", err)
	}
	if rs == nil {
		return "", errors.New("query.RunStored returned nil")
	}
	if len(rs.Columns) == 0 {
		return "", errors.New("result columns: the result set carries none")
	}
	if len(rs.Rows) == 0 || len(rs.Rows[0]) == 0 {
		return "", errors.New("result rows: the result set carries none")
	}
	if got, ok := rs.Rows[0][0].(string); !ok || got != id {
		return "", fmt.Errorf("result row value: got %v, want the created EHR id %q", rs.Rows[0][0], id)
	}
	return ehr.EHRID(id), nil
}

// TestCassette_ReplaysStoredQuery_PROBE066_PROBE079_REQ057_REQ082 replays the
// stored-query recording (PROBE-066, PROBE-079, REQ-082, REQ-057): create
// an EHR, store one fixed query, then execute it, checking the wire
// contract the recording holds. An extra HEAD is unmatched.
func TestCassette_ReplaysStoredQuery_PROBE066_PROBE079_REQ057_REQ082(t *testing.T) {
	t.Parallel()
	c := replayClientFor(t, loadRecording(t, "stored-query.har"))
	id, err := replayStoredQuery(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ehr.Exists(t.Context(), c, id)
	wantUnmatched(t, err)
}

// anyPath, as recordedEntry's suffix, matches every path: it selects the
// one exchange a recording holds for that method.
const anyPath = ""

// recordedEntry returns the one exchange in har whose method is method and
// whose URL path ends in suffix. It fails the test unless exactly one
// matches, so a mutation cannot land on the wrong exchange.
func recordedEntry(t *testing.T, har *probe.HAR, method, suffix string) *probe.HAREntry {
	t.Helper()
	var found *probe.HAREntry
	for i := range har.Log.Entries {
		e := &har.Log.Entries[i]
		u, err := url.Parse(e.Request.URL)
		if err != nil {
			t.Fatalf("recorded URL %q: %v", e.Request.URL, err)
		}
		if e.Request.Method != method || !strings.HasSuffix(u.Path, suffix) {
			continue
		}
		if found != nil {
			t.Fatalf("more than one recorded %s exchange ends in %q", method, suffix)
		}
		found = e
	}
	if found == nil {
		t.Fatalf("no recorded %s exchange ends in %q", method, suffix)
	}
	return found
}

// editResponseJSON decodes e's response body as a JSON object, applies
// edit, and stores the result. An edit that changes nothing fails the
// test, so a row cannot pass by mutating nothing.
func editResponseJSON(t *testing.T, e *probe.HAREntry, edit func(body map[string]any)) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(e.Response.Content.Text), &body); err != nil {
		t.Fatalf("recorded %s response body: %v", e.Request.Method, err)
	}
	before, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	edit(body)
	after, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, after) {
		t.Fatalf("mutation left the recorded %s response body unchanged", e.Request.Method)
	}
	e.Response.Content.Text = string(after)
}

// jsonObject returns the object m holds under key, failing the test when
// there is none.
func jsonObject(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	obj, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("recorded body has no object under %q", key)
	}
	return obj
}

// setResponseHeader replaces the value of a header e's response already
// carries, failing the test when it carries none.
func setResponseHeader(t *testing.T, e *probe.HAREntry, name, value string) {
	t.Helper()
	found := false
	for i := range e.Response.Headers {
		if strings.EqualFold(e.Response.Headers[i].Name, name) {
			e.Response.Headers[i].Value = value
			found = true
		}
	}
	if !found {
		t.Fatalf("recorded %s response carries no %s header", e.Request.Method, name)
	}
}

// dropResponseHeader removes a header e's response carries, failing the
// test when it carries none.
func dropResponseHeader(t *testing.T, e *probe.HAREntry, name string) {
	t.Helper()
	kept := e.Response.Headers[:0]
	for _, h := range e.Response.Headers {
		if !strings.EqualFold(h.Name, name) {
			kept = append(kept, h)
		}
	}
	if len(kept) == len(e.Response.Headers) {
		t.Fatalf("recorded %s response carries no %s header", e.Request.Method, name)
	}
	e.Response.Headers = kept
}

// TestCassette_ScenarioChecksFailOnAMutatedRecording_REQ082_REQ095_REQ094_REQ057
// is the can-fail control for the scenario witnesses above (REQ-082,
// REQ-095, REQ-094, REQ-057). Each row loads one recording, changes the
// one recorded value a check reads, replays the scenario against that
// copy, and requires the error to name that check. With a check removed,
// the replay no longer names it and that row fails.
func TestCassette_ScenarioChecksFailOnAMutatedRecording_REQ082_REQ095_REQ094_REQ057(t *testing.T) {
	t.Parallel()

	ehrStatus := func(ctx context.Context, c *transport.Client, _ probe.HAR) error {
		_, err := replayEHRStatus(ctx, c)
		return err
	}
	compositionMinimal := func(saveOpts ...composition.WriteOption) func(context.Context, *transport.Client, probe.HAR) error {
		return func(ctx context.Context, c *transport.Client, _ probe.HAR) error {
			_, err := replayCompositionMinimal(ctx, c, cassetteTemplateID, saveOpts...)
			return err
		}
	}
	storedQuery := func(ctx context.Context, c *transport.Client, _ probe.HAR) error {
		_, err := replayStoredQuery(ctx, c)
		return err
	}
	const otherEHRID = "00000000-0000-4000-8000-000000000000"

	for _, tc := range []struct {
		name      string
		recording string
		mutate    func(t *testing.T, har *probe.HAR)
		replay    func(ctx context.Context, c *transport.Client, har probe.HAR) error
		want      string
	}{
		{
			name:      "ehr-status create recovers no EHR id",
			recording: "ehr-status.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				editResponseJSON(t, recordedEntry(t, har, "POST", "/ehr"), func(b map[string]any) {
					jsonObject(t, b, "ehr_id")["value"] = ""
				})
			},
			replay: ehrStatus,
			want:   "create ehr_id",
		},
		{
			name:      "ehr-status create Location names another EHR",
			recording: "ehr-status.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				setResponseHeader(t, recordedEntry(t, har, "POST", "/ehr"), "Location",
					"http://127.0.0.1:8080/ehrbase/rest/openehr/v1/ehr/"+otherEHRID)
			},
			replay: ehrStatus,
			want:   "create Location",
		},
		{
			name:      "ehr-status read-back archetype_node_id differs",
			recording: "ehr-status.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				editResponseJSON(t, recordedEntry(t, har, "GET", "/ehr_status"), func(b map[string]any) {
					b["archetype_node_id"] = "openEHR-EHR-EHR_STATUS.other.v1"
				})
			},
			replay: ehrStatus,
			want:   "read-back archetype_node_id",
		},
		{
			name:      "ehr-status read-back name.value differs",
			recording: "ehr-status.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				editResponseJSON(t, recordedEntry(t, har, "GET", "/ehr_status"), func(b map[string]any) {
					jsonObject(t, b, "name")["value"] = "another EHR status"
				})
			},
			replay: ehrStatus,
			want:   "read-back name.value",
		},
		{
			name:      "ehr-status read-back is_queryable flipped",
			recording: "ehr-status.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				editResponseJSON(t, recordedEntry(t, har, "GET", "/ehr_status"), func(b map[string]any) {
					b["is_queryable"] = false
				})
			},
			replay: ehrStatus,
			want:   "read-back is_queryable",
		},
		{
			name:      "ehr-status read-back is_modifiable flipped",
			recording: "ehr-status.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				editResponseJSON(t, recordedEntry(t, har, "GET", "/ehr_status"), func(b map[string]any) {
					b["is_modifiable"] = false
				})
			},
			replay: ehrStatus,
			want:   "read-back is_modifiable",
		},
		{
			// Under return=minimal the SDK never decodes a body, so no change
			// to the recording alone can make the default save return a
			// composition. This row asks for a representation and records
			// one, which is the only way to show the check can fire.
			name:      "composition-minimal save returns a composition",
			recording: "composition-minimal.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				save := recordedEntry(t, har, "POST", "/composition")
				read := recordedEntry(t, har, "GET", anyPath)
				save.Response.Status = 201
				save.Response.Content.Text = read.Response.Content.Text
				save.Response.Headers = append(save.Response.Headers, probe.HARHeader{Name: "Content-Type", Value: "application/json"})
			},
			replay: compositionMinimal(composition.WithPrefer(transport.PreferRepresentation)),
			want:   "save returned a composition",
		},
		{
			name:      "composition-minimal save carries no version uid",
			recording: "composition-minimal.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				dropResponseHeader(t, recordedEntry(t, har, "POST", "/composition"), "Location")
			},
			replay: compositionMinimal(),
			want:   "save version uid",
		},
		{
			name:      "composition-minimal read-back has no archetype_node_id",
			recording: "composition-minimal.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				editResponseJSON(t, recordedEntry(t, har, "GET", anyPath), func(b map[string]any) {
					delete(b, "archetype_node_id")
				})
			},
			replay: compositionMinimal(),
			want:   "read-back archetype_node_id",
		},
		{
			name:      "composition-minimal read-back template_id differs",
			recording: "composition-minimal.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				editResponseJSON(t, recordedEntry(t, har, "GET", anyPath), func(b map[string]any) {
					jsonObject(t, jsonObject(t, b, "archetype_details"), "template_id")["value"] = "another_template.v1"
				})
			},
			replay: compositionMinimal(),
			want:   "read-back template_id",
		},
		{
			name:      "stored-query store Location names another query",
			recording: "stored-query.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				setResponseHeader(t, recordedEntry(t, har, "PUT", "/definition/query/"+cassetteStoredQueryName), "Location",
					"http://127.0.0.1:8080/ehrbase/rest/openehr/v1/definition/query/org.cadasto.sdk::another_stored/2.0.0")
			},
			replay: storedQuery,
			want:   "store name",
		},
		{
			name:      "stored-query store reply carries no Location",
			recording: "stored-query.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				dropResponseHeader(t, recordedEntry(t, har, "PUT", "/definition/query/"+cassetteStoredQueryName), "Location")
			},
			replay: storedQuery,
			want:   "store version",
		},
		{
			name:      "stored-query result has no columns",
			recording: "stored-query.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				editResponseJSON(t, recordedEntry(t, har, "POST", "/query/"+cassetteStoredQueryName), func(b map[string]any) {
					b["columns"] = []any{}
				})
			},
			replay: storedQuery,
			want:   "result columns",
		},
		{
			name:      "stored-query result has no rows",
			recording: "stored-query.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				editResponseJSON(t, recordedEntry(t, har, "POST", "/query/"+cassetteStoredQueryName), func(b map[string]any) {
					b["rows"] = []any{}
				})
			},
			replay: storedQuery,
			want:   "result rows",
		},
		{
			name:      "stored-query result row is another EHR",
			recording: "stored-query.har",
			mutate: func(t *testing.T, har *probe.HAR) {
				editResponseJSON(t, recordedEntry(t, har, "POST", "/query/"+cassetteStoredQueryName), func(b map[string]any) {
					b["rows"] = []any{[]any{otherEHRID}}
				})
			},
			replay: storedQuery,
			want:   "result row value",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			har := loadRecording(t, tc.recording)
			tc.mutate(t, &har)
			err := tc.replay(t.Context(), replayClientFor(t, har), har)
			if err == nil {
				t.Fatalf("replay of %s with %s = nil error, want one naming %q", tc.recording, tc.name, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("replay of %s with %s = %v, want an error naming %q", tc.recording, tc.name, err, tc.want)
			}
		})
	}
}

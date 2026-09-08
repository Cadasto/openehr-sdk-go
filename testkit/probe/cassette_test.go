package probe_test

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
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

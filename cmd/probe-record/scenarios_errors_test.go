package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

// errorServer answers the error-envelopes scenario the way EHRbase 2.36.0
// does: the sandbox's EHR surface answers 404 for an EHR it never created,
// and every ad-hoc query is refused with 400 and an {error, message}
// envelope. With accept set it answers each query with an empty result set
// instead, a success the scenario must refuse to record as a refusal.
type errorServer struct{ accept bool }

func (s errorServer) backend() *sandbox.Backend {
	b := sandbox.New()
	b.HandleFunc(http.MethodPost, "/query/aql", s.runAQL)
	return b
}

func (s errorServer) runAQL(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.accept {
		_, _ = io.WriteString(w, `{"meta":{"_type":"RESULTSET"},"q":"","columns":[],"rows":[]}`)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	_, _ = io.WriteString(w, `{"error":"Bad Request","message":"refused"}`)
}

// TestCaptureScenario_ErrorEnvelopesRecordsTheRefusals drives the
// error-envelopes scenario through the recorder against an in-memory server,
// with no CDR and no network. The recording holds the read of the fixed
// unknown EHR id, refused with 404, then the scenario's three fixed queries
// in order, each refused with 400.
func TestCaptureScenario_ErrorEnvelopesRecordsTheRefusals(t *testing.T) {
	t.Parallel()
	recPath, err := captureScenario(t.Context(), scenarios["error-envelopes"], sandboxBase,
		errorServer{}.backend(), nil, validProvenance(), t.TempDir())
	if err != nil {
		t.Fatalf("captureScenario(error-envelopes) against the refusing server = %v", err)
	}
	if filepath.Base(recPath) != "error-envelopes.har" {
		t.Fatalf("recording path = %q, want basename error-envelopes.har", recPath)
	}
	har, err := probe.ValidateHAR(recPath)
	if err != nil {
		t.Fatalf("published recording did not validate: %v", err)
	}

	calls, statuses := recordedCalls(t, har)
	wantCalls := []string{"GET /ehr/" + cassetteUnknownEHRID, "POST /query/aql", "POST /query/aql", "POST /query/aql"}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("error-envelopes recorded exchanges %q, want %q", calls, wantCalls)
	}
	if want := []int{http.StatusNotFound, http.StatusBadRequest, http.StatusBadRequest, http.StatusBadRequest}; !slices.Equal(statuses, want) {
		t.Fatalf("error-envelopes recorded statuses %v, want %v", statuses, want)
	}

	var sent []string
	for _, e := range har.Log.Entries[1:] {
		if e.Request.PostData == nil {
			t.Fatal("a recorded query carries no request body, want the AQL text")
		}
		var body struct {
			Q string `json:"q"`
		}
		if err := json.Unmarshal([]byte(e.Request.PostData.Text), &body); err != nil {
			t.Fatalf("recorded query body %s: %v", e.Request.PostData.Text, err)
		}
		sent = append(sent, body.Q)
	}
	if want := []string{cassetteMalformedAQL, cassetteUnknownAttributeAQL, cassetteUnsupportedPathAQL}; !slices.Equal(sent, want) {
		t.Fatalf("recorded queries %q, want %q", sent, want)
	}
}

// TestCaptureScenario_ErrorEnvelopesRefusesAnAnsweredQuery: a server that
// answers a query the scenario expects refused fails the capture, which
// names that query and writes nothing, so a success is never published as
// a recorded refusal.
func TestCaptureScenario_ErrorEnvelopesRefusesAnAnsweredQuery(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := captureScenario(t.Context(), scenarios["error-envelopes"], sandboxBase,
		errorServer{accept: true}.backend(), nil, validProvenance(), dir)
	if err == nil || !strings.Contains(err.Error(), cassetteMalformedAQL) {
		t.Fatalf("captureScenario(error-envelopes) against an answering server = %v, want a refusal naming %q", err, cassetteMalformedAQL)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("the refused capture left %d entries in the output directory, want none", len(entries))
	}
}

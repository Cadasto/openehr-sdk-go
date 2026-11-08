package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

// contributionServer answers the contribution-audit scenario. The sandbox
// serves each POST /ehr itself; this server answers each POST
// /ehr/{id}/contribution with the next of statuses, in call order, so a
// test scripts which variant the server refuses. A 2xx carries no body, as
// a commit under Prefer return=minimal does; any other status carries an
// {error, message} envelope.
type contributionServer struct {
	statuses []int

	mu    sync.Mutex
	calls int
}

func (s *contributionServer) backend() *sandbox.Backend {
	b := sandbox.New()
	b.HandleFunc(http.MethodPost, "/ehr/", s.commit)
	return b
}

func (s *contributionServer) commit(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/contribution") {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	i := s.calls
	s.calls++
	s.mu.Unlock()
	status := http.StatusInternalServerError
	if i < len(s.statuses) {
		status = s.statuses[i]
	}
	if status/100 == 2 {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, `{"error":"Bad Request","message":"refused"}`)
}

// recordedAudit is the part of a commit audit the scenario varies. A nil
// SystemID means the audit carries no system_id member at all.
type recordedAudit struct {
	Type     string  `json:"_type"`
	SystemID *string `json:"system_id"`
}

// recordedCommitAudits decodes a recorded contribution body and returns its
// batch audit and the commit_audit of its one version.
func recordedCommitAudits(t *testing.T, e probe.HAREntry) (batch, version recordedAudit) {
	t.Helper()
	if e.Request.PostData == nil {
		t.Fatalf("recorded %s %s carries no request body, want the contribution", e.Request.Method, e.Request.URL)
	}
	var body struct {
		Audit    recordedAudit `json:"audit"`
		Versions []struct {
			CommitAudit recordedAudit `json:"commit_audit"`
		} `json:"versions"`
	}
	if err := json.Unmarshal([]byte(e.Request.PostData.Text), &body); err != nil {
		t.Fatalf("recorded contribution body %s: %v", e.Request.PostData.Text, err)
	}
	if len(body.Versions) != 1 {
		t.Fatalf("recorded contribution body carries %d versions, want 1", len(body.Versions))
	}
	return body.Audit, body.Versions[0].CommitAudit
}

// systemIDOf renders a recorded system_id for a failure message.
func systemIDOf(a recordedAudit) string {
	if a.SystemID == nil {
		return "<absent>"
	}
	return *a.SystemID
}

// TestCaptureScenario_ContributionAuditRecordsTheThreeCommits_REQ130_REQ082 drives the
// contribution-audit scenario through the recorder against an in-memory
// server that refuses the first and third commits with 400, as EHRbase
// 2.37.1 does, with no CDR and no network. The recording holds three
// creates, each followed by a commit to the EHR it created, and each
// commit's body carries its variant's audit on the batch and on the
// version, which inherits it (REQ-130).
func TestCaptureScenario_ContributionAuditRecordsTheThreeCommits_REQ130_REQ082(t *testing.T) {
	t.Parallel()
	srv := &contributionServer{statuses: []int{http.StatusBadRequest, http.StatusCreated, http.StatusBadRequest}}
	recPath, err := captureScenario(t.Context(), scenarios["contribution-audit"], sandboxBase,
		srv.backend(), nil, validProvenance(), t.TempDir())
	if err != nil {
		t.Fatalf("captureScenario(contribution-audit) against the refusing server = %v", err)
	}
	if filepath.Base(recPath) != "contribution-audit.har" {
		t.Fatalf("recording path = %q, want basename contribution-audit.har", recPath)
	}
	har, err := probe.ValidateHAR(recPath)
	if err != nil {
		t.Fatalf("published recording did not validate: %v", err)
	}

	calls, statuses := recordedCalls(t, har)
	if len(calls) != 6 {
		t.Fatalf("contribution-audit recorded exchanges %q, want 6", calls)
	}
	var wantCalls []string
	seen := map[string]bool{}
	for i := 0; i < len(har.Log.Entries); i += 2 {
		var created struct {
			EHRID struct {
				Value string `json:"value"`
			} `json:"ehr_id"`
		}
		if err := json.Unmarshal([]byte(har.Log.Entries[i].Response.Content.Text), &created); err != nil || created.EHRID.Value == "" {
			t.Fatalf("recorded create %d answered no ehr_id: %v", i/2, err)
		}
		id := created.EHRID.Value
		if seen[id] {
			t.Fatalf("two commits went to EHR %s, want an EHR of their own each", id)
		}
		seen[id] = true
		wantCalls = append(wantCalls, "POST /ehr", "POST /ehr/"+id+"/contribution")
	}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("contribution-audit recorded exchanges %q, want %q", calls, wantCalls)
	}
	wantStatuses := []int{
		http.StatusCreated, http.StatusBadRequest,
		http.StatusCreated, http.StatusCreated,
		http.StatusCreated, http.StatusBadRequest,
	}
	if !slices.Equal(statuses, wantStatuses) {
		t.Fatalf("contribution-audit recorded statuses %v, want %v", statuses, wantStatuses)
	}

	for i, want := range []struct {
		variant  string
		typ      string
		systemID string // "" means absent
	}{
		{variant: "without a system id", typ: "AUDIT_DETAILS"},
		{variant: "with a system id", typ: "AUDIT_DETAILS", systemID: cassetteContributionSystemID},
		{variant: "UPDATE_AUDIT", typ: "UPDATE_AUDIT", systemID: cassetteContributionSystemID},
	} {
		batch, version := recordedCommitAudits(t, har.Log.Entries[2*i+1])
		for _, a := range []struct {
			where string
			audit recordedAudit
		}{{"audit", batch}, {"versions[0].commit_audit", version}} {
			if a.audit.Type != want.typ {
				t.Errorf("%s: recorded %s _type = %q, want %q", want.variant, a.where, a.audit.Type, want.typ)
			}
			if want.systemID == "" && a.audit.SystemID != nil {
				t.Errorf("%s: recorded %s system_id = %q, want none", want.variant, a.where, *a.audit.SystemID)
			}
			if want.systemID != "" && (a.audit.SystemID == nil || *a.audit.SystemID != want.systemID) {
				t.Errorf("%s: recorded %s system_id = %s, want %q", want.variant, a.where, systemIDOf(a.audit), want.systemID)
			}
		}
	}
}

// TestCaptureScenario_ContributionAuditRefusals_REQ082: the capture
// records any deployment, so the commits without a system id and with
// UPDATE_AUDIT may be accepted or refused with a 4xx. Anything else fails
// the capture, which names the variant and writes nothing: a refused
// commit with a system id, or a 5xx on either of the others.
func TestCaptureScenario_ContributionAuditRefusals_REQ082(t *testing.T) {
	t.Parallel()
	const (
		ok      = http.StatusCreated
		refused = http.StatusBadRequest
		broken  = http.StatusInternalServerError
	)
	for _, tc := range []struct {
		name     string
		statuses []int
		want     string // "" means the capture succeeds
	}{
		{name: "every commit accepted", statuses: []int{ok, ok, ok}},
		{name: "commit with a system id refused", statuses: []int{refused, refused, refused}, want: "with a system id"},
		{name: "commit without a system id answered 500", statuses: []int{broken, ok, refused}, want: "without a system id"},
		{name: "UPDATE_AUDIT commit answered 500", statuses: []int{refused, ok, broken}, want: "UPDATE_AUDIT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			srv := &contributionServer{statuses: tc.statuses}
			_, err := captureScenario(t.Context(), scenarios["contribution-audit"], sandboxBase,
				srv.backend(), nil, validProvenance(), dir)
			entries, rerr := os.ReadDir(dir)
			if rerr != nil {
				t.Fatal(rerr)
			}
			if tc.want == "" {
				if err != nil {
					t.Fatalf("captureScenario(contribution-audit) with statuses %v = %v, want success", tc.statuses, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("captureScenario(contribution-audit) with statuses %v = %v, want a failure naming %q", tc.statuses, err, tc.want)
			}
			if len(entries) != 0 {
				t.Fatalf("the failed capture left %d entries in the output directory, want none", len(entries))
			}
		})
	}
}

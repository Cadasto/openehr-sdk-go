package queryprobes_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/query"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const scopedEHRID = "1234abcd-5678-9012-3456-7890abcdef00"

func newClient(t *testing.T, b *sandbox.Backend) *transport.Client {
	t.Helper()
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", b.HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// emptyResultSet decodes as an aql.ResultSet with no rows, so every query
// execution returns cleanly and the probe's assertion is purely on request
// shape.
const emptyResultSet = `{"_type":"RESULTSET","columns":[],"rows":[]}`

// scopeBackend records every request and answers every query route with an
// empty result set. mutate, when non-nil, rewrites the recorded clone (never
// the live request) so a can-fail test can move the scope onto the wrong
// channel and prove the probe's assertion bites.
func scopeBackend(mutate func(*http.Request)) (*sandbox.Backend, func() []*http.Request) {
	var (
		mu       sync.Mutex
		captured []*http.Request
	)
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		clone := r.Clone(r.Context())
		if mutate != nil {
			mutate(clone)
		}
		mu.Lock()
		captured = append(captured, clone)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(emptyResultSet))
	})
	snapshot := func() []*http.Request {
		mu.Lock()
		defer mu.Unlock()
		return append([]*http.Request(nil), captured...)
	}
	return b, snapshot
}

func setParam(r *http.Request, k, v string) {
	qv := r.URL.Query()
	qv.Set(k, v)
	r.URL.RawQuery = qv.Encode()
}

func delParam(r *http.Request, k string) {
	qv := r.URL.Query()
	qv.Del(k)
	r.URL.RawQuery = qv.Encode()
}

func TestProbe078VerbAwareScoping(t *testing.T) { // PROBE-078, REQ-055
	b, snapshot := scopeBackend(nil)
	r, err := probes.Probe078VerbAwareScoping(t.Context(), newClient(t, b), snapshot, scopedEHRID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-078 = %s: %s", r.Status, r.Detail)
	}
	if got := snapshot(); len(got) != 4 {
		t.Errorf("captured %d requests, want 4 (ad-hoc and stored, each POST and GET)", len(got))
	}
}

// TestProbe078FlagsMisScopedRequests plants one mis-scoping per case — each
// exercises a different one of the probe's four assertions (POST must carry the
// header and no ehr_id param; GET must carry the param and no header), so a
// probe that stopped checking any of them would fail here rather than keep
// reporting green. Each case also names the substring its Detail must carry, so
// a case cannot rot into failing for an unrelated reason.
func TestProbe078FlagsMisScopedRequests(t *testing.T) { // PROBE-078
	cases := []struct {
		name       string
		mutate     func(*http.Request)
		wantDetail string
	}{
		{
			name:       "POST scope moved off the header",
			mutate:     func(r *http.Request) { r.Header.Del("openehr-ehr-id") },
			wantDetail: "openehr-ehr-id header",
		},
		{
			name:       "POST carries a stray ehr_id query parameter",
			mutate:     func(r *http.Request) { setParam(r, "ehr_id", scopedEHRID) },
			wantDetail: "ehr_id query parameter",
		},
		{
			name:       "GET scope moved off the query parameter",
			mutate:     func(r *http.Request) { delParam(r, "ehr_id") },
			wantDetail: "ehr_id query parameter",
		},
		{
			name:       "GET carries a stray openehr-ehr-id header",
			mutate:     func(r *http.Request) { r.Header.Set("openehr-ehr-id", scopedEHRID) },
			wantDetail: "openehr-ehr-id header",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The mutation applies only to the verb the case targets; the other
			// verb's arm must still pass so the probe reaches the arm under test.
			method := http.MethodPost
			if strings.HasPrefix(tc.name, "GET") {
				method = http.MethodGet
			}
			b, snapshot := scopeBackend(func(r *http.Request) {
				if r.Method == method {
					tc.mutate(r)
				}
			})
			r, err := probes.Probe078VerbAwareScoping(t.Context(), newClient(t, b), snapshot, scopedEHRID)
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "fail" {
				t.Fatalf("PROBE-078 = %s, want fail (detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.wantDetail) {
				t.Errorf("PROBE-078 failed for the wrong reason:\n got: %s\nwant it to mention: %s", r.Detail, tc.wantDetail)
			}
		})
	}
}

func TestProbe078RejectsMissingInputs(t *testing.T) { // PROBE-078
	b, snapshot := scopeBackend(nil)
	if _, err := probes.Probe078VerbAwareScoping(t.Context(), nil, snapshot, scopedEHRID); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := probes.Probe078VerbAwareScoping(t.Context(), newClient(t, b), nil, scopedEHRID); err == nil {
		t.Error("nil recorder: want an error")
	}
	if _, err := probes.Probe078VerbAwareScoping(t.Context(), newClient(t, b), snapshot, ""); err == nil {
		t.Error("empty ehr id: want an error")
	}
}

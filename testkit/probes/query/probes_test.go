package queryprobes_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
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

// The endpoint a captured request addresses, as classified by endpointOf. A
// can-fail case names one to plant its mis-scoping on that endpoint alone.
const (
	endpointAdhoc     = "adhoc"
	endpointStored    = "stored"
	endpointVersioned = "versioned"
	endpointUnknown   = "unknown"
)

// endpointOf classifies a captured query request by its path. The sandbox base
// is /openehr/v1, so what follows the "/query/" segment is either "aql" (the
// ad-hoc route), the stored query's qualified name alone, or that name plus a
// version. The qualified name contains "::" and arrives percent-decoded in
// URL.Path, so it is still a single path segment.
func endpointOf(t *testing.T, r *http.Request) string {
	t.Helper()
	_, rest, ok := strings.Cut(r.URL.Path, "/query/")
	if !ok {
		t.Errorf("captured request path %q carries no /query/ segment", r.URL.Path)
		return endpointUnknown
	}
	segs := strings.Split(rest, "/")
	switch {
	case len(segs) == 1 && segs[0] == "aql":
		return endpointAdhoc
	case len(segs) == 1:
		return endpointStored
	case len(segs) == 2:
		return endpointVersioned
	default:
		t.Errorf("captured request path %q has %d segments after /query/, want 1 or 2", r.URL.Path, len(segs))
		return endpointUnknown
	}
}

// scopeBackend records every request and answers every query route with an
// empty result set. Each recorded clone gets its own buffered copy of the
// request body — the live one is closed once the exchange is served, and the
// probe reads the body on its POST arms. mutate, when non-nil, rewrites the
// clone (never the live request) so a can-fail test can move the scope onto
// the wrong channel and prove the probe's assertion bites; it runs after the
// body is buffered, so it can rewrite that too.
func scopeBackend(t *testing.T, mutate func(*http.Request)) (*sandbox.Backend, func() []*http.Request) {
	t.Helper()
	var (
		mu       sync.Mutex
		captured []*http.Request
	)
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil { // a GET carries no body at all
			var err error
			if body, err = io.ReadAll(r.Body); err != nil {
				t.Errorf("reading the live %s %s request body: %v", r.Method, r.URL.Path, err)
			}
		}
		clone := r.Clone(r.Context())
		clone.Body = io.NopCloser(bytes.NewReader(body))
		clone.ContentLength = int64(len(body))
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
		return slices.Clone(captured)
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

// setBodyField rewrites r's buffered JSON body with k set to v.
func setBodyField(t *testing.T, r *http.Request, k string, v any) {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("reading the captured %s body: %v", r.Method, err)
		return
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Errorf("captured %s body is not a JSON object: %v", r.Method, err)
		return
	}
	m[k] = v
	out, err := json.Marshal(m)
	if err != nil {
		t.Errorf("re-encoding the captured %s body: %v", r.Method, err)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(out))
	r.ContentLength = int64(len(out))
}

func TestProbe078VerbAwareScoping(t *testing.T) { // PROBE-078, REQ-055
	b, snapshot := scopeBackend(t, nil)
	r, err := probes.Probe078VerbAwareScoping(t.Context(), newClient(t, b), snapshot, scopedEHRID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-078 = %s: %s", r.Status, r.Detail)
	}
	if got := snapshot(); len(got) != 6 {
		t.Errorf("captured %d requests, want 6 (ad-hoc, stored and stored-versioned, each POST and GET)", len(got))
	}
}

// TestProbe078FlagsMisScopedRequests plants one mis-scoping per case — each
// exercises a different one of the probe's assertions (a POST must carry the
// header and neither an ehr_id query parameter nor an ehr_id body field; a GET
// must carry the parameter and no header), so a probe that stopped checking
// any of them would fail here rather than keep reporting green. The last two
// cases pin their plant to one endpoint, so dropping that endpoint's arm from
// the probe leaves nothing mutated and the case fails on the pass. Each case
// also names the substring its Detail must carry, so a case cannot rot into
// failing for an unrelated reason.
func TestProbe078FlagsMisScopedRequests(t *testing.T) { // PROBE-078
	cases := []struct {
		name string
		// method and endpoint select which captured request the plant lands
		// on; an empty endpoint means any.
		method     string
		endpoint   string
		mutate     func(*testing.T, *http.Request)
		wantDetail string
	}{
		{
			name:       "POST scope moved off the header",
			method:     http.MethodPost,
			mutate:     func(_ *testing.T, r *http.Request) { r.Header.Del("openehr-ehr-id") },
			wantDetail: "openehr-ehr-id header",
		},
		{
			name:       "POST carries a stray ehr_id query parameter",
			method:     http.MethodPost,
			mutate:     func(_ *testing.T, r *http.Request) { setParam(r, "ehr_id", scopedEHRID) },
			wantDetail: "ehr_id query parameter",
		},
		{
			name:       "POST carries an ehr_id request-body field",
			method:     http.MethodPost,
			mutate:     func(t *testing.T, r *http.Request) { setBodyField(t, r, "ehr_id", scopedEHRID) },
			wantDetail: "ehr_id request-body field",
		},
		{
			name:       "GET scope moved off the query parameter",
			method:     http.MethodGet,
			mutate:     func(_ *testing.T, r *http.Request) { delParam(r, "ehr_id") },
			wantDetail: "ehr_id query parameter",
		},
		{
			name:       "GET carries a stray openehr-ehr-id header",
			method:     http.MethodGet,
			mutate:     func(_ *testing.T, r *http.Request) { r.Header.Set("openehr-ehr-id", scopedEHRID) },
			wantDetail: "openehr-ehr-id header",
		},
		{
			name:       "stored POST scope moved off the header",
			method:     http.MethodPost,
			endpoint:   endpointStored,
			mutate:     func(_ *testing.T, r *http.Request) { r.Header.Del("openehr-ehr-id") },
			wantDetail: "stored POST /query/{name}: openehr-ehr-id header",
		},
		{
			name:       "stored-versioned GET scope moved off the query parameter",
			method:     http.MethodGet,
			endpoint:   endpointVersioned,
			mutate:     func(_ *testing.T, r *http.Request) { delParam(r, "ehr_id") },
			wantDetail: "stored-versioned GET /query/{name}/{version}: ehr_id query parameter",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The plant lands only on the request the case targets; every
			// other arm must still pass so the probe reaches the one under
			// test.
			b, snapshot := scopeBackend(t, func(r *http.Request) {
				if ep := endpointOf(t, r); r.Method != tc.method || (tc.endpoint != "" && tc.endpoint != ep) {
					return
				}
				tc.mutate(t, r)
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
	b, snapshot := scopeBackend(t, nil)
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

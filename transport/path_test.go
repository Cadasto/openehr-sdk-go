package transport

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestValidatePathSegment pins the per-parameter rule: a segment that IS
// `.` / `..`, is empty, or carries `/`, `\`, or a control character is
// refused; everything a well-formed openEHR identifier can contain passes
// (REQ-150).
func TestValidatePathSegment(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"Referral Request.v1", false},
		{"openEHR-EHR-COMPOSITION.t_vital_signs.v1", false},
		{"..", true},
		{".", true},
		{"", true},
		{"a/b", true},
		{`a\b`, true},
		{"a\x00b", true},
		{"a\x7fb", true},  // DEL — path.go refuses it alongside the C0 range
		{"%2e%2e", false}, // not decoded — ordinary segment
	}
	for _, tc := range cases {
		err := ValidatePathSegment(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("ValidatePathSegment(%q) = nil, want an error", tc.in)
			continue
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ValidatePathSegment(%q) = %v, want nil", tc.in, err)
			continue
		}
		if tc.wantErr && !errors.Is(err, ErrInvalidPathSegment) {
			t.Errorf("ValidatePathSegment(%q) = %v, want ErrInvalidPathSegment in the chain", tc.in, err)
		}
		if tc.wantErr && !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("ValidatePathSegment(%q) = %v, want ErrInvalidConfig in the chain", tc.in, err)
		}
	}
}

// TestValidatePathSegmentSentinelsAreIndependent pins that the two
// sentinels are distinct values joined only by the returned chain — the
// architecture note in the plan, and what lets a caller branch on either
// without the other implying it (REQ-150).
func TestValidatePathSegmentSentinelsAreIndependent(t *testing.T) {
	if errors.Is(ErrInvalidPathSegment, ErrInvalidConfig) {
		t.Error("ErrInvalidPathSegment must not itself wrap ErrInvalidConfig")
	}
	if errors.Is(ErrInvalidConfig, ErrInvalidPathSegment) {
		t.Error("ErrInvalidConfig must not itself wrap ErrInvalidPathSegment")
	}
}

// TestValidateRequestPathAcceptsTheServiceRoot pins the carve-out REQ-150
// makes for `OPTIONS /` — the System API's only operation, issued by the
// landed system.Capabilities / Version / Health.
func TestValidateRequestPathAcceptsTheServiceRoot(t *testing.T) {
	if err := ValidateRequestPath("/"); err != nil {
		t.Fatalf("service root refused: %v (the landed System API sends OPTIONS /)", err)
	}
}

// TestValidateRequestPathIgnoresLeadingSlash pins whole-path validation
// (REQ-150).
func TestValidateRequestPathIgnoresLeadingSlash(t *testing.T) {
	if err := ValidateRequestPath("/ehr/abc/composition"); err != nil {
		t.Fatalf("ValidateRequestPath on a well-formed path = %v, want nil", err)
	}
	if err := ValidateRequestPath("/ehr/a/../../definition/query/evil/composition"); err == nil {
		t.Fatal("ValidateRequestPath on a traversal path = nil, want an error")
	}
	if err := ValidateRequestPath("/ehr/abc/composition/"); err == nil {
		t.Fatal("ValidateRequestPath on a trailing empty segment = nil, want an error")
	}
	// An empty path is not the service root: it aliases to the bare service
	// base URL, and only "/" is carved out.
	if err := ValidateRequestPath(""); err == nil {
		t.Fatal(`ValidateRequestPath("") = nil, want an error (only "/" is exempt)`)
	}
}

// TestSegmentCountNormalisesLeadingSlash pins that the arity comparison is
// leading-slash agnostic on each side independently, so a Route written
// without a leading slash cannot produce a spurious off-by-one refusal.
func TestSegmentCountNormalisesLeadingSlash(t *testing.T) {
	cases := []struct{ path, route string }{
		{"/ehr/abc", "/ehr/{ehr_id}"},
		{"/ehr/abc", "ehr/{ehr_id}"},
		{"ehr/abc", "/ehr/{ehr_id}"},
		{"/", "/"},
	}
	for _, tc := range cases {
		if p, r := segmentCount(tc.path), segmentCount(tc.route); p != r {
			t.Errorf("segmentCount(%q) = %d, segmentCount(%q) = %d; want equal", tc.path, p, tc.route, r)
		}
	}
}

// TestDoRejectsTraversalPath pins that a traversal segment fails closed at
// Do with nothing on the wire (REQ-150, PROBE-091).
func TestDoRejectsTraversalPath(t *testing.T) {
	var hits atomic.Int32 // Store/Load cross handler and test goroutines; -race aborts on a plain int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(srv.Close)
	c, err := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Do(t.Context(), &Request{
		Method: http.MethodGet,
		Path:   "/ehr/a/../../definition/query/evil/composition",
	})
	if !errors.Is(err, ErrInvalidPathSegment) || !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Do with a traversal path = %v, want both ErrInvalidPathSegment and ErrInvalidConfig", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("issued %d requests, want 0", n)
	}
}

// TestDoRejectsSmuggledSeparator pins the route-arity half: a parameter
// carrying `/` yields individually legal segments, so only the count
// betrays it (REQ-150, PROBE-091).
func TestDoRejectsSmuggledSeparator(t *testing.T) {
	// ehr_id="foo/bar" interpolated into the path: every segment is legal,
	// only the count betrays it — 5 path segments vs 4 in the route template.
	var hits atomic.Int32 // Store/Load cross handler and test goroutines; -race aborts on a plain int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(srv.Close)
	c, err := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Do(t.Context(), &Request{
		Method: http.MethodGet,
		Path:   "/ehr/foo/bar/contribution/x",
		Route:  "/ehr/{ehr_id}/contribution/{contribution_uid}",
	})
	if !errors.Is(err, ErrInvalidPathSegment) || !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Do with a smuggled separator = %v, want both ErrInvalidPathSegment and ErrInvalidConfig", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("issued %d requests, want 0", n)
	}
}

// TestDoAcceptsTheServiceRoot pins that the REQ-150 carve-out survives the
// enforcement point, not just the validator: the System API still reaches
// the wire.
func TestDoAcceptsTheServiceRoot(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	c, err := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(t.Context(), &Request{
		Method: http.MethodOptions,
		Path:   "/",
		Route:  "/",
	}); err != nil {
		t.Fatalf("Do on the service root = %v, want nil", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("issued %d requests, want 1", n)
	}
}

// TestDoSkipsArityCheckWithoutRoute pins that a Route-less raw request is
// not subjected to the arity half — Route is optional on direct
// transport.Do use, and the leaf obligation is enforced by the tripwire in
// openehr/client, not here (REQ-150).
func TestDoSkipsArityCheckWithoutRoute(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	c, err := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(t.Context(), &Request{
		Method: http.MethodGet,
		Path:   "/ehr/foo/bar/contribution/x",
	}); err != nil {
		t.Fatalf("Do without Route = %v, want nil (arity check is Route-gated)", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("issued %d requests, want 1", n)
	}
}

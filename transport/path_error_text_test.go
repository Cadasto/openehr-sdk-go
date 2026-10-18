package transport

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// patientID stands for a caller-supplied identifier interpolated into a
// path. No path refusal may print it.
const patientID = "PATIENT-NHS-9434765919"

// assertPathRefusal checks a REQ-150 refusal: both sentinels in the chain,
// the rule named, and no caller identifier in the text (REQ-093).
func assertPathRefusal(t *testing.T, err error, wantText string, identifiers ...string) {
	t.Helper()
	if !errors.Is(err, ErrInvalidPathSegment) || !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("error = %v, want both ErrInvalidPathSegment and ErrInvalidConfig in the chain", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, wantText) {
		t.Errorf("error = %q, want it to contain %q", msg, wantText)
	}
	for _, id := range identifiers {
		if strings.Contains(msg, id) {
			t.Errorf("error = %q, want no caller identifier %q", msg, id)
		}
	}
}

// TestREQ150_SegmentRefusalNamesTheRuleNotTheValue pins the text of a
// ValidatePathSegment refusal: the rule the segment breaks and the byte
// index where, never the segment, which is usually a caller-supplied
// identifier (REQ-150, REQ-093).
func TestREQ150_SegmentRefusalNamesTheRuleNotTheValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		segment  string
		wantText string
	}{
		{name: "empty", segment: "", wantText: "invalid path segment: empty segment"},
		{name: "dot", segment: ".", wantText: `invalid path segment: dot segment ("." or "..")`},
		{name: "dot dot", segment: "..", wantText: `invalid path segment: dot segment ("." or "..")`},
		{name: "separator", segment: "PATIENT/NHS-9434765919", wantText: `invalid path segment: separator "/" at index 7`},
		{name: "backslash", segment: `PATIENT\NHS-9434765919`, wantText: "invalid path segment: backslash at index 7"},
		{name: "NUL", segment: patientID + "\x00", wantText: "invalid path segment: control byte at index 22"},
		{name: "DEL", segment: patientID + "\x7f", wantText: "invalid path segment: control byte at index 22"},
		// "Ä" is two bytes, so the index counts bytes, not characters.
		{name: "index counts bytes", segment: "Ä" + patientID + "\x01", wantText: "invalid path segment: control byte at index 24"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertPathRefusal(t, ValidatePathSegment(tc.segment), tc.wantText, "9434765919", "NHS")
		})
	}
}

// TestREQ150_RequestPathRefusalNamesTheSegmentNotTheValue pins the text of
// a ValidateRequestPath refusal: which segment, counted from 1, and the rule
// it breaks, never the path or the segment (REQ-150, REQ-093).
func TestREQ150_RequestPathRefusalNamesTheSegmentNotTheValue(t *testing.T) {
	t.Parallel()
	const ehrID = "7d44b88c-4199-4bad-97dc-d78268e01398"
	tests := []struct {
		name     string
		path     string
		wantText string
	}{
		{name: "empty path", path: "", wantText: "invalid path segment: segment 1: empty segment"},
		{name: "trailing slash", path: "/ehr/" + ehrID + "/composition/", wantText: "invalid path segment: segment 4: empty segment"},
		{name: "traversal", path: "/ehr/" + ehrID + "/../definition", wantText: `invalid path segment: segment 3: dot segment ("." or "..")`},
		{name: "control byte", path: "/ehr/" + ehrID + "\x00", wantText: "invalid path segment: segment 2: control byte at index 36"},
		{name: "relative path", path: "ehr/" + ehrID + `\x`, wantText: "invalid path segment: segment 2: backslash at index 36"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertPathRefusal(t, ValidateRequestPath(tc.path), tc.wantText, ehrID)
		})
	}
}

// TestREQ150_DoPathRefusalIsValueFree pins the two refusals Do itself
// returns before any request is sent: a segment that breaks a rule, and a
// path whose segment count differs from its route template's, which names
// the counts and the template, never the path (REQ-150, REQ-093).
func TestREQ150_DoPathRefusalIsValueFree(t *testing.T) {
	t.Parallel()
	const ehrID = "7d44b88c-4199-4bad-97dc-d78268e01398"
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	c, err := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tests := []struct {
		name     string
		req      Request
		wantText string
	}{
		{
			name:     "smuggled separator",
			req:      Request{Method: http.MethodGet, Route: "/ehr/{ehr_id}/composition/{version_uid}", Path: "/ehr/" + ehrID + "/composition/abc/def"},
			wantText: `invalid path segment: path has 5 segments, route template "/ehr/{ehr_id}/composition/{version_uid}" has 4`,
		},
		{
			name:     "control byte",
			req:      Request{Method: http.MethodGet, Route: "/ehr/{ehr_id}", Path: "/ehr/" + patientID + "\x00"},
			wantText: "invalid path segment: segment 2: control byte at index 22",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Do(t.Context(), &tc.req)
			assertPathRefusal(t, err, tc.wantText, ehrID, "abc/def", "9434765919")
		})
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("issued %d requests, want 0", n)
	}
}

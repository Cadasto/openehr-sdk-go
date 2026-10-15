package definition_test

// body_read_error_test.go — REQ-172 § Metadata still arrives, held against
// every Definition leaf that returns a *transport.Metadata: a 2xx whose body
// is cut mid-read still hands back the response headers beside the
// *transport.BodyReadError.
//
// The status and headers are the only part of such a response that arrived
// intact. For the PUT and upload leaves they are the proof that the server
// stored the query or template, and the Location names what it stored, so a
// caller that loses them cannot tell whether to send the request again.

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// cutBodyClient returns a client aimed at a server that answers every request
// with status, an ETag, the given Location (when non-empty) and a body cut
// mid-read: the Content-Length announces more bytes than are written, and the
// handler then drops the connection.
func cutBodyClient(t *testing.T, status int, etag, location string) *transport.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		const partial = `{"template_id":"`
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"`+etag+`"`)
		if location != "" {
			w.Header().Set("Location", location)
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(partial)+100))
		w.WriteHeader(status)
		_, _ = io.WriteString(w, partial)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(srv.Close)
	return newClient(t, srv)
}

// TestDefinitionCutBodyKeepsMetadata pins REQ-172 § Metadata still arrives at
// each Definition leaf's transport-error return. Each case serves a 2xx with
// an ETag and a cut body and requires three things: no decoded value, the
// response metadata carrying the ETag (and the Location on the leaves that
// read it), and a *transport.BodyReadError recoverable through the leaf's
// return.
//
// PutStoredQuery and PutStoredQueryVersion reach the same return in the
// shared putStoredQuery helper; both rows are kept so the site stays pinned
// whichever entry point a later refactor moves it under.
func TestDefinitionCutBodyKeepsMetadata(t *testing.T) { // REQ-172
	const (
		etag        = "org.example::q::1.0.0"
		queryLoc    = "/openehr/v1/definition/query/org.example::q/1.0.0"
		templateLoc = "/openehr/v1/definition/template/adl1.4/body_weight.v1"
		aqlText     = "SELECT c FROM COMPOSITION c"
	)
	cases := []struct {
		name     string
		status   int
		location string
		call     func(t *testing.T, c *transport.Client) (decoded bool, meta *transport.Metadata, err error)
	}{
		{
			name:     "PutStoredQuery",
			status:   http.StatusOK,
			location: queryLoc,
			call: func(t *testing.T, c *transport.Client) (bool, *transport.Metadata, error) {
				v, meta, err := definition.PutStoredQuery(t.Context(), c, "org.example::q", aqlText)
				return v != nil, meta, err
			},
		},
		{
			name:     "PutStoredQueryVersion",
			status:   http.StatusOK,
			location: queryLoc,
			call: func(t *testing.T, c *transport.Client) (bool, *transport.Metadata, error) {
				v, meta, err := definition.PutStoredQueryVersion(t.Context(), c, "org.example::q", "1.0.0", aqlText)
				return v != nil, meta, err
			},
		},
		{
			name:   "GetStoredQuery",
			status: http.StatusOK,
			call: func(t *testing.T, c *transport.Client) (bool, *transport.Metadata, error) {
				v, meta, err := definition.GetStoredQuery(t.Context(), c, "org.example::q", "1.0.0")
				return v != nil, meta, err
			},
		},
		{
			name:   "ListStoredQueries",
			status: http.StatusOK,
			call: func(t *testing.T, c *transport.Client) (bool, *transport.Metadata, error) {
				v, meta, err := definition.ListStoredQueries(t.Context(), c, "org.example")
				return v != nil, meta, err
			},
		},
		{
			name:   "DeleteStoredQuery",
			status: http.StatusOK,
			call: func(t *testing.T, c *transport.Client) (bool, *transport.Metadata, error) {
				meta, err := definition.DeleteStoredQuery(t.Context(), c, "org.example::q", "1.0.0")
				return false, meta, err
			},
		},
		{
			name:     "UploadTemplate",
			status:   http.StatusCreated,
			location: templateLoc,
			call: func(t *testing.T, c *transport.Client) (bool, *transport.Metadata, error) {
				v, meta, err := definition.UploadTemplate(t.Context(), c, definition.FormatADL14,
					strings.NewReader(`<template xmlns="http://schemas.openehr.org/v1"/>`))
				return v != nil, meta, err
			},
		},
		{
			name:   "GetTemplate",
			status: http.StatusOK,
			call: func(t *testing.T, c *transport.Client) (bool, *transport.Metadata, error) {
				v, meta, err := definition.GetTemplate(t.Context(), c, "body_weight.v1", definition.FormatADL14)
				return v != nil, meta, err
			},
		},
		{
			name:   "ListTemplates",
			status: http.StatusOK,
			call: func(t *testing.T, c *transport.Client) (bool, *transport.Metadata, error) {
				v, meta, err := definition.ListTemplates(t.Context(), c, definition.FormatADL14)
				return v != nil, meta, err
			},
		},
		{
			name:   "DeleteTemplate",
			status: http.StatusOK,
			call: func(t *testing.T, c *transport.Client) (bool, *transport.Metadata, error) {
				meta, err := definition.DeleteTemplate(t.Context(), c, "body_weight.v1", definition.FormatADL14)
				return false, meta, err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			decoded, meta, err := tc.call(t, cutBodyClient(t, tc.status, etag, tc.location))
			if err == nil {
				t.Fatalf("%s against a %d whose body is cut succeeded; the premise of this test is gone", tc.name, tc.status)
			}
			if decoded {
				t.Errorf("%s returned a value beside a body-read failure; a partial body is not the representation", tc.name)
			}
			if meta == nil {
				t.Fatalf("%s returned nil *transport.Metadata; REQ-172 requires the response headers come back beside the error", tc.name)
			}
			if meta.ETag != etag {
				t.Errorf("%s: Metadata.ETag = %q, want %q", tc.name, meta.ETag, etag)
			}
			if meta.Location != tc.location {
				t.Errorf("%s: Metadata.Location = %q, want %q", tc.name, meta.Location, tc.location)
			}
			bre, ok := errors.AsType[*transport.BodyReadError](err)
			if !ok {
				t.Fatalf("%s: errors.AsType[*transport.BodyReadError] did not match %T (%v)", tc.name, err, err)
			}
			if bre.StatusCode != tc.status {
				t.Errorf("%s: BodyReadError.StatusCode = %d, want %d", tc.name, bre.StatusCode, tc.status)
			}
		})
	}
}

package query_test

// body_read_error_test.go — REQ-172 § Metadata still arrives, held against the
// result-set funnel (doResultSet, behind Execute, ExecuteString and
// RunStored): a 200 whose body is cut mid-read still hands back the response
// headers beside the *transport.BodyReadError, through mapQueryError.

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestResultSetCutBodyKeepsMetadata requires each query entry point, against a
// 200 whose body is cut, to return no result set, the metadata carrying the
// ETag the server sent, and an error from which the *transport.BodyReadError
// is recovered. The three rows reach the same return in doResultSet through
// different requests.
func TestResultSetCutBodyKeepsMetadata(t *testing.T) { // REQ-172
	const (
		etag    = "rs-1"
		aqlText = "SELECT c FROM COMPOSITION c"
	)
	cases := []struct {
		name string
		call func(t *testing.T, c *transport.Client) (*aql.ResultSet, *transport.Metadata, error)
	}{
		{
			name: "Execute",
			call: func(t *testing.T, c *transport.Client) (*aql.ResultSet, *transport.Metadata, error) {
				return query.Execute(t.Context(), c, aql.NewQuery(aqlText))
			},
		},
		{
			name: "ExecuteString",
			call: func(t *testing.T, c *transport.Client) (*aql.ResultSet, *transport.Metadata, error) {
				return query.ExecuteString(t.Context(), c, aqlText, nil)
			},
		},
		{
			name: "RunStored",
			call: func(t *testing.T, c *transport.Client) (*aql.ResultSet, *transport.Metadata, error) {
				return query.RunStored(t.Context(), c, "org.example::q", nil)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("ETag", `"`+etag+`"`)
				// Announce more bytes than are written, then drop the connection.
				w.Header().Set("Content-Length", "100")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, `{"q":"SELECT`)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				panic(http.ErrAbortHandler)
			}))
			t.Cleanup(srv.Close)

			rs, meta, err := tc.call(t, newClient(t, srv))
			if err == nil {
				t.Fatalf("%s against a 200 whose body is cut succeeded; the premise of this test is gone", tc.name)
			}
			if rs != nil {
				t.Errorf("%s returned %+v beside a body-read failure; a partial body is not the representation", tc.name, rs)
			}
			if meta == nil {
				t.Fatalf("%s returned nil *transport.Metadata; REQ-172 requires the response headers come back beside the error", tc.name)
			}
			if meta.ETag != etag {
				t.Errorf("%s: Metadata.ETag = %q, want %q", tc.name, meta.ETag, etag)
			}
			bre, ok := errors.AsType[*transport.BodyReadError](err)
			if !ok {
				t.Fatalf("%s: errors.AsType[*transport.BodyReadError] did not match %T (%v)", tc.name, err, err)
			}
			if bre.StatusCode != http.StatusOK {
				t.Errorf("%s: BodyReadError.StatusCode = %d, want %d", tc.name, bre.StatusCode, http.StatusOK)
			}
		})
	}
}

package transport_test

// query_encoding_test.go — REQ-095. Pins the Request.Query encoding contract:
// the pinned OAS declares query parameters `style: form`, which OpenAPI
// defines by RFC 6570 form-style query expansion, so a space reaches the wire
// as %20 and never as `+`. A `+` is a space only to an HTML-form decoder; a
// server that percent-decodes by RFC 3986 reads it as a literal plus and
// receives a different query. With %20 both decoders recover the original
// text, and a literal plus in a value stays %2B.

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/transport"
)

func TestREQ095_RequestQuerySpaceIsPercentEncoded(t *testing.T) {
	const aqlText = "SELECT e/ehr_id/value FROM EHR e WHERE e/ehr_id/value = 'a+b'"
	tests := []struct {
		name    string
		query   url.Values
		wantRaw string
	}{
		{
			name:    "value with spaces and a literal plus",
			query:   url.Values{"q": {aqlText}},
			wantRaw: "q=SELECT%20e%2Fehr_id%2Fvalue%20FROM%20EHR%20e%20WHERE%20e%2Fehr_id%2Fvalue%20%3D%20%27a%2Bb%27",
		},
		{
			name:    "key with a space",
			query:   url.Values{"key with space": {"value"}},
			wantRaw: "key%20with%20space=value",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var raw string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw = r.URL.RawQuery
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			c, err := transport.New(newDecodeCatalog(t, srv), transport.WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Do(t.Context(), &transport.Request{
				Method: http.MethodGet,
				Path:   "/query/aql",
				Route:  "/query/aql",
				Query:  tc.query,
			}); err != nil {
				t.Fatalf("Do: %v", err)
			}

			if raw != tc.wantRaw {
				t.Errorf("wire query = %q, want %q", raw, tc.wantRaw)
			}
			if strings.Contains(raw, "+") {
				t.Errorf("wire query = %q carries a '+', which an RFC 3986 decoder reads as a literal plus", raw)
			}

			// HTML-form decoding: '+' would be a space, %20 is a space.
			form, err := url.ParseQuery(raw)
			if err != nil {
				t.Fatalf("url.ParseQuery(%q): %v", raw, err)
			}
			if !maps.EqualFunc(form, tc.query, slices.Equal[[]string]) {
				t.Errorf("url.ParseQuery(%q) = %q, want %q", raw, form, tc.query)
			}

			// RFC 3986 percent-decoding: '+' stays '+', %20 is a space.
			strict := url.Values{}
			for pair := range strings.SplitSeq(raw, "&") {
				k, v, _ := strings.Cut(pair, "=")
				key, err := url.PathUnescape(k)
				if err != nil {
					t.Fatalf("url.PathUnescape(%q): %v", k, err)
				}
				val, err := url.PathUnescape(v)
				if err != nil {
					t.Fatalf("url.PathUnescape(%q): %v", v, err)
				}
				strict[key] = append(strict[key], val)
			}
			if !maps.EqualFunc(strict, tc.query, slices.Equal[[]string]) {
				t.Errorf("RFC 3986 decode of %q = %q, want %q", raw, strict, tc.query)
			}
		})
	}
}

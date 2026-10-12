package transport_test

// query_encoding_test.go — REQ-095. Pins the Request.Query encoding contract:
// the pinned OAS declares query parameters `style: form`, which OpenAPI
// defines by RFC 6570 form-style query expansion, so a space reaches the wire
// as %20 and never as `+`. A `+` is a space only to an HTML-form decoder; a
// server that percent-decodes by RFC 3986 reads it as a literal plus and
// receives a different query. With %20 both decoders recover the original
// text, and a literal plus in a value stays %2B.

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
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
			strict, err := rfc3986Query(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !maps.EqualFunc(strict, tc.query, slices.Equal[[]string]) {
				t.Errorf("RFC 3986 decode of %q = %q, want %q", raw, strict, tc.query)
			}
		})
	}
}

// TestREQ095_BaseURLQueryKeptAsConfigured pins how Request.Query meets a query
// already in the service base URL. The base pairs reach the wire byte for
// byte and in their configured order, so each kind of server reads them the
// same with or without a request query. The request's pairs follow, encoded
// as above, and a request key replaces the base pair of the same name.
func TestREQ095_BaseURLQueryKeptAsConfigured(t *testing.T) {
	// A '+' is a space to an HTML-form decoder and a plus to an RFC 3986
	// decoder; %20 is a space to both. Either may be what the operator meant.
	const baseQuery = "key=a+b&sp=x%20y"
	tests := []struct {
		name    string
		query   url.Values
		wantRaw string
	}{
		{
			name:    "no request query",
			wantRaw: "key=a+b&sp=x%20y",
		},
		{
			name:    "request adds a key",
			query:   url.Values{"q": {"x y"}},
			wantRaw: "key=a+b&sp=x%20y&q=x%20y",
		},
		{
			name:    "request replaces a base key",
			query:   url.Values{"key": {"new v"}},
			wantRaw: "sp=x%20y&key=new%20v",
		},
		{
			name:    "request key with no values removes the base key",
			query:   url.Values{"key": {}},
			wantRaw: "sp=x%20y",
		},
	}
	baseForm, err := url.ParseQuery(baseQuery)
	if err != nil {
		t.Fatalf("url.ParseQuery(%q): %v", baseQuery, err)
	}
	baseStrict, err := rfc3986Query(baseQuery)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var raw string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw = r.URL.RawQuery
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
				Issuer: "https://test.example.com",
				Services: map[string]discovery.ServiceEntry{
					discovery.ServiceIDOpenEHRRest: {
						BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1?" + baseQuery),
						SpecVersion: discovery.SpecVersionPin,
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
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
				t.Errorf("base query %q, Request.Query %q: wire query = %q, want %q", baseQuery, tc.query, raw, tc.wantRaw)
			}

			// An HTML-form decoder reads each base pair the request does not
			// replace as it reads the configured base query.
			form, err := url.ParseQuery(raw)
			if err != nil {
				t.Fatalf("url.ParseQuery(%q): %v", raw, err)
			}
			if want := withRequestQuery(baseForm, tc.query); !maps.EqualFunc(form, want, slices.Equal[[]string]) {
				t.Errorf("url.ParseQuery(%q) = %q, want %q", raw, form, want)
			}

			// So does an RFC 3986 decoder.
			strict, err := rfc3986Query(raw)
			if err != nil {
				t.Fatal(err)
			}
			if want := withRequestQuery(baseStrict, tc.query); !maps.EqualFunc(strict, want, slices.Equal[[]string]) {
				t.Errorf("RFC 3986 decode of %q = %q, want %q", raw, strict, want)
			}
		})
	}
}

// rfc3986Query decodes a raw query the way an RFC 3986 server does: '&'
// separates the pairs, and percent-escapes are decoded while '+' stays a plus.
func rfc3986Query(raw string) (url.Values, error) {
	out := url.Values{}
	for pair := range strings.SplitSeq(raw, "&") {
		k, v, _ := strings.Cut(pair, "=")
		key, err := url.PathUnescape(k)
		if err != nil {
			return nil, fmt.Errorf("url.PathUnescape(%q): %w", k, err)
		}
		val, err := url.PathUnescape(v)
		if err != nil {
			return nil, fmt.Errorf("url.PathUnescape(%q): %w", v, err)
		}
		out[key] = append(out[key], val)
	}
	return out, nil
}

// withRequestQuery returns the decoded base query as a request query should
// leave it: each key of query replaces the base key of the same name, and a
// key with no values removes it.
func withRequestQuery(base, query url.Values) url.Values {
	out := maps.Clone(base)
	for k, vv := range query {
		if len(vv) == 0 {
			delete(out, k)
			continue
		}
		out[k] = vv
	}
	return out
}

package transport

// query_encoding_test.go — REQ-095. Pins the Request.Query encoding contract:
// a space in a query value reaches the wire as %20, never as "+".
//
// ITS-REST declares the AQL q parameter as style: form, and RFC 6570 form-style
// query expansion percent-encodes a space as %20. RFC 3986 gives "+" no special
// meaning in a query, so a server that decodes the query string by RFC 3986 (as
// FerroEHR >= 4.3.3 does) reads a literal "+" where the caller sent a space and
// refuses the query. url.Values.Encode writes a space as "+" (the HTML form
// convention), so joinTarget rewrites those to %20; a literal plus is already
// rendered %2B by Encode and is left untouched.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRequestQueryEncodesSpaceAsPercent20(t *testing.T) {
	const q = "SELECT e/ehr_id/value FROM EHR e WHERE e/ehr_id/value = 'a+b'"
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw = r.URL.RawQuery
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	if _, err := c.Do(t.Context(), &Request{
		Method: "GET",
		Path:   "/query/aql",
		Query:  url.Values{"q": {q}},
	}); err != nil {
		t.Fatal(err)
	}
	// A literal "+" on the wire would be read as a space by an RFC 3986 server;
	// the only "+" the caller sent is "a+b", which MUST ride as %2B.
	if strings.Contains(raw, "+") {
		t.Errorf("wire query %q carries a literal %q; a space MUST be %%20", raw, "+")
	}
	// Byte-for-byte, so the regression cannot pass by dropping a character.
	if want := "q=SELECT%20e%2Fehr_id%2Fvalue%20FROM%20EHR%20e%20WHERE%20e%2Fehr_id%2Fvalue%20%3D%20%27a%2Bb%27"; raw != want {
		t.Errorf("wire query = %q, want %q", raw, want)
	}
	// A server decoding by either convention MUST recover the caller's value.
	if got, err := url.QueryUnescape(raw); err != nil || got != "q="+q {
		t.Errorf("form decode = %q (err %v), want %q", got, err, "q="+q)
	}
	if got, err := url.PathUnescape(raw); err != nil || got != "q="+q {
		t.Errorf("RFC 3986 decode = %q (err %v), want %q", got, err, "q="+q)
	}
}

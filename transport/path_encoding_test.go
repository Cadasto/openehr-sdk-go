package transport

// path_encoding_test.go — REQ-095. Pins the Request.Path encoding contract:
// Request.Path is a DECODED path (url.URL.Path semantics) and the transport is
// the single canonical path encoder. A path parameter carrying a percent-
// encodable character reaches the wire encoded exactly once and the server
// decodes it back to the original character. A caller that pre-escapes would
// double-encode — hence leaf clients interpolate the raw id.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestPathIsDecodedAndSingleEncoded(t *testing.T) {
	var decoded, escaped string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoded = r.URL.Path
		escaped = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	// Path carries a raw, decoded space — as a leaf client now interpolates it.
	if _, err := c.Do(t.Context(), &Request{
		Method: "GET",
		Path:   "/definition/template/adl1.4/Referral Request.v1",
	}); err != nil {
		t.Fatal(err)
	}
	if want := "/openehr/v1/definition/template/adl1.4/Referral Request.v1"; decoded != want {
		t.Errorf("server-decoded path = %q, want %q", decoded, want)
	}
	if want := "/openehr/v1/definition/template/adl1.4/Referral%20Request.v1"; escaped != want {
		t.Errorf("wire path = %q, want %q (single-encoded, not %%2520)", escaped, want)
	}
}

package query_test

// query_encoding_test.go — REQ-095. An ad-hoc AQL query sent with WithGET must
// reach the wire with every space as %20, as the pinned OAS `q` parameter
// (`style: form`, RFC 6570 form-style expansion) requires. A `+` is a space
// only to an HTML-form decoder; a server that percent-decodes by RFC 3986
// reads it as a literal plus and refuses the query.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
)

func TestREQ095_ExecuteStringGETQuerySpaceIsPercentEncoded(t *testing.T) {
	const aqlText = "SELECT e/ehr_id/value FROM EHR e WHERE e/ehr_id/value = 'a+b'"
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw = r.URL.RawQuery
		w.WriteHeader(http.StatusNotImplemented) // assert request shape only
	}))
	defer srv.Close()
	_, _, _ = query.ExecuteString(t.Context(), newClient(t, srv), aqlText, nil, query.WithGET())

	q, ok := strings.CutPrefix(raw, "q=")
	if !ok || strings.Contains(q, "&") {
		t.Fatalf("wire query = %q, want only the q parameter", raw)
	}
	if strings.Contains(q, "+") {
		t.Errorf("wire q = %q carries a '+', which an RFC 3986 decoder reads as a literal plus", q)
	}
	got, err := url.PathUnescape(q)
	if err != nil {
		t.Fatalf("url.PathUnescape(%q): %v", q, err)
	}
	if got != aqlText {
		t.Errorf("RFC 3986 decode of q = %q, want %q", got, aqlText)
	}
}

package transport_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestREQ093_BuildRequestErrorMasksBaseURLPassword checks that when the
// request URL built from a hand-built service base URL cannot be parsed, the
// error names the base URL's host but not the password in its userinfo.
func TestREQ093_BuildRequestErrorMasksBaseURLPassword(t *testing.T) { // REQ-093
	t.Parallel()
	const password = "s3cret"
	// The port is not a number, so the text url.URL.String makes of the
	// request URL does not parse and the request cannot be built.
	base := &url.URL{
		Scheme: "https",
		User:   url.UserPassword("svc", password),
		Host:   "x.example:bad",
		Path:   "/openehr/v1",
	}
	if _, err := url.Parse(base.String()); err == nil {
		t.Fatalf("url.Parse(%q) error = nil, want an error: the case would not reach the refusal", base)
	}
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://idp.example",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {BaseURL: base, SpecVersion: discovery.SpecVersionPin},
		},
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	c, err := transport.New(cat, transport.WithHTTPClient(http.DefaultClient))
	if err != nil {
		t.Fatalf("transport.New: %v", err)
	}

	_, err = c.Do(t.Context(), &transport.Request{Method: http.MethodGet, Path: "/definition/template/adl1.4", Route: "/definition/template/adl1.4"})

	if err == nil {
		t.Fatal("Do error = nil, want the request-build error")
	}
	msg := err.Error()
	if strings.Contains(msg, password) {
		t.Errorf("Do error = %q, want no password", msg)
	}
	if !strings.Contains(msg, "x.example:bad") {
		t.Errorf("Do error = %q, want it to name the host %q", msg, "x.example:bad")
	}
}

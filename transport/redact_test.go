package transport_test

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestREQ093_BuildRequestErrorMasksBaseURLPassword checks that when the
// request URL built from a hand-built service base URL cannot be parsed, the
// error names the request by its route template, or by "(unrouted)" when it
// has none, in place of the URL. It carries neither the password in the base
// URL's userinfo nor the expanded request path with its caller identifier,
// and it is still the *url.Error that url.Parse returned.
func TestREQ093_BuildRequestErrorMasksBaseURLPassword(t *testing.T) { // REQ-093
	t.Parallel()
	const (
		password = "s3cret"
		ehrID    = "7d44b88c-4199-4bad-97dc-d78268e01398"
		basePath = "/openehr/v1"
	)
	// The port is not a number, so the text url.URL.String makes of the
	// request URL does not parse and the request cannot be built.
	base := &url.URL{
		Scheme: "https",
		User:   url.UserPassword("svc", password),
		Host:   "x.example:bad",
		Path:   basePath,
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

	tests := []struct {
		name      string
		route     string
		wantRoute string
	}{
		{name: "routed", route: "/ehr/{ehr_id}", wantRoute: "/ehr/{ehr_id}"},
		{name: "unrouted", route: "", wantRoute: "(unrouted)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &transport.Request{Method: http.MethodGet, Path: "/ehr/" + ehrID, Route: tc.route}

			_, err := c.Do(t.Context(), req)

			if err == nil {
				t.Fatal("Do error = nil, want the request-build error")
			}
			msg := err.Error()
			if strings.Contains(msg, password) {
				t.Errorf("Do error = %q, want no password", msg)
			}
			if strings.Contains(msg, ehrID) {
				t.Errorf("Do error = %q, want no caller identifier %q", msg, ehrID)
			}
			if strings.Contains(msg, req.Path) || strings.Contains(msg, basePath) {
				t.Errorf("Do error = %q, want no part of the expanded request path %q or the resolved path %q", msg, req.Path, basePath+req.Path)
			}
			if !strings.Contains(msg, tc.wantRoute) {
				t.Errorf("Do error = %q, want it to name the route %q", msg, tc.wantRoute)
			}
			ue, ok := errors.AsType[*url.Error](err)
			if !ok || ue == nil {
				t.Fatalf("Do error = %v, want a *url.Error in its chain", err)
			}
			if ue.Op != "parse" || ue.URL != tc.wantRoute || ue.Err == nil {
				t.Errorf("*url.Error = {Op: %q, URL: %q, Err: %v}, want {Op: %q, URL: %q} with its cause", ue.Op, ue.URL, ue.Err, "parse", tc.wantRoute)
			}
		})
	}
}

package transport_test

// span_url_test.go — REQ-090 § Spans: the URL on a request's span is
// sanitised. The tracer is the recording one span_status_test.go defines.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestSpanURLCarriesNoUserinfo pins the sentence "The URL on the span MUST be
// sanitised: it carries no token and none of the request URL's userinfo (a
// user name or password)". The service base URL carries a user name and a
// password, so the URL the request goes to carries them too. The span's
// http.url names neither. It is the request URL without them, so it still
// names the host and the path, which is also the control that shows the
// recorder sees the attribute at all.
func TestSpanURLCarriesNoUserinfo(t *testing.T) { // REQ-090
	const (
		user     = "alice"
		password = "s3cret"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	base := discovery.MustParseURL(srv.URL + "/openehr/v1")
	base.User = url.UserPassword(user, password)
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {BaseURL: base, SpecVersion: discovery.SpecVersionPin},
		},
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	tp := newRecordingTracerProvider()
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()), transport.WithTracerProvider(tp))
	if err != nil {
		t.Fatalf("transport.New: %v", err)
	}

	req := &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"}
	if _, err := c.Do(t.Context(), req); err != nil {
		t.Fatalf("Do(GET %s) error = %v, want nil; the premise of this test is gone", req.Path, err)
	}

	span := tp.tracer.only(t)
	got, ok := span.attr("http.url")
	if !ok {
		t.Fatalf("span %q carries no http.url attribute", span.name)
	}
	for _, secret := range []string{user, password} {
		if strings.Contains(got.String(), secret) {
			t.Errorf("span %q http.url = %q, want no %q from the base URL's userinfo", span.name, got.String(), secret)
		}
	}
	if want := srv.URL + "/openehr/v1" + req.Path; got.Type() != attribute.STRING || got.AsString() != want {
		t.Errorf("span %q http.url = %q (%s), want %q", span.name, got.String(), got.Type(), want)
	}
}

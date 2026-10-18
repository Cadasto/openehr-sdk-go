package discovery_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// plaintextDocumentWarning is the warning the resolver logs when it fetched
// the SMART configuration over http, as slog's text handler writes its
// message. It is the whole message, so counting it also pins that the
// message carries nothing more, such as an internal requirement id.
const plaintextDocumentWarning = `msg="discovery: SMART configuration fetched over plaintext http"`

// plaintextDocumentMention is a part of that message, to check that no such
// warning is logged whatever its full text.
const plaintextDocumentMention = "fetched over plaintext http"

// loggingResolver builds a Resolver on client that allows insecure URLs and
// logs to the returned buffer.
func loggingResolver(t *testing.T, client *http.Client) (*discovery.Resolver, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	res, err := discovery.NewResolver(nil, discovery.WithHTTPClient(client), discovery.WithAllowInsecure(),
		discovery.WithLogger(slog.New(slog.NewTextHandler(&buf, nil))))
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return res, &buf
}

// TestREQ092_WarnsWhenDocumentFetchedOverHTTP pins REQ-092's warning on the
// SMART configuration itself: a document a resolver with WithAllowInsecure
// fetched over http logs one warning naming the base URL and the document
// URL with their passwords masked, an https document logs none, and an
// https request redirected to http logs the URL the document came from.
func TestREQ092_WarnsWhenDocumentFetchedOverHTTP(t *testing.T) { // REQ-092
	doc := serve(func(string) string { return smartDocument("", "") })

	t.Run("http document", func(t *testing.T) {
		p := startPlatform(t, true, doc, notFound)
		res, logs := loggingResolver(t, p.srv.Client())
		base := withUserinfo(p.baseURL())
		if _, err := res.Resolve(t.Context(), base); err != nil {
			t.Fatalf("Resolve(%q): %v", base, err)
		}
		got := logs.String()
		if n := strings.Count(got, plaintextDocumentWarning); n != 1 {
			t.Fatalf("log = %q holds %d %q warnings, want 1", got, n, plaintextDocumentWarning)
		}
		masked := strings.Replace(p.baseURL(), "://", "://svc:xxxxx@", 1)
		if !strings.Contains(got, "base_url="+masked) || !strings.Contains(got, "url="+masked+discovery.WellKnownPath) {
			t.Errorf("log = %q, want base_url=%s and url=%s", got, masked, masked+discovery.WellKnownPath)
		}
		if strings.Contains(got, secret) {
			t.Errorf("log = %q carries the URL password %q", got, secret)
		}
		if strings.Contains(got, "REQ-") {
			t.Errorf("log = %q names an internal requirement id", got)
		}
	})

	t.Run("https document", func(t *testing.T) {
		p := startPlatform(t, false, doc, notFound)
		res, logs := loggingResolver(t, p.srv.Client())
		if _, err := res.Resolve(t.Context(), p.baseURL()); err != nil {
			t.Fatalf("Resolve(%q): %v", p.baseURL(), err)
		}
		if got := logs.String(); strings.Contains(got, plaintextDocumentMention) {
			t.Errorf("log = %q, want no %q warning for an https document", got, plaintextDocumentMention)
		}
	})

	t.Run("https redirected to http", func(t *testing.T) {
		plain, _ := countingServer(t, func(string) string { return smartDocument("", "") })
		tlsSrv := httptest.NewTLSServer(http.RedirectHandler(plain.URL+"/smart", http.StatusFound))
		t.Cleanup(tlsSrv.Close)
		res, logs := loggingResolver(t, tlsSrv.Client())
		base := tlsSrv.URL + platformPath
		if _, err := res.Resolve(t.Context(), base); err != nil {
			t.Fatalf("Resolve(%q): %v", base, err)
		}
		got := logs.String()
		if n := strings.Count(got, plaintextDocumentWarning); n != 1 {
			t.Fatalf("log = %q holds %d %q warnings, want 1", got, n, plaintextDocumentWarning)
		}
		if !strings.Contains(got, "url="+plain.URL+"/smart") {
			t.Errorf("log = %q, want url=%s, where the document came from", got, plain.URL+"/smart")
		}
	})
}

// TestREQ092_WarnsWhenDocumentRenewedOverHTTP pins the same warning on a
// 304 Not Modified: each fetch over http, the conditional one included,
// logs it once.
func TestREQ092_WarnsWhenDocumentRenewedOverHTTP(t *testing.T) { // REQ-092
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = io.WriteString(w, smartDocument("", ""))
	}))
	t.Cleanup(srv.Close)
	res, logs := loggingResolver(t, srv.Client())
	base := srv.URL + platformPath
	if _, err := res.Resolve(t.Context(), base); err != nil {
		t.Fatalf("Resolve(%q): %v", base, err)
	}
	if _, err := res.Refresh(t.Context(), base); err != nil {
		t.Fatalf("Refresh(%q): %v", base, err)
	}
	if got := logs.String(); strings.Count(got, plaintextDocumentWarning) != 2 {
		t.Errorf("log = %q, want one %q warning for the 200 and one for the 304", got, plaintextDocumentWarning)
	}
}

// TestREQ092_WarnsWhenTransportLeavesNoRequest pins the warning for an
// injected RoundTripper that answers without setting Response.Request: the
// URL the resolver asked for stands in for the last request's, and the
// resolver neither panics nor stays silent.
func TestREQ092_WarnsWhenTransportLeavesNoRequest(t *testing.T) { // REQ-092, REQ-025
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(smartDocument("", ""))),
		}, nil
	})}
	res, logs := loggingResolver(t, client)
	const base = "http://platform.example" + platformPath
	if v := catchPanic(func() {
		if _, err := res.Resolve(t.Context(), base); err != nil {
			t.Errorf("Resolve(%q): %v", base, err)
		}
	}); v != nil {
		t.Fatalf("Resolve(%q) through a transport that sets no Response.Request panicked: %v", base, v)
	}
	got := logs.String()
	if strings.Count(got, plaintextDocumentWarning) != 1 || !strings.Contains(got, "url="+base+discovery.WellKnownPath) {
		t.Errorf("log = %q, want one %q warning naming url=%s", got, plaintextDocumentWarning, base+discovery.WellKnownPath)
	}
}

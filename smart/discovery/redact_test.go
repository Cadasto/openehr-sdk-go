package discovery_test

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// secret is the password the URLs in these tests carry; userinfo puts it,
// after the user name svc, in a URL's authority.
const (
	secret   = "s3cret"
	userinfo = "svc:" + secret + "@"
)

// withUserinfo inserts userinfo into the authority of rawURL, which starts
// with a scheme and "://".
func withUserinfo(rawURL string) string {
	return strings.Replace(rawURL, "://", "://"+userinfo, 1)
}

// authorityEnds are the characters that end a URL's authority. A password
// that holds one unescaped makes url.Parse refuse the URL and quote the
// part of the password before it.
var authorityEnds = []struct{ name, char string }{
	{name: "slash", char: "/"},
	{name: "question mark", char: "?"},
	{name: "hash", char: "#"},
}

// cutUserinfo is userinfo with end, one of authorityEnds, in the password
// after secret.
func cutUserinfo(end string) string {
	return "svc:" + secret + end + "tail@"
}

// maskCase is one way the package refuses a URL that carries a password.
// call makes the refused call and returns the base URL it was for, which
// the DiscoveryError's Issuer field must still hold as given, and the
// error.
type maskCase struct {
	name   string
	call   func(t *testing.T) (baseURL string, err error)
	reason discovery.DiscoveryErrorReason
	keep   string // text the error must still carry, so it stays useful
}

// runMaskCases checks each case's error: a *DiscoveryError with the
// case's Reason and the raw base URL in Issuer, whose text names keep and
// carries no password.
func runMaskCases(t *testing.T, cases []maskCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL, err := tc.call(t)
			if err == nil {
				t.Fatalf("error = nil, want a DiscoveryError with Reason %q", tc.reason)
			}
			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr == nil {
				t.Fatalf("error = %v (%T), want a *DiscoveryError", err, err)
			}
			if derr.Reason != tc.reason {
				t.Errorf("Reason = %q, want %q (error %v)", derr.Reason, tc.reason, err)
			}
			if derr.Issuer != baseURL {
				t.Errorf("Issuer = %q, want the base URL as given, %q", derr.Issuer, baseURL)
			}
			text := err.Error()
			if strings.Contains(text, secret) {
				t.Errorf("error text %q carries the URL password %q", text, secret)
			}
			if !strings.Contains(text, tc.keep) {
				t.Errorf("error text %q does not name %q", text, tc.keep)
			}
		})
	}
}

// staticError builds a catalog from cfg and returns the base URL the
// catalog would have and the error.
func staticError(cfg discovery.StaticConfig) (string, error) {
	_, err := discovery.NewStaticCatalog(cfg)
	return cmp.Or(cfg.BaseURL, cfg.Issuer), err
}

// resolveError resolves on a stub platform that serves smart as its SMART
// configuration and openID as the OpenID configuration of the issuer under
// idpPath; a nil document is not served. baseURL picks the URL passed to
// Resolve from the platform's own base URL.
func resolveError(t *testing.T, smart, openID func(origin string) string, baseURL func(platform string) string, opts ...discovery.Option) (string, error) {
	t.Helper()
	smartDoc, openIDDoc := documentFunc(notFound), documentFunc(notFound)
	if smart != nil {
		smartDoc = serve(smart)
	}
	if openID != nil {
		openIDDoc = serve(openID)
	}
	p := startPlatform(t, false, smartDoc, openIDDoc)
	base := p.baseURL()
	if baseURL != nil {
		base = baseURL(base)
	}
	_, err := p.resolver(t, opts...).Resolve(t.Context(), base)
	return base, err
}

// fixed returns a document, or a base URL, that does not depend on the
// stub platform's address.
func fixed(s string) func(string) string { return func(string) string { return s } }

// refreshCachedError refreshes a catalog that the cache already holds,
// built by cached from the base URL, against a platform that answers the
// conditional request with 304 Not Modified, so the resolver checks the
// cached catalog as it stands.
func refreshCachedError(t *testing.T, cached func(baseURL string) *discovery.ServiceCatalog) (string, error) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `"v1"` {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.WriteHeader(http.StatusNotModified)
	}))
	t.Cleanup(srv.Close)
	base := srv.URL + platformPath
	cache := discovery.NewMemoryCache()
	cat := cached(base)
	cat.ETag = `"v1"`
	if err := cache.Put(t.Context(), base, cat); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	res, err := discovery.NewResolver(cache, discovery.WithHTTPClient(srv.Client()), discovery.WithoutOpenIDConfigurationCheck())
	if err != nil {
		t.Fatal(err)
	}
	_, err = res.Refresh(t.Context(), base)
	return base, err
}

// TestREQ070_ErrorTextMasksBaseURLPassword pins REQ-070: DiscoveryError
// prints the Platform base URL as base_url= with any password masked, for
// a hand-built catalog and a resolved one, while its Issuer field keeps the
// URL as given.
func TestREQ070_ErrorTextMasksBaseURLPassword(t *testing.T) { // REQ-070
	cases := []maskCase{
		{
			name: "DiscoveryError with a password in Issuer",
			call: func(*testing.T) (string, error) {
				const base = "https://" + userinfo + "x.example/"
				return base, &discovery.DiscoveryError{Issuer: base, Reason: discovery.ReasonFetchFailed}
			},
			reason: discovery.ReasonFetchFailed,
			keep:   "x.example",
		},
		{
			name: "DiscoveryError with an unparseable Issuer",
			call: func(*testing.T) (string, error) {
				const base = "https://" + userinfo + "x.example/%zz"
				return base, &discovery.DiscoveryError{Issuer: base, Reason: discovery.ReasonMalformedURL}
			},
			reason: discovery.ReasonMalformedURL,
			keep:   "x.example",
		},
		{
			name: "NewStaticCatalog, issuer as base URL, nil service BaseURL",
			call: func(*testing.T) (string, error) {
				return staticError(discovery.StaticConfig{
					Issuer:   "https://" + userinfo + "x.example/",
					Services: map[string]discovery.ServiceEntry{"s": {}},
				})
			},
			reason: discovery.ReasonMalformedURL,
			keep:   "x.example",
		},
		{
			name: "NewStaticCatalog, base URL, relative service BaseURL",
			call: func(*testing.T) (string, error) {
				return staticError(discovery.StaticConfig{
					BaseURL:  "https://" + userinfo + "x.example/",
					Issuer:   "https://idp.example/",
					Services: map[string]discovery.ServiceEntry{"s": {BaseURL: discovery.MustParseURL("/rest")}},
				})
			},
			reason: discovery.ReasonMalformedURL,
			keep:   "x.example",
		},
		{
			name: "Resolve, platform answers 404",
			call: func(t *testing.T) (string, error) {
				return resolveError(t, nil, nil, withUserinfo)
			},
			reason: discovery.ReasonFetchFailed,
			keep:   "127.0.0.1",
		},
	}
	for _, end := range authorityEnds {
		cases = append(cases, maskCase{
			name: "DiscoveryError with a " + end.name + " in the password",
			call: func(*testing.T) (string, error) {
				base := "https://" + cutUserinfo(end.char) + "x.example/"
				return base, &discovery.DiscoveryError{Issuer: base, Reason: discovery.ReasonMalformedURL}
			},
			reason: discovery.ReasonMalformedURL,
			keep:   "x.example",
		})
	}
	runMaskCases(t, cases)
}

// TestREQ072_ErrorTextMasksURLPassword pins REQ-072: a malformed base URL,
// service baseUrl or auth endpoint is refused with a DiscoveryError whose
// text masks the URL's password.
func TestREQ072_ErrorTextMasksURLPassword(t *testing.T) { // REQ-072
	const apiBaseURL = "https://api.example.com/openehr/v1"
	cases := []maskCase{
		{
			name: "base URL url.Parse refuses",
			call: func(t *testing.T) (string, error) {
				return resolveError(t, nil, nil, fixed("https://"+userinfo+"x.example/%zz"))
			},
			reason: discovery.ReasonMalformedURL,
			keep:   "x.example",
		},
		{
			name: "base URL without a scheme",
			call: func(t *testing.T) (string, error) {
				return resolveError(t, nil, nil, fixed("//"+userinfo+"x.example/"))
			},
			reason: discovery.ReasonMalformedURL,
			keep:   "x.example",
		},
		{
			// url.Parse accepts the base URL, but not the document URL built
			// from it, which escapes the zone's é as %C3%A9.
			name: "base URL whose document URL cannot be requested",
			call: func(t *testing.T) (string, error) {
				return resolveError(t, nil, nil, fixed("https://"+userinfo+"[fe80::1%25é]/"))
			},
			reason: discovery.ReasonFetchFailed,
			keep:   "fe80::1",
		},
		{
			name: "service baseUrl url.Parse refuses",
			call: func(t *testing.T) (string, error) {
				return resolveError(t, fixed(documentWith("https://"+userinfo+"api.example/%zz", nil)), nil, nil)
			},
			reason: discovery.ReasonMalformedURL,
			keep:   "api.example",
		},
		{
			name: "service baseUrl without a host",
			call: func(t *testing.T) (string, error) {
				return resolveError(t, fixed(documentWith("https://"+userinfo+":8443/v1", nil)), nil, nil)
			},
			reason: discovery.ReasonMalformedURL,
			keep:   "svc:xxxxx@:8443",
		},
		{
			name: "auth endpoint url.Parse refuses",
			call: func(t *testing.T) (string, error) {
				doc := documentWith(apiBaseURL, map[string]string{"token_endpoint": "https://" + userinfo + "auth.example/%zz"})
				return resolveError(t, fixed(doc), nil, nil)
			},
			reason: discovery.ReasonMalformedURL,
			keep:   "auth.example",
		},
		{
			name: "auth endpoint without a host",
			call: func(t *testing.T) (string, error) {
				doc := documentWith(apiBaseURL, map[string]string{"token_endpoint": "https://" + userinfo + ":8443/token"})
				return resolveError(t, fixed(doc), nil, nil)
			},
			reason: discovery.ReasonMalformedURL,
			keep:   "svc:xxxxx@:8443",
		},
	}
	for _, end := range authorityEnds {
		cases = append(cases, maskCase{
			name: "base URL with a " + end.name + " in the password",
			call: func(t *testing.T) (string, error) {
				return resolveError(t, nil, nil, fixed("https://"+cutUserinfo(end.char)+"x.example/"))
			},
			reason: discovery.ReasonMalformedURL,
			keep:   "x.example",
		})
	}
	runMaskCases(t, cases)
}

// TestREQ073_ErrorTextMasksURLPassword pins REQ-073: a plaintext base URL,
// a refused issuer, an issuer the OpenID configuration does not confirm
// and a plaintext URL in a catalog renewed by 304 Not Modified are refused
// with a DiscoveryError whose text masks each URL's password.
func TestREQ073_ErrorTextMasksURLPassword(t *testing.T) { // REQ-073
	issuerCase := func(name, issuer string, reason discovery.DiscoveryErrorReason, keep string) maskCase {
		return maskCase{
			name: "issuer " + name,
			call: func(t *testing.T) (string, error) {
				return resolveError(t, fixed(smartDocument(issuer, "")), nil, nil)
			},
			reason: reason,
			keep:   keep,
		}
	}
	cases := []maskCase{
		{
			name: "plaintext base URL",
			call: func(t *testing.T) (string, error) {
				return resolveError(t, nil, nil, fixed("http://"+userinfo+"x.example/"))
			},
			reason: discovery.ReasonInsecureURL,
			keep:   "x.example",
		},
		issuerCase("url.Parse refuses", "https://"+userinfo+"idp.example/%zz", discovery.ReasonMalformedURL, "idp.example"),
		issuerCase("not https", "ftp://"+userinfo+"idp.example/", discovery.ReasonMalformedURL, "idp.example"),
		issuerCase("without a host", "https://"+userinfo+":8443/idp", discovery.ReasonMalformedURL, "svc:xxxxx@:8443"),
		issuerCase("with a query", "https://"+userinfo+"idp.example/?tenant=1", discovery.ReasonMalformedURL, "idp.example"),
		issuerCase("with a fragment", "https://"+userinfo+"idp.example/#top", discovery.ReasonMalformedURL, "idp.example"),
		issuerCase("plaintext", "http://"+userinfo+"idp.example/", discovery.ReasonInsecureURL, "idp.example"),
		{
			// url.Parse accepts the issuer, but not the OpenID configuration
			// URL built from it, which escapes the zone's é as %C3%A9.
			name: "issuer whose OpenID configuration URL cannot be requested",
			call: func(t *testing.T) (string, error) {
				return resolveError(t, fixed(smartDocument("https://"+userinfo+"[fe80::1%25é]"+idpPath, "")), nil, nil)
			},
			reason: discovery.ReasonFetchFailed,
			keep:   "fe80::1",
		},
		{
			name: "OpenID configuration names another issuer",
			call: func(t *testing.T) (string, error) {
				smart := func(o string) string { return smartDocument(withUserinfo(o)+idpPath, "") }
				openID := func(o string) string { return openIDDocument(withUserinfo(o)+"/other", "") }
				return resolveError(t, smart, openID, nil)
			},
			reason: discovery.ReasonIssuerMismatch,
			keep:   "127.0.0.1",
		},
		{
			name: "OpenID configuration names another jwks_uri",
			call: func(t *testing.T) (string, error) {
				smart := func(o string) string { return smartDocument(o+idpPath, "https://"+userinfo+"keys.example/jwks") }
				openID := func(o string) string { return openIDDocument(o+idpPath, "https://"+userinfo+"other.example/jwks") }
				return resolveError(t, smart, openID, nil)
			},
			reason: discovery.ReasonIssuerMismatch,
			keep:   "keys.example",
		},
		{
			name: "304 renewal, plaintext issuer",
			call: func(t *testing.T) (string, error) {
				return refreshCachedError(t, func(base string) *discovery.ServiceCatalog {
					return &discovery.ServiceCatalog{BaseURL: base, Issuer: "http://" + userinfo + "idp.example/"}
				})
			},
			reason: discovery.ReasonInsecureURL,
			keep:   "idp.example",
		},
		{
			name: "304 renewal, plaintext auth endpoint",
			call: func(t *testing.T) (string, error) {
				return refreshCachedError(t, func(base string) *discovery.ServiceCatalog {
					return &discovery.ServiceCatalog{
						BaseURL: base,
						Issuer:  base,
						Auth:    discovery.AuthEndpoints{TokenEndpoint: discovery.MustParseURL("http://" + userinfo + "auth.example/token")},
					}
				})
			},
			reason: discovery.ReasonInsecureURL,
			keep:   "auth.example",
		},
	}
	for _, end := range authorityEnds {
		cases = append(cases, issuerCase("with a "+end.name+" in the password", "https://"+cutUserinfo(end.char)+"idp.example/", discovery.ReasonMalformedURL, "idp.example"))
	}
	runMaskCases(t, cases)
}

// failingCache is a MemoryCache whose Put and Invalidate fail.
type failingCache struct{ *discovery.MemoryCache }

var errCacheDown = errors.New("cache down")

func (failingCache) Put(context.Context, string, *discovery.ServiceCatalog) error {
	return errCacheDown
}

func (failingCache) Invalidate(context.Context, string) error { return errCacheDown }

// logOf resolves a base URL that carries a password on a stub platform
// serving smart (nil serves none) and returns what the resolver logged.
func logOf(t *testing.T, smart func(origin string) string, cache discovery.Cache, opts ...discovery.Option) string {
	t.Helper()
	smartDoc := documentFunc(notFound)
	if smart != nil {
		smartDoc = serve(smart)
	}
	p := startPlatform(t, false, smartDoc, notFound)
	var buf bytes.Buffer
	opts = append([]discovery.Option{
		discovery.WithHTTPClient(p.srv.Client()),
		discovery.WithLogger(slog.New(slog.NewTextHandler(&buf, nil))),
	}, opts...)
	res, err := discovery.NewResolver(cache, opts...)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = res.Resolve(t.Context(), withUserinfo(p.baseURL()))
	return buf.String()
}

// TestREQ071_CacheFailureLogMasksBaseURLPassword pins REQ-071's logged
// cache failure: the warning names the base URL with its password masked.
func TestREQ071_CacheFailureLogMasksBaseURLPassword(t *testing.T) { // REQ-071
	tests := []struct {
		name    string
		smart   func(origin string) string
		wantMsg string
	}{
		{
			name:    "Put fails after a resolution",
			smart:   fixed(documentWith("https://api.example.com/openehr/v1", nil)),
			wantMsg: "cache put failed",
		},
		{
			name:    "Invalidate fails after a failed fetch",
			wantMsg: "cache invalidate failed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := logOf(t, tc.smart, failingCache{discovery.NewMemoryCache()})
			if !strings.Contains(got, tc.wantMsg) {
				t.Fatalf("log = %q, want a %q warning", got, tc.wantMsg)
			}
			if strings.Contains(got, secret) {
				t.Errorf("log = %q carries the URL password %q", got, secret)
			}
			if !strings.Contains(got, "base_url=https://svc:xxxxx@127.0.0.1") {
				t.Errorf("log = %q, want the base URL with its password masked", got)
			}
		})
	}
}

// TestREQ073_PlaintextWarningMasksBaseURLPassword pins the REQ-073 warning
// on a plaintext service baseUrl: it names the base URL with its password
// masked.
func TestREQ073_PlaintextWarningMasksBaseURLPassword(t *testing.T) { // REQ-073
	got := logOf(t, fixed(documentWith("http://api.example.com/openehr/v1", nil)), nil, discovery.WithAllowInsecure())
	if !strings.Contains(got, "plaintext URL in catalog") {
		t.Fatalf("log = %q, want a plaintext URL warning", got)
	}
	if strings.Contains(got, secret) {
		t.Errorf("log = %q carries the URL password %q", got, secret)
	}
	if !strings.Contains(got, "base_url=https://svc:xxxxx@127.0.0.1") {
		t.Errorf("log = %q, want the base URL with its password masked", got)
	}
}

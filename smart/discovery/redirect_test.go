package discovery_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// countingServer is a plaintext server that counts its requests and answers
// every one with the body built from the server's own origin.
func countingServer(t *testing.T, body func(origin string) string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body("http://"+r.Host))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestResolveRefusesRedirectDowngrade pins REQ-073: while fetching the SMART
// configuration or the issuer's OpenID configuration, a resolver without
// WithAllowInsecure does not follow a redirect to a URL that is not https;
// it reports ReasonInsecureURL and never contacts the redirect target. With
// WithAllowInsecure the redirect is followed.
func TestResolveRefusesRedirectDowngrade(t *testing.T) { // REQ-073
	tests := []struct {
		name string
		// redirected is the path the https server answers with a 302.
		redirected string
		// target is the Location of the 302, built from the plaintext server's
		// origin; the plaintext server serves body there.
		target     func(plain string) string
		body       func(https string) string
		opts       []discovery.Option
		wantReason discovery.DiscoveryErrorReason // empty means success
		wantHits   int32                          // requests the plaintext server receives
	}{
		{
			name:       "SMART configuration to http",
			redirected: smartDocPath,
			target:     func(plain string) string { return plain + "/smart" },
			body:       func(string) string { return smartDocument("", "") },
			wantReason: discovery.ReasonInsecureURL,
		},
		{
			name:       "SMART configuration to ftp",
			redirected: smartDocPath,
			target:     func(string) string { return "ftp://files.example.com/smart" },
			body:       func(string) string { return smartDocument("", "") },
			wantReason: discovery.ReasonInsecureURL,
		},
		{
			name:       "OpenID configuration to http",
			redirected: openIDDocPath,
			target:     func(plain string) string { return plain + "/oidc" },
			body:       func(https string) string { return openIDDocument(https+idpPath, "") },
			wantReason: discovery.ReasonInsecureURL,
		},
		{
			name:       "SMART configuration to http with WithAllowInsecure",
			redirected: smartDocPath,
			target:     func(plain string) string { return plain + "/smart" },
			body:       func(string) string { return smartDocument("", "") },
			opts:       []discovery.Option{discovery.WithAllowInsecure()},
			wantHits:   1,
		},
		{
			name:       "OpenID configuration to http with WithAllowInsecure",
			redirected: openIDDocPath,
			target:     func(plain string) string { return plain + "/oidc" },
			body:       func(https string) string { return openIDDocument(https+idpPath, "") },
			opts:       []discovery.Option{discovery.WithAllowInsecure()},
			wantHits:   1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var secureOrigin atomic.Value // the https server's origin, stored before any request
			plain, plainHits := countingServer(t, func(string) string { return tc.body(secureOrigin.Load().(string)) })
			secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				origin := "https://" + r.Host
				switch r.URL.Path {
				case tc.redirected:
					http.Redirect(w, r, tc.target(plain.URL), http.StatusFound)
				case smartDocPath:
					_, _ = io.WriteString(w, smartDocument(origin+idpPath, ""))
				case openIDDocPath:
					_, _ = io.WriteString(w, openIDDocument(origin+idpPath, ""))
				default:
					http.NotFound(w, r)
				}
			}))
			defer secure.Close()
			secureOrigin.Store(secure.URL)

			res, err := discovery.NewResolver(nil, append([]discovery.Option{discovery.WithHTTPClient(secure.Client())}, tc.opts...)...)
			if err != nil {
				t.Fatal(err)
			}
			baseURL := secure.URL + platformPath
			_, err = res.Resolve(t.Context(), baseURL)

			if got := plainHits.Load(); got != tc.wantHits {
				t.Errorf("plaintext server received %d request(s), want %d", got, tc.wantHits)
			}
			if tc.wantReason == "" {
				if err != nil {
					t.Fatalf("Resolve(%q) error = %v, want success", baseURL, err)
				}
				return
			}
			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr.Reason != tc.wantReason {
				t.Fatalf("Resolve(%q) error = %v, want a DiscoveryError with Reason %q", baseURL, err, tc.wantReason)
			}
		})
	}
}

// redirectingPlatform serves the SMART configuration and the OpenID
// configuration only after one https-to-https redirect each, from
// /moved/smart and /moved/oidc. A request to /loop redirects to itself; the
// server stops looping after 50 hops so a broken redirect limit cannot hang
// the test.
func redirectingPlatform(t *testing.T, smartRedirectsTo string) *httptest.Server {
	t.Helper()
	var loops atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := "https://" + r.Host
		switch r.URL.Path {
		case smartDocPath:
			http.Redirect(w, r, smartRedirectsTo, http.StatusFound)
		case openIDDocPath:
			http.Redirect(w, r, "/moved/oidc", http.StatusFound)
		case "/loop":
			if loops.Add(1) <= 50 {
				http.Redirect(w, r, "/loop", http.StatusFound)
				return
			}
			_, _ = io.WriteString(w, smartDocument(origin+idpPath, ""))
		case "/moved/smart":
			_, _ = io.WriteString(w, smartDocument(origin+idpPath, ""))
		case "/moved/oidc":
			_, _ = io.WriteString(w, openIDDocument(origin+idpPath, ""))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestResolveKeepsClientRedirectPolicy pins REQ-073: apart from refusing a
// downgrade, the injected client's own redirect policy applies. An
// https-to-https redirect is followed; a caller's CheckRedirect is consulted
// and its refusal stands; without one, the standard limit of 10 redirects
// holds. The caller's client is never modified.
func TestResolveKeepsClientRedirectPolicy(t *testing.T) { // REQ-073
	t.Run("https to https is followed", func(t *testing.T) {
		srv := redirectingPlatform(t, "/moved/smart")
		client := srv.Client()
		res, err := discovery.NewResolver(nil, discovery.WithHTTPClient(client))
		if err != nil {
			t.Fatal(err)
		}
		cat, err := res.Resolve(t.Context(), srv.URL+platformPath)
		if err != nil {
			t.Fatalf("Resolve error = %v, want both https redirects followed", err)
		}
		if cat.Issuer != srv.URL+idpPath {
			t.Errorf("catalog Issuer = %q, want %q", cat.Issuer, srv.URL+idpPath)
		}
		if client.CheckRedirect != nil {
			t.Error("the caller's client has a CheckRedirect after NewResolver and Resolve, want it left nil")
		}
	})

	t.Run("caller policy is consulted", func(t *testing.T) {
		srv := redirectingPlatform(t, "/moved/smart")
		client := srv.Client()
		var consulted atomic.Int32
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			consulted.Add(1)
			return nil
		}
		res, err := discovery.NewResolver(nil, discovery.WithHTTPClient(client))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := res.Resolve(t.Context(), srv.URL+platformPath); err != nil {
			t.Fatalf("Resolve error = %v, want success", err)
		}
		if got := consulted.Load(); got != 2 {
			t.Errorf("caller's CheckRedirect consulted %d times, want 2 (one per redirected document)", got)
		}
	})

	t.Run("caller policy refusal stands", func(t *testing.T) {
		srv := redirectingPlatform(t, "/moved/smart")
		client := srv.Client()
		errNoRedirects := errors.New("caller allows no redirects")
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return errNoRedirects }
		res, err := discovery.NewResolver(nil, discovery.WithHTTPClient(client))
		if err != nil {
			t.Fatal(err)
		}
		_, err = res.Resolve(t.Context(), srv.URL+platformPath)
		derr, ok := errors.AsType[*discovery.DiscoveryError](err)
		if !ok || derr.Reason != discovery.ReasonFetchFailed || !errors.Is(err, errNoRedirects) {
			t.Fatalf("Resolve error = %v, want a fetch_failed DiscoveryError wrapping the caller's refusal", err)
		}
	})

	t.Run("default limit of 10 redirects", func(t *testing.T) {
		srv := redirectingPlatform(t, "/loop")
		res, err := discovery.NewResolver(nil, discovery.WithHTTPClient(srv.Client()))
		if err != nil {
			t.Fatal(err)
		}
		_, err = res.Resolve(t.Context(), srv.URL+platformPath)
		derr, ok := errors.AsType[*discovery.DiscoveryError](err)
		if !ok || derr.Reason != discovery.ReasonFetchFailed {
			t.Fatalf("Resolve error = %v, want a fetch_failed DiscoveryError once the redirect limit is reached", err)
		}
	})
}

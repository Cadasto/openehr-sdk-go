package smart_test

import (
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// fullAuthEndpoints returns endpoints with every field set: each URL to its
// own https URL on idp.test, with user information, each list to S256 and
// one more value, and each flag to true. A field of a kind it cannot set
// fails the test, so a field added to discovery.AuthEndpoints later is not
// left out of the copy the source keeps.
func fullAuthEndpoints(t *testing.T) discovery.AuthEndpoints {
	t.Helper()
	var ep discovery.AuthEndpoints
	for field, f := range reflect.ValueOf(&ep).Elem().Fields() {
		name := field.Name
		switch f.Type() {
		case reflect.TypeFor[*url.URL]():
			f.Set(reflect.ValueOf(&url.URL{Scheme: "https", User: url.UserPassword("svc", "pw"), Host: "idp.test", Path: "/" + name}))
		case reflect.TypeFor[[]string]():
			f.Set(reflect.ValueOf([]string{"S256", name}))
		case reflect.TypeFor[bool]():
			f.SetBool(true)
		default:
			t.Fatalf("discovery.AuthEndpoints.%s has type %s, which this test cannot set: copy it in discovery.AuthEndpoints.Clone and set it here", name, f.Type())
		}
	}
	return ep
}

// editAuthEndpoints changes, in place, every URL and list ep points to: the
// edits a caller could make after handing ep to a source.
func editAuthEndpoints(ep discovery.AuthEndpoints) {
	for _, field := range reflect.ValueOf(ep).Fields() {
		switch f := field.Interface().(type) {
		case *url.URL:
			f.Host = "attacker.test"
			*f.User = *url.User("attacker")
		case []string:
			for j := range f {
				f[j] = "edited"
			}
		}
	}
}

// TestREQ026_SourceKeepsItsOwnCopyOfItsConfiguration pins REQ-026 (idiom.md §
// Concurrency) with REQ-061: the scopes, the trusted ID-token audiences and
// the endpoints a source is built with, through options, Config or a
// catalog, are copied, the URLs and lists the endpoints point to included.
// A change the caller makes to them afterwards neither changes the source
// nor races with it: dropping openid from the caller's slice does not drop
// the nonce, and editing an endpoint URL does not move the authorization
// request.
func TestREQ026_SourceKeepsItsOwnCopyOfItsConfiguration(t *testing.T) {
	const audience = "https://platform.example/openehr"
	builds := []struct {
		name string
		// build returns a source built from scopes, aud and ep, calling edit
		// once at the point the case tests.
		build func(t *testing.T, scopes, aud []string, ep discovery.AuthEndpoints, edit func()) (*smart.Source, error)
	}{
		{name: "options, edited after New", build: func(t *testing.T, scopes, aud []string, ep discovery.AuthEndpoints, edit func()) (*smart.Source, error) {
			defer edit()
			return newSource("client-id", ep, smart.WithHTTPClient(&http.Client{Transport: &scriptedServer{}}),
				smart.WithScopes(scopes...), smart.WithIDTokenTrustedAudiences(aud...), smart.WithAuthEndpoints(ep))
		}},
		{name: "options, edited before New", build: func(t *testing.T, scopes, aud []string, ep discovery.AuthEndpoints, edit func()) (*smart.Source, error) {
			opts := []smart.Option{
				smart.WithHTTPClient(&http.Client{Transport: &scriptedServer{}}),
				smart.WithScopes(scopes...), smart.WithIDTokenTrustedAudiences(aud...), smart.WithAuthEndpoints(ep),
			}
			edit()
			// New's own endpoints argument is a fresh value: the option is
			// what this case tests.
			return newSource("client-id", fullAuthEndpoints(t), opts...)
		}},
		{name: "Config", build: func(t *testing.T, scopes, aud []string, ep discovery.AuthEndpoints, edit func()) (*smart.Source, error) {
			defer edit()
			return smart.FromConfig(smart.Config{
				HTTPClient: &http.Client{Transport: &scriptedServer{}},
				ClientID:   "client-id", Audience: testAudience,
				Scopes: scopes, IDTokenTrustedAudiences: aud, Auth: ep,
			})
		}},
		{name: "NewFromCatalog", build: func(t *testing.T, scopes, aud []string, ep discovery.AuthEndpoints, edit func()) (*smart.Source, error) {
			defer edit()
			catalog := &discovery.ServiceCatalog{BaseURL: testAudience, Issuer: "https://idp.test", Auth: ep}
			return smart.NewFromCatalog(catalog, "client-id", smart.WithHTTPClient(&http.Client{Transport: &scriptedServer{}}),
				smart.WithScopes(scopes...), smart.WithIDTokenTrustedAudiences(aud...))
		}},
	}
	for _, b := range builds {
		t.Run(b.name, func(t *testing.T) {
			scopes := []string{"openid", "fhirUser"}
			aud := []string{audience}
			ep := fullAuthEndpoints(t)
			src, err := b.build(t, scopes, aud, ep, func() {
				scopes[0] = "profile"
				aud[0] = "https://attacker.example"
				editAuthEndpoints(ep)
			})
			if err != nil {
				t.Fatal(err)
			}

			got := src.Settings()
			if want := []string{"openid", "fhirUser"}; !slices.Equal(got.Scopes, want) {
				t.Errorf("the source's scopes = %q after the caller edited its slice, want %q", got.Scopes, want)
			}
			if want := []string{audience}; !slices.Equal(got.IDTokenTrustedAudiences, want) {
				t.Errorf("the source's trusted audiences = %q after the caller edited its slice, want %q", got.IDTokenTrustedAudiences, want)
			}
			if want := fullAuthEndpoints(t); !reflect.DeepEqual(got.Auth, want) {
				t.Errorf("the source's endpoints changed when the caller edited its own:\n got %+v\nwant %+v", got.Auth, want)
			}
			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatal(err)
			}
			if req.Nonce == "" {
				t.Errorf("BeginAuthorization() sent no nonce: the caller's edit dropped openid from the source's scopes")
			}
			raw, err := src.AuthorizeURL(req, "")
			if err != nil {
				t.Fatal(err)
			}
			if u, err := url.Parse(raw); err != nil || u.Host != "idp.test" || u.User.Username() != "svc" {
				t.Errorf("AuthorizeURL() = %q, want the endpoint svc@idp.test the source was built with", raw)
			}
		})
	}
}

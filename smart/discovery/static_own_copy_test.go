package discovery_test

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// fullAuthEndpoints returns endpoints with every field set: each URL to its
// own https URL on idp.test, with user information, each list to S256 and
// one more value, and each flag to true. A field of a kind it cannot set
// fails the test, so a field added to discovery.AuthEndpoints later is not
// left out of the copy AuthEndpoints.Clone makes, which NewStaticCatalog
// keeps.
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
			t.Fatalf("discovery.AuthEndpoints.%s has type %s, which this test cannot set: copy it in AuthEndpoints.Clone and set it here", name, f.Type())
		}
	}
	return ep
}

// editAuthEndpoints changes, in place, every URL, user information and list
// ep points to: the edits a holder of ep could make after building a
// catalog from it.
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

// TestREQ070_NewStaticCatalogKeepsItsOwnCopyOfAuth pins that a hand-built
// catalog shares none of its auth endpoints with the caller's StaticConfig,
// as it shares none of its Services: every URL, its user information and
// every list is copied. An edit the caller makes to its endpoints afterwards
// does not reach the catalog, and an edit to the catalog's endpoints does
// not reach the caller's.
func TestREQ070_NewStaticCatalogKeepsItsOwnCopyOfAuth(t *testing.T) { // REQ-070, REQ-026
	tests := []struct {
		name string
		// edit changes either the caller's endpoints or the catalog's.
		edit func(caller discovery.AuthEndpoints, cat *discovery.ServiceCatalog)
		// kept returns the endpoints the edit must leave as they were.
		kept func(caller discovery.AuthEndpoints, cat *discovery.ServiceCatalog) discovery.AuthEndpoints
	}{
		{
			name: "caller edits its endpoints",
			edit: func(caller discovery.AuthEndpoints, _ *discovery.ServiceCatalog) { editAuthEndpoints(caller) },
			kept: func(_ discovery.AuthEndpoints, cat *discovery.ServiceCatalog) discovery.AuthEndpoints {
				return cat.Auth
			},
		},
		{
			name: "catalog's endpoints edited",
			edit: func(_ discovery.AuthEndpoints, cat *discovery.ServiceCatalog) { editAuthEndpoints(cat.Auth) },
			kept: func(caller discovery.AuthEndpoints, _ *discovery.ServiceCatalog) discovery.AuthEndpoints {
				return caller
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := fullAuthEndpoints(t)
			cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{Issuer: "https://x.example/", Auth: caller})
			if err != nil {
				t.Fatalf("NewStaticCatalog: %v", err)
			}
			if want := fullAuthEndpoints(t); !reflect.DeepEqual(cat.Auth, want) {
				t.Fatalf("NewStaticCatalog(Auth) catalog Auth = %+v, want the endpoints as given %+v", cat.Auth, want)
			}
			tc.edit(caller, cat)
			if got, want := tc.kept(caller, cat), fullAuthEndpoints(t); !reflect.DeepEqual(got, want) {
				t.Errorf("after the edit, the other side's endpoints changed:\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

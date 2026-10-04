package discovery_test

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// serviceDocument is a SMART configuration whose only service is the
// openEHR REST entry, extended with the given members.
func serviceDocument(members map[string]any) string {
	entry := map[string]any{"baseUrl": "https://api.example.com/openehr/v1"}
	maps.Copy(entry, members)
	return mustJSON(map[string]any{
		"services": map[string]any{discovery.ServiceIDOpenEHRRest: entry},
	})
}

// TestResolveSurfacesServiceEntryMembers pins REQ-070: each ServiceEntry
// carries the canonical version, description, documentation and openapi
// members of its services entry verbatim, empty when absent, and the SDK
// neither fetches nor validates the documentation and openapi links.
func TestResolveSurfacesServiceEntryMembers(t *testing.T) { // REQ-070
	type members struct{ version, specVersion, description, documentation, openAPI string }
	tests := []struct {
		name string
		doc  func(origin string) map[string]any
		want func(origin string) members
	}{
		{
			// The links point at the stub server, so a fetch would be recorded.
			name: "every member, links on the Platform",
			doc: func(o string) map[string]any {
				return map[string]any{
					"version":       "1.0.2",
					"spec_version":  discovery.SpecVersionPin,
					"description":   "openEHR REST API",
					"documentation": o + "/docs",
					"openapi":       o + "/openapi.json",
				}
			},
			want: func(o string) members {
				return members{"1.0.2", discovery.SpecVersionPin, "openEHR REST API", o + "/docs", o + "/openapi.json"}
			},
		},
		{
			// Links an endpoint check would refuse: unparseable, and plaintext
			// under a resolver without WithAllowInsecure.
			name: "links that are not valid https URLs",
			doc: func(string) map[string]any {
				return map[string]any{
					"documentation": "::not a url",
					"openapi":       "http://plaintext.example/openapi.yaml",
				}
			},
			want: func(string) members {
				return members{documentation: "::not a url", openAPI: "http://plaintext.example/openapi.yaml"}
			},
		},
		{
			name: "members absent",
			doc:  func(string) map[string]any { return nil },
			want: func(string) members { return members{} },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := startPlatform(t, false, serve(func(o string) string {
				return serviceDocument(tc.doc(o))
			}), notFound)
			cat, err := p.resolver(t).Resolve(t.Context(), p.baseURL())
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v", p.baseURL(), err)
			}
			e, ok := cat.OpenEHRRest()
			if !ok {
				t.Fatalf("catalog has no %s entry", discovery.ServiceIDOpenEHRRest)
			}
			got := members{e.Version, e.SpecVersion, e.Description, e.Documentation, e.OpenAPI}
			if want := tc.want(p.srv.URL); got != want {
				t.Errorf("entry members = %+v, want %+v", got, want)
			}
			if paths, _ := p.requests(); !slices.Equal(paths, []string{smartDocPath}) {
				t.Errorf("requested paths %q, want only %q: documentation and openapi links are never fetched", paths, smartDocPath)
			}
		})
	}
}

// TestNewStaticCatalogKeepsServiceEntryMembers pins REQ-070: a hand-built
// catalog keeps the canonical service-entry members it was given.
func TestNewStaticCatalogKeepsServiceEntryMembers(t *testing.T) { // REQ-070
	in := discovery.ServiceEntry{
		BaseURL:       discovery.MustParseURL("https://ehrbase.example/rest/openehr/v1"),
		Version:       "1.0.2",
		Description:   "openEHR REST API",
		Documentation: "https://ehrbase.example/docs",
		OpenAPI:       "https://ehrbase.example/openapi.json",
	}
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer:   "https://ehrbase.example/",
		Services: map[string]discovery.ServiceEntry{discovery.ServiceIDOpenEHRRest: in},
	})
	if err != nil {
		t.Fatal(err)
	}
	e, _ := cat.OpenEHRRest()
	if e.Version != in.Version || e.Description != in.Description || e.Documentation != in.Documentation || e.OpenAPI != in.OpenAPI {
		t.Errorf("static entry Version, Description, Documentation, OpenAPI = %q, %q, %q, %q, want %q, %q, %q, %q",
			e.Version, e.Description, e.Documentation, e.OpenAPI, in.Version, in.Description, in.Documentation, in.OpenAPI)
	}
}

// TestResolveVersionGate pins REQ-072: without WithAcceptedSpecVersions only
// an advertised spec_version is compared and the canonical version member
// never is; with it, the compared value is spec_version when the entry
// advertises one and version otherwise, and an entry with neither fails.
// The list given to WithAcceptedSpecVersions replaces SpecVersionPin.
func TestResolveVersionGate(t *testing.T) { // REQ-072
	tests := []struct {
		name     string
		accepted []string // nil means WithAcceptedSpecVersions is not called
		members  map[string]any
		wantGot  string // SpecVersionGot of the refusal
		refused  bool
	}{
		{
			name:    "default: version alone is not compared",
			members: map[string]any{"version": "2.0.0"},
		},
		{
			name:    "default: neither member",
			members: nil,
		},
		{
			name:    "default: accepted spec_version, other version",
			members: map[string]any{"spec_version": discovery.SpecVersionPin, "version": "9.9.9"},
		},
		{
			name:    "default: refused spec_version, pinned version",
			members: map[string]any{"spec_version": "1.0.3", "version": discovery.SpecVersionPin},
			refused: true,
			wantGot: "1.0.3",
		},
		{
			name:     "strict: accepted version without spec_version",
			accepted: []string{"2.0.0"},
			members:  map[string]any{"version": "2.0.0"},
		},
		{
			name:     "strict: refused version without spec_version",
			accepted: []string{"2.0.0"},
			members:  map[string]any{"version": "3.0.0"},
			refused:  true,
			wantGot:  "3.0.0",
		},
		{
			name:     "strict: spec_version wins over an unaccepted version",
			accepted: []string{discovery.SpecVersionPin},
			members:  map[string]any{"spec_version": discovery.SpecVersionPin, "version": "3.0.0"},
		},
		{
			name:     "strict: spec_version wins over an accepted version",
			accepted: []string{"2.0.0"},
			members:  map[string]any{"spec_version": "1.0.3", "version": "2.0.0"},
			refused:  true,
			wantGot:  "1.0.3",
		},
		{
			// The list replaces the pinned target rather than adding to it.
			name:     "strict: a list without the pin refuses the pin",
			accepted: []string{"1.1.0"},
			members:  map[string]any{"spec_version": discovery.SpecVersionPin},
			refused:  true,
			wantGot:  discovery.SpecVersionPin,
		},
		{
			name:     "strict: a list naming the pin accepts it",
			accepted: []string{discovery.SpecVersionPin, "1.1.0"},
			members:  map[string]any{"spec_version": discovery.SpecVersionPin},
		},
		{
			name:     "strict: neither member",
			accepted: []string{discovery.SpecVersionPin},
			members:  nil,
			refused:  true,
			wantGot:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := serviceDocument(tc.members)
			p := startPlatform(t, false, serve(func(string) string { return body }), notFound)
			var opts []discovery.Option
			if tc.accepted != nil {
				opts = append(opts, discovery.WithAcceptedSpecVersions(tc.accepted...))
			}
			_, err := p.resolver(t, opts...).Resolve(t.Context(), p.baseURL())

			if !tc.refused {
				if err != nil {
					t.Fatalf("Resolve(%s) error = %v, want success", body, err)
				}
				return
			}
			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr.Reason != discovery.ReasonSpecVersionMismatch {
				t.Fatalf("Resolve(%s) error = %v, want a DiscoveryError with Reason %q", body, err, discovery.ReasonSpecVersionMismatch)
			}
			if derr.SpecVersionGot != tc.wantGot {
				t.Errorf("SpecVersionGot = %q, want %q", derr.SpecVersionGot, tc.wantGot)
			}
		})
	}
}

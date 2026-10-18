package discovery_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestREQ072_ListOptionsKeepTheirOwnCopy pins that WithRequiredServices and
// WithAcceptedSpecVersions keep the list they were given: a caller that
// changes its slice after building the option, and before NewResolver
// applies it, does not change what the resolver checks. The document
// advertises only the openEHR REST service with the pinned spec_version, so
// each option as given accepts it and as changed would refuse it.
func TestREQ072_ListOptionsKeepTheirOwnCopy(t *testing.T) { // REQ-072
	tests := []struct {
		name  string
		build func(list []string) discovery.Option
		given string
	}{
		{name: "WithRequiredServices", build: func(l []string) discovery.Option { return discovery.WithRequiredServices(l...) }, given: discovery.ServiceIDOpenEHRRest},
		{name: "WithAcceptedSpecVersions", build: func(l []string) discovery.Option { return discovery.WithAcceptedSpecVersions(l...) }, given: discovery.SpecVersionPin},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := startPlatform(t, false, serve(func(string) string {
				return serviceDocument(map[string]any{"spec_version": discovery.SpecVersionPin})
			}), notFound)
			list := []string{tc.given}
			opt := tc.build(list)
			list[0] = "changed.after.the.option"
			if _, err := p.resolver(t, opt).Resolve(t.Context(), p.baseURL()); err != nil {
				t.Errorf("Resolve with %s(%q), its slice changed afterwards: %v; want the list as given", tc.name, tc.given, err)
			}
		})
	}
}

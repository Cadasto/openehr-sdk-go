package discovery_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// staticServicesWithoutIDs returns a fresh services map whose entries leave
// ID empty, as a caller of NewStaticCatalog may.
func staticServicesWithoutIDs() map[string]discovery.ServiceEntry {
	return map[string]discovery.ServiceEntry{
		discovery.ServiceIDOpenEHRRest: {BaseURL: discovery.MustParseURL("https://x.example/rest/openehr/v1")},
		"org.example.extra":            {BaseURL: discovery.MustParseURL("https://x.example/extra")},
	}
}

// TestREQ070_NewStaticCatalogLeavesCallerConfigUntouched pins that building
// a hand-built catalog never writes into the caller's StaticConfig.Services:
// the catalog sets each entry's ID on its own copy, and the caller's map
// keeps its entries and their empty IDs.
func TestREQ070_NewStaticCatalogLeavesCallerConfigUntouched(t *testing.T) { // REQ-070
	services := staticServicesWithoutIDs()
	cfg := discovery.StaticConfig{Issuer: "https://x.example/", Services: services}

	cat, err := discovery.NewStaticCatalog(cfg)
	if err != nil {
		t.Fatalf("NewStaticCatalog(%d services) error = %v", len(services), err)
	}

	if len(services) != 2 {
		t.Errorf("after NewStaticCatalog, len(caller's Services) = %d, want 2", len(services))
	}
	for id, e := range services {
		if e.ID != "" {
			t.Errorf("after NewStaticCatalog, caller's Services[%q].ID = %q, want empty: the constructor wrote into the caller's map", id, e.ID)
		}
	}
	for id := range services {
		got, ok := cat.Service(id)
		if !ok {
			t.Errorf("catalog.Service(%q) missing", id)
			continue
		}
		if got.ID != id {
			t.Errorf("catalog.Service(%q).ID = %q, want %q", id, got.ID, id)
		}
	}
}

// TestREQ026_NewStaticCatalogConcurrentSharedConfig pins that several
// goroutines may build catalogs from one shared StaticConfig at once. Run
// under -race: a constructor that writes into the shared Services map is a
// data race.
func TestREQ026_NewStaticCatalogConcurrentSharedConfig(t *testing.T) { // REQ-026
	cfg := discovery.StaticConfig{Issuer: "https://x.example/", Services: staticServicesWithoutIDs()}

	const builders = 8
	var wg sync.WaitGroup
	for range builders {
		wg.Go(func() {
			cat, err := discovery.NewStaticCatalog(cfg)
			if err != nil {
				t.Errorf("NewStaticCatalog(shared config) error = %v", err)
				return
			}
			e, ok := cat.OpenEHRRest()
			if !ok || e.ID != discovery.ServiceIDOpenEHRRest {
				t.Errorf("catalog.OpenEHRRest() = ID %q, ok %t, want ID %q, ok true", e.ID, ok, discovery.ServiceIDOpenEHRRest)
			}
		})
	}
	wg.Wait()
}

// TestREQ070_NewStaticCatalogRefusesRelativeServiceURLRedacted pins the
// refusal of a service BaseURL that is not absolute, and that the refusal
// names the URL without the password its userinfo carries.
func TestREQ070_NewStaticCatalogRefusesRelativeServiceURLRedacted(t *testing.T) { // REQ-070
	const secret = "s3cret"
	cfg := discovery.StaticConfig{
		Issuer:   "https://x.example/",
		Services: map[string]discovery.ServiceEntry{"org.example.svc": {BaseURL: discovery.MustParseURL("//svc:" + secret + "@x.example/rest")}},
	}

	_, err := discovery.NewStaticCatalog(cfg)
	derr, ok := errors.AsType[*discovery.DiscoveryError](err)
	if !ok || derr.Reason != discovery.ReasonMalformedURL {
		t.Fatalf("NewStaticCatalog(relative service BaseURL) error = %v, want a *DiscoveryError with reason %q", err, discovery.ReasonMalformedURL)
	}
	if msg := err.Error(); strings.Contains(msg, secret) {
		t.Errorf("error text %q carries the URL's password", msg)
	} else if !strings.Contains(msg, "x.example/rest") {
		t.Errorf("error text %q does not name the refused URL", msg)
	}
}

// TestREQ070_NewStaticCatalogKeepsExpiresAt pins that a hand-built catalog
// takes its expiry from StaticConfig.ExpiresAt: none when it is zero, the
// caller's time when it is set.
func TestREQ070_NewStaticCatalogKeepsExpiresAt(t *testing.T) { // REQ-070
	expiry := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, want := range []time.Time{{}, expiry} {
		cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{Issuer: "https://x.example/", ExpiresAt: want})
		if err != nil {
			t.Fatalf("NewStaticCatalog(ExpiresAt %v) error = %v", want, err)
		}
		if !cat.ExpiresAt.Equal(want) {
			t.Errorf("catalog.ExpiresAt = %v, want %v", cat.ExpiresAt, want)
		}
	}
}

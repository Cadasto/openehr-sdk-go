package discovery

import (
	"fmt"
	"net/url"
	"time"
)

// StaticConfig is the input to NewStaticCatalog — a hand-built catalog
// for non-discovering openEHR backends.
type StaticConfig struct {
	Issuer   string
	Services map[string]ServiceEntry
	Auth     AuthEndpoints
	// ExpiresAt may be left zero (no TTL).
	ExpiresAt time.Time
	// ETag may be left empty.
	ETag string
}

// NewStaticCatalog builds a ServiceCatalog without a network round
// trip. Used for static EHRbase deployments, local development, and
// tests. Validates that every Services entry has a parseable BaseURL —
// callers SHOULD pre-parse URLs when possible.
//
// Hand-built catalogs are exempt from spec-version validation at
// construction (callers are presumed to know what they configured);
// transport-level mismatch surfaces as a wire error rather than a
// DiscoveryError.
func NewStaticCatalog(cfg StaticConfig) (*ServiceCatalog, error) {
	if cfg.Issuer == "" {
		return nil, &DiscoveryError{Reason: ReasonParseError, Inner: fmt.Errorf("StaticConfig.Issuer is required")}
	}
	for id, e := range cfg.Services {
		if e.BaseURL == nil {
			return nil, &DiscoveryError{Issuer: cfg.Issuer, Reason: ReasonMalformedURL, Inner: fmt.Errorf("service %q has nil BaseURL", id)}
		}
		if !e.BaseURL.IsAbs() {
			return nil, &DiscoveryError{Issuer: cfg.Issuer, Reason: ReasonMalformedURL, Inner: fmt.Errorf("service %q BaseURL %q is not absolute", id, e.BaseURL.String())}
		}
		e.ID = id
		cfg.Services[id] = e
	}
	return &ServiceCatalog{
		Issuer:     cfg.Issuer,
		Services:   cloneServices(cfg.Services),
		Auth:       cfg.Auth,
		ResolvedAt: time.Now(),
		ExpiresAt:  cfg.ExpiresAt,
		ETag:       cfg.ETag,
	}, nil
}

func cloneServices(in map[string]ServiceEntry) map[string]ServiceEntry {
	out := make(map[string]ServiceEntry, len(in))
	for k, v := range in {
		// Clone the URL so callers cannot mutate the catalog's copy.
		if v.BaseURL != nil {
			u := *v.BaseURL
			v.BaseURL = &u
		}
		caps := make([]string, len(v.Capabilities))
		copy(caps, v.Capabilities)
		v.Capabilities = caps
		out[k] = v
	}
	return out
}

// MustParseURL panics on invalid URLs; intended for static catalog
// construction in tests and example programs only.
func MustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(fmt.Sprintf("MustParseURL(%q): %v", raw, err))
	}
	return u
}

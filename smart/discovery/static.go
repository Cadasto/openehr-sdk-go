package discovery

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/urlredact"
)

// StaticConfig is the input to NewStaticCatalog, a hand-built catalog
// for openEHR backends that do not publish a discovery document.
type StaticConfig struct {
	// BaseURL is the Platform base URL. Leave it empty when it is the same
	// as Issuer; the catalog's BaseURL then takes the value of Issuer.
	BaseURL string
	// Issuer is the OpenID Connect issuer. Required. With no BaseURL,
	// auth/smart's NewFromCatalog sends the Issuer as the audience of an
	// authorization request when the caller sets none, so set BaseURL when
	// the issuer is a separate identity provider.
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
// tests. It checks that every Services entry has a BaseURL that is
// non-nil and absolute. It copies the Services map and sets each copied
// entry's ID to its key, leaving the caller's StaticConfig unchanged. It
// also copies Auth: every endpoint URL, with its user information, and
// every list, so a later change to the caller's endpoints does not reach
// the catalog, and a change to the catalog's does not reach the caller's.
//
// The catalog's BaseURL is StaticConfig.BaseURL, or StaticConfig.Issuer
// when BaseURL is empty.
//
// Hand-built catalogs are exempt from spec-version validation at
// construction (callers are presumed to know what they configured);
// transport-level mismatch surfaces as a wire error rather than a
// DiscoveryError.
func NewStaticCatalog(cfg StaticConfig) (*ServiceCatalog, error) {
	if cfg.Issuer == "" {
		return nil, &DiscoveryError{Reason: ReasonParseError, Inner: errors.New("StaticConfig.Issuer is required")}
	}
	baseURL := cmp.Or(cfg.BaseURL, cfg.Issuer)
	// cfg.Services is the caller's map, possibly shared between goroutines:
	// read it once into a copy and set IDs on the copy only.
	services := cloneServices(cfg.Services)
	for id, e := range services {
		if e.BaseURL == nil {
			return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonMalformedURL, Inner: fmt.Errorf("service %q has nil BaseURL", id)}
		}
		if !e.BaseURL.IsAbs() {
			return nil, &DiscoveryError{Issuer: baseURL, Reason: ReasonMalformedURL, Inner: fmt.Errorf("service %q BaseURL %q is not absolute", id, e.BaseURL.Redacted())}
		}
		e.ID = id
		services[id] = e
	}
	return &ServiceCatalog{
		BaseURL:    baseURL,
		Issuer:     cfg.Issuer,
		Services:   services,
		Auth:       cloneAuthEndpoints(cfg.Auth),
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
		v.Capabilities = slices.Clone(v.Capabilities)
		out[k] = v
	}
	return out
}

// cloneAuthEndpoints returns a copy of a that shares nothing a caller can
// change with a: each endpoint URL, with its user information, and each
// list is copied. The endpoint URLs are the ones authMembers lists.
func cloneAuthEndpoints(a AuthEndpoints) AuthEndpoints {
	for _, m := range authMembers {
		u := m.endpoint(&a)
		*u = cloneURL(*u)
	}
	for _, l := range []*[]string{
		&a.ScopesSupported, &a.ResponseTypesSupported, &a.CodeChallengeMethodsSupported,
		&a.GrantTypesSupported, &a.TokenEndpointAuthMethodsSupported,
		&a.TokenEndpointAuthSigningAlgValuesSupported, &a.IDTokenSigningAlgValuesSupported,
		&a.Capabilities,
	} {
		*l = slices.Clone(*l)
	}
	return a
}

// cloneURL returns a copy of u with its own copy of u's user information,
// or nil for a nil u.
func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	c := *u
	if u.User != nil {
		user := *u.User
		c.User = &user
	}
	return &c
}

// MustParseURL panics on invalid URLs; intended for static catalog
// construction in tests and example programs only.
func MustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		// raw may carry a password in its userinfo, and url.Parse's error
		// quotes raw too; the panic value must not carry it.
		panic(fmt.Sprintf("MustParseURL(%q): %v", urlredact.String(raw), urlredact.Error(err, raw)))
	}
	return u
}

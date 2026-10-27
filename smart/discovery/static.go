package discovery

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/urlredact"
)

// StaticConfig is the input to NewStaticCatalog, a hand-built catalog
// for openEHR backends that do not publish a discovery document.
//
// Printed with fmt or logged with log/slog, a StaticConfig shows every URL it
// holds with its password masked: see [StaticConfig.String]. Its JSON
// encoding is unchanged.
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

// String describes c by its fields, with the password of every URL that
// carries one masked as [url.URL.Redacted] masks it: BaseURL, Issuer, the
// base URL and links of each service, and the auth endpoints. Each service is
// shown by the text of [ServiceEntry.String], in the order of its key, Auth
// by the text of [AuthEndpoints.String], and ExpiresAt in RFC 3339 form.
//
// String, GoString, Format and LogValue are methods of StaticConfig, because
// [NewStaticCatalog] takes it as a value, so a *StaticConfig prints the same
// way. fmt answers %T and %p without calling Format, and %p of a
// StaticConfig value prints its fields, BaseURL and Issuer as written, so
// never print a StaticConfig with %p.
func (c StaticConfig) String() string {
	var services strings.Builder
	services.WriteString("map[")
	for i, id := range slices.Sorted(maps.Keys(c.Services)) {
		if i > 0 {
			services.WriteString(", ")
		}
		fmt.Fprintf(&services, "%q: %s", id, c.Services[id].String())
	}
	services.WriteString("]")
	return fmt.Sprintf("discovery.StaticConfig{BaseURL: %q, Issuer: %q, Services: %s, Auth: %s, ExpiresAt: %s, ETag: %q}",
		urlredact.String(c.BaseURL), urlredact.String(c.Issuer), services.String(), c.Auth.String(),
		c.ExpiresAt.Format(time.RFC3339Nano), c.ETag)
}

// GoString returns the text of [StaticConfig.String], so that printing c in
// Go syntax shows no password either.
func (c StaticConfig) GoString() string { return c.String() }

// Format writes the text of [StaticConfig.String] for every fmt verb,
// whatever its flags and width. With a nil f it does nothing.
func (c StaticConfig) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, c.String())
}

// LogValue lets log/slog log c as a group of its fields, keyed by field
// name, with every URL's password masked as [StaticConfig.String] masks it.
// Services is a group keyed by service identifier, each entry logged as
// [ServiceEntry.LogValue] logs it, and Auth is logged as
// [AuthEndpoints.LogValue] logs it.
func (c StaticConfig) LogValue() slog.Value {
	services := make([]slog.Attr, 0, len(c.Services))
	for _, id := range slices.Sorted(maps.Keys(c.Services)) {
		services = append(services, slog.Attr{Key: id, Value: c.Services[id].LogValue()})
	}
	return slog.GroupValue(
		slog.String("BaseURL", urlredact.String(c.BaseURL)),
		slog.String("Issuer", urlredact.String(c.Issuer)),
		slog.Attr{Key: "Services", Value: slog.GroupValue(services...)},
		slog.Attr{Key: "Auth", Value: c.Auth.LogValue()},
		slog.Time("ExpiresAt", c.ExpiresAt),
		slog.String("ETag", c.ETag),
	)
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
		Auth:       cfg.Auth.Clone(),
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

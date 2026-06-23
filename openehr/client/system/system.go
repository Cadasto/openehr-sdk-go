package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// ServiceCapabilities is the openEHR REST 1.1.0-development service-
// capabilities response per the System API. The exact shape varies
// across deployments; documented fields are typed and unknown fields
// are preserved verbatim in Extras for forward-compatibility.
//
// Returned by [Capabilities]. The standard fields are populated from
// the documented JSON keys; deployment-specific fields land in Extras
// as json.RawMessage so consumers can re-decode them with strict
// typing when their deployment is known.
type ServiceCapabilities struct {
	// Solution is the deployment's product name, e.g. "Cadasto" or
	// "EHRbase".
	Solution string `json:"solution,omitempty"`
	// SolutionVersion is the deployment's product version.
	SolutionVersion string `json:"solution_version,omitempty"`
	// Vendor is the deployment's vendor identifier.
	Vendor string `json:"vendor,omitempty"`
	// RESTAPISpecsVersion is the declared ITS-REST contract version.
	// Compared against the SDK pin at discovery time (REQ-072); this
	// field exposes the raw value for diagnostic display.
	RESTAPISpecsVersion string `json:"restapi_specs_version,omitempty"`
	// ConformanceProfile names the deployment's claimed conformance
	// profile (typically "default" or a deployment-specific label).
	ConformanceProfile string `json:"conformance_profile,omitempty"`
	// Endpoints lists the REST paths the deployment advertises. The
	// SDK does NOT use this for routing — service-catalog entries
	// (REQ-070) are the authoritative source — but it surfaces the
	// list for diagnostic and feature-detection use.
	Endpoints []string `json:"endpoints,omitempty"`
	// Extras preserves deployment-specific fields not in the
	// documented capabilities shape.
	Extras map[string]json.RawMessage `json:"-"`
}

// knownCapabilityFields is the JSON-tag set decoded into typed
// ServiceCapabilities fields. UnmarshalJSON routes every other key
// to Extras.
var knownCapabilityFields = map[string]struct{}{
	"solution":              {},
	"solution_version":      {},
	"vendor":                {},
	"restapi_specs_version": {},
	"conformance_profile":   {},
	"endpoints":             {},
}

// UnmarshalJSON decodes both the documented fields and the deployment-
// specific Extras in a single pass.
func (s *ServiceCapabilities) UnmarshalJSON(data []byte) error {
	type alias ServiceCapabilities
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*s = ServiceCapabilities(a)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		if _, ok := knownCapabilityFields[k]; ok {
			continue
		}
		if s.Extras == nil {
			s.Extras = map[string]json.RawMessage{}
		}
		s.Extras[k] = v
	}
	return nil
}

// MarshalJSON emits the documented fields first, then the Extras keys
// in insertion order. Round-trips through Unmarshal+Marshal preserve
// key sets (not necessarily key order — go map iteration order on the
// extras side is randomised).
func (s ServiceCapabilities) MarshalJSON() ([]byte, error) {
	type alias ServiceCapabilities
	known, err := json.Marshal(alias(s))
	if err != nil {
		return nil, err
	}
	if len(s.Extras) == 0 {
		return known, nil
	}
	// Re-decode known into a generic map so we can merge Extras and
	// re-encode with stable JSON-object semantics.
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(known, &merged); err != nil {
		return nil, err
	}
	if merged == nil {
		merged = map[string]json.RawMessage{}
	}
	maps.Copy(merged, s.Extras)
	return json.Marshal(merged)
}

// HealthStatus reports a coarse health signal derived from a probe
// against the deployment's base endpoint.
type HealthStatus struct {
	// Status is "up" when the probe returned 2xx, "down" otherwise
	// (non-2xx HTTP, network failure, or transport error).
	Status string
	// HTTPStatusCode is the raw HTTP status from the probe response.
	// Zero on network failure (no response was received).
	HTTPStatusCode int
	// CheckedAt records when the probe ran.
	CheckedAt time.Time
}

// IsUp reports whether the deployment was reachable and returned 2xx.
func (h *HealthStatus) IsUp() bool { return h != nil && h.Status == "up" }

const (
	healthUp   = "up"
	healthDown = "down"
)

// Capabilities issues OPTIONS against the openEHR REST service base URL
// and returns the typed service capabilities response.
//
// openEHR REST 1.1.0-development defines the System API's single operation
// as `OPTIONS /` (operationId `options`) — see
// resources/its-rest/system-validation.openapi.yaml line 52. Consumers
// SHOULD call this once at startup to confirm the deployment's declared
// spec version matches the SDK's pinned target. Capabilities respects the
// transport's configured auth path — anonymous calls are possible by
// constructing a separate transport.Client with auth.AnonymousTokenSource.
func Capabilities(ctx context.Context, c *transport.Client) (*ServiceCapabilities, *transport.Metadata, error) {
	resp, err := c.Do(ctx, &transport.Request{
		Method: http.MethodOptions,
		Path:   "/",
		Route:  "/",
	})
	if err != nil {
		if resp != nil {
			return nil, resp.Metadata, err
		}
		return nil, nil, err
	}
	if len(resp.Body) == 0 {
		return nil, resp.Metadata, fmt.Errorf("system.Capabilities: %w: empty body", transport.ErrInvalidShape)
	}
	var sc ServiceCapabilities
	if err := json.Unmarshal(resp.Body, &sc); err != nil {
		return nil, resp.Metadata, fmt.Errorf("system.Capabilities: decode: %w", err)
	}
	return &sc, resp.Metadata, nil
}

// Version returns the deployment's declared ITS-REST specification
// version as a convenience over the full [Capabilities] call. The
// returned string is the raw value advertised by the deployment;
// callers SHOULD compare against constants from smart/discovery
// rather than literal strings.
func Version(ctx context.Context, c *transport.Client) (string, error) {
	caps, _, err := Capabilities(ctx, c)
	if err != nil {
		return "", err
	}
	return caps.RESTAPISpecsVersion, nil
}

// Health probes the deployment's base endpoint and returns a coarse
// "up"/"down" signal. A 2xx response yields Status="up"; a wire error
// (non-2xx HTTP) yields Status="down" with HTTPStatusCode populated
// from the response; a transport error (network failure, ctx
// cancellation pre-response) returns a non-nil error.
//
// Health issues an anonymous `OPTIONS /` request (the System API's only
// operation) — a misconfigured TokenSource MUST NOT skew the health
// signal, since monitoring tools commonly run without credentials.
// Deployments exposing a dedicated /health path SHOULD wire that up via a
// Cadasto-platform Extra rather than this method; the SDK targets the
// standard ITS-REST capabilities endpoint here for portability.
func Health(ctx context.Context, c *transport.Client) (*HealthStatus, error) {
	resp, err := c.Do(ctx, &transport.Request{
		Method: http.MethodOptions,
		Path:   "/",
		Route:  "/",
		NoAuth: true,
	})
	h := &HealthStatus{CheckedAt: time.Now()}
	if err != nil {
		var we *transport.WireError
		if errors.As(err, &we) {
			h.Status = healthDown
			h.HTTPStatusCode = we.StatusCode
			return h, nil
		}
		h.Status = healthDown
		return h, err
	}
	h.Status = healthUp
	if resp != nil {
		h.HTTPStatusCode = resp.StatusCode
	}
	return h, nil
}

// Repository mirrors the package-level System functions as a method
// set bound to a single *transport.Client. Useful for dependency-
// injection seams (REQ-023); the package-level functions remain the
// primary call surface.
type Repository interface {
	Capabilities(ctx context.Context) (*ServiceCapabilities, *transport.Metadata, error)
	Version(ctx context.Context) (string, error)
	Health(ctx context.Context) (*HealthStatus, error)
}

// NewRepository binds c to a Repository. The resulting value is safe
// for concurrent use by multiple goroutines (transport.Client is, and
// the binding is read-only).
func NewRepository(c *transport.Client) Repository {
	return &repository{c: c}
}

type repository struct{ c *transport.Client }

func (r *repository) Capabilities(ctx context.Context) (*ServiceCapabilities, *transport.Metadata, error) {
	return Capabilities(ctx, r.c)
}

func (r *repository) Version(ctx context.Context) (string, error) {
	return Version(ctx, r.c)
}

func (r *repository) Health(ctx context.Context) (*HealthStatus, error) {
	return Health(ctx, r.c)
}

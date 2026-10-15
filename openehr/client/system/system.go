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

// ServiceCapabilities is the openEHR REST 1.1.0 service-
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
	// Compared against the ITS-REST version the SDK targets at discovery
	// time; this
	// field exposes the raw value for diagnostic display.
	RESTAPISpecsVersion string `json:"restapi_specs_version,omitempty"`
	// ConformanceProfile names the deployment's claimed conformance
	// profile (typically "default" or a deployment-specific label).
	ConformanceProfile string `json:"conformance_profile,omitempty"`
	// Endpoints lists the REST paths the deployment advertises. The
	// SDK does not use this for routing (service-catalog entries
	// are the authoritative source), but it surfaces the
	// list for diagnostic and feature-detection use.
	Endpoints []string `json:"endpoints,omitempty"`
	// Extras preserves deployment-specific fields not in the
	// documented capabilities shape.
	// [ServiceCapabilities.MarshalJSON] re-emits them, as for the other
	// response descriptors that preserve unknown keys.
	//
	// Extras keys are matched against the documented field names
	// case-sensitively, while encoding/json decodes those field names
	// case-insensitively. A wire key differing from a documented field
	// only by case ("Solution") therefore populates the documented field
	// and is also preserved here verbatim, so encode emits both keys when
	// the documented field emits one at all. `solution` is omitempty, so
	// an empty Solution emits only the preserved key.
	Extras map[string]json.RawMessage `json:"-"`
}

// knownCapabilityFields is the JSON-tag set decoded into typed
// ServiceCapabilities fields. UnmarshalJSON routes every other key
// to Extras, and MarshalJSON deletes these keys from the Extras clone
// it merges, so a colliding caller-set entry cannot displace the
// documented field.
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

// MarshalJSON re-emits the documented fields plus Extras.
//
// An Extras key that collides with a documented field name is ignored on
// encode; the documented field is authoritative. Unknown keys are
// preserved and re-emitted, and documented fields are emitted per their
// own contract (each is omitted when empty), so the emitted key set is
// not guaranteed to be identical to the wire body a value was decoded
// from. Neither is its spelling: encoding/json compacts insignificant
// whitespace and escapes `<`, `>` and `&` as `\u003c`, `\u003e` and
// `\u0026` inside a preserved value (the escaped spelling decodes to
// the identical value). Key order is not part of the contract either: a
// non-empty Extras path marshals a map, and encoding/json sorts a map's
// keys, so documented fields are not emitted first there.
func (s ServiceCapabilities) MarshalJSON() ([]byte, error) {
	type alias ServiceCapabilities
	known, err := json.Marshal(alias(s))
	if err != nil {
		return nil, err
	}
	if len(s.Extras) == 0 {
		return known, nil
	}
	// Start from Extras with every documented field name deleted, then
	// overlay the typed emission — re-decoding known into a generic map so
	// the merge and the re-encode keep JSON-object semantics. Deleting
	// rather than merely letting the overlay win is what makes the rule
	// hold for an omitted field: every documented field here carries
	// omitempty, so an empty one emits no key for the overlay to win with
	// and a caller-set Extras entry would otherwise survive in its place.
	merged := maps.Clone(s.Extras)
	for k := range knownCapabilityFields {
		delete(merged, k)
	}
	if err := json.Unmarshal(known, &merged); err != nil {
		return nil, err
	}
	return json.Marshal(merged)
}

// HealthStatus reports a coarse health signal derived from a probe
// against the deployment's base endpoint.
type HealthStatus struct {
	// Status is "up" when the probe returned a 2xx whose body was read in
	// full, "down" otherwise: a status that is not 2xx, a 2xx whose body
	// could not be read, a network failure, or any other transport error.
	Status string
	// HTTPStatusCode is the raw HTTP status of the last probe response,
	// whether the probe was up or down. It is zero when no response
	// arrived (a network failure, for instance).
	HTTPStatusCode int
	// CheckedAt records when the probe ran.
	CheckedAt time.Time
}

// IsUp reports whether the deployment was reachable and returned a 2xx
// whose body was read in full.
func (h *HealthStatus) IsUp() bool { return h != nil && h.Status == "up" }

const (
	healthUp   = "up"
	healthDown = "down"
)

// Capabilities issues OPTIONS against the openEHR REST service base URL
// and returns the typed service capabilities response.
//
// openEHR REST 1.1.0 defines the System API's single operation
// as `OPTIONS /` (operationId `options`). Call this once at startup to
// confirm the deployment's declared spec version matches the ITS-REST
// version the SDK targets. Capabilities respects the transport's
// configured auth path; anonymous calls are possible by
// constructing a separate transport.Client with auth.AnonymousTokenSource.
func Capabilities(ctx context.Context, c *transport.Client) (*ServiceCapabilities, *transport.Metadata, error) {
	req := &transport.Request{
		Method: http.MethodOptions,
		Path:   "/",
		Route:  "/",
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, resp.Metadata, err
		}
		return nil, nil, err
	}
	if transport.IsNoRepresentationBody(resp.Body) {
		return nil, resp.Metadata, fmt.Errorf("system.Capabilities: %w: empty or null body", transport.ErrInvalidShape)
	}
	var sc ServiceCapabilities
	if err := json.Unmarshal(resp.Body, &sc); err != nil {
		return nil, resp.Metadata, fmt.Errorf("system.Capabilities: %w", &transport.DecodeError{
			Method: req.Method, Route: req.Route, Body: resp.Body, Inner: err,
		})
	}
	return &sc, resp.Metadata, nil
}

// Version returns the deployment's declared ITS-REST specification
// version as a convenience over the full [Capabilities] call. The
// returned string is the raw value advertised by the deployment;
// callers should compare against constants from smart/discovery
// rather than literal strings.
func Version(ctx context.Context, c *transport.Client) (string, error) {
	caps, _, err := Capabilities(ctx, c)
	if err != nil {
		return "", err
	}
	return caps.RESTAPISpecsVersion, nil
}

// Health probes the deployment's base endpoint and returns a coarse
// "up"/"down" signal. It always returns a non-nil [HealthStatus]; the
// outcome depends on how far the probe got:
//
//   - A 2xx whose body was read in full gives Status "up", HTTPStatusCode
//     set to that status, and a nil error.
//   - A status that is not 2xx gives Status "down", HTTPStatusCode set to
//     that status, and a nil error: the deployment answered, so the
//     status is the signal, not a failure of the probe.
//   - A 2xx whose body could not be read (the connection dropped
//     mid-body, for instance) gives Status "down", HTTPStatusCode set to
//     that status, and an error from which
//     errors.AsType[*transport.BodyReadError] recovers the read failure:
//     the probe did not complete.
//   - A failure before any response arrived (a network error, or ctx
//     ending before the server answered) gives Status "down",
//     HTTPStatusCode zero, and that error.
//
// Any other error after a response arrived, such as ctx ending while a
// retry waits, also gives Status "down" and that error, with
// HTTPStatusCode set to the status of that response. With retries
// enabled, each case describes the last attempt.
//
// Health issues an anonymous `OPTIONS /` request (the System API's only
// operation), so a misconfigured TokenSource cannot skew the health
// signal; monitoring tools commonly run without credentials.
// Deployments exposing a dedicated /health path should wire that up via a
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
		// A boxed typed-nil *WireError matches with ok=true and a nil
		// pointer, so the nil check is load-bearing (REQ-025 nil-receiver
		// axis); such an error carries no status and takes the branch below.
		if we, ok := errors.AsType[*transport.WireError](err); ok && we != nil {
			h.Status = healthDown
			h.HTTPStatusCode = we.StatusCode
			return h, nil
		}
		// Every other error means the probe did not complete. When a
		// response arrived first (a 2xx whose body could not be read, which
		// Do returns beside its *BodyReadError, or ctx ending during a retry
		// wait), keep the status the server sent; it stays zero otherwise.
		h.Status = healthDown
		if resp != nil {
			h.HTTPStatusCode = resp.StatusCode
		}
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
// injection seams; the package-level functions remain the
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

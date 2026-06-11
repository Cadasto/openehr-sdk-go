package discovery

import (
	"fmt"
	"strings"
)

// DiscoveryErrorReason classifies a discovery failure.
type DiscoveryErrorReason string

const (
	// ReasonFetchFailed indicates the SMART configuration document
	// could not be retrieved (network error, non-2xx HTTP status).
	ReasonFetchFailed DiscoveryErrorReason = "fetch_failed"
	// ReasonParseError indicates the response body could not be parsed
	// as a SMART configuration document.
	ReasonParseError DiscoveryErrorReason = "parse_error"
	// ReasonMissingService indicates a required service identifier is
	// absent from the catalog. MissingServices enumerates which.
	ReasonMissingService DiscoveryErrorReason = "missing_service"
	// ReasonSpecVersionMismatch indicates a required service's declared
	// spec_version does not match the SDK's pinned target or accepted
	// set (REQ-072, PROBE-003).
	ReasonSpecVersionMismatch DiscoveryErrorReason = "spec_version_mismatch"
	// ReasonMalformedURL indicates a URL field (BaseURL,
	// AuthorizationEndpoint, etc.) failed parsing.
	ReasonMalformedURL DiscoveryErrorReason = "malformed_url"
	// ReasonAuthEndpointsMissing indicates the authorization or token
	// endpoint URL is absent from a SMART config that requires them.
	ReasonAuthEndpointsMissing DiscoveryErrorReason = "auth_endpoints_missing"
	// ReasonInsecureURL indicates an http:// URL was rejected by the
	// default fetcher (REQ-092). Override with WithAllowInsecure to opt
	// into plaintext fetches in development.
	ReasonInsecureURL DiscoveryErrorReason = "insecure_url"
	// ReasonIssuerMismatch indicates the discovery document's "issuer"
	// field does not equal the URL used to fetch it. Per OIDC Discovery
	// §4.3 this is a hard validation failure — accepting a mismatched
	// issuer would let a hostile server impersonate another identity
	// provider downstream.
	ReasonIssuerMismatch DiscoveryErrorReason = "issuer_mismatch"
)

// DiscoveryError is the typed error every discovery failure surfaces
// as. Distinguish from transport.WireError via errors.As.
type DiscoveryError struct {
	// Issuer is the deployment issuer URL whose discovery failed.
	Issuer string
	// Reason classifies the failure.
	Reason DiscoveryErrorReason
	// MissingServices enumerates absent required services when Reason
	// is ReasonMissingService. Empty otherwise.
	MissingServices []string
	// SpecVersionGot / SpecVersionWant carry the version comparison when
	// Reason is ReasonSpecVersionMismatch. Empty otherwise.
	SpecVersionGot, SpecVersionWant string
	// Inner is the underlying network / parse error, when applicable.
	Inner error
}

// Error implements error.
func (e *DiscoveryError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "discovery: %s", e.Reason)
	if e.Issuer != "" {
		fmt.Fprintf(&b, " issuer=%s", e.Issuer)
	}
	switch e.Reason {
	case ReasonMissingService:
		if len(e.MissingServices) > 0 {
			fmt.Fprintf(&b, " missing=[%s]", strings.Join(e.MissingServices, ","))
		}
	case ReasonSpecVersionMismatch:
		if e.SpecVersionGot != "" || e.SpecVersionWant != "" {
			fmt.Fprintf(&b, " got=%q want=%q", e.SpecVersionGot, e.SpecVersionWant)
		}
	}
	if e.Inner != nil {
		fmt.Fprintf(&b, ": %v", e.Inner)
	}
	return b.String()
}

// Unwrap exposes the inner cause to errors.Is / errors.As.
func (e *DiscoveryError) Unwrap() error { return e.Inner }

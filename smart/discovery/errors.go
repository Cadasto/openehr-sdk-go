package discovery

import (
	"fmt"
	"strings"
)

// DiscoveryErrorReason classifies a discovery failure.
type DiscoveryErrorReason string

const (
	// ReasonFetchFailed indicates the SMART configuration document
	// could not be retrieved (network error, non-2xx HTTP status, or a
	// 304 Not Modified the resolver did not ask for), or the issuer's
	// OpenID configuration document could not be retrieved or read, or
	// names no issuer.
	ReasonFetchFailed DiscoveryErrorReason = "fetch_failed"
	// ReasonParseError indicates the response body could not be parsed
	// as a SMART configuration document.
	ReasonParseError DiscoveryErrorReason = "parse_error"
	// ReasonMissingService indicates a required service identifier is
	// absent from the catalog. MissingServices enumerates which.
	ReasonMissingService DiscoveryErrorReason = "missing_service"
	// ReasonSpecVersionMismatch indicates a required service's declared
	// spec_version does not match the SDK's pinned target or accepted
	// set. With WithAcceptedSpecVersions, an entry without spec_version is
	// judged by its version member instead.
	ReasonSpecVersionMismatch DiscoveryErrorReason = "spec_version_mismatch"
	// ReasonMalformedURL indicates a URL the resolver cannot use. The base
	// URL passed to the resolver, the issuer the document declares, an
	// auth endpoint URL or a service baseUrl failed parsing, is not
	// absolute or has no host name; the declared issuer or an auth endpoint
	// uses a scheme other than https or http; or the declared issuer has a
	// query or fragment. A plaintext http URL where https is required is
	// ReasonInsecureURL instead.
	ReasonMalformedURL DiscoveryErrorReason = "malformed_url"
	// ReasonAuthEndpointsMissing indicates the SMART configuration
	// declares authorization-server members but omits one it needs:
	// token_endpoint whenever any of authorization_endpoint, token_endpoint
	// or jwks_uri is present; authorization_endpoint when capabilities
	// lists launch-ehr or launch-standalone; jwks_uri when it lists
	// sso-openid-connect. A document with none of the three is an
	// anonymous-only deployment and is not refused.
	ReasonAuthEndpointsMissing DiscoveryErrorReason = "auth_endpoints_missing"
	// ReasonInsecureURL indicates a plaintext URL was refused where https
	// is required: an http base URL passed to the resolver, an http issuer
	// the document declares, an http auth endpoint URL, or a redirect to a
	// URL that is not https while fetching the SMART configuration or the
	// issuer's OpenID configuration. Override with WithAllowInsecure to opt
	// into plaintext URLs in development.
	ReasonInsecureURL DiscoveryErrorReason = "insecure_url"
	// ReasonIssuerMismatch indicates the issuer's own OpenID configuration
	// does not confirm what the SMART configuration declares: its "issuer"
	// differs from the declared issuer, or its "jwks_uri" differs from the
	// SMART configuration's. The resolver checks this only when the
	// declared issuer differs from the base URL, and not at all when built
	// with WithoutOpenIDConfigurationCheck.
	ReasonIssuerMismatch DiscoveryErrorReason = "issuer_mismatch"
)

// DiscoveryError is the typed error every discovery failure surfaces
// as. Distinguish from transport.WireError via errors.As.
type DiscoveryError struct {
	// Issuer is the Platform base URL the resolution was for: the URL
	// passed to Resolver.Resolve or Resolver.Refresh, or the base URL of a
	// hand-built catalog. Despite its name it is not the OpenID Connect
	// issuer; ServiceCatalog explains the difference. Error prints it as
	// "base_url=".
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

// Error implements error. A nil receiver returns the zero
// DiscoveryError's text instead of panicking, so the typed nil a failed
// errors.As or errors.AsType leaves behind is safe to print.
func (e *DiscoveryError) Error() string {
	if e == nil {
		return (&DiscoveryError{}).Error()
	}
	var b strings.Builder
	// Every SDK producer sets Reason; only a caller-built zero value leaves
	// it empty, and that must not render as a dangling "discovery: ".
	reason := e.Reason
	if reason == "" {
		reason = "unspecified"
	}
	fmt.Fprintf(&b, "discovery: %s", reason)
	if e.Issuer != "" {
		fmt.Fprintf(&b, " base_url=%s", e.Issuer)
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
	case ReasonFetchFailed, ReasonParseError, ReasonMalformedURL,
		ReasonAuthEndpointsMissing, ReasonInsecureURL, ReasonIssuerMismatch:
		// No reason-specific detail beyond the reason name written above.
	}
	if e.Inner != nil {
		fmt.Fprintf(&b, ": %v", e.Inner)
	}
	return b.String()
}

// Unwrap exposes the inner cause to errors.Is / errors.As. A nil
// receiver unwraps to nil.
func (e *DiscoveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}

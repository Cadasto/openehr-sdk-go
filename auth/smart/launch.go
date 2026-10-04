package smart

import (
	"fmt"
	"net/url"
)

// EHRLaunch holds the parameters an EHR appends to the app's launch URL
// when it launches the app from inside its own session.
type EHRLaunch struct {
	// Issuer is the iss parameter: the Platform base URL to resolve the
	// SMART configuration from (see smart/discovery).
	Issuer string
	// Launch is the launch parameter, an opaque value to pass unchanged to
	// [Source.AuthorizeURL].
	Launch string
}

// ParseEHRLaunch reads an embedded launch from query, the query of the
// app's launch URL, and checks its issuer against allow.
//
// Anyone can send a browser to the launch URL with any iss, so call
// ParseEHRLaunch before resolving discovery for that issuer: allow decides
// which Platforms the app trusts, and it receives the iss value exactly as
// the URL carried it.
//
// ParseEHRLaunch fails with [ErrLaunchInvalidRequest] when iss is missing
// or is not an absolute URL with a host, or when launch is missing. It
// checks those first, so allow is only asked about a well-formed launch.
// It then fails with [ErrLaunchIssuerNotAllowed] when allow is nil or
// returns false. The errors never repeat the parameter values.
func ParseEHRLaunch(query url.Values, allow func(iss string) bool) (EHRLaunch, error) {
	// A missing iss parses as an empty, relative URL. Hostname, not Host:
	// "https://:8443" has a Host of ":8443" and no host.
	iss := query.Get("iss")
	if u, err := url.Parse(iss); err != nil || !u.IsAbs() || u.Hostname() == "" {
		return EHRLaunch{}, fmt.Errorf("%w: iss is missing or not an absolute URL with a host", ErrLaunchInvalidRequest)
	}
	launch := query.Get("launch")
	if launch == "" {
		return EHRLaunch{}, fmt.Errorf("%w: launch is missing", ErrLaunchInvalidRequest)
	}
	if allow == nil || !allow(iss) {
		return EHRLaunch{}, ErrLaunchIssuerNotAllowed
	}
	return EHRLaunch{Issuer: iss, Launch: launch}, nil
}

package smart_test

import (
	"errors"
	"net/url"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

const (
	launchIssuer = "https://platform.example/openehr"
	launchValue  = "xyz123"
)

// recordingAllow returns an allowlist that trusts only trusted and records
// every issuer it is asked about.
func recordingAllow(trusted string) (func(string) bool, *[]string) {
	var asked []string
	return func(iss string) bool {
		asked = append(asked, iss)
		return iss == trusted
	}, &asked
}

// TestParseEHRLaunchAccepted pins REQ-061: a launch URL with an absolute iss
// that the allowlist trusts and a launch value parses into both, verbatim,
// and the allowlist is asked about that exact iss.
func TestParseEHRLaunchAccepted(t *testing.T) { // REQ-061
	allow, asked := recordingAllow(launchIssuer)
	got, err := smart.ParseEHRLaunch(url.Values{"iss": {launchIssuer}, "launch": {launchValue}}, allow)
	if err != nil {
		t.Fatalf("ParseEHRLaunch() error = %v, want success", err)
	}
	if want := (smart.EHRLaunch{Issuer: launchIssuer, Launch: launchValue}); got != want {
		t.Errorf("ParseEHRLaunch() = %+v, want %+v", got, want)
	}
	if !slices.Equal(*asked, []string{launchIssuer}) {
		t.Errorf("allowlist asked about %q, want [%q]", *asked, launchIssuer)
	}
}

// TestParseEHRLaunchRefused pins REQ-061: a launch URL without iss, with an
// iss that is not an absolute URL with a host, or without launch fails
// with ErrLaunchInvalidRequest before the allowlist is asked; a well-formed
// launch whose iss the allowlist refuses, or that comes with no allowlist,
// fails with ErrLaunchIssuerNotAllowed.
func TestParseEHRLaunchRefused(t *testing.T) { // REQ-061
	tests := []struct {
		name    string
		query   url.Values
		nilList bool // pass a nil allowlist
		want    error
	}{
		{name: "iss missing", query: url.Values{"launch": {launchValue}}, want: smart.ErrLaunchInvalidRequest},
		{name: "iss missing and no allowlist", query: url.Values{"launch": {launchValue}}, nilList: true, want: smart.ErrLaunchInvalidRequest},
		{name: "iss empty", query: url.Values{"iss": {""}, "launch": {launchValue}}, want: smart.ErrLaunchInvalidRequest},
		{name: "iss relative", query: url.Values{"iss": {"/openehr"}, "launch": {launchValue}}, want: smart.ErrLaunchInvalidRequest},
		{name: "iss without scheme", query: url.Values{"iss": {"platform.example/openehr"}, "launch": {launchValue}}, want: smart.ErrLaunchInvalidRequest},
		{name: "iss scheme-relative", query: url.Values{"iss": {"//platform.example/openehr"}, "launch": {launchValue}}, want: smart.ErrLaunchInvalidRequest},
		{name: "iss without host", query: url.Values{"iss": {"https:///openehr"}, "launch": {launchValue}}, want: smart.ErrLaunchInvalidRequest},
		{name: "iss with a port and no host", query: url.Values{"iss": {"https://:8443/openehr"}, "launch": {launchValue}}, want: smart.ErrLaunchInvalidRequest},
		{name: "iss opaque", query: url.Values{"iss": {"urn:example:platform"}, "launch": {launchValue}}, want: smart.ErrLaunchInvalidRequest},
		{name: "iss unparseable", query: url.Values{"iss": {"https://plat form.example"}, "launch": {launchValue}}, want: smart.ErrLaunchInvalidRequest},
		{name: "launch missing", query: url.Values{"iss": {launchIssuer}}, want: smart.ErrLaunchInvalidRequest},
		{name: "launch empty", query: url.Values{"iss": {launchIssuer}, "launch": {""}}, want: smart.ErrLaunchInvalidRequest},
		{name: "launch missing for an untrusted iss", query: url.Values{"iss": {"https://other.example"}}, want: smart.ErrLaunchInvalidRequest},
		{name: "iss not on the allowlist", query: url.Values{"iss": {"https://other.example/openehr"}, "launch": {launchValue}}, want: smart.ErrLaunchIssuerNotAllowed},
		{name: "no allowlist", query: url.Values{"iss": {launchIssuer}, "launch": {launchValue}}, nilList: true, want: smart.ErrLaunchIssuerNotAllowed},
	}
	sentinels := []error{smart.ErrLaunchInvalidRequest, smart.ErrLaunchIssuerNotAllowed}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			allow, asked := recordingAllow(launchIssuer)
			if tc.nilList {
				allow = nil
			}
			got, err := smart.ParseEHRLaunch(tc.query, allow)
			for _, s := range sentinels {
				if is, want := errors.Is(err, s), errors.Is(tc.want, s); is != want {
					t.Errorf("ParseEHRLaunch(%v) error = %v; errors.Is(%v) = %v, want %v", tc.query, err, s, is, want)
				}
			}
			if got != (smart.EHRLaunch{}) {
				t.Errorf("ParseEHRLaunch(%v) = %+v on failure, want the zero EHRLaunch", tc.query, got)
			}
			if errors.Is(tc.want, smart.ErrLaunchInvalidRequest) && len(*asked) > 0 {
				t.Errorf("ParseEHRLaunch(%v) asked the allowlist about %q, want no call for a malformed launch", tc.query, *asked)
			}
		})
	}
}

package auth

import "strings"

// Launch-context and refresh scope constants for SMART App Launch.
//
// These values are defined in the HL7 SMART App Launch v2 specification
// (https://hl7.org/fhir/smart-app-launch/) "Scopes and Launch Context" section
// and in the openEHR SMART App Launch specification
// (https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html).
//
// All constants are purely lexical — the SDK does NOT enforce their presence or
// absence; the deployment is authoritative (consistent with the BuildScope note
// below). [REQ-061]
const (
	ScopeOpenID        = "openid"         // required for ID-token issuance
	ScopeProfile       = "profile"        // request standard profile claims
	ScopeFHIRUser      = "fhirUser"       // request fhirUser identity claim
	ScopeLaunch        = "launch"         // EHR-launch context (embedded / iFrame launch)
	ScopeLaunchPatient = "launch/patient" // standalone patient-context launch; triggers ehrId claim on openEHR
	ScopeLaunchEpisode = "launch/episode" // standalone episode-context launch (openEHR experimental)
	ScopeOfflineAccess = "offline_access" // request a refresh token (persists beyond browser session)
	ScopeOnlineAccess  = "online_access"  // request a refresh token scoped to the current session only
)

// BuildScope composes an openEHR-formatted scope from its three parts
// per the SMART-on-openEHR convention: <compartment>/<resource>.<permission>.
//
// Empty parts collapse to omitted segments — BuildScope("", "COMPOSITION", "read")
// returns "COMPOSITION.read". BuildScope is purely lexical and does NOT
// validate the parts against any scope grammar; the deployment is
// authoritative on which scopes it accepts (docs/specifications/auth.md § Scope handling).
//
// The helper exists so consumers do not template scope strings by hand
// in the most common case; consumers MAY pass raw scopes to providers
// when they need shapes BuildScope does not cover.
func BuildScope(compartment, resource, permission string) string {
	resource = strings.TrimSpace(resource)
	permission = strings.TrimSpace(permission)
	compartment = strings.TrimSpace(compartment)

	var b strings.Builder
	if compartment != "" {
		b.WriteString(compartment)
		b.WriteByte('/')
	}
	b.WriteString(resource)
	if permission != "" {
		b.WriteByte('.')
		b.WriteString(permission)
	}
	return b.String()
}

// JoinScopes joins scope strings into the space-separated form the
// OAuth2 authorization request expects. Empty inputs are skipped.
func JoinScopes(scopes ...string) string {
	out := scopes[:0:0]
	for _, s := range scopes {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, " ")
}

package auth

import (
	"fmt"
	"strings"
)

// Launch-context and refresh scope constants for SMART App Launch.
//
// These values are defined in the HL7 FHIR SMART App Launch v2 specification
// (https://hl7.org/fhir/smart-app-launch/) "Scopes and Launch Context" section
// and in the openEHR SMART App Launch specification
// (https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html).
//
// All constants are purely lexical: the SDK does not enforce their presence or
// absence, and the deployment decides (as with BuildScope below).
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

// BuildScope joins a compartment, a resource and a permission into one
// scope string of the shape "<compartment>/<resource>.<permission>", after
// trimming surrounding white space from each part.
//
// Empty parts collapse to omitted segments: BuildScope("", "launch", "")
// returns "launch". BuildScope does not validate its parts against any
// scope grammar, so it also serves SMART on FHIR scopes such as
// BuildScope("patient", "Observation", "rs"); use [OpenEHRScope] for
// checked openEHR resource scopes. The deployment decides which scopes it
// accepts.
//
// The helper saves callers from templating scope strings by hand in the
// most common case; callers can pass raw scopes to providers when they
// need shapes BuildScope does not cover.
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

// OpenEHRScope is one openEHR resource scope of the SMART on openEHR
// specification, written "<compartment>/<resource>-<pattern>.<permissions>",
// for example "patient/composition-*.rs".
//
// The zero value is not a valid scope. [OpenEHRScope.Token] writes a scope
// and [ParseOpenEHRScope] reads one. Neither interprets the "*" wildcards in
// Pattern: which requests a granted scope covers is decided by the
// authorization server and the resource server, not by the SDK.
type OpenEHRScope struct {
	// Compartment is "patient", "user" or "system".
	Compartment string
	// Resource is "template", "composition" or "aql".
	Resource string
	// Pattern is a template id or a stored-query name, written as given. It
	// may hold "*" wildcards, whose matching the authorization server defines.
	Pattern string
	// Permissions is a non-empty subset of the letters "cruds" (create, read,
	// update, delete, search), each used once and in that order, such as "rs".
	Permissions string
}

// permissionOrder holds the permission letters in the only order a scope
// token may use them.
const permissionOrder = "cruds"

// Token returns the scope token "<compartment>/<resource>-<pattern>.<permissions>".
//
// Token returns an error matching [ErrInvalidScope], and no token, when the
// compartment is not "patient", "user" or "system"; when the resource is not
// "template", "composition" or "aql"; when the permissions are empty, repeat
// a letter, use a letter other than c, r, u, d and s, or break the order
// c, r, u, d, s (the permission syntax of HL7 SMART App Launch v2); or when
// the pattern is empty or holds a space, a double quote, a backslash, or any
// character that is not printable ASCII (the RFC 6749 §3.3 scope-token set).
// Token does not escape or rewrite the pattern, so a template id with a
// space or a non-ASCII character cannot be written as a scope. The error
// names the part that is wrong.
func (s OpenEHRScope) Token() (string, error) {
	if err := s.check(); err != nil {
		return "", err
	}
	return s.Compartment + "/" + s.Resource + "-" + s.Pattern + "." + s.Permissions, nil
}

// ParseOpenEHRScope reads one openEHR resource scope token, such as one entry
// of a granted scope list. The compartment ends at the first "/", the
// permissions start after the last "." and the resource ends at the first
// "-", so dotted template ids and query names such as
// "org.openehr::bloodpressure.v1" come back whole.
//
// ParseOpenEHRScope reports false, with the zero OpenEHRScope, for a token
// that [OpenEHRScope.Token] would not write: for example "openid",
// "launch/patient" or the SMART on FHIR scope "patient/Observation.rs". When
// it reports true, Token on the result returns the same token.
func ParseOpenEHRScope(token string) (OpenEHRScope, bool) {
	compartment, rest, found := strings.Cut(token, "/")
	if !found {
		return OpenEHRScope{}, false
	}
	dot := strings.LastIndexByte(rest, '.')
	if dot < 0 {
		return OpenEHRScope{}, false
	}
	resource, pattern, found := strings.Cut(rest[:dot], "-")
	if !found {
		return OpenEHRScope{}, false
	}
	s := OpenEHRScope{
		Compartment: compartment,
		Resource:    resource,
		Pattern:     pattern,
		Permissions: rest[dot+1:],
	}
	if s.check() != nil {
		return OpenEHRScope{}, false
	}
	return s, true
}

// check applies every rule of the scope-token grammar to s. Token and
// ParseOpenEHRScope share it, so a parsed scope always writes back.
// The errors name the broken rule but never quote the caller's value.
func (s OpenEHRScope) check() error {
	switch s.Compartment {
	case "patient", "user", "system":
	default:
		return fmt.Errorf("%w: compartment is not patient, user or system", ErrInvalidScope)
	}
	switch s.Resource {
	case "template", "composition", "aql":
	default:
		return fmt.Errorf("%w: resource is not template, composition or aql", ErrInvalidScope)
	}
	if err := checkPattern(s.Pattern); err != nil {
		return err
	}
	return checkPermissions(s.Permissions)
}

// checkPattern refuses an empty pattern and one with a byte outside the
// RFC 6749 §3.3 scope-token set.
func checkPattern(p string) error {
	if p == "" {
		return fmt.Errorf("%w: pattern is empty", ErrInvalidScope)
	}
	for i := range len(p) {
		if !isScopeTokenByte(p[i]) {
			return fmt.Errorf("%w: pattern contains a character outside the RFC 6749 scope-token set", ErrInvalidScope)
		}
	}
	return nil
}

// isScopeTokenByte reports whether b belongs to the RFC 6749 §3.3
// scope-token set %x21 / %x23-5B / %x5D-7E. Every byte of a multi-byte UTF-8
// character is 0x80 or above, so non-ASCII text falls outside it.
func isScopeTokenByte(b byte) bool {
	return b == 0x21 || (b >= 0x23 && b <= 0x5B) || (b >= 0x5D && b <= 0x7E)
}

// checkPermissions refuses empty permissions and any letter that is outside
// "cruds", repeats the letter before it, or comes before that letter in
// "cruds". A letter repeated further apart is refused as out of order: the
// letters between the two cannot all rise and still return to it.
func checkPermissions(p string) error {
	if p == "" {
		return fmt.Errorf("%w: permissions are empty", ErrInvalidScope)
	}
	last := -1
	for i := range len(p) {
		k := strings.IndexByte(permissionOrder, p[i])
		switch {
		case k < 0:
			return fmt.Errorf("%w: permissions use a letter outside %q", ErrInvalidScope, permissionOrder)
		case k == last:
			return fmt.Errorf("%w: permissions repeat a letter", ErrInvalidScope)
		case k < last:
			return fmt.Errorf("%w: permissions are out of order, want the order %q", ErrInvalidScope, permissionOrder)
		}
		last = k
	}
	return nil
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

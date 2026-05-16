package auth

import "strings"

// BuildScope composes an openEHR-formatted scope from its three parts
// per the SMART-on-openEHR convention: <compartment>/<resource>.<permission>.
//
// Empty parts collapse to omitted segments — BuildScope("", "COMPOSITION", "read")
// returns "COMPOSITION.read". BuildScope is purely lexical and does NOT
// validate the parts against any scope grammar; the deployment is
// authoritative on which scopes it accepts (specs/auth.md § Scope handling).
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

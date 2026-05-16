package transport

import (
	"context"
	"encoding/json"
	"strings"
)

// Prefer is the typed enum for HTTP Prefer return-mode negotiation per
// REQ-094. The zero value (PreferDefault) suppresses the header.
type Prefer string

const (
	// PreferDefault sends no Prefer header — server applies its own
	// default for the endpoint (typically "representation" on reads,
	// "minimal" on writes).
	PreferDefault Prefer = ""
	// PreferMinimal asks for an empty body plus Location/ETag.
	PreferMinimal Prefer = "return=minimal"
	// PreferIdentifier asks for the identifier-only response shape.
	PreferIdentifier Prefer = "return=identifier"
	// PreferRepresentation asks for the full new-resource body.
	PreferRepresentation Prefer = "return=representation"
)

// HeaderValue returns the on-wire value for the Prefer header or the
// empty string when no header should be emitted.
func (p Prefer) HeaderValue() string { return string(p) }

// CallerAttribution carries opt-in AI-mediated-provenance metadata
// emitted as an HTTP header and OTel attributes (REQ-066).
//
// Construct one and attach via WithCallerAttribution (client default)
// or WithCallerAttributionCtx (per-request). PII MUST NOT be placed in
// Attributes — user identity flows through the auth path, not here.
type CallerAttribution struct {
	// AgentID identifies the agent surface emitting the request,
	// e.g. "mcp-claude-code/1.2.0".
	AgentID string
	// ModelProvider names the upstream model vendor, e.g. "anthropic".
	ModelProvider string
	// Attributes carries deployment-specific opaque attributes.
	Attributes map[string]string
}

// HeaderJSON returns the JSON-encoded header value for a non-empty
// attribution; returns "" when the value is effectively empty.
func (a CallerAttribution) HeaderJSON() string {
	if a.AgentID == "" && a.ModelProvider == "" && len(a.Attributes) == 0 {
		return ""
	}
	type wire struct {
		AgentID       string            `json:"agent_id,omitempty"`
		ModelProvider string            `json:"model_provider,omitempty"`
		Attributes    map[string]string `json:"attributes,omitempty"`
	}
	b, err := json.Marshal(wire(a))
	if err != nil {
		return ""
	}
	return string(b)
}

// IsEmpty reports whether a has no attribution data.
func (a CallerAttribution) IsEmpty() bool {
	return a.AgentID == "" && a.ModelProvider == "" && len(a.Attributes) == 0
}

type callerAttributionCtxKey struct{}

// WithCallerAttributionCtx attaches a per-request CallerAttribution to
// ctx. transport.Client prefers the per-request value over the
// client-default when present. Empty attributions clear the field.
func WithCallerAttributionCtx(ctx context.Context, a CallerAttribution) context.Context {
	return context.WithValue(ctx, callerAttributionCtxKey{}, a)
}

// CallerAttributionFromContext returns the per-request value attached
// via WithCallerAttributionCtx, or (zero, false) when absent.
func CallerAttributionFromContext(ctx context.Context) (CallerAttribution, bool) {
	a, ok := ctx.Value(callerAttributionCtxKey{}).(CallerAttribution)
	return a, ok
}

// quoteIfMatch wraps an unquoted ETag value with the canonical double
// quotes the HTTP grammar requires. ETag and If-Match values are
// quoted strong validators per RFC 9110 § 8.8.3; bare hex values
// without quotes are non-conforming on many backends.
func quoteIfMatch(v string) string {
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
		return v
	}
	return `"` + v + `"`
}

// unquoteETag strips the canonical quotes from an ETag header value
// captured on a response so callers can round-trip it as an If-Match
// without double-quoting. Tolerates W/-prefixed weak validators.
func unquoteETag(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "W/")
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}

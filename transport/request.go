package transport

import (
	"net/http"
	"net/url"
	"strings"
)

// Request describes one outgoing openEHR REST request. Leaf clients
// construct Request values and pass them to Client.Do — the transport
// resolves the service base URL, plumbs the openehr-* and auth
// headers, and applies the retry / OTel envelope.
//
// Request is a value type; do not share one across goroutines while
// mutating it.
type Request struct {
	// Method is the HTTP method (GET / POST / PUT / DELETE / HEAD /
	// OPTIONS / PATCH).
	Method string
	// ServiceID is the catalog identifier this request targets;
	// defaults to "org.openehr.rest" when empty (the most common case
	// for leaf clients in openehr/client/*).
	ServiceID string
	// Path is the path segment appended to the resolved service
	// base URL. MUST begin with "/". Path parameters are caller-
	// substituted; the transport does not perform path templating.
	//
	// Path is a DECODED path (net/url [url.URL.Path] semantics): the
	// transport is the single canonical path encoder and percent-encodes
	// it exactly once via [url.URL.String]. Callers MUST interpolate raw,
	// decoded path parameters (e.g. a template id `Referral Request.v1`)
	// and MUST NOT pre-escape them with [url.PathEscape] — doing so
	// double-encodes (` ` → `%20` → `%2520`) and 404s.
	//
	// Segment legality is a separate question from encoding: the transport
	// validates every decoded segment before building the URL and refuses
	// a traversal, empty, backslash-bearing, or control-character segment
	// with [ErrInvalidPathSegment] (REQ-150). The SDK does not assume
	// openEHR ids carry no `/` — that assumption is exactly what a hostile
	// id exploits. Use [ValidatePathSegment] to preflight one interpolated
	// parameter.
	Path string
	// Route is the path template used for OTel span naming and error
	// attribution (e.g. "/ehr/{ehr_id}/composition"). When empty the
	// transport falls back to Path for those.
	//
	// Optional for a raw transport.Do caller, but REQUIRED of any request
	// that interpolates a parameter into Path: the REQ-150 arity check
	// that catches a separator smuggled inside one parameter runs only
	// when Route is set, so leaving it empty silently disables that
	// defence. A tripwire test in openehr/client holds every leaf to it.
	Route string
	// Query is appended to the resolved URL.
	Query url.Values
	// Headers carries extra caller-supplied headers — merged after
	// the transport's standard plumbing so callers can override.
	Headers http.Header
	// Body is the pre-marshalled request body. The codec choice is the
	// caller's responsibility; the transport sets Content-Type from
	// ContentType (default "application/json").
	Body []byte
	// ContentType, when non-empty, sets the Content-Type header. The
	// default is application/json (REQ-052 — canonical JSON).
	ContentType string
	// Accept, when non-empty, sets the Accept header. Default
	// "application/json".
	Accept string
	// IfMatch sets the If-Match header (REQ-054). The value is
	// canonicalised to a quoted strong validator if not already
	// quoted.
	IfMatch string
	// Prefer sets the Prefer header (REQ-094). PreferDefault omits the
	// header.
	Prefer Prefer
	// AuditDetailsHeader sets the openehr-audit-details header. The
	// caller pre-encodes their *rm.AuditDetails to the openEHR
	// dotted-attribute header grammar (via ehr.MarshalAuditDetails) —
	// NOT JSON; the transport does not allocate codecs. Empty omits the
	// header.
	AuditDetailsHeader string
	// RMVersion sets the openehr-version header (REQ-059). It conveys the
	// committed VERSION's lifecycle_state as the dotted-attribute value
	// `lifecycle_state.code_string="<code>"` (via
	// ehr.FormatLifecycleStateHeader), not an RM spec version. Empty omits
	// the header.
	RMVersion string
	// TemplateID sets the openehr-template-id header (REQ-059). Empty
	// omits the header.
	TemplateID string
	// URI sets the openehr-uri header (REQ-059). Empty omits.
	URI string
	// ItemTag sets the openehr-item-tag header (REQ-059). Empty omits.
	ItemTag string
	// VersionItemTag sets the openehr-version-item-tag header (REQ-059).
	// Empty omits.
	VersionItemTag string
	// NoAuth suppresses the Authorization header for this request
	// even when a TokenSource is configured. Used for endpoints that
	// reject bearer tokens (typically capabilities probes).
	NoAuth bool
}

// effectiveRoute returns Route, falling back to Path. Used for OTel
// span naming and the REQ-098 Observation.Route — telemetry surfaces, which
// may legitimately carry the resolved path. Human-readable error strings use
// routeOrPlaceholder instead (REQ-093): see that doc comment for why the two
// must not share a fallback.
func (r *Request) effectiveRoute() string {
	if r.Route != "" {
		return r.Route
	}
	return r.Path
}

// unroutedRoute is the stable, value-free placeholder substituted for the
// route template in diagnostic strings when the caller left Request.Route
// unset. Its exact text is normative — transport.md § REQ-093, the
// "Unrouted requests render a placeholder" clause — not an implementation
// detail free to change: a caller may already match against it.
const unroutedRoute = "(unrouted)"

// routeOrPlaceholder returns Route, or unroutedRoute when Route is unset.
// Unlike effectiveRoute, it never falls back to Path: Path may carry a
// caller-supplied identifier (an EHR id, a composition uid, …), and REQ-093
// requires transport error strings — doOnce's "transport: %s %s: …"
// diagnostics, and the Route captured on WireError and DecodeError — stay
// value-free. Every human-readable error surface in this package uses this
// method instead of effectiveRoute.
func (r *Request) routeOrPlaceholder() string {
	if r.Route != "" {
		return r.Route
	}
	return unroutedRoute
}

func (r *Request) effectiveServiceID() string {
	if r.ServiceID != "" {
		return r.ServiceID
	}
	return "org.openehr.rest"
}

func (r *Request) effectiveContentType() string {
	if r.ContentType != "" {
		return r.ContentType
	}
	return "application/json"
}

func (r *Request) effectiveAccept() string {
	if r.Accept != "" {
		return r.Accept
	}
	return "application/json"
}

func (r *Request) effectiveMethod() string {
	if r.Method == "" {
		return "GET"
	}
	return strings.ToUpper(r.Method)
}

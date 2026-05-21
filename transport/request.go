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
	Path string
	// Route is the optional path template used for OTel span naming
	// and error attribution (e.g. "/ehr/{ehr_id}/composition"). When
	// empty the transport falls back to Path.
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
	// caller pre-marshals their *rm.AuditDetails to canonical JSON;
	// the transport does not allocate codecs. Empty omits the header.
	AuditDetailsHeader string
	// RMVersion sets the openehr-version header (REQ-059). Empty
	// omits the header.
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
// span naming and error attribution.
func (r *Request) effectiveRoute() string {
	if r.Route != "" {
		return r.Route
	}
	return r.Path
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

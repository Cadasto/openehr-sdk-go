package transport

import (
	"net/http"
	"strings"
	"time"
)

// Response is the captured HTTP response: the body is fully consumed and
// the headers are surfaced in typed form on Metadata.
type Response struct {
	// StatusCode is the HTTP status from the wire.
	StatusCode int
	// Header is the raw response header map. Header is provided in
	// addition to Metadata so callers can inspect deployment-specific
	// headers without SDK churn.
	Header http.Header
	// Body is the raw response body, fully read.
	Body []byte
	// Metadata carries the openEHR-typed header subset.
	Metadata *Metadata
}

// Metadata extracts the headers leaf clients consume most often,
// parsed into typed values.
type Metadata struct {
	// ETag is the first response ETag, with surrounding quotes stripped
	// so the value round-trips into a future If-Match without double
	// quoting. Empty when the response carried no ETag. It stays that
	// first value when a later ETag is the one used as a version id.
	ETag string
	// ETags holds every ETag from the response, with surrounding quotes
	// stripped, in the order the response sent them. ETag is the first
	// entry when there is one. Empty when the response carried no ETag.
	ETags []string
	// Location captures the response Location header verbatim.
	Location string
	// LastModified is the parsed HTTP Last-Modified header. Zero
	// when missing or unparseable.
	LastModified time.Time
	// RMVersion captures the response openehr-version header.
	RMVersion string
	// AuditDetails captures the response openehr-audit-details header
	// verbatim. It is typically present on Contribution responses.
	AuditDetails string
	// URI captures the response openehr-uri header.
	URI string
	// ItemTag captures the response openehr-item-tag header.
	ItemTag string
	// VersionItemTag captures the response openehr-version-item-tag
	// header.
	VersionItemTag string
	// TemplateID captures the response openehr-template-id header,
	// surfaced when a composition response advertises it.
	TemplateID string
	// CadastoSpecVersion captures the Cadasto-OpenEhr-Spec-Version
	// response header when present.
	CadastoSpecVersion string
}

// parseMetadata extracts Metadata from h. Tolerates absent / malformed
// headers — populated fields surface what the wire provided, missing
// fields stay zero.
func parseMetadata(h http.Header) *Metadata {
	etags := etagValues(h)
	var etag string
	if len(etags) > 0 {
		etag = etags[0]
	}
	m := &Metadata{
		ETag:               etag,
		ETags:              etags,
		Location:           h.Get("Location"),
		RMVersion:          h.Get("openehr-version"),
		AuditDetails:       h.Get("openehr-audit-details"),
		URI:                h.Get("openehr-uri"),
		ItemTag:            joinHeaderField(h, "openehr-item-tag"),
		VersionItemTag:     joinHeaderField(h, "openehr-version-item-tag"),
		TemplateID:         h.Get("openehr-template-id"),
		CadastoSpecVersion: h.Get("Cadasto-OpenEhr-Spec-Version"),
	}
	if lm := h.Get("Last-Modified"); lm != "" {
		if t, err := http.ParseTime(lm); err == nil {
			m.LastModified = t
		}
	}
	return m
}

// etagValues returns every ETag header value, quotes and a weak
// prefix stripped, in wire order. The slice is a copy. Nil when the
// response carried no ETag.
func etagValues(h http.Header) []string {
	raw := h.Values("ETag")
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, len(raw))
	for i, v := range raw {
		out[i] = unquoteETag(v)
	}
	return out
}

// joinHeaderField returns one logical openEHR header value. When a
// server emits repeated header lines (one tag per line), they are
// joined with "; " before leaf clients parse the semicolon-separated
// ITS-REST item-tag shape (REQ-059).
func joinHeaderField(h http.Header, name string) string {
	lines := h.Values(name)
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines, "; "))
}

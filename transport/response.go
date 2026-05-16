package transport

import (
	"net/http"
	"time"
)

// Response is the captured HTTP response — body fully consumed, headers
// surfaced in typed form on Metadata.
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
// parsed into typed values per [specs/wire.md REQ-054, REQ-059].
type Metadata struct {
	// ETag is the response ETag, with surrounding quotes stripped so
	// the value round-trips into a future If-Match without double
	// quoting. Empty when the response carried no ETag.
	ETag string
	// Location captures the response Location header verbatim.
	Location string
	// LastModified is the parsed HTTP Last-Modified header. Zero
	// when missing or unparseable.
	LastModified time.Time
	// RMVersion captures the response openehr-version header (REQ-059).
	RMVersion string
	// AuditDetails captures the response openehr-audit-details header
	// verbatim — typically present on Contribution responses.
	AuditDetails string
	// URI captures the response openehr-uri header (REQ-059).
	URI string
	// ItemTag captures the response openehr-item-tag header (REQ-059).
	ItemTag string
	// TemplateID captures the response openehr-template-id header
	// (REQ-059), surfaced when a composition response advertises it.
	TemplateID string
	// CadastoSpecVersion captures the Cadasto-OpenEhr-Spec-Version
	// response header (REQ-051) when present.
	CadastoSpecVersion string
}

// parseMetadata extracts Metadata from h. Tolerates absent / malformed
// headers — populated fields surface what the wire provided, missing
// fields stay zero.
func parseMetadata(h http.Header) *Metadata {
	m := &Metadata{
		ETag:               unquoteETag(h.Get("ETag")),
		Location:           h.Get("Location"),
		RMVersion:          h.Get("openehr-version"),
		AuditDetails:       h.Get("openehr-audit-details"),
		URI:                h.Get("openehr-uri"),
		ItemTag:            h.Get("openehr-item-tag"),
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

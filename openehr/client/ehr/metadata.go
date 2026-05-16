package ehr

import (
	"github.com/cadasto/openehr-sdk-go/transport"
)

// VersionMetadata is the typed response metadata every versioned-
// resource GET in `openehr/client/ehr/*` returns alongside the
// decoded body. It embeds the transport-level metadata (ETag,
// Location, LastModified, openehr-* response headers) and adds the
// parsed VersionUID extracted from the response.
//
// VersionUID is preferred from the Location header (which the server
// canonically sets to the resource path) and falls back to body
// inspection in leaf clients where applicable.
type VersionMetadata struct {
	*transport.Metadata
	// VersionUID is the parsed identifier for the returned version,
	// empty when the resource is not versioned (the EHR root) or when
	// the response provided no Location header.
	VersionUID VersionUID
}

// NewVersionMetadata builds a VersionMetadata by extracting the
// VersionUID from the transport metadata's Location header. Returns
// nil if m itself is nil so caller error paths propagate naturally.
//
// Exported so sub-leaf packages (`openehr/client/ehr/composition`,
// `.../ehrstatus`, `.../directory`) can adopt the same parsing rule
// without duplicating logic.
func NewVersionMetadata(m *transport.Metadata) *VersionMetadata {
	if m == nil {
		return nil
	}
	return &VersionMetadata{
		Metadata:   m,
		VersionUID: extractVersionUIDFromLocation(m.Location),
	}
}

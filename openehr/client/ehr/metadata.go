package ehr

import (
	"github.com/cadasto/openehr-sdk-go/transport"
)

// VersionMetadata is the typed response metadata every
// versioned-resource GET in `openehr/client/ehr/*` returns alongside the
// decoded body. It embeds the transport-level metadata (ETag,
// Location, LastModified, openehr-* response headers) and adds the
// parsed VersionUID extracted from the response.
//
// VersionUID comes from the first ETag value that holds a well-formed
// version id, and from the last segment of the Location header
// otherwise. One ETag keeps the same choice as reading that header
// alone. Leaf clients may also fill VersionUID from the response body
// where applicable. The ETag values come first because openEHR REST
// names an ETag as an identifier for a version, sends Location only
// on a create, and some servers leave the version out of Location.
// On an error return, VersionUID still comes from that response's
// headers, so a 409 or 412 can name the server's current version
// rather than a version this call wrote.
type VersionMetadata struct {
	*transport.Metadata
	// VersionUID is the parsed identifier for the returned version,
	// empty when the response named none. It is the first well-formed
	// version id among the ETag values, or the last segment of
	// Location when none of them is. For the EHR root it is the ehr_id
	// from Location when that header is present, and empty otherwise.
	VersionUID VersionUID
}

// NewVersionMetadata builds a VersionMetadata for a versioned resource.
// VersionUID is the first ETag value that is a well-formed version id
// (object_id::creating_system_id::version_tree_id), and the last path
// segment of Location otherwise. A single ETag keeps that same choice.
// Returns nil if m itself is nil so caller error paths propagate
// naturally.
//
// Exported so sub-leaf packages (`openehr/client/ehr/composition`,
// `.../ehrstatus`, `.../directory`) can adopt the same parsing rule
// without duplicating logic.
func NewVersionMetadata(m *transport.Metadata) *VersionMetadata {
	if m == nil {
		return nil
	}
	uid := firstVersionUID(m)
	if uid == "" {
		uid = extractVersionUIDFromLocation(m.Location)
	}
	return &VersionMetadata{Metadata: m, VersionUID: uid}
}

// firstVersionUID returns the first ETag value that is a well-formed
// object_version_id (REQ-054). A value built with only ETag set, and
// no ETags slice, is that one header.
func firstVersionUID(m *transport.Metadata) VersionUID {
	values := m.ETags
	if len(values) == 0 {
		if m.ETag == "" {
			return ""
		}
		values = []string{m.ETag}
	}
	for _, v := range values {
		if uid := versionUIDFromETag(v); uid != "" {
			return uid
		}
	}
	return ""
}

// newEHRMetadata builds the metadata of an EHR root response.
// VersionUID is the ehr_id from Location when the response carries
// one, and empty when it does not. The ETag is not read: it names
// the ehr_id, not a version.
func newEHRMetadata(m *transport.Metadata) *VersionMetadata {
	if m == nil {
		return nil
	}
	return &VersionMetadata{Metadata: m, VersionUID: extractVersionUIDFromLocation(m.Location)}
}

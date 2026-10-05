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
// VersionUID comes from the ETag header when it holds a version id,
// and from the last segment of the Location header otherwise; leaf
// clients may also fill it from the response body where applicable.
// The ETag comes first because openEHR REST names the ETag as the
// version id, sends Location only on a create, and some servers leave
// the version out of Location. On an error return, VersionUID still
// comes from that response's headers, so a 409 or 412 can name the
// server's current version rather than a version this call wrote.
type VersionMetadata struct {
	*transport.Metadata
	// VersionUID is the parsed identifier for the returned version,
	// empty when the response named none in its ETag or Location
	// header. For the EHR root it is the ehr_id from Location.
	VersionUID VersionUID
}

// NewVersionMetadata builds a VersionMetadata for a versioned resource.
// VersionUID is the ETag when the ETag is a well-formed version id
// (object_id::creating_system_id::version_tree_id), and the last path
// segment of Location otherwise. Returns nil if m itself is nil so
// caller error paths propagate naturally.
//
// Exported so sub-leaf packages (`openehr/client/ehr/composition`,
// `.../ehrstatus`, `.../directory`) can adopt the same parsing rule
// without duplicating logic.
func NewVersionMetadata(m *transport.Metadata) *VersionMetadata {
	if m == nil {
		return nil
	}
	uid := versionUIDFromETag(m.ETag)
	if uid == "" {
		uid = extractVersionUIDFromLocation(m.Location)
	}
	return &VersionMetadata{Metadata: m, VersionUID: uid}
}

// newEHRMetadata builds the metadata of an EHR root response, whose
// VersionUID is the ehr_id taken from Location. It never reads the
// ETag: openEHR REST makes an EHR's ETag its ehr_id, but some servers
// send the EHR_STATUS version id there, which is not the EHR's id.
func newEHRMetadata(m *transport.Metadata) *VersionMetadata {
	if m == nil {
		return nil
	}
	return &VersionMetadata{Metadata: m, VersionUID: extractVersionUIDFromLocation(m.Location)}
}

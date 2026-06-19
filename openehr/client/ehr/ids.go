package ehr

import (
	"net/url"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// EHRID identifies an EHR root. Distinct type from the openEHR
// HierObjectID it wraps so call sites cannot accidentally mix it with
// VersionedObjectID / VersionUID. The string form is the UID value
// (typically a UUID) — not the full HierObjectID JSON envelope.
type EHRID string

// String returns the EHRID as a plain string for URL-path composition.
func (e EHRID) String() string { return string(e) }

// VersionedObjectID identifies a versioned-resource family (e.g. a
// Composition's "versioned_object_id"). Using one in a GET resolves to
// the latest version per the openEHR REST contract.
type VersionedObjectID string

// String returns the VersionedObjectID as a plain string.
func (v VersionedObjectID) String() string { return string(v) }

// VersionUID identifies a specific version of a versioned resource.
// Wire form: <object_id>::<creating_system_id>::<version_tree_id> per the
// openEHR ObjectVersionID grammar.
type VersionUID string

// String returns the VersionUID as a plain string.
func (v VersionUID) String() string { return string(v) }

// VersionedObjectID returns the versioned-object family this VersionUID
// belongs to — the object_id segment of the underlying ObjectVersionID.
// Empty when the VersionUID is not a well-formed object_version_id.
//
// Delegates to the canonical parser [rm.ParseObjectVersionID] so the
// "::"-splitting lexical logic has a single home (REQ-120).
func (v VersionUID) VersionedObjectID() VersionedObjectID {
	ovID, err := rm.ParseObjectVersionID(string(v))
	if err != nil {
		return ""
	}
	return VersionedObjectID(rm.UIDValue(ovID.ObjectID()))
}

// CreatingSystemID returns the creating-system segment of the
// VersionUID. Empty when the VersionUID is not a well-formed
// object_version_id. Delegates to [rm.ParseObjectVersionID] (REQ-120).
func (v VersionUID) CreatingSystemID() string {
	ovID, err := rm.ParseObjectVersionID(string(v))
	if err != nil {
		return ""
	}
	return rm.UIDValue(ovID.CreatingSystemID())
}

// VersionNumber returns the version-tree segment of the VersionUID.
// Empty when the VersionUID is not a well-formed object_version_id.
// Delegates to [rm.ParseObjectVersionID] (REQ-120).
func (v VersionUID) VersionNumber() string {
	ovID, err := rm.ParseObjectVersionID(string(v))
	if err != nil {
		return ""
	}
	return ovID.VersionTreeID().Value
}

// extractVersionUIDFromLocation parses a Location header value to a
// VersionUID by taking the last path segment. Tolerates leading or
// trailing slashes. Returns empty when the header is absent or has no
// resource path tail.
//
// Wire shape: Location: /ehr/{ehr_id}/composition/{version_uid}.
// Mirrors the pattern the CDR benchmark uses to round-trip ETags into
// follow-up If-Match writes.
func extractVersionUIDFromLocation(loc string) VersionUID {
	if loc == "" {
		return ""
	}
	// Absolute Location values may carry scheme/host/query; use the
	// path tail only (RFC 9110 §7.2).
	if u, err := url.Parse(loc); err == nil && u.Path != "" {
		loc = u.Path
	}
	loc = strings.TrimSuffix(loc, "/")
	i := strings.LastIndex(loc, "/")
	if i < 0 || i == len(loc)-1 {
		return ""
	}
	return VersionUID(loc[i+1:])
}

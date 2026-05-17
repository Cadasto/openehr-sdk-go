package ehr

import (
	"net/url"
	"strings"
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
// Wire form: <uid>::<creating_system_id>::<version_number> per the
// openEHR ObjectVersionID grammar.
type VersionUID string

// String returns the VersionUID as a plain string.
func (v VersionUID) String() string { return string(v) }

// VersionedObjectID returns the versioned-object family this VersionUID
// belongs to — the substring before the first "::". Empty when the
// VersionUID does not contain the separator (best-effort).
func (v VersionUID) VersionedObjectID() VersionedObjectID {
	if voID, _, ok := strings.Cut(string(v), "::"); ok && voID != "" {
		return VersionedObjectID(voID)
	}
	return ""
}

// CreatingSystemID returns the creating-system segment of the
// VersionUID — the substring between the first and second "::". Empty
// when the VersionUID is malformed.
func (v VersionUID) CreatingSystemID() string {
	_, rest, ok := strings.Cut(string(v), "::")
	if !ok {
		return ""
	}
	sys, _, _ := strings.Cut(rest, "::")
	return sys
}

// VersionNumber returns the version-number segment — the substring
// after the second "::". Empty when malformed.
func (v VersionUID) VersionNumber() string {
	_, rest, ok := strings.Cut(string(v), "::")
	if !ok {
		return ""
	}
	_, num, _ := strings.Cut(rest, "::")
	return num
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

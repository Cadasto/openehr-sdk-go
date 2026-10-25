package ehr

import "time"

// Ref identifies the target of a versioned-resource GET. The two
// concrete implementations are [ByVersionedObjectID] (returns the
// latest version of the named family) and [ByVersionUID] (returns one
// specific version).
//
// Ref is a sealed interface: only this package defines implementations
// (the unexported isRef marker prevents external satisfaction), so
// leaf clients cannot accidentally accept arbitrary types from
// callers. Construct refs via [LatestOf], [LatestAtTime], or
// [VersionOf].
type Ref interface {
	isRef()
	// PathSegment returns the URL-path tail that addresses this Ref
	// under a versioned resource base (e.g. ".../composition/{seg}").
	// Used by leaf clients to compose request paths.
	PathSegment() string
	// Query returns any query parameters this Ref carries (notably the
	// version_at_time variant). Returns "", "" when none. Callers check
	// Validate first: a Ref that fails it has no value to send.
	Query() (key, value string)
	// Validate reports whether the Ref can be sent. A time whose UTC year
	// falls outside 0000-9999, which RFC 3339 cannot write, fails with
	// [transport.ErrInvalidConfig]. The SDK's reads call it before any
	// request.
	Validate() error
}

// ByVersionedObjectID addresses the latest version of a versioned
// resource family. Use with [LatestOf] for readability:
//
//	composition.Get(ctx, c, ehrID, ehr.LatestOf(voID))
type ByVersionedObjectID struct {
	VOID VersionedObjectID
	// AtTime optionally requests the version that was current at the
	// given instant, sent in UTC with its fraction of a second. Zero
	// time omits the version_at_time query param and returns the
	// absolute latest.
	AtTime time.Time
}

func (r ByVersionedObjectID) isRef() {}

// PathSegment implements Ref, yielding the versioned-object id.
func (r ByVersionedObjectID) PathSegment() string { return string(r.VOID) }

// Query implements Ref, adding version_at_time when AtTime is set,
// written by [FormatVersionAtTime]. The value is "" for a time that
// [ByVersionedObjectID.Validate] refuses, so callers check Validate
// first, as the SDK's reads do.
func (r ByVersionedObjectID) Query() (string, string) {
	if r.AtTime.IsZero() {
		return "", ""
	}
	// Validate reports the formatter's error; Query only carries the value,
	// which is "" when there is an error.
	v, _ := FormatVersionAtTime(r.AtTime)
	return "version_at_time", v
}

// Validate implements Ref. A zero AtTime is valid; a set AtTime whose UTC
// year falls outside 0000-9999 fails with [transport.ErrInvalidConfig],
// as [FormatVersionAtTime] does.
func (r ByVersionedObjectID) Validate() error {
	if r.AtTime.IsZero() {
		return nil
	}
	_, err := FormatVersionAtTime(r.AtTime)
	return err
}

// ByVersionUID addresses one specific version of a versioned resource.
// Use with [VersionOf]:
//
//	composition.Get(ctx, c, ehrID, ehr.VersionOf(uid))
type ByVersionUID struct {
	UID VersionUID
}

func (r ByVersionUID) isRef() {}

// PathSegment implements Ref, yielding the version uid.
func (r ByVersionUID) PathSegment() string { return string(r.UID) }

// Query implements Ref; a version uid needs no query parameter.
func (r ByVersionUID) Query() (string, string) { return "", "" }

// Validate implements Ref; a version uid carries nothing to refuse.
func (r ByVersionUID) Validate() error { return nil }

// LatestOf returns a Ref addressing the latest version of voID.
func LatestOf(voID VersionedObjectID) Ref { return ByVersionedObjectID{VOID: voID} }

// LatestAtTime returns a Ref addressing the version of voID that was
// current at t. The time is sent in UTC with its fraction of a second.
func LatestAtTime(voID VersionedObjectID, t time.Time) Ref {
	return ByVersionedObjectID{VOID: voID, AtTime: t}
}

// VersionOf returns a Ref addressing one specific VersionUID.
func VersionOf(uid VersionUID) Ref { return ByVersionUID{UID: uid} }

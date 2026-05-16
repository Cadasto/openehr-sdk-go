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
	// version_at_time variant). Returns "", "" when none.
	Query() (key, value string)
}

// ByVersionedObjectID addresses the latest version of a versioned
// resource family. Use with [LatestOf] for readability:
//
//	composition.Get(ctx, c, ehrID, ehr.LatestOf(voID))
type ByVersionedObjectID struct {
	VOID VersionedObjectID
	// AtTime optionally requests the version that was current at the
	// given instant. Zero time omits the version_at_time query param
	// and returns the absolute latest.
	AtTime time.Time
}

func (r ByVersionedObjectID) isRef()              {}
func (r ByVersionedObjectID) PathSegment() string { return string(r.VOID) }
func (r ByVersionedObjectID) Query() (string, string) {
	if r.AtTime.IsZero() {
		return "", ""
	}
	return "version_at_time", r.AtTime.UTC().Format(time.RFC3339)
}

// ByVersionUID addresses one specific version of a versioned resource.
// Use with [VersionOf]:
//
//	composition.Get(ctx, c, ehrID, ehr.VersionOf(uid))
type ByVersionUID struct {
	UID VersionUID
}

func (r ByVersionUID) isRef()                  {}
func (r ByVersionUID) PathSegment() string     { return string(r.UID) }
func (r ByVersionUID) Query() (string, string) { return "", "" }

// LatestOf returns a Ref addressing the latest version of voID.
func LatestOf(voID VersionedObjectID) Ref { return ByVersionedObjectID{VOID: voID} }

// LatestAtTime returns a Ref addressing the version of voID that was
// current at t.
func LatestAtTime(voID VersionedObjectID, t time.Time) Ref {
	return ByVersionedObjectID{VOID: voID, AtTime: t}
}

// VersionOf returns a Ref addressing one specific VersionUID.
func VersionOf(uid VersionUID) Ref { return ByVersionUID{UID: uid} }

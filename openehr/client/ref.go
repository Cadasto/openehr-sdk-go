package client

import (
	"fmt"
	"time"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// Ref identifies the target of a versioned-resource GET. The two
// concrete implementations are [ByVersionedObjectID] (returns the
// latest version of the named family) and [ByVersionUID] (returns one
// specific version).
//
// Ref is a sealed interface: the unexported isRef marker keeps other
// packages from declaring their own implementations. A caller type that
// embeds one of the two still satisfies it, and [ValidateRef] refuses
// such a type. Construct refs via [LatestOf], [LatestAtTime], or
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
	// [transport.ErrInvalidConfig]. The SDK's reads refuse a Ref that fails
	// it before any request.
	Validate() error
}

// ByVersionedObjectID addresses the latest version of a versioned
// resource family. Use with [LatestOf] for readability:
//
//	composition.Get(ctx, c, ehrID, client.LatestOf(voID))
type ByVersionedObjectID struct {
	VOID VersionedObjectID
	// AtTime optionally requests the version that was current at the
	// given instant, sent in UTC with its fraction of a second. Zero
	// time omits the version_at_time query param and returns the
	// absolute latest. A time whose UTC year falls outside 0000-9999
	// is refused by [ByVersionedObjectID.Validate] and by the reads
	// that take the Ref.
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
//	composition.Get(ctx, c, ehrID, client.VersionOf(uid))
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
// current at t. The time is sent in UTC with its fraction of a second; a
// t whose UTC year falls outside 0000-9999 is refused before any request.
func LatestAtTime(voID VersionedObjectID, t time.Time) Ref {
	return ByVersionedObjectID{VOID: voID, AtTime: t}
}

// VersionOf returns a Ref addressing one specific VersionUID.
func VersionOf(uid VersionUID) Ref { return ByVersionUID{UID: uid} }

// ValidateRef reports whether r names a target a read can send. It fails
// with an error wrapping [transport.ErrInvalidConfig] when r is nil, holds
// a nil *[ByVersionUID] or *[ByVersionedObjectID], is of any other type,
// such as a caller struct embedding one of them, or has an empty
// [Ref.PathSegment]; otherwise it returns what r.Validate returns. It
// never panics. A read whose route carries the Ref's path segment calls it
// before any request.
func ValidateRef(r Ref) error {
	// REQ-025 (idiom.md § No panics): a type embedding a nil pointer to one
	// of the two satisfies Ref through the promoted marker, and calling
	// its methods would dereference that pointer, so only the four forms
	// this package declares pass.
	switch v := r.(type) {
	case nil:
		return fmt.Errorf("%w: nil Ref", transport.ErrInvalidConfig)
	case ByVersionUID, ByVersionedObjectID:
	case *ByVersionUID:
		if v == nil {
			return fmt.Errorf("%w: nil Ref", transport.ErrInvalidConfig)
		}
	case *ByVersionedObjectID:
		if v == nil {
			return fmt.Errorf("%w: nil Ref", transport.ErrInvalidConfig)
		}
	default:
		return fmt.Errorf("%w: unsupported Ref type %T", transport.ErrInvalidConfig, r)
	}
	if r.PathSegment() == "" {
		return fmt.Errorf("%w: empty Ref path segment", transport.ErrInvalidConfig)
	}
	return r.Validate()
}

package ehr

import (
	"context"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// The names below moved to package client, which every resource client
// shares. Each one here is the same type, constant or behaviour, so code
// written against this package keeps compiling.

// VersionedObjectID identifies a versioned-resource family.
//
// Deprecated: use [client.VersionedObjectID] instead.
type VersionedObjectID = client.VersionedObjectID

// VersionUID identifies a specific version of a versioned resource.
//
// Deprecated: use [client.VersionUID] instead.
type VersionUID = client.VersionUID

// VersionMetadata is the typed response metadata of a versioned resource.
//
// Deprecated: use [client.VersionMetadata] instead.
type VersionMetadata = client.VersionMetadata

// Identifier is the ITS-REST Identifier write-response body.
//
// Deprecated: use [client.Identifier] instead.
type Identifier = client.Identifier

// Ref identifies the target of a versioned-resource GET.
//
// Deprecated: use [client.Ref] instead.
type Ref = client.Ref

// ByVersionedObjectID addresses the latest version of a versioned resource
// family.
//
// Deprecated: use [client.ByVersionedObjectID] instead.
type ByVersionedObjectID = client.ByVersionedObjectID

// ByVersionUID addresses one specific version of a versioned resource.
//
// Deprecated: use [client.ByVersionUID] instead.
type ByVersionUID = client.ByVersionUID

// ItemTag is a parsed ITEM_TAG from an item-tag header value.
//
// Deprecated: use [client.ItemTag] instead.
type ItemTag = client.ItemTag

// StoredItemTag is an ITEM_TAG as a server stores it.
//
// Deprecated: use [client.StoredItemTag] instead.
type StoredItemTag = client.StoredItemTag

// LifecycleState is an openEHR version lifecycle state code.
//
// Deprecated: use [client.LifecycleState] instead.
type LifecycleState = client.LifecycleState

// The openEHR version lifecycle state codes.
const (
	// LifecycleStateComplete is code 532 ("complete").
	//
	// Deprecated: use [client.LifecycleStateComplete] instead.
	LifecycleStateComplete = client.LifecycleStateComplete
	// LifecycleStateIncomplete is code 553 ("incomplete").
	//
	// Deprecated: use [client.LifecycleStateIncomplete] instead.
	LifecycleStateIncomplete = client.LifecycleStateIncomplete
	// LifecycleStateDeleted is code 523 ("deleted").
	//
	// Deprecated: use [client.LifecycleStateDeleted] instead.
	LifecycleStateDeleted = client.LifecycleStateDeleted
	// LifecycleStateInactive is code 800 ("inactive").
	//
	// Deprecated: use [client.LifecycleStateInactive] instead.
	LifecycleStateInactive = client.LifecycleStateInactive
	// LifecycleStateAbandoned is code 801 ("abandoned").
	//
	// Deprecated: use [client.LifecycleStateAbandoned] instead.
	LifecycleStateAbandoned = client.LifecycleStateAbandoned
)

// WriteConfig is the option set shared by every versioned-write leaf client.
//
// Deprecated: use [client.WriteConfig] instead.
type WriteConfig = client.WriteConfig

// NoRepresentationError reports a committed write whose representation body
// was empty or could not be decoded.
//
// Deprecated: use [client.NoRepresentationError] instead.
type NoRepresentationError = client.NoRepresentationError

// LatestOf returns a Ref addressing the latest version of voID.
//
// Deprecated: use [client.LatestOf] instead.
func LatestOf(voID client.VersionedObjectID) client.Ref { return client.LatestOf(voID) }

// LatestAtTime returns a Ref addressing the version of voID that was current
// at t.
//
// Deprecated: use [client.LatestAtTime] instead.
func LatestAtTime(voID client.VersionedObjectID, t time.Time) client.Ref {
	return client.LatestAtTime(voID, t)
}

// VersionOf returns a Ref addressing one specific VersionUID.
//
// Deprecated: use [client.VersionOf] instead.
func VersionOf(uid client.VersionUID) client.Ref { return client.VersionOf(uid) }

// NewVersionMetadata builds a VersionMetadata for a versioned resource.
//
// Deprecated: use [client.NewVersionMetadata] instead.
func NewVersionMetadata(m *transport.Metadata) *client.VersionMetadata {
	return client.NewVersionMetadata(m)
}

// HasResource reports whether a write returned a usable resource.
//
// Deprecated: use [client.HasResource] instead.
func HasResource[T any](v T) bool { return client.HasResource(v) }

// MarshalAuditDetails encodes an AUDIT_DETAILS into the
// openehr-audit-details request-header value.
//
// Deprecated: use [client.MarshalAuditDetails] instead.
func MarshalAuditDetails(a *rm.AuditDetails) (string, error) { return client.MarshalAuditDetails(a) }

// FormatItemTagHeader encodes tags into the item-tag header shape.
//
// Deprecated: use [client.FormatItemTagHeader] instead.
func FormatItemTagHeader(tags []client.ItemTag) (string, error) {
	return client.FormatItemTagHeader(tags)
}

// ParseItemTagHeader decodes an item-tag header value.
//
// Deprecated: use [client.ParseItemTagHeader] instead.
func ParseItemTagHeader(header string) ([]client.ItemTag, error) {
	return client.ParseItemTagHeader(header)
}

// FormatVersionAtTime returns t as the value of a version_at_time query
// parameter.
//
// Deprecated: use [client.FormatVersionAtTime] instead.
func FormatVersionAtTime(t time.Time) (string, error) { return client.FormatVersionAtTime(t) }

// FormatLifecycleStateHeader encodes a lifecycle_state code into the
// openehr-version request-header value.
//
// Deprecated: use [client.FormatLifecycleStateHeader] instead.
func FormatLifecycleStateHeader(s client.LifecycleState) (string, error) {
	return client.FormatLifecycleStateHeader(s)
}

// WriteResult executes a versioned write and decodes its response per the
// Prefer value.
//
// Deprecated: use [client.WriteResult] instead.
func WriteResult[T any](ctx context.Context, c *transport.Client, req *transport.Request, label string, decode func([]byte) (T, error)) (T, *client.VersionMetadata, error) {
	return client.WriteResult(ctx, c, req, label, decode)
}

// DoDelete issues a logical-delete request and returns the version metadata.
//
// Deprecated: use [client.DoDelete] instead.
func DoDelete(ctx context.Context, c *transport.Client, req *transport.Request) (*client.VersionMetadata, error) {
	return client.DoDelete(ctx, c, req)
}

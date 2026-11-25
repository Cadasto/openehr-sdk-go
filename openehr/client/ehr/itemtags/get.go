package itemtags

import (
	"context"
	"fmt"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/directory"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Tags holds ITEM_TAG lists returned from response headers.
type Tags struct {
	// Object tags from openehr-item-tag (VERSIONED_OBJECT scope).
	Object []Tag
	// Version tags from openehr-version-item-tag (VERSION scope).
	Version []Tag
}

// GetComposition reads ITEM_TAG headers from a Composition GET.
func GetComposition(ctx context.Context, c *transport.Client, ehrID ehr.EHRID, ref openehrclient.Ref) (Tags, *openehrclient.VersionMetadata, error) {
	_, meta, err := composition.Get(ctx, c, ehrID, ref)
	if err != nil {
		return Tags{}, meta, err
	}
	tags, err := tagsFromMetadata(meta.Metadata)
	return tags, meta, err
}

// GetEHRStatus reads ITEM_TAG headers from an EHR_STATUS GET. A Ref that
// [openehrclient.ValidateRef] refuses fails with [transport.ErrInvalidConfig]
// before any request.
func GetEHRStatus(ctx context.Context, c *transport.Client, ehrID ehr.EHRID, ref openehrclient.Ref) (Tags, *openehrclient.VersionMetadata, error) {
	if err := openehrclient.ValidateRef(ref); err != nil {
		return Tags{}, nil, fmt.Errorf("itemtags.GetEHRStatus: %w", err)
	}
	meta, err := getEHRStatusMeta(ctx, c, ehrID, ref)
	if err != nil {
		return Tags{}, meta, err
	}
	tags, err := tagsFromMetadata(meta.Metadata)
	return tags, meta, err
}

// GetDirectory reads ITEM_TAG headers from a Directory GET. A Ref that
// [openehrclient.ValidateRef] refuses fails with [transport.ErrInvalidConfig]
// before any request.
func GetDirectory(ctx context.Context, c *transport.Client, ehrID ehr.EHRID, ref openehrclient.Ref) (Tags, *openehrclient.VersionMetadata, error) {
	if err := openehrclient.ValidateRef(ref); err != nil {
		return Tags{}, nil, fmt.Errorf("itemtags.GetDirectory: %w", err)
	}
	meta, err := getDirectoryMeta(ctx, c, ehrID, ref)
	if err != nil {
		return Tags{}, meta, err
	}
	tags, err := tagsFromMetadata(meta.Metadata)
	return tags, meta, err
}

// refValue returns the Ref a non-nil pointer Ref points to, so a pointer
// reads the same target as its value, as it does in composition.Get.
func refValue(ref openehrclient.Ref) openehrclient.Ref {
	switch r := ref.(type) {
	case *openehrclient.ByVersionUID:
		if r != nil {
			return *r
		}
	case *openehrclient.ByVersionedObjectID:
		if r != nil {
			return *r
		}
	}
	return ref
}

func getDirectoryMeta(ctx context.Context, c *transport.Client, ehrID ehr.EHRID, ref openehrclient.Ref) (*openehrclient.VersionMetadata, error) {
	switch r := refValue(ref).(type) {
	case openehrclient.ByVersionUID:
		_, meta, err := directory.GetVersioned(ctx, c, ehrID, r.UID)
		return meta, err
	case openehrclient.ByVersionedObjectID:
		if r.AtTime.IsZero() {
			_, meta, err := directory.Get(ctx, c, ehrID)
			return meta, err
		}
		_, meta, err := directory.GetAtTime(ctx, c, ehrID, r.AtTime)
		return meta, err
	default:
		return nil, fmt.Errorf("itemtags: %w: unsupported Ref type", transport.ErrInvalidConfig)
	}
}

func getEHRStatusMeta(ctx context.Context, c *transport.Client, ehrID ehr.EHRID, ref openehrclient.Ref) (*openehrclient.VersionMetadata, error) {
	switch r := refValue(ref).(type) {
	case openehrclient.ByVersionUID:
		_, meta, err := ehrstatus.GetVersioned(ctx, c, ehrID, r.UID)
		return meta, err
	case openehrclient.ByVersionedObjectID:
		if r.AtTime.IsZero() {
			_, meta, err := ehrstatus.Get(ctx, c, ehrID)
			return meta, err
		}
		_, meta, err := ehrstatus.GetAtTime(ctx, c, ehrID, r.AtTime)
		return meta, err
	default:
		return nil, fmt.Errorf("itemtags: %w: unsupported Ref type", transport.ErrInvalidConfig)
	}
}

func tagsFromMetadata(m *transport.Metadata) (Tags, error) {
	if m == nil {
		return Tags{}, nil
	}
	obj, err := ParseHeader(m.ItemTag)
	if err != nil {
		return Tags{}, err
	}
	ver, err := ParseHeader(m.VersionItemTag)
	if err != nil {
		return Tags{}, err
	}
	return Tags{Object: obj, Version: ver}, nil
}

package itemtags

import (
	"context"
	"fmt"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// SetCompositionOptions carries ITEM_TAG headers for a Composition PUT.
type SetCompositionOptions struct {
	Object    []Tag
	Version   []Tag
	WriteOpts []composition.WriteOption
}

// SetComposition replaces ITEM_TAG headers on a Composition update (PUT).
// The composition body is sent unchanged aside from tag headers; ifMatch
// is required (REQ-054).
func SetComposition(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, ifMatch string, comp *rm.Composition, opt SetCompositionOptions) (*openehrclient.VersionMetadata, error) {
	if comp == nil {
		return nil, fmt.Errorf("itemtags.SetComposition: %w: nil Composition", transport.ErrInvalidConfig)
	}
	opts := append([]composition.WriteOption{}, opt.WriteOpts...)
	if h, err := FormatHeader(opt.Object); err != nil {
		return nil, err
	} else if h != "" {
		opts = append(opts, composition.WithObjectItemTags(h))
	}
	if h, err := FormatHeader(opt.Version); err != nil {
		return nil, err
	} else if h != "" {
		opts = append(opts, composition.WithVersionItemTags(h))
	}
	_, meta, err := composition.Update(ctx, c, ehrID, voID, ifMatch, comp, opts...)
	return meta, err
}

// DeleteComposition removes tags whose keys match key in the given scope
// and PUTs the composition back with the reduced tag set.
func DeleteComposition(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, ref openehrclient.Ref, ifMatch string, comp *rm.Composition, key string, objectScope bool) (*openehrclient.VersionMetadata, error) {
	if key == "" {
		return nil, fmt.Errorf("itemtags.DeleteComposition: %w: empty key", transport.ErrInvalidConfig)
	}
	cur, _, err := GetComposition(ctx, c, ehrID, ref)
	if err != nil {
		return nil, err
	}
	opt := SetCompositionOptions{WriteOpts: nil}
	if objectScope {
		opt.Object = removeKey(cur.Object, key)
	} else {
		opt.Version = removeKey(cur.Version, key)
	}
	return SetComposition(ctx, c, ehrID, voID, ifMatch, comp, opt)
}

func removeKey(tags []Tag, key string) []Tag {
	out := make([]Tag, 0, len(tags))
	for _, t := range tags {
		if t.Key != key {
			out = append(out, t)
		}
	}
	return out
}

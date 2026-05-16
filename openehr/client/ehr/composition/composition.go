package composition

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const routeTemplate = "/ehr/{ehr_id}/composition/{versioned_object_or_version_uid}"

// Get retrieves a Composition under the given EHR. The Ref selects
// the target: [ehr.LatestOf](voID) for the latest version of a
// versioned-object family, [ehr.LatestAtTime](voID, t) for the
// as-of-time variant, or [ehr.VersionOf](uid) for a specific version.
//
// Wire: GET /ehr/{ehr_id}/composition/{ref} [?version_at_time=...].
func Get(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, ref openehrclient.Ref) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("composition.Get: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if ref == nil {
		return nil, nil, fmt.Errorf("composition.Get: %w: nil Ref", transport.ErrInvalidConfig)
	}
	seg := ref.PathSegment()
	if seg == "" {
		return nil, nil, fmt.Errorf("composition.Get: %w: empty Ref path segment", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/ehr/" + url.PathEscape(string(ehrID)) + "/composition/" + url.PathEscape(seg),
		Route:  routeTemplate,
	}
	if qk, qv := ref.Query(); qk != "" {
		req.Query = url.Values{qk: []string{qv}}
	}
	out, meta, err := transport.Decode[rm.Composition](ctx, c, req)
	return out, openehrclient.NewVersionMetadata(meta), err
}

// Repository mirrors the package-level Composition functions.
type Repository interface {
	Get(ctx context.Context, ehrID openehrclient.EHRID, ref openehrclient.Ref) (*rm.Composition, *openehrclient.VersionMetadata, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) Get(ctx context.Context, ehrID openehrclient.EHRID, ref openehrclient.Ref) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	return Get(ctx, r.c, ehrID, ref)
}

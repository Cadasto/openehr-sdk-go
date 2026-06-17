package demographic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const versionedPartyBase = "/demographic/versioned_party"

// GetVersionedParty retrieves a PARTY via the VERSIONED_PARTY read family.
// voUID is the bare VERSIONED_OBJECT uuid (first segment of a version uid).
// When versionUID is empty, GET .../version returns the latest VERSION;
// otherwise GET .../version/{version_uid} returns that specific VERSION.
// The PARTY is extracted from the ORIGINAL_VERSION.data payload.
func GetVersionedParty(ctx context.Context, c *transport.Client, voUID, versionUID string) (rm.Party, *openehrclient.VersionMetadata, error) {
	if voUID == "" {
		return nil, nil, fmt.Errorf("demographic.GetVersionedParty: %w: empty versioned_object_uid", transport.ErrInvalidConfig)
	}
	path := versionedPartyBase + "/" + uidBasedIDPathSegment(voUID) + "/version"
	route := versionedPartyBase + "/{versioned_object_uid}/version"
	if versionUID != "" {
		path += "/" + uidBasedIDPathSegment(versionUID)
		route += "/{version_uid}"
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   path,
		Route:  route,
	}
	return decodeVersionedParty(ctx, c, req)
}

func decodeVersionedParty(ctx context.Context, c *transport.Client, req *transport.Request) (rm.Party, *openehrclient.VersionMetadata, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, openehrclient.NewVersionMetadata(resp.Metadata), err
		}
		return nil, nil, err
	}
	meta := openehrclient.NewVersionMetadata(resp.Metadata)
	if len(resp.Body) == 0 {
		if resp.StatusCode == http.StatusNoContent {
			return nil, meta, nil
		}
		return nil, meta, fmt.Errorf("demographic: %w: %d response with empty body", transport.ErrInvalidShape, resp.StatusCode)
	}
	var wire struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &wire); err != nil {
		return nil, meta, fmt.Errorf("demographic: decode VERSION envelope: %w", err)
	}
	if len(wire.Data) == 0 {
		return nil, meta, fmt.Errorf("demographic: %w: VERSION envelope missing data", transport.ErrInvalidShape)
	}
	party, err := typereg.DecodeAs[rm.Party](wire.Data)
	if err != nil {
		return nil, meta, fmt.Errorf("demographic: decode PARTY from VERSION: %w", err)
	}
	return party, meta, nil
}

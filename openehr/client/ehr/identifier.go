package ehr

import (
	"encoding/json"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// Identifier is the ITS-REST `Identifier` response body returned on
// versioned write paths (composition / directory / ehr_status create
// and update) when the server honours `Prefer: return=identifier`:
//
//	{"uid": "<version_uid>"}
//
// It is a bespoke ITS-REST wrapper — no `_type` discriminator, and not
// an RM `OBJECT_VERSION_ID` — distinct from the full-representation arm
// of the response body `oneOf` (REQ-094). The `uid` is the same version
// identifier the server also exposes in the `Location` path segment and
// the `ETag`.
type Identifier struct {
	// UID is the returned version identifier in OBJECT_VERSION_ID
	// lexical form: `<object_id>::<creating_system_id>::<version_tree_id>`.
	UID string `json:"uid"`
}

// ResolveIdentifierBody decodes an ITS-REST [Identifier] write-response
// body (sent when `Prefer: return=identifier` is honoured) and, when
// VersionUID was not already parsed from the Location header, populates
// it from the body's uid. Location stays canonical (REQ-094); the body
// is the documented fallback noted on [VersionMetadata].
//
// It enforces the REQ-094 "MUST NOT silently downgrade" rule for the
// identifier mode: a non-empty body that does not decode to an
// Identifier carrying a uid is returned as a [transport.ErrInvalidShape]
// error rather than silently discarded. An empty body is not an error —
// the identifier remains available via Location/ETag → VersionUID.
//
// No-op (returns nil) when m is nil or body is empty.
func (m *VersionMetadata) ResolveIdentifierBody(body []byte) error {
	if m == nil || len(body) == 0 {
		return nil
	}
	var id Identifier
	if err := json.Unmarshal(body, &id); err != nil {
		return fmt.Errorf("%w: decode Prefer=identifier body: %w", transport.ErrInvalidShape, err)
	}
	if id.UID == "" {
		return fmt.Errorf("%w: Prefer=identifier body has no uid", transport.ErrInvalidShape)
	}
	if m.VersionUID == "" {
		m.VersionUID = VersionUID(id.UID)
	}
	return nil
}

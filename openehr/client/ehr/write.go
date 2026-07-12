package ehr

import (
	"context"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// WriteConfig is the option set shared by every versioned-write leaf
// client — Composition/Directory Save & Update, demographic Create &
// Update, EHR_STATUS Put: the Prefer response-shape (REQ-094), the
// commit-time audit envelope (REQ-059), and the committed VERSION's
// lifecycle_state (REQ-059).
//
// Leaf packages define their own unexported writeConfig struct that
// embeds WriteConfig — either with no extra fields (directory,
// demographic, ehrstatus) or adding resource-specific options
// (composition, which adds template id and item tags). Embedding
// (rather than a type alias) keeps each leaf's writeConfig a distinct,
// unexported type, so its WriteOption / PutOption function type stays
// opaque to external callers even though the underlying option struct
// is structurally identical across leaves. The leaf's own WriteOption /
// PutOption type and With* constructors are unaffected (idiom.md
// public-API stability); only their bodies now set fields on the
// embedded struct.
type WriteConfig struct {
	Prefer         transport.Prefer
	AuditDetails   *rm.AuditDetails
	LifecycleState LifecycleState
}

// ResolveAuditHeader formats the openehr-audit-details request header
// (REQ-059) from the resolved config, wrapping any formatting error with
// label (e.g. "composition.Save") so each call site's error string stays
// exactly as it was before consolidation.
func (c WriteConfig) ResolveAuditHeader(label string) (string, error) {
	h, err := MarshalAuditDetails(c.AuditDetails)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return h, nil
}

// ResolveLifecycleHeader formats the openehr-version request header
// (REQ-059) from the resolved config, wrapping any formatting error with
// label (e.g. "composition.Save") so each call site's error string stays
// exactly as it was before consolidation.
func (c WriteConfig) ResolveLifecycleHeader(label string) (string, error) {
	h, err := FormatLifecycleStateHeader(c.LifecycleState)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return h, nil
}

// WriteResult executes a Save / Update / Create / Put request and
// decodes the response body per the Prefer state machine (REQ-094),
// shared by the four versioned-write leaf clients (composition,
// directory, demographic, ehrstatus). The Prefer value that drives the
// decode switch is read from req.Prefer — the single source of truth,
// since it is also what was sent on the wire:
//
//   - PreferRepresentation decodes the bare resource body via decode.
//     REQ-094: representation MUST NOT silently downgrade to an empty
//     body — an empty body returns [transport.ErrInvalidShape] rather
//     than a nil/zero resource.
//   - PreferIdentifier resolves the ITS-REST Identifier body into the
//     returned metadata's VersionUID. REQ-094: populate the identifier
//     slot from the body when present; never silently discard it.
//   - Any other Prefer (minimal, the spec default, or unset) returns a
//     nil/zero resource; the version id is in Location/ETag.
//
// label prefixes every error WriteResult itself raises (e.g.
// "composition", "ehrstatus.Put") so each site's error strings stay
// byte-identical to the pre-consolidation duplicated code; decode is the
// site's own response-body decoder and is responsible for wrapping its
// own decode errors with its own message.
//
// T instantiates as an interface for demographic ([rm.Party]) — safe
// because the zero value of an interface type is a true nil, the same
// pattern typereg.DecodeAs[T] already relies on (REQ-024: no reflection).
func WriteResult[T any](ctx context.Context, c *transport.Client, req *transport.Request, label string, decode func([]byte) (T, error)) (T, *VersionMetadata, error) {
	var zero T
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return zero, NewVersionMetadata(resp.Metadata), err
		}
		return zero, nil, err
	}
	meta := NewVersionMetadata(resp.Metadata)
	switch req.Prefer {
	case transport.PreferRepresentation:
		if len(resp.Body) == 0 {
			return zero, meta, fmt.Errorf("%s: %w: Prefer=return=representation but response body is empty", label, transport.ErrInvalidShape)
		}
		out, err := decode(resp.Body)
		if err != nil {
			return zero, meta, err
		}
		return out, meta, nil
	case transport.PreferIdentifier:
		if err := meta.ResolveIdentifierBody(resp.Body); err != nil {
			return zero, meta, fmt.Errorf("%s: %w", label, err)
		}
		return zero, meta, nil
	default:
		return zero, meta, nil
	}
}

// DoDelete issues a logical-delete request (Composition / Directory /
// demographic PARTY — EHR_STATUS has no delete operation) and returns
// only the version metadata; a delete response carries no body.
func DoDelete(ctx context.Context, c *transport.Client, req *transport.Request) (*VersionMetadata, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return NewVersionMetadata(resp.Metadata), err
		}
		return nil, err
	}
	return NewVersionMetadata(resp.Metadata), nil
}

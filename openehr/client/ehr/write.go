package ehr

import (
	"context"
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// WriteConfig is the option set shared by every versioned-write leaf
// client (Composition/Directory Save & Update, demographic Create &
// Update, EHR_STATUS Put): the Prefer response-shape, the
// commit-time audit envelope, and the committed VERSION's
// lifecycle_state.
//
// Leaf packages define their own unexported writeConfig struct that
// embeds WriteConfig, either with no extra fields (directory,
// demographic, ehrstatus) or with resource-specific options
// (composition, which adds template id and item tags). Because each
// leaf's writeConfig is a distinct unexported type, its WriteOption /
// PutOption function type stays opaque to external callers even though
// the underlying option struct is structurally identical across leaves.
type WriteConfig struct {
	Prefer         transport.Prefer
	AuditDetails   *rm.AuditDetails
	LifecycleState LifecycleState
}

// ResolveAuditHeader formats the openehr-audit-details request header
// from the resolved config, wrapping any formatting error with
// label (e.g. "composition.Save") so the error names the calling
// operation.
func (c WriteConfig) ResolveAuditHeader(label string) (string, error) {
	h, err := MarshalAuditDetails(c.AuditDetails)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return h, nil
}

// ResolveLifecycleHeader formats the openehr-version request header
// from the resolved config, wrapping any formatting error with
// label (e.g. "composition.Save") so the error names the calling
// operation.
func (c WriteConfig) ResolveLifecycleHeader(label string) (string, error) {
	h, err := FormatLifecycleStateHeader(c.LifecycleState)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return h, nil
}

// WriteResult executes a Save / Update / Create / Put request and
// decodes the response body per the Prefer state machine,
// shared by the four versioned-write leaf clients (composition,
// directory, demographic, ehrstatus). The Prefer value that drives the
// decode switch is read from req.Prefer, since that is also what was
// sent on the wire:
//
//   - PreferRepresentation decodes the bare resource body via decode.
//     Representation never silently downgrades to an empty body: an
//     empty or undecodable body returns a
//     [*NoRepresentationError] (wrapping [transport.ErrInvalidShape] for
//     an empty body, the decoder's error otherwise) that carries the
//     commit metadata, not a nil-error success. The resource slot is
//     still the zero value.
//   - PreferIdentifier resolves the ITS-REST Identifier body into the
//     returned metadata's VersionUID. The identifier is taken from the
//     body when present and never silently discarded.
//   - Any other Prefer (minimal, the ITS-REST default, or unset) returns a
//     nil/zero resource; the version id is in Location/ETag.
//
// A successful minimal or identifier write returns a zero resource: a
// typed-nil pointer for a concrete-pointer T (`== nil` is a correct
// test there) and a bare-nil interface for an interface T (demographic
// [rm.Party]). An interface return can in general hold a boxed
// typed-nil pointer, for which `== nil` lies. [HasResource] is the
// uniform presence test across the return types; [rm.IsTypedNil] is the
// typed-nil absence check for callers already holding a registered RM
// pointer (false for a bare-nil interface).
//
// label prefixes the identifier-arm errors WriteResult itself raises
// (e.g. "composition", "ehrstatus.Put") and the empty-body Cause inside
// [*NoRepresentationError]; the representation arm's outer error string
// is the typed error's own value-free classification. decode is the
// site's own response-body decoder and is responsible for wrapping its
// own decode errors with its own message.
//
// T instantiates as an interface for demographic ([rm.Party]). That is
// safe because the zero value of an interface type is a true nil, the
// same pattern typereg.DecodeAs[T] relies on, and needs no reflection.
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
		if transport.IsNoRepresentationBody(resp.Body) {
			return zero, meta, &NoRepresentationError{
				Meta:  meta,
				Cause: fmt.Errorf("%s: %w: Prefer=return=representation but response body is empty or null", label, transport.ErrInvalidShape),
			}
		}
		out, err := decode(resp.Body)
		if err != nil {
			return zero, meta, &NoRepresentationError{Meta: meta, Cause: err}
		}
		return out, meta, nil
	case transport.PreferIdentifier:
		if err := meta.ResolveIdentifierBody(resp.Body); err != nil {
			return zero, meta, fmt.Errorf("%s: %w", label, err)
		}
		return zero, meta, nil
	case transport.PreferDefault, transport.PreferMinimal:
		return zero, meta, nil
	default:
		// An unknown Prefer value decodes nothing: metadata-only is the
		// fail-safe arm (REQ-094), so a member added later cannot be
		// mistaken for a representation the server never negotiated.
		return zero, meta, nil
	}
}

// NoRepresentationError reports a committed write (a 2xx response) whose
// `representation` body was empty or could not be decoded as the expected
// resource. It lets callers tell "no body, write succeeded" from
// "write committed, body unusable" with errors.As alone: it is never a
// [*transport.WireError], and a non-2xx failure is never wrapped in it.
//
// Meta carries the version metadata that proves the commit (VersionUID when
// the server supplied it); errors raised by this SDK always carry a non-nil
// Meta. Together with the classification (the type itself, via errors.As),
// Meta is the boundary-safe surface. Cause is internal diagnostics and may
// carry payload-derived text (the strconv and encoding/json causes beneath
// an rm decode error quote the offending literal, the same class
// [transport.WithRawErrorBodies] gates for
// [transport.OpenEHRErrorDetail]), so, like [*transport.WireError],
// Error is value-free and never interpolates Cause; callers that need the
// diagnostics unwrap or read Cause deliberately.
type NoRepresentationError struct {
	Meta  *VersionMetadata
	Cause error
}

// Error names the classification only: never Cause text, never a
// payload-derived value.
func (e *NoRepresentationError) Error() string {
	if e == nil {
		return "ehr: no representation"
	}
	if errors.Is(e.Cause, transport.ErrInvalidShape) {
		return "ehr: committed write has no usable representation (empty or null body)"
	}
	if e.Cause != nil {
		return "ehr: committed write has no usable representation (decode failed)"
	}
	return "ehr: committed write has no usable representation"
}

// Unwrap exposes Cause so errors.Is/As reach the wrapped sentinel or decode
// error.
func (e *NoRepresentationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// DoDelete issues a logical-delete request (Composition / Directory /
// demographic PARTY; EHR_STATUS has no delete operation) and returns
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

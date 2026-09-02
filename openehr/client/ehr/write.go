package ehr

import (
	"bytes"
	"context"
	"errors"
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

// isNoRepresentationBody reports whether b is empty, whitespace-only, or the
// JSON `null` literal — the REQ-094 "no usable representation" classification
// shared by every 2xx representation-body consumer in this package
// (WriteResult and ehr.Create). A JSON null literal unmarshals into a struct
// target as a nil-error no-op, so classifying against the raw bytes here,
// before decode is attempted, is what stops a null body from masquerading as
// a populated, all-zero-value resource.
func isNoRepresentationBody(b []byte) bool {
	body := bytes.TrimSpace(b)
	return len(body) == 0 || bytes.Equal(body, []byte("null"))
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
//     body — an empty or undecodable body returns a
//     [*NoRepresentationError] (wrapping [transport.ErrInvalidShape] for
//     an empty body, the decoder's error otherwise) that carries the
//     commit metadata, not a nil-error success. The resource slot is
//     still the zero value.
//   - PreferIdentifier resolves the ITS-REST Identifier body into the
//     returned metadata's VersionUID. REQ-094: populate the identifier
//     slot from the body when present; never silently discard it.
//   - Any other Prefer (minimal, the spec default, or unset) returns a
//     nil/zero resource; the version id is in Location/ETag.
//
// A successful minimal or identifier write returns a zero resource: a
// typed-nil pointer for a concrete-pointer T (`== nil` is a correct
// test there) and a bare-nil interface for an interface T (demographic
// [rm.Party]) — but an interface return can in general hold a boxed
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
		if isNoRepresentationBody(resp.Body) {
			return zero, meta, &NoRepresentationError{
				Meta:  meta,
				Cause: fmt.Errorf("%s: %w: Prefer=return=representation but response body is empty", label, transport.ErrInvalidShape),
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
	default:
		return zero, meta, nil
	}
}

// NoRepresentationError reports a committed write — a 2xx response — whose
// `representation` body was empty or could not be decoded as the expected
// resource (REQ-094). It lets callers tell "no body, write succeeded" from
// "write committed, body unusable" with errors.As alone: it is never a
// [*transport.WireError], and a non-2xx failure is never wrapped in it.
//
// Meta carries the version metadata that proves the commit (VersionUID when
// the server supplied it); errors raised by this SDK always carry a non-nil
// Meta. Together with the classification (the type itself, via errors.As),
// Meta is the boundary-safe surface. Cause is internal diagnostics and may
// carry payload-derived text — rm decode errors embed the offending value
// (`parse %q`), the same class [transport.WithRawErrorBodies] gates for
// [transport.OpenEHRErrorDetail] — so, like [*transport.WireError] (REQ-093),
// Error is value-free and never interpolates Cause; callers that need the
// diagnostics unwrap or read Cause deliberately.
type NoRepresentationError struct {
	Meta  *VersionMetadata
	Cause error
}

// Error names the classification only (REQ-093 value-free discipline):
// never Cause text, never a payload-derived value.
func (e *NoRepresentationError) Error() string {
	if e == nil {
		return "ehr: no representation"
	}
	if errors.Is(e.Cause, transport.ErrInvalidShape) {
		return "ehr: committed write has no usable representation (empty body)"
	}
	if e.Cause != nil {
		return "ehr: committed write has no usable representation (decode failed)"
	}
	return "ehr: committed write has no usable representation"
}

// Unwrap exposes Cause so errors.Is/As reach the wrapped sentinel or decode
// error (REQ-025).
func (e *NoRepresentationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
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

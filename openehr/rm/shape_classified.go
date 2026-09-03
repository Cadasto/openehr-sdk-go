package rm

import "github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"

// classifyShape attaches typereg.ErrInvalidShape to a decode failure
// raised inside one of this package's hand-written primitive codecs —
// [Character.UnmarshalJSON]'s two arms and [Real.UnmarshalJSON]'s
// precision budget — without touching the failure's text.
//
// docs/specifications/wire.md § REQ-052 (Decode-side shape sentinel)
// asks for both halves of that: the failure's message MUST be unchanged
// by the classification, and the underlying error MUST stay reachable
// through unwrapping, "so the sentinel costs no diagnostic". The obvious
// fmt.Errorf("%w: %w", typereg.ErrInvalidShape, err) meets the second
// but not the first — it splices the sentinel's own text ("canjson:
// invalid JSON shape") into Error(), so the diagnostic a consumer reads
// is no longer the diagnostic the codec produced.
//
// typereg.WrapShapeError is not reusable here: it prepends
// `canjson: <RM_TYPE>: `, which is right for a generated funnel naming
// the type it was decoding and wrong for a primitive codec that has
// already named itself ("rm.Character: …"). Hence a sibling wrapper
// rather than a shared one.
//
// A nil err returns nil, so a caller cannot manufacture an error out of
// a success — the same guarantee typereg.WrapShapeError gives.
func classifyShape(err error) error {
	if err == nil {
		return nil
	}
	return &shapeClassified{cause: err}
}

// shapeClassified is the [typereg.ErrInvalidShape]-carrying error
// classifyShape returns, modelled on typereg's unexported shapeError
// (openehr/rm/typereg/registry.go). It is likewise unexported:
// consumers classify with errors.Is against the sentinel and reach the
// cause with errors.Is / errors.AsType, so the concrete type is not part
// of the public surface and can change without a breaking release.
type shapeClassified struct {
	cause error
}

// Error reproduces the cause's text verbatim. The classification is
// invisible in the message, which is what REQ-052's "unchanged by the
// classification" clause asks for.
func (e *shapeClassified) Error() string {
	return e.cause.Error()
}

// Unwrap returns the cause alone, so a single errors.Unwrap step lands
// on the error the codec actually failed with and errors.AsType reaches
// an encoding/json or strconv type held inside it.
//
// The [typereg.ErrInvalidShape] classification rides on Is below rather
// than on a second Unwrap branch. A multi-error Unwrap (returning
// []error{cause, ErrInvalidShape}) would satisfy errors.Is just as well,
// but it makes errors.Unwrap answer nil for every classified failure —
// not what REQ-052's "the cause stays reachable through unwrapping"
// promises. typereg.shapeError records the same reasoning for the
// generated funnel.
func (e *shapeClassified) Unwrap() error {
	return e.cause
}

// Is reports the sentinel this type carries. It matches
// [typereg.ErrInvalidShape] only — the classification classifyShape
// attaches — and leaves every other target to the ordinary unwrap walk
// down e.cause.
func (e *shapeClassified) Is(target error) bool {
	return target == typereg.ErrInvalidShape
}

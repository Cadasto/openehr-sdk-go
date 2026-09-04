package typereg

// Error surface of the type registry: the sentinels, the DecodeError
// envelope the canjson and canxml decoders share, and the shape-classification
// wrapper the generated UnmarshalJSON funnel uses. Registry, Decode and
// DecodeAs live in registry.go.

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by [Registry.Decode] and [DecodeAs]. They
// are unwrap-compatible (errors.Is) so call sites such as the canjson
// codec can wrap them in a richer [DecodeError] without losing the
// classification. PROBE-031 asserts ErrUnknownType.
var (
	// ErrMissingType signals that the input JSON object lacks the
	// `_type` discriminator required at a polymorphic site.
	ErrMissingType = errors.New("typereg: _type discriminator missing")
	// ErrUnknownType signals that `_type` is present but no
	// constructor is registered for the given discriminator.
	ErrUnknownType = errors.New("typereg: _type not in registry")
	// ErrTypeMismatch signals that the decoded concrete value does
	// not satisfy the target interface or type parameter T at a
	// [DecodeAs] call site.
	ErrTypeMismatch = errors.New("typereg: decoded type does not satisfy target")
	// ErrMaxDepthExceeded signals that the JSON nesting depth of a value
	// handed to Decode exceeds maxDecodeDepth — a guard against stack
	// exhaustion and quadratic re-parsing from a crafted deeply-nested
	// polymorphic document (e.g. nested CLUSTER/SECTION trees).
	ErrMaxDepthExceeded = errors.New("typereg: nesting depth exceeds limit")
	// ErrNilReceiver classifies an UnmarshalJSON / UnmarshalText call on a
	// nil receiver — caller-constructible misuse (a failed errors.AsType
	// leaves a typed nil behind; a zero-value struct holds one in a field)
	// that the no-panic rule (REQ-025, idiom.md § No panics) turns into an
	// error instead of a dereference. Every generated UnmarshalJSON and the
	// hand-written primitive codecs (rm.Real, rm.Integer, rm.Character) wrap
	// it; openehr/rm's nil-receiver census pins the whole registry.
	ErrNilReceiver = errors.New("typereg: nil receiver")
	// ErrInvalidShape classifies a JSON-level shape failure — valid JSON
	// that is the wrong shape for the target type — or a hand-written
	// primitive codec's refusal of a value it will not accept: rm.Real
	// past its 17-significant-digit budget (a digit count, not a
	// representability test), rm.Character not exactly one character
	// (REQ-052). It is usually attached by [WrapShapeError] from inside a
	// generated UnmarshalJSON method; the hand-written codecs attach it
	// directly through their own message-preserving wrapper. Its message names
	// canjson because canjson is where callers meet it: it is
	// re-exported as canjson.ErrInvalidShape and lives here only so code
	// in openehr/rm can attach it without forming an
	// `openehr/rm → openehr/serialize` import cycle.
	ErrInvalidShape = errors.New("canjson: invalid JSON shape")
)

// DecodeError is the unified envelope returned by the canjson and
// canxml decoders at polymorphic-dispatch sites. It lives here in
// typereg (rather than in a codec-specific package) so the
// generator-emitted UnmarshalJSON methods on the generated RM types
// can construct it without forming an `openehr/rm → serialize/...`
// import cycle.
//
// Path is a JSON-pointer-ish or XPath-ish string describing the
// failed node; Type is the observed discriminator (may be empty when
// the discriminator was missing); Inner unwraps to one of the
// typereg sentinels (or a codec-defined shape error).
//
// The envelope is classification-neutral in both directions: it never
// adds [ErrInvalidShape] to what it wraps, and — because Unwrap exposes
// Inner — it never strips one raised beneath it. So a dispatch failure
// (missing / unknown / mismatched `_type`) stays outside the shape
// sentinel, while a shape failure inside the concrete type selected at
// a slot answers true to both errors.AsType[*DecodeError] and
// errors.Is(_, ErrInvalidShape): the path from this type, the kind from
// the sentinel (REQ-052).
type DecodeError struct {
	Path  string
	Type  string
	Inner error
}

// Error names the failed node and the wrapped cause. A nil receiver
// answers with the zero DecodeError's text rather than dereferencing
// (REQ-025 nil-receiver axis). This type is the documented errors.As /
// errors.AsType out-parameter for both codecs — it is re-exported as
// canjson.DecodeError and canxml.DecodeError — so a failed match leaves
// a typed nil in consumer hands on the most-travelled decode route in
// the SDK.
func (e *DecodeError) Error() string {
	if e == nil {
		return (&DecodeError{}).Error()
	}
	// Every producer sets Inner; only a caller-built zero value leaves it
	// nil. Inner is any error a codec or caller can supply — including a
	// boxed typed-nil pointer whose own Error() dereferences unguarded, the
	// shape a failed errors.As / errors.AsType leaves behind — so a direct
	// err.Error() call is not safe here the way it is for this package's own
	// guarded types. %v is: fmt recovers a panicking Error() method itself
	// and renders a nil-pointer receiver as "<nil>" (REQ-025 nil-receiver
	// axis).
	cause := "unspecified error"
	if e.Inner != nil {
		cause = fmt.Sprintf("%v", e.Inner)
	}
	switch {
	case e.Path != "" && e.Type != "":
		return fmt.Sprintf("decode %s (_type=%q): %s", e.Path, e.Type, cause)
	case e.Path != "":
		return fmt.Sprintf("decode %s: %s", e.Path, cause)
	case e.Type != "":
		return fmt.Sprintf("decode _type=%q: %s", e.Type, cause)
	default:
		return "decode: " + cause
	}
}

// Unwrap returns the wrapped error so errors.Is / errors.As reach the
// underlying sentinel. A nil receiver unwraps to nil (REQ-025
// nil-receiver axis).
func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}

// WrapShapeError builds the error a generated UnmarshalJSON method
// returns when the whole value fails to decode into its wire struct.
// rmType is the openEHR class name (e.g. "DV_QUANTITY"); err is the
// encoding/json failure underneath.
//
// The message is exactly `canjson: <rmType>: <err>`, and err stays
// reachable through errors.As, so the classification costs no
// diagnostic. What it adds is [ErrInvalidShape] under errors.Is —
// except when err already carries a [DecodeError]: this arm does not
// add the sentinel to it, and whatever classification that DecodeError
// already carries — its dispatch sentinel, or a shape classification
// raised beneath it — is kept as is (REQ-052).
//
// The bypass is one-directional: it withholds a classification the
// enclosing funnel would otherwise add, never removes one raised
// further down. So a shape failure inside the concrete type selected
// at a polymorphic slot keeps ErrInvalidShape while the DecodeError
// keeps the path — both true of the same error — whereas a dispatch
// failure (missing, unknown or mismatched `_type`) stays outside the
// sentinel, because ErrInvalidShape means "JSON-level shape", not "any
// decode failure".
//
// A nil err returns nil, so a caller cannot manufacture an error out
// of a success.
func WrapShapeError(rmType string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*DecodeError](err); ok {
		return fmt.Errorf("canjson: %s: %w", rmType, err)
	}
	return &shapeError{rmType: rmType, cause: err}
}

// shapeError is the [ErrInvalidShape]-carrying error [WrapShapeError]
// returns. It is unexported: consumers classify with errors.Is against
// the sentinel and reach the cause with errors.As, so the concrete type
// is not part of the public surface.
type shapeError struct {
	rmType string
	cause  error
}

// Error reproduces the `canjson: <RM_TYPE>: <cause>` text the generated
// methods have always returned.
func (e *shapeError) Error() string {
	return "canjson: " + e.rmType + ": " + e.cause.Error()
}

// Unwrap returns the cause alone, so a single errors.Unwrap step lands
// on the encoding/json error the generated method actually failed with,
// and errors.As reaches it as before. The [ErrInvalidShape]
// classification rides on Is below rather than on a second unwrap
// branch: a multi-error Unwrap would make errors.Unwrap answer nil for
// every generated decode failure, which is not what the "the cause
// stays reachable through unwrapping" clause promises (REQ-052).
func (e *shapeError) Unwrap() error {
	return e.cause
}

// Is reports the sentinel this type carries. It matches [ErrInvalidShape]
// only — the classification [WrapShapeError] attaches — and leaves every
// other target to the ordinary unwrap walk down e.cause.
func (e *shapeError) Is(target error) bool {
	return target == ErrInvalidShape
}

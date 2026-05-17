package canxml

import (
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/internal/poly"
)

// ErrInvalidShape is the canxml-local sentinel for XML-level shape
// errors (malformed XML, element-name mismatch on a non-polymorphic
// field, numeric overflow, `xmi:type` rejection). Polymorphic-
// discrimination errors come from the typereg package — callers
// MUST `errors.Is` against [typereg.ErrMissingType] /
// [typereg.ErrUnknownType] / [typereg.ErrTypeMismatch] rather than
// against this sentinel.
var ErrInvalidShape = errors.New("canxml: invalid XML shape")

// ErrNamespace is the canxml-local sentinel for foreign-namespace
// errors: an element decoded outside the openEHR canonical default
// namespace at a position the spec pins to it. The decoder tolerates
// redundant `xmlns` declarations on inner elements and the XSI
// namespace for `xsi:type`, but rejects unknown namespaces with this
// error.
var ErrNamespace = errors.New("canxml: foreign XML namespace")

// DecodeError is the unified error returned by the decoder at
// polymorphic dispatch sites. Re-exported from the internal poly
// helper so consumers can `errors.As` against a stable type without
// importing internal packages — same envelope shape as canjson.
type DecodeError = poly.DecodeError

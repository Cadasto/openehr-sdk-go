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

// ErrNamespace is the canxml-local sentinel reserved for foreign-
// namespace rejection: an element decoded outside the openEHR
// canonical default namespace at a position the spec pins to it.
//
// NOTE: the decoder does not currently return it — the generated
// UnmarshalXML methods match children by local name and skip
// unrecognised elements, so foreign namespaces are tolerated rather
// than rejected. The sentinel is retained as a released public symbol
// (shipped since v0.1.0) for source compatibility and for a future
// strict-namespace decode path.
var ErrNamespace = errors.New("canxml: foreign XML namespace")

// DecodeError is the unified error returned by the decoder at
// polymorphic dispatch sites. Re-exported from the internal poly
// helper so consumers can `errors.As` against a stable type without
// importing internal packages — same envelope shape as canjson.
type DecodeError = poly.DecodeError

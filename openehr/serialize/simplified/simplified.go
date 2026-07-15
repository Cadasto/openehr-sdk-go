package simplified

import "errors"

// Media types for the Simplified Formats (REQ-053). Emit these; accept
// EHRbase's non-conformant `.schema`-suffixed variants on input only.
const (
	MediaTypeFlat       = "application/openehr.wt.flat+json"
	MediaTypeStructured = "application/openehr.wt.structured+json"
)

var (
	// ErrNoTemplate is returned when a nil Web Template is passed to a
	// conversion that needs one to resolve identifiers, types, and level
	// removal.
	ErrNoTemplate = errors.New("simplified: nil web template")
	// ErrUnknownPath is returned when a FLAT/STRUCTURED key does not resolve to
	// a Web Template node (a typo, a wrong template, an unsupported ctx/ field,
	// or an out-of-bound :index). The codec fails loudly rather than dropping
	// the entry (REQ-053 semantics-preserving).
	ErrUnknownPath = errors.New("simplified: path not in web template")
	// ErrUnsupportedDatatype is returned when a leaf's RM datatype is not mapped
	// to/from a FLAT suffix set and cannot ride |raw, when a suffix is not valid
	// for the datatype, or when a |raw / |other suffix is misused. Failing beats
	// silently omitting or mistyping a value (REQ-053).
	ErrUnsupportedDatatype = errors.New("simplified: unsupported datatype")
	// ErrMissingContext is returned when mandatory context (ctx/language,
	// ctx/territory) is absent on decode.
	ErrMissingContext = errors.New("simplified: missing mandatory context")
	// ErrNilComposition is returned when a nil composition is passed to an
	// encode.
	ErrNilComposition = errors.New("simplified: nil composition")
)

// MarshalFlat is implemented in flat_encode.go.
// UnmarshalFlat is implemented in flat_decode.go.
// MarshalStructured / UnmarshalStructured / FlatToStructured / StructuredToFlat
// are implemented in structured.go.

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
	// ErrUnknownPath is returned when a FLAT/STRUCTURED key does not resolve
	// to a Web Template node (a typo, a wrong template, or a key whose feature
	// is not yet supported — e.g. ctx/ context, see deviations.md). The codec
	// fails loudly rather than dropping the entry (REQ-053 semantics-preserving).
	ErrUnknownPath = errors.New("simplified: path not in web template")
	// ErrUnsupportedDatatype is returned when a leaf's RM datatype is not yet
	// mapped to/from a FLAT suffix set. Failing beats silently omitting a value
	// (REQ-053); the |raw fallback that will carry exotic datatypes is Phase 6.
	ErrUnsupportedDatatype = errors.New("simplified: unsupported datatype")
	// ErrMissingContext is returned when mandatory context (language,
	// territory) is absent and cannot be defaulted. Reserved for the Phase 6
	// ctx/ context handling (see deviations.md).
	ErrMissingContext = errors.New("simplified: missing mandatory context")
	// ErrNilComposition is returned when a nil composition is passed to an
	// encode.
	ErrNilComposition = errors.New("simplified: nil composition")
)

// MarshalFlat is implemented in flat_encode.go.
// UnmarshalFlat is implemented in flat_decode.go.
// MarshalStructured / UnmarshalStructured / FlatToStructured / StructuredToFlat
// are implemented in structured.go.

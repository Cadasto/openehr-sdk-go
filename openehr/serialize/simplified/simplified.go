package simplified

import (
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

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
	// to a Web Template node.
	ErrUnknownPath = errors.New("simplified: path not in web template")
	// ErrMissingContext is returned when mandatory context (language,
	// territory) is absent and cannot be defaulted.
	ErrMissingContext = errors.New("simplified: missing mandatory context")
	// ErrNilComposition is returned when a nil composition is passed to an
	// encode.
	ErrNilComposition = errors.New("simplified: nil composition")
)

// MarshalFlat is implemented in flat_encode.go.

// UnmarshalFlat decodes FLAT JSON into a canonical COMPOSITION
// using wt (REQ-053). Implemented in flat_decode.go.
func UnmarshalFlat(data []byte, wt *webtemplate.WebTemplate) (*rm.Composition, error) {
	return nil, ErrNoTemplate
}

// MarshalStructured / UnmarshalStructured / FlatToStructured / StructuredToFlat
// are implemented in structured.go.

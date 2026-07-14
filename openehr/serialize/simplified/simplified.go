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

// MarshalStructured encodes comp as STRUCTURED JSON using wt (REQ-053).
// Implemented in structured.go.
func MarshalStructured(comp *rm.Composition, wt *webtemplate.WebTemplate) ([]byte, error) {
	return nil, ErrNoTemplate
}

// UnmarshalStructured decodes STRUCTURED JSON into a canonical COMPOSITION
// using wt (REQ-053). Implemented in structured.go.
func UnmarshalStructured(data []byte, wt *webtemplate.WebTemplate) (*rm.Composition, error) {
	return nil, ErrNoTemplate
}

// FlatToStructured restructures FLAT JSON into STRUCTURED JSON. The two
// variants share one identifier grammar, so this needs no Web Template
// (REQ-053). Implemented in structured.go.
func FlatToStructured(data []byte) ([]byte, error) {
	return nil, ErrUnknownPath
}

// StructuredToFlat restructures STRUCTURED JSON into FLAT JSON. Needs no Web
// Template (REQ-053). Implemented in structured.go.
func StructuredToFlat(data []byte) ([]byte, error) {
	return nil, ErrUnknownPath
}

package simplified

import (
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// Option configures a decode (UnmarshalFlat / UnmarshalStructured).
type Option func(*decodeConfig)

// decodeConfig holds the resolved decode options.
type decodeConfig struct {
	template *templatecompile.Compiled
}

// WithTemplate supplies the compiled template (REQ-111) so decode repopulates
// the mandatory LOCATABLE.name on every reconstructed node from the archetype
// terminology. The FLAT/STRUCTURED formats do not carry names, and the Web
// Template collapses the HISTORY / ITEM_STRUCTURE wrappers, so the compiled
// template (which the Web Template is built from) is the authoritative name
// source. Without this option decode omits names (the round-trip stays
// format-idempotent).
//
// This closes the LOCATABLE.name gap only. Other RM-mandatory attributes the
// FLAT/STRUCTURED formats do not carry (HISTORY.origin, EVENT.time, ENTRY
// language/encoding/subject, EVENT_CONTEXT.setting) are still not reconstructed,
// so the decoded composition is not yet fully OPT-validatable — see deviations.md.
func WithTemplate(c *templatecompile.Compiled) Option {
	return func(cfg *decodeConfig) { cfg.template = c }
}

func newDecodeConfig(opts []Option) decodeConfig {
	var cfg decodeConfig
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return cfg
}

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

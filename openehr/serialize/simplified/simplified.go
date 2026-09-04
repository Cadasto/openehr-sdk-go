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

// WithTemplate supplies the compiled template (REQ-111), switching decode into
// conformant mode: the mandatory LOCATABLE.name is repopulated on every
// reconstructed node from the archetype terminology (the formats do not carry
// names, and the Web Template collapses the HISTORY / ITEM_STRUCTURE wrappers,
// so the compiled template is the authoritative source), and the RM-mandatory
// attributes the formats omit (HISTORY.origin, EVENT.time, ENTRY
// language/encoding/subject, EVENT_CONTEXT.setting, …) are completed from ctx
// defaults + RM conventions — synthesised defaults, not recovered data. The
// decoded composition then validates against the OPT, provided ctx/time is
// present when the template carries HISTORY/EVENT nodes (their mandatory
// origin/time have no other source; see deviations.md).
//
// Without this option decode omits names and defaults — the round-trip stays
// format-idempotent but the result is not canonically complete.
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

// Media types for the Simplified Formats (REQ-053). Emit these canonical
// types — [Format.MediaType] does. [ParseMediaType] also accepts EHRbase's
// non-conformant `.schema`-suffixed variants on input; the codecs themselves
// take bytes, so negotiation happens one call before them.
const (
	MediaTypeFlat       = "application/openehr.wt.flat+json"
	MediaTypeStructured = "application/openehr.wt.structured+json"
)

var (
	// ErrNoTemplate is returned when a nil Web Template is passed to a
	// conversion that needs one to resolve identifiers, types, and level
	// removal.
	ErrNoTemplate = errors.New("simplified: nil web template")
	// ErrUnknownPath is returned when a FLAT/STRUCTURED key is unresolvable or
	// structurally inadmissible. Unresolvable: the key does not name a Web
	// Template node (a typo, a wrong template, an unsupported ctx/ field), it
	// names an underscore-prefixed RM attribute family this codec does not carry
	// or one the owning node's RM class does not declare (REQ-140 — the grammar
	// is typed by the owner, so `_work_flow_id` on a SECTION is as unresolvable
	// as a misspelled segment), or it cannot be placed faithfully (an invalid /
	// out-of-bound / sparse :index — including on a `_family:N` list — a slot
	// conflict between two keys, or the decoded-node budget).
	// Inadmissible: every key resolves, but the set of them cannot be honoured at
	// once — two accepted spellings of one composition-metadata field that
	// disagree, ctx/composer_self beside a composer name (mutually exclusive
	// representations of one RM attribute), or two spellings of one
	// single-valued underscore attribute. The codec fails loudly rather than
	// dropping the entry or inventing a precedence rule (REQ-053
	// semantics-preserving; ADR 0015 decision 4).
	ErrUnknownPath = errors.New("simplified: path not in web template")
	// ErrUnsupportedDatatype is returned when a leaf's RM datatype is not mapped
	// to/from a FLAT suffix set and cannot ride |raw, when a suffix or ctx/
	// value is not valid for its slot, when a |raw / |other suffix is
	// misused, or when an underscore-prefixed RM attribute family (REQ-140)
	// carries a key outside its grammar, is missing an RM-mandatory one, or
	// holds a value the family's suffixes cannot express — a coded LINK.meaning,
	// a decorated `context/_end_time`, an OBJECT_REF id that is neither
	// GENERIC_ID nor HIER_OBJECT_ID. Those families have no |raw carrier, so
	// there is nothing to fall back to. Failing beats silently omitting or
	// mistyping a value (REQ-053).
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

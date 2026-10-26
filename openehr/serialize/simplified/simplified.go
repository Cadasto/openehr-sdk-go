package simplified

import (
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// Option configures a decode (UnmarshalFlat / UnmarshalStructured).
// [WithTemplate] supplies the compiled template; [WithUnmatched] and
// [WithStrict] say what happens to input the decode does not read.
type Option func(*decodeConfig)

// decodeConfig holds the resolved decode options.
type decodeConfig struct {
	template  *templatecompile.Compiled
	unmatched func(path string)
	strict    bool
}

// WithUnmatched passes every unmatched item of the input to fn while the
// decode carries on. An item is unmatched when the decode does not read it:
// a key whose path the Web Template does not resolve, an underscore-prefixed
// segment that names no RM attribute this codec carries at that node, a
// suffix outside the suffix set of its datatype (or of its underscore
// family), and a `ctx/` field this codec does not read. At every level of a
// STRUCTURED body, a member whose name the FLAT grammar does not read there is
// unmatched whatever its value holds. A key the decode reads is never
// unmatched, even where the decode then
// refuses it, and these fail the decode in every mode: the keys with no
// carrier (a PARTICIPATION `time`, a FEEDER_AUDIT_DETAILS `other_details`,
// `_instruction_details`, `_wf_definition`); the reference's in-context
// spelling of RM data this codec's Web Template does not model (an ACTION's
// ISM transition and time, an INTERVAL_EVENT's `math_function`, `width` and
// `sample_count`, an OBSERVATION's `history_origin`); at a leaf whose datatype
// has neither a suffix set nor a sub-path grammar, its bare value and a suffix
// naming an attribute of its RM class; and any malformed value, missing required suffix (a lone bare value
// where the datatype has no bare spelling among them), disagreeing pair of
// spellings of a key the decode reads, or index past its bound.
//
// By default the decoder ignores unmatched content: the value it returns
// holds the rest of the input and none of it. With this option fn receives
// each item, once, in the order it appears in the input, and the value
// returned is the same as without the option.
//
// fn gets the item's path. For FLAT that is the key as written, such as
// "vital_signs/body_temperature:0/any_event:0/temperature|x-note". For
// STRUCTURED it is the path of members from the root, joined by "/", with
// the position of each array element as its own segment, counting from 0:
// "vital_signs/body_temperature/0/any_event/0/temperature/0/|x-note", or
// "ctx/x-flag" for a field of the ctx object. A member whose name resolves
// nothing is reported as that member, not as what it holds. The path carries
// names only,
// never a value from the input. fn runs on the caller's goroutine, after the
// decode has read the input. Without [WithStrict], fn runs only when the decode
// succeeds.
//
// A nil fn is the same as leaving the option out. Combined with
// [WithStrict], fn receives the first unmatched item, and no other, before
// the decode is refused.
func WithUnmatched(fn func(path string)) Option {
	return func(cfg *decodeConfig) { cfg.unmatched = fn }
}

// WithStrict refuses the decode at its first unmatched item (see
// [WithUnmatched] for what is unmatched and for the form of its path), the
// first in the order the input gives. The error wraps [ErrUnknownPath] and
// names the item's path, quoted; it carries none of the item's value.
//
// Strict covers unmatched content only. An absent key is never refused,
// mandatory or not, and neither is a boundary flag an interval leaves out.
// The keys the decode reads but refuses (see [WithUnmatched]) fail in every
// mode, so a body holding one of them can fail on it before strict refuses
// its unmatched content.
//
// By default the decoder ignores unmatched content.
func WithStrict() Option {
	return func(cfg *decodeConfig) { cfg.strict = true }
}

// tracked reports whether the decode reports or refuses unmatched content.
func (c decodeConfig) tracked() bool {
	return c.unmatched != nil || c.strict
}

// WithTemplate supplies the compiled template, switching decode into
// conformant mode. The mandatory LOCATABLE.name is repopulated on every
// reconstructed node from the archetype terminology (the formats carry no
// name but the composition's own, `<root>/_name`, which decode reads in both
// modes, and the Web Template collapses the HISTORY / ITEM_STRUCTURE
// wrappers, so the compiled template is the authoritative source). Where the
// template constrains a node's name as a DV_CODED_TEXT and settles both its
// code and its text, the node gets that coded name. Where it leaves the code
// or the text open (a code from an external value set, say), the node gets a
// DV_TEXT name, since the formats carry no name to read the rest from. A
// structure the formats fold away takes the concrete ITEM_STRUCTURE subtype
// the template gives it, such as an ITEM_LIST, which the path alone cannot
// tell from an ITEM_TREE. The RM-mandatory attributes the formats omit
// (HISTORY.origin, EVENT.time, ENTRY language/encoding/subject,
// EVENT_CONTEXT.setting, …) are completed from ctx defaults and RM
// conventions; these are synthesised defaults, not recovered data. An empty structure (an ITEM_TREE or ITEM_LIST with no
// items, say) that the template requires and allows to stay empty has no key
// in the formats, so decode adds it back, empty, under a parent the body
// rebuilt, with the type, node id and name the template gives it. It is
// synthesised too, and decode adds none where the template asks the
// structure for a member. The decoded composition then validates against the
// OPT, provided ctx/time is present when the template carries HISTORY/EVENT
// nodes (their mandatory origin/time have no other source; see
// deviations.md).
//
// Without this option decode omits names, defaults and empty structures, and
// infers a folded structure's subtype from its path: the round-trip stays
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

// Media types for the Simplified Formats. Emit these canonical
// types, as [Format.MediaType] does. [ParseMediaType] also accepts the
// deprecated `.schema`-suffixed variants (retired from the specification,
// still served by EHRbase) on input; the codecs themselves take bytes, so
// negotiation happens one call before them.
const (
	MediaTypeFlat       = "application/openehr.wt.flat+json"
	MediaTypeStructured = "application/openehr.wt.structured+json"
)

var (
	// ErrNoTemplate is returned when a nil Web Template is passed to a
	// conversion that needs one to resolve identifiers, types, and level
	// removal.
	ErrNoTemplate = errors.New("simplified: nil web template")
	// ErrUnknownPath is returned when a FLAT/STRUCTURED key is unmatched and
	// the decode is strict, or when a key the decode reads cannot be placed.
	//
	// Under [WithStrict] the first unmatched item refuses the decode with an
	// error that wraps this sentinel and names the item (see [WithUnmatched]).
	// By default unmatched content is ignored and never returns it.
	//
	// In every mode it is returned for a key that cannot be placed faithfully
	// (an invalid, out-of-bound or sparse :index, including on a `_family:N`
	// list, an index past :0 on a single-valued attribute, a slot conflict
	// between two keys, a key that walks through one of several reused
	// siblings, or the decoded-node budget), and for keys that each resolve
	// but cannot be honoured at once: two accepted spellings of one
	// composition-metadata field that disagree, a ctx/composer_self of true
	// beside a composer name, identifier or relationship (a PARTY_SELF has
	// none of them),
	// two spellings of one single-valued underscore attribute, or a key the
	// decode reads and the same key ending in a bare `|` with differing values.
	// The codec fails
	// rather than dropping the entry or inventing a precedence rule. The two
	// deferred underscore families, `_instruction_details` and `_wf_definition`,
	// and the reference's in-context spelling of an ACTION's ISM transition and
	// time, an INTERVAL_EVENT's `math_function`, `width` and `sample_count`
	// and an OBSERVATION's `history_origin` return it in every mode too.
	ErrUnknownPath = errors.New("simplified: path not in web template")
	// ErrUnsupportedDatatype is returned when a leaf's RM datatype is not mapped
	// to/from a FLAT suffix set and cannot ride |raw, when a suffix or ctx/
	// value is not valid for its slot (a value the canonical-JSON decode of its
	// RM attribute would refuse, named by its FLAT key, or a required suffix
	// that is missing or null), when a |raw / |other suffix is
	// misused, when a key the decode reads has no carrier (a PARTICIPATION
	// `time`, a FEEDER_AUDIT_DETAILS `other_details`), when a composer cannot
	// be written (a PARTY_IDENTIFIED with an empty name, or with no name,
	// reference or identifiers), when a PARTY_RELATED at any party position
	// has no relationship, or when an underscore-prefixed
	// RM attribute family is missing an RM-mandatory key or holds a value the
	// family's suffixes cannot express (a coded LINK.meaning, a decorated
	// `context/_end_time`, an OBJECT_REF id that is neither GENERIC_ID nor
	// HIER_OBJECT_ID). Those families have no |raw carrier, so there is
	// nothing to fall back to. The codec fails rather than silently omitting
	// or mistyping a value.
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

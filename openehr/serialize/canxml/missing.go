package canxml

// WithMissing passes every missing member of the input to fn while the
// decode carries on. A member is missing when the Reference Model makes the
// attribute mandatory on the class its element decodes as, the class its
// `xsi:type` names, or else the type the RM declares where it sits, as
// [WithUnmatched] reads it, and the element carries no value for it: it has
// no child element of that name, nor one the AM 1.4 schema declares in its
// place (`terminology_id` for a C_CODE_PHRASE's terminology) holding a
// value, and no XML attribute of it where the ITS-XML schema carries the
// attribute as one (`archetype_node_id`, an ARCHETYPE_TERM's `code`); the
// child it has carries `xsi:nil="true"` (or `1`), whatever it holds; or the
// child is a primitive element whose text is empty or XML whitespace only.
// A C_DV_QUANTITY's `property` written as a CODE_PHRASE carries a value
// when its `terminology_id` value or its `code_string` does, and is
// reported, with nothing inside it, when neither does. An ORDINAL symbol
// written as a DV_CODED_TEXT has the members of its `defining_code`, named
// as the symbol's own, and its `value` is never missing.
// The value the decode returns cannot always show that: a member held by
// value, a number, a boolean, a string or a value struct such as PARTY_SELF
// or ARCHETYPE_ID, decodes to its zero value whether the input carried it or
// not.
//
// A member that carries a value is never missing, whatever the value: an
// empty struct
// element such as `<subject/>` or a bare `<subject xsi:type="PARTY_SELF"/>`,
// `<magnitude>0</magnitude>`, `<units>kg</units>` where `<units></units>` is
// missing, an `archetype_node_id` written as an XML attribute, and a `Hash`
// or `Any` element the codec does not model are present. Canonical XML has
// no element for an empty list, so a mandatory list the input leaves empty,
// a CLUSTER's `items`, is missing, where canonical JSON's `[]` is present.
// Three kinds of attribute are never missing: one no serialization carries
// (a function the model computes, such as EVENT `offset`, and
// RESOURCE_DESCRIPTION `parent_resource`); the four interval boundary flags,
// which take their defaults when absent; and `xsi:type`, which names no
// attribute.
//
// The option reports only. A missing member is never a decode error, with or
// without [WithStrict], which refuses unmatched content only, and the decode
// returns the same value as without the option, each missing member holding
// its zero value. fn receives each member once, as its JSON Pointer (RFC
// 6901) in the canonical-JSON form of the instance, from the root of the
// value, such as "/content/0/data/events/0/data/items/0/value/magnitude":
// the attribute's name under the pointer of the element that lacks it, and
// each child element of a multi-valued attribute by its position among its
// same-named siblings, counted from 0. That is not the element path
// [WithUnmatched] reports: a missing member is a member of the instance,
// not an item of the document, so both codecs name it one way and a
// consumer of either format hands the validators one list. The pointer
// carries no value from the input. The members of one element are reported
// as the element closes, in the order the RM declares the attributes (an
// ancestor's before the class's own), so the members of the elements inside
// it come first. Nothing inside a nilled element is reported: the member is
// reported once, by its parent, as a `null` is in canonical JSON.
//
// The check runs as the decode streams, in the pass that serves
// [WithUnmatched], so the two options may be combined, each with its own
// grammar, and fn runs during the decode, on the caller's goroutine. A
// decode that fails makes no promise about which members it reported before
// the failure. Every class the generated decoders read is checked, the AOM
// 1.4 classes included, where canonical JSON checks the RM classes only
// (see the package documentation, "Missing members"). One member escapes
// the check: a primitive already held in an interface-typed slot, which
// encoding/xml reads itself, counts as a value even when its element is
// empty.
//
// The option applies to [Unmarshal] and to a [Decoder] from [NewDecoder],
// where each value's pointers are relative to that value. A nil fn is the
// same as leaving the option out.
func WithMissing(fn func(pointer string)) DecoderOption {
	return func(c *decoderConfig) { c.missing = fn }
}

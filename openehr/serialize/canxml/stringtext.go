package canxml

// WithStringText keeps the text of a string member sent empty or as XML
// whitespace only. By default such a member is sent without a value (see the
// package documentation, "Wire profile"): an optional one decodes as absent
// and a mandatory one as the empty string, so `<value>  </value>` reads as
// "" and `<version_id></version_id>` as no version_id, where canonical JSON
// keeps "  " and "". Given this option, a member of the RM type String whose
// element is not nilled decodes as the text it carries, so the same content
// decodes to the same value in both formats.
//
// Such a member carries a value: [WithMissing] does not report it, as
// canonical JSON's report does not report an empty string, and
// [WithPresence] still reports it as [PresenceEmpty], so a caller can tell
// it was sent empty. A nilled member, a number or boolean element and an
// empty element of any other primitive type decode as without the option.
//
// The option applies to [Unmarshal] and to a [Decoder] from [NewDecoder].
func WithStringText() DecoderOption {
	return func(c *decoderConfig) { c.stringText = true }
}

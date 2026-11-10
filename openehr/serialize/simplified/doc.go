// Package simplified converts an openEHR COMPOSITION to and from the FLAT
// and STRUCTURED Simplified Formats. These are the two variants named by
// the STABLE ITS-REST Simplified Formats specification; the older
// "Simplified Data Template" / simSDT / structSDT naming is superseded.
//
// These are serializations of a data instance, not a template. Field
// identifiers are the human-readable Web Template ids, not canonical
// AQL/AOM paths, so conversion to and from canonical RM is
// template-specific and requires the composition's Web Template. Given the
// template the conversion is bidirectional and semantics-preserving. The
// simplified forms are not self-standing (they depend on the template),
// but they are not lossy. Reconstructing the template/OPT from a data
// instance is out of scope.
//
// The codecs take an *rm.Composition and a *webtemplate.WebTemplate and
// return bytes (or the reverse). They import only openehr/rm (+ rmpath /
// rminfo / typereg), openehr/terminology, openehr/template (+ constraints /
// webtemplate), openehr/templatecompile (the WithTemplate name/default
// source), openehr/serialize/canjson, module-internal helpers, and the
// standard library. They never import the transport, auth, or client layers.
//
// Context output-form and exotic-datatype fallbacks are documented in
// deviations.md beside the package tests.
//
// # Composition context
//
// Composition-level metadata travels in the ctx/ short forms: ctx/language,
// ctx/territory, ctx/composer_name, ctx/composer_self, ctx/time and the
// ctx/setting|code and ctx/setting|value pair. Decode also reads the
// reference implementation's spelling of each under the template root, such
// as "<root>/language|code", and two spellings of one field must agree.
//
// The format gives a body without ctx/time the current time, but decode reads
// no clock. With [WithTemplate], [WithDefaultTime] passes it one: a body with
// no start time decodes as if its ctx/time held the caller's time, and on a
// persistent composition that default builds no EVENT_CONTEXT.
//
// The composer is the one field those short forms carry only in part:
// ctx/composer_name carries a party with a name and nothing else, and
// ctx/composer_self a composer that is the record subject, with no reference.
// The composer's own path under the template root carries the whole composer,
// in the party spelling every other party position uses: "<root>/composer|name",
// "|id", "|id_scheme", "|id_namespace" and "|type" for its external
// reference, "<root>/composer/_identifier:N" for its identifiers and
// "<root>/composer/relationship" for a related party's relationship. Decode
// reads those keys beside the ctx/ fields and merges them into one composer;
// a composer_self of true beside a reference is a PARTY_SELF carrying that
// reference, and beside a name, an identifier or a relationship it is
// refused. Encode writes a composer one ctx/ short form carries whole in that
// form, and every other composer whole under the template root with no ctx/
// composer key. It refuses a PARTY_IDENTIFIED composer with an empty name or
// with no name, reference or identifiers, and a PARTY_RELATED with no
// relationship at any party position.
//
// The composition's own name travels as "<root>/_name", spelled as a text
// leaf is: a bare name, beside its "|formatting" if it has one, or a coded
// name as "|code", "|value" and "|terminology". Decode names the composition
// from it, and after the Web Template root, or the template id where the root
// has no name, when the body has none. Encode writes it unless the name is
// plain text, uncoded and with nothing but its value, equal to that root name.
// A "_name" under any other node is unmatched content.
//
// # Attributes with no Web Template node
//
// The Simplified Formats spell four RM attributes the Web Template gives no
// node of its own, and the codecs read and write them: an event's sample
// count as "<event>|sample_count", at an EVENT or INTERVAL_EVENT node; an
// interval event's "<event>/math_function" and "<event>/width" at an abstract
// EVENT node, which an INTERVAL_EVENT node has as children; and an
// OBSERVATION's history origin as "<observation>/history_origin". At an
// abstract EVENT node an event the body gives any of those three keys decodes
// as an INTERVAL_EVENT, and any other as a POINT_EVENT. Decode reads
// history_origin ahead of the origin [WithTemplate] derives, and encode writes
// it unless it holds nothing but a value that is, as written, the earliest
// time the events carry. Any other
// spelling of the four, such as "<event>/sample_count", is unmatched content.
//
// # Unmatched content
//
// Input the decode does not read is unmatched: a key whose path the Web
// Template does not resolve, an underscore-prefixed segment that names no RM
// attribute this codec carries at that node, a suffix outside the suffix set
// of its datatype or underscore family, and a ctx/ field the codec does not
// read. STRUCTURED applies one rule at every level of the body: a member
// whose name the FLAT grammar does not read at that level is unmatched
// whatever its value holds, and is reported as that member; a name it reads
// keeps the shape refusals. A key the decode reads is never unmatched, even
// where the decode then refuses it: a PARTICIPATION `time`, a
// FEEDER_AUDIT_DETAILS `other_details` and `_instruction_details` have no
// carrier and fail the decode in every mode. So do, in the reference's
// in-context spelling and where the Web Template does not resolve them, an
// ACTION's ISM transition and time (a `current_state`, `transition` or
// `careflow_step` leaf or a `_reason` family under an `ism_transition` node,
// and the ACTION's own `time` leaf), which name RM data decode would otherwise
// drop or replace. Those keys and `_instruction_details` are paths the
// Simplified Formats define and this SDK does not model: their refusal matches
// [ErrNotModelled] as well as [ErrUnknownPath]. A leaf whose
// datatype has neither a suffix set nor a sub-path grammar refuses its bare
// value and any suffix naming an attribute of its RM class.
// So does malformed content: a value of the wrong shape, a missing required
// suffix (a lone bare value where the datatype has no bare spelling, a null
// required suffix, and with [WithTemplate] a coded text's |terminology the
// template does not determine; a coded text's |value or |terminology, or an
// ordinal's |value or |ordinal, that the template determines is taken from it
// and not missing), two spellings of one field the decode
// reads that disagree (such as `x` and `x|`; a lone `x|` reads as `x`), an
// index past its bound, and a key that is not well formed in the FLAT grammar,
// with an empty path segment ("a//b|"), a segment with no id before its
// :index, a second |suffix or a segment after its |suffix. A value that does not fit its RM attribute, one the
// canonical-JSON decode of that attribute would refuse, is refused with
// [ErrUnsupportedDatatype] naming the FLAT key.
//
// A JSON null held by a key the decode reads is absent, never the zero of its
// type: a leaf whose keys are all null is not built, a null ctx/ field is a
// missing one, and a null key the decode would refuse for want of a carrier
// carries nothing to refuse. An empty string at the bare value of a
// DV_DATE_TIME, DV_DATE, DV_TIME, DV_DURATION, DV_URI or DV_EHR_URI leaf is
// absent the same way where no other key decorates that value, since encode
// writes no key for it, and so is an empty ctx/time, which spells the start
// time, a DV_DATE_TIME. A key the decode does not read is unmatched
// whatever its value. In STRUCTURED a null member or array element reads the
// same way, and OPT-free conversion carries it to FLAT as a key holding null.
// Two spellings of a key the decode does not read are two unmatched keys. A
// key is judged segment by segment from the left: an index past the bound of a
// segment that resolves makes the key malformed, whatever follows it. In
// STRUCTURED the same holds for an element's position in its member's array,
// whatever the element holds.
//
// By default the decoder ignores unmatched content: the value holds the rest
// of the input and none of it, exactly as if the input had left those keys
// out. [WithUnmatched] passes each item, in the order the input gives it, to
// a caller function as its path: the key as written for FLAT, such as
// "vital_signs/body_temperature:0/any_event:0/temperature|x-note", and for
// STRUCTURED the member names from the root joined by "/", with each array
// element's position as a segment, such as
// "vital_signs/body_temperature/0/any_event/0/temperature/0/|x-note". The
// path names the item and carries none of its value. [WithStrict] refuses the
// decode at the first item with an error wrapping [ErrUnknownPath]. Strict
// never refuses an absent key.
//
// The options belong to one decode: concurrent decodes with different options
// do not see each other's policy. [FlatToStructured] and [StructuredToFlat]
// take no options; they restructure every key whatever it names.
package simplified

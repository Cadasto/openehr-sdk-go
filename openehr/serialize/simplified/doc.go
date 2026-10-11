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
// rminfo / typereg), openehr/terminology, openehr/template/webtemplate,
// openehr/templatecompile (the WithTemplate name/default source),
// openehr/serialize/canjson, a module-internal type-name helper, and the
// standard library. They never import the transport, auth, or client layers.
//
// Context output-form and exotic-datatype fallbacks are documented in
// deviations.md beside the package tests.
//
// # Unmatched content
//
// Input the decode does not read is unmatched: a key whose path the Web
// Template does not resolve, an underscore-prefixed segment that names no RM
// attribute this codec carries at that node, a suffix outside the suffix set
// of its datatype or underscore family, and a ctx/ field the codec does not
// read. A key the decode reads is never unmatched, even where the decode then
// refuses it: the composer's `external_ref` suffixes and party sub-structure,
// a PARTICIPATION `time`, a FEEDER_AUDIT_DETAILS `other_details`,
// `_instruction_details` and `_wf_definition` have no carrier and fail the
// decode in every mode. So do an ACTION's ISM transition and time in the
// reference's in-context spelling where the Web Template does not resolve them
// (a `current_state`, `transition` or `careflow_step` leaf or a `_reason`
// family under an `ism_transition` node, and the ACTION's own `time` leaf),
// which name RM data decode would otherwise drop or replace, and malformed
// content (a value of the wrong shape, a missing required suffix, two spellings
// of one field that disagree, such as `x` and `x|`, an index past its bound). A key is judged segment by segment from the left:
// an index past the bound of a segment that resolves makes the key malformed,
// whatever follows it.
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

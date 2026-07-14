// Package simplified converts an openEHR COMPOSITION to and from the FLAT
// and STRUCTURED Simplified Formats (REQ-053). These are the two variants
// named by the STABLE ITS-REST Simplified Formats spec; the older
// "Simplified Data Template" / simSDT / structSDT naming is superseded.
//
// These are serializations of a data instance, not a template. Field
// identifiers are the human-readable Web Template ids (REQ-106) — not
// canonical AQL/AOM paths — so conversion to and from canonical RM is
// template-specific and requires the composition's Web Template. Given the
// template the conversion is bidirectional and semantics-preserving: the
// simplified forms are not self-standing (they depend on the template),
// which is distinct from being lossy. Reconstructing the template/OPT from a
// data instance is out of scope.
//
// The codecs are a building block (REQ-013): they take an *rm.Composition
// and a *webtemplate.WebTemplate and return bytes (or vice versa), importing
// only openehr/rm (+ rmpath / rminfo / typereg), openehr/template/webtemplate,
// openehr/serialize/canjson, and the standard library — never the transport,
// auth, or client layers.
//
// Context output-form and exotic-datatype fallbacks are documented in
// deviations.md beside the package tests.
package simplified

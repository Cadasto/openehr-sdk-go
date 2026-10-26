package canjson

// WithMissing passes every missing member of the input to fn while the
// decode carries on. A member is missing when the Reference Model makes the
// attribute mandatory on the RM type its object decodes as, the class its
// `_type` names, or else the type the RM declares where it sits, as
// [WithUnmatched] reads it, and the object has no member of that name, or
// holds `null` there. The value the decode returns cannot always show that:
// a member held by value, a number, a boolean, a string or a value struct
// such as PARTY_SELF or ARCHETYPE_ID, decodes to its zero value whether the
// input carried it or not.
//
// A present member is never missing, whatever it holds: an empty string, a
// zero, an empty list or object and a bare {"_type":"PARTY_SELF"} are
// present. Three kinds of attribute are never missing either: one no
// serialization carries (a function the model computes, such as EVENT
// offset, and RESOURCE_DESCRIPTION parent_resource); the four interval
// boundary flags, which take their defaults when absent; and `_type`, which
// names no attribute.
//
// The option reports only. A missing member is never a decode error, with or
// without [WithStrict], which refuses unmatched members only, and the decode
// returns the same value as without the option, each missing member holding
// its zero value. fn receives each member once, as its JSON Pointer (RFC
// 6901) from the root of the value, such as
// "/content/0/data/events/0/data/items/0/value/magnitude": the attribute's
// name under the pointer of the object that lacks it, each item of a list by
// its position counted from 0 and each entry of a `Hash` by its key. The
// pointer carries no value from the input. The members of one object are
// reported as the object is read to its end, in the order the RM declares
// the attributes (an ancestor's before the class's own), so the members of
// the objects inside it come first.
//
// The members are checked once the decode has succeeded, against the RM's
// own attribute lists, in the same pass that serves [WithUnmatched]: fn runs
// after the value is filled, on the caller's goroutine, and a decode that
// fails reports nothing. What is checked is what [WithUnmatched] checks: the
// RM values reached from v's own RM shape. A value of a class outside the
// RM, such as an AOM 1.4 archetype, a BASE interval, a generic payload the
// decoder holds as plain JSON and a `Hash` of plain values are not checked,
// and nothing inside them is reported (see the package documentation,
// "Missing members").
//
// A nil fn is the same as leaving the option out.
func WithMissing(fn func(pointer string)) DecoderOption {
	return func(c *decoderConfig) { c.missing = fn }
}

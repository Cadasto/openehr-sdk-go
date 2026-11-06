package canxml

import (
	"strconv"

	"github.com/cadasto/openehr-sdk-go/internal/reportpath"
)

// Presence is the form in which a document sends a member without a value,
// as [WithPresence] reports it.
type Presence int

const (
	// PresenceNil is a member whose element carries `xsi:nil="true"` (or
	// `1`), whatever it holds.
	PresenceNil Presence = iota + 1
	// PresenceEmpty is a member whose element is a primitive element with no
	// text, or a string element whose text is XML whitespace only.
	PresenceEmpty
)

// String returns "nil" for [PresenceNil], "empty" for [PresenceEmpty], and
// "Presence(n)" for any other value.
func (p Presence) String() string {
	switch p {
	case PresenceNil:
		return "nil"
	case PresenceEmpty:
		return "empty"
	default:
		return "Presence(" + strconv.Itoa(int(p)) + ")"
	}
}

// WithPresence passes every member the input sends without a value to fn
// while the decode carries on. A member is sent without a value when its
// element is a member of the class its parent element decodes as, mandatory
// or optional, and the element carries `xsi:nil="true"` (or `1`), whatever
// it holds, which fn receives as [PresenceNil]; or is a primitive element
// whose text is empty, or XML whitespace only for a string, which fn
// receives as [PresenceEmpty]. An optional member sent so decodes as absent,
// a nil pointer, interface or slice, and a mandatory one to its zero value,
// as if the input had left it out (see the package documentation, "Wire
// profile"), so the value the decode returns cannot say that the input sent
// it, and [WithMissing] reports mandatory members only.
//
// A member that carries a value is not reported: an empty struct element
// such as `<subject xsi:type="PARTY_SELF"/>`, `<magnitude>0</magnitude>`,
// `xsi:nil="false"`, and an entry of a `Hash` member, whatever its text,
// unless it is nilled. Nor is an element that is no member: the root, an
// unmatched element (see [WithUnmatched]), an element in a foreign
// namespace, a second child element of a single-valued member, which the
// decode skips, keeping the first, and an element the schema accepts that no
// member holds, which the decode drops. Nothing inside a nilled element is
// reported. A number or boolean element whose text is whitespace only, and an
// empty element a primitive type refuses, such as TERM_MAPPING's `match`, are
// primitive failures, which fail the decode; a decode that fails makes no
// promise about which members it reported before the failure.
//
// The option reports only: no member sent without a value refuses the
// decode, with or without [WithStrict], and the decode returns the same value
// as without the option. fn receives each member once, as its element
// closes, so in document order, with its JSON Pointer (RFC 6901) in the
// canonical-JSON form of the instance, from the root of the value, as
// [WithMissing] names a member, such as
// "/content/0/data/events/0/data/items/0/value/precision". The pointer is a
// [Pointer], which writes its text only when asked, so a caller that only
// counts the members pays nothing for the text. It carries no value from the
// input. A mandatory member sent without a value
// is reported by both options, by one pointer, and an absent one by
// [WithMissing] alone.
//
// The check runs as the decode streams, in the pass that serves
// [WithUnmatched] and [WithMissing], so the options may be combined, and fn
// runs during the decode, on the caller's goroutine. A decode that fails
// makes no promise about which members it reported before the failure.
// Every class the generated decoders read is checked, the AOM 1.4 classes
// included. One member escapes the check, as it escapes [WithMissing]: a
// primitive already held in an interface-typed slot, which encoding/xml
// reads itself, counts as a value even when its element is empty.
// Canonical JSON has no counterpart to this option.
//
// The option applies to [Unmarshal] and to a [Decoder] from [NewDecoder],
// where each value's pointers are relative to that value. A nil fn is the
// same as leaving the option out.
func WithPresence(fn func(p Pointer, pr Presence)) DecoderOption {
	return func(c *decoderConfig) { c.presence = fn }
}

// reportPresence adapts fn to the tracker's hook, which says whether the
// member was nilled.
func reportPresence(fn func(p Pointer, pr Presence)) func(pointer *reportpath.Node, nilled bool) {
	if fn == nil {
		return nil
	}
	return func(pointer *reportpath.Node, nilled bool) {
		if nilled {
			fn(Pointer{node: pointer}, PresenceNil)
			return
		}
		fn(Pointer{node: pointer}, PresenceEmpty)
	}
}

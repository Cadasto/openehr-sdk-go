package typereg

// Tracking of unmatched members on the canonical-JSON decode path. A member is
// unmatched when its name is neither `_type` nor a member of the RM or AOM 1.4
// type its object decodes as. By default the decoder skips it, as
// encoding/json/v2 skips any member a struct does not declare. A caller can
// instead track every such member: report each one, in document order, as a
// JSON Pointer into the input, and optionally refuse the first one.
//
// How it works. A tracked decode carries one SDK-owned hook set, the one
// [trackedUnmarshalers] returns, in place of the plain aggregate. Seeing that
// exact set on the decoder is how [DecodeInto] and [DecodePolymorphic] know a
// decode is tracked; an untracked decode pays one pointer comparison and
// nothing else.
//
//   - [DecodeInto] decodes a tracked object through [withUnmatched], which adds
//     an embedded fallback map beside the wire struct. encoding/json/v2 hands
//     every member the wire struct does not declare to that map, in the order
//     the members appear, and the map's element hook ([unmatchedHook]) reports
//     the member and skips its value. So "unmatched" is exactly "not read by
//     the decoder", and the same struct definitions decide it in every mode.
//   - The hook finds the caller's policy through the decoder it runs on: each
//     tracked decoder is registered in [trackers] while it decodes. JSON
//     Pointers from a decoder are relative to that decoder's input, so each
//     registration carries the pointer of where that input sits in the whole
//     document.
//   - [DecodePolymorphic] decodes a slot's buffered value on a fresh decoder.
//     When tracked, it registers that decoder with the slot's pointer added to
//     the base, so a member inside a slot is still named from the document
//     root.
//
// This file imports no reflect: the fallback wrapper is a generic struct, so
// no Go type is built at run time (REQ-024).

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"sync"
)

// unmatchedTracker is one tracked decoder's policy: where its input sits in
// the document, the caller's report function and whether the first unmatched
// member refuses the decode.
type unmatchedTracker struct {
	base   string
	report func(pointer string)
	strict bool
}

// member applies the policy to the member at rel, a JSON Pointer relative to
// the tracked decoder's input. A member named `_type` is never unmatched, even
// on a type that declares no discriminator (the abstract BASE Interval).
func (t *unmatchedTracker) member(rel jsontext.Pointer) error {
	if rel.LastToken() == "_type" {
		return nil
	}
	pointer := t.base + string(rel)
	if t.report != nil {
		t.report(pointer)
	}
	if t.strict {
		return &unknownMemberError{pointer: pointer}
	}
	return nil
}

// at returns the policy for a value whose input starts at rel, a JSON Pointer
// relative to t's own input: the same policy, with rel added to the base.
func (t *unmatchedTracker) at(rel jsontext.Pointer) *unmatchedTracker {
	return &unmatchedTracker{base: t.base + string(rel), report: t.report, strict: t.strict}
}

// trackers maps each decoder taking part in a tracked decode to its policy,
// for exactly as long as that decoder decodes the tracked value.
var trackers sync.Map // *jsontext.Decoder → *unmatchedTracker

// trackerOf returns the policy registered for dec. A decoder that is not
// registered gets an empty policy, which skips every member as an untracked
// decode does; no route reaches that case, since only a registered decoder
// carries the tracked hook set.
func trackerOf(dec *jsontext.Decoder) *unmatchedTracker {
	if t, ok := trackers.Load(dec); ok {
		return t.(*unmatchedTracker)
	}
	return &unmatchedTracker{}
}

// unmatchedMember is the element type of the fallback map; [unmatchedHook]
// decodes every element, so the type itself holds nothing.
type unmatchedMember struct{}

// unmatchedMembers is the embedded fallback encoding/json/v2 fills with the
// members a wire struct does not declare.
type unmatchedMembers map[string]unmatchedMember

// unmatchedHook decodes one unmatched member: it applies the tracked
// decoder's policy to the member's pointer and skips the member's value.
var unmatchedHook = json.UnmarshalFromFunc(func(dec *jsontext.Decoder, _ *unmatchedMember) error {
	rel := dec.StackPointer()
	if err := dec.SkipValue(); err != nil {
		return err
	}
	return trackerOf(dec).member(rel)
})

// withUnmatched is the decode target of a tracked object: the wire struct,
// embedded, plus the fallback map that catches every member the wire struct
// does not declare. Both fields use the `embed` option, so the wire struct's
// members stay at the top level of the object.
type withUnmatched[T any] struct {
	Value     *T               `json:",embed"`
	Unmatched unmatchedMembers `json:",embed"`
}

// trackedValue is the decode target that starts tracking: decoding into it
// registers the decoder with the policy, then decodes target under the
// tracked hook set.
type trackedValue struct {
	target  any
	tracker *unmatchedTracker
}

// UnmarshalJSONFrom decodes the next value into the tracked target while the
// decoder is registered with the policy. Every decoder it registers is one the
// tracked decode owns for that value: the caller's own decoder for the value
// being decoded, or the fresh decoder of one polymorphic slot.
func (v *trackedValue) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	trackers.Store(dec, v.tracker)
	defer trackers.Delete(dec)
	return json.UnmarshalDecode(dec, v.target, json.WithUnmarshalers(trackedUnmarshalers()))
}

// TrackUnmatched returns a decode target that decodes into v and tracks its
// unmatched members: the members, at any depth, whose name is neither `_type`
// nor a member of the RM or AOM 1.4 type their object decodes as. Each one is
// named by a JSON Pointer (RFC 6901) into the decoded value; the pointer
// carries no value from the input.
//
// report, when not nil, receives every unmatched member, once, in document
// order, while the decode carries on and fills v as an untracked decode would.
// strict refuses the first unmatched member: the decode stops with an error
// wrapping [ErrUnknownMember] that names the member's pointer (after report
// has seen it). Strict covers unmatched members only: an absent member is
// never refused.
//
// Decode the result with encoding/json/v2 (json.Unmarshal or
// json.UnmarshalDecode), as canjson does. The tracked decode runs under the
// SDK's own polymorphic hooks, in place of any hook set the caller passed. A
// strict refusal reaches the caller inside the decoder's own error
// envelopes; [UnwrapUnknownMember] returns it alone.
//
// Members of a Go struct that is not an RM or AOM 1.4 type are not tracked:
// they are skipped, as an untracked decode skips them.
func TrackUnmatched(v any, report func(pointer string), strict bool) any {
	return &trackedValue{target: v, tracker: &unmatchedTracker{report: report, strict: strict}}
}

package canjson

import (
	"errors"
	"slices"
	"strconv"

	"github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk"
)

// ErrUnknownMember is the sentinel for a member the decode does not read: its
// name is neither `_type` nor an attribute of the RM type its object decodes
// as. Such a member is ignored by default. Only a decode given [WithStrict]
// refuses one: the refusal wraps this sentinel and names the member's JSON
// Pointer from the root of the value (see [WithUnmatched]). The pointer
// carries the member's name but none of its value.
//
// It is distinct from [ErrInvalidShape] and [ErrInvalidValue]: errors.Is
// against one of the three never matches an error raised for another.
var ErrUnknownMember = errors.New("canjson: unknown member")

// unknownMemberError is the [WithStrict] refusal of an unmatched member. It
// names the member by its JSON Pointer and carries nothing else from the
// input, so the member's value never reaches the message.
type unknownMemberError struct {
	pointer string
}

// Error names the member's pointer, quoted so a name holding a control
// character cannot break the line it is logged on.
func (e *unknownMemberError) Error() string {
	return ErrUnknownMember.Error() + " " + strconv.Quote(e.pointer)
}

// Unwrap returns [ErrUnknownMember], the classification the refusal carries.
func (e *unknownMemberError) Unwrap() error {
	return ErrUnknownMember
}

// WithUnmatched passes every unmatched member of the input to fn while the
// decode carries on. A member is unmatched when its name is neither `_type`
// nor an attribute of the RM type its object decodes as: the class its
// `_type` names, or else the type the RM declares where it sits. At the root
// that is the RM type v points to, or for a slice, array or map of RM values
// the RM type of its elements; a generic RM type such as
// rm.DVInterval[rm.DVQuantity] also declares its bounds' class. It applies at
// any depth: inside a polymorphic slot, an interval bound and a `Hash` of RM
// values too.
//
// The walk starts from v's own RM shape: an RM class or a polymorphic RM
// interface, or a slice, array or map of one. When v is none of these, as
// with a pointer to any, to a map[string]any or to a type of the caller's
// own, the option reports nothing: RM values held inside a type of the
// caller's own are not checked.
// The members of a value of a class outside the RM, such as an AOM 1.4
// archetype, are not checked either (see the package documentation,
// "Unmatched members"). Nor are those of a BASE interval, such as an
// rm.Interval[rm.Integer], whether v points to one or it is the value of a
// member: the RM lists that interval among its primitive types, not its
// classes. Its descendants are classes and are checked: DV_INTERVAL
// (rm.DVInterval), and the point and proper intervals (rm.PointInterval,
// rm.ProperInterval).
//
// By default the decoder ignores unmatched members: the value it returns
// holds the rest of the input and none of them. With this option fn receives
// each one, once, in the order it appears in the input, and the value
// returned is the same as without the option. The members are checked against
// the RM's own attribute lists once the decode has succeeded: fn runs after
// the value is filled, on the caller's goroutine, and a decode that fails
// reports nothing.
//
// fn gets the member's JSON Pointer (RFC 6901) from the root of the value,
// such as "/content/0/data/x-note". The pointer carries the member's name,
// never its value.
//
// A nil fn is the same as leaving the option out. Combined with
// [WithStrict], fn sees the first unmatched member before the decode is
// refused.
func WithUnmatched(fn func(pointer string)) DecoderOption {
	return func(c *decoderConfig) { c.unmatched = fn }
}

// WithStrict refuses the decode at its first unmatched member (see
// [WithUnmatched] for what is unmatched and for the form of its pointer). The
// error wraps [ErrUnknownMember] and names the member's pointer from the root
// of the value; it carries none of the member's value. The value v points to
// is unspecified after the refusal.
//
// Strict covers unmatched members only. An absent member is never refused,
// mandatory or not, and neither is a boundary flag an interval leaves out.
// The members are checked after the decode succeeds, so a decode error, such
// as an unknown `_type`, is returned as itself in every mode, wherever it
// sits in the input (see [DecodeError]).
//
// By default the decoder ignores unmatched members.
func WithStrict() DecoderOption {
	return func(c *decoderConfig) { c.strict = true }
}

// checksMembers reports whether the decode reports or refuses unmatched
// members.
func (c *decoderConfig) checksMembers() bool {
	return c.unmatched != nil || c.strict
}

// checkUnmatched applies opts to the members of data, which the default
// decode has just read into v without error. Options that neither report nor
// refuse unmatched members, nil ones included, leave the decode as it is.
func checkUnmatched(data []byte, v any, opts []DecoderOption) error {
	if !slices.ContainsFunc(opts, func(o DecoderOption) bool { return o != nil }) {
		return nil
	}
	c := newDecoderConfig(opts)
	if !c.checksMembers() {
		return nil
	}
	return c.walkUnmatched(data, v)
}

// walkUnmatched walks data, the input the default decode has just read into v
// without error, against the RM's attribute lists, and reports or refuses each
// unmatched member as c says. v's Go type gives the root's shape: the RM type
// each value at the root is declared as, inside any slices and maps. A target
// whose own shape is no RM class is not walked.
func (c *decoderConfig) walkUnmatched(data []byte, v any) error {
	model, err := rmjsonwalk.Default()
	if err != nil {
		return err
	}
	root, ok := model.RootOf(v)
	if !ok {
		return nil
	}
	return model.Unmatched(data, root, func(pointer string) error {
		if c.unmatched != nil {
			c.unmatched(pointer)
		}
		if c.strict {
			return &unknownMemberError{pointer: pointer}
		}
		return nil
	})
}

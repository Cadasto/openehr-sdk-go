package typereg

// Streaming (encoding/json/v2) runtime for the generated canonical-JSON
// codec. The generator emits a MarshalJSONTo / UnmarshalJSONFrom pair per
// concrete RM/AOM type and one json.UnmarshalFromFunc hook per polymorphic
// interface; both funnel through the helpers here so the _type discipline,
// the deterministic encode options and the shape classification live in one
// place rather than in 110 generated bodies (ADR 0022, REQ-052).
//
// This file imports encoding/json/v2 and encoding/json/jsontext only. It must
// not import openehr/rm: the generated trees import typereg, so a reverse edge
// would form a cycle. Everything here works through Default and the sentinels
// declared in this package.

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"sync"

	json "encoding/json/v2"
)

// decodeOptions are the options every nested decode threads: the caller's own
// options, the polymorphic interface hooks, and — regardless of whether a v1 or
// v2 caller drives the outermost decode — v2 error semantics, so a hook failure
// carries its JSON position (which [classifyDecode] lifts onto the DecodeError
// path). A v1 caller's DefaultOptionsV1 sets ReportErrorsWithLegacySemantics,
// which otherwise returns a hook error position-less; the generated codec is
// v2-native, so it reports positions the same way from either entry point.
func decodeOptions(dec *jsontext.Decoder) json.Options {
	return json.JoinOptions(
		dec.Options(),
		Unmarshalers(),
		jsonv1.ReportErrorsWithLegacySemantics(false),
	)
}

// unmarshalers is the process-wide aggregate of per-interface decode hooks.
// The rm and aom14 packages each register their json.UnmarshalFromFunc hooks
// into it at package init (via [RegisterUnmarshaler]); [Unmarshalers] hands
// the joined option to every nested decode so a polymorphic slot resolves from
// any entry point, including a bare encoding/json/v2 Unmarshal with no options.
var (
	unmarshalersMu     sync.RWMutex
	unmarshalerHooks   []*json.Unmarshalers
	unmarshalersJoined json.Options
)

// RegisterUnmarshaler adds one per-interface decode hook to the aggregate.
// The generator emits one call per polymorphic interface from the owning
// package's init(); the interface's dispatch runs through [DecodePolymorphic].
// A nil hook is ignored so a partially generated tree cannot panic here.
func RegisterUnmarshaler(hook *json.Unmarshalers) {
	if hook == nil {
		return
	}
	unmarshalersMu.Lock()
	defer unmarshalersMu.Unlock()
	unmarshalerHooks = append(unmarshalerHooks, hook)
	unmarshalersJoined = json.WithUnmarshalers(json.JoinUnmarshalers(unmarshalerHooks...))
}

// Unmarshalers returns the aggregate of every registered interface hook as a
// single json.Options, ready to join into a decode. It is safe for concurrent
// use; the option is rebuilt only when a hook registers (during init). A tree
// with no registered hooks returns an empty, harmless option.
func Unmarshalers() json.Options {
	unmarshalersMu.RLock()
	defer unmarshalersMu.RUnlock()
	if unmarshalersJoined == nil {
		return json.WithUnmarshalers(nil)
	}
	return unmarshalersJoined
}

// MarshalOptions returns the encode options every generated MarshalJSONTo
// joins on top of the encoder's own options: json.Deterministic(true) so a
// map (openEHR Hash) emits its keys in lexicographic order (REQ-052), and
// FormatNilSliceAsNull / FormatNilMapAsNull so a mandatory nil container keeps
// its v1 "null" spelling instead of v2's default "[]" / "{}" (Q6). The three
// are joined afresh in every method so the spelling holds no matter which
// package (v1 or v2, canonical or a stay-on-v1 caller) drives the encode.
func MarshalOptions(enc *jsontext.Encoder) json.Options {
	return json.JoinOptions(
		enc.Options(),
		json.Deterministic(true),
		json.FormatNilSliceAsNull(true),
		json.FormatNilMapAsNull(true),
	)
}

// DecodeInto is the shared decode body for a concrete type's
// UnmarshalJSONFrom. It reads the next value, refuses a _type that names a
// different concrete type (the mismatch carries [ErrTypeMismatch] inside a
// [DecodeError] on "/_type"), then decodes the raw bytes into out with the
// interface hooks threaded in, wrapping any whole-value shape failure through
// [WrapShapeError] so it keeps the `canjson: <rmType>:` text and gains
// [ErrInvalidShape] (REQ-052).
//
// out is either the receiver viewed through its method-free alias (the
// zero-copy shape) or a flat wire struct the caller copies back (the shape used
// for a type that embeds a marshaler-bearing ancestor, whose alias would
// promote the ancestor's methods). Either way out declares a _type field, so a
// caller who sets json.RejectUnknownMembers is not tripped by the SDK's own
// discriminator reaching out (Q5).
func DecodeInto(dec *jsontext.Decoder, rmType string, out any) error {
	raw, err := dec.ReadValue()
	if err != nil {
		// A syntactic or IO error from the tokenizer is left unwrapped: it is
		// not a shape failure of this type, and callers classify it by kind.
		return err
	}
	if head, err := peekType(raw); err != nil {
		return WrapShapeError(rmType, err)
	} else if head != "" && head != rmType {
		return &DecodeError{
			Path:  "/_type",
			Inner: fmt.Errorf("canjson: expected %q, got %q: %w", rmType, head, ErrTypeMismatch),
		}
	}
	if err := json.Unmarshal(raw, out, decodeOptions(dec)); err != nil {
		return classifyDecode(rmType, err)
	}
	return nil
}

// classifyDecode turns the error from a nested decode into the codec's error
// contract (REQ-052). A dispatch failure at a polymorphic slot arrives as a
// [DecodeError] a hook produced with no Path; this fills the Path from the v2
// decoder's SemanticError position (the slot relative to the current object)
// and lets [WrapShapeError] leave it outside the shape sentinel. Any other
// failure is a whole-value or plain-field shape failure and goes through
// WrapShapeError, which adds [ErrInvalidShape] and the `canjson: <rmType>:`
// text.
func classifyDecode(rmType string, err error) error {
	var de *DecodeError
	if errors.As(err, &de) && de.Path == "" {
		if p := slotPointer(err); p != "" {
			return WrapShapeError(rmType, &DecodeError{Path: p, Type: de.Type, Inner: de.Inner})
		}
	}
	return WrapShapeError(rmType, err)
}

// slotPointer returns the JSON pointer of the outermost SemanticError in the
// chain — the position, relative to the value the current decode is filling,
// where a nested UnmarshalerFrom or hook failed. Empty when no position is
// available.
func slotPointer(err error) string {
	if se, ok := errors.AsType[*json.SemanticError](err); ok && se != nil {
		return string(se.JSONPointer)
	}
	return ""
}

// DecodePolymorphic is the shared body of every per-interface decode hook. It
// reads the next value, resolves the concrete type from its _type through
// [Default], decodes into a fresh instance (threading the interface hooks so a
// nested polymorphic slot resolves too), and stores it in out. A missing _type
// falls back to fallback when the interface is narrow (the parent concrete type
// is the natural default at a concrete-typed slot, wire.md:147); an abstract
// interface passes a nil fallback and refuses the missing discriminator.
//
// Dispatch failures (missing / unknown / mismatched _type) return a
// [DecodeError] wrapping the matching sentinel, so errors.Is finds the kind and
// errors.As finds the envelope; the JSON position is supplied by the v2
// decoder's own SemanticError wrapper around this return (REQ-052, Q2).
func DecodePolymorphic[T any](dec *jsontext.Decoder, out *T, fallback func() any) error {
	raw, err := dec.ReadValue()
	if err != nil {
		return err
	}
	if string(raw) == "null" {
		// A JSON null leaves the interface slot at its zero value (nil): the
		// old per-field decode skipped null before dispatch, and a narrow slot
		// must not synthesise an empty fallback instance from it, which would
		// break round-trip stability (an absent value re-encoding as `{}`).
		return nil
	}
	if d := jsonNestingDepth(raw); d > maxDecodeDepth {
		return &DecodeError{Inner: fmt.Errorf("typereg.Decode: %w (%d > %d)", ErrMaxDepthExceeded, d, maxDecodeDepth)}
	}
	name, err := peekType(raw)
	if err != nil {
		return WrapShapeError("", err)
	}
	var v any
	switch {
	case name == "" && fallback != nil:
		v = fallback()
	case name == "":
		return &DecodeError{Inner: fmt.Errorf("typereg.Decode: %w", ErrMissingType)}
	default:
		ctor, ok := Default.Lookup(name)
		if !ok {
			return &DecodeError{Type: name, Inner: fmt.Errorf("typereg.Decode %q: %w", name, ErrUnknownType)}
		}
		v = ctor()
	}
	if err := json.Unmarshal(raw, v, decodeOptions(dec)); err != nil {
		// A shape failure inside the concrete type selected at this slot stays a
		// shape failure (it already carries ErrInvalidShape from the concrete's
		// own funnel); wrapping it in a DecodeError adds the slot classification
		// so both hold at once — the path from the envelope (filled by the
		// enclosing DecodeInto), the kind from the sentinel (REQ-052).
		return &DecodeError{Type: name, Inner: err}
	}
	tv, ok := v.(T)
	if !ok {
		return &DecodeError{Type: name, Inner: fmt.Errorf("typereg.DecodeAs: decoded %T: %w", v, ErrTypeMismatch)}
	}
	*out = tv
	return nil
}

// peekType reads the _type discriminator from a raw JSON object without
// decoding the whole value. An empty string means the member is absent; a
// non-object or malformed value returns the decode error for the caller to
// classify as a shape failure.
func peekType(raw jsontext.Value) (string, error) {
	var head struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return "", err
	}
	return head.Type, nil
}

package typereg

// Streaming (encoding/json/v2) runtime for the generated canonical-JSON
// codec. The generator emits a MarshalJSONTo / UnmarshalJSONFrom pair per
// concrete RM/AOM type and one json.UnmarshalFromFunc hook per polymorphic
// interface; both funnel through the helpers here so the _type discipline,
// the deterministic encode options and the shape classification live in one
// place rather than in 110 generated bodies (ADR 0022, REQ-052).
//
// This file imports the encoding/json family: v2 and jsontext for the codec,
// plus encoding/json (v1) for the one ReportErrorsWithLegacySemantics option. It
// must not import openehr/rm: the generated trees import typereg, so a reverse
// edge would form a cycle. Everything here works through Default and the
// sentinels declared in this package.

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

// decodeOptions are the options every nested decode threads: the caller's own
// options, the polymorphic interface hooks, and, regardless of whether a v1 or
// v2 caller drives the outermost decode, v2 error semantics, so a hook failure
// carries its JSON position (which [classifyDecode] lifts onto the DecodeError
// path). A v1 caller's DefaultOptionsV1 sets ReportErrorsWithLegacySemantics,
// which otherwise returns a hook error position-less; the generated codec is
// v2-native, so it reports positions the same way from either entry point.
//
// A caller may pass its own json.WithUnmarshalers to the outermost Unmarshal.
// Because WithUnmarshalers is single-valued, joining our aggregate on top would
// drop the caller's hooks for every nested value. So the caller's hooks are
// joined AFTER our aggregate (SDK first: the canonical-JSON polymorphic
// dispatch wins for the interfaces it covers; the caller's hooks reach every
// other type, at every depth). A nil caller (json.WithUnmarshalers(nil)) is
// treated as no caller. The pointer check keeps a nested decode, whose options
// already carry our aggregate, from re-joining it to itself. The join is
// memoised on the last caller hook set seen (see [joinCallerUnmarshalers]): a
// repeat of that set reuses one join, so sibling values at one nesting level and
// a later decode reusing the same WithUnmarshalers value cost nothing, but a cold
// decode still joins once per nesting level, because each level's options carry
// the previous level's joined pointer as that level's caller.
func decodeOptions(dec *jsontext.Decoder) json.Options {
	hooks := aggregateUnmarshalers()
	if caller, ok := json.GetOption(dec.Options(), json.WithUnmarshalers); ok && caller != nil && caller != hooks {
		hooks = joinCallerUnmarshalers(hooks, caller)
	}
	return json.JoinOptions(
		dec.Options(),
		json.WithUnmarshalers(hooks),
		jsonv1.ReportErrorsWithLegacySemantics(false),
	)
}

// joinEntry is one memoised caller-hook join: the aggregate and caller pointers
// that produced joined. It is the whole memo. [joinCallerUnmarshalers] keeps
// exactly one, published through an atomic.Pointer, so no caller's closures
// outlive the next distinct caller. The hooks pointer is part of the key so a
// late hook registration that swaps the aggregate misses rather than reuses a
// stale join; registration is init-time only, so in practice the aggregate is
// stable by the time any decode runs.
type joinEntry struct {
	hooks  *json.Unmarshalers
	caller *json.Unmarshalers
	joined *json.Unmarshalers
}

// lastCallerJoin holds the most recent (aggregate, caller) join and only that
// one: a single-entry, last-seen memo. It retains one caller pointer at a time,
// never a growing set, so a consumer that builds a fresh hook set per decode
// does not accumulate an entry per call for the process lifetime.
var lastCallerJoin atomic.Pointer[joinEntry]

// callerJoinCount counts the json.JoinUnmarshalers calls [joinCallerUnmarshalers]
// performs on a memo miss. It exists only so a white-box test can assert that a
// repeat of one caller hook set reuses its join; it is unexported and read only
// by that test.
var callerJoinCount atomic.Int64

// joinCallerUnmarshalers returns the SDK aggregate joined with a caller's hooks.
// It memoises only the last (aggregate, caller) pair, in [lastCallerJoin]: a call
// whose two pointers match the stored pair reuses that join; any other pair
// recomputes and overwrites. A repeat of one caller hook set therefore reuses a
// single join (sibling values at one nesting level, and a later decode reusing
// the same WithUnmarshalers value), while a cold decode still joins once per
// nesting level, because each level's options carry the previous level's joined
// pointer as that level's caller. Two concurrent misses may both join; the
// results are equivalent, so storing either is benign. SDK hooks win for the
// interfaces they cover; the caller's reach every other type at every depth (see
// [decodeOptions]).
func joinCallerUnmarshalers(hooks, caller *json.Unmarshalers) *json.Unmarshalers {
	if e := lastCallerJoin.Load(); e != nil && e.hooks == hooks && e.caller == caller {
		return e.joined
	}
	callerJoinCount.Add(1)
	joined := json.JoinUnmarshalers(hooks, caller)
	lastCallerJoin.Store(&joinEntry{hooks: hooks, caller: caller, joined: joined})
	return joined
}

// The process-wide aggregate of per-interface decode hooks. The rm and aom14
// packages each register their json.UnmarshalFromFunc hooks into it at package
// init. Registration serialises through unmarshalerRegMu (init-time only); the
// joined aggregate is published in an atomic.Pointer so [decodeOptions] reads
// it lock-free on the decode hot path.
var (
	unmarshalerRegMu sync.Mutex           // guards unmarshalerHooks during registration
	unmarshalerHooks []*json.Unmarshalers // append-only, init-time
	unmarshalerAgg   atomic.Pointer[json.Unmarshalers]
)

// RegisterUnmarshaler adds one per-interface decode hook to the aggregate.
// The generator emits one call per polymorphic interface from the owning
// package's init(); the interface's dispatch runs through [DecodePolymorphic].
// A nil hook is ignored so a partially generated tree cannot panic here.
// Registration is expected only at package init; it is safe for concurrent use
// but is not part of any hot path.
func RegisterUnmarshaler(hook *json.Unmarshalers) {
	if hook == nil {
		return
	}
	unmarshalerRegMu.Lock()
	defer unmarshalerRegMu.Unlock()
	unmarshalerHooks = append(unmarshalerHooks, hook)
	unmarshalerAgg.Store(json.JoinUnmarshalers(unmarshalerHooks...))
}

// aggregateUnmarshalers returns the joined SDK interface hooks, or nil when
// none are registered (JoinUnmarshalers treats a nil element as empty).
func aggregateUnmarshalers() *json.Unmarshalers {
	return unmarshalerAgg.Load()
}

// Unmarshalers returns the aggregate of every registered interface hook as a
// single json.Options. Kept for external callers; the SDK decode path uses
// [decodeOptions], which also honours a caller-supplied WithUnmarshalers.
func Unmarshalers() json.Options {
	return json.WithUnmarshalers(aggregateUnmarshalers())
}

// EncodeOptions returns the encoder-independent canonical-JSON encode option
// set: json.Deterministic(true) so a map (openEHR Hash) emits its keys in
// lexicographic order (REQ-052), and FormatNilSliceAsNull / FormatNilMapAsNull
// so a mandatory nil container keeps its "null" spelling instead of v2's
// default "[]" / "{}" (Q6). It is the single home for these three options: an
// entry point with no encoder in hand (canjson.Marshal / MarshalIndent) joins
// it directly, and [MarshalOptions] joins it on top of an encoder's own
// options. The set is rebuilt afresh on every call so the spelling holds no
// matter which package (v1 or v2, canonical or a stay-on-v1 caller) drives the
// encode.
func EncodeOptions() json.Options {
	return json.JoinOptions(
		json.Deterministic(true),
		json.FormatNilSliceAsNull(true),
		json.FormatNilMapAsNull(true),
	)
}

// MarshalOptions returns the encode options every generated MarshalJSONTo
// joins on top of the encoder's own options: the encoder's own options first,
// then [EncodeOptions] so the deterministic-map and nil-container spellings
// hold from any entry point.
func MarshalOptions(enc *jsontext.Encoder) json.Options {
	return json.JoinOptions(enc.Options(), EncodeOptions())
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
		// A duplicate object member name is a shape refusal (REQ-052), so it is
		// classified here: a caller driving this generated method through
		// encoding/json/v2 directly then gets ErrInvalidShape just as a canjson
		// entry point does, and the canjson and registry routes re-apply the
		// same gate idempotently. Any other syntactic or IO error from the
		// tokenizer is left unwrapped: it is not a shape failure of this type,
		// and callers classify it by kind.
		return ClassifyDuplicate(err)
	}
	// Bound recursion the same way [Registry.Decode] and [DecodePolymorphic]
	// do (REQ-040). This is the shared body of every generated
	// UnmarshalJSONFrom, so the check reaches every canonical-JSON decode
	// route — canjson.Unmarshal, canjson.Decoder.Decode and a bare v2/v1
	// caller — not only the registry path. At the outermost type raw is the
	// whole document, so the top-level call rejects an over-deep value before
	// it recurses; nested calls re-measure their own subtree, a redundancy the
	// single-pass decode follow-up can fold away.
	if d := jsonNestingDepth(raw); d > maxDecodeDepth {
		return &DecodeError{Inner: fmt.Errorf("typereg.Decode: %w (%d > %d)", ErrMaxDepthExceeded, d, maxDecodeDepth)}
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
	if de, ok := errors.AsType[*DecodeError](err); ok && de != nil && de.Path == "" {
		if p := slotPointer(err); p != "" {
			return WrapShapeError(rmType, &DecodeError{Path: p, Type: de.Type, Inner: de.Inner})
		}
	}
	return WrapShapeError(rmType, err)
}

// slotPointer returns the JSON pointer of the outermost SemanticError in the
// chain: the position, relative to the value the current decode is filling,
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
		// Same rule as [DecodeInto]: a duplicate member name is a shape refusal
		// (REQ-052); any other tokenizer error is left for the caller to
		// classify by kind.
		return ClassifyDuplicate(err)
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
		// The slot value is not a JSON object (e.g. an array where an RM value
		// is expected): a shape failure. Name the interface being filled so the
		// message reads `canjson: <Interface>:` rather than a bare colon.
		return WrapShapeError(reflect.TypeFor[T]().Name(), err)
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
		// so both hold at once: the path from the envelope (filled by the
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

// typeDiscriminator is the head struct peekType decodes into. It is a named
// unexported type so a decode failure names `typereg.typeDiscriminator` rather
// than leaking an anonymous `struct { Type string … }` Go spelling into a
// consumer-visible message.
type typeDiscriminator struct {
	Type string `json:"_type"`
}

// peekType reads the _type discriminator from a raw JSON object without
// decoding the whole value. An empty string means the member is absent; a
// non-object or malformed value returns the decode error for the caller to
// classify as a shape failure.
func peekType(raw jsontext.Value) (string, error) {
	var head typeDiscriminator
	if err := json.Unmarshal(raw, &head); err != nil {
		return "", err
	}
	return head.Type, nil
}

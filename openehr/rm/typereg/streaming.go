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
	unmarshalers, legacySemantics := hookOptions(hooks)
	return json.JoinOptions(dec.Options(), unmarshalers, legacySemantics)
}

// hookOptions returns the SDK's two decode options for a resolved interface-hook
// aggregate: the hooks themselves, and v2 error semantics (so a hook failure
// carries its JSON position regardless of whether a v1 or v2 caller drove the
// outermost decode). [decodeOptions] joins them on top of the caller's decoder
// options in one JoinOptions; [Registry.Decode] passes them straight to the
// variadic json.Unmarshal, because a []byte decode carries no caller options.
// Building the pair in one place keeps the two call sites from drifting, and
// returning them unjoined keeps decodeOptions to a single JoinOptions.
func hookOptions(hooks *json.Unmarshalers) (unmarshalers, legacySemantics json.Options) {
	return json.WithUnmarshalers(hooks), jsonv1.ReportErrorsWithLegacySemantics(false)
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
// UnmarshalJSONFrom. It decodes the next value straight into out in one pass,
// then refuses a _type that names a different concrete type: gotType points at
// the wrapper's declared _type field, which the same single decode populates,
// so the discriminator is read once rather than by a separate peek. The
// mismatch carries [ErrTypeMismatch] inside a [DecodeError] on "/_type". A
// whole-value shape failure goes through [WrapShapeError], keeping the
// `canjson: <rmType>:` text and gaining [ErrInvalidShape]; an error carrying
// neither a shape failure (a *json.SemanticError) nor a dispatch failure (a
// [DecodeError]) passes through unwrapped, because it is malformed input (a
// syntactic error or a failing reader), not a shape failure of this type
// (REQ-052, ADR 0022). Two errors are exceptions to that rule. A duplicate
// member name is a syntactic error that is nonetheless a shape refusal, so it
// gains [ErrInvalidShape] through [ClassifyDuplicate] (without the
// `canjson: <rmType>:` text). A depth refusal ([ErrMaxDepthExceeded], REQ-108)
// from a nested value passes through unwrapped and does not gain the shape
// sentinel.
//
// out is either the receiver viewed through its method-free alias (the
// zero-copy shape) or a flat wire struct the caller copies back (the shape used
// for a type that embeds a marshaler-bearing ancestor, whose alias would
// promote the ancestor's methods). Either way out declares the _type field
// gotType points into, so the guard reads the value this decode populated, and
// a caller who sets json.RejectUnknownMembers is not tripped by the SDK's own
// discriminator reaching out (Q5).
//
// A nil dec, a nil out (the interface itself, not a typed nil inside it) or a
// nil gotType is caller misuse (no generated body passes one); each is refused
// before any decode with a [DecodeError] wrapping [ErrNilArgument] that names
// the argument, rather than dereferenced (REQ-025). A typed nil inside out
// reaches the decode and is reported by encoding/json/v2 as a shape failure.
func DecodeInto(dec *jsontext.Decoder, rmType string, out any, gotType *string) error {
	switch {
	case dec == nil:
		return nilArgument("DecodeInto", "dec", rmType)
	case out == nil:
		return nilArgument("DecodeInto", "out", rmType)
	case gotType == nil:
		return nilArgument("DecodeInto", "gotType", rmType)
	}
	// Bound recursion the same way [Registry.Decode] and [DecodePolymorphic]
	// do (REQ-108). This is the shared body of every generated
	// UnmarshalJSONFrom, so the check reaches every canonical-JSON decode
	// route, a bare v2/v1 caller included. With no buffered value to measure,
	// the decoder's own stack depth says where this value opens in the
	// document, and each nested RM value re-checks on entry. A buffered
	// polymorphic slot is decoded on a fresh decoder, so [DecodePolymorphic]
	// adds the slot's own nesting to the enclosing depth before it dispatches.
	// Members the target type does not declare are skipped by the tokenizer
	// without reaching this check; outside a buffered value they are bounded
	// only by jsontext's own nesting limit, while inside a polymorphic slot
	// [DecodePolymorphic]'s bracket count over the buffered slot includes them
	// (REQ-108).
	if d := dec.StackDepth() + 1; d > maxDecodeDepth {
		return &DecodeError{Inner: fmt.Errorf("typereg.Decode: %w (%d > %d)", ErrMaxDepthExceeded, d, maxDecodeDepth)}
	}
	if err := json.UnmarshalDecode(dec, out, decodeOptions(dec)); err != nil {
		return classifyDecode(rmType, err)
	}
	if *gotType != "" && *gotType != rmType {
		return &DecodeError{
			Path:  "/_type",
			Inner: fmt.Errorf("canjson: expected %q, got %q: %w", rmType, *gotType, ErrTypeMismatch),
		}
	}
	return nil
}

// nilArgument builds the [ErrNilArgument] refusal for a nil argument to fn,
// naming the argument and the type being decoded.
func nilArgument(fn, arg, typeName string) error {
	return &DecodeError{Inner: fmt.Errorf("typereg.%s: %w: %s for %s", fn, ErrNilArgument, arg, typeName)}
}

// classifyDecode turns the error from a nested decode into the codec's error
// contract (REQ-052). encoding/json/v2 wraps a returned error in a
// *json.SemanticError unless it is already semantic, syntactic or an IO error
// (go doc encoding/json/v2.UnmarshalerFrom). So the codec's own shape and
// dispatch failures are exactly the errors whose chain carries a
// *json.SemanticError or a [DecodeError]; every other error is malformed input,
// a jsontext.SyntacticError (bad tokens, a duplicate name, a truncated value) or
// a failing reader's IO error, and passes through unwrapped so it keeps no SDK
// sentinel and no `canjson: <rmType>:` prefix. Two errors are exceptions. A
// duplicate member name reaches here as a SyntacticError but is a shape refusal
// (REQ-052), so it goes through [ClassifyDuplicate] here, where a caller driving
// a generated method through bare encoding/json/v2 still sees it. A depth
// refusal ([ErrMaxDepthExceeded], REQ-108) from a nested value passes through
// as it is: it is not a shape failure of this type, and re-wrapping it at every
// enclosing level would repeat the path per level, growing the message with
// the square of the depth.
//
// A dispatch failure at a polymorphic slot arrives as a [DecodeError] a hook
// produced with no Path; this fills the Path from the v2 decoder's SemanticError
// position (the slot relative to the current object) and lets [WrapShapeError]
// leave it outside the shape sentinel. Any other shape failure is a whole-value
// or plain-field one and goes through WrapShapeError, which adds [ErrInvalidShape]
// and the `canjson: <rmType>:` text.
func classifyDecode(rmType string, err error) error {
	if errors.Is(err, jsontext.ErrDuplicateName) {
		return ClassifyDuplicate(err)
	}
	if errors.Is(err, ErrMaxDepthExceeded) {
		return err
	}
	se, hasSemantic := errors.AsType[*json.SemanticError](err)
	de, hasDecode := errors.AsType[*DecodeError](err)
	if (!hasSemantic || se == nil) && (!hasDecode || de == nil) {
		return err
	}
	if hasDecode && de != nil && de.Path == "" {
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
//
// A nil dec or a nil out is caller misuse (no generated hook passes one),
// refused before any read with a [DecodeError] wrapping [ErrNilArgument]
// (REQ-025).
func DecodePolymorphic[T any](dec *jsontext.Decoder, out *T, fallback func() any) error {
	switch {
	case dec == nil:
		return nilArgument("DecodePolymorphic", "dec", reflect.TypeFor[T]().Name())
	case out == nil:
		return nilArgument("DecodePolymorphic", "out", reflect.TypeFor[T]().Name())
	}
	raw, err := dec.ReadValue()
	if err != nil {
		// Same rule as [classifyDecode]: a duplicate member name is a shape
		// refusal (REQ-052); any other tokenizer error is left for the caller
		// to classify by kind.
		return ClassifyDuplicate(err)
	}
	if string(raw) == "null" {
		// A JSON null leaves the interface slot at its zero value (nil): the
		// old per-field decode skipped null before dispatch, and a narrow slot
		// must not synthesise an empty fallback instance from it, which would
		// break round-trip stability (an absent value re-encoding as `{}`).
		return nil
	}
	// Bound recursion from the document root, not from the slot (REQ-108).
	// The slot's value is decoded below on a fresh decoder over raw, whose
	// stack depth starts again at zero, so the concrete values inside it
	// measure their depth from the slot. Adding the enclosing decoder's stack
	// depth to the slot's own nesting keeps a chain of polymorphic slots, each
	// shallow on its own, from nesting past the cap in total.
	if d := dec.StackDepth() + jsonNestingDepth(raw); d > maxDecodeDepth {
		return &DecodeError{Inner: fmt.Errorf("typereg.Decode: %w (%d > %d)", ErrMaxDepthExceeded, d, maxDecodeDepth)}
	}
	// The peek honours the caller's decoder options (REQ-052): ReadValue above
	// already tokenised the slot under them, so a v1 caller's leniencies
	// (duplicate names last-wins, invalid UTF-8 accepted, case-insensitive
	// `_type`) hold inside the slot exactly as they do at top level, and the
	// peek and the concrete decode below agree on which member is `_type`.
	// Because the peek can no longer be stricter than the tokenizer that
	// accepted raw, a peek failure here is a slot value that is not a JSON
	// object (e.g. an array where an RM value is expected), whose `_type`
	// is not a string, or whose `_type` a caller-registered unmarshal hook
	// refused: a shape failure in each case. Name the interface being
	// filled so the message reads
	// `canjson: <Interface>:` rather than a bare colon.
	name, err := peekType(raw, dec.Options())
	if err != nil {
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
// classify. opts are the decoder options the peek runs under: [DecodePolymorphic]
// passes the enclosing decoder's own, so the peek tokenises the slot exactly as
// the caller's decode did; [Registry.Decode] passes none, keeping its
// encoding/json/v2 default strictness. RejectUnknownMembers is switched off on
// top of opts, because the head struct declares only _type and every other
// member of an RM value is unknown to it (a caller's RejectUnknownMembers is
// meant for the concrete decode, not for this head). The override is joined
// only when the caller set the option, so the common path adds no allocation.
// Running under the caller's options also means any unmarshal hook the caller
// registered for string values runs on the `_type` member during the peek, and
// again during the concrete decode.
func peekType(raw jsontext.Value, opts ...json.Options) (string, error) {
	var head typeDiscriminator
	for _, o := range opts {
		if reject, ok := json.GetOption(o, json.RejectUnknownMembers); ok && reject {
			opts = append(opts, json.RejectUnknownMembers(false))
			break
		}
	}
	if err := json.Unmarshal(raw, &head, opts...); err != nil {
		return "", err
	}
	return head.Type, nil
}

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
// treated as no caller. The pointer check keeps a decoder that already carries
// our aggregate but reports legacy errors (a v1 caller passing [Unmarshalers])
// from having the aggregate joined to itself. The join is memoised on the last
// caller hook set seen (see [joinCallerUnmarshalers]): a repeat of that set
// reuses one join, so sibling values at one nesting level and a later decode
// reusing the same WithUnmarshalers value cost nothing.
//
// The result is nil when dec already carries everything it would add, which is
// the common case for every value below the outermost: json.UnmarshalDecode and
// json.Unmarshal apply the options a call passes to the decoder for that call's
// duration, so a nested value's decoder holds the hooks and error semantics its
// parent added. That is the case when dec does not report errors with legacy
// semantics and its hook set is either our aggregate or the join
// [lastCallerJoin] last produced for it. Recognising the join keeps a cold decode
// to one join in all, rather than one per nesting level with each level joining
// our aggregate onto the level above's join. Every json/v2 entry point skips a
// nil option, so [DecodeInto] passes the result as it is; [DecodePolymorphic]
// decodes on a fresh decoder and passes dec's own options in its place.
// Otherwise the result is dec's options with the hooks and v2 error semantics
// joined on top: the outermost value under a bare v2 or v1 call, or a nested one
// whose memo entry another caller's decode has displaced, which joins again.
func decodeOptions(dec *jsontext.Decoder) json.Options {
	hooks := aggregateUnmarshalers()
	opts := dec.Options()
	caller, ok := json.GetOption(opts, json.WithUnmarshalers)
	if legacy, _ := json.GetOption(opts, jsonv1.ReportErrorsWithLegacySemantics); !legacy {
		if caller == hooks {
			return nil
		}
		if e := lastCallerJoin.Load(); e != nil && e.hooks == hooks && e.joined == caller {
			return nil
		}
	}
	if ok && caller != nil && caller != hooks {
		hooks = joinCallerUnmarshalers(hooks, caller)
	}
	unmarshalers, legacySemantics := hookOptions(hooks)
	return json.JoinOptions(opts, unmarshalers, legacySemantics)
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
// does not accumulate an entry per call for the process lifetime. Its joined
// pointer is also what [decodeOptions] looks for on a nested value's decoder,
// which then adds nothing.
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
// the same WithUnmarshalers value). A nested value's decoder carries the joined
// pointer itself, which [decodeOptions] recognises from the memo without calling
// here, so a cold decode joins once, not once per nesting level; only a nested
// level whose entry another caller displaced meanwhile joins again, taking the
// joined pointer as its caller. Two concurrent misses may both join; the
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
// lexicographic order, and FormatNilSliceAsNull / FormatNilMapAsNull so a
// nil slice or map that is written is spelled "null", as encoding/json v1
// spells it, instead of v2's default "[]" / "{}".
//
// A generated list or map field that is nil is never written: its omit
// option leaves it out, so the attribute is absent from the document rather
// than null. The two FormatNil options decide
// the spelling of a nil slice or map that has no omit option of its own: one
// held in an any field, one behind a non-nil pointer such as an optional
// Hash field's *map[string]V, or one passed straight to canjson.Marshal.
//
// This is the single home for these three options. An entry point with no
// encoder in hand (canjson.Marshal / MarshalIndent) joins the set directly,
// and [MarshalOptions] hands it to every generated MarshalJSONTo, so the
// spelling holds no matter which package (encoding/json v1 or v2) drives the
// encode. The set is built once and every call returns that same value: an
// options value cannot change once built (json/v2 only reads an option passed
// to Join or JoinOptions, and exports no way to modify one), so sharing it is
// safe and saves an allocation per call.
func EncodeOptions() json.Options {
	return encodeOptions
}

// encodeOptions is the one value [EncodeOptions] returns.
var encodeOptions = json.JoinOptions(
	json.Deterministic(true),
	json.FormatNilSliceAsNull(true),
	json.FormatNilMapAsNull(true),
)

// MarshalOptions returns the encode options every generated MarshalJSONTo
// passes to json.MarshalEncode: the [EncodeOptions] value, so the
// deterministic-map and nil-container spellings hold from any entry point.
// MarshalEncode joins the options it is passed on top of the encoder's own for
// that call's duration, which gives the same result as joining the two here,
// without building a new set per value. The result is never nil, whatever the
// encoder carries, so a caller that passes it to an encode on another encoder
// still gets the three options. The encoder is not read, and may be nil.
func MarshalOptions(_ *jsontext.Encoder) json.Options {
	return encodeOptions
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
// syntactic error or a failing reader), not a shape failure of this type.
// Two errors are exceptions to that rule. A duplicate member name is a
// syntactic error that is nonetheless a shape refusal, so it gains
// [ErrInvalidShape] through [ClassifyDuplicate] (without the
// `canjson: <rmType>:` text). A depth refusal ([ErrMaxDepthExceeded])
// from a nested value passes through unwrapped and does not gain the shape
// sentinel.
//
// out is either the receiver viewed through its method-free alias (the
// zero-copy shape) or a flat wire struct the caller copies back (the shape used
// for a type that embeds a marshaler-bearing ancestor, whose alias would
// promote the ancestor's methods). Either way out declares the _type field
// gotType points into, so the guard reads the value this decode populated, and
// a caller who sets json.RejectUnknownMembers is not tripped by the SDK's own
// discriminator reaching out.
//
// A nil dec, a nil out (the interface itself, not a typed nil inside it) or a
// nil gotType is caller misuse (no generated body passes one); each is refused
// before any decode with a [DecodeError] wrapping [ErrNilArgument] that names
// the argument, rather than dereferenced. A typed nil inside out
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
// is the natural default at a concrete-typed slot); an abstract
// interface passes a nil fallback and refuses the missing discriminator.
//
// Dispatch failures (missing / unknown / mismatched _type) return a
// [DecodeError] wrapping the matching sentinel, so errors.Is finds the kind and
// errors.As finds the envelope; the JSON position is supplied by the v2
// decoder's own SemanticError wrapper around this return.
//
// A nil dec or a nil out is caller misuse (no generated hook passes one),
// refused before any read with a [DecodeError] wrapping [ErrNilArgument].
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
	// shallow on its own, from nesting past the cap in total. A value nested d
	// levels deep holds d opening and d closing brackets, so d is at most
	// len(raw)/2; when even that bound stays within the cap the check cannot
	// fail, and the bracket scan is skipped.
	if depth := dec.StackDepth(); depth+len(raw)/2 > maxDecodeDepth {
		if d := depth + jsonNestingDepth(raw); d > maxDecodeDepth {
			return &DecodeError{Inner: fmt.Errorf("typereg.Decode: %w (%d > %d)", ErrMaxDepthExceeded, d, maxDecodeDepth)}
		}
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
	// `canjson: <Interface>:` rather than a bare colon. [leadingType] reads a
	// leading `_type` without the peek's decode when nothing can make the two
	// disagree; any other slot takes the peek.
	//
	// The peek runs on a decoder of its own, so the position in its failure
	// counts from the slot. The [DecodeError] around it marks where the slot
	// begins, as it does for every other failure at a slot, so the enclosing
	// decode fills in the slot's position and an entry point reads the
	// failure's pointer from the document's root (REQ-052). It adds no
	// classification: the shape failure inside keeps [ErrInvalidShape], and
	// no dispatch sentinel joins it.
	opts := dec.Options()
	name, ok := leadingType(raw, opts)
	if !ok {
		if name, err = peekType(raw, opts); err != nil {
			return &DecodeError{Inner: WrapShapeError(reflect.TypeFor[T]().Name(), err)}
		}
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
	// raw decodes on a fresh decoder, which starts with none of dec's options:
	// when decodeOptions has nothing to add, dec's own options carry it all.
	slotOpts := decodeOptions(dec)
	if slotOpts == nil {
		slotOpts = opts
	}
	if err := json.Unmarshal(raw, v, slotOpts); err != nil {
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

// leadingType reads the _type discriminator straight from raw's bytes, without
// the decode [peekType] runs, when raw opens with it: _type is the first
// member, its name is spelled exactly `"_type"` (JSON whitespace allowed around
// the name, the colon and the value), and its value is a JSON string of plain
// ASCII with no backslash escape, so its bytes are the string a decode would
// produce (an escape, or an invalid UTF-8 byte a caller allows, decodes to
// something else). ADR 0021 makes _type first a SHOULD, so this is the shape
// canonical-JSON encoders write and most slots carry.
//
// ok is false for any other shape, and also whenever opts could make the full
// decode read a different member as _type, or run a caller's code on it:
// duplicate names allowed (a later _type wins), case-insensitive name matching
// (a later _TYPE matches too), or a hook set other than the SDK aggregate (a
// caller's string hook runs on _type during the peek). The caller then falls
// back to peekType, so a document that puts _type elsewhere still decodes,
// through the peek.
//
// raw must be a complete JSON value the decoder has already accepted; the scan
// relies on that and checks syntax only as far as the _type value.
func leadingType(raw jsontext.Value, opts json.Options) (name string, ok bool) {
	if dup, _ := json.GetOption(opts, jsontext.AllowDuplicateNames); dup {
		return "", false
	}
	if fold, _ := json.GetOption(opts, json.MatchCaseInsensitiveNames); fold {
		return "", false
	}
	if hooks, _ := json.GetOption(opts, json.WithUnmarshalers); hooks != aggregateUnmarshalers() {
		return "", false
	}
	i := skipWhitespace(raw, 0)
	if i >= len(raw) || raw[i] != '{' {
		return "", false
	}
	const key = `"_type"`
	i = skipWhitespace(raw, i+1)
	if len(raw)-i < len(key) || string(raw[i:i+len(key)]) != key {
		return "", false
	}
	i = skipWhitespace(raw, i+len(key))
	if i >= len(raw) || raw[i] != ':' {
		return "", false
	}
	i = skipWhitespace(raw, i+1)
	if i >= len(raw) || raw[i] != '"' {
		return "", false
	}
	start := i + 1
	for j := start; j < len(raw); j++ {
		switch c := raw[j]; {
		case c == '"':
			return string(raw[start:j]), true
		case c == '\\' || c >= 0x80:
			return "", false
		}
	}
	return "", false
}

// skipWhitespace returns the index of the first byte of b at or after i that is
// not JSON whitespace (space, tab, line feed or carriage return), or len(b).
func skipWhitespace(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return i
}

// Package typereg holds the central type registry that maps the
// openEHR _type discriminator to concrete Go RM types for JSON
// decoding. Every polymorphic decoding site consults the registry.
//
// The registry is populated by the rm package's init() (the generator
// emits openehr/rm/typereg_gen.go which calls [Default.Register] for
// every concrete RM type). External consumers MUST NOT register types
// for the standard RM — the registry is append-only and panics on
// duplicate registration (REQ-040).
package typereg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
)

// maxDecodeDepth bounds the JSON nesting depth Decode accepts. encoding/json
// caps nesting at 10000; this far lower bound reflects real RM data, which
// nests only a few dozen levels (COMPOSITION > SECTION > … > CLUSTER > ELEMENT),
// while still bounding the recursive polymorphic decode path.
const maxDecodeDepth = 512

// jsonNestingDepth returns the maximum bracket/brace nesting depth in
// data, ignoring braces inside strings (with escape handling). It exits
// early once the depth exceeds maxDecodeDepth, so a hostile document
// costs only O(bytes up to the limit) rather than O(whole document).
func jsonNestingDepth(data []byte) int {
	depth, maxDepth := 0, 0
	inStr, esc := false, false
	for _, b := range data {
		if inStr {
			switch {
			case esc:
				esc = false
			case b == '\\':
				esc = true
			case b == '"':
				inStr = false
			}
			continue
		}
		switch b {
		case '"':
			inStr = true
		case '{', '[':
			depth++
			maxDepth = max(maxDepth, depth)
			if depth > maxDecodeDepth {
				return depth // early exit; caller rejects
			}
		case '}', ']':
			depth--
		}
	}
	return maxDepth
}

// Sentinel errors returned by [Registry.Decode] and [DecodeAs]. They
// are unwrap-compatible (errors.Is) so call sites such as the canjson
// codec can wrap them in a richer [DecodeError] without losing the
// classification. PROBE-031 asserts ErrUnknownType.
var (
	// ErrMissingType signals that the input JSON object lacks the
	// `_type` discriminator required at a polymorphic site.
	ErrMissingType = errors.New("typereg: _type discriminator missing")
	// ErrUnknownType signals that `_type` is present but no
	// constructor is registered for the given discriminator.
	ErrUnknownType = errors.New("typereg: _type not in registry")
	// ErrTypeMismatch signals that the decoded concrete value does
	// not satisfy the target interface or type parameter T at a
	// [DecodeAs] call site.
	ErrTypeMismatch = errors.New("typereg: decoded type does not satisfy target")
	// ErrMaxDepthExceeded signals that the JSON nesting depth of a value
	// handed to Decode exceeds maxDecodeDepth — a guard against stack
	// exhaustion and quadratic re-parsing from a crafted deeply-nested
	// polymorphic document (e.g. nested CLUSTER/SECTION trees).
	ErrMaxDepthExceeded = errors.New("typereg: nesting depth exceeds limit")
	// ErrInvalidShape classifies a JSON-level shape failure — valid JSON
	// that is the wrong shape for the target type — or a hand-written
	// primitive codec's refusal of a value it will not accept: rm.Real
	// past its 17-significant-digit budget (a digit count, not a
	// representability test), rm.Character not exactly one character
	// (REQ-052). It is usually attached by [WrapShapeError] from inside a
	// generated UnmarshalJSON method; the hand-written codecs attach it
	// directly through their own message-preserving wrapper. Its message names
	// canjson because canjson is where callers meet it: it is
	// re-exported as canjson.ErrInvalidShape and lives here only so code
	// in openehr/rm can attach it without forming an
	// `openehr/rm → openehr/serialize` import cycle.
	ErrInvalidShape = errors.New("canjson: invalid JSON shape")
)

// DecodeError is the unified envelope returned by the canjson and
// canxml decoders at polymorphic-dispatch sites. It lives here in
// typereg (rather than in a codec-specific package) so the
// generator-emitted UnmarshalJSON methods on the generated RM types
// can construct it without forming an `openehr/rm → serialize/...`
// import cycle.
//
// Path is a JSON-pointer-ish or XPath-ish string describing the
// failed node; Type is the observed discriminator (may be empty when
// the discriminator was missing); Inner unwraps to one of the
// typereg sentinels (or a codec-defined shape error).
//
// The envelope is classification-neutral in both directions: it never
// adds [ErrInvalidShape] to what it wraps, and — because Unwrap exposes
// Inner — it never strips one raised beneath it. So a dispatch failure
// (missing / unknown / mismatched `_type`) stays outside the shape
// sentinel, while a shape failure inside the concrete type selected at
// a slot answers true to both errors.AsType[*DecodeError] and
// errors.Is(_, ErrInvalidShape): the path from this type, the kind from
// the sentinel (REQ-052).
type DecodeError struct {
	Path  string
	Type  string
	Inner error
}

// Error names the failed node and the wrapped cause. A nil receiver
// answers with the zero DecodeError's text rather than dereferencing
// (REQ-025 nil-receiver axis). This type is the documented errors.As /
// errors.AsType out-parameter for both codecs — it is re-exported as
// canjson.DecodeError and canxml.DecodeError — so a failed match leaves
// a typed nil in consumer hands on the most-travelled decode route in
// the SDK.
func (e *DecodeError) Error() string {
	if e == nil {
		return (&DecodeError{}).Error()
	}
	// Every producer sets Inner; only a caller-built zero value leaves it
	// nil. Inner is any error a codec or caller can supply — including a
	// boxed typed-nil pointer whose own Error() dereferences unguarded, the
	// shape a failed errors.As / errors.AsType leaves behind — so a direct
	// err.Error() call is not safe here the way it is for this package's own
	// guarded types. %v is: fmt recovers a panicking Error() method itself
	// and renders a nil-pointer receiver as "<nil>" (REQ-025 nil-receiver
	// axis).
	cause := "unspecified error"
	if e.Inner != nil {
		cause = fmt.Sprintf("%v", e.Inner)
	}
	switch {
	case e.Path != "" && e.Type != "":
		return fmt.Sprintf("decode %s (_type=%q): %s", e.Path, e.Type, cause)
	case e.Path != "":
		return fmt.Sprintf("decode %s: %s", e.Path, cause)
	case e.Type != "":
		return fmt.Sprintf("decode _type=%q: %s", e.Type, cause)
	default:
		return "decode: " + cause
	}
}

// Unwrap returns the wrapped error so errors.Is / errors.As reach the
// underlying sentinel. A nil receiver unwraps to nil (REQ-025
// nil-receiver axis).
func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}

// WrapShapeError builds the error a generated UnmarshalJSON method
// returns when the whole value fails to decode into its wire struct.
// rmType is the openEHR class name (e.g. "DV_QUANTITY"); err is the
// encoding/json failure underneath.
//
// The message is exactly `canjson: <rmType>: <err>`, and err stays
// reachable through errors.As, so the classification costs no
// diagnostic. What it adds is [ErrInvalidShape] under errors.Is —
// except when err already carries a [DecodeError]: this arm does not
// add the sentinel to it, and whatever classification that DecodeError
// already carries — its dispatch sentinel, or a shape classification
// raised beneath it — is kept as is (REQ-052).
//
// The bypass is one-directional: it withholds a classification the
// enclosing funnel would otherwise add, never removes one raised
// further down. So a shape failure inside the concrete type selected
// at a polymorphic slot keeps ErrInvalidShape while the DecodeError
// keeps the path — both true of the same error — whereas a dispatch
// failure (missing, unknown or mismatched `_type`) stays outside the
// sentinel, because ErrInvalidShape means "JSON-level shape", not "any
// decode failure".
//
// A nil err returns nil, so a caller cannot manufacture an error out
// of a success.
func WrapShapeError(rmType string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*DecodeError](err); ok {
		return fmt.Errorf("canjson: %s: %w", rmType, err)
	}
	return &shapeError{rmType: rmType, cause: err}
}

// shapeError is the [ErrInvalidShape]-carrying error [WrapShapeError]
// returns. It is unexported: consumers classify with errors.Is against
// the sentinel and reach the cause with errors.As, so the concrete type
// is not part of the public surface.
type shapeError struct {
	rmType string
	cause  error
}

// Error reproduces the `canjson: <RM_TYPE>: <cause>` text the generated
// methods have always returned.
func (e *shapeError) Error() string {
	return "canjson: " + e.rmType + ": " + e.cause.Error()
}

// Unwrap returns the cause alone, so a single errors.Unwrap step lands
// on the encoding/json error the generated method actually failed with,
// and errors.As reaches it as before. The [ErrInvalidShape]
// classification rides on Is below rather than on a second unwrap
// branch: a multi-error Unwrap would make errors.Unwrap answer nil for
// every generated decode failure, which is not what the "the cause
// stays reachable through unwrapping" clause promises (REQ-052).
func (e *shapeError) Unwrap() error {
	return e.cause
}

// Is reports the sentinel this type carries. It matches [ErrInvalidShape]
// only — the classification [WrapShapeError] attaches — and leaves every
// other target to the ordinary unwrap walk down e.cause.
func (e *shapeError) Is(target error) bool {
	return target == ErrInvalidShape
}

// Registry maps each openEHR _type discriminator string (e.g.
// "DV_QUANTITY") to a constructor returning a fresh zero-value
// instance of the corresponding concrete Go type. Per REQ-040 the
// registry never uses reflection to instantiate types; the
// constructor closure is the only sanctioned mechanism.
//
// Registry is safe for concurrent reads. Writes (Register) are
// serialised under a sync.RWMutex; they are expected to happen once,
// during package init.
type Registry struct {
	mu    sync.RWMutex
	ctors map[string]func() any
}

// Default is the process-wide registry. The rm package's init()
// populates it.
var Default = NewRegistry()

// NewRegistry returns an empty registry. Useful for tests that want
// an isolated registry — production code uses [Default].
func NewRegistry() *Registry {
	return &Registry{ctors: make(map[string]func() any)}
}

// Register associates an openEHR _type string with a constructor.
// Panics on duplicate registration: a name collision is a programmer
// error (REQ-040), not a recoverable condition.
func (r *Registry) Register(typeName string, ctor func() any) {
	if typeName == "" {
		panic("typereg.Register: typeName is empty")
	}
	if ctor == nil {
		panic("typereg.Register: ctor is nil for " + typeName)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.ctors[typeName]; exists {
		panic(fmt.Sprintf("typereg.Register: duplicate registration for %q", typeName))
	}
	r.ctors[typeName] = ctor
}

// Lookup returns the constructor for typeName and a boolean
// indicating whether one was registered.
func (r *Registry) Lookup(typeName string) (func() any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.ctors[typeName]
	return c, ok
}

// Names returns a sorted snapshot of every registered type name — the
// supported-type inventory. Enumeration exists so reversibility parity
// (REQ-040: registration name ↔ [rm.RMTypeName]) can be verified over
// the whole registry rather than a hand-picked sample.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Sorted(maps.Keys(r.ctors))
}

// Decode peeks the JSON object's "_type" discriminator, looks up the
// matching constructor, and decodes data into a fresh instance of the
// concrete type. The returned value is a non-nil pointer typed as any.
//
// Returns an error if:
//
//   - data is not a JSON object,
//   - the "_type" field is missing or not a string,
//   - no constructor is registered for the discriminator,
//   - the body fails to decode into the concrete type.
func (r *Registry) Decode(data []byte) (any, error) {
	if d := jsonNestingDepth(data); d > maxDecodeDepth {
		return nil, fmt.Errorf("typereg.Decode: %w (%d > %d)", ErrMaxDepthExceeded, d, maxDecodeDepth)
	}
	var head struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("typereg.Decode: read _type: %w", err)
	}
	if head.Type == "" {
		return nil, fmt.Errorf("typereg.Decode: %w", ErrMissingType)
	}
	ctor, ok := r.Lookup(head.Type)
	if !ok {
		return nil, fmt.Errorf("typereg.Decode %q: %w", head.Type, ErrUnknownType)
	}
	v := ctor()
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(v); err != nil {
		return nil, fmt.Errorf("typereg.Decode %q: %w", head.Type, err)
	}
	return v, nil
}

// DecodeAs decodes data on the [Default] registry and returns the
// decoded value typed as T. See [Registry.DecodeAs] for the
// type-parameter rules it defers to.
func DecodeAs[T any](data []byte) (T, error) {
	return Default.DecodeAs[T](data)
}

// DecodeAs is a typed wrapper over [Registry.Decode]. It returns the
// decoded value type-asserted to T. The zero value of T is returned
// together with the error on any failure.
//
// Useful at codec call sites: typereg.DecodeAs[*rm.DVQuantity](data)
// or r.DecodeAs[T](data) on an isolated registry.
//
// Registry constructors return pointers (`&Concrete{}`) so the JSON
// decoder can populate them. Callers may parameterise T with either
// the pointer shape (`*Concrete`), an interface satisfied by the
// pointer (e.g. abstract `DVOrdered`), OR the value shape
// (`Concrete`) — the last case arises when a generic codec method is
// instantiated with a concrete value type (e.g.
// `DVInterval[DVQuantity].Lower` dispatched via `DecodeAs[DVQuantity]`).
// The method first asserts to T directly (matches the pointer /
// interface shapes), then to `*T` and dereferences if successful —
// closing the value-T gap without reflection.
func (r *Registry) DecodeAs[T any](data []byte) (T, error) {
	var zero T
	v, err := r.Decode(data)
	if err != nil {
		return zero, err
	}
	if t, ok := v.(T); ok {
		return t, nil
	}
	if pt, ok := v.(*T); ok && pt != nil {
		return *pt, nil
	}
	return zero, fmt.Errorf("typereg.DecodeAs: decoded %T: %w", v, ErrTypeMismatch)
}

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
			if depth > maxDepth {
				maxDepth = depth
			}
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
type DecodeError struct {
	Path  string
	Type  string
	Inner error
}

func (e *DecodeError) Error() string {
	switch {
	case e.Path != "" && e.Type != "":
		return fmt.Sprintf("decode %s (_type=%q): %v", e.Path, e.Type, e.Inner)
	case e.Path != "":
		return fmt.Sprintf("decode %s: %v", e.Path, e.Inner)
	case e.Type != "":
		return fmt.Sprintf("decode _type=%q: %v", e.Type, e.Inner)
	default:
		return fmt.Sprintf("decode: %v", e.Inner)
	}
}

// Unwrap returns the wrapped error so errors.Is / errors.As reach the
// underlying sentinel.
func (e *DecodeError) Unwrap() error { return e.Inner }

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

// DecodeAs is a typed wrapper over [Registry.Decode] on the [Default]
// registry. It returns the decoded value type-asserted to T. The zero
// value of T is returned together with the error on any failure.
//
// Useful at codec call sites: typereg.DecodeAs[*rm.DVQuantity](data).
//
// Registry constructors return pointers (`&Concrete{}`) so the JSON
// decoder can populate them. Callers may parameterise T with either
// the pointer shape (`*Concrete`), an interface satisfied by the
// pointer (e.g. abstract `DVOrdered`), OR the value shape
// (`Concrete`) — the last case arises when a generic codec method is
// instantiated with a concrete value type (e.g.
// `DVInterval[DVQuantity].Lower` dispatched via `DecodeAs[DVQuantity]`).
// The function first asserts to T directly (matches the pointer /
// interface shapes), then to `*T` and dereferences if successful —
// closing the value-T gap without reflection.
func DecodeAs[T any](data []byte) (T, error) {
	var zero T
	v, err := Default.Decode(data)
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

package query_test

// nilaqlerror_test.go — REQ-025 § No panics, on the nil-receiver axis of this
// package's exported pointer types. *AQLError is the only one: ExecuteOption is
// a func type with no methods (and a nil option is already skipped at every
// call site), Repository is an interface, and the *repository behind it is
// unexported and never handed out as a typed nil by NewRepository.

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
)

// TestNilAQLErrorAnswersInsteadOfPanicking pins the three answers a nil
// *AQLError gives (REQ-025 § No panics): no sentinel match, a stable non-empty
// Error string, and nothing to unwrap. *AQLError is an exported type the
// consumer holds and passes around, so a nil receiver is caller input the SDK
// must answer rather than dereference.
func TestNilAQLErrorAnswersInsteadOfPanicking(t *testing.T) {
	var e *query.AQLError

	t.Run("Is", func(t *testing.T) {
		for _, target := range []error{aql.ErrPathResolution, aql.ErrEngineCapability, query.ErrInvalidConfig, nil} {
			if got := e.Is(target); got {
				t.Errorf("nil.Is(%v) = true, want false — a nil AQLError classifies as nothing", target)
			}
		}
	})

	t.Run("Error", func(t *testing.T) {
		got := e.Error()
		if got == "" {
			t.Fatal("nil.Error() = \"\", want a non-empty string — fmt would print nothing for this error")
		}
		// The zero AQLError's own text, so the nil and zero receivers agree
		// and the string stays stable for anything matching on it.
		if want := (&query.AQLError{}).Error(); got != want {
			t.Errorf("nil.Error() = %q, want %q (what the zero AQLError says)", got, want)
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		if got := e.Unwrap(); got != nil {
			t.Errorf("nil.Unwrap() = %v, want nil — a nil AQLError wraps nothing", got)
		}
	})
}

// TestFailedErrorsAsLeavesAnAQLErrorThatStillAnswers walks the exact route the
// defect was reachable by, so the guard is pinned against the documented API
// rather than against a hand-built nil. In the errors.As out-parameter form
//
//	var e *query.AQLError
//	if errors.As(err, &e) { … }
//
// a FAILED match leaves that variable nil. A caller that passes it onward boxes
// a typed nil in a non-nil error interface, so errors.Is finds a non-nil error,
// sees the Is method, and calls it on the nil receiver.
//
// This test MUST keep errors.As. The hazard is the out-parameter a failed match
// leaves behind, which errors.AsType cannot reproduce — it returns the zero
// value beside an ok, so nothing persists in the caller's scope. Converting
// this call would delete the guard and leave the test green.
func TestFailedErrorsAsLeavesAnAQLErrorThatStillAnswers(t *testing.T) {
	notAnAQLError := errors.New("query: some other failure")

	var e *query.AQLError
	if errors.As(notAnAQLError, &e) {
		t.Fatalf("premise gone: errors.As matched %T as *query.AQLError", notAnAQLError)
	}
	if e != nil {
		t.Fatalf("premise gone: a failed errors.As left e = %v, want it still nil", e)
	}

	// Boxing that nil in an error interface is what a caller does the moment it
	// passes the variable onward. The interface value is non-nil — it carries
	// the *AQLError type with a nil value — so errors.Is finds a non-nil error,
	// finds the Is method, and calls it on the nil receiver. There is no
	// runtime assertion for that here because it holds by construction:
	// staticcheck proves `boxed == nil` is never true (SA4023) and rejects the
	// check as dead code.
	boxed := error(e)

	// Each of these dispatches into a method on the nil receiver.
	if errors.Is(boxed, aql.ErrPathResolution) {
		t.Error("errors.Is(nil AQLError, aql.ErrPathResolution) = true, want false")
	}
	if errors.Is(boxed, aql.ErrEngineCapability) {
		t.Error("errors.Is(nil AQLError, aql.ErrEngineCapability) = true, want false")
	}
	if got := errors.Unwrap(boxed); got != nil {
		t.Errorf("errors.Unwrap(nil AQLError) = %v, want nil", got)
	}
	if got := boxed.Error(); got == "" {
		t.Error("Error() on a boxed nil AQLError = \"\", want a non-empty string")
	}
}

// TestEveryExportedMethodToleratesANilReceiver is the tripwire for the axis
// rather than for the three methods that exist today: it reflects over the
// whole exported method set of *AQLError and calls each on a nil receiver. A
// method added later that dereferences the receiver without going through
// orZero fails here on the day it lands, which is the only way a
// REQ-025 § No panics guarantee stays true as the surface grows.
func TestEveryExportedMethodToleratesANilReceiver(t *testing.T) {
	var nilErr *query.AQLError
	rt := reflect.TypeOf(nilErr)

	if n := rt.NumMethod(); n < 3 {
		t.Fatalf("reflected %d exported methods on *AQLError; expected at least the 3 known ones (Error, Is, Unwrap) — the sweep is not looking at what it thinks it is", n)
	}

	for m := range rt.Methods() {
		t.Run(m.Name, func(t *testing.T) {
			for i, args := range nilReceiverCalls(m, reflect.ValueOf(nilErr)) {
				out := m.Func.Call(args) // panics here = the finding

				// Forward-looking, and free: no method hands back an *AQLError
				// today, but one that did must hand back a usable one rather
				// than re-seeding the same nil into a caller's hands.
				for _, o := range out {
					if o.Type() == rt && o.IsNil() {
						t.Errorf("%s (argument vector %d) returned a nil *AQLError", m.Name, i)
					}
				}
			}
		})
	}
}

// errorArgCandidates are the values the tripwire feeds to an error-typed
// parameter. Zero values alone are NOT enough, and Is is the live proof: it
// switches on target, so a nil target matches no case and the method returns
// before ever reading a field of the receiver. A tripwire calling Is(nil) would
// therefore pass with the guard deleted. The classification sentinels are the
// targets that actually drive Is into the receiver's fields, so they have to be
// in the mix — the same lesson as the variadic trap in nilReceiverCalls, where
// an early return skips the receiver entirely.
var errorArgCandidates = []error{
	aql.ErrPathResolution,
	aql.ErrEngineCapability,
	query.ErrInvalidConfig,
	errors.New("query: an unrelated error"),
	nil,
}

// nilReceiverCalls builds the argument vectors each method is attempted with on
// a nil receiver: one all-zero vector, plus one per errorArgCandidates entry for
// every error-typed parameter. reflect's m.Func takes the receiver as argument
// 0, so recv leads every vector.
func nilReceiverCalls(m reflect.Method, recv reflect.Value) [][]reflect.Value {
	n := m.Type.NumIn()
	base := make([]reflect.Value, 0, n)
	base = append(base, recv)
	for j := 1; j < n; j++ {
		at := m.Type.In(j)
		// For a variadic method pass ONE zero element rather than an empty
		// list: an empty list is the case an early length check can answer
		// without ever touching the receiver, so it would not exercise the
		// guard.
		if m.Type.IsVariadic() && j == n-1 {
			at = at.Elem()
		}
		base = append(base, reflect.Zero(at))
	}

	vectors := [][]reflect.Value{base}
	errType := reflect.TypeFor[error]()
	for j := 1; j < len(base); j++ {
		if base[j].Type() != errType {
			continue
		}
		for _, cand := range errorArgCandidates {
			v := slices.Clone(base)
			// Via a pointer's Elem so the Value keeps the static type error
			// even when cand is nil (reflect.ValueOf(nil) is not usable).
			v[j] = reflect.ValueOf(&cand).Elem()
			vectors = append(vectors, v)
		}
	}
	return vectors
}

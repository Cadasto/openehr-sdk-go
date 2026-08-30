package transport_test

// nilwireerror_test.go — REQ-025 § No panics, on the nil-receiver axis of this
// package's exported pointer types. Three carry nil receivers: *WireError, the
// typed error a failed errors.As / errors.AsType leaves behind in a consumer's
// variable, and *Client, which a consumer holds and passes into every leaf
// call, are pinned here; *DecodeError (REQ-151) is the third, pinned in its own
// decode_error_test.go and not repeated here. The remaining exported
// identifiers are off-axis: Option is a func type and
// Observer an interface (neither has a nil receiver of its own); Request,
// Response, Metadata, Observation, OpenEHRErrorDetail, CodedTextItem and
// RetryPolicy are plain structs with no exported methods; CallerAttribution
// and Prefer carry only value-receiver methods, which cannot be reached
// through a nil pointer.

import (
	"errors"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestNilWireErrorAnswersInsteadOfPanicking pins REQ-025 § No panics on
// *WireError: a typed nil (the value a failed errors.As / errors.AsType
// leaves behind) answers Error and Unwrap instead of dereferencing.
// Removing either nil-receiver guard MUST fail this test.
func TestNilWireErrorAnswersInsteadOfPanicking(t *testing.T) {
	var e *transport.WireError

	t.Run("Error", func(t *testing.T) {
		got := e.Error()
		if got == "" {
			t.Fatal(`nil.Error() = "", want a non-empty string`)
		}
		if want := (&transport.WireError{}).Error(); got != want {
			t.Errorf("nil.Error() = %q, want %q (what the zero WireError says)", got, want)
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		if got := e.Unwrap(); got != nil {
			t.Errorf("nil.Unwrap() = %v, want nil", got)
		}
	})
}

// TestFailedErrorsAsLeavesAWireErrorThatStillAnswers walks the
// errors.As out-parameter shape: a failed match leaves the variable
// nil, boxing it as error still answers rather than panicking.
func TestFailedErrorsAsLeavesAWireErrorThatStillAnswers(t *testing.T) {
	notAWireError := errors.New("transport: some other failure")

	var e *transport.WireError
	if errors.As(notAWireError, &e) {
		t.Fatalf("premise gone: errors.As matched %T as *transport.WireError", notAWireError)
	}
	if e != nil {
		t.Fatalf("premise gone: a failed errors.As left e = %v, want it still nil", e)
	}

	// Boxing that nil in an error interface is what a caller does the moment
	// it passes the variable onward: the interface value is non-nil (it
	// carries the *WireError type with a nil value), so a %v or an
	// errors.Is/As walk dispatches into a method on the nil receiver. There
	// is no runtime assertion for that here because it holds by
	// construction: staticcheck proves `boxed == nil` is never true (SA4023)
	// and rejects the check as dead code.
	boxed := error(e)

	got := boxed.Error()
	if got == "" {
		t.Fatal(`boxed typed-nil Error() = "", want a non-empty string`)
	}
	if unwrapped := errors.Unwrap(boxed); unwrapped != nil {
		t.Errorf("boxed typed-nil Unwrap() = %v, want nil", unwrapped)
	}
	// The classification entry the WireError godoc documents (errors.Is
	// against a package sentinel) walks Unwrap on the nil receiver.
	if errors.Is(boxed, transport.ErrNotFound) {
		t.Error("errors.Is(boxed typed-nil WireError, transport.ErrNotFound) = true, want false — a nil WireError classifies as nothing")
	}
}

// TestNilClientAnswersInsteadOfPanicking pins the accessors on the other
// exported pointer type of this package. Do already rules a nil *Client
// caller-constructible and fails closed with a typed error; Catalog and
// HTTPClient are reached by the same consumer holding the same value, so they
// answer nil rather than dereferencing (REQ-025 § No panics). Removing either
// guard MUST fail this test.
func TestNilClientAnswersInsteadOfPanicking(t *testing.T) {
	var c *transport.Client

	t.Run("Catalog", func(t *testing.T) {
		if got := c.Catalog(); got != nil {
			t.Errorf("nil.Catalog() = %v, want nil", got)
		}
	})

	t.Run("HTTPClient", func(t *testing.T) {
		if got := c.HTTPClient(); got != nil {
			t.Errorf("nil.HTTPClient() = %v, want nil", got)
		}
	})

	t.Run("Do", func(t *testing.T) {
		resp, err := c.Do(t.Context(), &transport.Request{Method: http.MethodGet, Path: "/"})
		if resp != nil {
			t.Errorf("nil.Do() returned a Response %v, want nil", resp)
		}
		if !errors.Is(err, transport.ErrInvalidConfig) {
			t.Errorf("nil.Do() error = %v, want one matching transport.ErrInvalidConfig", err)
		}
	})
}

// TestEveryExportedMethodToleratesANilReceiver is the tripwire for the axis
// rather than for the two methods that exist today: it reflects over the whole
// exported method set of *WireError and calls each on a nil receiver. A method
// added later that dereferences the receiver without a guard fails here on the
// day it lands, which is the only way a REQ-025 § No panics guarantee stays
// true as the surface grows.
func TestEveryExportedMethodToleratesANilReceiver(t *testing.T) {
	var nilErr *transport.WireError
	rt := reflect.TypeOf(nilErr)

	if n := rt.NumMethod(); n < 2 {
		t.Fatalf("reflected %d exported methods on *WireError; expected at least the 2 known ones (Error, Unwrap) — the sweep is not looking at what it thinks it is", n)
	}

	for m := range rt.Methods() {
		t.Run(m.Name, func(t *testing.T) {
			for i, args := range nilReceiverCalls(m, reflect.ValueOf(nilErr)) {
				out := m.Func.Call(args) // panics here = the finding

				// Forward-looking, and free: no method hands back a *WireError
				// today, but one that did must hand back a usable one rather
				// than re-seeding the same nil into a caller's hands.
				for _, o := range out {
					if o.Type() == rt && o.IsNil() {
						t.Errorf("%s (argument vector %d) returned a nil *WireError", m.Name, i)
					}
				}
			}
		})
	}
}

// TestEveryExportedClientMethodToleratesANilReceiver is the same tripwire for
// the package's other on-axis pointer type. *Client grows accessors as leaves
// need to project more of the construction inputs (Catalog and HTTPClient are
// both such additions), so the sweep — not the two hand-written subtests above
// — is what keeps the guarantee true for the next one.
func TestEveryExportedClientMethodToleratesANilReceiver(t *testing.T) {
	var nilClient *transport.Client
	rt := reflect.TypeOf(nilClient)

	if n := rt.NumMethod(); n < 3 {
		t.Fatalf("reflected %d exported methods on *Client; expected at least the 3 known ones (Catalog, Do, HTTPClient) — the sweep is not looking at what it thinks it is", n)
	}

	for m := range rt.Methods() {
		t.Run(m.Name, func(t *testing.T) {
			for _, args := range nilReceiverCalls(m, reflect.ValueOf(nilClient)) {
				m.Func.Call(args) // panics here = the finding
			}
		})
	}
}

// errorArgCandidates are the values the tripwire feeds to an error-typed
// parameter. Zero values alone are NOT enough: a method that switches on its
// error argument can return before ever reading a field of the receiver, so a
// tripwire calling it with nil alone would pass with the guard deleted. The
// package's own classification sentinels are the targets that actually drive
// such a method into the receiver's fields, so they have to be in the mix.
var errorArgCandidates = []error{
	transport.ErrNotFound,
	transport.ErrUnauthorized,
	transport.ErrServerError,
	transport.ErrInvalidShape,
	errors.New("transport: an unrelated error"),
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

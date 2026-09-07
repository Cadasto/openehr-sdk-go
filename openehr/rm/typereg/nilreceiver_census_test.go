package typereg_test

// nilreceiver_census_test.go — REQ-025 § No panics, the nil-receiver axis on
// the decode side. A nil *T is caller-constructible input (a failed
// errors.AsType leaves one behind; a zero-value struct holds one in a field),
// so every UnmarshalJSON MUST refuse it with an error rather than dereference
// it. The census runs over the whole type registry — RM and AOM 1.4 both
// register into typereg.Default — plus the three hand-written primitive codecs
// the registry does not hold, so a generated type that loses its guard, or a
// new primitive codec written without one, fails here by name.
//
// It lives in typereg's external test package rather than in openehr/rm so
// that importing openehr/aom/aom14 (to register the AOM types) does not widen
// the registry that rm's own REQ-040 name-parity test enumerates.
//
// reflect is used to manufacture a typed nil of a type known only through its
// registry constructor. REQ-024 binds library code, not tests
// (canjson/field_order_test.go is the precedent).

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	_ "github.com/cadasto/openehr-sdk-go/openehr/aom/aom14" // registers the AOM 1.4 types
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

func TestNilReceiverUnmarshalJSONCensus(t *testing.T) { // REQ-025
	names := typereg.Default.Names()
	if len(names) < 100 {
		t.Fatalf("registry holds %d types; the census expects the full RM + AOM 1.4 inventory", len(names))
	}
	// The census is an exact inventory over the LIBRARY's own registered types,
	// not a sample: every one MUST carry a generated UnmarshalJSON, so a type
	// without the method is a failure rather than a silent `continue` (which
	// would let generation drop guards and still pass).
	//
	// It is scoped by package path because typereg.Default is process-global and
	// registry_test.go (package typereg, an in-package test) registers fixtures
	// into it — FAKE_BOX_VALUE_T and FAKE_BOX_ASGUARD, which carry no guard by
	// design and appear or not depending on which tests ran. Asserting over
	// every registered name would therefore make this test order-dependent.
	// Production typereg registers nothing into Default (the generated
	// openehr/rm/typereg_gen.go does), so anything registered FROM this package
	// is a test fixture by construction and is the one thing excluded.
	const (
		libraryPrefix = "github.com/cadasto/openehr-sdk-go/openehr/"
		fixturePkg    = "github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	)
	var withoutMethod, foreign []string
	libraryTypes := 0
	for _, name := range names {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok {
			t.Fatalf("Lookup(%q) = false for a name Names() returned", name)
		}
		rt := reflect.TypeOf(ctor())
		for rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		if !strings.HasPrefix(rt.PkgPath(), libraryPrefix) || rt.PkgPath() == fixturePkg {
			foreign = append(foreign, name+" ("+rt.PkgPath()+")")
			continue
		}
		libraryTypes++
		typedNil := reflect.Zero(reflect.TypeOf(ctor())).Interface()
		u, ok := typedNil.(json.Unmarshaler)
		if !ok {
			withoutMethod = append(withoutMethod, name+" ("+rt.PkgPath()+")")
			continue
		}
		t.Run(name, func(t *testing.T) {
			err := callWithoutPanicking(t, func() error { return u.UnmarshalJSON([]byte(`{}`)) })
			if !errors.Is(err, typereg.ErrNilReceiver) {
				t.Fatalf("%s: (nil).UnmarshalJSON({}) = %v, want errors.Is(err, typereg.ErrNilReceiver)", name, err)
			}
		})
	}
	if len(withoutMethod) > 0 {
		t.Errorf("%d of %d library types carry no UnmarshalJSON, so the census could not guard them: %v\n"+
			"The generator emits one for every registered type; a missing method means a dropped guard.",
			len(withoutMethod), libraryTypes, withoutMethod)
	}
	if libraryTypes < 100 {
		t.Errorf("census covered only %d library types (of %d registered); expected the full RM + AOM 1.4 inventory. Skipped as non-library: %v",
			libraryTypes, len(names), foreign)
	}

	primitives := map[string]json.Unmarshaler{
		"rm.Real":      (*rm.Real)(nil),
		"rm.Integer":   (*rm.Integer)(nil),
		"rm.Character": (*rm.Character)(nil),
	}
	for name, u := range primitives {
		err := callWithoutPanicking(t, func() error { return u.UnmarshalJSON([]byte(`1`)) })
		if !errors.Is(err, typereg.ErrNilReceiver) {
			t.Errorf("%s: (nil).UnmarshalJSON(1) = %v, want errors.Is(err, typereg.ErrNilReceiver)", name, err)
		}
	}
	err := callWithoutPanicking(t, func() error { return (*rm.Character)(nil).UnmarshalText([]byte("x")) })
	if !errors.Is(err, typereg.ErrNilReceiver) {
		t.Errorf("rm.Character: (nil).UnmarshalText(x) = %v, want errors.Is(err, typereg.ErrNilReceiver)", err)
	}
}

// callWithoutPanicking runs f and converts a panic into a test failure that
// names it, so a dropped guard reads as "panicked: runtime error: invalid
// memory address" rather than as a crashed test binary.
func callWithoutPanicking(t *testing.T, f func() error) (err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	return f()
}

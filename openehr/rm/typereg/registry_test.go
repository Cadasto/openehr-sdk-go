package typereg

import (
	"errors"
	"strings"
	"testing"
)

type fakeBox struct {
	Type    string `json:"_type"`
	Payload string `json:"payload"`
}

func TestRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	r.Register("FAKE_BOX", func() any { return &fakeBox{} })
	ctor, ok := r.Lookup("FAKE_BOX")
	if !ok {
		t.Fatal("Lookup miss after Register")
	}
	v := ctor()
	if _, ok := v.(*fakeBox); !ok {
		t.Errorf("ctor returned %T, want *fakeBox", v)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	r := NewRegistry()
	r.Register("X", func() any { return &fakeBox{} })
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic on duplicate Register")
		}
	}()
	r.Register("X", func() any { return &fakeBox{} })
}

func TestDecode(t *testing.T) {
	r := NewRegistry()
	r.Register("FAKE_BOX", func() any { return &fakeBox{} })
	v, err := r.Decode([]byte(`{"_type":"FAKE_BOX","payload":"hi"}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	box, ok := v.(*fakeBox)
	if !ok {
		t.Fatalf("Decode returned %T", v)
	}
	if box.Payload != "hi" {
		t.Errorf("payload = %q, want %q", box.Payload, "hi")
	}
}

func TestDecodeUnknownType(t *testing.T) {
	r := NewRegistry()
	_, err := r.Decode([]byte(`{"_type":"UNKNOWN"}`))
	if err == nil {
		t.Fatal("expected error for unknown _type")
	}
	if !strings.Contains(err.Error(), "UNKNOWN") {
		t.Errorf("error %q should mention UNKNOWN", err)
	}
}

func TestDecodeMissingType(t *testing.T) {
	r := NewRegistry()
	_, err := r.Decode([]byte(`{}`))
	if err == nil {
		t.Fatal("expected error for missing _type")
	}
}

// Sentinels: callers MUST be able to distinguish missing _type from
// unknown _type via errors.Is so that the canjson codec can wrap them
// in a DecodeError without dropping the kind. PROBE-031 asserts
// ErrUnknownType specifically.
func TestDecodeMissingTypeIsErrMissingType(t *testing.T) {
	r := NewRegistry()
	_, err := r.Decode([]byte(`{"foo":"bar"}`))
	if err == nil {
		t.Fatal("expected error for missing _type")
	}
	if !errors.Is(err, ErrMissingType) {
		t.Errorf("err = %v; want errors.Is(_, ErrMissingType)", err)
	}
}

func TestDecodeUnknownTypeIsErrUnknownType(t *testing.T) {
	r := NewRegistry()
	_, err := r.Decode([]byte(`{"_type":"UNKNOWN"}`))
	if err == nil {
		t.Fatal("expected error for unknown _type")
	}
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("err = %v; want errors.Is(_, ErrUnknownType)", err)
	}
}

func TestDecodeAsConcreteValueTypeParameter(t *testing.T) {
	const typeName = "FAKE_BOX_VALUE_T"
	if _, ok := Default.Lookup(typeName); !ok {
		Default.Register(typeName, func() any { return &fakeBox{} })
	}
	got, err := DecodeAs[fakeBox]([]byte(`{"_type":"FAKE_BOX_VALUE_T","payload":"ok"}`))
	if err != nil {
		t.Fatalf("DecodeAs[fakeBox]: %v", err)
	}
	if got.Payload != "ok" {
		t.Errorf("Payload = %q, want %q", got.Payload, "ok")
	}
}

func TestDecodeAsTypeMismatchIsErrTypeMismatch(t *testing.T) {
	r := NewRegistry()
	r.Register("FAKE_BOX", func() any { return &fakeBox{} })
	// DecodeAs operates on Default — register there too for this test.
	if _, ok := Default.Lookup("FAKE_BOX_ASGUARD"); !ok {
		Default.Register("FAKE_BOX_ASGUARD", func() any { return &fakeBox{} })
	}
	type unrelated interface{ unrelated() }
	_, err := DecodeAs[unrelated]([]byte(`{"_type":"FAKE_BOX_ASGUARD"}`))
	if err == nil {
		t.Fatal("expected type-mismatch error")
	}
	if !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("err = %v; want errors.Is(_, ErrTypeMismatch)", err)
	}
}

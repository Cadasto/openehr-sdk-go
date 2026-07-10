package jsonpoly_test

import (
	"encoding/json"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/jsonpoly"
)

// leaf mimics an RM data type: `_type` is emitted only by a
// pointer-receiver MarshalJSON, so a value-in-interface would drop it
// under plain encoding/json — the exact REQ-052 sub-gap A mechanism.
type leaf struct {
	Value string `json:"value"`
}

func (l *leaf) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Class string `json:"_type"`
		Value string `json:"value"`
	}{Class: "LEAF", Value: l.Value})
}

// iface is the polymorphic slot type.
type iface interface{ isLeaf() }

// Value receiver so both leaf and *leaf satisfy iface.
func (l leaf) isLeaf() {}

func TestMarshal_valueInInterfaceEmitsType(t *testing.T) {
	// Baseline: plain encoding/json drops _type for a value in an interface.
	var slot iface = leaf{Value: "x"}
	plain, err := json.Marshal(slot)
	if err != nil {
		t.Fatalf("plain marshal: %v", err)
	}
	if got := string(plain); got != `{"value":"x"}` {
		t.Fatalf("baseline changed, want no _type: %s", got)
	}

	// Helper boxes the value so the pointer-receiver MarshalJSON runs.
	raw, err := jsonpoly.Marshal(slot)
	if err != nil {
		t.Fatalf("jsonpoly.Marshal: %v", err)
	}
	if got := string(raw); got != `{"_type":"LEAF","value":"x"}` {
		t.Fatalf("value form: want _type present, got %s", got)
	}

	// Pointer form is unchanged (already correct).
	var ptr iface = &leaf{Value: "y"}
	raw, err = jsonpoly.Marshal(ptr)
	if err != nil {
		t.Fatalf("jsonpoly.Marshal ptr: %v", err)
	}
	if got := string(raw); got != `{"_type":"LEAF","value":"y"}` {
		t.Fatalf("pointer form: got %s", got)
	}
}

func TestMarshal_nilInterface(t *testing.T) {
	raw, err := jsonpoly.Marshal(nil)
	if err != nil {
		t.Fatalf("nil: %v", err)
	}
	if raw != nil {
		t.Fatalf("nil interface should yield nil RawMessage, got %q", raw)
	}
}

func TestMarshalSlice(t *testing.T) {
	// Mixed value/pointer elements both gain _type; nil/empty omit.
	got, err := jsonpoly.MarshalSlice([]iface{leaf{Value: "a"}, &leaf{Value: "b"}})
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	want := `[{"_type":"LEAF","value":"a"},{"_type":"LEAF","value":"b"}]`
	if string(got) != want {
		t.Fatalf("slice: got %s want %s", got, want)
	}

	if raw, _ := jsonpoly.MarshalSlice([]iface(nil)); raw != nil {
		t.Fatalf("nil slice should yield nil RawMessage, got %q", raw)
	}
	if raw, _ := jsonpoly.MarshalSlice([]iface{}); raw != nil {
		t.Fatalf("empty slice should yield nil RawMessage, got %q", raw)
	}
}

// TestMarshal_typedNilPointerInInterface guards the partially-built-value
// case: an interface holding a typed-nil pointer (not a nil interface)
// takes the pointer path and must marshal to "null" without panicking —
// boxing must not turn it into a non-nil pointer that a pointer-receiver
// MarshalJSON would dereference.
func TestMarshal_typedNilPointerInInterface(t *testing.T) {
	var slot iface = (*leaf)(nil)
	raw, err := jsonpoly.Marshal(slot)
	if err != nil {
		t.Fatalf("typed-nil pointer: %v", err)
	}
	if string(raw) != "null" {
		t.Fatalf("typed-nil pointer should marshal to null, got %q", raw)
	}

	// Inside a non-empty slice it renders as a literal null element.
	got, err := jsonpoly.MarshalSlice([]iface{(*leaf)(nil), leaf{Value: "a"}})
	if err != nil {
		t.Fatalf("slice with typed-nil element: %v", err)
	}
	if want := `[null,{"_type":"LEAF","value":"a"}]`; string(got) != want {
		t.Fatalf("slice with typed-nil element: got %s want %s", got, want)
	}
}

// TestMarshal_nilUnderMandatoryFieldIsNull locks the documented contract:
// the nil RawMessage from Marshal(nil), placed in a mandatory (no
// omitempty) wire field, re-emits as JSON null — matching encoding/json's
// treatment of a nil interface field.
func TestMarshal_nilUnderMandatoryFieldIsNull(t *testing.T) {
	raw, err := jsonpoly.Marshal(nil)
	if err != nil {
		t.Fatalf("nil: %v", err)
	}
	type wire struct {
		Name json.RawMessage `json:"name"`
	}
	out, err := json.Marshal(wire{Name: raw})
	if err != nil {
		t.Fatalf("wire marshal: %v", err)
	}
	if want := `{"name":null}`; string(out) != want {
		t.Fatalf("mandatory field with nil RawMessage: got %s want %s", out, want)
	}
}

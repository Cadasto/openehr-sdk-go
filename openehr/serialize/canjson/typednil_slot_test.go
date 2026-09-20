package canjson_test

import (
	jsonv1 "encoding/json"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestTypedNilInOptionalSlot pins the fourth encoded-byte change of ADR 0022
// (REQ-052 § Typed-nil in an optional polymorphic slot): a typed-nil pointer
// held in an interface-typed optional attribute is OMITTED by canjson (v2
// omitempty drops a member that would encode as null) but spelled "null" by a
// v1 encoding/json caller (v1's legacy omitempty treats only a nil interface as
// empty). Both decode back to a nil interface. The canjson omission is the
// spelling rm.IsTypedNil / ValidateRM / the simplified encoders already give
// such a value, so it is intentional, not a regression.
//
// Can-fail controls: add jsonv1.OmitEmptyWithLegacySemantics(false) to
// typereg.EncodeOptions and the v1 assertion below goes red; retag NullReason
// `omitzero` in bmmgen's renderField and regenerate and the canjson assertion
// goes red (a typed-nil is not the zero interface, so omitzero would emit null).
func TestTypedNilInOptionalSlot(t *testing.T) {
	e := &rm.Element{ArchetypeNodeID: "at0000", Name: rm.DVText{Value: "n"}, NullReason: (*rm.DVText)(nil)}

	got, err := canjson.Marshal(e)
	if err != nil {
		t.Fatalf("canjson.Marshal(%T): %v", e, err)
	}
	if strings.Contains(string(got), `"null_reason"`) {
		t.Errorf("canjson.Marshal(typed-nil null_reason) = %s\nwant the member absent", got)
	}

	v1, err := jsonv1.Marshal(e)
	if err != nil {
		t.Fatalf("encoding/json.Marshal(%T): %v", e, err)
	}
	if !strings.Contains(string(v1), `"null_reason":null`) {
		t.Errorf("encoding/json.Marshal(typed-nil null_reason) = %s\nwant it to contain %q (v1 legacy omitempty keeps a non-nil interface)", v1, `"null_reason":null`)
	}

	var back rm.Element
	if err := canjson.Unmarshal(v1, &back); err != nil {
		t.Fatalf("canjson.Unmarshal(%s): %v", v1, err)
	}
	if back.NullReason != nil {
		t.Errorf("canjson.Unmarshal(%s): NullReason = %v, want nil interface", v1, back.NullReason)
	}
}

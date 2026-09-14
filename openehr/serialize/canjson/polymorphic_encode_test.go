package canjson_test

import (
	v1json "encoding/json"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestEncodeSubstitutedSubtypeKeepsType reproduces REQ-052 sub-gap A:
// a DV_CODED_TEXT *value* (not pointer) placed in a DVTextLike slot must
// still emit its mandatory `_type` on the wire, and keep the nested
// CODE_PHRASE `_type`, so the value round-trips as DV_CODED_TEXT rather
// than silently degrading to DV_TEXT.
func TestEncodeSubstitutedSubtypeKeepsType(t *testing.T) {
	el := &rm.Element{
		ArchetypeNodeID: "at0000",
		// Value, not &rm.DVCodedText{...}: the exact rmwrite coercion path.
		Name: rm.DVCodedText{
			DVText:       rm.DVText{Value: "Episode A"},
			DefiningCode: rm.CodePhrase{CodeString: "at0001", TerminologyID: rm.TerminologyID{Value: "local"}},
		},
		Value: &rm.DVText{Value: "x"},
	}

	data, err := canjson.Marshal(el)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(data)
	if !strings.Contains(js, `"name":{"_type":"DV_CODED_TEXT"`) {
		t.Fatalf("name lost its DV_CODED_TEXT _type:\n%s", js)
	}
	if !strings.Contains(js, `"defining_code":{"_type":"CODE_PHRASE"`) {
		t.Fatalf("nested defining_code lost its CODE_PHRASE _type:\n%s", js)
	}

	var back rm.Element
	if err := canjson.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	coded, ok := codedName(back.Name)
	if !ok {
		t.Fatalf("name re-decoded as %T, want DV_CODED_TEXT", back.Name)
	}
	if coded.DefiningCode.CodeString != "at0001" {
		t.Fatalf("defining_code dropped on round-trip: %+v", coded)
	}
}

// TestEncodeSubstitutedSubtypeInSliceKeepsType covers the slice arm:
// a DV_CODED_TEXT value inside a []DVTextLike (DV_PARAGRAPH.items) must
// also emit its `_type`.
func TestEncodeSubstitutedSubtypeInSliceKeepsType(t *testing.T) {
	p := &rm.DVParagraph{
		Items: []rm.DVTextLike{
			rm.DVText{Value: "plain"},
			rm.DVCodedText{
				Value:        "coded",
				DefiningCode: rm.CodePhrase{CodeString: "at0002", TerminologyID: rm.TerminologyID{Value: "local"}},
			},
		},
	}
	data, err := canjson.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(data)
	if !strings.Contains(js, `"_type":"DV_TEXT"`) || !strings.Contains(js, `"_type":"DV_CODED_TEXT"`) {
		t.Fatalf("slice elements lost their _type:\n%s", js)
	}
}

// TestEncodeConcreteIntervalKeepsBoundType locks the DV_INTERVAL[T]
// behaviour documented in rm/doc.go: a concrete DVInterval[DVQuantity]
// holding *value* bounds still emits each bound's `_type` because the
// streaming codec gives DVQuantity a value-receiver MarshalJSONTo (ADR 0022),
// so the method runs whether the bound is held by value or pointer. The
// `_type` must also survive a round-trip (REQ-052).
func TestEncodeConcreteIntervalKeepsBoundType(t *testing.T) {
	iv := &rm.DVInterval[rm.DVQuantity]{}
	iv.Lower = rm.DVQuantity{Magnitude: 5, Units: "cm"}
	iv.Upper = rm.DVQuantity{Magnitude: 20, Units: "cm"}
	iv.LowerIncluded, iv.UpperIncluded = true, true

	data, err := canjson.Marshal(iv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"_type":"DV_QUANTITY"`) {
		t.Fatalf("interval value bound lost its DV_QUANTITY _type:\n%s", data)
	}

	// `_type` survives a round-trip through the generic unmarshaller.
	var back rm.DVInterval[rm.DVQuantity]
	if err := canjson.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	again, err := canjson.Marshal(&back)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !strings.Contains(string(again), `"_type":"DV_QUANTITY"`) {
		t.Fatalf("interval bound _type dropped on round-trip:\n%s", again)
	}
}

// TestPolymorphicSlotEncoding re-homes the assertions from the deleted
// openehr/internal/jsonpoly package (git show 90473f9f:
// openehr/internal/jsonpoly/jsonpoly.go and jsonpoly_test.go). jsonpoly
// existed because v1 encoding/json only calls a pointer-receiver
// MarshalJSON when the receiver is addressable, so a concrete DV_CODED_TEXT
// *value* (not a pointer) placed in a DVTextLike-typed field silently
// dropped its mandatory `_type` (REQ-052 sub-gap A); jsonpoly boxed such
// values into a pointer before marshalling to route around that rule. It
// is unnecessary now: the generator gives every concrete type's
// MarshalJSONTo a value receiver (ADR 0022), and value-receiver methods
// are always in the pointer's method set too, addressable or not (go doc
// encoding/json/v2 Marshal: "Functions or methods that operate on *T are
// only called when encoding a value of type T ... or a non-nil value of
// *T" places no addressability condition on T itself).
func TestPolymorphicSlotEncoding(t *testing.T) {
	coded := rm.DVCodedText{
		Value:        "Episode A",
		DefiningCode: rm.CodePhrase{CodeString: "at0001", TerminologyID: rm.TerminologyID{Value: "local"}},
	}

	cases := []struct {
		name string
		// value is marshalled through canjson.Marshal; when alsoV1 is set,
		// it is also marshalled through v1 encoding/json.Marshal and must
		// carry the same contains substrings there too.
		value    any
		contains []string // substrings the canonical wire MUST carry
		absent   []string // substrings the canonical wire MUST NOT carry
		alsoV1   bool
	}{
		{
			// jsonpoly_test.go TestMarshal_valueInInterfaceEmitsType, value
			// arm: a DV_CODED_TEXT value (not a pointer) placed in the
			// DVTextLike slot must keep its `_type`. Verified can-fail
			// control for the alsoV1 half: temporarily reverting
			// DVCodedText's generated MarshalJSONTo to a pointer receiver
			// (undoing ADR 0022) leaves canjson.Marshal (v2) green here, since
			// go doc encoding/json/v2 Marshal says "Marshal ensures that a value
			// is always addressable (by copying the value if necessary) so
			// that these functions and methods can be consistently called".
			// It turns the v1 assertion below red, because v1's default
			// CallMethodsWithLegacySemantics(true) restores the old rule
			// that a pointer-receiver method is not called on a value
			// "obtained from an interface" (go doc
			// encoding/json.CallMethodsWithLegacySemantics), exactly
			// reproducing the REQ-052 sub-gap A bug jsonpoly existed to fix.
			name:     "a value held in a mandatory interface slot keeps _type",
			value:    &rm.Element{ArchetypeNodeID: "at0000", Name: coded},
			contains: []string{`"name":{"_type":"DV_CODED_TEXT"`},
			alsoV1:   true,
		},
		{
			// jsonpoly_test.go TestMarshal_valueInInterfaceEmitsType,
			// pointer arm: already addressable, so it was never affected by
			// the bug above and stays green under both receiver styles.
			name:     "a pointer held in the same interface slot keeps _type",
			value:    &rm.Element{ArchetypeNodeID: "at0000", Name: &coded},
			contains: []string{`"name":{"_type":"DV_CODED_TEXT"`},
			alsoV1:   true,
		},
		{
			// jsonpoly.go:29-33: "under a mandatory field encoding/json
			// re-emits it as JSON null."
			name:     "a nil interface in a mandatory slot encodes JSON null",
			value:    &rm.Element{ArchetypeNodeID: "at0000"}, // Name left nil
			contains: []string{`"name":null`},
		},
		{
			// jsonpoly.go:29-33, the omitempty half: "under an omitempty
			// wire field the key is omitted." NullReason is DVTextLike
			// `json:"null_reason,omitempty"` (data_structures_representation_gen.go).
			name:   "a nil interface in an omitempty slot is omitted",
			value:  &rm.Element{ArchetypeNodeID: "at0000", Name: coded}, // NullReason left nil
			absent: []string{`"null_reason"`},
		},
		{
			// jsonpoly.go:53-57 (MarshalSlice): "an empty or nil slice
			// returns a nil RawMessage, preserving encoding/json's
			// omitempty behaviour." Agent.Languages is []DVTextLike
			// `json:"languages,omitempty"` (demographic_gen.go).
			name:   "a nil polymorphic slice under omitempty is omitted",
			value:  &rm.Agent{ArchetypeNodeID: "at0000", Name: coded}, // Languages left nil
			absent: []string{`"languages"`},
		},
		{
			name:   "an empty polymorphic slice under omitempty is omitted",
			value:  &rm.Agent{ArchetypeNodeID: "at0000", Name: coded, Languages: []rm.DVTextLike{}},
			absent: []string{`"languages"`},
		},
		{
			// jsonpoly_test.go TestMarshal_typedNilPointerInInterface: a
			// typed-nil pointer takes the pointer path, not the nil-interface
			// path, and must marshal to null without dereferencing. Under
			// encoding/json/v2 that guarantee is the stdlib's own, and
			// unconditional: "Functions or methods that operate on *T are
			// only called ... [for] a non-nil value of *T", and "A Go
			// pointer is encoded as a JSON null if nil" (go doc
			// encoding/json/v2 Marshal), so MarshalJSONTo is never invoked on
			// this receiver at all, regardless of encode options. Verified:
			// adding jsonv1.CallMethodsWithLegacySemantics(true) to
			// typereg.EncodeOptions (its documented nil-pointer-interface
			// bullet reads like it would force the call) leaves this row
			// green; that option is scoped to the legacy Marshaler
			// interface, not MarshalerTo. canjson does not implement or
			// configure this guarantee itself, so no in-repo mutation turns
			// it red; it is pinned here because it is the exact case
			// jsonpoly_test.go guarded and the Task 4 reviewer named as
			// still holding after jsonpoly's deletion.
			name:     "a typed-nil pointer in a polymorphic slot marshals to null, not a panic",
			value:    &rm.Element{ArchetypeNodeID: "at0000", Name: (*rm.DVCodedText)(nil)},
			contains: []string{`"name":null`},
		},
		{
			// The real can-fail control behind item 2: since Go 1.27, v1
			// also dispatches to MarshalJSONTo (see the two alsoV1 rows
			// above stay green under v1 too), so a type carrying the
			// generated marshaler emits `_type` under both entry points
			// regardless of value versus pointer. What still discriminates
			// is whether the type carries the generated marshaler at all:
			// polyNoMarshaler has none, so it falls to plain struct
			// encoding and never gains `_type`. Mutate this row's value to
			// a type generated by the RM codegen and it goes red (gains a
			// `_type` this row asserts it must not have).
			name:   "a type with no marshaler in an any field emits no _type",
			value:  &polySlotHolder{Slot: polyNoMarshaler{Value: "x"}},
			absent: []string{`"_type"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := canjson.Marshal(tc.value)
			if err != nil {
				t.Fatalf("canjson.Marshal(%T) returned error %v, want success", tc.value, err)
			}
			wire := string(data)
			for _, want := range tc.contains {
				if !strings.Contains(wire, want) {
					t.Errorf("canjson.Marshal(%T) = %s\nwant substring %q", tc.value, wire, want)
				}
			}
			for _, notWant := range tc.absent {
				if strings.Contains(wire, notWant) {
					t.Errorf("canjson.Marshal(%T) = %s\nwant %q absent", tc.value, wire, notWant)
				}
			}

			if !tc.alsoV1 {
				return
			}
			v1Data, err := v1json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("encoding/json.Marshal(%T) returned error %v, want success (v1 dispatches to MarshalJSONTo since Go 1.27)", tc.value, err)
			}
			v1Wire := string(v1Data)
			for _, want := range tc.contains {
				if !strings.Contains(v1Wire, want) {
					t.Errorf("encoding/json.Marshal(%T) = %s\nwant substring %q (v1 dispatches to the generated MarshalJSONTo since Go 1.27, Task 2 Q7)", tc.value, v1Wire, want)
				}
			}
		})
	}
}

// polySlotHolder isolates the no-marshaler control above from the RM types
// it is compared against: a single `any` field standing in for a
// polymorphic slot, with nothing else on the wire to interfere.
type polySlotHolder struct {
	Slot any `json:"slot"`
}

// polyNoMarshaler carries neither MarshalJSONTo nor v1's MarshalJSON.
// Unlike every generated RM data type, canjson.Marshal has no per-type
// method to dispatch to for it, so plain encoding/json/v2 struct encoding
// runs and no `_type` discriminator appears. That is the fact the no-
// marshaler control above turns on: `_type` presence tracks whether a type
// carries the generated marshaler, not whether the value is held by
// pointer or by value (the latter stopped mattering once MarshalJSONTo
// took a value receiver).
type polyNoMarshaler struct {
	Value string `json:"value"`
}

func codedName(n rm.DVTextLike) (*rm.DVCodedText, bool) {
	switch v := n.(type) {
	case *rm.DVCodedText:
		return v, true
	case rm.DVCodedText:
		return &v, true
	}
	return nil, false
}

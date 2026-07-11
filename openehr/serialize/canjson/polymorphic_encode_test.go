package canjson_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestEncodeSubstitutedSubtypeKeepsType reproduces REQ-052 sub-gap A:
// a DV_CODED_TEXT *value* (not pointer) placed in a DVTextLike slot must
// still emit its mandatory `_type` on the wire — and keep the nested
// CODE_PHRASE `_type` — so the value round-trips as DV_CODED_TEXT rather
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
				DVText:       rm.DVText{Value: "coded"},
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
// exception documented in rm/doc.go: the generic bounds are NOT routed
// through jsonpoly, yet a concrete DVInterval[DVQuantity] holding *value*
// bounds still emits each bound's `_type` — the bound is an addressable
// struct field, so its pointer-receiver MarshalJSON runs when the wire
// struct is marshalled by-pointer. The `_type` must also survive a
// round-trip (REQ-052).
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

func codedName(n rm.DVTextLike) (*rm.DVCodedText, bool) {
	switch v := n.(type) {
	case *rm.DVCodedText:
		return v, true
	case rm.DVCodedText:
		return &v, true
	}
	return nil, false
}

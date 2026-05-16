package canjson_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestUnmarshalLeafConcreteType — a non-polymorphic leaf type
// (DV_QUANTITY) decodes cleanly with default encoding/json: no
// generated UnmarshalJSON required.
func TestUnmarshalLeafConcreteType(t *testing.T) {
	in := []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`)
	var q rm.DVQuantity
	if err := canjson.Unmarshal(in, &q); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if q.Magnitude != 80.5 || q.Units != "kg" {
		t.Errorf("got Magnitude=%v Units=%v; want 80.5 kg", q.Magnitude, q.Units)
	}
}

// TestUnmarshalCompositionDispatchesContent — Composition.content is a
// []ContentItem; the generated UnmarshalJSON MUST consult typereg
// per-item and produce the right concrete types.
func TestUnmarshalCompositionDispatchesContent(t *testing.T) {
	in := []byte(`{
        "_type": "COMPOSITION",
        "archetype_node_id": "x",
        "name": {"_type": "DV_TEXT", "value": "x"},
        "language": {"_type": "CODE_PHRASE", "code_string": "en"},
        "territory": {"_type": "CODE_PHRASE", "code_string": "GB"},
        "category": {"_type": "DV_CODED_TEXT", "value": "event"},
        "composer": {"_type": "PARTY_SELF"},
        "content": [
            {"_type": "OBSERVATION", "archetype_node_id": "obs1", "name": {"_type":"DV_TEXT","value":"obs1"}, "language":{"_type":"CODE_PHRASE","code_string":"en"}, "encoding":{"_type":"CODE_PHRASE","code_string":"UTF-8"}, "subject":{"_type":"PARTY_SELF"}}
        ]
    }`)
	var c rm.Composition
	if err := canjson.Unmarshal(in, &c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(c.Content) != 1 {
		t.Fatalf("content len = %d; want 1", len(c.Content))
	}
	obs, ok := c.Content[0].(*rm.Observation)
	if !ok {
		t.Errorf("content[0] is %T; want *rm.Observation", c.Content[0])
	} else if obs.ArchetypeNodeID != "obs1" {
		t.Errorf("obs.ArchetypeNodeID = %q; want obs1", obs.ArchetypeNodeID)
	}
	if _, ok := c.Composer.(*rm.PartySelf); !ok {
		t.Errorf("composer is %T; want *rm.PartySelf", c.Composer)
	}
}

// TestUnmarshalUnknownTypeWrapsTypereg — an unrecognised `_type` at a
// polymorphic site MUST return an error that errors.Is against
// typereg.ErrUnknownType (PROBE-031).
func TestUnmarshalUnknownTypeWrapsTypereg(t *testing.T) {
	in := []byte(`{
        "_type": "COMPOSITION",
        "archetype_node_id": "x",
        "name": {"_type": "DV_TEXT", "value": "x"},
        "language": {"_type": "CODE_PHRASE", "code_string": "en"},
        "territory": {"_type": "CODE_PHRASE", "code_string": "GB"},
        "category": {"_type": "DV_CODED_TEXT", "value": "event"},
        "composer": {"_type": "NEVER_REGISTERED_TYPE"}
    }`)
	var c rm.Composition
	err := canjson.Unmarshal(in, &c)
	if err == nil {
		t.Fatal("expected error for unknown _type")
	}
	if !errors.Is(err, typereg.ErrUnknownType) {
		t.Errorf("err = %v; want errors.Is(_, typereg.ErrUnknownType)", err)
	}
}

// TestUnmarshalMissingTypeStrictDefault — strict default: a missing
// `_type` at a polymorphic site is an error wrapping
// typereg.ErrMissingType.
func TestUnmarshalMissingTypeStrictDefault(t *testing.T) {
	in := []byte(`{
        "_type": "COMPOSITION",
        "archetype_node_id": "x",
        "name": {"_type": "DV_TEXT", "value": "x"},
        "language": {"_type": "CODE_PHRASE", "code_string": "en"},
        "territory": {"_type": "CODE_PHRASE", "code_string": "GB"},
        "category": {"_type": "DV_CODED_TEXT", "value": "event"},
        "composer": {"name": {"value": "Dr. X"}}
    }`)
	var c rm.Composition
	err := canjson.Unmarshal(in, &c)
	if err == nil {
		t.Fatal("expected error for missing _type at polymorphic site")
	}
	if !errors.Is(err, typereg.ErrMissingType) {
		t.Errorf("err = %v; want errors.Is(_, typereg.ErrMissingType)", err)
	}
}

// TestDecodeErrorCarriesPath — the DecodeError envelope MUST carry
// a JSON-pointer-ish path so callers can locate the bad node.
func TestDecodeErrorCarriesPath(t *testing.T) {
	in := []byte(`{
        "_type": "COMPOSITION",
        "archetype_node_id": "x",
        "name": {"_type": "DV_TEXT", "value": "x"},
        "language": {"_type": "CODE_PHRASE", "code_string": "en"},
        "territory": {"_type": "CODE_PHRASE", "code_string": "GB"},
        "category": {"_type": "DV_CODED_TEXT", "value": "event"},
        "composer": {"_type": "PARTY_SELF"},
        "content": [
            {"_type": "BOGUS_ITEM"}
        ]
    }`)
	var c rm.Composition
	err := canjson.Unmarshal(in, &c)
	if err == nil {
		t.Fatal("expected error for bogus _type inside content[0]")
	}
	var de *canjson.DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v; want *canjson.DecodeError", err)
	}
	if !strings.Contains(de.Path, "content") {
		t.Errorf("DecodeError.Path = %q; want path to mention content", de.Path)
	}
}

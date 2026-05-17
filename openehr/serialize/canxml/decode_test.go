package canxml_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestUnmarshalLeafConcreteType — a non-polymorphic leaf
// (DV_QUANTITY) decodes from canonical XML without any polymorphic
// dispatch.
func TestUnmarshalLeafConcreteType(t *testing.T) {
	in := []byte(`<dv_quantity xmlns="http://schemas.openehr.org/v1"><magnitude>80.5</magnitude><units>kg</units></dv_quantity>`)
	var q rm.DVQuantity
	if err := canxml.Unmarshal(in, &q); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if float64(q.Magnitude) != 80.5 || q.Units != "kg" {
		t.Errorf("got Magnitude=%v Units=%v; want 80.5 kg", q.Magnitude, q.Units)
	}
}

// TestUnmarshalCompositionDispatchesContent — Composition.content
// (polymorphic []ContentItem) MUST dispatch via xsi:type and produce
// the right concrete type.
func TestUnmarshalCompositionDispatchesContent(t *testing.T) {
	in := []byte(`<composition xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><name><value>x</value></name><archetype_node_id>x</archetype_node_id><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language><territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory><category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category><composer xsi:type="PARTY_SELF"></composer><content xsi:type="OBSERVATION"><name><value>obs1</value></name><archetype_node_id>obs1</archetype_node_id><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language><encoding><terminology_id><value>IANA_character-sets</value></terminology_id><code_string>UTF-8</code_string></encoding><subject xsi:type="PARTY_SELF"></subject></content></composition>`)
	var c rm.Composition
	if err := canxml.Unmarshal(in, &c); err != nil {
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

// TestUnmarshalUnknownXSITypeWrapsTypereg — an unregistered
// `xsi:type` at a polymorphic site MUST return an error that
// errors.Is against typereg.ErrUnknownType (PROBE-034).
func TestUnmarshalUnknownXSITypeWrapsTypereg(t *testing.T) {
	in := []byte(`<composition xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><name><value>x</value></name><archetype_node_id>x</archetype_node_id><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language><territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory><category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category><composer xsi:type="NEVER_REGISTERED_TYPE"></composer></composition>`)
	var c rm.Composition
	err := canxml.Unmarshal(in, &c)
	if err == nil {
		t.Fatal("expected error for unknown xsi:type")
	}
	if !errors.Is(err, typereg.ErrUnknownType) {
		t.Errorf("err = %v; want errors.Is(_, typereg.ErrUnknownType)", err)
	}
}

// TestUnmarshalMissingXSITypeStrictDefault — strict default: a
// missing `xsi:type` at a polymorphic site is an error wrapping
// typereg.ErrMissingType.
func TestUnmarshalMissingXSITypeStrictDefault(t *testing.T) {
	in := []byte(`<composition xmlns="http://schemas.openehr.org/v1"><name><value>x</value></name><archetype_node_id>x</archetype_node_id><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language><territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory><category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category><composer><name><value>Dr. X</value></name></composer></composition>`)
	var c rm.Composition
	err := canxml.Unmarshal(in, &c)
	if err == nil {
		t.Fatal("expected error for missing xsi:type at polymorphic site")
	}
	if !errors.Is(err, typereg.ErrMissingType) {
		t.Errorf("err = %v; want errors.Is(_, typereg.ErrMissingType)", err)
	}
}

// TestUnmarshalDecodeErrorCarriesPath — the DecodeError envelope
// MUST carry an element-path so callers can locate the bad node.
func TestUnmarshalDecodeErrorCarriesPath(t *testing.T) {
	in := []byte(`<composition xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><name><value>x</value></name><archetype_node_id>x</archetype_node_id><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language><territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory><category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category><composer xsi:type="PARTY_SELF"></composer><content xsi:type="BOGUS_ITEM"></content></composition>`)
	var c rm.Composition
	err := canxml.Unmarshal(in, &c)
	if err == nil {
		t.Fatal("expected error for bogus xsi:type inside content[0]")
	}
	var de *canxml.DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v; want *canxml.DecodeError", err)
	}
	if !strings.Contains(de.Path, "content") {
		t.Errorf("DecodeError.Path = %q; want path to mention content", de.Path)
	}
}

// TestUnmarshalRejectsXMIType — ITS-XML pins xsi:type; xmi:type on
// the wire is a hard error wrapping canxml.ErrInvalidShape.
func TestUnmarshalRejectsXMIType(t *testing.T) {
	// Note: xmlns:xmi prefix bound to the XMI namespace.
	in := []byte(`<composition xmlns="http://schemas.openehr.org/v1" xmlns:xmi="http://www.omg.org/XMI"><name><value>x</value></name><archetype_node_id>x</archetype_node_id><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language><territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory><category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category><composer xmi:type="PARTY_SELF"></composer></composition>`)
	var c rm.Composition
	err := canxml.Unmarshal(in, &c)
	if err == nil {
		t.Fatal("expected error for xmi:type discriminator")
	}
	if !errors.Is(err, canxml.ErrInvalidShape) {
		t.Errorf("err = %v; want errors.Is(_, canxml.ErrInvalidShape)", err)
	}
}

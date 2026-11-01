package canjson_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// itemStructureList is one item-structure list that canonical JSON writes as
// [] when it is present and empty.
type itemStructureList struct {
	class  string // the BMM class
	member string // the list attribute
	// value builds an instance whose list is nil, or a non-nil empty slice
	// when present is true. A present instance also carries a non-nil empty
	// links, an optional list outside the exception.
	value func(present bool) any
	// target returns a fresh decode target.
	target func() any
	// list reads the list of a decoded target: whether it is non-nil, and
	// its length.
	list func(v any) (present bool, n int)
}

func itemStructureLists() []itemStructureList {
	name := rm.DVText{Value: "list"}
	return []itemStructureList{
		{
			class: "ITEM_LIST", member: "items",
			value: func(present bool) any {
				v := &rm.ItemList{ArchetypeNodeID: "at0007", Name: name}
				if present {
					v.Items, v.Links = []rm.Element{}, []rm.Link{}
				}
				return v
			},
			target: func() any { return new(rm.ItemList) },
			list: func(v any) (bool, int) {
				l := v.(*rm.ItemList).Items
				return l != nil, len(l)
			},
		},
		{
			class: "ITEM_TREE", member: "items",
			value: func(present bool) any {
				v := &rm.ItemTree{ArchetypeNodeID: "at0007", Name: name}
				if present {
					v.Items, v.Links = []rm.Item{}, []rm.Link{}
				}
				return v
			},
			target: func() any { return new(rm.ItemTree) },
			list: func(v any) (bool, int) {
				l := v.(*rm.ItemTree).Items
				return l != nil, len(l)
			},
		},
		{
			class: "ITEM_TABLE", member: "rows",
			value: func(present bool) any {
				v := &rm.ItemTable{ArchetypeNodeID: "at0007", Name: name}
				if present {
					v.Rows, v.Links = []rm.Cluster{}, []rm.Link{}
				}
				return v
			},
			target: func() any { return new(rm.ItemTable) },
			list: func(v any) (bool, int) {
				l := v.(*rm.ItemTable).Rows
				return l != nil, len(l)
			},
		},
	}
}

// TestREQ052_ItemStructureListKeepsPresentEmpty pins the one exception to the
// re-encode collapse of an empty optional list: ITEM_LIST.items,
// ITEM_TREE.items and ITEM_TABLE.rows (issue 341). An archetype may make such
// a list mandatory by existence, and a CDR refuses a structure that leaves it
// out, so canonical JSON writes a present empty one as [] and leaves out only
// a nil one.
//
// For each list: a present empty one encodes as [] while the empty links
// beside it, an optional list outside the exception, is still left out; a nil
// one is left out; a decoded [] is a non-nil empty slice that re-encodes as
// []; and canonical XML, which has no spelling for a present empty list,
// writes no element for it and decodes the result as absent.
//
// REQ-052 § An absent mandatory member; § REQ-056 for the canonical-XML side.
func TestREQ052_ItemStructureListKeepsPresentEmpty(t *testing.T) {
	for _, tc := range itemStructureLists() {
		t.Run(tc.class+"."+tc.member, func(t *testing.T) {
			t.Run("present empty encodes as []", func(t *testing.T) {
				out, err := canjson.Marshal(tc.value(true))
				if err != nil {
					t.Fatalf("canjson.Marshal(%s with an empty %s): %v", tc.class, tc.member, err)
				}
				if !isEmptyArrayMember(t, out, tc.member) {
					t.Errorf("canjson.Marshal(%s with an empty %s) = %s, want the member %q written as []", tc.class, tc.member, out, tc.member)
				}
				if _, ok := members(t, out)["links"]; ok {
					t.Errorf("canjson.Marshal(%s with an empty links) = %s, want links left out: only the item-structure lists keep a present empty list", tc.class, out)
				}
			})

			t.Run("nil is left out", func(t *testing.T) {
				out, err := canjson.Marshal(tc.value(false))
				if err != nil {
					t.Fatalf("canjson.Marshal(%s with a nil %s): %v", tc.class, tc.member, err)
				}
				if raw, ok := members(t, out)[tc.member]; ok {
					t.Errorf("canjson.Marshal(%s with a nil %s) writes %s = %s, want the member left out", tc.class, tc.member, tc.member, raw)
				}
			})

			t.Run("decoded [] re-encodes as []", func(t *testing.T) {
				in := `{"_type":"` + tc.class + `","archetype_node_id":"at0007","name":{"_type":"DV_TEXT","value":"list"},"` + tc.member + `":[]}`
				v := tc.target()
				if err := canjson.Unmarshal([]byte(in), v); err != nil {
					t.Fatalf("canjson.Unmarshal(%s): %v", in, err)
				}
				if present, n := tc.list(v); !present || n != 0 {
					t.Fatalf("canjson.Unmarshal(%s) -> %s non-nil %t, length %d; want a non-nil empty slice", in, tc.member, present, n)
				}
				out, err := canjson.Marshal(v)
				if err != nil {
					t.Fatalf("canjson.Marshal(decoded %s): %v", in, err)
				}
				if !isEmptyArrayMember(t, out, tc.member) {
					t.Errorf("re-encoded %s = %s, want the member %q written as []", in, out, tc.member)
				}
			})

			t.Run("canonical XML writes no element", func(t *testing.T) {
				out, err := canxml.Marshal(tc.value(true))
				if err != nil {
					t.Fatalf("canxml.Marshal(%s with an empty %s): %v", tc.class, tc.member, err)
				}
				// The control: the scan finds an element the encoder does
				// write, so a missing member is not a broken scan.
				if !hasXMLElement(t, out, "name") {
					t.Fatalf("canxml.Marshal(%s) = %s: the scan finds no name element, so it checks nothing", tc.class, out)
				}
				if hasXMLElement(t, out, tc.member) {
					t.Errorf("canxml.Marshal(%s with an empty %s) = %s, want no %s element", tc.class, tc.member, out, tc.member)
				}
				v := tc.target()
				if err := canxml.Unmarshal(out, v); err != nil {
					t.Fatalf("canxml.Unmarshal(%s): %v", out, err)
				}
				if present, n := tc.list(v); present || n != 0 {
					t.Errorf("canxml.Unmarshal(%s) -> %s non-nil %t, length %d; want nil, the list read as absent", out, tc.member, present, n)
				}
			})
		})
	}
}

// members returns the top-level members of the JSON object b.
func members(t *testing.T, b []byte) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode encoded output %s for its members: %v", b, err)
	}
	return m
}

// isEmptyArrayMember reports whether the JSON object b carries the member
// name as an empty array: present, not null, and with no elements.
func isEmptyArrayMember(t *testing.T, b []byte, name string) bool {
	t.Helper()
	raw, ok := members(t, b)[name]
	if !ok {
		return false
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode member %q = %s as an array: %v", name, raw, err)
	}
	return list != nil && len(list) == 0
}

// hasXMLElement reports whether the XML document b holds an element whose
// local name is local, at any depth.
func hasXMLElement(t *testing.T, b []byte, local string) bool {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return false
		}
		if err != nil {
			t.Fatalf("read the XML %s: %v", b, err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == local {
			return true
		}
	}
}

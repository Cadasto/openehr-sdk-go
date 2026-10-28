package templatecompile_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// heldOPT is a template whose root, of rmType, carries attrs.
func heldOPT(rmType string, attrs ...string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>untyped</value></template_id><concept>untyped</concept>
<definition><rm_type_name>` + rmType + `</rm_type_name><node_id>at0000</node_id>` + strings.Join(attrs, "") +
		`</definition>
</template>`
}

// heldObject is a C_COMPLEX_OBJECT of rmType, "" for one that names no RM
// type, at nodeID, carrying attrs.
func heldObject(rmType, nodeID string, attrs ...string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<node_id>` + nodeID + `</node_id>` + strings.Join(attrs, "") + `</children>`
}

// heldSingle is a C_SINGLE_ATTRIBUTE called name, of existence 0..1 when
// optional and 1..1 otherwise, over children.
func heldSingle(name string, optional bool, children ...string) string {
	lower := 1
	if optional {
		lower = 0
	}
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence>` + closedInterval(lower, 1) + `</existence>` + strings.Join(children, "") + `</attributes>`
}

// heldList is a C_MULTIPLE_ATTRIBUTE called name, of existence 1..1, over
// children.
func heldList(name string, children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence>` + closedInterval(1, 1) + `</existence>` + strings.Join(children, "") +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		openAboveInterval(1) + `</interval></cardinality></attributes>`
}

// TestREQ100_UntypedNodeReadsItsAttributesOnItsHoldingType pins § REQ-100
// Node taxonomy for a node the template gives no RM type: the compile reads
// its attributes on the type the BMM declares for the attribute that holds
// it, read on the holding node's own type, or, for a holding node the
// template gives no type either, on the type the compile reads that node as.
// Where the BMM types the holding attribute with a formal generic parameter
// the holding node's type leaves open, the type the parameter must conform
// to is read. So an attribute the RM makes mandatory reports Required, in
// both compile modes, and the implicit-attribute mode injects the mandatory
// attributes the template leaves out, as on a node of that type. The node's
// own RMTypeName stays empty, and the attributes of a template root that
// names no type, which no attribute holds, are read on no type.
func TestREQ100_UntypedNodeReadsItsAttributesOnItsHoldingType(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// path is the AQL path of the untyped node.
		path string
		// required is a declared attribute of the node the RM makes
		// mandatory on the type it is read as, or "" for none.
		required string
		// optional is a declared attribute of the node the RM leaves
		// optional on that type.
		optional string
		// implicit are the attributes the implicit-attribute mode injects,
		// with their RM types.
		implicit map[string]string
	}{
		{
			name: "an event under a HISTORY's events",
			opt: heldOPT("HISTORY", heldList("events", heldObject("", "at0002",
				heldSingle("data", true, heldObject("ITEM_TREE", "at0001")),
				heldSingle("state", true, heldObject("ITEM_TREE", "at0003"))))),
			path:     "/events[at0002]",
			required: "data",
			optional: "state",
			implicit: map[string]string{"name": "DV_TEXT", "archetype_node_id": "String", "time": "DV_DATE_TIME"},
		},
		{
			name: "an event under the events of a history that names no type",
			opt: heldOPT("OBSERVATION", heldSingle("data", false, heldObject("", "at0001",
				heldList("events", heldObject("", "at0002",
					heldSingle("data", true, heldObject("ITEM_TREE", "at0003"))))))),
			path:     "/data/events[at0002]",
			required: "data",
			implicit: map[string]string{"name": "DV_TEXT", "archetype_node_id": "String", "time": "DV_DATE_TIME"},
		},
		{
			name: "a structure under a POINT_EVENT's data, typed by its formal parameter",
			opt: heldOPT("POINT_EVENT", heldSingle("data", false, heldObject("", "at0001",
				heldSingle("name", true, heldObject("DV_TEXT", ""))))),
			path:     "/data",
			required: "name",
			implicit: map[string]string{"archetype_node_id": "String"},
		},
		{
			name:     "a template root",
			opt:      heldOPT("", heldSingle("name", true, heldObject("DV_TEXT", ""))),
			path:     "/",
			optional: "name",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/implicit=%t", tc.name, implicit), func(t *testing.T) {
				opt, err := template.ParseOPT(strings.NewReader(tc.opt))
				if err != nil {
					t.Fatalf("ParseOPT: %v", err)
				}
				c, err := templatecompile.Compile(opt, templatecompile.Options{SkipImplicitAttributes: !implicit})
				if err != nil {
					t.Fatalf("Compile: %v", err)
				}
				n, err := c.NodeAt(tc.path)
				if err != nil {
					t.Fatalf("NodeAt(%q): %v", tc.path, err)
				}
				if got := n.RMTypeName(); got != "" {
					t.Errorf("RMTypeName() = %q, want the empty name the template writes", got)
				}
				for name, want := range map[string]bool{tc.required: true, tc.optional: false} {
					if name == "" {
						continue
					}
					a := n.Attribute(name)
					if a == nil {
						t.Errorf("declared attribute %q missing", name)
						continue
					}
					if a.Required() != want || a.Implicit() {
						t.Errorf("%s: Required=%v Implicit=%v, want %v, false", name, a.Required(), a.Implicit(), want)
					}
				}
				injected := 0
				for _, a := range n.Attributes() {
					if !a.Implicit() {
						continue
					}
					injected++
					if want, ok := tc.implicit[a.Name()]; !ok || !a.Required() || a.RMTypeName() != want {
						t.Errorf("implicit %s: Required=%v RMTypeName=%q, want an injected attribute of %v", a.Name(), a.Required(), a.RMTypeName(), tc.implicit)
					}
				}
				if want := len(tc.implicit); !implicit {
					if injected != 0 {
						t.Errorf("SkipImplicitAttributes: %d implicit attributes, want none", injected)
					}
				} else if injected != want {
					t.Errorf("%d implicit attributes, want %d: %v", injected, want, tc.implicit)
				}
			})
		}
	}
}

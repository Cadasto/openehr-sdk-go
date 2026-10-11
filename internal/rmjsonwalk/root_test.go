package rmjsonwalk_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// callerStruct is a caller's own type with a member named like an extension.
type callerStruct struct {
	X string `json:"x-a"`
}

// TestREQ170_RootOfReadsTheTargetsRMShape pins what a decode target says
// about the root (REQ-170: every object that decodes as an RM class is
// checked, and only those): the containers around its RM values, the class
// each value is declared as, and the class a generic root binds its type
// parameter to. A target that holds no RM value has no root to check.
func TestREQ170_RootOfReadsTheTargetsRMShape(t *testing.T) {
	m, err := rmjsonwalk.Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	cases := []struct {
		name   string
		target any
		want   rmjsonwalk.Root
		ok     bool
	}{
		{name: "RM class", target: new(rm.Composition), want: rmjsonwalk.Root{Type: "COMPOSITION"}, ok: true},
		{name: "slice", target: new([]rm.DVText), want: rmjsonwalk.Root{Nest: "[", Type: "DV_TEXT"}, ok: true},
		{name: "array", target: new([2]rm.DVText), want: rmjsonwalk.Root{Nest: "[", Type: "DV_TEXT"}, ok: true},
		{name: "map", target: new(map[string]rm.DVText), want: rmjsonwalk.Root{Nest: "{", Type: "DV_TEXT"}, ok: true},
		{name: "map of slices of pointers", target: new(map[string][]*rm.DVText), want: rmjsonwalk.Root{Nest: "{[", Type: "DV_TEXT"}, ok: true},
		{name: "generic bound to a class", target: new(rm.DVInterval[rm.DVQuantity]), want: rmjsonwalk.Root{Type: "DV_INTERVAL", Bound: "DV_QUANTITY"}, ok: true},
		{name: "generic bound to an abstract interface", target: new(rm.DVInterval[rm.DVOrdered]), want: rmjsonwalk.Root{Type: "DV_INTERVAL"}, ok: true},
		{name: "generic payload bound to a class", target: new(rm.OriginalVersion[rm.Composition]), want: rmjsonwalk.Root{Type: "ORIGINAL_VERSION", Bound: "COMPOSITION"}, ok: true},
		{name: "generic payload left open", target: new(rm.OriginalVersion[any]), want: rmjsonwalk.Root{Type: "ORIGINAL_VERSION"}, ok: true},
		{name: "abstract RM interface", target: new(rm.DataValue), want: rmjsonwalk.Root{}, ok: true},
		{name: "narrow RM interface falls back to its parent", target: new(rm.DVTextLike), want: rmjsonwalk.Root{Type: "DV_TEXT"}, ok: true},
		{name: "slice of an RM interface", target: new([]rm.DataValue), want: rmjsonwalk.Root{Nest: "["}, ok: true},
		{name: "nil", target: nil},
		{name: "any", target: new(any)},
		{name: "map of any", target: new(map[string]any)},
		{name: "caller's struct", target: new(callerStruct)},
		{name: "AOM 1.4 class", target: new(aom14.Archetype)},
		{name: "slice of AOM 1.4 class", target: new([]aom14.CComplexObject)},
		{name: "bytes", target: new([]byte)},
		{name: "string", target: new(string)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for range 2 { // the second call reads the kept result
				got, ok := m.RootOf(tc.target)
				if ok != tc.ok || got != tc.want {
					t.Fatalf("RootOf(%T) = %+v, %v, want %+v, %v", tc.target, got, ok, tc.want, tc.ok)
				}
			}
		})
	}
}

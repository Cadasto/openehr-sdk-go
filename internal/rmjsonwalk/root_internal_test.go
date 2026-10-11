package rmjsonwalk

import (
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// Go types for synthetic generic classes, each holding its type-parameter
// attributes as fields with JSON tags.
type (
	agreeingBounds struct {
		Lower rm.DVQuantity  `json:"lower"`
		Upper *rm.DVQuantity `json:"upper,omitzero"`
	}
	disagreeingBounds struct {
		Lower rm.DVQuantity `json:"lower"`
		Upper rm.DVCount    `json:"upper"`
	}
	interfaceBound struct {
		Data rm.DataValue `json:"data"`
	}
	payloadBound struct {
		Data *any `json:"data,omitzero"`
	}
)

// TestBoundOfMapsTheTypeParameterField pins how a generic target binds its
// type parameter (REQ-170 *Scope*, REQ-024 field mapping): the Go field whose
// JSON tag is a type-parameter attribute, peeled of its pointer, names the
// class; a polymorphic RM interface binds the class it stands for, and Go's
// any binds nothing. Attributes of one parameter must agree, and attributes
// of two parameters cannot share one binding: either way there is none.
func TestBoundOfMapsTheTypeParameterField(t *testing.T) {
	attr := func(typ string) rminfo.AttrMeta { return rminfo.AttrMeta{TypeName: typ, DeclaredIn: "PAIR"} }
	class := func(attrs map[string]rminfo.AttrMeta, order ...string) rminfo.ClassMeta {
		return rminfo.ClassMeta{Attributes: attrs, AttrOrder: order}
	}
	data := map[string]rminfo.ClassMeta{
		"PAIR":        class(map[string]rminfo.AttrMeta{"lower": attr("T"), "upper": attr("T"), "name": attr("String")}, "name", "lower", "upper"),
		"TWO_PARAMS":  class(map[string]rminfo.AttrMeta{"lower": attr("T"), "upper": attr("U")}, "lower", "upper"),
		"HOLDER":      class(map[string]rminfo.AttrMeta{"data": attr("T")}, "data"),
		"PLAIN":       class(map[string]rminfo.AttrMeta{"lower": attr("DV_QUANTITY")}, "lower"),
		"DV_QUANTITY": class(map[string]rminfo.AttrMeta{}),
		"DV_COUNT":    class(map[string]rminfo.AttrMeta{}),
		"DATA_VALUE":  {Attributes: map[string]rminfo.AttrMeta{}, AttrOrder: []string{}, Abstract: true},
	}
	m, err := New(rminfo.NewWithAbsence(data, map[string]rminfo.AbsenceReason{"String": rminfo.AbsencePrimitive}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cases := []struct {
		name  string
		goTyp reflect.Type
		class string
		want  string
	}{
		{name: "fields agree, a pointer peeled", goTyp: reflect.TypeFor[agreeingBounds](), class: "PAIR", want: "DV_QUANTITY"},
		{name: "fields disagree", goTyp: reflect.TypeFor[disagreeingBounds](), class: "PAIR"},
		{name: "two parameters", goTyp: reflect.TypeFor[agreeingBounds](), class: "TWO_PARAMS"},
		{name: "polymorphic RM interface", goTyp: reflect.TypeFor[interfaceBound](), class: "HOLDER", want: "DATA_VALUE"},
		{name: "Go's any", goTyp: reflect.TypeFor[payloadBound](), class: "HOLDER"},
		{name: "no type-parameter attribute", goTyp: reflect.TypeFor[agreeingBounds](), class: "PLAIN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.boundOf(tc.goTyp, tc.class); got != tc.want {
				t.Errorf("boundOf(%s, %s) = %q, want %q", tc.goTyp, tc.class, got, tc.want)
			}
		})
	}
}

package bmmtype_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
)

// benchNames are the shapes of type name the callers pass: a plain class
// name (the common case, once per node or attribute), a generic name and a
// nested generic name.
var benchNames = []struct {
	name, typ string
}{
	{name: "plain", typ: "DV_QUANTITY"},
	{name: "generic", typ: "DV_INTERVAL<DV_QUANTITY>"},
	{name: "nested", typ: "DV_INTERVAL<DV_INTERVAL<DV_COUNT>>"},
}

func BenchmarkSplit(b *testing.B) {
	for _, bc := range benchNames {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				bmmtype.Split(bc.typ)
			}
		})
	}
}

func BenchmarkClass(b *testing.B) {
	for _, bc := range benchNames {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				bmmtype.Class(bc.typ)
			}
		})
	}
}

func BenchmarkSubstitute(b *testing.B) {
	cases := []struct {
		name, owner, declared string
	}{
		{name: "plain", owner: "ELEMENT", declared: "DV_TEXT"},
		{name: "generic", owner: "DV_INTERVAL<DV_QUANTITY>", declared: "T"},
		{name: "generic not formal", owner: "DV_INTERVAL<DV_QUANTITY>", declared: "Boolean"},
		{name: "nested", owner: "DV_INTERVAL<DV_INTERVAL<DV_COUNT>>", declared: "T"},
	}
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				bmmtype.Substitute(bc.owner, bc.declared)
			}
		})
	}
}

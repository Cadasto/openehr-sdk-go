package composition_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// benchOptions are the composition options every builder benchmark uses.
func benchOptions() []composition.Option {
	return []composition.Option{
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	}
}

// BenchmarkNewBuilder measures building the skeleton a Builder starts
// from, for a small, a medium and a large template.
func BenchmarkNewBuilder(b *testing.B) {
	for _, name := range []string{"BMI", "vital_signs", "Demonstration.v1"} {
		b.Run(name, func(b *testing.B) {
			c := compileFixture(b, name)
			opts := benchOptions()
			b.ReportAllocs()
			for b.Loop() {
				if _, err := composition.NewBuilder(b.Context(), c, opts...); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSet measures one Set at the systolic pressure, the work a
// caller pays for every value it sets. The queued assignments are drained
// by an untimed Build every so often, so the queue stays short.
func BenchmarkSet(b *testing.B) {
	c := compileFixture(b, "vital_signs")
	bl, err := composition.NewBuilder(b.Context(), c, benchOptions()...)
	if err != nil {
		b.Fatal(err)
	}
	v := &rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"}
	b.ReportAllocs()
	n := 0
	for b.Loop() {
		if err := bl.Set(systolicPath, v); err != nil {
			b.Fatal(err)
		}
		n++
		if n%1024 == 0 {
			b.StopTimer()
			if _, err := bl.Build(); err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
		}
	}
}

// BenchmarkSetBuild measures one Set at the systolic pressure followed by
// the Build that writes it into the composition.
func BenchmarkSetBuild(b *testing.B) {
	c := compileFixture(b, "vital_signs")
	bl, err := composition.NewBuilder(b.Context(), c, benchOptions()...)
	if err != nil {
		b.Fatal(err)
	}
	v := &rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"}
	b.ReportAllocs()
	for b.Loop() {
		if err := bl.Set(systolicPath, v); err != nil {
			b.Fatal(err)
		}
		if _, err := bl.Build(); err != nil {
			b.Fatal(err)
		}
	}
}

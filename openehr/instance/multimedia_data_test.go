package instance_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// checkSizedData fails t unless mm carries inline data, a non-empty
// payload, whose byte count is its size, and no uri: the generator writes
// the payload, so RM Not_empty needs no uri default.
func checkSizedData(t *testing.T, mm *rm.DVMultimedia) {
	t.Helper()
	if len(mm.Data) == 0 {
		t.Errorf("DV_MULTIMEDIA.data is empty, want the example payload")
	}
	if int(mm.Size) != len(mm.Data) {
		t.Errorf("DV_MULTIMEDIA.size = %d, want %d, the bytes of its data %q", mm.Size, len(mm.Data), mm.Data)
	}
	if mm.URI != nil {
		t.Errorf("DV_MULTIMEDIA.uri = %#v, want none beside data", mm.URI)
	}
}

// TestREQ107_CorpusMultimediaCarriesSizedData is the REQ-107 RM defaults
// check, over every vendored template, that each DV_MULTIMEDIA the
// generator writes carries inline data, a non-empty example payload, whose
// byte count is its size, under every policy, both value fills and several
// fixed seeds, and that the payload is one and the same throughout. No
// vendored OPT prohibits a DV_MULTIMEDIA's data or constrains its size.
func TestREQ107_CorpusMultimediaCarriesSizedData(t *testing.T) {
	checked := 0
	payloads := make(map[string]int)
	eachCorpusRun(t, func(run corpusRun) {
		visitRM(run.out, func(path string, mm rm.DVMultimedia) {
			checked++
			payloads[string(mm.Data)]++
			if len(mm.Data) == 0 || int(mm.Size) != len(mm.Data) {
				t.Errorf("%s: DV_MULTIMEDIA at %s has size %d and data %q, want a non-empty payload of that many bytes",
					run, path, mm.Size, mm.Data)
			}
		})
	})
	if checked == 0 {
		t.Fatal("no DV_MULTIMEDIA generated, so nothing was checked")
	}
	if len(payloads) != 1 {
		t.Errorf("DV_MULTIMEDIA payloads = %q, want one, the same under every setting", slices.Sorted(maps.Keys(payloads)))
	}
}

// TestREQ107_MultimediaCarriesSizedData is the REQ-107 RM defaults check
// on inline data: a DV_MULTIMEDIA carries the example payload with its
// byte count as its size, the same payload under both value fills; where
// the OPT prohibits data, the value carries none
// and takes the uri http://example.com instead (Precedence 2); where the
// OPT's constraint on size rejects the payload's byte count, the size the
// walk wrote stays (Precedence 1), and where it admits that count, the
// size is that count. It holds at the root and as an ELEMENT's value,
// under every policy, both value fills and both compile modes, and the RM
// floor and the template validator pass.
func TestREQ107_MultimediaCarriesSizedData(t *testing.T) {
	sizeIn := func(lo, hi float64) string {
		return optSingle("size", optPrimitive("INTEGER", "C_INTEGER", instance.IntRange(instance.Closed(lo), instance.Closed(hi))))
	}
	cases := []struct {
		name  string
		attrs []string
		check func(t *testing.T, mm *rm.DVMultimedia)
	}{
		{
			name:  "silent",
			check: checkSizedData,
		},
		{
			name:  "data prohibited",
			attrs: []string{optProhibitedSingle("data")},
			check: func(t *testing.T, mm *rm.DVMultimedia) {
				t.Helper()
				if len(mm.Data) != 0 {
					t.Errorf("DV_MULTIMEDIA.data = %q, want none: the OPT prohibits it", mm.Data)
				}
				if uri, ok := mm.URI.(*rm.DVURI); !ok || uri == nil || uri.Value != "http://example.com" {
					t.Errorf("DV_MULTIMEDIA.uri = %#v, want a DV_URI http://example.com", mm.URI)
				}
			},
		},
		{
			name:  "size range that rejects the payload's byte count",
			attrs: []string{sizeIn(100, 200)},
			check: func(t *testing.T, mm *rm.DVMultimedia) {
				t.Helper()
				if len(mm.Data) == 0 {
					t.Errorf("DV_MULTIMEDIA.data is empty, want the example payload")
				}
				if mm.Size < 100 || mm.Size > 200 {
					t.Errorf("DV_MULTIMEDIA.size = %d, want the walk's size in 100..200", mm.Size)
				}
			},
		},
		{
			name:  "size range that admits the payload's byte count",
			attrs: []string{sizeIn(1, 1000)},
			check: checkSizedData,
		},
	}
	for _, tc := range cases {
		for _, place := range multimediaPlacements {
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, place.opt(tc.attrs), implicit)
				payloads := make(map[string]bool)
				for _, opts := range defaultsOptions() {
					t.Run(fmt.Sprintf("%s/%s/implicit=%t/%v/%v", tc.name, place.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, opts)
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						mm := place.multimedia(t, out)
						tc.check(t, mm)
						payloads[string(mm.Data)] = true
						checkValid(t, out, c)
					})
				}
				if len(payloads) > 1 {
					t.Errorf("%s/%s/implicit=%t: payloads %q differ between settings, want one", tc.name, place.name, implicit, slices.Sorted(maps.Keys(payloads)))
				}
			}
		}
	}
}

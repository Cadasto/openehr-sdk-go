package instance_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
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

// textMedia reports whether mt is a media type of the text top-level
// type, text/plain among them.
func textMedia(mt rm.CodePhrase) bool {
	return strings.HasPrefix(strings.ToLower(mt.CodeString), "text/")
}

// TestREQ107_CorpusMultimediaCarriesDataOrAURIByMediaType is the REQ-107
// RM defaults check, over every vendored template, that each DV_MULTIMEDIA
// the generator writes carries inline data, a non-empty example payload
// whose byte count is its size, where its media type is a text one, and a
// uri with a positive size and no data where it is any other, under every
// policy, both value fills and several fixed seeds; that the payload is
// one and the same throughout; and that the corpus gives both kinds. No
// vendored OPT prohibits a DV_MULTIMEDIA's data or uri, or constrains its
// size.
func TestREQ107_CorpusMultimediaCarriesDataOrAURIByMediaType(t *testing.T) {
	texts, others := 0, 0
	payloads := make(map[string]int)
	eachCorpusRun(t, func(run corpusRun) {
		visitRM(run.out, func(path string, mm rm.DVMultimedia) {
			if textMedia(mm.MediaType) {
				texts++
				payloads[string(mm.Data)]++
				if len(mm.Data) == 0 || int(mm.Size) != len(mm.Data) || mm.URI != nil {
					t.Errorf("%s: DV_MULTIMEDIA %s at %s has size %d, data %q and uri %#v, want a non-empty payload of that many bytes and no uri",
						run, mm.MediaType.CodeString, path, mm.Size, mm.Data, mm.URI)
				}
				return
			}
			others++
			if len(mm.Data) != 0 || mm.URI == nil || mm.Size <= 0 {
				t.Errorf("%s: DV_MULTIMEDIA %s at %s has size %d, data %q and uri %#v, want a uri, a positive size and no data",
					run, mm.MediaType.CodeString, path, mm.Size, mm.Data, mm.URI)
			}
		})
	})
	if texts == 0 || others == 0 {
		t.Fatalf("DV_MULTIMEDIA generated: %d of a text media type and %d of another, want both", texts, others)
	}
	if len(payloads) != 1 {
		t.Errorf("DV_MULTIMEDIA payloads = %q, want one, the same under every setting", slices.Sorted(maps.Keys(payloads)))
	}
}

// checkURIOnly fails t unless mm carries the uri want, a positive size and
// no data.
func checkURIOnly(t *testing.T, mm *rm.DVMultimedia, want string) {
	t.Helper()
	if len(mm.Data) != 0 {
		t.Errorf("DV_MULTIMEDIA.data = %q, want none beside a uri", mm.Data)
	}
	if uri, ok := mm.URI.(*rm.DVURI); !ok || uri == nil || uri.Value != want {
		t.Errorf("DV_MULTIMEDIA.uri = %#v, want a DV_URI %s", mm.URI, want)
	}
	if mm.Size <= 0 {
		t.Errorf("DV_MULTIMEDIA.size = %d, want a positive size", mm.Size)
	}
}

// TestREQ107_MultimediaTakesAURIForAMediaTypeOtherThanText is the REQ-107
// RM defaults check that a DV_MULTIMEDIA whose media type is not a text
// one carries a uri, with a fixed positive size and no inline data: the
// uri its OPT child gives, where the OPT names one, under every policy,
// and else http://example.com. Where the OPT prohibits the uri, it carries
// the inline example payload sized by its bytes instead, as a text media
// type does; where it prohibits the data, a uri. Where the OPT's
// constraint on size rejects the fixed size, the size the walk wrote
// stays. It holds at the root and as an ELEMENT's value, under every
// policy, both value fills and both compile modes, and the RM floor and
// the template validator pass.
func TestREQ107_MultimediaTakesAURIForAMediaTypeOtherThanText(t *testing.T) {
	png := optSingle("media_type", optCodePhrase("IANA_media-types", "image/png"))
	html := optSingle("media_type", optCodePhrase("IANA_media-types", "text/html"))
	scan := optOptionalSingleOver("uri", optNode("DV_URI", "", optSingle("value", optPrimitive("STRING", "C_STRING", "<list>http://example.org/scan.png</list>"))))
	cases := []struct {
		name  string
		attrs []string
		check func(t *testing.T, mm *rm.DVMultimedia)
	}{
		{
			name:  "image/png",
			attrs: []string{png},
			check: func(t *testing.T, mm *rm.DVMultimedia) { t.Helper(); checkURIOnly(t, mm, "http://example.com") },
		},
		{
			name:  "image/png with a uri the OPT constrains",
			attrs: []string{png, scan},
			check: func(t *testing.T, mm *rm.DVMultimedia) {
				t.Helper()
				checkURIOnly(t, mm, "http://example.org/scan.png")
			},
		},
		{
			name:  "image/png with the data prohibited",
			attrs: []string{png, optProhibitedSingle("data")},
			check: func(t *testing.T, mm *rm.DVMultimedia) { t.Helper(); checkURIOnly(t, mm, "http://example.com") },
		},
		{
			name:  "image/png with the uri prohibited",
			attrs: []string{png, optProhibitedSingle("uri")},
			check: checkSizedData,
		},
		{
			name:  "text/html",
			attrs: []string{html},
			check: checkSizedData,
		},
		{
			name:  "image/png with a size range that rejects the fixed size",
			attrs: []string{png, optSingle("size", optPrimitive("INTEGER", "C_INTEGER", instance.IntRange(instance.Closed(1), instance.Closed(5))))},
			check: func(t *testing.T, mm *rm.DVMultimedia) {
				t.Helper()
				if len(mm.Data) != 0 || mm.URI == nil {
					t.Errorf("DV_MULTIMEDIA has data %q and uri %#v, want a uri alone", mm.Data, mm.URI)
				}
				if mm.Size < 1 || mm.Size > 5 {
					t.Errorf("DV_MULTIMEDIA.size = %d, want the walk's size in 1..5", mm.Size)
				}
			},
		},
	}
	for _, tc := range cases {
		for _, place := range multimediaPlacements {
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, place.opt(tc.attrs), implicit)
				for _, opts := range defaultsOptions() {
					t.Run(fmt.Sprintf("%s/%s/implicit=%t/%v/%v", tc.name, place.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, opts)
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						tc.check(t, place.multimedia(t, out))
						checkValid(t, out, c)
					})
				}
			}
		}
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

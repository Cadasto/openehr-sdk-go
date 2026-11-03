package templateprobes

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// probe111WithheldMember is the operational template member the can-fail
// control withholds from the decoder: C_CODE_REFERENCE.referenceSetUri,
// which several corpus templates carry.
const probe111WithheldMember = "referenceSetUri"

// TestProbe111GuardCatchesAWithheldMember is the can-fail control for
// PROBE-111's round-trip leg (REQ-176 § Round trip). On every corpus
// operational template that carries C_CODE_REFERENCE.referenceSetUri, the leg
// runs with a decoder that withholds that member, and it must fail, naming
// the member, through the check that sees it:
//
//   - "read and dropped" stands in for a decoder that reads the element and
//     keeps nothing of it, reporting nothing: only the comparison of the
//     re-encoded document with the input can see the loss, so removing that
//     comparison from probe111RoundTrip turns this subtest red;
//   - "unknown to the decoder" stands in for a decoder whose model lacks the
//     member, which meets the element under a name it does not know: the
//     unmatched report fires first, so removing the report check from
//     probe111RoundTrip turns this subtest red (the strict leg would fail it
//     with another detail);
//   - "lost on the second decode" withholds the member only from the decode
//     of the SDK's own re-encoding, which then holds it: only the comparison
//     of the two decoded values can see the loss, so removing that comparison
//     from probe111RoundTrip turns this subtest red.
func TestProbe111GuardCatchesAWithheldMember(t *testing.T) {
	names, err := Probe111OperationalTemplates()
	if err != nil {
		t.Fatalf("Probe111OperationalTemplates: %v", err)
	}
	element := regexp.MustCompile(`<` + probe111WithheldMember + `>[^<]*</` + probe111WithheldMember + `>`)
	doubles := []struct {
		name   string
		decode func(body []byte) probe111Decoder
		want   *regexp.Regexp
	}{
		{
			name: "read and dropped",
			decode: func([]byte) probe111Decoder {
				return func(data []byte, v any, opts ...canxml.DecoderOption) error {
					return canxml.Unmarshal(element.ReplaceAll(data, nil), v, opts...)
				}
			},
			want: regexp.MustCompile(`^the re-encoded document differs from the input at \S*/` + probe111WithheldMember + `\[\d+\]: `),
		},
		{
			name: "unknown to the decoder",
			decode: func([]byte) probe111Decoder {
				return func(data []byte, v any, opts ...canxml.DecoderOption) error {
					renamed := bytes.ReplaceAll(data, []byte("<"+probe111WithheldMember+">"), []byte("<"+probe111WithheldMember+"Withheld>"))
					renamed = bytes.ReplaceAll(renamed, []byte("</"+probe111WithheldMember+">"), []byte("</"+probe111WithheldMember+"Withheld>"))
					return canxml.Unmarshal(renamed, v, opts...)
				}
			},
			want: regexp.MustCompile(`^decode: \d+ unmatched items, the first at \S*/` + probe111WithheldMember + `Withheld\[\d+\]$`),
		},
		{
			name: "lost on the second decode",
			decode: func(body []byte) probe111Decoder {
				return func(data []byte, v any, opts ...canxml.DecoderOption) error {
					if !bytes.Equal(data, body) {
						data = element.ReplaceAll(data, nil)
					}
					return canxml.Unmarshal(data, v, opts...)
				}
			},
			want: regexp.MustCompile(`^the decode of the re-encoded document differs from the first decode: .*` + probe111WithheldMember),
		},
	}
	carriers := 0
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !element.Match(body) {
			continue
		}
		carriers++
		for _, d := range doubles {
			t.Run(d.name+"/"+name, func(t *testing.T) {
				t.Parallel()
				r, err := probe111RoundTrip(body, d.decode(body))
				if err != nil {
					t.Fatalf("probe111RoundTrip: framework error %v", err)
				}
				if r.Status != "fail" {
					t.Fatalf("status = %q, want fail: the decoder withholds %s, which %s carries", r.Status, probe111WithheldMember, name)
				}
				if !d.want.MatchString(r.Detail) {
					t.Errorf("detail = %q, want it to match %s", r.Detail, d.want)
				}
			})
		}
	}
	if carriers == 0 {
		t.Fatalf("no corpus operational template carries %s: the control proves nothing", probe111WithheldMember)
	}
}

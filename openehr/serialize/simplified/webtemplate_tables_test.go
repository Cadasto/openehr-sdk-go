package simplified_test

// REQ-053 — the codecs keep what they derive from a Web Template for later
// calls with it, so the calls that share one Web Template, on any goroutine,
// read the same tables.

import (
	"os"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// sharedRoundTrip is what one body gives in one round trip: the FLAT and
// STRUCTURED encodings of its FLAT decode, and the FLAT encoding of the
// STRUCTURED decode of those, or the first error met.
type sharedRoundTrip struct {
	flat, structured, again, err string
}

// roundTripShared decodes raw with wt, the compiled template c where it is
// not nil, and encodes and decodes the result in both formats.
func roundTripShared(raw []byte, wt *webtemplate.WebTemplate, c *templatecompile.Compiled) sharedRoundTrip {
	var opts []simplified.Option
	if c != nil {
		opts = append(opts, simplified.WithTemplate(c))
	}
	comp, err := simplified.UnmarshalFlat(raw, wt, opts...)
	if err != nil {
		return sharedRoundTrip{err: "UnmarshalFlat: " + err.Error()}
	}
	flat, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		return sharedRoundTrip{err: "MarshalFlat: " + err.Error()}
	}
	structured, err := simplified.MarshalStructured(comp, wt)
	if err != nil {
		return sharedRoundTrip{err: "MarshalStructured: " + err.Error()}
	}
	back, err := simplified.UnmarshalStructured(structured, wt, opts...)
	if err != nil {
		return sharedRoundTrip{err: "UnmarshalStructured: " + err.Error()}
	}
	again, err := simplified.MarshalFlat(back, wt)
	if err != nil {
		return sharedRoundTrip{err: "MarshalFlat after UnmarshalStructured: " + err.Error()}
	}
	return sharedRoundTrip{flat: string(flat), structured: string(structured), again: string(again)}
}

// TestREQ053CodecsShareAWebTemplateAcrossGoroutines pins that many goroutines
// can decode and encode with one Web Template at once, its first calls
// included, which build what the codecs keep for it: each round trip gives
// what the same round trip gives on its own afterwards. Under the race
// detector it also pins that no call writes to what the calls share.
func TestREQ053CodecsShareAWebTemplateAcrossGoroutines(t *testing.T) {
	t.Parallel()
	opt, err := readOPTFile(fixtures.FlatConformanceOpt())
	if err != nil {
		t.Fatalf("parse corpus OPT: %v", err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile corpus OPT: %v", err)
	}
	// A Web Template no call has used yet, so the goroutines below are the
	// ones that build what the codecs keep for it.
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatalf("build corpus Web Template: %v", err)
	}
	names, err := fixtures.ListFlatConformance()
	if err != nil {
		t.Fatalf("list FLAT conformance corpus: %v", err)
	}
	bodies := make([][]byte, len(names))
	for i, name := range names {
		if bodies[i], err = os.ReadFile(fixtures.FlatConformanceFlat(name)); err != nil {
			t.Fatalf("read FLAT body %s: %v", name, err)
		}
	}
	// Half the workers decode with the compiled template and half without,
	// so the calls of both modes run at once.
	modes := []*templatecompile.Compiled{compiled, nil}
	const workers = 8
	got := make([][]sharedRoundTrip, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			<-start
			c := modes[w%len(modes)]
			out := make([]sharedRoundTrip, len(bodies))
			for i, raw := range bodies {
				out[i] = roundTripShared(raw, wt, c)
			}
			got[w] = out
		})
	}
	close(start)
	wg.Wait()

	for m, c := range modes {
		for i, raw := range bodies {
			want := roundTripShared(raw, wt, c)
			if m == 0 && want.err != "" {
				t.Errorf("%s with the compiled template: %s", names[i], want.err)
			}
			for w := m; w < workers; w += len(modes) {
				if got[w][i] != want {
					t.Errorf("%s, worker %d of %d at once: round trip differs from the same round trip on its own:\n got %+v\nwant %+v",
						names[i], w, workers, got[w][i], want)
				}
			}
		}
	}
}

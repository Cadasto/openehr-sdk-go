package instance_test

// concurrent_generate_test.go: Generate called from several goroutines at
// once on one compiled template. The generator keeps what it resolves for
// each node on the compiled node the first time it asks, so the callers
// share that template's nodes while they fill them. Run it under the race
// detector.

import (
	"bytes"
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// compileTemplateID reads and compiles the corpus template id.
func compileTemplateID(t *testing.T, id string) *templatecompile.Compiled {
	t.Helper()
	opt, err := readOPTFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile %s: %v", id, err)
	}
	return c
}

// generateLargeJSON generates an instance of c under Large, with a fixed
// clock and uids from a counter of the call's own, and returns its
// canonical JSON.
func generateLargeJSON(ctx context.Context, c *templatecompile.Compiled) ([]byte, error) {
	uid := 0
	out, err := instance.Generate(ctx, c, instance.Options{
		Policy:    instance.Large,
		Territory: "NL",
		Composer:  testComposer(),
		Now:       time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		UIDSource: func() *rm.HierObjectID {
			uid++
			return &rm.HierObjectID{Value: "uid-" + strconv.Itoa(uid)}
		},
	})
	if err != nil {
		return nil, err
	}
	return canjson.Marshal(out)
}

// TestGenerateConcurrentCallersShareOneCompiledTemplate: callers that run
// Generate at once on one compiled template, which no earlier call has
// generated from, each get the instance a lone call gives on a template
// compiled apart.
func TestGenerateConcurrentCallersShareOneCompiledTemplate(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"Demonstration.v1", "vital_signs"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			want, err := generateLargeJSON(t.Context(), compileTemplateID(t, id))
			if err != nil {
				t.Fatalf("Generate %s alone: %v", id, err)
			}
			shared := compileTemplateID(t, id)
			const callers = 8
			got := make([][]byte, callers)
			errs := make([]error, callers)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range callers {
				wg.Go(func() {
					<-start
					got[i], errs[i] = generateLargeJSON(t.Context(), shared)
				})
			}
			close(start)
			wg.Wait()
			for i := range callers {
				if errs[i] != nil {
					t.Errorf("caller %d: Generate %s: %v", i, id, errs[i])
					continue
				}
				if !bytes.Equal(got[i], want) {
					t.Errorf("caller %d: Generate %s differs from a lone call: %s", i, id,
						firstJSONDifference(decodeJSON(t, got[i]), decodeJSON(t, want), ""))
				}
			}
		})
	}
}

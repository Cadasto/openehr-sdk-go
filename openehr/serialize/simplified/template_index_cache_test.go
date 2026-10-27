package simplified

import (
	"runtime"
	"testing"
	"weak"

	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// TestTemplateIndexBuiltOncePerTemplate pins that decodes sharing a compiled
// template share its index (REQ-053: WithTemplate reads the names, RM types
// and empty structures from it), and that each template gets its own. Build
// the index per call again and the repeat call returns a fresh one; key the
// cache by anything coarser than the template and the second template gets
// the first one's.
func TestTemplateIndexBuiltOncePerTemplate(t *testing.T) {
	t.Parallel()
	identity := func(opt string) string { return opt }
	first, second := compileValues(t, identity), compileValues(t, identity)
	x := templateIndexFor(first)
	if again := templateIndexFor(first); again != x {
		t.Errorf("templateIndexFor(first) twice = %p, then %p: want one index per template", x, again)
	}
	if other := templateIndexFor(second); other == x {
		t.Errorf("templateIndexFor(second) = %p, the index of first: want each template its own", other)
	}
	if len(x.names) == 0 {
		t.Errorf("templateIndexFor(first).names is empty: want the index buildTemplateIndex builds")
	}
}

// TestTemplateIndexDroppedWithItsTemplate pins that the cache keeps no
// template alive and keeps no entry for a template once it is collected, so a
// process that compiles templates over and over does not grow it. Key the
// cache by the template's pointer and the template is never collected; leave
// out the cleanup and its entry stays.
func TestTemplateIndexDroppedWithItsTemplate(t *testing.T) {
	// The template is reachable only inside this function, so once it returns
	// the cache's weak key is the only way left to it.
	key := func() weak.Pointer[templatecompile.Compiled] {
		compiled := compileValues(t, func(opt string) string { return opt })
		templateIndexFor(compiled)
		return weak.Make(compiled)
	}()
	if _, ok := templateIndexes.Load(key); !ok {
		t.Fatal("templateIndexes holds no entry for a template templateIndexFor just indexed")
	}
	// A cleanup runs on its own goroutine after the collection that frees
	// the template, so the entry goes some time after one.
	for range 100 {
		runtime.GC()
		if _, ok := templateIndexes.Load(key); !ok {
			if key.Value() != nil {
				t.Fatal("templateIndexes dropped the entry of a template that is still alive")
			}
			return
		}
		runtime.Gosched()
	}
	if key.Value() != nil {
		t.Fatal("the template was not collected after 100 collections: something keeps it alive, the cache's entry included")
	}
	t.Fatal("templateIndexes kept the entry of a template that was collected")
}

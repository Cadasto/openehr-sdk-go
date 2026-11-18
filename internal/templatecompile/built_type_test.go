package templatecompile_test

// built_type_test.go: BuiltType keeps the first answer its resolver gives
// for a node, so the resolver runs once per node however often, and from
// however many goroutines, a caller asks.

import (
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
)

// compiledNodes returns every node of c, the root first, each before its
// children.
func compiledNodes(c *templatecompile.Compiled) []*templatecompile.CompiledNode {
	var out []*templatecompile.CompiledNode
	var walk func(n *templatecompile.CompiledNode)
	walk = func(n *templatecompile.CompiledNode) {
		out = append(out, n)
		for _, a := range templatecompile.NodeAttributes(n) {
			for _, child := range templatecompile.ChildNodes(a) {
				walk(child)
			}
		}
	}
	walk(c.Root())
	return out
}

// countingResolver resolves a node as the instance generator does, from
// its parent's answer, and counts its runs per node. Each run's answer
// carries the run's sequence number, so two runs for one node give two
// answers.
type countingResolver struct {
	seq  atomic.Int64
	mu   sync.Mutex
	runs map[*templatecompile.CompiledNode]int
}

func newCountingResolver() *countingResolver {
	return &countingResolver{runs: map[*templatecompile.CompiledNode]int{}}
}

func (r *countingResolver) resolve(n *templatecompile.CompiledNode) (rmType, class string) {
	r.mu.Lock()
	r.runs[n]++
	r.mu.Unlock()
	parent := ""
	if p := n.Parent(); p != nil {
		parent, _ = templatecompile.BuiltType(p, r.resolve)
	}
	class = n.RMTypeName()
	return parent + "/" + class + "#" + strconv.FormatInt(r.seq.Add(1), 10), class
}

func (r *countingResolver) runsOf(n *templatecompile.CompiledNode) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runs[n]
}

// TestBuiltTypeResolvesOncePerNode: the first BuiltType call for a node
// runs the resolver, which asks for the parent's answer first, and every
// later call returns the answer kept, without running a resolver again.
func TestBuiltTypeResolvesOncePerNode(t *testing.T) {
	t.Parallel()
	c, err := templatecompile.Compile(nested(3))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	nodes := compiledNodes(c)
	if len(nodes) != 4 {
		t.Fatalf("compiled nodes = %d, want 4 (a COMPOSITION over three CLUSTERs)", len(nodes))
	}
	r := newCountingResolver()
	// The deepest node first: its resolver resolves every ancestor.
	first := map[*templatecompile.CompiledNode][2]string{}
	for _, n := range slices.Backward(nodes) {
		rmType, class := templatecompile.BuiltType(n, r.resolve)
		first[n] = [2]string{rmType, class}
		if class != n.RMTypeName() {
			t.Errorf("BuiltType(%s) class = %q, want %q", n.AQLPath(), class, n.RMTypeName())
		}
	}
	unexpected := func(n *templatecompile.CompiledNode) (string, string) {
		t.Errorf("BuiltType(%s) ran a resolver for a node already resolved", n.AQLPath())
		return "", ""
	}
	for _, n := range nodes {
		rmType, class := templatecompile.BuiltType(n, unexpected)
		if got, want := [2]string{rmType, class}, first[n]; got != want {
			t.Errorf("BuiltType(%s) second call = %q, want the first answer %q", n.AQLPath(), got, want)
		}
		if runs := r.runsOf(n); runs != 1 {
			t.Errorf("resolver ran %d times for %s, want 1", runs, n.AQLPath())
		}
	}
}

// TestBuiltTypeConcurrentCallersGetOneAnswer: goroutines that ask BuiltType
// for every node of one compiled template at once, some from the root down
// and some from the leaves up, run the resolver once per node and all get
// the same answer. Run it under the race detector.
func TestBuiltTypeConcurrentCallersGetOneAnswer(t *testing.T) {
	t.Parallel()
	c, err := templatecompile.Compile(nested(6))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	nodes := compiledNodes(c)
	r := newCountingResolver()
	const callers = 8
	answers := make([][]string, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			<-start
			got := make([]string, len(nodes))
			for k := range nodes {
				j := k
				if i%2 == 1 {
					j = len(nodes) - 1 - k
				}
				rmType, class := templatecompile.BuiltType(nodes[j], r.resolve)
				got[j] = rmType + " " + class
			}
			answers[i] = got
		})
	}
	close(start)
	wg.Wait()
	for i := 1; i < callers; i++ {
		for j, n := range nodes {
			if answers[i][j] != answers[0][j] {
				t.Errorf("caller %d: BuiltType(%s) = %q, caller 0 got %q", i, n.AQLPath(), answers[i][j], answers[0][j])
			}
		}
	}
	for _, n := range nodes {
		if runs := r.runsOf(n); runs != 1 {
			t.Errorf("resolver ran %d times for %s, want 1", runs, n.AQLPath())
		}
	}
}

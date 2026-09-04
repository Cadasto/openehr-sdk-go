package lint

// advance_internal_test.go: CutLast vs Cut on a two-newline walk. Public
// span tests only ever feed one newline, where first and last separator
// coincide, so swapping CutLast for Cut in [advance] would stay green.
// Removing CutLast (or the +1) must fail this name.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

func TestAdvancePastTwoNewlines(t *testing.T) {
	t.Parallel()
	const text = "foo\n\nbar"
	got := advance(parse.Position{Line: 1, Col: 1}, text)
	want := parse.Position{Line: 3, Col: 4}
	if got != want {
		t.Errorf("advance({Line:1, Col:1}, %q) = %+v, want %+v — last newline, not first", text, got, want)
	}
}

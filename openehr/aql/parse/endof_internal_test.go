package parse

// endof_internal_test.go: CutLast vs Cut on a token whose text carries two
// raw newlines. [endOf] has no public entry that feeds that shape, so a
// first-newline split would not fail any existing test.

import (
	"testing"

	"github.com/antlr4-go/antlr/v4"
)

// lineTextToken is the subset of antlr.Token that [endOf] reads. Unused
// methods return zero values.
type lineTextToken struct {
	line, col int
	text      string
}

func (t lineTextToken) GetSource() *antlr.TokenSourceCharStreamPair { return nil }
func (t lineTextToken) GetTokenType() int                           { return 0 }
func (t lineTextToken) GetChannel() int                             { return 0 }
func (t lineTextToken) GetStart() int                               { return 0 }
func (t lineTextToken) GetStop() int                                { return 0 }
func (t lineTextToken) GetLine() int                                { return t.line }
func (t lineTextToken) GetColumn() int                              { return t.col }
func (t lineTextToken) GetText() string                             { return t.text }
func (t lineTextToken) SetText(string)                              {}
func (t lineTextToken) GetTokenIndex() int                          { return 0 }
func (t lineTextToken) SetTokenIndex(int)                           {}
func (t lineTextToken) GetTokenSource() antlr.TokenSource           { return nil }
func (t lineTextToken) GetInputStream() antlr.CharStream            { return nil }
func (t lineTextToken) String() string                              { return t.text }

func TestEndOfPastTwoNewlines(t *testing.T) {
	t.Parallel()
	const text = "foo\n\nbar"
	got := endOf(lineTextToken{line: 1, text: text})
	want := Position{Line: 3, Col: 4}
	if got != want {
		t.Errorf("endOf(line=1, %q) = %+v, want %+v — last newline, not first", text, got, want)
	}
}

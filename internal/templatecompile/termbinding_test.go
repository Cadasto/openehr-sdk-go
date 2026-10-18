package templatecompile

// termbinding_test.go: which term-binding locators name a node id. A
// binding names the node when its locator is the id itself, holds the id
// as a bracketed predicate, or ends with a slash and the id; an id that is
// only part of a longer code or name does not count.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

func TestTermBindingMatchesNode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		locator string
		want    bool
	}{
		{locator: "at0004", want: true},
		{locator: "/data[at0001]/events[at0006]/data[at0003]/items[at0004]", want: true},
		{locator: "/data[at0001]/items[at0004]/value", want: true},
		{locator: "/data/at0004", want: true},
		{locator: "at0004.1", want: false},
		{locator: "/data[at0004.1]", want: false},
		{locator: "/data/xat0004", want: false},
		{locator: "/data/at0004/value", want: false},
		{locator: "at00041", want: false},
		{locator: "", want: false},
	}
	const nodeID = "at0004"
	for _, tc := range tests {
		b := template.TermBinding{NodeOrPath: tc.locator}
		if got := termBindingMatchesNode(b, nodeID, "["+nodeID+"]"); got != tc.want {
			t.Errorf("termBindingMatchesNode(%q, %q) = %t, want %t", tc.locator, nodeID, got, tc.want)
		}
	}
}

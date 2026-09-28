package transport

import (
	"net/http"
	"testing"
)

// REQ-059: repeated openehr-item-tag response headers join into the one value
// the typed response metadata carries.
func TestJoinHeaderField(t *testing.T) {
	h := http.Header{}
	h.Add("openehr-item-tag", `key="a",value="1"`)
	h.Add("openehr-item-tag", `key="b",value="2"`)
	got := joinHeaderField(h, "openehr-item-tag")
	want := `key="a",value="1"; key="b",value="2"`
	if got != want {
		t.Fatalf("join = %q, want %q", got, want)
	}
}

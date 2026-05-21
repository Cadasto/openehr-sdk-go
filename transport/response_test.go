package transport

import (
	"net/http"
	"testing"
)

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

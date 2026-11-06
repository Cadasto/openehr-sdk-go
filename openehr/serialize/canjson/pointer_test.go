package canjson_test

// pointer_test.go: REQ-170 and REQ-175 (wire.md § REQ-170 *Report*,
// § REQ-175 *Identification*). The Pointer a report passes writes the JSON
// Pointer of its member when asked, stays what it was after the function
// returns and after later reports, and may be read from several goroutines.

import (
	"slices"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

func TestREQ170_REQ175_PointerStaysValid(t *testing.T) {
	in := []byte(`{"_type":"FOLDER","archetype_node_id":"a","x~/1":0,"folders":[` +
		`{"_type":"FOLDER","archetype_node_id":"b"},` +
		`{"_type":"FOLDER","archetype_node_id":"c","x":1}]}`)
	var (
		kept  []canjson.Pointer
		texts []string
	)
	record := func(p canjson.Pointer) {
		kept = append(kept, p)
		texts = append(texts, p.String())
	}
	if err := canjson.Unmarshal(in, new(rm.Folder), canjson.WithUnmatched(record), canjson.WithMissing(record)); err != nil {
		t.Fatalf("Unmarshal = %v, want nil", err)
	}
	want := []string{"/x~0~11", "/folders/0/name", "/folders/1/x", "/folders/1/name", "/name"}
	if !slices.Equal(texts, want) {
		t.Fatalf("reported %q, want %q", texts, want)
	}
	var wg sync.WaitGroup
	for i, p := range kept {
		if got := p.String(); got != want[i] {
			t.Errorf("pointer %d after the decode = %q, want %q, its text when reported", i, got, want[i])
		}
		if got, err := p.AppendText([]byte("x")); err != nil || string(got) != "x"+want[i] {
			t.Errorf("pointer %d AppendText(%q) = %q, %v, want %q, nil", i, "x", got, err, "x"+want[i])
		}
		for range 4 {
			wg.Go(func() {
				if got := p.String(); got != want[i] {
					t.Errorf("pointer %d read on another goroutine = %q, want %q", i, got, want[i])
				}
			})
		}
	}
	wg.Wait()
}

func TestREQ170_REQ175_ZeroPointerIsTheRoot(t *testing.T) {
	var p canjson.Pointer
	if got := p.String(); got != "" {
		t.Errorf("Pointer{}.String() = %q, want %q", got, "")
	}
	if got, err := p.AppendText([]byte("x")); err != nil || string(got) != "x" {
		t.Errorf("Pointer{}.AppendText(%q) = %q, %v, want %q, nil", "x", got, err, "x")
	}
}

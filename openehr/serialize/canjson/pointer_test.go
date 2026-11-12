package canjson_test

// pointer_test.go: REQ-170 and REQ-175 (wire.md § REQ-170 *Report*,
// § REQ-175 *Identification*). The Pointer a report passes writes the JSON
// Pointer of its member when asked, stays what it was after the function
// returns and after later reports, and may be read from several goroutines.

import (
	"slices"
	"strings"
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

// escapeToken escapes s as one JSON Pointer reference token (RFC 6901).
var escapeToken = strings.NewReplacer("~", "~0", "/", "~1").Replace

// TestREQ175_PointerParentAndToken holds that each Pointer a decode reports
// walks back to the root through Parent, giving at each step its last
// reference token unescaped: a name or a Hash key with "~" and "/" as
// themselves, and an item's position in decimal. The tokens, escaped and
// joined from the root down, give back the pointer's text. The members of
// one object share their parent, so their parents compare equal with ==.
func TestREQ175_PointerParentAndToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		into func() any
		want [][]string // the tokens of each reported pointer, from the root down
	}{
		{
			name: "names and items",
			in: `{"_type":"FOLDER","archetype_node_id":"a","x~/1":0,"folders":[` +
				`{"_type":"FOLDER","archetype_node_id":"b"},` +
				`{"_type":"FOLDER","archetype_node_id":"c","x":1}]}`,
			into: func() any { return new(rm.Folder) },
			want: [][]string{{"x~/1"}, {"folders", "0", "name"}, {"folders", "1", "x"}, {"folders", "1", "name"}, {"name"}},
		},
		{
			name: "a Hash key",
			in: `{"_type":"RESOURCE_DESCRIPTION","lifecycle_state":"published","details":{` +
				`"en/GB~0":{"language":` + mcLanguage + `,"a/b~":1}}}`,
			into: func() any { return new(rm.ResourceDescription) },
			want: [][]string{{"details", "en/GB~0", "a/b~"}, {"details", "en/GB~0", "purpose"}, {"original_author"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var kept []canjson.Pointer
			record := func(p canjson.Pointer) { kept = append(kept, p) }
			if err := canjson.Unmarshal([]byte(tc.in), tc.into(), canjson.WithUnmatched(record), canjson.WithMissing(record)); err != nil {
				t.Fatalf("Unmarshal = %v, want nil", err)
			}
			if len(kept) != len(tc.want) {
				t.Fatalf("reported %d pointers, want %d", len(kept), len(tc.want))
			}
			for i, p := range kept {
				tokens, ended := tokensOf(p)
				if !ended {
					t.Fatalf("pointer %q: Parent never reports false on the way to the root", p)
				}
				if !slices.Equal(tokens, tc.want[i]) {
					t.Errorf("pointer %q: tokens from the root down = %q, want %q", p, tokens, tc.want[i])
				}
				var joined strings.Builder
				for _, token := range tokens {
					joined.WriteString("/" + escapeToken(token))
				}
				if joined.String() != p.String() {
					t.Errorf("pointer %q: escaped tokens joined = %q, want its text", p, joined.String())
				}
			}
			for i := range kept {
				for j := i + 1; j < len(kept); j++ {
					a, b := tc.want[i], tc.want[j]
					if !slices.Equal(a[:len(a)-1], b[:len(b)-1]) {
						continue
					}
					pa, _ := kept[i].Parent()
					pb, _ := kept[j].Parent()
					if pa != pb {
						t.Errorf("parents of the siblings %q and %q differ under ==, want one shared parent", kept[i], kept[j])
					}
				}
			}
		})
	}
}

// tokensOf returns p's reference tokens from the root down, read through
// Parent and Token, and whether the walk ended at the zero Pointer, whose
// Parent reports false. A pointer has fewer tokens than its text has bytes,
// so the walk stops there.
func tokensOf(p canjson.Pointer) ([]string, bool) {
	var tokens []string
	for range len(p.String()) + 1 {
		parent, ok := p.Parent()
		if !ok {
			slices.Reverse(tokens)
			return tokens, p == canjson.Pointer{}
		}
		tokens = append(tokens, p.Token())
		p = parent
	}
	return tokens, false
}

func TestREQ175_ZeroPointerHasNoParent(t *testing.T) {
	var p canjson.Pointer
	if parent, ok := p.Parent(); ok || parent != (canjson.Pointer{}) {
		t.Errorf("Pointer{}.Parent() = %q, %t, want the zero Pointer, false", parent, ok)
	}
	if got := p.Token(); got != "" {
		t.Errorf("Pointer{}.Token() = %q, want %q", got, "")
	}
}

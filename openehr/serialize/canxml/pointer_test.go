package canxml_test

// pointer_test.go: REQ-170 and REQ-175 (wire.md § REQ-170 *Report*,
// § REQ-175 *Identification* and *Sent without a value*). The Path and the
// Pointer a report passes write their text when asked, stay what they were
// after the function returns and after later reports, and may be read from
// several goroutines. A member inside a Hash entry whose key lies after it in
// the entry's value is named under that key.

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// texter is what a reported Path or Pointer offers.
type texter interface {
	String() string
	AppendText(b []byte) ([]byte, error)
}

func TestREQ170_REQ175_XMLPathAndPointerStayValid(t *testing.T) {
	language := func(code string) string {
		return `<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>` + code + `</code_string></language>`
	}
	in := []byte(`<description ` + ucNS + ` x-flag="1">` +
		`<lifecycle_state>published</lifecycle_state>` +
		`<details xsi:type="RESOURCE_DESCRIPTION_ITEM"><use></use><x-note/>` + language("fr") + `</details>` +
		`<details xsi:type="RESOURCE_DESCRIPTION_ITEM">` + language("en/GB") + `<misuse xsi:nil="true"/><purpose>p</purpose></details>` +
		`</description>`)
	var (
		kept  []texter
		texts []string
	)
	record := func(p texter) {
		kept = append(kept, p)
		texts = append(texts, p.String())
	}
	opts := []canxml.DecoderOption{
		canxml.WithUnmatched(func(p canxml.Path) { record(p) }),
		canxml.WithMissing(func(p canxml.Pointer) { record(p) }),
		canxml.WithPresence(func(p canxml.Pointer, _ canxml.Presence) { record(p) }),
	}
	if err := canxml.Unmarshal(in, new(rm.ResourceDescription), opts...); err != nil {
		t.Fatalf("Unmarshal = %v, want nil", err)
	}
	want := []string{
		"/description[1]/@x-flag",
		"/details/fr/use",
		"/description[1]/details[1]/x-note[1]",
		"/details/fr/purpose",
		"/details/en~1GB/misuse",
		"/original_author",
	}
	if !slices.Equal(texts, want) {
		t.Fatalf("reported %q, want %q", texts, want)
	}
	var wg sync.WaitGroup
	for i, p := range kept {
		if got := p.String(); got != want[i] {
			t.Errorf("item %d after the decode = %q, want %q, its text when reported", i, got, want[i])
		}
		if got, err := p.AppendText([]byte("x")); err != nil || string(got) != "x"+want[i] {
			t.Errorf("item %d AppendText(%q) = %q, %v, want %q, nil", i, "x", got, err, "x"+want[i])
		}
		for range 4 {
			wg.Go(func() {
				if got := p.String(); got != want[i] {
					t.Errorf("item %d read on another goroutine = %q, want %q", i, got, want[i])
				}
			})
		}
	}
	wg.Wait()
}

func TestREQ170_REQ175_XMLZeroPathAndPointerAreTheRoot(t *testing.T) {
	for name, p := range map[string]texter{"Path": canxml.Path{}, "Pointer": canxml.Pointer{}} {
		if got := p.String(); got != "" {
			t.Errorf("%s{}.String() = %q, want %q", name, got, "")
		}
		if got, err := p.AppendText([]byte("x")); err != nil || string(got) != "x" {
			t.Errorf("%s{}.AppendText(%q) = %q, %v, want %q, nil", name, "x", got, err, "x")
		}
	}
}

// escapeToken escapes s as one JSON Pointer reference token (RFC 6901).
var escapeToken = strings.NewReplacer("~", "~0", "/", "~1").Replace

// TestREQ175_XMLPointerParentAndToken holds that each Pointer a decode
// reports walks back to the root through Parent, giving at each step its last
// reference token unescaped: a name or a Hash key with "~" and "/" as
// themselves, the key given once the entry's value has been read, and an
// item's position in decimal. The tokens, escaped and joined from the root
// down, give back the pointer's text. The members of one element share their
// parent, so their parents compare equal with ==.
func TestREQ175_XMLPointerParentAndToken(t *testing.T) {
	language := func(code string) string {
		return `<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>` + code + `</code_string></language>`
	}
	cases := []struct {
		name string
		in   string
		into func() any
		want [][]string // the tokens of each reported pointer, from the root down
	}{
		{
			name: "Hash keys after the value",
			in: `<description ` + ucNS + `>` +
				`<lifecycle_state>published</lifecycle_state>` +
				`<details xsi:type="RESOURCE_DESCRIPTION_ITEM"><use></use>` + language("fr") + `</details>` +
				`<details xsi:type="RESOURCE_DESCRIPTION_ITEM">` + language("en/GB~0") + `<misuse xsi:nil="true"/><purpose>p</purpose></details>` +
				`</description>`,
			into: func() any { return new(rm.ResourceDescription) },
			want: [][]string{{"details", "fr", "use"}, {"details", "fr", "purpose"}, {"details", "en/GB~0", "misuse"}, {"original_author"}},
		},
		{
			name: "items",
			in: `<folder ` + ucNS + ` archetype_node_id="a"><name><value>f</value></name>` +
				`<folders/><folders archetype_node_id="c"></folders></folder>`,
			into: func() any { return new(rm.Folder) },
			want: [][]string{{"folders", "0", "name"}, {"folders", "0", "archetype_node_id"}, {"folders", "1", "name"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var kept []canxml.Pointer
			record := func(p canxml.Pointer) { kept = append(kept, p) }
			opts := []canxml.DecoderOption{
				canxml.WithMissing(record),
				canxml.WithPresence(func(p canxml.Pointer, _ canxml.Presence) { record(p) }),
			}
			if err := canxml.Unmarshal([]byte(tc.in), tc.into(), opts...); err != nil {
				t.Fatalf("Unmarshal = %v, want nil", err)
			}
			if len(kept) != len(tc.want) {
				t.Fatalf("reported %q, want %d pointers", kept, len(tc.want))
			}
			for i, p := range kept {
				tokens, ended := xmlTokensOf(p)
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

// xmlTokensOf returns p's reference tokens from the root down, read through
// Parent and Token, and whether the walk ended at the zero Pointer, whose
// Parent reports false. A pointer has fewer tokens than its text has bytes,
// so the walk stops there.
func xmlTokensOf(p canxml.Pointer) ([]string, bool) {
	var tokens []string
	for range len(p.String()) + 1 {
		parent, ok := p.Parent()
		if !ok {
			slices.Reverse(tokens)
			return tokens, p == canxml.Pointer{}
		}
		tokens = append(tokens, p.Token())
		p = parent
	}
	return tokens, false
}

func TestREQ175_XMLZeroPointerHasNoParent(t *testing.T) {
	var p canxml.Pointer
	if parent, ok := p.Parent(); ok || parent != (canxml.Pointer{}) {
		t.Errorf("Pointer{}.Parent() = %q, %t, want the zero Pointer, false", parent, ok)
	}
	if got := p.Token(); got != "" {
		t.Errorf("Pointer{}.Token() = %q, want %q", got, "")
	}
}

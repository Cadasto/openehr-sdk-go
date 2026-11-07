package canxml_test

// pointer_test.go: REQ-170 and REQ-175 (wire.md § REQ-170 *Report*,
// § REQ-175 *Identification* and *Sent without a value*). The Path and the
// Pointer a report passes write their text when asked, stay what they were
// after the function returns and after later reports, and may be read from
// several goroutines. A member inside a Hash entry whose key lies after it in
// the entry's value is named under that key.

import (
	"slices"
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

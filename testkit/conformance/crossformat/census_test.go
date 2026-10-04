package crossformat_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/conformance/crossformat"
	"github.com/cadasto/openehr-sdk-go/testkit/conformance/webtemplate"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

var update = flag.Bool("update", false, "regenerate CENSUS.md from the harness")

// censusFile is the committed census, beside the harness.
const censusFile = "CENSUS.md"

// TestCensus regenerates the PROBE-105 census and fails when the committed
// CENSUS.md differs from it, so the published outcomes and reasons are always
// the ones the harness and recorded.go produce, never a copy kept by hand
// (REQ-080). With -update it writes the file instead:
//
//	go test ./testkit/conformance/crossformat/ -run TestCensus -update
func TestCensus(t *testing.T) {
	got := crossformat.Census(runCorpus(t), crossformat.Recorded)
	if *update {
		if err := os.WriteFile(censusFile, got, 0o644); err != nil {
			t.Fatalf("write %s: %v", censusFile, err)
		}
		return
	}
	want, err := os.ReadFile(censusFile)
	if err != nil {
		t.Fatalf("read %s: %v (generate it with -update)", censusFile, err)
	}
	if err := sameCensus(want, got); err != nil {
		t.Errorf("%s is stale: %v\nregenerate it with: go test ./testkit/conformance/crossformat/ -run TestCensus -update", censusFile, err)
	}
}

// TestCensusHasNoEmDash keeps the generated prose free of em dashes, the
// house style for generated files, whatever a codec error or a reason says.
func TestCensusHasNoEmDash(t *testing.T) {
	got := crossformat.Census(runCorpus(t), crossformat.Recorded)
	if i := bytes.IndexRune(got, '\u2014'); i >= 0 {
		t.Errorf("Census() has an em dash at byte %d: %q", i, got[max(0, i-40):min(len(got), i+40)])
	}
}

// TestProbe105CensusRendering pins what the census shows for a leg over
// synthetic results: sets in name order, at most ten examples per difference
// class, the recorded reason or "no record", a refusal's error shortened and
// its recorded substring, the reducing decode's removals by reason, the same
// wording whichever modal verb encoding/json/v2 picked for the run, and no em
// dash even when a codec error carries one.
func TestProbe105CensusRendering(t *testing.T) {
	var missing []string
	for i := range 12 {
		missing = append(missing, fmt.Sprintf("r/k%02d", i))
	}
	results := []crossformat.SetResult{
		{Set: "zeta", Legs: []crossformat.LegResult{{
			Leg:         crossformat.LegFlatStructured,
			Outcome:     crossformat.Outcome{Compared: 20, Missing: 12, Altered: 1},
			MissingKeys: missing,
			Alterations: []crossformat.Alteration{{Key: "r/x", Reference: `"a"`, Ours: `"b"`}},
			Refusals: []webtemplate.Refusal{
				{Key: "r/a", Reason: "path not in web template", Keys: 2},
				{Key: "r/c", Reason: "unsupported datatype: PARTY_PROXY", Keys: 4},
				{Key: "r/b", Reason: "path not in web template", Keys: 1},
			},
		}}},
		{Set: "alpha", Legs: []crossformat.LegResult{{
			Leg:     crossformat.LegJSONXML,
			Outcome: crossformat.Outcome{Refused: "canonical XML decode: unable to parse \"" + strings.Repeat("A", 200) + "\" \u2014 invalid"},
		}}},
	}
	table := map[string]map[crossformat.Leg]crossformat.Record{
		"alpha": {crossformat.LegJSONXML: {Outcome: crossformat.Outcome{Refused: "parsing"}, Reason: "a recorded reason"}},
	}
	got := string(crossformat.Census(results, table))

	if a, z := strings.Index(got, "## alpha"), strings.Index(got, "## zeta"); a < 0 || z < 0 || a > z {
		t.Errorf("Census() does not list the sets in name order:\n%s", got)
	}
	for _, want := range []string{
		"| alpha | json-xml | refused |",
		"| zeta | flat-structured | compared 20, missing 12, extra 0, altered 1 |",
		"- Reason: a recorded reason",
		"- Recorded as an error containing `parsing`",
		"- Error: `canonical XML decode: cannot parse",
		"- Reason: no record",
		"- Missing (12, first 10 shown):",
		"  - `r/k09`",
		"  - `r/x`: reference `\"a\"`, ours `\"b\"`",
		"- Reducing decode removed 7 keys:\n  - 4: unsupported datatype: PARTY_PROXY\n  - 3: path not in web template\n",
		strings.Repeat("A", 39) + "...",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Census() lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"`r/k10`", strings.Repeat("A", 41), "\u2014", "unable to"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("Census() contains %q:\n%s", unwanted, got)
		}
	}
}

// TestSameCensus pins the staleness check TestCensus relies on: equal bytes
// pass, and any difference fails naming the first line that differs.
func TestSameCensus(t *testing.T) {
	tests := []struct {
		name       string
		want, got  string
		wantErr    bool
		errMention string
	}{
		{name: "equal", want: "a\nb\n", got: "a\nb\n"},
		{name: "changed line", want: "a\nb\n", got: "a\nc\n", wantErr: true, errMention: "line 2"},
		{name: "extra line", want: "a\n", got: "a\nb\n", wantErr: true, errMention: "line 2"},
		{name: "missing trailing newline", want: "a\n", got: "a", wantErr: true, errMention: "line 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sameCensus([]byte(tt.want), []byte(tt.got))
			if (err != nil) != tt.wantErr {
				t.Fatalf("sameCensus(%q, %q) = %v, wantErr %v", tt.want, tt.got, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tt.errMention) {
				t.Errorf("sameCensus(%q, %q) = %q, want it to mention %q", tt.want, tt.got, err, tt.errMention)
			}
		})
	}
}

// sameCensus reports the first line where the committed census want and the
// generated census got differ, or nil when they are byte-identical.
func sameCensus(want, got []byte) error {
	if bytes.Equal(want, got) {
		return nil
	}
	wl, gl := strings.SplitAfter(string(want), "\n"), strings.SplitAfter(string(got), "\n")
	for i := range max(len(wl), len(gl)) {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Errorf("line %d: committed %q, generated %q", i+1, w, g)
		}
	}
	return fmt.Errorf("the files differ (%d and %d bytes)", len(want), len(got))
}

// runCorpus runs every set of the cross-format corpus.
func runCorpus(t *testing.T) []crossformat.SetResult {
	t.Helper()
	sets, err := fixtures.ListCrossFormatSets()
	if err != nil {
		t.Fatalf("ListCrossFormatSets() error = %v", err)
	}
	if len(sets) == 0 {
		t.Fatal("ListCrossFormatSets() returned no sets; the census would assert nothing")
	}
	results := make([]crossformat.SetResult, 0, len(sets))
	for _, set := range sets {
		res, err := crossformat.Run(set)
		if err != nil {
			t.Fatalf("Run(%s) error = %v", set.Name, err)
		}
		results = append(results, res)
	}
	return results
}

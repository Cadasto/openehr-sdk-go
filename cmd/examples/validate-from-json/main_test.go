package main

import (
	"slices"
	"testing"
)

// -cassette must keep setting the same switch as -corpus: deleting that
// alias fails the "deprecated -cassette spelling" case.
func TestParseFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCorpus bool
		wantRest   []string
	}{
		{name: "no flags", args: nil, wantCorpus: false},
		{name: "-corpus", args: []string{"-corpus"}, wantCorpus: true},
		{name: "deprecated -cassette spelling", args: []string{"-cassette"}, wantCorpus: true},
		{name: "own files", args: []string{"comp.json", "tmpl.opt"}, wantRest: []string{"comp.json", "tmpl.opt"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotCorpus, gotRest := parseFlags(tc.args)
			if gotCorpus != tc.wantCorpus || !slices.Equal(gotRest, tc.wantRest) {
				t.Errorf("parseFlags(%q) = %v, %q; want %v, %q", tc.args, gotCorpus, gotRest, tc.wantCorpus, tc.wantRest)
			}
		})
	}
}

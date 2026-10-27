package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
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

// A DV_QUANTITY with no "magnitude" member decodes to a magnitude of 0, which
// the decoded composition cannot tell apart from a magnitude the document
// gave as 0. Decoding first and validating the value afterwards passes the
// composition; validating the bytes reports the member as required.
func TestREQ175_ValidateFilesReportsMissingMagnitude(t *testing.T) { // REQ-112
	src, err := defaultCompositionPath()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("parse %s: %v", src, err)
	}
	if n := dropMagnitude(doc); n != 1 {
		t.Fatalf("%s holds %d DV_QUANTITY objects with a magnitude, want 1 (the systolic reading)", src, n)
	}
	edited, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(t.TempDir(), "no_magnitude.json")
	if err := os.WriteFile(jsonPath, edited, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := validateFiles(jsonPath, fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("validateFiles(%s, vital_signs.opt) error = %v, want nil", jsonPath, err)
	}
	if result.OK {
		t.Errorf("validateFiles(%s, vital_signs.opt).OK = true, want false", jsonPath)
	}
	if !slices.ContainsFunc(result.Issues, func(i validation.Issue) bool {
		return i.Code == "required" && strings.HasSuffix(i.Path, "/magnitude")
	}) {
		t.Errorf("validateFiles(%s, vital_signs.opt) issues = %+v, want a required issue at a path ending in /magnitude", jsonPath, result.Issues)
	}
}

// dropMagnitude removes the "magnitude" member of every DV_QUANTITY object in
// a document parsed into generic values, and returns how many it removed.
func dropMagnitude(v any) int {
	n := 0
	switch node := v.(type) {
	case map[string]any:
		if _, ok := node["magnitude"]; ok && node["_type"] == "DV_QUANTITY" {
			delete(node, "magnitude")
			n++
		}
		for _, child := range node {
			n += dropMagnitude(child)
		}
	case []any:
		for _, item := range node {
			n += dropMagnitude(item)
		}
	}
	return n
}

// decoded must read a decode failure, and only that, as "no composition to
// describe": removing the path or the code test makes a row below fail.
func TestDecoded(t *testing.T) {
	tests := []struct {
		name   string
		issues []validation.Issue
		want   bool
	}{
		{name: "no issues", want: true},
		{name: "a required member", issues: []validation.Issue{{Path: "/content/0/value/magnitude", Code: "required"}}, want: true},
		{name: "a shape failure below the root", issues: []validation.Issue{{Path: "/content/0", Code: "invalid_shape"}}, want: true},
		{name: "the decode failure at the root", issues: []validation.Issue{{Path: "/", Code: "invalid_shape"}}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := decoded(validation.Result{Issues: tc.issues}); got != tc.want {
				t.Errorf("decoded(%+v) = %v, want %v", tc.issues, got, tc.want)
			}
		})
	}
}

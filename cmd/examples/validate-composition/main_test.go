package main

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// vitalSignsComposition says it matches the default fixture of the
// validate-from-json example. This holds the two to the same JSON value, so a
// change to either one alone fails here. The member order is not compared:
// the fixture sorts the members of every object, and canjson does not.
func TestVitalSignsCompositionMatchesValidateFromJSONFixture(t *testing.T) {
	comp, err := vitalSignsComposition()
	if err != nil {
		t.Fatalf("vitalSignsComposition() error = %v", err)
	}
	built, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal(vitalSignsComposition()) error = %v", err)
	}
	path := filepath.Join("..", "validate-from-json", "testdata", "minimal_blood_pressure.json")
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	got, want := sortedJSON(t, built), sortedJSON(t, committed)
	if got != want {
		t.Errorf("vitalSignsComposition() as canonical JSON is not the composition in %s; "+
			"change both, or neither.\n%s", path, firstDifference(got, want))
	}
}

// sortedJSON re-encodes doc with the members of every object sorted by name,
// so two documents that differ only in member order compare equal.
func sortedJSON(t *testing.T, doc []byte) string {
	t.Helper()

	var v any
	if err := jsonv2.Unmarshal(doc, &v); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	b, err := jsonv2.Marshal(v, jsonv2.Deterministic(true))
	if err != nil {
		t.Fatalf("re-encode JSON: %v", err)
	}
	return string(b)
}

// firstDifference shows where two documents first differ, with some context
// either side, rather than dumping both.
func firstDifference(got, want string) string {
	at := 0
	for at < min(len(got), len(want)) && got[at] == want[at] {
		at++
	}
	window := func(s string) string {
		return s[max(at-60, 0):min(at+60, len(s))]
	}
	return fmt.Sprintf("first difference at byte %d of the sorted documents\n"+
		"  built:     …%s…\n  fixture:   …%s…", at, window(got), window(want))
}

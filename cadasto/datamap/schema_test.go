package datamap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// REQ-058 — Schema(opt) must reproduce the dmv2 schema fixtures. Comparison is
// structural (parsed JSON), not byte-exact: JSON object key order is not
// significant.

func TestSchemaAgainstFixtures(t *testing.T) {
	for _, name := range []string{"development-1", "development-2", "development-3"} {
		t.Run(name, func(t *testing.T) {
			if name != "development-1" {
				// development-2 (ADMIN_ENTRY) and development-3 (INSTRUCTION) use a
				// label-only content key and, for INSTRUCTION, an `activities`
				// structure the walk does not yet handle. OBSERVATION/COMPOSITION
				// (the Lab24 result/order shape) is covered by development-1.
				// TODO(REQ-058): port ADMIN_ENTRY label-key + INSTRUCTION.activities.
				t.Skip("entry-type not yet ported (ADMIN_ENTRY/INSTRUCTION) — follow-up")
			}
			optBytes, err := os.ReadFile("testdata/fixtures/" + name + ".opt")
			if err != nil {
				t.Fatal(err)
			}
			opt, err := template.ParseOPT(bytes.NewReader(optBytes))
			if err != nil {
				t.Fatalf("ParseOPT: %v", err)
			}
			got := normalizeJSON(t, Schema(opt))

			expBytes, err := os.ReadFile("testdata/fixtures/" + name + ".schema.json")
			if err != nil {
				t.Fatal(err)
			}
			var want any
			if err := json.Unmarshal(expBytes, &want); err != nil {
				t.Fatal(err)
			}

			if diff := diffJSON("$", got, want); diff != "" {
				t.Errorf("schema mismatch (first diff):\n%s", diff)
			}
		})
	}
}

func normalizeJSON(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// diffJSON returns a human-readable description of the first structural
// difference between got and want, or "" when they are equal.
func diffJSON(path string, got, want any) string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: got %T, want object", path, got)
		}
		for _, k := range sortedKeys(w) {
			gv, present := g[k]
			if !present {
				return fmt.Sprintf("%s.%s: missing\n  got keys: %v\n  want %v", path, k, sortedKeys(g), compact(w[k]))
			}
			if d := diffJSON(path+"."+k, gv, w[k]); d != "" {
				return d
			}
		}
		for _, k := range sortedKeys(g) {
			if _, present := w[k]; !present {
				return fmt.Sprintf("%s.%s: unexpected (got %v)", path, k, compact(g[k]))
			}
		}
		return ""
	case []any:
		g, ok := got.([]any)
		if !ok {
			return fmt.Sprintf("%s: got %T, want array", path, got)
		}
		if len(g) != len(w) {
			return fmt.Sprintf("%s: array len got %d want %d", path, len(g), len(w))
		}
		for i := range w {
			if d := diffJSON(fmt.Sprintf("%s[%d]", path, i), g[i], w[i]); d != "" {
				return d
			}
		}
		return ""
	default:
		if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
			return fmt.Sprintf("%s: got %v want %v", path, got, want)
		}
		return ""
	}
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func compact(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 120 {
		return string(b[:120]) + "…"
	}
	return string(b)
}

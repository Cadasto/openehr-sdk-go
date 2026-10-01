package fixtures

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PROBE-030 and PROBE-033 read compositions/ and rm/ and must not enter a
// subdirectory. A nested file in a scratch corpus turns this red if either
// walk starts descending.
func TestFixtureWalksStayInTheTopLevelDirectory(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"compositions/top.json",
		"compositions/nested/hidden.json",
		"rm/top.json",
		"rm/nested/hidden.json",
		"compositions/top.xml",
		"compositions/nested/hidden.xml",
		"rm/top.xml",
		"rm/nested/hidden.xml",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	jsonRels, err := listCompositionJSON(root)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON := map[string]bool{}
	for _, rel := range jsonRels {
		if strings.Count(rel.Rel, "/") != 1 {
			t.Errorf("listCompositionJSON returned %q, a path under a subdirectory", rel.Rel)
		}
		gotJSON[rel.Rel] = true
	}
	for _, want := range []string{"compositions/top.json", "rm/top.json"} {
		if !gotJSON[want] {
			t.Errorf("listCompositionJSON missing %q", want)
		}
	}
	if len(gotJSON) != 2 {
		t.Errorf("listCompositionJSON returned %d paths, want 2", len(gotJSON))
	}

	xmlRels, err := listRMXML(root)
	if err != nil {
		t.Fatal(err)
	}
	gotXML := map[string]bool{}
	for _, rel := range xmlRels {
		if strings.Count(rel, "/") != 1 {
			t.Errorf("listRMXML returned %q, a path under a subdirectory", rel)
		}
		gotXML[rel] = true
	}
	for _, want := range []string{"compositions/top.xml", "rm/top.xml"} {
		if !gotXML[want] {
			t.Errorf("listRMXML missing %q", want)
		}
	}
	if len(gotXML) != 2 {
		t.Errorf("listRMXML returned %d paths, want 2", len(gotXML))
	}
}

package bmmgen

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scripts/sync-bmm.sh verify, which `make bmm-check` runs in `make ci`, holds
// the generator's inputs under resources/bmm/ to the sha256 lines of their
// MANIFEST.txt (REQ-041). The first test runs it on the vendored pins; the
// second runs it on scratch trees that each break the pin one way, and the
// check must name that way.

func TestSyncBMMVerifyVendoredPins(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	script := filepath.Join("..", "..", "scripts", "sync-bmm.sh")
	out, err := exec.CommandContext(t.Context(), bash, script, "verify").CombinedOutput()
	if err != nil {
		t.Fatalf("sync-bmm.sh verify on resources/bmm: %v (never hand-edit a pin; run make bmm-sync)\n%s", err, out)
	}
}

func TestSyncBMMVerifyNamesTheBrokenPin(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "sync-bmm.sh"))
	if err != nil {
		t.Fatal(err)
	}
	// The pinned file names, from the vendored manifest: the script checks
	// that each name it pins has a hash line, so the scratch tree uses them.
	manifest, err := os.ReadFile(filepath.Join("..", "..", "resources", "bmm", "MANIFEST.txt"))
	if err != nil {
		t.Fatal(err)
	}
	_, block, ok := strings.Cut(string(manifest), "# sha256  filename\n")
	if !ok {
		t.Fatal("resources/bmm/MANIFEST.txt has no `# sha256  filename` block")
	}
	var names []string
	for line := range strings.Lines(block) {
		if _, name, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "  "); ok {
			names = append(names, name)
		}
	}
	if len(names) < 2 {
		t.Fatalf("resources/bmm/MANIFEST.txt pins %d files, want at least 2", len(names))
	}
	first := names[0]

	content := func(name string) string { return `{"bmm_version": "2.4", "schema_name": "` + name + `"}` + "\n" }
	hashLine := func(name, data string) string {
		sum := sha256.Sum256([]byte(data))
		return hex.EncodeToString(sum[:]) + "  " + name + "\n"
	}
	// files returns the scratch resources/bmm/, by file name; skip is a
	// pinned name left without its hash line.
	files := func(skip string) map[string]string {
		m := map[string]string{}
		var hashes strings.Builder
		for _, name := range names {
			m[name] = content(name)
			if name != skip {
				hashes.WriteString(hashLine(name, m[name]))
			}
		}
		m["MANIFEST.txt"] = "# scratch\ncommit: 0123456789abcdef0123456789abcdef01234567\n#\n# sha256  filename\n" + hashes.String()
		return m
	}

	cases := []struct {
		name string
		tree map[string]string
		// change edits tree; a name set to "" is left out.
		change map[string]string
		// want is what the refusal must name; empty wants the check to pass.
		want string
	}{
		{name: "intact pin", tree: files("")},
		{
			name: "edited schema", tree: files(""),
			change: map[string]string{first: content(first) + " "},
			want:   "FAILED: " + first + " (sha256 mismatch)",
		},
		{
			name: "missing schema", tree: files(""),
			change: map[string]string{first: ""},
			want:   "MISSING: " + first,
		},
		{
			name: "pinned schema with no hash line", tree: files(first),
			want: "NOT PINNED: " + first,
		},
		{
			name: "schema the manifest does not list", tree: files(""),
			change: map[string]string{"openehr_rm_9.9.9.bmm.json": content("openehr_rm_9.9.9.bmm.json")},
			want:   "UNTRACKED: openehr_rm_9.9.9.bmm.json",
		},
		{
			name: "manifest without its hash block", tree: files(""),
			change: map[string]string{"MANIFEST.txt": "# scratch\ncommit: 0123456789abcdef0123456789abcdef01234567\n"},
			want:   "manifest has no hash block",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			write := func(rel, data string) {
				t.Helper()
				path := filepath.Join(root, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write("scripts/sync-bmm.sh", string(script))
			for name, data := range tc.tree {
				if v, changed := tc.change[name]; changed {
					data = v
				}
				if data != "" {
					write("resources/bmm/"+name, data)
				}
			}
			for name, data := range tc.change {
				if _, inTree := tc.tree[name]; !inTree && data != "" {
					write("resources/bmm/"+name, data)
				}
			}
			cmd := exec.CommandContext(t.Context(), bash, filepath.Join(root, "scripts", "sync-bmm.sh"), "verify")
			raw, err := cmd.CombinedOutput()
			out := string(raw)
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("sync-bmm.sh verify = %v; want success\n%s", err, out)
			case tc.want != "" && err == nil:
				t.Fatalf("sync-bmm.sh verify succeeded; want a refusal naming %q\n%s", tc.want, out)
			case tc.want != "" && !strings.Contains(out, tc.want):
				t.Errorf("sync-bmm.sh verify refused with %q; want it to name %q", out, tc.want)
			}
		})
	}
}

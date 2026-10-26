package bmmgen

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The generator reads the BMM schemas under resources/bmm/ (REQ-041) and
// writes canonical XML code that follows the ITS-XML schemas under
// resources/its-xml/. Each directory is held to the sha256 lines of its
// MANIFEST.txt by its sync script's verify subcommand, which `make ci` runs
// (bmm-check, its-xml-check). The first test runs each on the vendored pins;
// the second runs each on scratch trees that break the pin one way, and the
// check must name that way.

// pinScript is a sync script whose verify `make ci` runs over a pinned
// resources/ directory.
type pinScript struct {
	script, dir string
	// extra is a path, relative to dir, of a schema the manifest does not
	// list.
	extra string
	// edited and unhashed are how verify names, by path, a schema whose
	// bytes no longer match its hash line and a schema with no hash line.
	// sync-bmm.sh lists the paths it pins, so it reports the second as not
	// pinned; sync-its-xml-schemas.sh follows the schemas' includes, which
	// it cannot do offline, so it reports any file without a hash line as
	// untracked.
	edited, unhashed string
}

var pinScripts = []pinScript{
	{
		script: "sync-bmm.sh", dir: "resources/bmm", extra: "openehr_rm_9.9.9.bmm.json",
		edited: "FAILED: %s (sha256 mismatch)", unhashed: "NOT PINNED: %s",
	},
	{
		script: "sync-its-xml-schemas.sh", dir: "resources/its-xml", extra: "components/RM/Release-9.9.9/Extra.xsd",
		edited: "MODIFIED: %s", unhashed: "UNTRACKED: %s",
	},
}

func TestPinScriptsVerifyVendoredPins(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	for _, ps := range pinScripts {
		t.Run(ps.script, func(t *testing.T) {
			t.Parallel()
			script := filepath.Join("..", "..", "scripts", ps.script)
			out, err := exec.CommandContext(t.Context(), bash, script, "verify").CombinedOutput()
			if err != nil {
				t.Fatalf("%s verify on %s: %v (never hand-edit a pin; re-run its sync)\n%s", ps.script, ps.dir, err, out)
			}
		})
	}
}

func TestPinScriptsVerifyNamesTheBrokenPin(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	for _, ps := range pinScripts {
		t.Run(ps.script, func(t *testing.T) {
			t.Parallel()
			testVerifyNamesTheBrokenPin(t, bash, ps)
		})
	}
}

// testVerifyNamesTheBrokenPin runs ps's verify over scratch copies of its
// directory whose files carry the vendored manifest's paths, each copy
// broken one way.
func testVerifyNamesTheBrokenPin(t *testing.T, bash string, ps pinScript) {
	t.Helper()
	scriptName, dir, extra := ps.script, ps.dir, ps.extra
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", scriptName))
	if err != nil {
		t.Fatal(err)
	}
	// The pinned paths and the hash block's header, from the vendored
	// manifest: the script checks that each path it pins has a hash line, so
	// the scratch tree uses them.
	manifest, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(dir), "MANIFEST.txt"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(manifest), "\n# sha256  ")
	if !ok {
		t.Fatalf("%s/MANIFEST.txt has no `# sha256  ` hash block", dir)
	}
	label, block, _ := strings.Cut(rest, "\n")
	header := "# sha256  " + label + "\n"
	var paths []string
	for line := range strings.Lines(block) {
		if _, path, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "  "); ok {
			paths = append(paths, path)
		}
	}
	if len(paths) < 2 {
		t.Fatalf("%s/MANIFEST.txt pins %d files, want at least 2", dir, len(paths))
	}
	last := paths[len(paths)-1]

	content := func(path string) string { return "scratch copy of " + path + "\n" }
	hashLine := func(path, data string) string {
		sum := sha256.Sum256([]byte(data))
		return hex.EncodeToString(sum[:]) + "  " + path + "\n"
	}
	// files returns the scratch directory, by path; skip is a pinned path
	// left without its hash line.
	files := func(skip string) map[string]string {
		m := map[string]string{}
		var hashes strings.Builder
		for _, path := range paths {
			m[path] = content(path)
			if path != skip {
				hashes.WriteString(hashLine(path, m[path]))
			}
		}
		m["MANIFEST.txt"] = "# scratch\ncommit: 0123456789abcdef0123456789abcdef01234567\n#\n" + header + hashes.String()
		return m
	}

	cases := []struct {
		name string
		tree map[string]string
		// change edits tree; a path set to "" is left out.
		change map[string]string
		// want is what the refusal must name; empty wants the check to pass.
		want string
	}{
		{name: "intact pin", tree: files("")},
		{
			name: "edited schema", tree: files(""),
			change: map[string]string{last: content(last) + " "},
			want:   fmt.Sprintf(ps.edited, last),
		},
		{
			name: "missing schema", tree: files(""),
			change: map[string]string{last: ""},
			want:   "MISSING: " + last,
		},
		{
			name: "schema with no hash line", tree: files(last),
			want: fmt.Sprintf(ps.unhashed, last),
		},
		{
			name: "schema the manifest does not list", tree: files(""),
			change: map[string]string{extra: content(extra)},
			want:   "UNTRACKED: " + extra,
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
			write("scripts/"+scriptName, string(script))
			tree := maps.Clone(tc.tree)
			maps.Copy(tree, tc.change)
			for path, data := range tree {
				if data != "" {
					write(dir+"/"+path, data)
				}
			}
			cmd := exec.CommandContext(t.Context(), bash, filepath.Join(root, "scripts", scriptName), "verify")
			raw, err := cmd.CombinedOutput()
			out := string(raw)
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("%s verify = %v; want success\n%s", scriptName, err, out)
			case tc.want != "" && err == nil:
				t.Fatalf("%s verify succeeded; want a refusal naming %q\n%s", scriptName, tc.want, out)
			case tc.want != "" && !strings.Contains(out, tc.want):
				t.Errorf("%s verify refused with %q; want it to name %q", scriptName, out, tc.want)
			}
		})
	}
}

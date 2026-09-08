package termgen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pin writes the fixture and a manifest into a fresh resources directory and
// returns it alongside an output module root, mirroring the two paths
// resources/terminology/ and the repo root have in a real run.
func pin(t *testing.T, manifest string) (resources, outDir string) {
	t.Helper()
	resources = filepath.Join(t.TempDir(), "terminology")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		t.Fatalf("mkdir resources: %v", err)
	}
	if err := os.WriteFile(filepath.Join(resources, "openehr_terminology.xml"), []byte(fixture), 0o644); err != nil {
		t.Fatalf("write pin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(resources, "MANIFEST.txt"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return resources, t.TempDir()
}

const fixtureManifest = "file: openehr_terminology.xml\nref: Release-9.9.9\n"

func TestRunWritesTheTableThenVerifiesItClean(t *testing.T) {
	t.Parallel()
	resources, outDir := pin(t, fixtureManifest)

	res, err := Run(Options{ResourcesDir: resources, OutDir: outDir})
	if err != nil {
		t.Fatalf("Run(write) = _, %v", err)
	}
	if res.Drift || res.Missing {
		t.Errorf("Run(write) = %+v, want no drift flags", res)
	}
	want := filepath.Join(outDir, "openehr", "terminology", "openehr_gen.go")
	if res.Path != want {
		t.Errorf("Run(write).Path = %q, want %q", res.Path, want)
	}
	body, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read the generated table: %v", err)
	}

	// The sha256 in the generated file must be the one the generator computed
	// over the bytes it read — never a value copied from the manifest.
	sum := sha256.Sum256([]byte(fixture))
	if wantConst := `const SourceSHA256 = "` + hex.EncodeToString(sum[:]) + `"`; !bytes.Contains(body, []byte(wantConst)) {
		t.Errorf("generated table does not carry %s", wantConst)
	}
	// The release in the header comes from the manifest's ref: line.
	if !bytes.Contains(body, []byte("openEHR TERM Release-9.9.9")) {
		t.Error("generated header does not name the manifest's ref")
	}

	var stderr bytes.Buffer
	res, err = Run(Options{ResourcesDir: resources, OutDir: outDir, Verify: true, Stderr: &stderr})
	if err != nil {
		t.Fatalf("Run(verify) = _, %v", err)
	}
	if res.Drift || res.Missing {
		t.Errorf("Run(verify) right after Run(write) = %+v, want no drift", res)
	}
	if stderr.Len() != 0 {
		t.Errorf("Run(verify) wrote %q to stderr, want silence on a clean table", stderr.String())
	}
}

func TestRunVerifyReportsDriftWithoutWriting(t *testing.T) {
	t.Parallel()
	resources, outDir := pin(t, fixtureManifest)
	if _, err := Run(Options{ResourcesDir: resources, OutDir: outDir}); err != nil {
		t.Fatalf("Run(write) = _, %v", err)
	}
	out := filepath.Join(outDir, "openehr", "terminology", "openehr_gen.go")
	clean, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read the generated table: %v", err)
	}
	edited := append(bytes.Clone(clean), []byte("\n// a hand edit\n")...)
	if err := os.WriteFile(out, edited, 0o644); err != nil {
		t.Fatalf("hand-edit the generated table: %v", err)
	}

	var stderr bytes.Buffer
	res, err := Run(Options{ResourcesDir: resources, OutDir: outDir, Verify: true, Stderr: &stderr})
	if err != nil {
		t.Fatalf("Run(verify) = _, %v", err)
	}
	if !res.Drift || res.Missing {
		t.Errorf("Run(verify) over a hand-edited table = %+v, want Drift without Missing", res)
	}
	if !strings.Contains(stderr.String(), "DIFFER") || !strings.Contains(stderr.String(), out) {
		t.Errorf("Run(verify) stderr = %q, want it to name DIFFER and %s", stderr.String(), out)
	}
	// Verify must not repair the file — that is `make termgen`'s job.
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("re-read the generated table: %v", err)
	}
	if !bytes.Equal(got, edited) {
		t.Error("Run(verify) rewrote the file; verify must only report")
	}
}

func TestRunVerifyReportsAMissingTable(t *testing.T) {
	t.Parallel()
	resources, outDir := pin(t, fixtureManifest)

	var stderr bytes.Buffer
	res, err := Run(Options{ResourcesDir: resources, OutDir: outDir, Verify: true, Stderr: &stderr})
	if err != nil {
		t.Fatalf("Run(verify) = _, %v", err)
	}
	if !res.Drift || !res.Missing {
		t.Errorf("Run(verify) with no table on disk = %+v, want Drift and Missing", res)
	}
	if !strings.Contains(stderr.String(), "MISSING") {
		t.Errorf("Run(verify) stderr = %q, want it to report MISSING", stderr.String())
	}
	if _, err := os.Stat(res.Path); err == nil {
		t.Error("Run(verify) created the missing table; verify must only report")
	}
}

func TestRunRefusesAManifestWithoutARef(t *testing.T) {
	t.Parallel()
	resources, outDir := pin(t, "file: openehr_terminology.xml\ncommit: deadbeef\n")

	_, err := Run(Options{ResourcesDir: resources, OutDir: outDir})
	if err == nil {
		t.Fatal("Run with a ref-less manifest = _, nil; want a refusal — the header must name the release")
	}
	if !strings.Contains(err.Error(), "MANIFEST.txt") || !strings.Contains(err.Error(), "ref:") {
		t.Errorf("Run error = %q, want it to name MANIFEST.txt and the missing ref: line", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "openehr", "terminology", "openehr_gen.go")); err == nil {
		t.Error("Run wrote a table despite the broken manifest")
	}
}

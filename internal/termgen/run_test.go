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

// pin writes the two fixture files and a manifest into a fresh resources
// directory and returns it alongside an output module root, mirroring the
// two paths resources/terminology/ and the repo root have in a real run.
func pin(t *testing.T, manifest string) (resources, outDir string) {
	t.Helper()
	resources = filepath.Join(t.TempDir(), "terminology")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		t.Fatalf("mkdir resources: %v", err)
	}
	files := map[string]string{
		"openehr_terminology.xml":            fixture,
		"openehr_external_terminologies.xml": externalFixture,
		"MANIFEST.txt":                       manifest,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(resources, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return resources, t.TempDir()
}

const fixtureManifest = "ref: Release-9.9.9\n"

// sha256Hex is the lower-case hex sha256 the generated constants carry.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

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

	// The sha256 of each file in the generated table must be the one the
	// generator computed over the bytes it read — never a value copied from
	// the manifest.
	for _, wantConst := range []string{
		`const SourceSHA256 = "` + sha256Hex(fixture) + `"`,
		`const ExternalSourceSHA256 = "` + sha256Hex(externalFixture) + `"`,
	} {
		if !bytes.Contains(body, []byte(wantConst)) {
			t.Errorf("generated table does not carry %s", wantConst)
		}
	}
	// The external file's code sets are in the table, after the
	// terminology file's own.
	if !bytes.Contains(body, []byte("var codeSets = []*CodeSet{NormalStatuses, Languages, CharacterSets}")) {
		t.Error("generated registry does not list the code sets of both files in source order")
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

// REQ-034: the drift check fails on a hand-edited generated table and leaves
// the file as it found it.
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

// REQ-034: the generated table follows both pinned files, so the drift check
// fails when either one moves under a committed table. Each row changes the
// bytes of one file only (a trailing XML comment keeps it a valid pin), and
// the verdict must be drift.
func TestRunVerifyReportsDriftFromEitherPinnedFile(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"openehr_terminology.xml", "openehr_external_terminologies.xml"} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()
			resources, outDir := pin(t, fixtureManifest)
			if _, err := Run(Options{ResourcesDir: resources, OutDir: outDir}); err != nil {
				t.Fatalf("Run(write) = _, %v", err)
			}
			path := filepath.Join(resources, file)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			if err := os.WriteFile(path, append(body, []byte("\n<!-- a version bump -->\n")...), 0o644); err != nil {
				t.Fatalf("change %s: %v", file, err)
			}

			var stderr bytes.Buffer
			res, err := Run(Options{ResourcesDir: resources, OutDir: outDir, Verify: true, Stderr: &stderr})
			if err != nil {
				t.Fatalf("Run(verify) after changing %s = _, %v", file, err)
			}
			if !res.Drift || res.Missing {
				t.Errorf("Run(verify) after changing %s = %+v, want Drift without Missing", file, res)
			}
			if !strings.Contains(stderr.String(), "DIFFER") {
				t.Errorf("Run(verify) stderr = %q, want it to report DIFFER", stderr.String())
			}
		})
	}
}

// REQ-034: the pin is the two files together; a resources directory missing
// the external one is a broken sync, refused before anything is written.
func TestRunRefusesAPinWithoutTheExternalFile(t *testing.T) {
	t.Parallel()
	resources, outDir := pin(t, fixtureManifest)
	if err := os.Remove(filepath.Join(resources, "openehr_external_terminologies.xml")); err != nil {
		t.Fatalf("remove the external file: %v", err)
	}
	_, err := Run(Options{ResourcesDir: resources, OutDir: outDir})
	if err == nil {
		t.Fatal("Run without openehr_external_terminologies.xml = _, nil; want a refusal")
	}
	if !strings.Contains(err.Error(), "openehr_external_terminologies.xml") {
		t.Errorf("Run error = %q, want it to name the missing file", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "openehr", "terminology", "openehr_gen.go")); err == nil {
		t.Error("Run wrote a table despite the missing file")
	}
}

// REQ-034: a refusal of one file names that file, so the maintainer knows
// which half of the pin to read.
func TestRunNamesTheFileItRefuses(t *testing.T) {
	t.Parallel()
	resources, outDir := pin(t, fixtureManifest)
	broken := strings.Replace(externalFixture, `issuer="ISO" `, ``, 1)
	if err := os.WriteFile(filepath.Join(resources, "openehr_external_terminologies.xml"), []byte(broken), 0o644); err != nil {
		t.Fatalf("write the broken external file: %v", err)
	}
	_, err := Run(Options{ResourcesDir: resources, OutDir: outDir})
	if err == nil {
		t.Fatal("Run over an external file with an issuer-less code set = _, nil; want a refusal")
	}
	if !strings.Contains(err.Error(), "openehr_external_terminologies.xml") || !strings.Contains(err.Error(), "issuer") {
		t.Errorf("Run error = %q, want it to name openehr_external_terminologies.xml and the missing issuer", err)
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
	resources, outDir := pin(t, "source_repo: openEHR/specifications-TERM\ncommit: deadbeef\n")

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

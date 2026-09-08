package termgen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// pinFile is the vendored terminology inside ResourcesDir.
	pinFile = "openehr_terminology.xml"
	// manifestFile is the provenance scripts/sync-terminology.sh writes
	// beside the pin; its `ref:` line names the TERM release.
	manifestFile = "MANIFEST.txt"
	// headerPath is the pin's repo-relative path as recorded in the
	// generated header. It is a constant rather than the ResourcesDir the
	// caller passed, so the committed file stays byte-identical however
	// `-resources` was spelled (or wherever a test stages a copy).
	headerPath = "resources/terminology/" + pinFile
)

// outPath is where the generated table lives, relative to a module root.
var outPath = filepath.Join("openehr", "terminology", "openehr_gen.go")

// Options configures one [Run]. ResourcesDir holds the pin and its manifest;
// OutDir is the module root the generated file is written under; Verify turns
// the run into a read-only drift check; Stderr, when non-nil, receives one
// diagnostic line per drifting file.
type Options struct {
	ResourcesDir string
	OutDir       string
	Verify       bool
	Stderr       io.Writer
}

// Result reports what one [Run] found: the table's path, whether it drifts
// from the pin, and whether it drifts because it is absent. Both flags stay
// false outside verify mode, where the table is simply rewritten.
type Result struct {
	Path    string
	Drift   bool
	Missing bool
}

// Run reads <ResourcesDir>/openehr_terminology.xml and the `ref:` line of
// <ResourcesDir>/MANIFEST.txt, renders openehr_gen.go under
// <OutDir>/openehr/terminology/, and either writes it atomically or — with
// Verify — compares it with the file on disk, reporting Drift / Missing
// without writing anything.
//
// The sha256 the generated file carries is computed here, over the very bytes
// that were parsed: the manifest's own hash is the sync script's integrity
// check (`make terminology-verify`), not an input to the tables.
func Run(opts Options) (Result, error) {
	res := Result{Path: filepath.Join(opts.OutDir, outPath)}

	pin := filepath.Join(opts.ResourcesDir, pinFile)
	data, err := os.ReadFile(pin)
	if err != nil {
		return res, fmt.Errorf("read the pinned terminology: %w", err)
	}
	ref, err := manifestRef(filepath.Join(opts.ResourcesDir, manifestFile))
	if err != nil {
		return res, err
	}
	term, err := Parse(bytes.NewReader(data))
	if err != nil {
		return res, fmt.Errorf("%s: %w", pin, err)
	}
	sum := sha256.Sum256(data)
	body, err := Render(term, SourceInfo{Path: headerPath, Ref: ref, SHA256: hex.EncodeToString(sum[:])})
	if err != nil {
		return res, err
	}

	if !opts.Verify {
		if err := writeAtomic(res.Path, body); err != nil {
			return res, err
		}
		return res, nil
	}

	existing, err := os.ReadFile(res.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		res.Drift, res.Missing = true, true
	case err != nil:
		return res, fmt.Errorf("read the generated table %s: %w", res.Path, err)
	case !bytes.Equal(existing, body):
		res.Drift = true
	}
	if res.Drift && opts.Stderr != nil {
		what := "DIFFER "
		if res.Missing {
			what = "MISSING"
		}
		// A diagnostic line: the verdict is in res, so a failed write to the
		// caller's own writer changes nothing worth reporting.
		_, _ = fmt.Fprintf(opts.Stderr, "%s %s\n", what, res.Path)
	}
	return res, nil
}

// manifestRef returns the TERM release recorded on the manifest's `ref:`
// line. A pin whose provenance is missing is a broken sync, not something
// the generator may guess around: the header has to name the release the
// tables came from.
func manifestRef(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read the terminology manifest: %w", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "ref:")
		if !ok {
			continue
		}
		if ref := strings.TrimSpace(rest); ref != "" {
			return ref, nil
		}
	}
	return "", fmt.Errorf("%s carries no non-empty \"ref:\" line — the pinned release is unknown", path)
}

// writeAtomic writes body to path via a rename from a uniquely named
// temporary file in the same directory, creating the directory if needed. It
// skips the write entirely when path already holds byte-identical bytes,
// which keeps the modification time (and editors) still on a no-op
// regeneration.
//
// The temporary name is unique per call rather than a fixed "<path>.tmp": two
// concurrent generator runs sharing one fixed name could rename each other's
// bytes into place and both report success.
func writeAtomic(path string, body []byte) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, body) {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	name := tmp.Name()
	// Close is checked because these bytes are the generated table: a failed
	// flush would rename a truncated file over a good one.
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close %s: %w", name, err)
	}
	// CreateTemp makes the file 0600; the generated table is source, so it
	// carries the same mode a plain WriteFile would have given it.
	if err := os.Chmod(name, 0o644); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("chmod %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("rename %s -> %s: %w", name, path, err)
	}
	return nil
}

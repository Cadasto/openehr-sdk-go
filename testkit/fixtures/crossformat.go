package fixtures

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// File names inside a cross-format set directory, and of the manifest at the
// root of the cross-format corpus.
const (
	crossFormatManifest      = "MANIFEST.txt"
	crossFormatTemplate      = "template.opt"
	crossFormatCanonicalJSON = "canonical.json"
	crossFormatCanonicalXML  = "canonical.xml"
	crossFormatFlat          = "flat.json"
	crossFormatStructured    = "structured.json"
)

// CrossFormatRoot returns testkit/corpus/crossformat, the pinned upstream
// cross-format corpus: one directory per set, each giving one composition in
// two or more of canonical JSON, canonical XML, FLAT and STRUCTURED. The
// MANIFEST.txt there records every file's upstream repository, commit, path
// and sha256; scripts/ingest-crossformat.sh regenerates it. Vendored
// Apache-2.0; provenance in THIRD_PARTY_LICENSES.md.
func CrossFormatRoot() string {
	return filepath.Join(CorpusRoot(), "crossformat")
}

// CrossFormatSet is one upstream composition given in two or more formats,
// with the operational template of its template. Every path is absolute. A
// format the set does not carry is "".
type CrossFormatSet struct {
	// Name is the set's directory name under [CrossFormatRoot], such as
	// "corona".
	Name string
	// OPT is the operational template the composition instantiates: the set's
	// own template.opt, or an OPT vendored elsewhere under [CorpusRoot] that
	// the manifest points to because it is byte-identical to the upstream one.
	OPT string
	// CanonicalJSON is the composition in canonical JSON (canonical.json).
	CanonicalJSON string
	// CanonicalXML is the composition in canonical XML (canonical.xml).
	CanonicalXML string
	// FLAT is the composition in the simplified FLAT format (flat.json).
	FLAT string
	// STRUCTURED is the composition in the simplified STRUCTURED format
	// (structured.json).
	STRUCTURED string
}

// ListCrossFormatSets returns the sets of the cross-format corpus, sorted by
// Name. It reads them from the corpus's MANIFEST.txt rather than from the
// directory tree, so it reports exactly the files whose provenance is
// recorded. It returns an error when the manifest is absent, lists no files,
// or has a line it cannot place, so a caller can skip rather than silently
// assert nothing.
func ListCrossFormatSets() ([]CrossFormatSet, error) {
	return listCrossFormatSets(CorpusRoot())
}

// listCrossFormatSets is [ListCrossFormatSets] over the corpus rooted at
// corpusRoot, so tests can run it against a scratch corpus.
func listCrossFormatSets(corpusRoot string) ([]CrossFormatSet, error) {
	// PROBE-105 (REQ-080): the manifest, not the directory listing, names the
	// sets, so a file without recorded provenance never reaches the probe.
	m, err := readCrossFormatManifest(filepath.Join(corpusRoot, "crossformat", crossFormatManifest))
	if err != nil {
		return nil, err
	}
	sets := map[string]*CrossFormatSet{}
	for _, f := range m.files {
		s := sets[f.set]
		if s == nil {
			s = &CrossFormatSet{Name: f.set}
			sets[f.set] = s
		}
		path := filepath.Join(corpusRoot, "crossformat", f.set, f.local)
		switch f.local {
		case crossFormatTemplate:
			s.OPT = path
		case crossFormatCanonicalJSON:
			s.CanonicalJSON = path
		case crossFormatCanonicalXML:
			s.CanonicalXML = path
		case crossFormatFlat:
			s.FLAT = path
		case crossFormatStructured:
			s.STRUCTURED = path
		default:
			return nil, fmt.Errorf("cross-format manifest: %s/%s: unknown file name", f.set, f.local)
		}
	}
	for _, p := range m.opts {
		s := sets[p.set]
		if s == nil {
			return nil, fmt.Errorf("cross-format manifest: opt pointer for set %q, which lists no files", p.set)
		}
		if s.OPT != "" {
			return nil, fmt.Errorf("cross-format manifest: set %q has both a %s and an opt pointer", p.set, crossFormatTemplate)
		}
		s.OPT = filepath.Join(corpusRoot, filepath.FromSlash(p.target))
	}
	out := make([]CrossFormatSet, 0, len(sets))
	for _, s := range sets {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b CrossFormatSet) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// crossFormatManifestData is the parsed MANIFEST.txt of the cross-format
// corpus, its records in file order.
type crossFormatManifestData struct {
	sources []crossFormatSource
	files   []crossFormatFile
	opts    []crossFormatOPTPointer
}

// crossFormatSource is a `source` record: an upstream repository and the
// commit the corpus is pinned to.
type crossFormatSource struct {
	name, repo, commit, licence string
}

// crossFormatFile is a `file` record: one vendored file of a set.
type crossFormatFile struct {
	set, local       string // crossformat/{set}/{local}
	source, upstream string // source record name and path within that repository
	sha256           string
}

// crossFormatOPTPointer is an `opt` record: a set whose OPT is vendored
// elsewhere under testkit/corpus/, byte-identical to an upstream OPT.
type crossFormatOPTPointer struct {
	set              string
	target           string // slash-separated, relative to testkit/corpus/
	source, upstream string
	sha256           string
}

// readCrossFormatManifest opens and parses the manifest at path.
func readCrossFormatManifest(path string) (crossFormatManifestData, error) {
	f, err := os.Open(path)
	if err != nil {
		return crossFormatManifestData{}, err
	}
	defer func() { _ = f.Close() }() // read-only
	m, err := parseCrossFormatManifest(f)
	if err != nil {
		return crossFormatManifestData{}, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// parseCrossFormatManifest parses the tab-separated manifest records. Blank
// lines and lines starting with # are comments. Every record must have its
// kind's field count, every file and opt record must name a declared source,
// and no file or set may be recorded twice.
func parseCrossFormatManifest(r io.Reader) (crossFormatManifestData, error) {
	// PROBE-105 (REQ-080): per file, the manifest must yield the source
	// repository, commit, upstream path and sha256, so a record that cannot
	// be resolved to all four is an error, not a skipped line.
	var m crossFormatManifestData
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if err := parseCrossFormatRecord(&m, fields); err != nil {
			return crossFormatManifestData{}, fmt.Errorf("line %d: %w", n, err)
		}
	}
	if err := sc.Err(); err != nil {
		return crossFormatManifestData{}, err
	}
	if err := checkCrossFormatManifest(m); err != nil {
		return crossFormatManifestData{}, err
	}
	return m, nil
}

// parseCrossFormatRecord appends the record in fields to m.
func parseCrossFormatRecord(m *crossFormatManifestData, fields []string) error {
	want := map[string]int{"source": 5, "file": 5, "opt": 6}[fields[0]]
	if want == 0 {
		return fmt.Errorf("unknown record kind %q", fields[0])
	}
	if len(fields) != want || slices.Contains(fields, "") {
		return fmt.Errorf("%s record has %d non-empty fields, want %d", fields[0], countNonEmpty(fields), want)
	}
	switch fields[0] {
	case "source":
		m.sources = append(m.sources, crossFormatSource{name: fields[1], repo: fields[2], commit: fields[3], licence: fields[4]})
	case "file":
		set, local, ok := strings.Cut(fields[1], "/")
		if !ok || !isCrossFormatSetName(set) {
			return fmt.Errorf("file %q is not {set}/{file name}", fields[1])
		}
		if !slices.Contains([]string{crossFormatTemplate, crossFormatCanonicalJSON, crossFormatCanonicalXML, crossFormatFlat, crossFormatStructured}, local) {
			return fmt.Errorf("file %q: unknown file name %q", fields[1], local)
		}
		m.files = append(m.files, crossFormatFile{set: set, local: local, source: fields[2], upstream: fields[3], sha256: fields[4]})
	case "opt":
		if !isCrossFormatSetName(fields[1]) {
			return fmt.Errorf("opt pointer set %q is not a directory name", fields[1])
		}
		if !filepath.IsLocal(filepath.FromSlash(fields[2])) {
			return fmt.Errorf("opt pointer target %q is not a path inside testkit/corpus/", fields[2])
		}
		m.opts = append(m.opts, crossFormatOPTPointer{set: fields[1], target: fields[2], source: fields[3], upstream: fields[4], sha256: fields[5]})
	}
	return nil
}

// checkCrossFormatManifest reports what no single record shows: an
// undeclared or duplicate source, a duplicate file or opt pointer, and a
// manifest that lists no files.
func checkCrossFormatManifest(m crossFormatManifestData) error {
	declared := map[string]bool{}
	var errs []error
	for _, s := range m.sources {
		if declared[s.name] {
			errs = append(errs, fmt.Errorf("source %q declared twice", s.name))
		}
		declared[s.name] = true
	}
	seen := map[string]bool{}
	for _, f := range m.files {
		key := f.set + "/" + f.local
		if seen[key] {
			errs = append(errs, fmt.Errorf("file %s listed twice", key))
		}
		seen[key] = true
		if !declared[f.source] {
			errs = append(errs, fmt.Errorf("file %s names undeclared source %q", key, f.source))
		}
	}
	pointed := map[string]bool{}
	for _, p := range m.opts {
		if pointed[p.set] {
			errs = append(errs, fmt.Errorf("set %q has two opt pointers", p.set))
		}
		pointed[p.set] = true
		if !declared[p.source] {
			errs = append(errs, fmt.Errorf("opt pointer of set %q names undeclared source %q", p.set, p.source))
		}
	}
	if len(m.files) == 0 {
		errs = append(errs, errors.New("manifest lists no files"))
	}
	return errors.Join(errs...)
}

// isCrossFormatSetName reports whether name is a single, ordinary directory
// name.
func isCrossFormatSetName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

func countNonEmpty(fields []string) int {
	n := 0
	for _, f := range fields {
		if f != "" {
			n++
		}
	}
	return n
}

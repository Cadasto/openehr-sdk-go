package templateprobes_test

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/template"
)

// schemaDocuments names the folder TestProbe111WriteSchemaDocuments writes
// to; make operational-template-schema-check sets it.
var schemaDocuments = flag.String("schema-documents", "",
	"write the re-encoded corpus operational templates PROBE-111's schema leg validates to this folder, relative to the module root (make operational-template-schema-check)")

// probe111CorpusFloor is the number of operational templates the corpus
// vendors today: 84. A walk that finds fewer fails, so a corpus that silently
// empties, or a walk that stops finding a folder, cannot pass as clean. Raise
// it when the corpus grows.
const probe111CorpusFloor = 84

func readCorpusTemplate(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return body
}

// PROBE-111 (REQ-176 § Round trip): every operational template the
// corpus vendors decodes with nothing unmatched and nothing the strict mode
// refuses, and re-encodes to a document that decodes to the same value and
// holds the input's content, compared after the normalisation and rewrites
// REQ-176 names.
func TestProbe111OperationalTemplateRoundTrip(t *testing.T) {
	names, err := probes.Probe111OperationalTemplates()
	if err != nil {
		t.Fatalf("Probe111OperationalTemplates: %v", err)
	}
	if len(names) < probe111CorpusFloor {
		t.Fatalf("Probe111OperationalTemplates found %d operational templates under %s, want at least %d", len(names), fixtures.CorpusRoot(), probe111CorpusFloor)
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r, err := probes.Probe111OperationalTemplateRoundTrip(readCorpusTemplate(t, name))
			if err != nil {
				t.Fatalf("Probe111OperationalTemplateRoundTrip(%s): framework error %v", name, err)
			}
			if r.Probe != "PROBE-111" || r.Status != "pass" {
				t.Errorf("Probe111OperationalTemplateRoundTrip(%s) = %s %s (%s), want PROBE-111 pass", name, r.Probe, r.Status, r.Detail)
			}
		})
	}
}

// PROBE-111 (REQ-176): the walk reaches the operational templates beside the
// cross-format sets, which fixtures.ListAllOPTs leaves out, so the probe
// covers the whole corpus.
func TestProbe111OperationalTemplatesReachTheCrossFormatSets(t *testing.T) {
	names, err := probes.Probe111OperationalTemplates()
	if err != nil {
		t.Fatalf("Probe111OperationalTemplates: %v", err)
	}
	listed, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("fixtures.ListAllOPTs: %v", err)
	}
	crossFormat := 0
	for _, name := range names {
		if strings.HasPrefix(name, "crossformat/") {
			crossFormat++
		}
	}
	if crossFormat == 0 {
		t.Errorf("Probe111OperationalTemplates found no operational template under crossformat/, want the cross-format sets' templates")
	}
	if len(names) != len(listed)+crossFormat {
		t.Errorf("Probe111OperationalTemplates found %d operational templates, want the %d fixtures.ListAllOPTs lists plus the %d under crossformat/", len(names), len(listed), crossFormat)
	}
}

// PROBE-111 (REQ-176): every schema ratchet entry names an operational
// template the corpus vendors, so an entry cannot outlive its template.
func TestProbe111SchemaRatchetNamesCorpusTemplates(t *testing.T) {
	entries, err := probes.Probe111SchemaRatchet()
	if err != nil {
		t.Fatalf("Probe111SchemaRatchet: %v", err)
	}
	names, err := probes.Probe111OperationalTemplates()
	if err != nil {
		t.Fatalf("Probe111OperationalTemplates: %v", err)
	}
	for _, e := range entries {
		if !slices.Contains(names, e.Name) {
			t.Errorf("the schema ratchet lists %q, which names no operational template under %s", e.Name, fixtures.CorpusRoot())
		}
	}
}

// PROBE-111 (REQ-176 § Round trip): writes the documents the schema leg
// validates, for make operational-template-schema-check. Under the folder
// -schema-documents names it writes re-encoded/<name> for each corpus
// operational template the schema ratchet does not list, and ratchet.txt,
// the names the ratchet lists, one per line, sorted. Without the flag it is
// skipped.
func TestProbe111WriteSchemaDocuments(t *testing.T) {
	if *schemaDocuments == "" {
		t.Skip("no -schema-documents folder; make operational-template-schema-check sets one")
	}
	dir := *schemaDocuments
	if !filepath.IsAbs(dir) {
		moduleRoot := filepath.Dir(filepath.Dir(fixtures.CorpusRoot()))
		dir = filepath.Join(moduleRoot, dir)
	}
	names, err := probes.Probe111OperationalTemplates()
	if err != nil {
		t.Fatalf("Probe111OperationalTemplates: %v", err)
	}
	entries, err := probes.Probe111SchemaRatchet()
	if err != nil {
		t.Fatalf("Probe111SchemaRatchet: %v", err)
	}
	var held []string
	for _, e := range entries {
		held = append(held, e.Name)
	}
	slices.Sort(held)
	var list strings.Builder
	for _, name := range held {
		list.WriteString(name + "\n")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ratchet.txt"), []byte(list.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	written := 0
	for _, name := range names {
		if slices.Contains(held, name) {
			continue
		}
		out, err := probes.Probe111ReEncode(readCorpusTemplate(t, name))
		if err != nil {
			t.Errorf("Probe111ReEncode(%s): %v", name, err)
			continue
		}
		path := filepath.Join(dir, "re-encoded", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
		written++
	}
	if written == 0 {
		t.Fatalf("wrote no re-encoded document to %s: the schema leg would validate nothing", dir)
	}
}

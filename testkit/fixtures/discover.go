package fixtures

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CompositionJSONRel is a composition or rm JSON cassette path relative to
// [CassettesRoot], e.g. `compositions/body_weight.json` or
// `rm/minimal_evaluation.json`.
type CompositionJSONRel struct {
	Rel      string // forward-slash path under cassettes root
	Template string // filename stem (template id or rm sample name)
	Kind     string // "compositions" or "rm"
}

// compositionJSONExcluded template ids with OPT + JSON on disk but omitted from
// [ListCompositionJSON] until canjson round-trip passes.
var compositionJSONExcluded = map[string]bool{
	"Address.v2":       true, // ADDRESS / PARTY_IDENTITY DV_CODED_TEXT
	"Demonstration.v1": true, // DV_MULTIMEDIA in composition
	"TestPerson.v2":    true, // PERSON / PARTY_IDENTITY DV_CODED_TEXT
}

// compositionXMLExcluded composition XML on disk but not exercised by canxml
// round-trip probes (DV_MULTIMEDIA wire shape, etc.).
var compositionXMLExcluded = map[string]bool{
	"Demonstration.v1": true,
	"TestPerson.v2":    true,
}

// ListCompositionJSON returns every *.json under compositions/ and rm/.
func ListCompositionJSON() ([]CompositionJSONRel, error) {
	var out []CompositionJSONRel
	for _, kind := range []string{"compositions", "rm"} {
		dir := filepath.Join(CassettesRoot(), kind)
		if err := collectJSON(dir, kind, &out); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

func collectJSON(dir, kind string, out *[]CompositionJSONRel) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("fixtures: read %q: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ".json")
		if kind == "compositions" && compositionJSONExcluded[stem] {
			continue
		}
		*out = append(*out, CompositionJSONRel{
			Rel:      filepath.ToSlash(filepath.Join(kind, e.Name())),
			Template: stem,
			Kind:     kind,
		})
	}
	return nil
}

// ResolveCompositionJSON opens a path from [ListCompositionJSON].
func ResolveCompositionJSON(rel CompositionJSONRel) string {
	return filepath.Join(CassettesRoot(), filepath.FromSlash(rel.Rel))
}

// TemplateIDsWithCompositionXML lists template ids with both composition JSON and XML.
func TemplateIDsWithCompositionXML() ([]string, error) {
	dir := compositionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("fixtures: read %q: %w", dir, err)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".xml") {
			continue
		}
		tid := strings.TrimSuffix(e.Name(), ".xml")
		if compositionXMLExcluded[tid] {
			continue
		}
		jsonPath := filepath.Join(dir, tid+".json")
		if _, err := os.Stat(jsonPath); err != nil {
			continue
		}
		if compositionJSONExcluded[tid] {
			continue
		}
		ids = append(ids, tid)
	}
	sort.Strings(ids)
	return ids, nil
}

// ListRMXML returns *.xml paths relative to [CassettesRoot] under compositions/ and rm/.
func ListRMXML() ([]string, error) {
	var out []string
	for _, kind := range []string{"compositions", "rm"} {
		dir := filepath.Join(CassettesRoot(), kind)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("fixtures: read %q: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".xml") {
				continue
			}
			if kind == "compositions" {
				tid := strings.TrimSuffix(e.Name(), ".xml")
				if compositionXMLExcluded[tid] {
					continue
				}
			}
			out = append(out, filepath.ToSlash(filepath.Join(kind, e.Name())))
		}
	}
	sort.Strings(out)
	return out, nil
}

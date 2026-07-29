// Package fixtures resolves vendored cassette paths under testkit/cassettes/.
//
// Layout:
//
//	templates/{template-id}.opt
//	compositions/{template-id}.json
//	compositions/{template-id}.xml
//	rm/{name}.json | .xml          # RM probe samples (ehrbase, leaf, …)
//	submissions/{name}.json       # CONTRIBUTION POST wire (inline ORIGINAL_VERSION)
//	its_rest/                     # ITS-REST wire records
//	flat-conformance/             # pinned upstream FLAT corpus (MANIFEST.txt)
//	  templates/{name}.opt
//	  compositions/{name}.json
//
// Vendor provenance is indexed in testkit/cassettes/README.md (not in paths).
package fixtures

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// CassettesRoot is testkit/cassettes (absolute).
func CassettesRoot() string {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		panic("fixtures: cannot resolve package path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(here), "..", "cassettes"))
}

func templatesDir() string    { return filepath.Join(CassettesRoot(), "templates") }
func compositionsDir() string { return filepath.Join(CassettesRoot(), "compositions") }
func rmDir() string           { return filepath.Join(CassettesRoot(), "rm") }
func submissionsDir() string  { return filepath.Join(CassettesRoot(), "submissions") }

// TemplateOpt returns testkit/cassettes/templates/{template-id}.opt.
func TemplateOpt(templateID string) string {
	return filepath.Join(templatesDir(), resolveTemplateID(templateID)+".opt")
}

// CompositionJSON returns testkit/cassettes/compositions/{template-id}.json.
func CompositionJSON(templateID string) string {
	return filepath.Join(compositionsDir(), resolveTemplateID(templateID)+".json")
}

// CompositionXML returns testkit/cassettes/compositions/{template-id}.xml.
func CompositionXML(templateID string) string {
	return filepath.Join(compositionsDir(), resolveTemplateID(templateID)+".xml")
}

// RMJSON returns testkit/cassettes/rm/{name}.json (ehrbase / leaf RM samples).
func RMJSON(name string) string {
	return filepath.Join(rmDir(), name+".json")
}

// RMXML returns testkit/cassettes/rm/{name}.xml.
func RMXML(name string) string {
	return filepath.Join(rmDir(), name+".xml")
}

// SubmissionJSON returns testkit/cassettes/submissions/{name}.json.
// Files use the ehrbase Robot CONTRIBUTION POST shape (versions[] hold inline
// ORIGINAL_VERSION payloads), not persisted CONTRIBUTION with OBJECT_REF.
func SubmissionJSON(name string) string {
	return filepath.Join(submissionsDir(), name+".json")
}

// FlatConformanceRoot is testkit/cassettes/flat-conformance — the pinned
// upstream EHRbase FLAT serialisation corpus (see MANIFEST.txt there, and
// scripts/sync-flat-conformance.sh). Vendored Apache-2.0; provenance in
// THIRD_PARTY_LICENSES.md.
func FlatConformanceRoot() string {
	return filepath.Join(CassettesRoot(), "flat-conformance")
}

// FlatConformanceOpt returns the single operational template every fixture in
// the FLAT conformance corpus instantiates.
func FlatConformanceOpt() string {
	return filepath.Join(FlatConformanceRoot(), "templates", "conformance_ehrbase.de.v0.opt")
}

// FlatConformanceFlat returns testkit/cassettes/flat-conformance/compositions/{name}.json.
func FlatConformanceFlat(name string) string {
	return filepath.Join(FlatConformanceRoot(), "compositions", name+".json")
}

// ListFlatConformance returns the FLAT conformance fixture names (no
// extension), sorted. It reports an error when the corpus is absent so a
// caller can skip rather than silently assert nothing.
func ListFlatConformance() ([]string, error) {
	dir := filepath.Join(FlatConformanceRoot(), "compositions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	slices.Sort(names)
	return names, nil
}

// idAlias maps test shorthands to on-disk template ids when they differ.
var idAlias = map[string]string{
	"clinical_note": "clinical_notes.v0",
}

func resolveTemplateID(name string) string {
	key := TemplateSlug(name)
	if id, ok := idAlias[key]; ok {
		return id
	}
	return key
}

// TemplateSlug strips a trailing .opt from a fixture name.
func TemplateSlug(name string) string {
	const suffix = ".opt"
	if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
		return name[:len(name)-len(suffix)]
	}
	return name
}

// TemplateOptForName resolves [TemplateOpt] from a shorthand or template id.
func TemplateOptForName(name string) string {
	return TemplateOpt(name)
}

// CanonicalJSON is an alias for [CompositionJSON] (legacy name in call sites).
func CanonicalJSON(templateID string) string {
	return CompositionJSON(templateID)
}

// CanonicalXML is an alias for [CompositionXML].
func CanonicalXML(templateID string) string {
	return CompositionXML(templateID)
}

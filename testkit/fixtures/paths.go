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
//	webtemplate/{template-id}.opt # OPT + EHRbase reference WebTemplate golden
//	  | {template-id}.webtemplate.json   (PROBE-075 / REQ-116 oracles)
//	flat-conformance/             # pinned upstream FLAT corpus (MANIFEST.txt)
//	  templates/{name}.opt
//	  compositions/{name}.json
//	aql/conformance/              # pinned upstream AQL FROM corpus (AQL_SOURCE.txt)
//	  {family}/{name}.csv
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
func webtemplateDir() string  { return filepath.Join(CassettesRoot(), "webtemplate") }

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

// WebTemplateOpt returns testkit/cassettes/webtemplate/{template-id}.opt — an
// OPT vendored beside its EHRbase reference WebTemplate golden (the PROBE-075
// / REQ-116 oracles). Vendored Apache-2.0 at a pinned upstream commit;
// provenance in THIRD_PARTY_LICENSES.md. Stems match template_id values.
func WebTemplateOpt(templateID string) string {
	return filepath.Join(webtemplateDir(), templateID+".opt")
}

// WebTemplateReference returns
// testkit/cassettes/webtemplate/{template-id}.webtemplate.json — the EHRbase
// reference WebTemplate golden for the same-stem OPT.
func WebTemplateReference(templateID string) string {
	return filepath.Join(webtemplateDir(), templateID+".webtemplate.json")
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

// AQLConformanceRoot is testkit/cassettes/aql/conformance — the pinned
// upstream EHRbase Robot AQL FROM-family combination corpus (see AQL_SOURCE.txt
// and EXCLUDED.txt there, and scripts/ingest-robot-aql.sh). Vendored
// Apache-2.0; provenance in THIRD_PARTY_LICENSES.md. The family directories
// under it hold the CSVs PROBE-100 reconstructs into queries.
func AQLConformanceRoot() string {
	return filepath.Join(CassettesRoot(), "aql", "conformance")
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

// optDirs are every cassette directory holding operational templates,
// in [ListAllOPTs] scan order.
func optDirs() []string {
	return []string{
		templatesDir(),
		webtemplateDir(),
		filepath.Join(FlatConformanceRoot(), "templates"),
		filepath.Join(CassettesRoot(), "its_rest", "definition"),
	}
}

// OPTRef is one vendored operational template: a stable Name (the
// dir-qualified stem, e.g. `templates/minimal_evaluation.en.v1` or
// `webtemplate/Corona_Anamnese`) and its absolute Path. The name is
// dir-qualified because stems are only unique within a directory.
type OPTRef struct {
	Name string
	Path string
}

// ListAllOPTs returns every operational template vendored under
// [CassettesRoot] — across templates/, webtemplate/, the FLAT
// conformance corpus, and the ITS-REST definition cassettes — sorted by
// Name. Callers that must cover the whole corpus (REQ-116's compiled-path
// regression guard) use this rather than naming templates individually,
// so a newly vendored OPT is picked up automatically.
func ListAllOPTs() ([]OPTRef, error) {
	var refs []OPTRef
	for _, dir := range optDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		label := filepath.Base(dir)
		if label == "templates" && filepath.Base(filepath.Dir(dir)) == "flat-conformance" {
			label = "flat-conformance"
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".opt" {
				continue
			}
			refs = append(refs, OPTRef{
				Name: label + "/" + strings.TrimSuffix(e.Name(), ".opt"),
				Path: filepath.Join(dir, e.Name()),
			})
		}
	}
	slices.SortFunc(refs, func(a, b OPTRef) int { return strings.Compare(a.Name, b.Name) })
	return refs, nil
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

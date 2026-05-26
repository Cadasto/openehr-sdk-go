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
//
// Vendor provenance is indexed in testkit/cassettes/README.md (not in paths).
package fixtures

import (
	"path/filepath"
	"runtime"
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

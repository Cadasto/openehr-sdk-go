package fixtures

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

type rootEnvelope struct {
	Type string `json:"_type"`
}

// RootTypeFromJSON reads the top-level "_type" from a canonical JSON cassette.
func RootTypeFromJSON(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var env rootEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("fixtures: decode root _type from %q: %w", path, err)
	}
	if env.Type == "" {
		return "", fmt.Errorf("fixtures: missing _type in %q", path)
	}
	return env.Type, nil
}

// FactoryForJSONRel returns a fresh-target factory for a composition or rm JSON
// cassette. The second value is false when the root RM type is not wired for
// codec round-trip probes (callers should skip those cassettes).
func FactoryForJSONRel(rel CompositionJSONRel) (func() any, bool) {
	if rel.Kind == "rm" {
		return factoryForRMFilename(rel.Rel)
	}
	path := ResolveCompositionJSON(rel)
	root, err := RootTypeFromJSON(path)
	if err != nil {
		return nil, false
	}
	return FactoryForRootType(root)
}

// FactoryForRootType maps canonical JSON "_type" strings to RM structs.
func FactoryForRootType(root string) (func() any, bool) {
	switch root {
	case "COMPOSITION":
		return func() any { return new(rm.Composition) }, true
	case "PERSON":
		return func() any { return new(rm.Person) }, true
	case "ORGANISATION":
		return func() any { return new(rm.Organisation) }, true
	case "GROUP":
		return func() any { return new(rm.Group) }, true
	case "ADDRESS":
		return func() any { return new(rm.Address) }, true
	case "EHR_STATUS":
		return func() any { return new(rm.EHRStatus) }, true
	case "FOLDER":
		return func() any { return new(rm.Folder) }, true
	default:
		return nil, false
	}
}

func factoryForRMFilename(rel string) (func() any, bool) {
	base := strings.ToLower(filepath.Base(rel))
	switch {
	case strings.Contains(base, "ehr_status"):
		return func() any { return new(rm.EHRStatus) }, true
	case strings.Contains(base, "folder"):
		return func() any { return new(rm.Folder) }, true
	default:
		return func() any { return new(rm.Composition) }, true
	}
}

// FactoryForXMLBody picks a factory from the root element local name.
func FactoryForXMLBody(body []byte) (func() any, bool) {
	s := string(body)
	for {
		i := strings.Index(s, "<")
		if i < 0 {
			return nil, false
		}
		s = s[i:]
		if strings.HasPrefix(s, "<?") {
			end := strings.Index(s, "?>")
			if end < 0 {
				return nil, false
			}
			s = s[end+2:]
			continue
		}
		break
	}
	switch {
	case strings.HasPrefix(s, "<dv_quantity"), strings.HasPrefix(s, "<DV_QUANTITY"):
		return func() any { return new(rm.DVQuantity) }, true
	case strings.HasPrefix(s, "<composition"), strings.HasPrefix(s, "<COMPOSITION"):
		return func() any { return new(rm.Composition) }, true
	case strings.HasPrefix(s, "<folder"), strings.HasPrefix(s, "<FOLDER"):
		return func() any { return new(rm.Folder) }, true
	case strings.HasPrefix(s, "<ehr_status"), strings.HasPrefix(s, "<EHR_STATUS"):
		return func() any { return new(rm.EHRStatus) }, true
	case strings.HasPrefix(s, "<person"), strings.HasPrefix(s, "<PERSON"):
		return func() any { return new(rm.Person) }, true
	case strings.HasPrefix(s, "<organisation"), strings.HasPrefix(s, "<ORGANISATION"):
		return func() any { return new(rm.Organisation) }, true
	case strings.HasPrefix(s, "<group"), strings.HasPrefix(s, "<GROUP"):
		return func() any { return new(rm.Group) }, true
	case strings.HasPrefix(s, "<address"), strings.HasPrefix(s, "<ADDRESS"):
		return func() any { return new(rm.Address) }, true
	default:
		return nil, false
	}
}

// FactoryHintForRel returns the canonical JSON "_type" for a cassette when known.
// Filename hints are used for rm/ samples; compositions/ uses on-disk JSON.
func FactoryHintForRel(rel string) string {
	if strings.HasPrefix(rel, "rm/") {
		base := strings.ToLower(filepath.Base(rel))
		switch {
		case strings.Contains(base, "ehr_status"):
			return "EHR_STATUS"
		case strings.Contains(base, "folder"):
			return "FOLDER"
		default:
			return "COMPOSITION"
		}
	}
	if strings.HasPrefix(rel, "compositions/") {
		path := filepath.Join(CassettesRoot(), filepath.FromSlash(rel))
		root, err := RootTypeFromJSON(path)
		if err != nil {
			return "COMPOSITION"
		}
		return root
	}
	return "COMPOSITION"
}

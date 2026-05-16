package bmm

import (
	"encoding/json"
	"fmt"
	"io"
)

// Load parses a single P_BMM JSON document from r into an in-memory
// [Schema]. The decoder dispatches on the `_type` discriminator for
// every polymorphic node (properties, types, function parameters,
// classes) and validates the presence of required fields.
//
// Load does NOT resolve `includes` — to merge an ancestor schema in,
// use [LoadAll] with a [Resolver].
func Load(r io.Reader) (*Schema, error) {
	if r == nil {
		return nil, fmt.Errorf("bmm.Load: reader is nil")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("bmm.Load: read: %w", err)
	}
	return loadFromBytes(data)
}

func loadFromBytes(data []byte) (*Schema, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("bmm.Load: empty input")
	}
	var aux struct {
		BMMVersion           string                     `json:"bmm_version"`
		RMPublisher          string                     `json:"rm_publisher"`
		SchemaName           string                     `json:"schema_name"`
		RMRelease            string                     `json:"rm_release"`
		SchemaRevision       string                     `json:"schema_revision"`
		SchemaLifecycleState string                     `json:"schema_lifecycle_state"`
		SchemaDescription    string                     `json:"schema_description"`
		SchemaAuthor         string                     `json:"schema_author"`
		Includes             map[string]*IncludeRef     `json:"includes"`
		Packages             map[string]json.RawMessage `json:"packages"`
		PrimitiveTypes       map[string]json.RawMessage `json:"primitive_types"`
		ClassDefinitions     map[string]json.RawMessage `json:"class_definitions"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, fmt.Errorf("bmm.Load: parse JSON: %w", err)
	}
	if aux.SchemaName == "" {
		return nil, fmt.Errorf("%w: schema_name", ErrMissingField)
	}

	s := &Schema{
		BMMVersion:           aux.BMMVersion,
		RMPublisher:          aux.RMPublisher,
		SchemaName:           aux.SchemaName,
		RMRelease:            aux.RMRelease,
		SchemaRevision:       aux.SchemaRevision,
		SchemaLifecycleState: aux.SchemaLifecycleState,
		SchemaDescription:    aux.SchemaDescription,
		SchemaAuthor:         aux.SchemaAuthor,
		Includes:             aux.Includes,
	}
	if len(aux.Packages) > 0 {
		s.Packages = make(map[string]*Package, len(aux.Packages))
		for name, raw := range aux.Packages {
			pkg, err := decodePackage(raw, "packages."+name)
			if err != nil {
				return nil, err
			}
			s.Packages[name] = pkg
		}
	}
	if len(aux.PrimitiveTypes) > 0 {
		s.PrimitiveTypes = make(map[string]Class, len(aux.PrimitiveTypes))
		for name, raw := range aux.PrimitiveTypes {
			cls, err := decodeClass(raw, "primitive_types."+name)
			if err != nil {
				return nil, err
			}
			s.PrimitiveTypes[name] = cls
		}
	}
	if len(aux.ClassDefinitions) > 0 {
		s.ClassDefinitions = make(map[string]Class, len(aux.ClassDefinitions))
		for name, raw := range aux.ClassDefinitions {
			cls, err := decodeClass(raw, "class_definitions."+name)
			if err != nil {
				return nil, err
			}
			s.ClassDefinitions[name] = cls
		}
	}
	return s, nil
}

func decodePackage(raw json.RawMessage, path string) (*Package, error) {
	var aux struct {
		Name     string                     `json:"name"`
		Classes  []string                   `json:"classes"`
		Packages map[string]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return nil, fmt.Errorf("decode Package at %s: %w", path, err)
	}
	if aux.Name == "" {
		return nil, fmt.Errorf("%w: Package.name at %s", ErrMissingField, path)
	}
	p := &Package{
		Name:    aux.Name,
		Classes: aux.Classes,
	}
	if len(aux.Packages) > 0 {
		p.Packages = make(map[string]*Package, len(aux.Packages))
		for k, v := range aux.Packages {
			child, err := decodePackage(v, path+".packages."+k)
			if err != nil {
				return nil, err
			}
			p.Packages[k] = child
		}
	}
	return p, nil
}

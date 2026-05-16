package bmm

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Schema is the top-level model of a parsed P_BMM document.
type Schema struct {
	BMMVersion           string                 `json:"bmm_version"`
	RMPublisher          string                 `json:"rm_publisher"`
	SchemaName           string                 `json:"schema_name"`
	RMRelease            string                 `json:"rm_release"`
	SchemaRevision       string                 `json:"schema_revision"`
	SchemaLifecycleState string                 `json:"schema_lifecycle_state"`
	SchemaDescription    string                 `json:"schema_description,omitempty"`
	SchemaAuthor         string                 `json:"schema_author,omitempty"`
	Includes             map[string]*IncludeRef `json:"includes,omitempty"`
	Packages             map[string]*Package    `json:"packages,omitempty"`
	PrimitiveTypes       map[string]Class       `json:"primitive_types,omitempty"`
	ClassDefinitions     map[string]Class       `json:"class_definitions,omitempty"`
}

// IncludeRef mirrors an entry under the schema's "includes" map.
// Each entry has the shape {"id": "<schema_id>"}.
type IncludeRef struct {
	ID string `json:"id"`
}

// SchemaID returns the canonical id string used as a Resolver key —
// the same form that appears in the BMM "includes" map and as the
// resources/bmm/<id>.bmm.json filename stem. The id is composed as
// "<rm_publisher>_<schema_name>_<rm_release>" (e.g. "openehr_base_1.3.0").
// If RMPublisher is empty, the publisher prefix is omitted; if both
// RMPublisher and RMRelease are empty, SchemaName is returned alone.
func (s *Schema) SchemaID() string {
	if s.SchemaName == "" {
		return ""
	}
	parts := make([]string, 0, 3)
	if s.RMPublisher != "" {
		parts = append(parts, s.RMPublisher)
	}
	parts = append(parts, s.SchemaName)
	if s.RMRelease != "" {
		parts = append(parts, s.RMRelease)
	}
	return joinNonEmpty(parts, "_")
}

func joinNonEmpty(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i > 0 && out != "" {
			out += sep
		}
		out += p
	}
	return out
}

// Package is a node in the recursive package tree. Each package has a
// fully qualified Name (e.g. "org.openehr.rm.data_types.quantity"), an
// optional list of class names (which must be looked up in the schema's
// ClassDefinitions map), and an optional sub-package map.
type Package struct {
	Name     string              `json:"name"`
	Classes  []string            `json:"classes,omitempty"`
	Packages map[string]*Package `json:"packages,omitempty"`
}

// MarshalJSON for Schema — emits polymorphic Class values with their
// _type discriminators by relying on each concrete type's MarshalJSON.
func (s *Schema) MarshalJSON() ([]byte, error) {
	// Use a buffer-based emitter for stable key ordering on the
	// top-level fields. Polymorphic values flow through their own
	// MarshalJSON methods automatically.
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	emit := func(key string, val any) error {
		if val == nil {
			return nil
		}
		// Skip empty containers.
		switch v := val.(type) {
		case string:
			if v == "" {
				return nil
			}
		case map[string]*Package:
			if len(v) == 0 {
				return nil
			}
		case map[string]Class:
			if len(v) == 0 {
				return nil
			}
		case map[string]*IncludeRef:
			if len(v) == 0 {
				return nil
			}
		}
		b, err := json.Marshal(val)
		if err != nil {
			return err
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(key)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(b)
		return nil
	}
	for _, f := range []struct {
		k string
		v any
	}{
		{"bmm_version", s.BMMVersion},
		{"rm_publisher", s.RMPublisher},
		{"schema_name", s.SchemaName},
		{"rm_release", s.RMRelease},
		{"schema_revision", s.SchemaRevision},
		{"schema_lifecycle_state", s.SchemaLifecycleState},
		{"schema_description", s.SchemaDescription},
		{"schema_author", s.SchemaAuthor},
		{"includes", s.Includes},
		{"packages", s.Packages},
		{"primitive_types", classMapWrapper(s.PrimitiveTypes)},
		{"class_definitions", classMapWrapper(s.ClassDefinitions)},
	} {
		if err := emit(f.k, f.v); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// classMapWrapper marshals a map[string]Class as a stable JSON object,
// preserving each concrete Class's MarshalJSON (so polymorphism is
// preserved on the wire).
type classMapWrapper map[string]Class

func (m classMapWrapper) MarshalJSON() ([]byte, error) {
	if len(m) == 0 {
		return []byte("{}"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	keys := sortedKeys(m)
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(m[k])
		if err != nil {
			return nil, fmt.Errorf("marshal class %q: %w", k, err)
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

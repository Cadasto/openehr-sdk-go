package aql

import "encoding/json"

// ResultSet is the openEHR REST RESULT_SET shape (Query API).
type ResultSet struct {
	Meta    ResultMeta     `json:"meta"`
	Name    string         `json:"name,omitempty"`
	Q       string         `json:"q,omitempty"`
	Columns []Column       `json:"columns,omitempty"`
	Rows    [][]ResultCell `json:"rows"`
	// Extras preserves deployment-specific top-level fields.
	Extras map[string]json.RawMessage `json:"-"`
}

// ResultMeta carries RESULT_SET metadata (_type, _created, …).
type ResultMeta struct {
	Href          string                     `json:"_href,omitempty"`
	Type          string                     `json:"_type,omitempty"`
	SchemaVersion string                     `json:"_schema_version,omitempty"`
	Created       string                     `json:"_created,omitempty"`
	Generator     string                     `json:"_generator,omitempty"`
	ExecutedAQL   string                     `json:"_executed_aql,omitempty"`
	Extras        map[string]json.RawMessage `json:"-"`
}

// Column describes one result column (name + optional archetype path).
type Column struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// ResultCell is one cell in a result row (JSON-decoded as any).
type ResultCell = any

// UnmarshalJSON decodes ResultSet and preserves unknown top-level keys.
func (rs *ResultSet) UnmarshalJSON(data []byte) error {
	type alias ResultSet
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*rs = ResultSet(a)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	known := map[string]struct{}{
		"meta": {}, "name": {}, "q": {}, "columns": {}, "rows": {},
	}
	for k, v := range raw {
		if _, ok := known[k]; ok {
			continue
		}
		if rs.Extras == nil {
			rs.Extras = map[string]json.RawMessage{}
		}
		rs.Extras[k] = v
	}
	return nil
}

// UnmarshalJSON decodes ResultMeta and preserves unknown keys.
func (m *ResultMeta) UnmarshalJSON(data []byte) error {
	type alias ResultMeta
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*m = ResultMeta(a)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	known := map[string]struct{}{
		"_href": {}, "_type": {}, "_schema_version": {}, "_created": {},
		"_generator": {}, "_executed_aql": {},
	}
	for k, v := range raw {
		if _, ok := known[k]; ok {
			continue
		}
		if m.Extras == nil {
			m.Extras = map[string]json.RawMessage{}
		}
		m.Extras[k] = v
	}
	return nil
}

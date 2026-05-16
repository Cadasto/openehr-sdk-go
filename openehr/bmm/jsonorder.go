package bmm

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// orderedJSONObjectKeysFromClass extracts the key order of a nested JSON
// object on a class-shaped document (e.g. "properties", "functions").
func orderedJSONObjectKeysFromClass(classRaw json.RawMessage, field string) ([]string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(classRaw, &raw); err != nil {
		return nil, err
	}
	nested, ok := raw[field]
	if !ok || len(nested) == 0 {
		return nil, nil
	}
	return orderedJSONObjectKeys(nested)
}

// orderedJSONObjectKeys returns object keys in wire order.
func orderedJSONObjectKeys(raw json.RawMessage) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("expected JSON object, got %v", tok)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("expected object key string, got %T", tok)
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return keys, nil
}

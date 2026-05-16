package bmm

import (
	"bytes"
	"encoding/json"
	"sort"
)

// sortedKeys returns the keys of m in lexicographic order. Used to
// produce stable JSON output for polymorphic maps.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// strictUnmarshal decodes raw into v with DisallowUnknownFields = false
// — kept as a thin wrapper so we can revisit strictness in one place if
// the BMM corpus ever stabilises. Strict-mode parsing is currently OFF
// because the BMM files carry implementation-specific extra fields
// (e.g. item_documentations on otherwise standard objects) that aren't
// part of the loader's typed model but should not be a hard error.
func strictUnmarshal(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	return dec.Decode(v)
}

// isJSONNull reports whether raw is the four bytes spelling JSON null.
func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// marshalDiscriminated produces a JSON object with a leading _type
// field, then the common fields, then the extra fields. Stable key
// ordering: _type, then common fields in declaration order (matching
// the BMM convention), then extra keys sorted.
func marshalDiscriminated(disc string, common any, extra map[string]any) ([]byte, error) {
	// Marshal common to grab the embedded fields.
	cb, err := json.Marshal(common)
	if err != nil {
		return nil, err
	}
	var commonMap map[string]json.RawMessage
	if err := json.Unmarshal(cb, &commonMap); err != nil {
		return nil, err
	}
	// Build the output buffer.
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	writeKV := func(k string, vb []byte) {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	writeKV("_type", []byte("\""+disc+"\""))
	// common fields in stable order (sorted)
	commonKeys := sortedKeys(commonMap)
	for _, k := range commonKeys {
		writeKV(k, commonMap[k])
	}
	// extra keys
	extraKeys := sortedKeys(extra)
	for _, k := range extraKeys {
		v := extra[k]
		if v == nil {
			continue
		}
		// skip empty optionals
		if isEmptyOptional(v) {
			continue
		}
		vb, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		writeKV(k, vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func isEmptyOptional(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []string:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	case map[string]*GenericParameterDef:
		return len(x) == 0
	case map[string]Type:
		return len(x) == 0
	case map[string]string:
		return len(x) == 0
	}
	return false
}

// optMap returns nil for an empty map (so marshalDiscriminated skips
// it). Used to avoid emitting empty objects.
func optMap[K comparable, V any](m map[K]V) map[K]V {
	if len(m) == 0 {
		return nil
	}
	return m
}

// marshalClassObject emits a class object, optionally prefixed by a
// _type discriminator. classKind == "" omits the discriminator (the
// default class shape).
func marshalClassObject(classKind string, c *classCommon, extra map[string]any) ([]byte, error) {
	// Marshal the common into a map.
	type commonExport struct {
		Name       string               `json:"name"`
		Doc        string               `json:"documentation,omitempty"`
		Ancestors  []string             `json:"ancestors,omitempty"`
		IsAbstract bool                 `json:"is_abstract,omitempty"`
		Functions  map[string]*Function `json:"functions,omitempty"`
		Invariants map[string]string    `json:"invariants,omitempty"`
	}
	cb, err := json.Marshal(&commonExport{
		Name:       c.Name,
		Doc:        c.Doc,
		Ancestors:  c.Ancestors_,
		IsAbstract: c.IsAbstractFlag,
		Functions:  c.Functions,
		Invariants: c.Invariants,
	})
	if err != nil {
		return nil, err
	}
	var commonMap map[string]json.RawMessage
	if err := json.Unmarshal(cb, &commonMap); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	writeKV := func(k string, vb []byte) {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	if classKind != "" {
		writeKV("_type", []byte("\""+classKind+"\""))
	}
	if len(c.Properties) > 0 {
		propsJSON, err := marshalPropertiesOrdered(c)
		if err != nil {
			return nil, err
		}
		writeKV("properties", propsJSON)
		delete(commonMap, "properties")
	}
	for _, k := range sortedKeys(commonMap) {
		writeKV(k, commonMap[k])
	}
	for _, k := range sortedKeys(extra) {
		v := extra[k]
		if v == nil {
			continue
		}
		if isEmptyOptional(v) {
			continue
		}
		// special: marshalling a typed-nil slice/map yields "null"; skip null
		vb, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		if isJSONNull(vb) {
			continue
		}
		writeKV(k, vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalPropertiesOrdered emits the properties object with keys in
// BMM declaration order (PropertyOrder).
func marshalPropertiesOrdered(c *classCommon) ([]byte, error) {
	names := c.PropertyOrder
	if len(names) == 0 {
		names = sortedKeys(c.Properties)
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	for _, name := range names {
		p, ok := c.Properties[name]
		if !ok {
			continue
		}
		vb, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(name)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

package bmm

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Type is the abstract category of P_BMM_*_TYPE values: SimpleType,
// GenericType, and ContainerType. The unexported marker keeps external
// packages from declaring imposter implementations.
type Type interface {
	isType()
}

// SimpleType represents P_BMM_SIMPLE_TYPE — a reference to a named type
// (a class, an enumeration, a primitive, or an open generic parameter
// when the parent property is the *_OPEN variant).
type SimpleType struct {
	// TypeName is the BMM "type" field. The name "TypeName" avoids the
	// Go-reserved-feeling identifier "Type".
	TypeName string `json:"type"`
}

// TypeP_BMM_SIMPLE_TYPE is the SimpleType _type discriminator value.
const TypeP_BMM_SIMPLE_TYPE = "P_BMM_SIMPLE_TYPE"

func (*SimpleType) isType() {}

// MarshalJSON emits {"_type":"P_BMM_SIMPLE_TYPE","type":"X"}.
func (s *SimpleType) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     string `json:"_type"`
		TypeName string `json:"type"`
	}{TypeP_BMM_SIMPLE_TYPE, s.TypeName})
}

// GenericType represents P_BMM_GENERIC_TYPE — a root type with generic
// parameter bindings.
//
// Three on-wire shapes for the parameter slot are observed:
//
//  1. "generic_parameters": ["String","CODE_PHRASE"]      // bare names
//  2. "generic_parameters": [..., {"_type":"P_BMM_GENERIC_TYPE", ...}, ...]
//     — i.e. a mixed array where some entries are nested type objects
//  3. "generic_parameter_defs": { "K": {...Type...}, "V": {...} } — a
//     keyed map (named parameters), used at class-level generic defs and
//     also in some property type_defs.
//
// The loader normalises all three into GenericParameters ([]Type): each
// bare string becomes a *SimpleType{TypeName: name}; nested type objects
// are decoded via decodeType. The keyed-map form is preserved in
// GenericParameterDefs because the parameter names carry meaning.
type GenericType struct {
	RootType             string          `json:"root_type"`
	GenericParameters    []Type          `json:"generic_parameters,omitempty"`
	GenericParameterDefs map[string]Type `json:"generic_parameter_defs,omitempty"`
}

// TypeP_BMM_GENERIC_TYPE is the GenericType _type discriminator value.
const TypeP_BMM_GENERIC_TYPE = "P_BMM_GENERIC_TYPE"

func (*GenericType) isType() {}

// MarshalJSON emits the discriminator plus either generic_parameters or
// generic_parameter_defs (whichever is non-empty).
//
// generic_parameters is emitted as a mixed JSON array: any item that is a
// plain *SimpleType is written as a bare string ("X"); any other Type is
// written as its full discriminated object form. This preserves the
// original on-wire shape under round-trip.
func (g *GenericType) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	buf.WriteString(`"_type":"` + TypeP_BMM_GENERIC_TYPE + `",`)
	rt, err := json.Marshal(g.RootType)
	if err != nil {
		return nil, err
	}
	buf.WriteString(`"root_type":`)
	buf.Write(rt)
	if len(g.GenericParameters) > 0 {
		buf.WriteString(`,"generic_parameters":`)
		if err := marshalGenericParameters(&buf, g.GenericParameters); err != nil {
			return nil, err
		}
	}
	if len(g.GenericParameterDefs) > 0 {
		buf.WriteString(`,"generic_parameter_defs":`)
		if err := marshalTypeMap(&buf, g.GenericParameterDefs); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalGenericParameters writes a []Type slice as a JSON array where
// each *SimpleType item is emitted as a bare string and any other Type
// uses its concrete MarshalJSON.
func marshalGenericParameters(buf *bytes.Buffer, ps []Type) error {
	buf.WriteByte('[')
	for i, p := range ps {
		if i > 0 {
			buf.WriteByte(',')
		}
		switch v := p.(type) {
		case *SimpleType:
			b, err := json.Marshal(v.TypeName)
			if err != nil {
				return err
			}
			buf.Write(b)
		default:
			b, err := json.Marshal(p)
			if err != nil {
				return err
			}
			buf.Write(b)
		}
	}
	buf.WriteByte(']')
	return nil
}

// ContainerType represents P_BMM_CONTAINER_TYPE — a container kind
// (List|Set|Array|Hash) wrapping an inner type.
//
// Two on-wire shapes are observed:
//
//   - "type_def":  <nested Type object>   (the richer form)
//   - "type":      "X"                    (short form; the inner type is
//     a SimpleType named X)
//
// On load, the short form is normalised into TypeDef = *SimpleType{X}.
// On emit, the loader writes type_def to keep round-trip stable, since
// the short form is lossy w.r.t. nested generics or containers.
type ContainerType struct {
	ContainerType string `json:"container_type"`
	TypeDef       Type   `json:"type_def"`
}

// TypeP_BMM_CONTAINER_TYPE is the ContainerType _type discriminator value.
const TypeP_BMM_CONTAINER_TYPE = "P_BMM_CONTAINER_TYPE"

func (*ContainerType) isType() {}

// MarshalJSON emits the discriminator plus container_type and type_def.
func (c *ContainerType) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	buf.WriteString(`"_type":"` + TypeP_BMM_CONTAINER_TYPE + `"`)
	ct, err := json.Marshal(c.ContainerType)
	if err != nil {
		return nil, err
	}
	buf.WriteString(`,"container_type":`)
	buf.Write(ct)
	if c.TypeDef != nil {
		td, err := json.Marshal(c.TypeDef)
		if err != nil {
			return nil, err
		}
		buf.WriteString(`,"type_def":`)
		buf.Write(td)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// decodeType dispatches on _type and returns the concrete Type
// implementation. The path argument is threaded for error messages.
func decodeType(raw json.RawMessage, path string) (Type, error) {
	// Peek at _type.
	var head struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, fmt.Errorf("decode Type at %s: %w", path, err)
	}
	switch head.Type {
	case TypeP_BMM_SIMPLE_TYPE:
		return decodeSimpleType(raw, path)
	case TypeP_BMM_GENERIC_TYPE:
		return decodeGenericType(raw, path)
	case TypeP_BMM_CONTAINER_TYPE:
		return decodeContainerType(raw, path)
	default:
		return nil, &unknownTypeError{Discriminator: head.Type, Path: path}
	}
}

func decodeSimpleType(raw json.RawMessage, path string) (*SimpleType, error) {
	var s SimpleType
	if err := strictUnmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("decode SimpleType at %s: %w", path, err)
	}
	if s.TypeName == "" {
		return nil, fmt.Errorf("%w: SimpleType.type at %s", ErrMissingField, path)
	}
	return &s, nil
}

func decodeGenericType(raw json.RawMessage, path string) (*GenericType, error) {
	var aux struct {
		RootType             string                     `json:"root_type"`
		GenericParameters    []json.RawMessage          `json:"generic_parameters"`
		GenericParameterDefs map[string]json.RawMessage `json:"generic_parameter_defs"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return nil, fmt.Errorf("decode GenericType at %s: %w", path, err)
	}
	if aux.RootType == "" {
		return nil, fmt.Errorf("%w: GenericType.root_type at %s", ErrMissingField, path)
	}
	g := &GenericType{RootType: aux.RootType}
	if len(aux.GenericParameters) > 0 {
		g.GenericParameters = make([]Type, len(aux.GenericParameters))
		for i, p := range aux.GenericParameters {
			pt, err := decodeGenericParameter(p, fmt.Sprintf("%s.generic_parameters[%d]", path, i))
			if err != nil {
				return nil, err
			}
			g.GenericParameters[i] = pt
		}
	}
	if len(aux.GenericParameterDefs) > 0 {
		g.GenericParameterDefs = make(map[string]Type, len(aux.GenericParameterDefs))
		for k, v := range aux.GenericParameterDefs {
			t, err := decodeType(v, path+".generic_parameter_defs."+k)
			if err != nil {
				return nil, err
			}
			g.GenericParameterDefs[k] = t
		}
	}
	return g, nil
}

// decodeGenericParameter handles a single item of generic_parameters,
// which may be a bare string (name of a SimpleType / open parameter) or
// a nested object with a _type discriminator (typically another
// P_BMM_GENERIC_TYPE).
func decodeGenericParameter(raw json.RawMessage, path string) (Type, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var name string
		if err := json.Unmarshal(raw, &name); err != nil {
			return nil, fmt.Errorf("decode generic parameter (string) at %s: %w", path, err)
		}
		if name == "" {
			return nil, fmt.Errorf("%w: empty generic parameter at %s", ErrMissingField, path)
		}
		return &SimpleType{TypeName: name}, nil
	}
	return decodeType(raw, path)
}

// decodeContainerType handles both shapes:
//
//	{"_type":"P_BMM_CONTAINER_TYPE","container_type":"List","type_def":{...}}
//	{"_type":"P_BMM_CONTAINER_TYPE","container_type":"List","type":"ITEM"}
//
// In the short form the inner type is normalised to a *SimpleType.
func decodeContainerType(raw json.RawMessage, path string) (*ContainerType, error) {
	var aux struct {
		ContainerType string          `json:"container_type"`
		TypeDef       json.RawMessage `json:"type_def"`
		ShortType     string          `json:"type"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return nil, fmt.Errorf("decode ContainerType at %s: %w", path, err)
	}
	if aux.ContainerType == "" {
		return nil, fmt.Errorf("%w: ContainerType.container_type at %s", ErrMissingField, path)
	}
	c := &ContainerType{ContainerType: aux.ContainerType}
	switch {
	case len(aux.TypeDef) > 0 && !isJSONNull(aux.TypeDef):
		inner, err := decodeType(aux.TypeDef, path+".type_def")
		if err != nil {
			return nil, err
		}
		c.TypeDef = inner
	case aux.ShortType != "":
		c.TypeDef = &SimpleType{TypeName: aux.ShortType}
	default:
		// Observed in the wild (e.g. lang BMM_CLASS.features.result):
		// a P_BMM_CONTAINER_TYPE with only container_type set, no
		// inner type — typically an abstract / unresolved declaration.
		// We accept it and leave TypeDef nil; consumers MAY treat this
		// as an error in their own validation layer.
		c.TypeDef = nil
	}
	return c, nil
}

// decodeTypeNoDisc decodes a Type-like object that does NOT carry a
// _type discriminator. The caller specifies which shape it expects:
//
//   - "generic"   → GenericType (used inside P_BMM_GENERIC_PROPERTY's type_def)
//   - "container" → ContainerType (used inside P_BMM_CONTAINER_PROPERTY's type_def)
func decodeTypeNoDisc(raw json.RawMessage, kind, path string) (Type, error) {
	switch kind {
	case "generic":
		return decodeGenericType(raw, path)
	case "container":
		return decodeContainerType(raw, path)
	default:
		return nil, fmt.Errorf("decodeTypeNoDisc: unsupported kind %q at %s", kind, path)
	}
}

// marshalTypeMap writes a map[string]Type as a JSON object preserving
// key ordering by name (sorted) — important for stable round-trip
// output. Each value uses the concrete type's MarshalJSON.
func marshalTypeMap(buf *bytes.Buffer, m map[string]Type) error {
	keys := sortedKeys(m)
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(m[k])
		if err != nil {
			return err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return nil
}

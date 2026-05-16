package bmm

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Property is the abstract category of P_BMM_*_PROPERTY values. Four
// concrete variants: SingleProperty, SinglePropertyOpen,
// ContainerProperty, GenericProperty.
type Property interface {
	isProperty()
	// PropertyName returns the property's BMM name (the map key as
	// well as the embedded "name" field).
	PropertyName() string
}

// Cardinality is P_BMM_CONTAINER_PROPERTY.cardinality —
// {lower, upper?, upper_unbounded?}. The "upper" field is omitted
// when upper_unbounded is true.
type Cardinality struct {
	Lower          int  `json:"lower"`
	Upper          *int `json:"upper,omitempty"`
	UpperUnbounded bool `json:"upper_unbounded,omitempty"`
}

// propertyCommon holds fields shared by all four Property variants.
// It is embedded into each concrete impl so JSON unmarshalling and
// MarshalJSON share the field layout.
type propertyCommon struct {
	Name          string `json:"name"`
	Documentation string `json:"documentation,omitempty"`
	IsMandatory   bool   `json:"is_mandatory,omitempty"`
}

// PropertyName implements Property.
func (p propertyCommon) PropertyName() string { return p.Name }

// SingleProperty is P_BMM_SINGLE_PROPERTY: { type: "X" } where X is
// the name of a concrete class, an enumeration, or a primitive.
type SingleProperty struct {
	propertyCommon
	TypeName string `json:"type"`
}

const TypeP_BMM_SINGLE_PROPERTY = "P_BMM_SINGLE_PROPERTY"

func (*SingleProperty) isProperty() {}

// MarshalJSON emits _type plus the embedded common fields and "type".
func (s *SingleProperty) MarshalJSON() ([]byte, error) {
	return marshalDiscriminated(TypeP_BMM_SINGLE_PROPERTY, s.propertyCommon, map[string]any{
		"type": s.TypeName,
	})
}

// SinglePropertyOpen is P_BMM_SINGLE_PROPERTY_OPEN: like SingleProperty
// but the type name refers to an open generic parameter on the
// enclosing class (e.g. "T" on a generic Interval[T]).
type SinglePropertyOpen struct {
	propertyCommon
	TypeName string `json:"type"`
}

const TypeP_BMM_SINGLE_PROPERTY_OPEN = "P_BMM_SINGLE_PROPERTY_OPEN"

func (*SinglePropertyOpen) isProperty() {}

func (s *SinglePropertyOpen) MarshalJSON() ([]byte, error) {
	return marshalDiscriminated(TypeP_BMM_SINGLE_PROPERTY_OPEN, s.propertyCommon, map[string]any{
		"type": s.TypeName,
	})
}

// ContainerProperty is P_BMM_CONTAINER_PROPERTY:
//
//	{ type_def: <ContainerType-shape>, cardinality: {lower, upper?, upper_unbounded?} }
//
// Note: type_def in this position does NOT carry its own _type discriminator.
type ContainerProperty struct {
	propertyCommon
	TypeDef     *ContainerType `json:"type_def"`
	Cardinality *Cardinality   `json:"cardinality,omitempty"`
}

const TypeP_BMM_CONTAINER_PROPERTY = "P_BMM_CONTAINER_PROPERTY"

func (*ContainerProperty) isProperty() {}

func (c *ContainerProperty) MarshalJSON() ([]byte, error) {
	extra := map[string]any{}
	if c.TypeDef != nil {
		extra["type_def"] = containerTypeAsNoDiscWrapper{c.TypeDef}
	}
	if c.Cardinality != nil {
		extra["cardinality"] = c.Cardinality
	}
	return marshalDiscriminated(TypeP_BMM_CONTAINER_PROPERTY, c.propertyCommon, extra)
}

// GenericProperty is P_BMM_GENERIC_PROPERTY: { type_def: <GenericType-shape> }.
// type_def in this position does NOT carry its own _type discriminator.
type GenericProperty struct {
	propertyCommon
	TypeDef *GenericType `json:"type_def"`
}

const TypeP_BMM_GENERIC_PROPERTY = "P_BMM_GENERIC_PROPERTY"

func (*GenericProperty) isProperty() {}

func (g *GenericProperty) MarshalJSON() ([]byte, error) {
	extra := map[string]any{}
	if g.TypeDef != nil {
		extra["type_def"] = genericTypeAsNoDiscWrapper{g.TypeDef}
	}
	return marshalDiscriminated(TypeP_BMM_GENERIC_PROPERTY, g.propertyCommon, extra)
}

// containerTypeAsNoDiscWrapper marshals a *ContainerType WITHOUT its
// _type discriminator — the form expected inside a ContainerProperty's
// type_def.
type containerTypeAsNoDiscWrapper struct{ ct *ContainerType }

func (w containerTypeAsNoDiscWrapper) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	ct, err := json.Marshal(w.ct.ContainerType)
	if err != nil {
		return nil, err
	}
	buf.WriteString(`"container_type":`)
	buf.Write(ct)
	if w.ct.TypeDef != nil {
		td, err := json.Marshal(w.ct.TypeDef)
		if err != nil {
			return nil, err
		}
		buf.WriteString(`,"type_def":`)
		buf.Write(td)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// genericTypeAsNoDiscWrapper marshals a *GenericType WITHOUT its _type
// discriminator — the form expected inside a GenericProperty's type_def.
type genericTypeAsNoDiscWrapper struct{ gt *GenericType }

func (w genericTypeAsNoDiscWrapper) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	rt, err := json.Marshal(w.gt.RootType)
	if err != nil {
		return nil, err
	}
	buf.WriteString(`"root_type":`)
	buf.Write(rt)
	if len(w.gt.GenericParameters) > 0 {
		buf.WriteString(`,"generic_parameters":`)
		if err := marshalGenericParameters(&buf, w.gt.GenericParameters); err != nil {
			return nil, err
		}
	}
	if len(w.gt.GenericParameterDefs) > 0 {
		buf.WriteString(`,"generic_parameter_defs":`)
		if err := marshalTypeMap(&buf, w.gt.GenericParameterDefs); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// decodeProperty dispatches on _type and returns the concrete impl.
func decodeProperty(raw json.RawMessage, path string) (Property, error) {
	var head struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, fmt.Errorf("decode Property at %s: %w", path, err)
	}
	switch head.Type {
	case TypeP_BMM_SINGLE_PROPERTY:
		return decodeSingleProperty(raw, path)
	case TypeP_BMM_SINGLE_PROPERTY_OPEN:
		return decodeSinglePropertyOpen(raw, path)
	case TypeP_BMM_CONTAINER_PROPERTY:
		return decodeContainerProperty(raw, path)
	case TypeP_BMM_GENERIC_PROPERTY:
		return decodeGenericProperty(raw, path)
	default:
		return nil, &unknownTypeError{Discriminator: head.Type, Path: path}
	}
}

func decodeSingleProperty(raw json.RawMessage, path string) (*SingleProperty, error) {
	var p SingleProperty
	if err := strictUnmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("decode SingleProperty at %s: %w", path, err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("%w: SingleProperty.name at %s", ErrMissingField, path)
	}
	if p.TypeName == "" {
		return nil, fmt.Errorf("%w: SingleProperty.type at %s (%s)", ErrMissingField, path, p.Name)
	}
	return &p, nil
}

func decodeSinglePropertyOpen(raw json.RawMessage, path string) (*SinglePropertyOpen, error) {
	var p SinglePropertyOpen
	if err := strictUnmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("decode SinglePropertyOpen at %s: %w", path, err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("%w: SinglePropertyOpen.name at %s", ErrMissingField, path)
	}
	if p.TypeName == "" {
		return nil, fmt.Errorf("%w: SinglePropertyOpen.type at %s (%s)", ErrMissingField, path, p.Name)
	}
	return &p, nil
}

func decodeContainerProperty(raw json.RawMessage, path string) (*ContainerProperty, error) {
	var aux struct {
		propertyCommon
		TypeDef     json.RawMessage `json:"type_def"`
		Cardinality *Cardinality    `json:"cardinality,omitempty"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return nil, fmt.Errorf("decode ContainerProperty at %s: %w", path, err)
	}
	if aux.Name == "" {
		return nil, fmt.Errorf("%w: ContainerProperty.name at %s", ErrMissingField, path)
	}
	if len(aux.TypeDef) == 0 {
		return nil, fmt.Errorf("%w: ContainerProperty.type_def at %s (%s)", ErrMissingField, path, aux.Name)
	}
	td, err := decodeTypeNoDisc(aux.TypeDef, "container", path+".type_def")
	if err != nil {
		return nil, err
	}
	ct, ok := td.(*ContainerType)
	if !ok {
		return nil, fmt.Errorf("%w: ContainerProperty.type_def expected ContainerType at %s", ErrInvalidShape, path)
	}
	return &ContainerProperty{
		propertyCommon: aux.propertyCommon,
		TypeDef:        ct,
		Cardinality:    aux.Cardinality,
	}, nil
}

func decodeGenericProperty(raw json.RawMessage, path string) (*GenericProperty, error) {
	var aux struct {
		propertyCommon
		TypeDef json.RawMessage `json:"type_def"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return nil, fmt.Errorf("decode GenericProperty at %s: %w", path, err)
	}
	if aux.Name == "" {
		return nil, fmt.Errorf("%w: GenericProperty.name at %s", ErrMissingField, path)
	}
	if len(aux.TypeDef) == 0 {
		return nil, fmt.Errorf("%w: GenericProperty.type_def at %s (%s)", ErrMissingField, path, aux.Name)
	}
	td, err := decodeTypeNoDisc(aux.TypeDef, "generic", path+".type_def")
	if err != nil {
		return nil, err
	}
	gt, ok := td.(*GenericType)
	if !ok {
		return nil, fmt.Errorf("%w: GenericProperty.type_def expected GenericType at %s", ErrInvalidShape, path)
	}
	return &GenericProperty{
		propertyCommon: aux.propertyCommon,
		TypeDef:        gt,
	}, nil
}

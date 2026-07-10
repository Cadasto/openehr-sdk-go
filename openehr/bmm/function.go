package bmm

import (
	"encoding/json"
	"fmt"
)

// FunctionParameter is the abstract category of P_BMM_*_FUNCTION_PARAMETER
// values. Four concrete variants, parallel to the Property hierarchy.
type FunctionParameter interface {
	isFunctionParameter()
	// FunctionParameterName returns the parameter's BMM name.
	FunctionParameterName() string
}

// functionParameterCommon holds fields shared by all four
// FunctionParameter variants.
type functionParameterCommon struct {
	Name          string `json:"name"`
	Documentation string `json:"documentation,omitempty"`
}

// FunctionParameterName implements FunctionParameter.
func (p functionParameterCommon) FunctionParameterName() string { return p.Name }

// SingleFunctionParameter is P_BMM_SINGLE_FUNCTION_PARAMETER —
// structurally like SingleProperty.
type SingleFunctionParameter struct {
	functionParameterCommon
	TypeName string `json:"type"`
}

// TypeP_BMM_SINGLE_FUNCTION_PARAMETER is the SingleFunctionParameter
// _type discriminator value.
const TypeP_BMM_SINGLE_FUNCTION_PARAMETER = "P_BMM_SINGLE_FUNCTION_PARAMETER"

func (*SingleFunctionParameter) isFunctionParameter() {}

// MarshalJSON implements [encoding/json.Marshaler] for the
// P_BMM_SINGLE_FUNCTION_PARAMETER representation.
func (s *SingleFunctionParameter) MarshalJSON() ([]byte, error) {
	return marshalDiscriminated(TypeP_BMM_SINGLE_FUNCTION_PARAMETER, s.functionParameterCommon, map[string]any{
		"type": s.TypeName,
	})
}

// SingleFunctionParameterOpen is P_BMM_SINGLE_FUNCTION_PARAMETER_OPEN —
// parallels SinglePropertyOpen.
type SingleFunctionParameterOpen struct {
	functionParameterCommon
	TypeName string `json:"type"`
}

// TypeP_BMM_SINGLE_FUNCTION_PARAMETER_OPEN is the
// SingleFunctionParameterOpen _type discriminator value.
const TypeP_BMM_SINGLE_FUNCTION_PARAMETER_OPEN = "P_BMM_SINGLE_FUNCTION_PARAMETER_OPEN"

func (*SingleFunctionParameterOpen) isFunctionParameter() {}

// MarshalJSON implements [encoding/json.Marshaler] for the
// P_BMM_SINGLE_FUNCTION_PARAMETER_OPEN representation.
func (s *SingleFunctionParameterOpen) MarshalJSON() ([]byte, error) {
	return marshalDiscriminated(TypeP_BMM_SINGLE_FUNCTION_PARAMETER_OPEN, s.functionParameterCommon, map[string]any{
		"type": s.TypeName,
	})
}

// ContainerFunctionParameter is P_BMM_CONTAINER_FUNCTION_PARAMETER —
// parallels ContainerProperty.
type ContainerFunctionParameter struct {
	functionParameterCommon
	TypeDef     *ContainerType `json:"type_def"`
	Cardinality *Cardinality   `json:"cardinality,omitempty"`
}

// TypeP_BMM_CONTAINER_FUNCTION_PARAMETER is the
// ContainerFunctionParameter _type discriminator value.
const TypeP_BMM_CONTAINER_FUNCTION_PARAMETER = "P_BMM_CONTAINER_FUNCTION_PARAMETER"

func (*ContainerFunctionParameter) isFunctionParameter() {}

// MarshalJSON implements [encoding/json.Marshaler] for the
// P_BMM_CONTAINER_FUNCTION_PARAMETER representation.
func (c *ContainerFunctionParameter) MarshalJSON() ([]byte, error) {
	extra := map[string]any{}
	if c.TypeDef != nil {
		extra["type_def"] = containerTypeAsNoDiscWrapper{c.TypeDef}
	}
	if c.Cardinality != nil {
		extra["cardinality"] = c.Cardinality
	}
	return marshalDiscriminated(TypeP_BMM_CONTAINER_FUNCTION_PARAMETER, c.functionParameterCommon, extra)
}

// GenericFunctionParameter is P_BMM_GENERIC_FUNCTION_PARAMETER —
// parallels GenericProperty.
type GenericFunctionParameter struct {
	functionParameterCommon
	TypeDef *GenericType `json:"type_def"`
}

// TypeP_BMM_GENERIC_FUNCTION_PARAMETER is the GenericFunctionParameter
// _type discriminator value.
const TypeP_BMM_GENERIC_FUNCTION_PARAMETER = "P_BMM_GENERIC_FUNCTION_PARAMETER"

func (*GenericFunctionParameter) isFunctionParameter() {}

// MarshalJSON implements [encoding/json.Marshaler] for the
// P_BMM_GENERIC_FUNCTION_PARAMETER representation.
func (g *GenericFunctionParameter) MarshalJSON() ([]byte, error) {
	extra := map[string]any{}
	if g.TypeDef != nil {
		extra["type_def"] = genericTypeAsNoDiscWrapper{g.TypeDef}
	}
	return marshalDiscriminated(TypeP_BMM_GENERIC_FUNCTION_PARAMETER, g.functionParameterCommon, extra)
}

// Function captures a P_BMM function definition (operation / query)
// hanging off a Class. The Parameters map is keyed by parameter name;
// the same name appears in the FunctionParameter's Name field. Result
// is the polymorphic return-type definition.
type Function struct {
	Name           string                       `json:"name"`
	Documentation  string                       `json:"documentation,omitempty"`
	IsAbstract     bool                         `json:"is_abstract,omitempty"`
	Aliases        []string                     `json:"aliases,omitempty"`
	Parameters     map[string]FunctionParameter `json:"parameters,omitempty"`
	Result         Type                         `json:"result,omitempty"`
	PreConditions  map[string]string            `json:"pre_conditions,omitempty"`
	PostConditions map[string]string            `json:"post_conditions,omitempty"`
}

// MarshalJSON serialises Function so the polymorphic Parameters values
// and Result emit their _type discriminators (relying on the concrete
// MarshalJSON methods). Output is stable: encoding/json sorts map keys,
// so the assembled object's keys — and the nested parameters/condition
// map keys — emit in lexicographic order on every call.
func (f *Function) MarshalJSON() ([]byte, error) {
	m := map[string]any{
		"name": f.Name,
	}
	if f.Documentation != "" {
		m["documentation"] = f.Documentation
	}
	if f.IsAbstract {
		m["is_abstract"] = true
	}
	if len(f.Aliases) > 0 {
		m["aliases"] = f.Aliases
	}
	if len(f.Parameters) > 0 {
		m["parameters"] = f.Parameters
	}
	if f.Result != nil {
		m["result"] = f.Result
	}
	if len(f.PreConditions) > 0 {
		m["pre_conditions"] = f.PreConditions
	}
	if len(f.PostConditions) > 0 {
		m["post_conditions"] = f.PostConditions
	}
	return json.Marshal(m)
}

func decodeFunctionParameter(raw json.RawMessage, path string) (FunctionParameter, error) {
	var head struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, fmt.Errorf("decode FunctionParameter at %s: %w", path, err)
	}
	switch head.Type {
	case TypeP_BMM_SINGLE_FUNCTION_PARAMETER:
		var p SingleFunctionParameter
		if err := strictUnmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode SingleFunctionParameter at %s: %w", path, err)
		}
		if p.Name == "" {
			return nil, fmt.Errorf("%w: SingleFunctionParameter.name at %s", ErrMissingField, path)
		}
		return &p, nil
	case TypeP_BMM_SINGLE_FUNCTION_PARAMETER_OPEN:
		var p SingleFunctionParameterOpen
		if err := strictUnmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode SingleFunctionParameterOpen at %s: %w", path, err)
		}
		if p.Name == "" {
			return nil, fmt.Errorf("%w: SingleFunctionParameterOpen.name at %s", ErrMissingField, path)
		}
		return &p, nil
	case TypeP_BMM_CONTAINER_FUNCTION_PARAMETER:
		var aux struct {
			functionParameterCommon
			TypeDef     json.RawMessage `json:"type_def"`
			Cardinality *Cardinality    `json:"cardinality,omitempty"`
		}
		if err := json.Unmarshal(raw, &aux); err != nil {
			return nil, fmt.Errorf("decode ContainerFunctionParameter at %s: %w", path, err)
		}
		if aux.Name == "" {
			return nil, fmt.Errorf("%w: ContainerFunctionParameter.name at %s", ErrMissingField, path)
		}
		td, err := decodeTypeNoDisc(aux.TypeDef, "container", path+".type_def")
		if err != nil {
			return nil, err
		}
		ct, ok := td.(*ContainerType)
		if !ok {
			return nil, fmt.Errorf("%w: ContainerFunctionParameter.type_def expected ContainerType at %s", ErrInvalidShape, path)
		}
		return &ContainerFunctionParameter{
			functionParameterCommon: aux.functionParameterCommon,
			TypeDef:                 ct,
			Cardinality:             aux.Cardinality,
		}, nil
	case TypeP_BMM_GENERIC_FUNCTION_PARAMETER:
		var aux struct {
			functionParameterCommon
			TypeDef json.RawMessage `json:"type_def"`
		}
		if err := json.Unmarshal(raw, &aux); err != nil {
			return nil, fmt.Errorf("decode GenericFunctionParameter at %s: %w", path, err)
		}
		if aux.Name == "" {
			return nil, fmt.Errorf("%w: GenericFunctionParameter.name at %s", ErrMissingField, path)
		}
		td, err := decodeTypeNoDisc(aux.TypeDef, "generic", path+".type_def")
		if err != nil {
			return nil, err
		}
		gt, ok := td.(*GenericType)
		if !ok {
			return nil, fmt.Errorf("%w: GenericFunctionParameter.type_def expected GenericType at %s", ErrInvalidShape, path)
		}
		return &GenericFunctionParameter{
			functionParameterCommon: aux.functionParameterCommon,
			TypeDef:                 gt,
		}, nil
	default:
		return nil, &unknownTypeError{Discriminator: head.Type, Path: path}
	}
}

func decodeFunction(raw json.RawMessage, path string) (*Function, error) {
	var aux struct {
		Name           string                     `json:"name"`
		Documentation  string                     `json:"documentation"`
		IsAbstract     bool                       `json:"is_abstract"`
		Aliases        []string                   `json:"aliases"`
		Parameters     map[string]json.RawMessage `json:"parameters"`
		Result         json.RawMessage            `json:"result"`
		PreConditions  map[string]string          `json:"pre_conditions"`
		PostConditions map[string]string          `json:"post_conditions"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return nil, fmt.Errorf("decode Function at %s: %w", path, err)
	}
	if aux.Name == "" {
		return nil, fmt.Errorf("%w: Function.name at %s", ErrMissingField, path)
	}
	fn := &Function{
		Name:           aux.Name,
		Documentation:  aux.Documentation,
		IsAbstract:     aux.IsAbstract,
		Aliases:        aux.Aliases,
		PreConditions:  aux.PreConditions,
		PostConditions: aux.PostConditions,
	}
	if len(aux.Parameters) > 0 {
		fn.Parameters = make(map[string]FunctionParameter, len(aux.Parameters))
		for k, v := range aux.Parameters {
			p, err := decodeFunctionParameter(v, path+".parameters."+k)
			if err != nil {
				return nil, err
			}
			fn.Parameters[k] = p
		}
	}
	if len(aux.Result) > 0 && !isJSONNull(aux.Result) {
		r, err := decodeType(aux.Result, path+".result")
		if err != nil {
			return nil, err
		}
		fn.Result = r
	}
	return fn, nil
}

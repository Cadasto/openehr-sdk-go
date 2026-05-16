package bmm

import (
	"encoding/json"
	"fmt"
)

// Class is the abstract category for all class-like entries in a BMM
// schema's class_definitions or primitive_types map. The four concrete
// variants are: SimpleClass (the default — struct-like; may also be a
// "generic class" if it carries generic parameter defs), Enumeration
// (string or integer item codes), and Interface.
type Class interface {
	isClass()
	// ClassName returns the class's BMM name.
	ClassName() string
	// IsAbstract returns true for abstract classes (cannot be
	// instantiated directly).
	IsAbstract() bool
	// Ancestors returns the names of parent classes (empty if none).
	Ancestors() []string
	// Documentation returns the BMM documentation string.
	Documentation() string
}

// GenericParameterDef is a class-level open generic parameter
// (e.g. K, V on Hash<K, V>). conforms_to_type is the optional upper
// bound — a class or primitive name the parameter must conform to.
type GenericParameterDef struct {
	Name           string `json:"name"`
	ConformsToType string `json:"conforms_to_type,omitempty"`
}

// classCommon is the embedded common ancestor for every concrete Class
// variant. Captures the fields that all class kinds share.
type classCommon struct {
	Name           string              `json:"name"`
	Doc            string              `json:"documentation,omitempty"`
	Ancestors_     []string            `json:"ancestors,omitempty"`
	IsAbstractFlag bool                `json:"is_abstract,omitempty"`
	Properties     map[string]Property `json:"properties,omitempty"`
	// PropertyOrder lists property names in BMM JSON declaration order
	// (the key order of the properties object in the source file).
	PropertyOrder []string             `json:"-"`
	Functions     map[string]*Function `json:"functions,omitempty"`
	Invariants    map[string]string    `json:"invariants,omitempty"`
}

// ClassName implements Class.
func (c *classCommon) ClassName() string { return c.Name }

// IsAbstract implements Class.
func (c *classCommon) IsAbstract() bool { return c.IsAbstractFlag }

// Ancestors implements Class.
func (c *classCommon) Ancestors() []string { return c.Ancestors_ }

// Documentation implements Class.
func (c *classCommon) Documentation() string { return c.Doc }

// SimpleClass is the default class kind (no _type discriminator on the
// JSON side). It also covers the "generic class" sub-kind, which is
// distinguished only by the presence of GenericParameterDefs.
type SimpleClass struct {
	classCommon
	// GenericParameterDefs is non-empty for a generic class
	// (e.g. Hash, Interval, FUNCTION).
	GenericParameterDefs map[string]*GenericParameterDef `json:"generic_parameter_defs,omitempty"`
}

// IsGeneric returns true if the class carries class-level generic
// parameter definitions (i.e. is a generic class).
func (s *SimpleClass) IsGeneric() bool { return len(s.GenericParameterDefs) > 0 }

func (*SimpleClass) isClass() {}

// MarshalJSON emits the class fields without a _type discriminator
// (matching the input shape for the default class).
func (s *SimpleClass) MarshalJSON() ([]byte, error) {
	return marshalClassObject("", &s.classCommon, map[string]any{
		"generic_parameter_defs": optMap(s.GenericParameterDefs),
	})
}

// Interface is P_BMM_INTERFACE — structurally similar to SimpleClass
// but rendered as a Go interface (its functions become method
// signatures).
type Interface struct {
	classCommon
	GenericParameterDefs map[string]*GenericParameterDef `json:"generic_parameter_defs,omitempty"`
}

// IsGeneric returns true if the interface carries generic parameter
// definitions.
func (i *Interface) IsGeneric() bool { return len(i.GenericParameterDefs) > 0 }

const TypeP_BMM_INTERFACE = "P_BMM_INTERFACE"

func (*Interface) isClass() {}

func (i *Interface) MarshalJSON() ([]byte, error) {
	return marshalClassObject(TypeP_BMM_INTERFACE, &i.classCommon, map[string]any{
		"generic_parameter_defs": optMap(i.GenericParameterDefs),
	})
}

// Enumeration is the common shape behind both P_BMM_ENUMERATION_STRING
// and P_BMM_ENUMERATION_INTEGER. The item codes are stored as a slice
// of either string (P_BMM_ENUMERATION_STRING — codes = item_names) or
// int64 (P_BMM_ENUMERATION_INTEGER — codes = item_values).
//
// EnumKind discriminates the two variants on emit; the actual values
// live in either ItemValuesString or ItemValuesInt — whichever matches.
type Enumeration struct {
	classCommon
	// EnumKind is either TypeP_BMM_ENUMERATION_STRING or
	// TypeP_BMM_ENUMERATION_INTEGER.
	EnumKind           string   `json:"-"`
	ItemNames          []string `json:"item_names"`
	ItemDocumentations []string `json:"item_documentations,omitempty"`
	ItemValuesString   []string `json:"-"`
	ItemValuesInt      []int64  `json:"-"`
}

const (
	TypeP_BMM_ENUMERATION_STRING  = "P_BMM_ENUMERATION_STRING"
	TypeP_BMM_ENUMERATION_INTEGER = "P_BMM_ENUMERATION_INTEGER"
)

func (*Enumeration) isClass() {}

// IsStringEnum reports whether this is a P_BMM_ENUMERATION_STRING.
func (e *Enumeration) IsStringEnum() bool { return e.EnumKind == TypeP_BMM_ENUMERATION_STRING }

// IsIntegerEnum reports whether this is a P_BMM_ENUMERATION_INTEGER.
func (e *Enumeration) IsIntegerEnum() bool { return e.EnumKind == TypeP_BMM_ENUMERATION_INTEGER }

func (e *Enumeration) MarshalJSON() ([]byte, error) {
	extra := map[string]any{
		"item_names": e.ItemNames,
	}
	if len(e.ItemDocumentations) > 0 {
		extra["item_documentations"] = e.ItemDocumentations
	}
	if e.IsIntegerEnum() {
		extra["item_values"] = e.ItemValuesInt
	}
	return marshalClassObject(e.EnumKind, &e.classCommon, extra)
}

// decodeClass dispatches on _type. Returns the concrete impl plus the
// concrete kind string for diagnostics. The absence of _type defaults
// to SimpleClass; SimpleClass auto-detects whether it is a generic
// class via the presence of generic_parameter_defs.
func decodeClass(raw json.RawMessage, path string) (Class, error) {
	var head struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, fmt.Errorf("decode Class at %s: %w", path, err)
	}
	switch head.Type {
	case "":
		return decodeSimpleClass(raw, path)
	case TypeP_BMM_INTERFACE:
		return decodeInterface(raw, path)
	case TypeP_BMM_ENUMERATION_STRING:
		return decodeEnumeration(raw, TypeP_BMM_ENUMERATION_STRING, path)
	case TypeP_BMM_ENUMERATION_INTEGER:
		return decodeEnumeration(raw, TypeP_BMM_ENUMERATION_INTEGER, path)
	default:
		return nil, &unknownTypeError{Discriminator: head.Type, Path: path}
	}
}

func decodeClassCommon(raw json.RawMessage, c *classCommon, path string) error {
	var aux struct {
		Name          string                     `json:"name"`
		Documentation string                     `json:"documentation"`
		Ancestors     []string                   `json:"ancestors"`
		IsAbstract    bool                       `json:"is_abstract"`
		Properties    map[string]json.RawMessage `json:"properties"`
		Functions     map[string]json.RawMessage `json:"functions"`
		Invariants    map[string]string          `json:"invariants"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return fmt.Errorf("decode Class common at %s: %w", path, err)
	}
	if aux.Name == "" {
		return fmt.Errorf("%w: Class.name at %s", ErrMissingField, path)
	}
	c.Name = aux.Name
	c.Doc = aux.Documentation
	c.Ancestors_ = aux.Ancestors
	c.IsAbstractFlag = aux.IsAbstract
	c.Invariants = aux.Invariants
	propOrder, err := orderedJSONObjectKeysFromClass(raw, "properties")
	if err != nil {
		return fmt.Errorf("decode Class properties order at %s: %w", path, err)
	}
	c.PropertyOrder = propOrder
	if len(aux.Properties) > 0 {
		c.Properties = make(map[string]Property, len(aux.Properties))
		for _, k := range propOrder {
			v := aux.Properties[k]
			p, err := decodeProperty(v, path+".properties."+k)
			if err != nil {
				return err
			}
			c.Properties[k] = p
		}
		// Keys present in the map but missing from PropertyOrder (should
		// not happen with a well-formed decoder path) — append sorted.
		for k, v := range aux.Properties {
			if _, ok := c.Properties[k]; ok {
				continue
			}
			p, err := decodeProperty(v, path+".properties."+k)
			if err != nil {
				return err
			}
			c.Properties[k] = p
			c.PropertyOrder = append(c.PropertyOrder, k)
		}
	}
	if len(aux.Functions) > 0 {
		c.Functions = make(map[string]*Function, len(aux.Functions))
		for k, v := range aux.Functions {
			f, err := decodeFunction(v, path+".functions."+k)
			if err != nil {
				return err
			}
			c.Functions[k] = f
		}
	}
	return nil
}

func decodeSimpleClass(raw json.RawMessage, path string) (*SimpleClass, error) {
	c := &SimpleClass{}
	if err := decodeClassCommon(raw, &c.classCommon, path); err != nil {
		return nil, err
	}
	var aux struct {
		GenericParameterDefs map[string]*GenericParameterDef `json:"generic_parameter_defs"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return nil, fmt.Errorf("decode SimpleClass at %s: %w", path, err)
	}
	c.GenericParameterDefs = aux.GenericParameterDefs
	return c, nil
}

func decodeInterface(raw json.RawMessage, path string) (*Interface, error) {
	i := &Interface{}
	if err := decodeClassCommon(raw, &i.classCommon, path); err != nil {
		return nil, err
	}
	var aux struct {
		GenericParameterDefs map[string]*GenericParameterDef `json:"generic_parameter_defs"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return nil, fmt.Errorf("decode Interface at %s: %w", path, err)
	}
	i.GenericParameterDefs = aux.GenericParameterDefs
	return i, nil
}

func decodeEnumeration(raw json.RawMessage, kind, path string) (*Enumeration, error) {
	e := &Enumeration{EnumKind: kind}
	if err := decodeClassCommon(raw, &e.classCommon, path); err != nil {
		return nil, err
	}
	var aux struct {
		ItemNames          []string          `json:"item_names"`
		ItemDocumentations []string          `json:"item_documentations"`
		ItemValues         []json.RawMessage `json:"item_values"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return nil, fmt.Errorf("decode Enumeration at %s: %w", path, err)
	}
	if len(aux.ItemNames) == 0 {
		return nil, fmt.Errorf("%w: Enumeration.item_names at %s (%s)", ErrMissingField, path, e.Name)
	}
	e.ItemNames = aux.ItemNames
	e.ItemDocumentations = aux.ItemDocumentations
	switch kind {
	case TypeP_BMM_ENUMERATION_STRING:
		// item_values absent — values default to item_names.
		e.ItemValuesString = append([]string{}, aux.ItemNames...)
	case TypeP_BMM_ENUMERATION_INTEGER:
		if len(aux.ItemValues) > 0 {
			e.ItemValuesInt = make([]int64, len(aux.ItemValues))
			for i, v := range aux.ItemValues {
				var n int64
				if err := json.Unmarshal(v, &n); err != nil {
					return nil, fmt.Errorf("decode Enumeration.item_values[%d] at %s: %w", i, path, err)
				}
				e.ItemValuesInt[i] = n
			}
		}
	}
	return e, nil
}

package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// RenderUnmarshalJSONFile renders the canonical-JSON UnmarshalJSONFrom
// companions (encoding/json/v2, ADR 0022) for every concrete class in `file`.
//
// Returns (nil, nil) when the file has no concrete classes.
//
// # Strategy (ruling R19)
//
// Each method refuses a nil receiver (REQ-025), then hands the decoder to the
// shared [typereg.DecodeInto] helper, which reads the value, enforces the
// `_type` discipline, threads the polymorphic decode hooks and classifies a
// shape failure. The decode target is the receiver viewed through its
// method-free alias (zero-copy) for most classes, or a flat wire struct copied
// back field by field for a class that embeds a marshaler-bearing concrete
// ancestor (see the promotion note at [effectiveFields]). Polymorphic interface
// fields resolve through the registered hooks — there is no per-field
// typereg.DecodeAs dispatch or json.RawMessage staging any more.
func RenderUnmarshalJSONFile(plan *Plan, file *PlannedFile) ([]byte, error) {
	emitting := concreteClassesIn(file)
	if len(emitting) == 0 {
		return nil, nil
	}

	chunks := make([]string, 0, len(emitting))
	for _, pc := range emitting {
		fields, err := effectiveFields(plan, pc)
		if err != nil {
			return nil, err
		}
		chunk, err := renderUnmarshalJSON(plan, pc, fields)
		if err != nil {
			return nil, fmt.Errorf("render UnmarshalJSONFrom %s: %w", pc.BMMName, err)
		}
		chunks = append(chunks, chunk)
	}

	var body bytes.Buffer
	body.WriteString(renderGeneratedHeader(plan))
	body.WriteString("\n")
	body.WriteString("import (\n")
	body.WriteString("\t\"encoding/json/jsontext\"\n")
	body.WriteString("\t\"fmt\"\n\n")
	fmt.Fprintf(&body, "\t%q\n", typeregImportPath)
	if needsExternalImportForJSONMar(plan, chunks) {
		fmt.Fprintf(&body, "\t%q\n", plan.Target.ExternalImport)
	}
	body.WriteString(")\n\n")

	if file.PackagePath != "" {
		fmt.Fprintf(&body, "// BMM package: %s — canonical-JSON UnmarshalJSONFrom companions\n\n", file.PackagePath)
	} else {
		body.WriteString("// canonical-JSON UnmarshalJSONFrom companions (foundation classes)\n\n")
	}

	for _, c := range chunks {
		body.WriteString(c)
		body.WriteString("\n")
	}

	formatted, err := format.Source(body.Bytes())
	if err != nil {
		return body.Bytes(), fmt.Errorf("gofmt %s_jsonunmar_gen.go: %w", file.FileBase, err)
	}
	return formatted, nil
}

// renderUnmarshalJSON emits the UnmarshalJSONFrom method for a single concrete
// class. The wire type it decodes into is defined in the sibling
// _jsonmar_gen.go: a method-free alias (zero-copy) or a flat wire struct
// (copied back), chosen by [embedsMarshalerBearingConcrete].
func renderUnmarshalJSON(plan *Plan, pc *PlannedClass, fields []emittedField) (string, error) {
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return "", fmt.Errorf("expected SimpleClass for %s, got %T", pc.BMMName, pc.Class)
	}
	recv := jsonmarReceiverName(pc.GoName)
	typeArgs := ""
	if sc.IsGeneric() {
		typeArgs = genericTypeArgList(sc)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "// UnmarshalJSONFrom decodes canonical openEHR JSON into %s.\n", pc.GoName)
	b.WriteString("// A nil receiver is refused with typereg.ErrNilReceiver rather than\n")
	b.WriteString("// dereferenced (REQ-025). The shared helper checks the `_type`\n")
	b.WriteString("// discriminator, threads the polymorphic decode hooks so every nested\n")
	b.WriteString("// slot resolves, and wraps a whole-value shape failure through\n")
	b.WriteString("// typereg.WrapShapeError — keeping the `canjson: <RM_TYPE>:` text and\n")
	b.WriteString("// adding typereg.ErrInvalidShape (REQ-052, ADR 0022).\n")
	fmt.Fprintf(&b, "func (%s *%s%s) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n", recv, pc.GoName, typeArgs)
	fmt.Fprintf(&b, "\tif %s == nil {\n", recv)
	fmt.Fprintf(&b, "\t\treturn fmt.Errorf(\"canjson: %s: %%w\", typereg.ErrNilReceiver)\n", pc.BMMName)
	b.WriteString("\t}\n")

	if embedsMarshalerBearingConcrete(plan, pc) {
		wire := flatWireTypeName(pc.GoName)
		fmt.Fprintf(&b, "\tvar wire %s%s\n", wire, typeArgs)
		fmt.Fprintf(&b, "\tif err := typereg.DecodeInto(dec, %q, &wire); err != nil {\n", pc.BMMName)
		b.WriteString("\t\treturn err\n")
		b.WriteString("\t}\n")
		for _, ef := range fields {
			fn := FieldName(ef.Prop.PropertyName())
			fmt.Fprintf(&b, "\t%s.%s = wire.%s\n", recv, fn, fn)
		}
		b.WriteString("\treturn nil\n")
	} else {
		alias := aliasTypeName(pc.GoName)
		fmt.Fprintf(&b, "\treturn typereg.DecodeInto(dec, %q, &struct {\n", pc.BMMName)
		b.WriteString("\t\tType string `json:\"_type\"`\n")
		fmt.Fprintf(&b, "\t\t*%s%s\n", alias, typeArgs)
		fmt.Fprintf(&b, "\t}{%s: (*%s%s)(%s)})\n", alias, alias, typeArgs, recv)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// polyKind enumerates the polymorphism shapes the classifier recognises: a
// single polymorphic value, a container of polymorphic values, or the narrow
// variants (REQ-052) where the declared type is a concrete class with
// registered subtypes and the wire MAY omit the `_type` discriminator.
//
// The classifier drives the per-interface hook enumeration ([polymorphicInterfaces]);
// the decode itself is data-driven through the registered hooks.
type polyKind int

const (
	polyNone polyKind = iota
	polySingle
	polySlice
	polySingleNarrow
	polySliceNarrow
)

// polymorphicProperty inspects a BMM property and returns the Go type name of
// its abstract or narrow element together with a [polyKind] classification. If
// the property is monomorphic, kind == polyNone.
//
// `owner` is the BMM class that declared `prop`. `emitting` is the concrete
// class whose codec we are rendering; it differs from `owner` when `prop` is
// inherited (e.g. DV_INTERVAL inherits `lower: T` from `Interval`). Passing both
// lets the helper resolve open generic parameter constraints from either the
// declaring view or the narrowed emitting view.
func polymorphicProperty(plan *Plan, owner, emitting *bmm.SimpleClass, prop bmm.Property) (string, polyKind) {
	switch p := prop.(type) {
	case *bmm.SingleProperty:
		if name, ok := abstractGoName(plan, p.TypeName); ok {
			return name, polySingle
		}
		if name, ok := narrowInterfaceGoName(plan, p.TypeName); ok {
			return name, polySingleNarrow
		}
	case *bmm.SinglePropertyOpen:
		// Return the generic parameter name (e.g. "T"), not the resolved
		// interface: the XML codec instantiates the field's declared type
		// parameter here. The JSON hook enumeration resolves the bound to its
		// interface separately in [fieldPolyInterface].
		if bound := openBoundType(plan, owner, emitting, p); bound != "" {
			if _, ok := abstractGoName(plan, bound); ok {
				return p.TypeName, polySingle
			}
		}
		return "", polyNone
	case *bmm.ContainerProperty:
		if p.TypeDef == nil || p.TypeDef.TypeDef == nil {
			return "", polyNone
		}
		elemName, narrow := containerElementPolymorphicName(plan, p.TypeDef)
		if elemName != "" {
			switch p.TypeDef.ContainerType {
			case "Hash":
				return "", polyNone
			default:
				if narrow {
					return elemName, polySliceNarrow
				}
				return elemName, polySlice
			}
		}
	case *bmm.GenericProperty:
		// GenericProperty fixes concrete types at this site; the inner value
		// decodes directly.
		return "", polyNone
	}
	return "", polyNone
}

// openBoundType resolves the effective generic bound of an open property to its
// BMM type name (e.g. "DV_ORDERED"), preferring the emitting class's narrowed
// bound over the declaring owner's, then the owner's inherited bound. Returns ""
// when no bound resolves.
func openBoundType(plan *Plan, owner, emitting *bmm.SimpleClass, p *bmm.SinglePropertyOpen) string {
	if emitting != nil && emitting.GenericParameterDefs != nil {
		if def, ok := emitting.GenericParameterDefs[p.TypeName]; ok && def.ConformsToType != "" {
			return def.ConformsToType
		}
	}
	if owner != nil && owner.GenericParameterDefs != nil {
		if def, ok := owner.GenericParameterDefs[p.TypeName]; ok {
			if def.ConformsToType != "" {
				return def.ConformsToType
			}
			return inheritedGenericBound(plan, owner, p.TypeName)
		}
	}
	return ""
}

// containerElementPolymorphicName distinguishes abstract container elements
// (`narrow == false`) from narrow-interface container elements
// (`narrow == true`).
func containerElementPolymorphicName(plan *Plan, td *bmm.ContainerType) (string, bool) {
	if td == nil || td.TypeDef == nil {
		return "", false
	}
	switch inner := td.TypeDef.(type) {
	case *bmm.SimpleType:
		if name, ok := abstractGoName(plan, inner.TypeName); ok {
			return name, false
		}
		if name, ok := narrowInterfaceGoName(plan, inner.TypeName); ok {
			return name, true
		}
	case *bmm.GenericType:
		if name, ok := abstractGoName(plan, inner.RootType); ok {
			return name, false
		}
		if name, ok := narrowInterfaceGoName(plan, inner.RootType); ok {
			return name, true
		}
	}
	return "", false
}

// narrowInterfaceGoName returns the Go interface name (`<GoName>Like`) for a
// concrete BMM class that has registered subtypes per plan.ConcreteSubtypes.
// Returns ("", false) when the type has no registered subtypes.
func narrowInterfaceGoName(plan *Plan, typeName string) (string, bool) {
	if typeName == "" {
		return "", false
	}
	pc, ok := plan.Classes[typeName]
	if !ok {
		return "", false
	}
	sc, isSimple := pc.Class.(*bmm.SimpleClass)
	if !isSimple || sc.IsAbstract() {
		return "", false
	}
	if _, hasKids := plan.ConcreteSubtypes[pc.BMMName]; !hasKids {
		return "", false
	}
	return qualifyClassRef(plan, pc) + "Like", true
}

// abstractGoName returns the Go name of a BMM type if it resolves to an abstract
// class or interface in the plan; ok == false otherwise.
func abstractGoName(plan *Plan, typeName string) (string, bool) {
	if typeName == "" {
		return "", false
	}
	pc, ok := plan.Classes[typeName]
	if !ok {
		return "", false
	}
	switch cls := pc.Class.(type) {
	case *bmm.Interface:
		return qualifyClassRef(plan, pc), true
	case *bmm.SimpleClass:
		if cls.IsAbstract() && (!cls.IsGeneric() || codecPolymorphicAbstractGeneric(plan, pc)) {
			return qualifyClassRef(plan, pc), true
		}
	}
	return "", false
}

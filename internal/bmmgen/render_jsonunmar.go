package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"regexp"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// RenderUnmarshalJSONFile renders the canonical-JSON `UnmarshalJSON`
// companions for every concrete class in `file` — the same set that
// receives a generated MarshalJSON in the marshaller companion.
//
// Returns (nil, nil) when the file has no concrete classes. The
// caller should skip writing such files.
//
// # Strategy (Strategy B from the codec plan)
//
// For each emitting class C, the generator emits:
//
//  1. A package-level `<C>JSONUnmarshaller` wire struct mirroring C's
//     JSON-effective fields except that polymorphic fields are typed
//     `json.RawMessage` (single) or `[]json.RawMessage` (container).
//     Non-polymorphic fields keep their canonical Go types so
//     `encoding/json` populates them directly.
//
//  2. An `UnmarshalJSON([]byte) error` method on `*C` that:
//     - decodes `data` into the wire struct via `json.Unmarshal`;
//     - copies non-polymorphic fields into the receiver;
//     - for each polymorphic field, dispatches the raw bytes through
//     [typereg.DecodeAs[T]] and stores the concrete value;
//     - wraps any typereg sentinel into a [typereg.DecodeError] with
//     a JSON-pointer-ish path so callers can `errors.As` for the
//     location AND `errors.Is` for the sentinel.
func RenderUnmarshalJSONFile(plan *Plan, file *PlannedFile) ([]byte, error) {
	emitting := concreteClassesIn(file)
	if len(emitting) == 0 {
		return nil, nil
	}
	classFields := make(map[string][]emittedField, len(emitting))
	for _, pc := range emitting {
		fields, err := effectiveFields(plan, pc)
		if err != nil {
			return nil, err
		}
		classFields[pc.BMMName] = fields
	}

	var body bytes.Buffer
	body.WriteString(renderGeneratedHeader(plan))
	body.WriteString("\n")

	// Pre-render to decide whether the cross-target import is needed.
	chunks := make([]string, 0, len(emitting))
	for _, pc := range emitting {
		chunk, err := renderUnmarshalJSON(plan, pc, classFields[pc.BMMName])
		if err != nil {
			return nil, fmt.Errorf("render UnmarshalJSON %s: %w", pc.BMMName, err)
		}
		chunks = append(chunks, chunk)
	}

	body.WriteString("import (\n")
	body.WriteString("\t\"encoding/json\"\n")
	body.WriteString("\t\"fmt\"\n\n")
	body.WriteString("\t\"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg\"\n")
	if needsExternalImportForJSONMar(plan, chunks) {
		fmt.Fprintf(&body, "\t%q\n", plan.Target.ExternalImport)
	}
	body.WriteString(")\n\n")

	if file.PackagePath != "" {
		fmt.Fprintf(&body, "// BMM package: %s — canonical-JSON UnmarshalJSON companions\n\n", file.PackagePath)
	} else {
		body.WriteString("// canonical-JSON UnmarshalJSON companions (foundation classes)\n\n")
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

// polyKind enumerates the polymorphism shapes the generator can
// handle: a single polymorphic value, or a container of polymorphic
// values. Container-of-container or Hash<K, Iface> would extend this
// — neither appears in the current openEHR RM.
type polyKind int

const (
	polyNone polyKind = iota
	polySingle
	polySlice
)

// polymorphicProperty inspects a BMM property and returns the Go
// type name of its abstract element together with a [polyKind]
// classification. If the property is monomorphic, kind == polyNone.
//
// `owner` is the BMM class that declared `prop`; passed so the
// helper can resolve open generic parameter constraints (e.g.
// `Event[T ItemStructure].Data: T` is polymorphic when ItemStructure
// is an interface in the plan).
func polymorphicProperty(plan *Plan, owner *bmm.SimpleClass, prop bmm.Property) (string, polyKind) {
	switch p := prop.(type) {
	case *bmm.SingleProperty:
		if name, ok := abstractGoName(plan, p.TypeName); ok {
			return name, polySingle
		}
	case *bmm.SinglePropertyOpen:
		// Open generic parameter on the owner. When the parameter's
		// constraint (including inherited bounds) is an abstract type
		// or interface, route through typereg at decode time.
		if owner != nil && owner.GenericParameterDefs != nil {
			if def, ok := owner.GenericParameterDefs[p.TypeName]; ok {
				bound := def.ConformsToType
				if bound == "" {
					bound = inheritedGenericBound(plan, owner, p.TypeName)
				}
				if _, ok := abstractGoName(plan, bound); ok {
					return p.TypeName, polySingle
				}
			}
		}
		return "", polyNone
	case *bmm.ContainerProperty:
		if p.TypeDef == nil || p.TypeDef.TypeDef == nil {
			return "", polyNone
		}
		elemName := containerElementAbstractName(plan, p.TypeDef)
		if elemName != "" {
			switch p.TypeDef.ContainerType {
			case "Hash":
				return "", polyNone
			default:
				return elemName, polySlice
			}
		}
	case *bmm.GenericProperty:
		// GenericProperty.TypeDef.RootType is a concrete generic class
		// like DV_INTERVAL or REFERENCE_RANGE; the generic parameters
		// fix concrete types at this site, so json.Unmarshal handles
		// the inner value directly.
		return "", polyNone
	}
	return "", polyNone
}

// containerElementAbstractName returns the Go interface name for a
// container element when the element type is abstract in the plan.
func containerElementAbstractName(plan *Plan, td *bmm.ContainerType) string {
	if td == nil || td.TypeDef == nil {
		return ""
	}
	switch inner := td.TypeDef.(type) {
	case *bmm.SimpleType:
		name, ok := abstractGoName(plan, inner.TypeName)
		if ok {
			return name
		}
	case *bmm.GenericType:
		name, ok := abstractGoName(plan, inner.RootType)
		if ok {
			return name
		}
	}
	return ""
}

// abstractGoName returns the Go name of a BMM type if it resolves to
// an abstract class or interface in the plan; ok == false otherwise.
// Used by [polymorphicProperty] to detect polymorphic fields.
//
// Also returns the interface name for **concrete-with-descendants**
// types listed in [concreteSupertypeInterface]. Such types are concrete
// in the BMM but have RM-substitutable descendants (e.g. DV_TEXT ←
// DV_CODED_TEXT). To honour substitutability losslessly on decode,
// the generator promotes their field-type emissions to a marker
// interface so the polymorphic-dispatch path picks the concrete
// descendant by `_type`.
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
		if iface, ok := concreteSupertypeInterface[typeName]; ok {
			return iface, true
		}
	}
	return "", false
}

// concreteSupertypeInterface maps a concrete BMM class name to the Go
// marker-interface name we emit for its substitutability-polymorphism
// site. Limited to the cases the SDK ships first-class lossless
// round-trip support for; extend deliberately (REQ-058 §RM
// substitutability).
//
// The interface name MUST differ from the concrete Go struct name (the
// struct keeps its BMM-derived name; the interface gets the "RM-natural"
// name). For DV_TEXT the natural name is `DataValueText`.
var concreteSupertypeInterface = map[string]string{
	"DV_TEXT": "DataValueText",
}

// isConcreteSupertypeInterface reports whether the given Go interface
// name was emitted by the concrete-supertype path (i.e. is a value in
// [concreteSupertypeInterface]). Used by the unmarshal emitter to route
// such fields through a per-interface `Decode<Name>` helper that can
// default missing `_type` discriminators to the supertype's BMM name.
func isConcreteSupertypeInterface(name string) bool {
	for _, v := range concreteSupertypeInterface {
		if v == name {
			return true
		}
	}
	return false
}

// renderUnmarshalJSON emits the wire type + UnmarshalJSON method for
// a single concrete class. Field set + ownership is supplied by the
// caller so it does not need to be recomputed.
func renderUnmarshalJSON(plan *Plan, pc *PlannedClass, fields []emittedField) (string, error) {
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return "", fmt.Errorf("expected SimpleClass for %s, got %T", pc.BMMName, pc.Class)
	}

	wireName := jsonunmarWireTypeName(pc.GoName)
	recv := jsonmarReceiverName(pc.GoName)
	typeParams := ""
	typeArgs := ""
	if sc.IsGeneric() {
		typeParams = genericClassParamList(plan, sc)
		typeArgs = genericTypeArgList(sc)
	}

	var b strings.Builder

	// Wire struct definition — same field layout as the encode wire,
	// but polymorphic fields become json.RawMessage / []json.RawMessage.
	fmt.Fprintf(&b, "type %s%s struct {\n", wireName, typeParams)
	b.WriteString("\tClass string `json:\"_type\"`\n")
	for _, ef := range fields {
		ifaceName, kind := polymorphicProperty(plan, ef.Owner, ef.Prop)
		propName := ef.Prop.PropertyName()
		goField := FieldName(propName)
		tag := jsonTagFor(ef.Prop, propName)
		switch kind {
		case polySingle:
			fmt.Fprintf(&b, "\t%s json.RawMessage %s // polymorphic %s\n", goField, tag, ifaceName)
		case polySlice:
			fmt.Fprintf(&b, "\t%s []json.RawMessage %s // polymorphic []%s\n", goField, tag, ifaceName)
		default:
			line, err := renderField(plan, ef.Owner, ef.OwnerName, ef.Prop)
			if err != nil {
				return "", fmt.Errorf("render wire field %s.%s: %w", pc.BMMName, propName, err)
			}
			b.WriteString(line)
		}
	}
	b.WriteString("}\n\n")

	// UnmarshalJSON method.
	fmt.Fprintf(&b, "// UnmarshalJSON decodes canonical openEHR JSON into %s.\n", pc.GoName)
	b.WriteString("// Polymorphic fields are routed through typereg.DecodeAs so the\n")
	b.WriteString("// concrete type is selected by `_type` at each polymorphic site.\n")
	b.WriteString("// Missing/unknown/type-mismatch dispatch failures wrap typereg\n")
	b.WriteString("// sentinels inside *typereg.DecodeError for errors.Is / errors.As.\n")
	fmt.Fprintf(&b, "func (%s *%s%s) UnmarshalJSON(data []byte) error {\n", recv, pc.GoName, typeArgs)
	fmt.Fprintf(&b, "\tvar aux %s%s\n", wireName, typeArgs)
	b.WriteString("\tif err := json.Unmarshal(data, &aux); err != nil {\n")
	b.WriteString("\t\treturn fmt.Errorf(\"canjson: " + pc.BMMName + ": %w\", err)\n")
	b.WriteString("\t}\n")
	// Substitutability check (REQ-058): accept `_type` equal to this
	// class OR any of its known descendants. A `DV_CODED_TEXT` payload
	// landing in a `DV_TEXT`-typed slot is RM-conformant per openEHR
	// `data_types.text` (DV_CODED_TEXT IS-A DV_TEXT). The wire struct's
	// non-descendant fields are silently dropped by encoding/json's
	// field-name match; consumers who need full descendant fidelity
	// should type the slot as the marker interface (e.g. DataValueText
	// — generator support TBD per [docs/specifications/datamap.md]).
	descendants := plan.Descendants[pc.BMMName]
	fmt.Fprintf(&b, "\tif aux.Class != \"\" && aux.Class != %q", pc.BMMName)
	for _, d := range descendants {
		fmt.Fprintf(&b, " && aux.Class != %q", d)
	}
	b.WriteString(" {\n")
	b.WriteString("\t\treturn &typereg.DecodeError{\n")
	b.WriteString("\t\t\tPath: \"/_type\",\n")
	fmt.Fprintf(&b, "\t\t\tInner: fmt.Errorf(\"canjson: expected %%q (or a descendant), got %%q: %%w\", %q, aux.Class, typereg.ErrTypeMismatch),\n", pc.BMMName)
	b.WriteString("\t\t}\n")
	b.WriteString("\t}\n")
	// Copy non-polymorphic fields, then dispatch polymorphic ones.
	for _, ef := range fields {
		ifaceName, kind := polymorphicProperty(plan, ef.Owner, ef.Prop)
		propName := ef.Prop.PropertyName()
		goField := FieldName(propName)
		switch kind {
		case polyNone:
			fmt.Fprintf(&b, "\t%s.%s = aux.%s\n", recv, goField, goField)
		case polySingle:
			fmt.Fprintf(&b, "\tif len(aux.%s) > 0 && string(aux.%s) != \"null\" {\n", goField, goField)
			// Concrete-supertype interfaces (e.g. DataValueText) get a
			// per-interface decode helper instead of a raw typereg call
			// so the supertype can default missing `_type` discriminators
			// to itself — preserves canonical-JSON shorthand (REQ-058).
			if isConcreteSupertypeInterface(ifaceName) {
				fmt.Fprintf(&b, "\t\tdv, err := Decode%s(aux.%s)\n", ifaceName, goField)
			} else {
				fmt.Fprintf(&b, "\t\tdv, err := typereg.DecodeAs[%s](aux.%s)\n", ifaceName, goField)
			}
			b.WriteString("\t\tif err != nil {\n")
			fmt.Fprintf(&b, "\t\t\treturn &typereg.DecodeError{Path: \"/%s\", Inner: err}\n", propName)
			b.WriteString("\t\t}\n")
			fmt.Fprintf(&b, "\t\t%s.%s = dv\n", recv, goField)
			b.WriteString("\t}\n")
		case polySlice:
			// Loop and decoded-element variables use multi-letter names
			// (`idx`, `dv`) so they cannot shadow any single-letter
			// receiver (a/c/e/f/g/h/i/l/o/p/r/s …) used by the
			// generated MarshalJSON / UnmarshalJSON methods.
			fmt.Fprintf(&b, "\tif aux.%s != nil {\n", goField)
			fmt.Fprintf(&b, "\t\t%s.%s = make([]%s, len(aux.%s))\n", recv, goField, ifaceName, goField)
			fmt.Fprintf(&b, "\t\tfor idx, raw := range aux.%s {\n", goField)
			b.WriteString("\t\t\tif len(raw) == 0 || string(raw) == \"null\" {\n")
			b.WriteString("\t\t\t\tcontinue\n")
			b.WriteString("\t\t\t}\n")
			fmt.Fprintf(&b, "\t\t\tdv, err := typereg.DecodeAs[%s](raw)\n", ifaceName)
			b.WriteString("\t\t\tif err != nil {\n")
			fmt.Fprintf(&b, "\t\t\t\treturn &typereg.DecodeError{Path: fmt.Sprintf(\"/%s/%%d\", idx), Inner: err}\n", propName)
			b.WriteString("\t\t\t}\n")
			fmt.Fprintf(&b, "\t\t\t%s.%s[idx] = dv\n", recv, goField)
			b.WriteString("\t\t}\n")
			b.WriteString("\t}\n")
		}
	}
	b.WriteString("\treturn nil\n")
	b.WriteString("}\n")

	return b.String(), nil
}

// jsonunmarWireTypeName produces the per-class decode-wire type
// identifier. Distinct from the MarshalJSON wire because they hold
// different field shapes (RawMessage vs concrete types).
func jsonunmarWireTypeName(goName string) string {
	return goName + "JSONUnmarshaller"
}

// jsonTagFor returns the json struct tag for a property, mirroring
// renderField's logic for optional vs mandatory tagging. Sufficient
// for [polymorphic] fields where the type is fixed (RawMessage /
// []RawMessage); non-polymorphic fields go through renderField.
func jsonTagFor(prop bmm.Property, propName string) string {
	mandatory := false
	switch p := prop.(type) {
	case *bmm.SingleProperty:
		mandatory = p.IsMandatory
	case *bmm.ContainerProperty:
		mandatory = p.Cardinality != nil && p.Cardinality.Lower > 0
	}
	if mandatory {
		return fmt.Sprintf("`json:%q`", propName)
	}
	return fmt.Sprintf("`json:%q`", propName+",omitempty")
}

// QuoteMeta exposes the regexp helper used to escape qualifier
// strings when building per-target regular expressions. Kept in
// this file so the unmarshaller renderer is self-contained.
var _ = regexp.QuoteMeta

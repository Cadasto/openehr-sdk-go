package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// jsonpolyImportPath is the leaf helper package used to marshal
// interface-typed (polymorphic) fields so a concrete value sitting in
// an interface slot still emits its `_type` discriminator (SDK-GAP-13
// sub-gap A). It depends only on reflect + encoding/json, so the
// generated marshaller packages can import it without an import cycle.
const jsonpolyImportPath = "github.com/cadasto/openehr-sdk-go/openehr/internal/jsonpoly"

// RenderMarshalJSONFile renders the canonical-JSON `MarshalJSON`
// companions for every concrete (non-abstract, non-primitive,
// non-interface) class in the supplied [PlannedFile].
//
// The output is byte-stable per file. Returns (nil, nil) when the
// file has no concrete classes — the caller should skip writing such
// files entirely.
//
// # Emission strategy
//
// For each concrete class C, the file emits:
//
//  1. A package-level wire type `<goName>JSONMarshaller` (generic
//     when C is generic) — a flat struct whose first field is
//     `Type string \`json:"_type"\“ followed by every JSON-effective
//     field of C in encoding/json declaration-emit order (embedded
//     ancestors' fields first, in struct-declaration order, then C's
//     own + flattened-from-abstract-non-generic-ancestors fields in
//     BMM property declaration order).
//
//  2. A `MarshalJSON` method on `*C` that constructs the wire struct
//     by copying each field via Go's embedded-field promotion (so the
//     wire struct sees the descendant-shadows-ancestor view that the
//     Go type system already enforces).
//
// Field-enumeration avoids the embedded-pointer-alias trick, which
// breaks whenever the original struct embeds another concrete type
// that has its own MarshalJSON — encoding/json promotes the inner
// MarshalJSON via the embedded pointer and emits the inner type's
// payload instead of the wrapper. With explicit field copies the
// wire struct embeds nothing and so cannot promote any MarshalJSON.
//
// The `_type`-first + BMM-declaration-order rules pinned by REQ-052
// fall out naturally: `Type` is the first wire-struct field; the
// remaining fields are emitted in the same order as the original C
// struct's declaration; nil-pointer / empty-container optionality is
// preserved via the original `omitempty` tags reused verbatim.
func RenderMarshalJSONFile(plan *Plan, file *PlannedFile) ([]byte, error) {
	concrete := concreteClassesIn(file)
	if len(concrete) == 0 {
		return nil, nil
	}

	// Pre-render so the import set reflects what the chunks actually
	// reference: the cross-target rm qualifier (aom14 → rm) and the
	// jsonpoly helper (only when a polymorphic field is routed).
	chunks := make([]string, 0, len(concrete))
	for _, pc := range concrete {
		chunk, err := renderMarshalJSON(plan, pc)
		if err != nil {
			return nil, fmt.Errorf("render MarshalJSON %s: %w", pc.BMMName, err)
		}
		chunks = append(chunks, chunk)
	}

	needExternal := needsExternalImportForJSONMar(plan, chunks)
	needPoly := slices.ContainsFunc(chunks, func(c string) bool {
		return strings.Contains(c, "jsonpoly.")
	})

	var body bytes.Buffer
	body.WriteString(renderGeneratedHeader(plan))
	body.WriteString("\n")
	if needExternal || needPoly {
		body.WriteString("import (\n")
		body.WriteString("\t\"encoding/json\"\n\n")
		if needPoly {
			fmt.Fprintf(&body, "\t%q\n", jsonpolyImportPath)
		}
		if needExternal {
			fmt.Fprintf(&body, "\t%q\n", plan.Target.ExternalImport)
		}
		body.WriteString(")\n\n")
	} else {
		body.WriteString("import \"encoding/json\"\n\n")
	}
	if file.PackagePath != "" {
		fmt.Fprintf(&body, "// BMM package: %s — canonical-JSON MarshalJSON companions\n\n", file.PackagePath)
	} else {
		body.WriteString("// canonical-JSON MarshalJSON companions (foundation classes)\n\n")
	}

	for _, c := range chunks {
		body.WriteString(c)
		body.WriteString("\n")
	}

	formatted, err := format.Source(body.Bytes())
	if err != nil {
		return body.Bytes(), fmt.Errorf("gofmt %s_jsonmar_gen.go: %w", file.FileBase, err)
	}
	return formatted, nil
}

var (
	qualifierREMu    sync.Mutex
	qualifierRECache = map[string]*regexp.Regexp{}
)

func qualifierClassRE(qualifier string) *regexp.Regexp {
	qualifierREMu.Lock()
	defer qualifierREMu.Unlock()
	if re, ok := qualifierRECache[qualifier]; ok {
		return re
	}
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(qualifier) + `\.[A-Z]`)
	qualifierRECache[qualifier] = re
	return re
}

// needsExternalImportForJSONMar reports whether any rendered chunk
// references the target's external qualifier as a Go identifier
// (i.e. "rm." followed by an uppercase letter). Differs from the
// regular needsExternalImport's plain substring check, which trips
// over BMM doc comments containing words like "term." or "trim.".
func needsExternalImportForJSONMar(plan *Plan, chunks []string) bool {
	if plan.Target.ExternalQualifier == "" || plan.Target.ExternalImport == "" {
		return false
	}
	re := qualifierClassRE(plan.Target.ExternalQualifier)
	return slices.ContainsFunc(chunks, re.MatchString)
}

// concreteClassesIn returns the subset of file.Classes that should
// receive a generated MarshalJSON: non-external, non-primitive,
// non-abstract SimpleClass. Generic classes ARE included.
func concreteClassesIn(file *PlannedFile) []*PlannedClass {
	out := make([]*PlannedClass, 0, len(file.Classes))
	for _, pc := range file.Classes {
		if pc.External || pc.IsPrimitive {
			continue
		}
		sc, ok := pc.Class.(*bmm.SimpleClass)
		if !ok {
			continue
		}
		if sc.IsAbstract() {
			continue
		}
		out = append(out, pc)
	}
	return out
}

// emittedField captures one wire-struct field together with the BMM
// class where it was originally declared. The owner is used both for
// rendering the Go field type (via renderField) and for resolving
// cycle-break pointer overrides (via plan.CyclicSingleProps).
type emittedField struct {
	Prop      bmm.Property
	Owner     *bmm.SimpleClass
	OwnerName string
}

// effectiveFields returns the flat list of JSON-visible fields for a
// concrete class, in the same order encoding/json would emit them
// when marshalling an instance of the original struct.
//
// Order: embedded-ancestor fields first (preorder traversal,
// recursively), then the class's own + flattened-abstract-non-generic
// properties in BMM declaration order. The descendant shadows an
// embedded-ancestor property with the same name: at each visit, the
// `shadowedAbove` set carries the names declared by outer descendants
// so embedded ancestors skip those when listing their own.
func effectiveFields(plan *Plan, pc *PlannedClass) ([]emittedField, error) {
	var result []emittedField
	seen := map[string]bool{}

	var visit func(*PlannedClass, map[string]bool) error
	visit = func(cur *PlannedClass, shadowedAbove map[string]bool) error {
		sc, ok := cur.Class.(*bmm.SimpleClass)
		if !ok {
			return nil
		}
		// Compute embedded struct ancestors (concrete or abstract+generic)
		// in BMM declaration order — same set renderConcreteClass uses.
		embedded := map[string]bool{}
		var embeddedAncestors []*PlannedClass
		for _, anc := range cur.Class.Ancestors() {
			if isPrimitive(anc) || isSkippedPrimitive(anc) {
				continue
			}
			ap, ok := plan.Classes[anc]
			if !ok {
				continue
			}
			acls, isSimple := ap.Class.(*bmm.SimpleClass)
			if !isSimple {
				continue
			}
			isStruct := !acls.IsAbstract() || (acls.IsGeneric() && !codecPolymorphicAbstractGeneric(plan, ap))
			if !isStruct {
				continue
			}
			embedded[anc] = true
			embeddedAncestors = append(embeddedAncestors, ap)
		}
		// cur's own + flattened-abstract-non-generic properties.
		curProps := collectFlattenedProperties(plan, sc, embedded)
		// Propagate the shadowing set: outer-descendants AND cur shadow
		// the embedded ancestors' same-named properties.
		shadowedDownward := make(map[string]bool, len(shadowedAbove)+len(curProps))
		for k := range shadowedAbove {
			shadowedDownward[k] = true
		}
		for k := range curProps {
			shadowedDownward[k] = true
		}
		// Visit embeds first (their non-shadowed fields appear before
		// `cur`'s own fields in the encoded JSON).
		for _, ap := range embeddedAncestors {
			if err := visit(ap, shadowedDownward); err != nil {
				return err
			}
		}
		// Emit cur's own + flattened-abstract-non-generic fields in
		// BMM declaration order. Skip anything an outer descendant has
		// already declared (it will be emitted by that descendant).
		for _, name := range collectFlattenedPropertyOrder(plan, sc, embedded) {
			if shadowedAbove[name] || seen[name] {
				continue
			}
			seen[name] = true
			result = append(result, emittedField{
				Prop:      curProps[name],
				Owner:     sc,
				OwnerName: cur.BMMName,
			})
		}
		return nil
	}
	if err := visit(pc, map[string]bool{}); err != nil {
		return nil, err
	}
	return result, nil
}

// renderMarshalJSON emits the wire type definition + MarshalJSON
// method for a single concrete class.
func renderMarshalJSON(plan *Plan, pc *PlannedClass) (string, error) {
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return "", fmt.Errorf("expected SimpleClass for %s, got %T", pc.BMMName, pc.Class)
	}

	fields, err := effectiveFields(plan, pc)
	if err != nil {
		return "", err
	}

	wireName := jsonmarWireTypeName(pc.GoName)
	recv := jsonmarReceiverName(pc.GoName)
	typeParams := ""
	typeArgs := ""
	if sc.IsGeneric() {
		typeParams = genericClassParamList(plan, sc)
		typeArgs = genericTypeArgList(sc)
	}

	// Classify each field for polymorphic marshalling once, so the wire
	// struct and the method body stay in lockstep.
	kinds := make([]polyKind, len(fields))
	for i, ef := range fields {
		kinds[i] = marshalPolyKind(plan, ef.Owner, sc, ef.Prop)
	}

	var b strings.Builder

	// Wire struct definition.
	fmt.Fprintf(&b, "type %s%s struct {\n", wireName, typeParams)
	// `Class` is the Go-field name for the openEHR `_type` discriminator.
	// The JSON tag is what the wire sees; the Go name is generator-internal
	// and chosen to avoid colliding with the common BMM `type` property
	// (e.g. DV_PROPORTION.type) which would Pascal-case to `Type`.
	b.WriteString("\tClass string `json:\"_type\"`\n")
	for i, ef := range fields {
		var line string
		var err error
		if kinds[i] == polyNone {
			line, err = renderField(plan, ef.Owner, ef.OwnerName, ef.Prop)
		} else {
			// Polymorphic field: pre-marshalled to RawMessage by the
			// method body via the jsonpoly helper, which injects the
			// mandatory `_type` even when a concrete value (not pointer)
			// sits in the interface slot (SDK-GAP-13 sub-gap A).
			line, err = polyWireField(ef.Prop)
		}
		if err != nil {
			return "", fmt.Errorf("render wire field %s.%s: %w", pc.BMMName, ef.Prop.PropertyName(), err)
		}
		b.WriteString(line)
	}
	b.WriteString("}\n\n")

	// MarshalJSON method.
	fmt.Fprintf(&b, "// MarshalJSON emits canonical openEHR JSON for %s with `_type`\n", pc.GoName)
	fmt.Fprintf(&b, "// (value %q) as the leading object key. Field order matches the\n", pc.BMMName)
	b.WriteString("// concrete struct's declaration order — embedded-ancestor fields\n")
	b.WriteString("// first (in their original order), then own + flattened-abstract\n")
	b.WriteString("// ancestor fields in BMM property declaration order.\n")
	fmt.Fprintf(&b, "func (%s *%s%s) MarshalJSON() ([]byte, error) {\n", recv, pc.GoName, typeArgs)
	// Pre-marshal polymorphic fields through the jsonpoly helper so a
	// value-in-interface still carries its `_type`. Each `rawX` is a
	// fresh name, so `:=` is always valid even though `err` repeats.
	for i, ef := range fields {
		if kinds[i] == polyNone {
			continue
		}
		fieldName := FieldName(ef.Prop.PropertyName())
		helper := "Marshal"
		if kinds[i] == polySlice {
			helper = "MarshalSlice"
		}
		fmt.Fprintf(&b, "\traw%s, err := jsonpoly.%s(%s.%s)\n", fieldName, helper, recv, fieldName)
		b.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	}
	fmt.Fprintf(&b, "\treturn json.Marshal(&%s%s{\n", wireName, typeArgs)
	fmt.Fprintf(&b, "\t\tClass: %q,\n", pc.BMMName)
	for i, ef := range fields {
		fieldName := FieldName(ef.Prop.PropertyName())
		if kinds[i] == polyNone {
			fmt.Fprintf(&b, "\t\t%s: %s.%s,\n", fieldName, recv, fieldName)
		} else {
			fmt.Fprintf(&b, "\t\t%s: raw%s,\n", fieldName, fieldName)
		}
	}
	b.WriteString("\t})\n")
	b.WriteString("}\n")

	return b.String(), nil
}

// marshalPolyKind classifies a property for canonical-JSON marshalling.
// It reuses the decoder's polymorphicProperty classifier so encode and
// decode agree on which fields are polymorphic, then folds the
// abstract/narrow distinction (irrelevant on encode) into single/slice.
//
// SinglePropertyOpen (generic type-parameter bounds such as
// DV_INTERVAL.lower/upper) is deliberately left direct. For a concrete
// instantiation (e.g. DVInterval[DVQuantity]) the bound is a concrete
// value field that emits its `_type` because the wire struct is
// marshalled by-pointer, so the field is addressable; on decode typereg
// always rebuilds bounds as pointer concretes, so `_type` survives the
// round-trip. The DV_INTERVAL<T> collapse to DVInterval[DVOrdered] is
// handled validator-side (SDK-GAP-13 sub-gap B), not here. (A non-pointer
// value placed directly in a DVInterval[DVOrdered] bound would drop
// `_type`; that hand-built case is out of scope for this routing.)
func marshalPolyKind(plan *Plan, owner, emitting *bmm.SimpleClass, prop bmm.Property) polyKind {
	if _, isOpen := prop.(*bmm.SinglePropertyOpen); isOpen {
		return polyNone
	}
	_, kind := polymorphicProperty(plan, owner, emitting, prop)
	switch kind {
	case polySingle, polySingleNarrow:
		return polySingle
	case polySlice, polySliceNarrow:
		return polySlice
	}
	return polyNone
}

// polyWireField renders the wire-struct line for a polymorphic field:
// a json.RawMessage carrying the same doc comment and json tag
// (omitempty preserved) the typed field would have had. The method body
// fills it via the jsonpoly helper.
func polyWireField(prop bmm.Property) (string, error) {
	name := prop.PropertyName()
	goName := FieldName(name)
	docLines := propertyDoc(goName, propertyDocText(prop))
	return docLines + fmt.Sprintf("\t%s json.RawMessage %s\n", goName, polyJSONTag(prop, name)), nil
}

// polyJSONTag returns the json struct tag for a polymorphic wire field,
// replicating renderField's omitempty decision per property kind so the
// emitted key presence/absence is byte-identical to the typed field.
func polyJSONTag(prop bmm.Property, name string) string {
	jsonTag := fmt.Sprintf("`json:%q`", name)
	jsonTagOpt := fmt.Sprintf("`json:%q`", name+",omitempty")
	switch p := prop.(type) {
	case *bmm.SingleProperty:
		if p.IsMandatory {
			return jsonTag
		}
		return jsonTagOpt
	case *bmm.ContainerProperty:
		if p.Cardinality == nil || p.Cardinality.Lower == 0 {
			return jsonTagOpt
		}
		return jsonTag
	case *bmm.GenericProperty:
		if p.IsMandatory {
			return jsonTag
		}
		return jsonTagOpt
	}
	return jsonTag
}

// jsonmarWireTypeName produces the per-class wire type identifier.
// Convention: `<goName>JSONMarshaller` — clearly tied to the codec
// layer and unambiguous against user-defined types. Renaming this is
// a generator-only concern: nothing outside the generated files
// references these types.
func jsonmarWireTypeName(goName string) string {
	return goName + "JSONMarshaller"
}

// jsonmarReceiverName returns the single-letter receiver used in the
// generated MarshalJSON method.
func jsonmarReceiverName(goName string) string {
	if goName == "" {
		return "v"
	}
	return strings.ToLower(goName[:1])
}

// genericTypeArgList returns "[T, K, ...]" for use as the type
// argument list when instantiating a generic class with its own
// declared parameter names (no constraints). Sorted alphabetically.
func genericTypeArgList(sc *bmm.SimpleClass) string {
	if !sc.IsGeneric() {
		return ""
	}
	keys := make([]string, 0, len(sc.GenericParameterDefs))
	for k := range sc.GenericParameterDefs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return "[" + strings.Join(keys, ", ") + "]"
}

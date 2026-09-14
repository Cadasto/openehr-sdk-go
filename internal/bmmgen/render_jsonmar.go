package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// typeregImportPath is the shared type-registry subpackage. The generated
// codec companions import it for the streaming decode helper, the encode
// option join and the nil-receiver sentinel; it never imports back into a
// generated tree, so no cycle forms.
const typeregImportPath = "github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"

// RenderMarshalJSONFile renders the canonical-JSON MarshalJSONTo companions
// (encoding/json/v2, ADR 0022) for every concrete class in the supplied
// [PlannedFile]. It also emits the per-class wire type — a method-free alias
// or a flat wire struct — that the UnmarshalJSONFrom companion in the sibling
// _jsonunmar_gen.go references.
//
// The output is byte-stable per file. Returns (nil, nil) when the file has no
// concrete classes.
//
// # Two wire shapes (ADR 0022, ruling R19)
//
// Most classes take the zero-copy shape: a method-free alias `type rawC C`
// marshalled through an anonymous `struct{ _type; *rawC }`, so no fields are
// copied. That shape cannot be used for a class that embeds a marshaler-bearing
// concrete ancestor, because a defined type over such a struct PROMOTES the
// ancestor's MarshalJSONTo / UnmarshalJSONFrom: encoding/json/v2 would then
// dispatch to the ancestor and emit its payload under the wrong `_type`,
// silently (see the promotion note at [effectiveFields]). Those classes —
// [embedsMarshalerBearingConcrete] finds them — take a flat wire struct that
// embeds nothing and so cannot promote, with explicit field copies both ways.
//
// Either shape emits `_type` first, joins json.Deterministic(true) plus the
// FormatNil* options ([typereg.MarshalOptions]), and lets v2 resolve
// polymorphic interface fields through the registered decode hooks — there is
// no per-field routing and no json.RawMessage staging any more.
func RenderMarshalJSONFile(plan *Plan, file *PlannedFile) ([]byte, error) {
	concrete := concreteClassesIn(file)
	if len(concrete) == 0 {
		return nil, nil
	}

	chunks := make([]string, 0, len(concrete))
	for _, pc := range concrete {
		chunk, err := renderMarshalJSON(plan, pc)
		if err != nil {
			return nil, fmt.Errorf("render MarshalJSONTo %s: %w", pc.BMMName, err)
		}
		chunks = append(chunks, chunk)
	}

	needExternal := needsExternalImportForJSONMar(plan, chunks)

	var body bytes.Buffer
	body.WriteString(renderGeneratedHeader(plan))
	body.WriteString("\n")
	body.WriteString("import (\n")
	body.WriteString("\t\"encoding/json/jsontext\"\n")
	body.WriteString("\tjson \"encoding/json/v2\"\n\n")
	fmt.Fprintf(&body, "\t%q\n", typeregImportPath)
	if needExternal {
		fmt.Fprintf(&body, "\t%q\n", plan.Target.ExternalImport)
	}
	body.WriteString(")\n\n")
	if file.PackagePath != "" {
		fmt.Fprintf(&body, "// BMM package: %s — canonical-JSON MarshalJSONTo companions\n\n", file.PackagePath)
	} else {
		body.WriteString("// canonical-JSON MarshalJSONTo companions (foundation classes)\n\n")
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

// needsExternalImportForJSONMar reports whether any rendered chunk references
// the target's external qualifier as a Go identifier (i.e. "rm." followed by
// an uppercase letter). Differs from the regular needsExternalImport's plain
// substring check, which trips over BMM doc comments containing words like
// "term." or "trim.".
func needsExternalImportForJSONMar(plan *Plan, chunks []string) bool {
	if plan.Target.ExternalQualifier == "" || plan.Target.ExternalImport == "" {
		return false
	}
	re := qualifierClassRE(plan.Target.ExternalQualifier)
	return slices.ContainsFunc(chunks, re.MatchString)
}

// concreteClassesIn returns the subset of file.Classes that should receive a
// generated codec: non-external, non-primitive, non-abstract SimpleClass.
// Generic classes ARE included.
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

// emittedField captures one wire-struct field together with the BMM class where
// it was originally declared.
type emittedField struct {
	Prop      bmm.Property
	Owner     *bmm.SimpleClass
	OwnerName string
}

// embeddedStructAncestors returns the ancestors of cur that appear as Go
// embedded struct fields: concrete classes, plus abstract-generic classes that
// are not emitted as codec-facing interfaces. Abstract non-generic ancestors
// are flattened, not embedded, so they are excluded. The map keys are BMM
// names; the slice preserves BMM ancestor order. Both [effectiveFields] and
// [embedsMarshalerBearingConcrete] read this so the flat-field view and the
// promotion predicate agree on what "embedded" means.
func embeddedStructAncestors(plan *Plan, cur *PlannedClass) (map[string]bool, []*PlannedClass) {
	embedded := map[string]bool{}
	var ancestors []*PlannedClass
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
		ancestors = append(ancestors, ap)
	}
	return embedded, ancestors
}

// bearsGeneratedMarshaler reports whether pc is a class the generator equips
// with the MarshalJSONTo / UnmarshalJSONFrom pair — a non-primitive,
// non-abstract SimpleClass. An external concrete class counts: it carries the
// pair in its own package, so embedding it still promotes the methods.
func bearsGeneratedMarshaler(pc *PlannedClass) bool {
	if pc.IsPrimitive {
		return false
	}
	sc, ok := pc.Class.(*bmm.SimpleClass)
	return ok && !sc.IsAbstract()
}

// embedsMarshalerBearingConcrete reports whether pc embeds — directly or
// through a chain of embedded ancestors — a concrete class that bears the
// generated codec pair. Such a class cannot use the zero-copy `type rawC C`
// alias: the defined type promotes the embedded ancestor's MarshalJSONTo /
// UnmarshalJSONFrom, so v2 dispatches to the ancestor and emits the wrong
// `_type` with no compile error (the promotion trap documented at
// [effectiveFields]). These classes take the flat wire struct instead.
func embedsMarshalerBearingConcrete(plan *Plan, pc *PlannedClass) bool {
	_, ancestors := embeddedStructAncestors(plan, pc)
	for _, ap := range ancestors {
		if bearsGeneratedMarshaler(ap) {
			return true
		}
		if embedsMarshalerBearingConcrete(plan, ap) {
			return true
		}
	}
	return false
}

// effectiveFields returns the flat list of JSON-visible fields for a concrete
// class, in the same order encoding/json would emit them when marshalling an
// instance of the original struct.
//
// Order: embedded-ancestor fields first (preorder traversal, recursively), then
// the class's own + flattened-abstract-non-generic properties in BMM
// declaration order. The descendant shadows an embedded-ancestor property with
// the same name.
//
// # Promotion trap
//
// A flat field list matters for the flat wire struct: it embeds nothing, so it
// cannot promote any embedded ancestor's MarshalJSONTo / UnmarshalJSONFrom.
// encoding/json/v2 promotes a marshaler through an embedded pointer and emits
// the inner type's payload instead of the wrapper — which is exactly why a
// class embedding a marshaler-bearing concrete ancestor takes the flat shape
// rather than the `type rawC C` alias (ADR 0022, ruling R19).
func effectiveFields(plan *Plan, pc *PlannedClass) ([]emittedField, error) {
	var result []emittedField
	seen := map[string]bool{}

	var visit func(*PlannedClass, map[string]bool) error
	visit = func(cur *PlannedClass, shadowedAbove map[string]bool) error {
		sc, ok := cur.Class.(*bmm.SimpleClass)
		if !ok {
			return nil
		}
		embedded, embeddedAncestors := embeddedStructAncestors(plan, cur)
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

// renderMarshalJSON emits the wire type + MarshalJSONTo method for a single
// concrete class, choosing the alias or flat shape per
// [embedsMarshalerBearingConcrete].
func renderMarshalJSON(plan *Plan, pc *PlannedClass) (string, error) {
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return "", fmt.Errorf("expected SimpleClass for %s, got %T", pc.BMMName, pc.Class)
	}
	recv := jsonmarReceiverName(pc.GoName)
	typeParams, typeArgs := "", ""
	if sc.IsGeneric() {
		typeParams = genericClassParamList(plan, sc)
		typeArgs = genericTypeArgList(sc)
	}
	if embedsMarshalerBearingConcrete(plan, pc) {
		return renderMarshalFlat(plan, pc, recv, typeParams, typeArgs)
	}
	return renderMarshalAlias(pc, recv, typeParams, typeArgs), nil
}

// renderMarshalAlias emits the zero-copy shape: a method-free alias and a
// MarshalJSONTo that marshals it through an anonymous `struct{ _type; *alias }`.
func renderMarshalAlias(pc *PlannedClass, recv, typeParams, typeArgs string) string {
	alias := aliasTypeName(pc.GoName)
	var b strings.Builder
	fmt.Fprintf(&b, "// %s is the method-free canonical-JSON alias for %s. The alias\n", alias, pc.GoName)
	b.WriteString("// drops the codec methods so marshalling the anonymous wrapper below\n")
	b.WriteString("// does not recurse; the class embeds no marshaler-bearing concrete\n")
	b.WriteString("// ancestor, so nothing is promoted (ADR 0022).\n")
	fmt.Fprintf(&b, "type %s%s %s%s\n\n", alias, typeParams, pc.GoName, typeArgs)

	fmt.Fprintf(&b, "// MarshalJSONTo emits canonical openEHR JSON for %s with `_type`\n", pc.GoName)
	fmt.Fprintf(&b, "// (value %q) as the leading member. Field order otherwise follows the\n", pc.BMMName)
	b.WriteString("// struct declaration; json.Deterministic sorts any Hash keys and the\n")
	b.WriteString("// FormatNil* options keep a mandatory nil container's `null` spelling\n")
	b.WriteString("// (REQ-052, Q6). The receiver is a value so a concrete instance sitting\n")
	b.WriteString("// in a polymorphic interface slot by value — the shape the like-interface\n")
	b.WriteString("// accessors admit — still carries its `_type` (REQ-052 substitution).\n")
	fmt.Fprintf(&b, "func (%s %s%s) MarshalJSONTo(enc *jsontext.Encoder) error {\n", recv, pc.GoName, typeArgs)
	b.WriteString("\treturn json.MarshalEncode(enc, &struct {\n")
	b.WriteString("\t\tType string `json:\"_type\"`\n")
	fmt.Fprintf(&b, "\t\t*%s%s\n", alias, typeArgs)
	fmt.Fprintf(&b, "\t}{%q, (*%s%s)(&%s)}, typereg.MarshalOptions(enc))\n", pc.BMMName, alias, typeArgs, recv)
	b.WriteString("}\n")
	return b.String()
}

// renderMarshalFlat emits the flat shape for a class that embeds a
// marshaler-bearing concrete ancestor: a wire struct that flattens every
// JSON-visible field (so it embeds nothing) and a MarshalJSONTo that copies
// each field in.
func renderMarshalFlat(plan *Plan, pc *PlannedClass, recv, typeParams, typeArgs string) (string, error) {
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		return "", err
	}
	wire := flatWireTypeName(pc.GoName)
	var b strings.Builder
	fmt.Fprintf(&b, "// %s is the flat canonical-JSON wire struct for %s. %s embeds\n", wire, pc.GoName, pc.GoName)
	b.WriteString("// a marshaler-bearing concrete ancestor, so the zero-copy alias would\n")
	b.WriteString("// promote that ancestor's methods and emit the wrong `_type`; the flat\n")
	b.WriteString("// struct embeds nothing and so cannot promote (ADR 0022, ruling R19).\n")
	fmt.Fprintf(&b, "type %s%s struct {\n", wire, typeParams)
	b.WriteString("\tClass string `json:\"_type\"`\n")
	for _, ef := range fields {
		line, err := renderField(plan, ef.Owner, ef.OwnerName, ef.Prop)
		if err != nil {
			return "", fmt.Errorf("render wire field %s.%s: %w", pc.BMMName, ef.Prop.PropertyName(), err)
		}
		b.WriteString(line)
	}
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "// MarshalJSONTo emits canonical openEHR JSON for %s with `_type`\n", pc.GoName)
	fmt.Fprintf(&b, "// (value %q) as the leading member (REQ-052, Q6). The receiver is a\n", pc.BMMName)
	b.WriteString("// value so a by-value instance in a polymorphic slot keeps its `_type`.\n")
	fmt.Fprintf(&b, "func (%s %s%s) MarshalJSONTo(enc *jsontext.Encoder) error {\n", recv, pc.GoName, typeArgs)
	fmt.Fprintf(&b, "\treturn json.MarshalEncode(enc, &%s%s{\n", wire, typeArgs)
	fmt.Fprintf(&b, "\t\tClass: %q,\n", pc.BMMName)
	for _, ef := range fields {
		fn := FieldName(ef.Prop.PropertyName())
		fmt.Fprintf(&b, "\t\t%s: %s.%s,\n", fn, recv, fn)
	}
	b.WriteString("\t}, typereg.MarshalOptions(enc))\n")
	b.WriteString("}\n")
	return b.String(), nil
}

// aliasTypeName is the method-free alias type identifier for the zero-copy
// shape. Generator-internal: nothing outside the generated files references it.
func aliasTypeName(goName string) string {
	return "raw" + goName
}

// flatWireTypeName is the flat wire struct identifier for the flat shape.
func flatWireTypeName(goName string) string {
	return goName + "JSONWire"
}

// jsonmarReceiverName returns the single-letter receiver used in the generated
// codec methods.
func jsonmarReceiverName(goName string) string {
	if goName == "" {
		return "v"
	}
	return strings.ToLower(goName[:1])
}

// genericTypeArgList returns "[T, K, ...]" for use as the type argument list
// when instantiating a generic class with its own declared parameter names (no
// constraints). Sorted alphabetically.
func genericTypeArgList(sc *bmm.SimpleClass) string {
	if !sc.IsGeneric() {
		return ""
	}
	return "[" + strings.Join(sortedStringKeys(sc.GenericParameterDefs), ", ") + "]"
}

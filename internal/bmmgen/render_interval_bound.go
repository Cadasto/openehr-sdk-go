package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// intervalBoundFile is the per-target file that carries the interval-bound
// emptiness test the canonical encoders share.
const intervalBoundFile = "interval_bound_gen.go"

// The BASE Interval shape: the two bounds and the two open-side flags. A
// concrete class whose codec fields carry all four is interval-shaped,
// whether it descends from Interval (DV_INTERVAL, Proper_interval,
// Point_interval) or declares the members itself.
//
// Multiplicity_interval has no encoder of its own and marshals through the
// Proper_interval it embeds. The BMM makes it an interval of Integer, but the
// Go type embeds ProperInterval[any], so its bound is interface-typed: an open
// side's Integer(0) is a value and is kept, where ProperInterval[Integer]
// drops it.
const (
	propLower          = "lower"
	propUpper          = "upper"
	propLowerUnbounded = "lower_unbounded"
	propUpperUnbounded = "upper_unbounded"
)

// hasIntervalShape reports whether a class's codec field list carries the
// BASE Interval shape.
func hasIntervalShape(fields []emittedField) bool {
	have := make(map[string]bool, len(fields))
	for _, ef := range fields {
		have[ef.Prop.PropertyName()] = true
	}
	return have[propLower] && have[propUpper] && have[propLowerUnbounded] && have[propUpperUnbounded]
}

// intervalShaped reports whether pc's codec fields carry the BASE Interval
// shape, so its encoders leave out an open side's empty bound (REQ-052,
// REQ-056).
func intervalShaped(plan *Plan, pc *PlannedClass) (bool, error) {
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		return false, err
	}
	return hasIntervalShape(fields), nil
}

// openBoundFlag maps a bound property to the Go field of the flag that marks
// its side open.
func openBoundFlag(prop string) (string, bool) {
	switch prop {
	case propLower:
		return FieldName(propLowerUnbounded), true
	case propUpper:
		return FieldName(propUpperUnbounded), true
	}
	return "", false
}

// guardOpenIntervalBoundXML wraps the lines that emit one bound element of an
// interval-shaped class, so the element is left out when its side is open and
// the bound is empty. Lines for any other property pass through unchanged.
func guardOpenIntervalBoundXML(recv, prop, lines string) string {
	flag, ok := openBoundFlag(prop)
	if !ok {
		return lines
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\tif !omitIntervalBound(%s.%s, %s.%s) {\n", recv, flag, recv, FieldName(prop))
	for line := range strings.Lines(lines) {
		b.WriteString("\t" + line)
	}
	b.WriteString("\t}\n")
	return b.String()
}

// renderMarshalAliasInterval is [renderMarshalAlias] for an interval-shaped
// class. The alias wrapper stays the wire for every member; when a side is
// open and its bound empty, the wrapper declares a zero-size field of the
// bound's name at its own level, shallower than the bound embedded through the
// alias. The shallower field wins, and it is always omitted, so the member
// drops out and every other member keeps its place.
func renderMarshalAliasInterval(pc *PlannedClass, recv, typeParams, typeArgs string) string {
	alias := aliasTypeName(pc.GoName)

	var b strings.Builder
	b.WriteString(renderMarshalAliasDecl(pc, typeParams, typeArgs))

	fmt.Fprintf(&b, "// MarshalJSONTo emits canonical openEHR JSON for %s with `_type`\n", pc.GoName)
	fmt.Fprintf(&b, "// (value %q) as the leading member. Field order otherwise follows the\n", pc.BMMName)
	b.WriteString("// struct declaration; json.Deterministic sorts any Hash keys and the\n")
	b.WriteString("// FormatNil* options keep a mandatory nil container's `null` spelling.\n")
	b.WriteString("// The receiver is a value so a concrete instance sitting\n")
	b.WriteString("// in a polymorphic interface slot by value, the shape the like-interface\n")
	b.WriteString("// accessors admit, still carries its `_type`.\n")
	b.WriteString("//\n")
	fmt.Fprintf(&b, "// An open side (`%s` or `%s` set) whose bound is empty\n", propLowerUnbounded, propUpperUnbounded)
	fmt.Fprintf(&b, "// emits no `%s` or `%s` member. The wrapper then declares a zero-size\n", propLower, propUpper)
	b.WriteString("// field of that name at its own level, shallower than the bound embedded\n")
	b.WriteString("// through the alias, so it wins; it is always omitted, and the other\n")
	b.WriteString("// members keep their order.\n")
	fmt.Fprintf(&b, "func (%s %s%s) MarshalJSONTo(enc *jsontext.Encoder) error {\n", recv, pc.GoName, typeArgs)
	fmt.Fprintf(&b, "\tomitLower := omitIntervalBound(%s.%s, %s.%s)\n", recv, FieldName(propLowerUnbounded), recv, FieldName(propLower))
	fmt.Fprintf(&b, "\tomitUpper := omitIntervalBound(%s.%s, %s.%s)\n", recv, FieldName(propUpperUnbounded), recv, FieldName(propUpper))
	b.WriteString("\tswitch {\n")
	b.WriteString("\tcase omitLower && omitUpper:\n")
	b.WriteString(intervalWireReturn(pc, recv, alias, typeArgs, propLower, propUpper))
	b.WriteString("\tcase omitLower:\n")
	b.WriteString(intervalWireReturn(pc, recv, alias, typeArgs, propLower))
	b.WriteString("\tcase omitUpper:\n")
	b.WriteString(intervalWireReturn(pc, recv, alias, typeArgs, propUpper))
	b.WriteString("\t}\n")
	b.WriteString(intervalWireReturn(pc, recv, alias, typeArgs))
	b.WriteString("}\n")
	return b.String()
}

// intervalWireReturn renders one `return json.MarshalEncode(...)` over the
// `_type` + alias wrapper, with a zero-size `omitzero` field for each bound
// property named in omitted.
func intervalWireReturn(pc *PlannedClass, recv, alias, typeArgs string, omitted ...string) string {
	var b strings.Builder
	b.WriteString("\treturn json.MarshalEncode(enc, &struct {\n")
	b.WriteString("\t\tType string `json:\"_type\"`\n")
	fmt.Fprintf(&b, "\t\t*%s%s\n", alias, typeArgs)
	for _, prop := range omitted {
		fmt.Fprintf(&b, "\t\t%s struct{} `json:%q`\n", FieldName(prop), prop+",omitzero")
	}
	fmt.Fprintf(&b, "\t}{Type: %q, %s: (*%s%s)(&%s)}, typereg.MarshalOptions(enc))\n", pc.BMMName, alias, alias, typeArgs, recv)
	return b.String()
}

// RenderIntervalBoundFile renders <pkg>/interval_bound_gen.go for a target
// that owns an interval-shaped concrete class: the emptiness test the
// canonical encoders apply to an open side's bound (REQ-052, REQ-056), with
// one field-by-field zero predicate per concrete bound type, derived from the
// BMM class's properties and using no reflection (REQ-024).
//
// The bound types are read from each interval class's `lower` and `upper`
// fields (see [intervalBounds]): the concrete classes a generic bound admits
// (DV_ORDERED's for DV_INTERVAL) or a class the field names directly. Every
// Go built-in scalar type and RM primitive is covered by a comparison with
// its zero value, which is what BASE Interval's primitive bound Ordered
// admits. A predicate for a class reached by value from a bound type
// (DV_ORDINAL's `symbol`) is emitted too.
//
// Returns (nil, nil) when the target owns no interval-shaped concrete class.
func RenderIntervalBoundFile(plan *Plan) ([]byte, error) {
	shaped, boundClasses, err := intervalBounds(plan)
	if err != nil {
		return nil, err
	}
	if !shaped {
		return nil, nil
	}
	scalars, err := scalarBoundGoTypes()
	if err != nil {
		return nil, err
	}

	predicates, err := renderZeroPredicates(plan, boundClasses)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	b.WriteString(renderGeneratedHeader(plan))
	b.WriteString("\n")
	b.WriteString("// omitIntervalBound reports whether the canonical encoders leave out an\n")
	b.WriteString("// interval side's bound: the side is open (its `lower_unbounded` or\n")
	b.WriteString("// `upper_unbounded` flag is set) and its bound is empty. A non-empty bound\n")
	b.WriteString("// beside its own open flag, and any bound on a bounded side, is emitted as it\n")
	b.WriteString("// stands.\n")
	b.WriteString("func omitIntervalBound[T any](unbounded bool, bound T) bool {\n")
	b.WriteString("\treturn unbounded && isEmptyIntervalBound(bound)\n")
	b.WriteString("}\n\n")

	b.WriteString("// isEmptyIntervalBound reports whether an interval bound holds no value. A\n")
	b.WriteString("// bound typed by an interface, such as DVInterval[DVOrdered], is empty when\n")
	b.WriteString("// it is nil or holds a typed-nil pointer to an RM class; an all-zero value\n")
	b.WriteString("// behind the interface is still a value. A bound of a concrete type is empty\n")
	b.WriteString("// when it is that type's zero value: field by field for the RM data value\n")
	b.WriteString("// types the interval classes bound (the cases below), and by comparison for\n")
	b.WriteString("// a Go built-in scalar type or an RM primitive. A bound of any other Go type\n")
	b.WriteString("// is never empty, so it is emitted as it stands.\n")
	b.WriteString("func isEmptyIntervalBound[T any](bound T) bool {\n")
	b.WriteString("\tv := any(bound)\n")
	b.WriteString("\tif v == nil || IsTypedNil(v) {\n")
	b.WriteString("\t\treturn true\n")
	b.WriteString("\t}\n")
	b.WriteString("\tvar zero T\n")
	b.WriteString("\tif any(zero) == nil {\n")
	b.WriteString("\t\t// T is an interface type, and the bound behind it is a value.\n")
	b.WriteString("\t\treturn false\n")
	b.WriteString("\t}\n")
	b.WriteString("\tswitch x := v.(type) {\n")
	for _, pc := range boundClasses {
		fmt.Fprintf(&b, "\tcase %s:\n", pc.GoName)
		fmt.Fprintf(&b, "\t\treturn %s(x)\n", zeroPredicateName(pc))
	}
	fmt.Fprintf(&b, "\tcase %s:\n", strings.Join(scalars, ", "))
	b.WriteString("\t\treturn v == any(zero)\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn false\n")
	b.WriteString("}\n")

	for _, p := range predicates {
		b.WriteString("\n")
		b.WriteString(p)
	}

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), fmt.Errorf("gofmt %s: %w", intervalBoundFile, err)
	}
	return formatted, nil
}

// intervalBounds reports whether the target owns an interval-shaped concrete
// class, and returns the concrete classes its bounds admit, sorted by Go name.
// The list may be empty while shaped is true: a class whose bounds are all
// primitive needs no zero predicate, only the scalar case.
//
// The bound types come from each class's `lower` and `upper` fields. A field
// typed by a generic parameter admits the parameter's bound: the concrete
// descendants of an RM class (DV_ORDERED for DV_INTERVAL), or, for a
// primitive or absent bound (BASE Interval's Ordered), the scalar case. A
// field typed by a class names that class. A bound type with neither a zero
// predicate nor a scalar case is refused, so no bound is silently left
// "never empty".
func intervalBounds(plan *Plan) (bool, []*PlannedClass, error) {
	var shaped bool
	bounds := map[string]*PlannedClass{}
	for _, f := range plan.Files {
		for _, pc := range concreteClassesIn(f) {
			fields, err := effectiveFields(plan, pc)
			if err != nil {
				return false, nil, err
			}
			if !hasIntervalShape(fields) {
				continue
			}
			shaped = true
			for _, ef := range fields {
				if name := ef.Prop.PropertyName(); name != propLower && name != propUpper {
					continue
				}
				classes, err := boundFieldClasses(plan, pc, ef)
				if err != nil {
					return false, nil, fmt.Errorf("bmmgen: %s.%s: %w", pc.BMMName, ef.Prop.PropertyName(), err)
				}
				for _, bc := range classes {
					bounds[bc.BMMName] = bc
				}
			}
		}
	}
	out := slices.SortedFunc(maps.Values(bounds), func(a, b *PlannedClass) int { return strings.Compare(a.GoName, b.GoName) })
	return shaped, out, nil
}

// boundFieldClasses returns the concrete classes one bound field of an
// interval class admits by value, each of which needs a zero predicate. It
// returns none when the field's Go type is an interface or a pointer (a nil
// test covers it) or a scalar (the scalar case covers it).
func boundFieldClasses(plan *Plan, pc *PlannedClass, ef emittedField) ([]*PlannedClass, error) {
	switch p := ef.Prop.(type) {
	case *bmm.SinglePropertyOpen:
		return boundTypeClasses(plan, genericParamBound(plan, pc, ef, p.TypeName))
	case *bmm.SingleProperty:
		typ, err := singlePropTypeExpr(plan, ef.Owner, ef.OwnerName, p)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(typ, "*") || isInterfaceTypeRef(plan, p.TypeName) {
			return nil, nil
		}
		return boundTypeClasses(plan, p.TypeName)
	}
	return nil, fmt.Errorf("a bound of property kind %T has no emptiness test", ef.Prop)
}

// genericParamBound returns the BMM bound of the generic parameter a bound
// field is typed by. The emitting class forwards its own parameter to the
// embedded Interval, so its declaration wins; otherwise the nearest ancestor
// that declares the parameter, then the field's owner, decide.
func genericParamBound(plan *Plan, pc *PlannedClass, ef emittedField, param string) string {
	sc := pc.Class.(*bmm.SimpleClass)
	if def, ok := sc.GenericParameterDefs[param]; ok {
		return def.ConformsToType
	}
	if bound := inheritedGenericBound(plan, sc, param); bound != "" {
		return bound
	}
	if def, ok := ef.Owner.GenericParameterDefs[param]; ok {
		return def.ConformsToType
	}
	return ""
}

// boundTypeClasses resolves one BMM bound type to the concrete classes that
// need a zero predicate. A type the Go code spells `any`, a Go scalar or an
// RM primitive needs none: the interface and scalar cases cover it. A class
// contributes itself when concrete and its concrete descendants when it has
// any. Anything else is refused.
func boundTypeClasses(plan *Plan, bound string) ([]*PlannedClass, error) {
	switch {
	case bound == "", bound == "Any", isSkippedPrimitive(bound), isSkippedClass(bound):
		return nil, nil
	case isPrimitive(bound):
		goType := primitiveGoType[bound]
		scalars, err := scalarBoundGoTypes()
		if err != nil {
			return nil, err
		}
		if goType == "any" || slices.Contains(scalars, scalarAlias(goType)) {
			return nil, nil
		}
		return nil, fmt.Errorf("bound type %s (Go %s) has no scalar case", bound, goType)
	}
	pc, ok := plan.Classes[bound]
	if !ok {
		return nil, fmt.Errorf("bound type %s is neither a planned class nor a primitive", bound)
	}
	var out []*PlannedClass
	if sc, isSimple := pc.Class.(*bmm.SimpleClass); isSimple && !sc.IsAbstract() {
		out = append(out, pc)
	}
	for _, name := range plan.AbstractDescendants[bound] {
		out = append(out, plan.Classes[name])
	}
	for _, name := range plan.ConcreteSubtypes[bound] {
		out = append(out, plan.Classes[name])
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("bound type %s has no concrete class to test and no scalar case", bound)
	}
	for _, c := range out {
		if err := checkPredicateClass(c); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// checkPredicateClass refuses a class the zero-predicate renderer cannot
// handle: it must be an owned, non-generic class rendered as a Go struct.
func checkPredicateClass(pc *PlannedClass) error {
	sc, ok := pc.Class.(*bmm.SimpleClass)
	switch {
	case !ok:
		return fmt.Errorf("bmmgen: interval bound %s is not a simple class", pc.BMMName)
	case pc.External:
		return fmt.Errorf("bmmgen: interval bound %s belongs to another target", pc.BMMName)
	case sc.IsGeneric():
		return fmt.Errorf("bmmgen: interval bound %s is generic; its zero predicate is not generated", pc.BMMName)
	}
	return nil
}

// builtinScalarGoTypes are Go's predeclared scalar types, under their own
// names; byte and rune are aliases of uint8 and int32.
var builtinScalarGoTypes = []string{
	"bool", "complex128", "complex64", "float32", "float64",
	"int", "int16", "int32", "int64", "int8", "string",
	"uint", "uint16", "uint32", "uint64", "uint8", "uintptr",
}

// scalarAlias maps Go's predeclared alias types to the type they alias.
func scalarAlias(goType string) string {
	switch goType {
	case "byte":
		return "uint8"
	case "rune":
		return "int32"
	}
	return goType
}

// scalarBoundGoTypes lists, sorted, every Go built-in scalar type and the
// named scalar types the BMM primitives map to (Integer, Real, Character).
// A bound of one of these types is empty when it equals its zero value. A
// primitive mapped to anything but an identifier (a slice, a pointer) is
// refused: it could not be compared with its zero value.
func scalarBoundGoTypes() ([]string, error) {
	set := map[string]bool{}
	for _, goType := range builtinScalarGoTypes {
		set[goType] = true
	}
	for _, name := range sortedStringKeys(primitiveGoType) {
		goType := scalarAlias(primitiveGoType[name])
		if goType == "any" {
			continue
		}
		if !isGoIdentifier(goType) {
			return nil, fmt.Errorf("bmmgen: primitive %s maps to Go %s, which has no zero comparison", name, goType)
		}
		set[goType] = true
	}
	return slices.Sorted(maps.Keys(set)), nil
}

// isGoIdentifier reports whether s is a plain Go identifier, so a type of
// that name is a named or predeclared type, not a composite type literal.
func isGoIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

// zeroPredicateName is the generated predicate for pc's zero value.
func zeroPredicateName(pc *PlannedClass) string {
	return "isZero" + pc.GoName
}

// renderZeroPredicates renders one zero predicate per class in roots and per
// class they reach through a by-value field or an embedded ancestor, sorted
// by Go name.
func renderZeroPredicates(plan *Plan, roots []*PlannedClass) ([]string, error) {
	rendered := map[string]string{}
	queue := slices.Clone(roots)
	for len(queue) > 0 {
		pc := queue[0]
		queue = queue[1:]
		if _, done := rendered[pc.GoName]; done {
			continue
		}
		src, deps, err := renderZeroPredicate(plan, pc)
		if err != nil {
			return nil, err
		}
		rendered[pc.GoName] = src
		queue = append(queue, deps...)
	}
	out := make([]string, 0, len(rendered))
	for _, name := range slices.Sorted(maps.Keys(rendered)) {
		out = append(out, rendered[name])
	}
	return out, nil
}

// renderZeroPredicate renders the zero predicate of one class: every field of
// the Go struct, in declaration order, compared with its zero value. It
// returns the classes whose predicates the body calls.
func renderZeroPredicate(plan *Plan, pc *PlannedClass) (string, []*PlannedClass, error) {
	if err := checkPredicateClass(pc); err != nil {
		return "", nil, err
	}
	sc := pc.Class.(*bmm.SimpleClass)

	var conds []string
	var deps []*PlannedClass
	embedded, ancestors := embeddedStructAncestors(plan, pc)
	for _, ap := range ancestors {
		if err := checkPredicateClass(ap); err != nil {
			return "", nil, fmt.Errorf("bmmgen: %s embeds %s: %w", pc.BMMName, ap.BMMName, err)
		}
		conds = append(conds, fmt.Sprintf("%s(v.%s)", zeroPredicateName(ap), ap.GoName))
		deps = append(deps, ap)
	}
	props := collectFlattenedProperties(plan, sc, embedded)
	for _, name := range sortedStringKeys(props) {
		cond, dep, err := zeroFieldCondition(plan, pc, props[name])
		if err != nil {
			return "", nil, fmt.Errorf("bmmgen: zero predicate %s.%s: %w", pc.BMMName, name, err)
		}
		conds = append(conds, cond)
		if dep != nil {
			deps = append(deps, dep)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "// %s reports whether v is the zero %s, field by field.\n", zeroPredicateName(pc), pc.BMMName)
	fmt.Fprintf(&b, "func %s(v %s) bool {\n", zeroPredicateName(pc), pc.GoName)
	if len(conds) == 0 {
		b.WriteString("\treturn true\n")
	} else {
		fmt.Fprintf(&b, "\treturn %s\n", strings.Join(conds, " &&\n\t\t"))
	}
	b.WriteString("}\n")
	return b.String(), deps, nil
}

// zeroFieldCondition returns the Go condition that holds when one field of v
// is its zero value, and the class whose predicate the condition calls, if
// any. It reads the same type decisions the struct renderer makes.
func zeroFieldCondition(plan *Plan, pc *PlannedClass, prop bmm.Property) (string, *PlannedClass, error) {
	sc := pc.Class.(*bmm.SimpleClass)
	field := "v." + FieldName(prop.PropertyName())
	nilCond := field + " == nil"

	switch p := prop.(type) {
	case *bmm.ContainerProperty:
		return nilCond, nil, nil

	case *bmm.GenericProperty:
		typ, err := genericTypeRef(plan, sc, p.TypeDef)
		if err != nil {
			return "", nil, err
		}
		if !p.IsMandatory || strings.HasPrefix(typ, "[]") || strings.HasPrefix(typ, "map[") {
			return nilCond, nil, nil
		}
		return "", nil, fmt.Errorf("by-value generic field of type %s is not supported", typ)

	case *bmm.SingleProperty:
		typ, err := singlePropTypeExpr(plan, sc, pc.BMMName, p)
		if err != nil {
			return "", nil, err
		}
		switch {
		case strings.HasPrefix(typ, "*"), typ == "any", strings.HasPrefix(typ, "any "), isInterfaceTypeRef(plan, p.TypeName):
			return nilCond, nil, nil
		case isPrimitive(p.TypeName):
			switch primitiveGoType[p.TypeName] {
			case "bool":
				return "!" + field, nil, nil
			case "string", "Character":
				return field + ` == ""`, nil, nil
			case "Integer", "Real", "byte", "float64", "int64":
				return field + " == 0", nil, nil
			}
			return "", nil, fmt.Errorf("primitive %s (Go %s) has no zero comparison", p.TypeName, primitiveGoType[p.TypeName])
		}
		dep, ok := plan.Classes[p.TypeName]
		if !ok {
			return "", nil, fmt.Errorf("field type %s is not a planned class", p.TypeName)
		}
		if err := checkPredicateClass(dep); err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("%s(%s)", zeroPredicateName(dep), field), dep, nil
	}
	return "", nil, fmt.Errorf("property kind %T is not supported", prop)
}

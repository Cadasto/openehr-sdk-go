package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"maps"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// intervalBoundFile is the per-target file that carries the interval-bound
// emptiness test the canonical encoders share.
const intervalBoundFile = "interval_bound_gen.go"

// The BASE Interval shape: the two bounds and the two open-side flags. A
// concrete class whose codec fields carry all four is interval-shaped,
// whether it descends from Interval (DV_INTERVAL, Proper_interval,
// Point_interval) or declares the members itself.
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
// open and its bound empty, a zero-size field of the bound's name at the top
// of the wrapper shadows the embedded bound and is always omitted, so the
// member drops out and every other member keeps its place.
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
	fmt.Fprintf(&b, "// emits no `%s` or `%s` member. A zero-size field of that name at the top\n", propLower, propUpper)
	b.WriteString("// of the wrapper shadows the embedded bound and is always omitted, so the\n")
	b.WriteString("// other members keep their order.\n")
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
// The bound types are the concrete descendants of each interval-shaped
// class's generic bound (DV_ORDERED for DV_INTERVAL), plus the Go scalar
// types the BMM primitives map to (the bound of BASE Interval is the
// primitive Ordered). A predicate for a class reached by value from a bound
// type (DV_ORDINAL's `symbol`) is emitted too.
//
// Returns (nil, nil) when the target owns no interval-shaped concrete class.
func RenderIntervalBoundFile(plan *Plan) ([]byte, error) {
	boundClasses, err := intervalBoundClasses(plan)
	if err != nil {
		return nil, err
	}
	if boundClasses == nil {
		return nil, nil
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
	b.WriteString("// it is nil or holds a typed-nil pointer; an all-zero value behind the\n")
	b.WriteString("// interface is still a value. A bound of a concrete type is empty when it is\n")
	b.WriteString("// that type's zero value, compared field by field. A concrete type that is\n")
	b.WriteString("// not an interval bound type is never empty.\n")
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
	fmt.Fprintf(&b, "\tcase %s:\n", strings.Join(scalarBoundGoTypes(), ", "))
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

// intervalBoundClasses returns the concrete bound classes of the target's
// interval-shaped concrete classes, sorted by Go name. It returns nil when the
// target owns no interval-shaped concrete class, and an empty non-nil slice
// when it owns one whose bounds are all primitive.
func intervalBoundClasses(plan *Plan) ([]*PlannedClass, error) {
	var shaped bool
	bounds := map[string]*PlannedClass{}
	for _, f := range plan.Files {
		for _, pc := range concreteClassesIn(f) {
			ok, err := intervalShaped(plan, pc)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			shaped = true
			sc := pc.Class.(*bmm.SimpleClass)
			for _, name := range sortedStringKeys(sc.GenericParameterDefs) {
				bound := sc.GenericParameterDefs[name].ConformsToType
				bp, planned := plan.Classes[bound]
				if !planned {
					// A primitive bound (BASE Interval's Ordered): the
					// scalar case of the emptiness test covers it.
					continue
				}
				if bsc, isSimple := bp.Class.(*bmm.SimpleClass); isSimple && !bsc.IsAbstract() {
					bounds[bound] = bp
				}
				for _, d := range plan.AbstractDescendants[bound] {
					bounds[d] = plan.Classes[d]
				}
			}
		}
	}
	if !shaped {
		return nil, nil
	}
	out := slices.Collect(maps.Values(bounds))
	slices.SortFunc(out, func(a, b *PlannedClass) int { return strings.Compare(a.GoName, b.GoName) })
	for _, pc := range out {
		if err := checkPredicateClass(pc); err != nil {
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

// scalarBoundGoTypes lists the Go scalar types the BMM primitives map to,
// sorted. A bound of one of these types is empty when it equals its zero
// value.
func scalarBoundGoTypes() []string {
	set := map[string]bool{}
	for _, goType := range primitiveGoType {
		if goType != "any" {
			set[goType] = true
		}
	}
	return slices.Sorted(maps.Keys(set))
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

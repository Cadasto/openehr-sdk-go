package bmmgen

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// anyMemberFunc names the function a target package's generated encoders
// call on the value of a member typed Any that takes no choice of shapes
// ([xmlChoiceOf]), such as the AOM 1.4 `EXPR_LEAF.item`. It returns the value
// in the form the encoders write it, and reports whether the canonical
// decoders read such a member back as a value of the same type: so the
// canonical-JSON encoder's check ([jsonAnyChecks]) and the canonical-XML
// encoder ([marshalXMLAny]) accept the same values and refuse any other.
const anyMemberFunc = "anyMemberValue"

// anyMemberRefusal ends the message with which both encoders refuse the value
// of a member typed Any that [anyMemberFunc] does not accept, after the
// member's name and the Go type it holds.
const anyMemberRefusal = "which is no registered class, typed string or string"

// plainAnyMember reports whether ef, a member of the class pc, is typed Any
// and takes no choice of shapes.
func plainAnyMember(plan *Plan, pc *PlannedClass, ef emittedField) (bool, error) {
	p, ok := ef.Prop.(*bmm.SingleProperty)
	if !ok || p.TypeName != bmmAnyType {
		return false, nil
	}
	choice, err := xmlChoiceOf(plan, pc, ef)
	if err != nil {
		return false, err
	}
	return choice == nil, nil
}

// anyMemberFile returns the file whose canonical-JSON companion declares
// [anyMemberFunc]: the first of plan's files with a codec class that has a
// member typed Any taking no choice of shapes. It returns nil when no class
// of the target has one, and the target's package then declares no such
// function.
func anyMemberFile(plan *Plan) (*PlannedFile, error) {
	for _, f := range plan.Files {
		classes, err := codecClassesIn(plan, f)
		if err != nil {
			return nil, err
		}
		for _, pc := range classes {
			fields, err := effectiveFields(plan, pc)
			if err != nil {
				return nil, err
			}
			for _, ef := range fields {
				plain, err := plainAnyMember(plan, pc, ef)
				if err != nil {
					return nil, err
				}
				if plain {
					return f, nil
				}
			}
		}
	}
	return nil, nil
}

// anyMemberClasses returns the classes a member typed Any holds as the
// canonical decoders return it, through the type registry: each concrete
// class the registry holds where the target's package is linked in, the
// target's own and, for a target that refers to another target's classes,
// such as the AOM 1.4 target's RM classes, that target's, in the order of
// their BMM names.
func anyMemberClasses(plan *Plan) []*PlannedClass {
	var classes []*PlannedClass
	for _, name := range sortedStringKeys(plan.Classes) {
		if pc := plan.Classes[name]; isConcreteForRegistry(pc) {
			classes = append(classes, pc)
		}
	}
	return classes
}

// anyMemberGoType returns the Go type of pc, one of [anyMemberClasses], whose
// pointer its registration constructs: a generic class instantiated with its
// default arguments, qualified as the target's package refers to it.
func anyMemberGoType(plan *Plan, pc *PlannedClass) string {
	goType := qualifyClassRef(plan, pc)
	if sc, ok := pc.Class.(*bmm.SimpleClass); ok && sc.IsGeneric() {
		goType += defaultGenericArgs(plan, sc)
	}
	return goType
}

// renderAnyMemberValue renders [anyMemberFunc]: a closed type switch that
// accepts nil, a string, an xmlschema.TypedString or a pointer to one, and a
// value of each of [anyMemberClasses], through a pointer or by value, and
// refuses any other value, one whose type has a BMMName method included. A
// class value held by value is returned as a pointer to a copy of it, the
// form the decoders return and the canonical-XML encoder writes; any other
// accepted value is returned as it is.
func renderAnyMemberValue(plan *Plan) string {
	var classes []string
	for _, pc := range anyMemberClasses(plan) {
		classes = append(classes, anyMemberGoType(plan, pc))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "// %s returns v, the value of a member typed Any that takes no\n", anyMemberFunc)
	b.WriteString("// choice of shapes, in the form the canonical encoders write it, and\n")
	b.WriteString("// reports whether the canonical decoders read such a member back as a\n")
	b.WriteString("// value of the same type: nil, a string, an xmlschema.TypedString or a\n")
	b.WriteString("// pointer to one, or a value of a class the type registry constructs,\n")
	b.WriteString("// through a pointer or by value. A class value held by value is returned\n")
	b.WriteString("// as a pointer to a copy of it, the form the decoders return; any other\n")
	b.WriteString("// accepted value is returned as it is. Every other value is refused, one\n")
	b.WriteString("// whose type names itself after a registered class included, since the\n")
	b.WriteString("// decoders would not return it.\n")
	fmt.Fprintf(&b, "func %s(v any) (any, bool) {\n", anyMemberFunc)
	b.WriteString("\tswitch x := v.(type) {\n")
	b.WriteString("\tcase nil, string, xmlschema.TypedString, *xmlschema.TypedString")
	for _, t := range classes {
		fmt.Fprintf(&b, ",\n\t\t*%s", t)
	}
	b.WriteString(":\n\t\treturn v, true\n")
	for _, t := range classes {
		fmt.Fprintf(&b, "\tcase %s:\n\t\treturn &x, true\n", t)
	}
	b.WriteString("\t}\n")
	b.WriteString("\treturn nil, false\n")
	b.WriteString("}\n")
	return b.String()
}

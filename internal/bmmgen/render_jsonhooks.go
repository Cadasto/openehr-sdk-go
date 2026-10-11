package bmmgen

import (
	"bytes"
	"cmp"
	"fmt"
	"go/format"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// polyInterface names one polymorphic interface that needs a decode hook in the
// current target's package, together with the fallback used when the wire omits
// `_type`. A narrow interface (`<Parent>Like`) falls back to its parent concrete
// constructor (wire.md:147); an abstract interface has no fallback and refuses
// a missing discriminator.
type polyInterface struct {
	GoName   string // unqualified interface name owned by this target
	Narrow   bool
	ParentGo string // for a narrow interface: the parent concrete Go type
	// Class is the BMM class the interface stands for: the abstract class
	// (DATA_VALUE for DataValue), or a narrow interface's parent class
	// (DV_TEXT for DVTextLike), which is also its fallback.
	Class string
}

// genericClass is one concrete generic class owned by the target whose Go type
// takes a single type parameter.
type genericClass struct {
	GoName string // unqualified Go type name
	Param  string // the type parameter's name, such as "T"
}

// RenderJSONHooksFile renders <pkg>/jsonhooks_gen.go: one json.UnmarshalFromFunc
// per polymorphic interface owned by the target, registered into the typereg
// aggregate at init. Every nested decode threads the
// aggregate through [typereg.Unmarshalers], so a polymorphic slot resolves from
// any entry point, including a bare encoding/json/v2 Unmarshal with no options.
//
// Beside the hooks it renders the type information a reader of a decode
// target's Go type needs (REQ-170): PolymorphicClass, which names the BMM
// class each polymorphic interface stands for and its fallback, and a
// BMMTypeArgument method on each concrete generic class with one type
// parameter, which returns a new zero value of the type argument.
//
// Returns (nil, nil) when the target has neither polymorphic interfaces nor
// generic classes.
func RenderJSONHooksFile(plan *Plan) ([]byte, error) {
	ifaces, err := polymorphicInterfaces(plan)
	if err != nil {
		return nil, err
	}
	generics := singleParameterGenerics(plan)
	if len(ifaces) == 0 && len(generics) == 0 {
		return nil, nil
	}

	var b bytes.Buffer
	b.WriteString(renderGeneratedHeader(plan))
	b.WriteString("\n")
	if len(ifaces) == 0 {
		renderTypeArguments(&b, generics)
		return formatHooks(b.Bytes())
	}
	b.WriteString("import (\n")
	b.WriteString("\t\"encoding/json/jsontext\"\n")
	b.WriteString("\tjson \"encoding/json/v2\"\n\n")
	fmt.Fprintf(&b, "\t%q\n", plan.Target.RegistryImport)
	b.WriteString(")\n\n")
	b.WriteString("// init registers one decode hook per polymorphic interface owned by this\n")
	b.WriteString("// package into the shared typereg aggregate. Each hook resolves the\n")
	b.WriteString("// concrete type from `_type` through typereg.DecodePolymorphic; a narrow\n")
	b.WriteString("// interface falls back to its parent concrete type when the wire omits\n")
	b.WriteString("// the discriminator.\n")
	b.WriteString("func init() {\n")
	for _, pi := range ifaces {
		fmt.Fprintf(&b, "\ttypereg.RegisterUnmarshaler(json.UnmarshalFromFunc(func(dec *jsontext.Decoder, out *%s) error {\n", pi.GoName)
		if pi.Narrow {
			fmt.Fprintf(&b, "\t\treturn typereg.DecodePolymorphic(dec, out, func() any { return &%s{} })\n", pi.ParentGo)
		} else {
			b.WriteString("\t\treturn typereg.DecodePolymorphic(dec, out, nil)\n")
		}
		b.WriteString("\t}))\n")
	}
	b.WriteString("}\n\n")

	b.WriteString("// PolymorphicClass reports the BMM classes behind the polymorphic\n")
	b.WriteString("// interface of this package that p points to, for code that reads the\n")
	b.WriteString("// classes a decode target's Go type declares. class is the class the\n")
	b.WriteString("// interface stands for: an abstract class, or for a narrow interface\n")
	b.WriteString("// (<Parent>Like) its parent class. fallback is the class a value that\n")
	b.WriteString("// leaves out `_type` decodes as, which is the parent class of a narrow\n")
	b.WriteString("// interface; it is empty where the decoder refuses such a value. ok is false\n")
	b.WriteString("// when p points to no polymorphic interface of this package.\n")
	b.WriteString("func PolymorphicClass(p any) (class, fallback string, ok bool) {\n")
	b.WriteString("\tswitch p.(type) {\n")
	for _, pi := range ifaces {
		fallback := ""
		if pi.Narrow {
			fallback = pi.Class
		}
		fmt.Fprintf(&b, "\tcase *%s:\n\t\treturn %q, %q, true\n", pi.GoName, pi.Class, fallback)
	}
	b.WriteString("\t}\n")
	b.WriteString("\treturn \"\", \"\", false\n")
	b.WriteString("}\n")
	renderTypeArguments(&b, generics)
	return formatHooks(b.Bytes())
}

// formatHooks gofmts the rendered jsonhooks_gen.go source.
func formatHooks(src []byte) ([]byte, error) {
	formatted, err := format.Source(src)
	if err != nil {
		return src, fmt.Errorf("gofmt jsonhooks_gen.go: %w", err)
	}
	return formatted, nil
}

// renderTypeArguments writes one BMMTypeArgument method per generic class.
func renderTypeArguments(b *bytes.Buffer, generics []genericClass) {
	for _, g := range generics {
		b.WriteString("\n// BMMTypeArgument returns a new zero value of the type argument, for code\n")
		b.WriteString("// that reads the RM class a Go type binds the class's type parameter to.\n")
		fmt.Fprintf(b, "func (*%s[%s]) BMMTypeArgument() any { return new(%s) }\n", g.GoName, g.Param, g.Param)
	}
}

// singleParameterGenerics returns the concrete generic classes owned by the
// plan's target whose Go type takes exactly one type parameter, sorted by Go
// name. A class with more parameters gets no BMMTypeArgument method, so a
// reader of its Go type binds none of them.
func singleParameterGenerics(plan *Plan) []genericClass {
	var out []genericClass
	for _, pc := range plan.ConcreteClasses {
		if pc.External || !isConcreteCodecClass(pc) {
			continue
		}
		sc, ok := pc.Class.(*bmm.SimpleClass)
		if !ok || !sc.IsGeneric() || len(sc.GenericParameterDefs) != 1 {
			continue
		}
		out = append(out, genericClass{GoName: pc.GoName, Param: sortedStringKeys(sc.GenericParameterDefs)[0]})
	}
	slices.SortFunc(out, func(a, b genericClass) int { return cmp.Compare(a.GoName, b.GoName) })
	return out
}

// interfaceClassNames maps the Go name of every polymorphic interface the
// plan can emit to the BMM class it stands for: an abstract class or BMM
// interface, or the concrete parent of a narrow interface.
func interfaceClassNames(plan *Plan) (map[string]string, error) {
	out := map[string]string{}
	add := func(goName, class string, ok bool) error {
		if !ok {
			return nil
		}
		if prev, seen := out[goName]; seen && prev != class {
			return fmt.Errorf("polymorphic interface %s stands for both %s and %s", goName, prev, class)
		}
		out[goName] = class
		return nil
	}
	for _, name := range sortedStringKeys(plan.Classes) {
		abstract, isAbstract := abstractGoName(plan, name)
		if err := add(abstract, name, isAbstract); err != nil {
			return nil, err
		}
		narrow, isNarrow := narrowInterfaceGoName(plan, name)
		if err := add(narrow, name, isNarrow); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// polymorphicInterfaces returns the distinct polymorphic interfaces owned by the
// plan's target, sorted by Go name. An interface is owned by the target when its
// resolved Go name carries no cross-target qualifier ("rm."); an interface used
// only from the other target already has its hook generated in that target's
// package.
func polymorphicInterfaces(plan *Plan) ([]polyInterface, error) {
	seen := map[string]polyInterface{}
	for _, pc := range plan.ConcreteClasses {
		sc, ok := pc.Class.(*bmm.SimpleClass)
		if !ok {
			continue
		}
		fields, err := effectiveFields(plan, pc)
		if err != nil {
			return nil, fmt.Errorf("enumerate polymorphic interfaces for %s: %w", pc.BMMName, err)
		}
		for _, ef := range fields {
			name, narrow, parent, ok := fieldPolyInterface(plan, ef.Owner, sc, ef.Prop)
			if !ok || strings.Contains(name, ".") {
				continue
			}
			seen[name] = polyInterface{GoName: name, Narrow: narrow, ParentGo: parent}
		}
	}
	classes, err := interfaceClassNames(plan)
	if err != nil {
		return nil, err
	}
	out := make([]polyInterface, 0, len(seen))
	for _, v := range seen {
		class, ok := classes[v.GoName]
		if !ok {
			return nil, fmt.Errorf("polymorphic interface %s stands for no BMM class", v.GoName)
		}
		v.Class = class
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b polyInterface) int { return cmp.Compare(a.GoName, b.GoName) })
	return out, nil
}

// fieldPolyInterface resolves a property to the polymorphic interface its decode
// hook serves. An open generic bound resolves to the abstract interface it
// conforms to (e.g. DV_INTERVAL.lower : T conforms_to DV_ORDERED yields
// DVOrdered), which is how the single DVOrdered hook serves the interval bounds
// without a bespoke bound router (Q1). ok is false for a monomorphic property.
func fieldPolyInterface(plan *Plan, owner, emitting *bmm.SimpleClass, prop bmm.Property) (name string, narrow bool, parent string, ok bool) {
	if op, isOpen := prop.(*bmm.SinglePropertyOpen); isOpen {
		bound := openBoundType(plan, owner, emitting, op)
		if n, found := abstractGoName(plan, bound); found {
			return n, false, "", true
		}
		return "", false, "", false
	}
	n, kind := polymorphicProperty(plan, owner, emitting, prop)
	switch kind {
	case polySingle, polySlice:
		return n, false, "", true
	case polySingleNarrow, polySliceNarrow:
		return n, true, strings.TrimSuffix(n, "Like"), true
	case polyNone:
		// monomorphic, no hook
	}
	return "", false, "", false
}

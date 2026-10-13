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

// RenderJSONHooksFile renders <pkg>/jsonhooks_gen.go: one json.UnmarshalFromFunc
// per polymorphic interface owned by the target, registered into the typereg
// aggregate at init. Every nested decode threads the
// aggregate through [typereg.Unmarshalers], so a polymorphic slot resolves from
// any entry point, including a bare encoding/json/v2 Unmarshal with no options.
//
// For the RM target it also renders polymorphicClass, an unexported type
// switch naming the BMM class each interface stands for, and registers it
// into internal/rmjsonwalk/polyreg at init. The canonical-JSON walker behind
// the unmatched-member options reads a decode target's Go type through that
// table without importing openehr/rm, which canjson must not import, and
// without reflecting over the registry, which the SDK's reflection policy
// rules out. The AOM 1.4 target gets no table: the JSON walk does not
// check AOM 1.4 classes.
//
// Returns (nil, nil) when the target has no polymorphic interfaces.
func RenderJSONHooksFile(plan *Plan) ([]byte, error) {
	ifaces, err := polymorphicInterfaces(plan)
	if err != nil {
		return nil, err
	}
	if len(ifaces) == 0 {
		return nil, nil
	}

	var b bytes.Buffer
	b.WriteString(renderGeneratedHeader(plan))
	b.WriteString("\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"encoding/json/jsontext\"\n")
	b.WriteString("\tjson \"encoding/json/v2\"\n\n")
	walkerTable := plan.Target.GoPackage == "rm"
	if walkerTable {
		fmt.Fprintf(&b, "\t%q\n", polyregImport)
	}
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
	if walkerTable {
		b.WriteString("\tpolyreg.Register(polymorphicClass)\n")
	}
	b.WriteString("}\n")
	if walkerTable {
		renderPolymorphicClass(&b, ifaces)
	}
	return formatGenerated(b.Bytes(), "jsonhooks_gen.go")
}

// polyregImport is the import path of the seam the RM package registers its
// interface table into for the canonical-JSON walker.
const polyregImport = "github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk/polyreg"

// renderPolymorphicClass writes polymorphicClass, the type switch naming the
// BMM class behind each polymorphic interface: the abstract class, or for a
// narrow interface its parent class, which is also the class its hook falls
// back to.
func renderPolymorphicClass(b *bytes.Buffer, ifaces []polyInterface) {
	b.WriteString("\n// polymorphicClass names the BMM class behind the polymorphic interface p\n")
	b.WriteString("// points to: the abstract class the interface stands for, or for a narrow\n")
	b.WriteString("// interface (<Parent>Like) its parent class. ok is false when p points to no\n")
	b.WriteString("// polymorphic interface of this package. init registers it into\n")
	b.WriteString("// internal/rmjsonwalk/polyreg for the canonical-JSON walker, which reads a\n")
	b.WriteString("// decode target's Go type without importing this package.\n")
	b.WriteString("func polymorphicClass(p any) (string, bool) {\n")
	b.WriteString("\tswitch p.(type) {\n")
	for _, pi := range ifaces {
		fmt.Fprintf(b, "\tcase *%s:\n\t\treturn %q, true\n", pi.GoName, pi.Class)
	}
	b.WriteString("\t}\n")
	b.WriteString("\treturn \"\", false\n")
	b.WriteString("}\n")
}

// formatGenerated gofmts the rendered source of the generated file name.
func formatGenerated(src []byte, name string) ([]byte, error) {
	formatted, err := format.Source(src)
	if err != nil {
		return src, fmt.Errorf("gofmt %s: %w", name, err)
	}
	return formatted, nil
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

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
}

// RenderJSONHooksFile renders <pkg>/jsonhooks_gen.go: one json.UnmarshalFromFunc
// per polymorphic interface owned by the target, registered into the typereg
// aggregate at init (ADR 0022, ruling R6). Every nested decode threads the
// aggregate through [typereg.Unmarshalers], so a polymorphic slot resolves from
// any entry point, including a bare encoding/json/v2 Unmarshal with no options.
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
	fmt.Fprintf(&b, "\t%q\n", plan.Target.RegistryImport)
	b.WriteString(")\n\n")
	b.WriteString("// init registers one decode hook per polymorphic interface owned by this\n")
	b.WriteString("// package into the shared typereg aggregate. Each hook resolves the\n")
	b.WriteString("// concrete type from `_type` through typereg.DecodePolymorphic; a narrow\n")
	b.WriteString("// interface falls back to its parent concrete type when the wire omits\n")
	b.WriteString("// the discriminator (REQ-052, wire.md:147).\n")
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
	b.WriteString("}\n")

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), fmt.Errorf("gofmt jsonhooks_gen.go: %w", err)
	}
	return formatted, nil
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
	out := make([]polyInterface, 0, len(seen))
	for _, v := range seen {
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

package bmmgen

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// LOCATABLE identity surface (ADR 0013).
//
// The generator emits, for every owned concrete LOCATABLE descendant:
//   - four value-receiver Get<Field> accessors (widening the sealed
//     Locatable interface — value receivers because both value and
//     pointer forms occur in interface positions), and
//   - four pointer-receiver Set<Field> mutators collected behind the
//     sealed MutableLocatable interface (satisfied by *T only).
//
// Accessor and parameter types are resolved through the SAME
// single-property type rules as struct-field rendering
// ([singlePropTypeExpr]), so the surface cannot drift from the field
// declarations. Getter signatures resolve once at the LOCATABLE level
// (uniformity is required for interface satisfaction); descendant
// field values convert implicitly, and a descendant whose field type
// ever diverged would fail the setter assignment at codegen-verify
// time — loudly, by construction.

// locatableClassName is the BMM class whose descendants carry the
// identity surface.
const locatableClassName = "LOCATABLE"

// identityProps lists the LOCATABLE properties exposed by the surface,
// in emission order. feeder_audit and links are deliberately excluded
// (not identity; no polymorphic consumer).
var identityProps = []string{"archetype_node_id", "name", "uid", "archetype_details"}

// identityPropTypes resolves the Go type expression for each identity
// property from the LOCATABLE class definition itself.
func identityPropTypes(plan *Plan, pc *PlannedClass) (map[string]string, error) {
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return nil, fmt.Errorf("identity surface: %s is not a simple class", pc.BMMName)
	}
	types := make(map[string]string, len(identityProps))
	for _, name := range identityProps {
		prop, ok := sc.Properties[name]
		if !ok {
			return nil, fmt.Errorf("identity surface: %s lacks property %q", pc.BMMName, name)
		}
		sp, ok := prop.(*bmm.SingleProperty)
		if !ok {
			return nil, fmt.Errorf("identity surface: %s.%s is %T, want SingleProperty", pc.BMMName, name, prop)
		}
		expr, err := singlePropTypeExpr(plan, sc, pc.BMMName, sp)
		if err != nil {
			return nil, err
		}
		types[name] = expr
	}
	return types, nil
}

// emitLocatableGetterSignatures writes the Get<Field> signatures into
// the body of the Locatable interface declaration.
func emitLocatableGetterSignatures(b *strings.Builder, plan *Plan, pc *PlannedClass) error {
	types, err := identityPropTypes(plan, pc)
	if err != nil {
		return err
	}
	b.WriteString("\n\t// Generated identity accessors (ADR 0013): Get<Field> returns\n")
	b.WriteString("\t// the flattened LOCATABLE field verbatim. Value receivers — both\n")
	b.WriteString("\t// T and *T satisfy Locatable; calling a getter on a typed-nil *T\n")
	b.WriteString("\t// panics, so guard with IsTypedNil first (see rm.IsTypedNil).\n")
	for _, name := range identityProps {
		fmt.Fprintf(b, "\tGet%s() %s\n", FieldName(name), types[name])
	}
	return nil
}

// emitLocatableIdentity writes the MutableLocatable interface plus the
// per-descendant getter/setter methods. Called after the marker
// methods so the generated file reads: interface, markers, identity.
func emitLocatableIdentity(b *strings.Builder, plan *Plan, pc *PlannedClass) error {
	types, err := identityPropTypes(plan, pc)
	if err != nil {
		return err
	}

	// MutableLocatable — sealed by the same marker; pointer-receiver
	// setters mean only *T satisfies it.
	b.WriteString("\n// MutableLocatable is the write half of the generated LOCATABLE\n")
	b.WriteString("// identity surface (ADR 0013). Setters use pointer receivers, so the\n")
	b.WriteString("// interface is satisfied by *T only; it shares Locatable's unexported\n")
	b.WriteString("// marker and cannot be implemented outside this package.\n")
	b.WriteString("type MutableLocatable interface {\n")
	fmt.Fprintf(b, "\tis%s()\n", pc.GoName)
	for _, name := range identityProps {
		fmt.Fprintf(b, "\tSet%s(%s)\n", FieldName(name), types[name])
	}
	b.WriteString("}\n")

	// Per-descendant methods, in the same deterministic order as the
	// marker methods.
	for _, dn := range plan.AbstractDescendants[pc.BMMName] {
		dc, ok := plan.Classes[dn]
		if !ok {
			continue
		}
		desc, isSimple := dc.Class.(*bmm.SimpleClass)
		if !isSimple {
			continue
		}
		recv := dc.GoName + genericReceiverParams(desc)
		for _, name := range identityProps {
			field := FieldName(name)
			fmt.Fprintf(b, "\nfunc (x %s) Get%s() %s { return x.%s }\n", recv, field, types[name], field)
		}
		for _, name := range identityProps {
			field := FieldName(name)
			fmt.Fprintf(b, "\nfunc (x *%s) Set%s(v %s) { x.%s = v }\n", recv, field, types[name], field)
		}
	}
	return nil
}

// reverseRegistryArms returns, for one concrete class, the Go type
// expressions its reverse-lookup arms must cover. Non-generic classes
// yield their GoName. Generic classes yield one instantiation per
// possible type argument: the parameter's bound itself plus every
// owned concrete descendant of the bound — the closed set a consumer
// can actually hold (e.g. DVInterval[DVOrdered], DVInterval[DVQuantity],
// …; History[ItemStructure], History[ItemTree], …).
func reverseRegistryArms(plan *Plan, pc *PlannedClass) ([]string, error) {
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok || !sc.IsGeneric() {
		return []string{pc.GoName}, nil
	}
	keys := sortedStringKeys(sc.GenericParameterDefs)
	if len(keys) != 1 {
		return nil, fmt.Errorf("reverse registry: %s has %d type parameters; only single-parameter generics are supported", pc.BMMName, len(keys))
	}
	// The registry's own default instantiation is ALWAYS covered —
	// resolved by the same function the Register call uses, so parity
	// with the forward registration holds by construction.
	seen := map[string]bool{}
	arms := []string{pc.GoName + defaultGenericArgs(plan, sc)}
	seen[arms[0]] = true
	// When the parameter's bound is a planned class, additionally cover
	// every owned concrete descendant of the bound — the closed set of
	// instantiations a consumer can hold (DVInterval[DVQuantity], …).
	// Unbounded or unplanned bounds (ORIGINAL_VERSION's T, BASE's
	// Ordered) keep only the default arm: their space is open or
	// external, and no hand-written consumer enumerated them either.
	bound := sc.GenericParameterDefs[keys[0]].ConformsToType
	if bound == "" {
		bound = inheritedGenericBound(plan, sc, keys[0])
	}
	if _, ok := plan.Classes[bound]; ok {
		for _, dn := range plan.AbstractDescendants[bound] {
			dc, ok := plan.Classes[dn]
			if !ok {
				continue
			}
			if dsc, isSimple := dc.Class.(*bmm.SimpleClass); isSimple && dsc.IsGeneric() {
				// A generic argument would need its own fan-out; none
				// exists in the pinned corpus — fail loudly if one appears.
				return nil, fmt.Errorf("reverse registry: %s bound descendant %s is itself generic", pc.BMMName, dn)
			}
			arm := fmt.Sprintf("%s[%s]", pc.GoName, qualifyClassRef(plan, dc))
			if !seen[arm] {
				seen[arm] = true
				arms = append(arms, arm)
			}
		}
	}
	return arms, nil
}

// emitReverseRegistry writes RMTypeName and IsTypedNil — the reverse
// half of the type registry (ADR 0013). RMTypeName maps a value's
// concrete Go type back to the bare BMM class name it was registered
// under (generic instantiations all map to the unparameterised name);
// IsTypedNil reports whether v is an interface value carrying a
// typed-nil pointer to any registered concrete.
func emitReverseRegistry(b *bytes.Buffer, plan *Plan, concrete []*PlannedClass) error {
	b.WriteString("\n// RMTypeName returns the RM class name for v's concrete Go type —\n")
	b.WriteString("// the reverse of the typereg registration above (ADR 0013). Generic\n")
	b.WriteString("// instantiations map to the bare class name (e.g. DVInterval[DVQuantity]\n")
	b.WriteString("// → \"DV_INTERVAL\"). A nil interface, a typed-nil pointer, or a non-RM\n")
	b.WriteString("// value reports (\"\", false). REQ-024: no reflection.\n")
	b.WriteString("func RMTypeName(v any) (string, bool) {\n")
	b.WriteString("\tswitch x := v.(type) {\n")
	for _, pc := range concrete {
		arms, err := reverseRegistryArms(plan, pc)
		if err != nil {
			return err
		}
		for _, arm := range arms {
			fmt.Fprintf(b, "\tcase *%s:\n\t\tif x == nil {\n\t\t\treturn \"\", false\n\t\t}\n\t\treturn %q, true\n", arm, pc.BMMName)
			fmt.Fprintf(b, "\tcase %s:\n\t\treturn %q, true\n", arm, pc.BMMName)
		}
	}
	b.WriteString("\t}\n\treturn \"\", false\n}\n")

	b.WriteString("\n// IsTypedNil reports whether v is an interface value carrying a\n")
	b.WriteString("// typed-nil pointer to a registered RM concrete (ADR 0013). Bare nil\n")
	b.WriteString("// interfaces and value-typed structs report false. Consumers use it\n")
	b.WriteString("// as the guard before calling Locatable getters (a getter on a\n")
	b.WriteString("// typed-nil *T panics). REQ-024: no reflection.\n")
	b.WriteString("func IsTypedNil(v any) bool {\n")
	b.WriteString("\tswitch x := v.(type) {\n")
	for _, pc := range concrete {
		arms, err := reverseRegistryArms(plan, pc)
		if err != nil {
			return err
		}
		for _, arm := range arms {
			fmt.Fprintf(b, "\tcase *%s:\n\t\treturn x == nil\n", arm)
		}
	}
	b.WriteString("\t}\n\treturn false\n}\n")
	return nil
}

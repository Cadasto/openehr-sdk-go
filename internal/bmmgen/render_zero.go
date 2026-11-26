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

// The zero tests of the RM target (wire.md § REQ-052 *An absent mandatory
// member*, clinical-modeling.md § REQ-171 *Structural access*).
//
// [RenderZeroFile] emits <pkg>/zero_gen.go: one zero test per class held by
// value that something reads as empty, each comparing every field with its
// zero value ([rmaccessGen.renderZeroTest]). Three readers share them:
//
//   - the canonical JSON and XML encoders, which leave out a mandatory member
//     held by value as a struct when its zero test holds ([emptyMember]);
//   - the interval-bound emptiness test, IsEmptyIntervalBound, for a bound of
//     a concrete class;
//   - the validation floor in internal/rmaccess, which reads a member held by
//     value as absent when its zero test holds. The floor cannot call an
//     unexported function of package rm, so the file's init registers the
//     tests it reads with internal/rmzero, and the generated access looks
//     each one up once, when it is initialised.
//
// One test per class means the encoders and the floor cannot disagree on
// what is empty: a member the encoders leave out is one the floor already
// reads as absent.

// zeroFile is the generated file that holds a target's zero tests.
const zeroFile = "zero_gen.go"

// rmzeroImport is the import path of the registry the RM target's zero tests
// are handed to the validation floor through.
const rmzeroImport = "github.com/cadasto/openehr-sdk-go/internal/rmzero"

// emptyHeldFunc names the generated function that reads the value of a
// mandatory member typed by its class's own generic parameter as empty.
const emptyHeldFunc = "isEmptyHeld"

// emptyMember is a mandatory member of an RM class that the canonical
// encoders leave out when its value is empty: the member's Go field holds a
// struct directly, not through a pointer, interface, list or map, and the
// floor reads it by that struct's zero test, through the content rule of its
// declared type or through the zero test itself. Each content rule reads a
// zero value as absent, and the zero test reads a value with any field set as
// present, so a member left out is always one the floor reports as missing,
// and a partly filled one is written in full. A member whose presence a
// fixed reading decides ([fixedPresence]) is always written: an EHR_STATUS
// subject with no external reference is a PARTY_SELF, not an absent one.
type emptyMember struct {
	// test is the zero test of the struct the field holds, "" for a member
	// typed by the class's own generic parameter.
	test string
	// param marks a member typed by the class's own generic parameter. Its
	// value is empty when the parameter is bound to a struct whose zero test
	// holds, which [emptyHeldFunc] reads at run time; held lists the
	// structs the parameter's bound admits by value.
	param bool
	held  []*goType
}

// cond returns the Go condition under which member field of recv is written.
func (m emptyMember) cond(field string) string {
	if m.param {
		return fmt.Sprintf("!%s(%s)", emptyHeldFunc, field)
	}
	return fmt.Sprintf("!%s(%s)", m.test, field)
}

// emptyMembers returns, by BMM attribute name, the members of class pc that
// the canonical encoders leave out when empty ([emptyMember]). Only the RM
// target has them: the floor that reads such a member as absent is the RM's.
// The members of a generic class are read with its own type parameters, so
// the encoders its methods carry apply to every instantiation.
func (g *rmaccessGen) emptyMembers(pc *PlannedClass) (map[string]emptyMember, error) {
	if g.plan.Target.GoPackage != "rm" {
		return nil, nil
	}
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return nil, fmt.Errorf("bmmgen: empty members: %s is not a simple class", pc.BMMName)
	}
	self := &goType{kind: goNamed, name: pc.GoName}
	params := map[string]bool{}
	for _, k := range sortedStringKeys(sc.GenericParameterDefs) {
		self.args = append(self.args, &goType{kind: goNamed, name: k})
		params[k] = true
	}
	binds, err := g.bindings(pc, self)
	if err != nil {
		return nil, err
	}
	interval, err := intervalShaped(g.plan, pc)
	if err != nil {
		return nil, err
	}
	declared, _ := effectiveProperties(g.plan, pc.BMMName)
	fields, err := effectiveFields(g.plan, pc)
	if err != nil {
		return nil, err
	}
	out := map[string]emptyMember{}
	for _, ef := range fields {
		name := ef.Prop.PropertyName()
		if !declared[name].required || declared[name].container {
			continue
		}
		if _, fixed := fixedPresence[pc.BMMName][name]; fixed {
			continue
		}
		if interval && (name == propLower || name == propUpper) {
			continue
		}
		t, err := g.fieldType(ef.Owner, ef.OwnerName, ef.Prop, binds[ef.OwnerName])
		if err != nil {
			return nil, err
		}
		if t.kind != goNamed {
			continue
		}
		if params[t.name] && len(t.args) == 0 {
			held, err := g.heldStructs(sc, t.name)
			if err != nil {
				return nil, fmt.Errorf("bmmgen: empty members: %s.%s: %w", pc.BMMName, name, err)
			}
			if len(held) > 0 {
				out[name] = emptyMember{param: true, held: held}
			}
			continue
		}
		if _, ok := g.structs[t.name]; !ok {
			continue
		}
		if mentionsAny(t, params) {
			return nil, fmt.Errorf("bmmgen: empty members: %s.%s holds %s by value, whose zero test depends on the class's type parameter", pc.BMMName, name, t.spell(""))
		}
		test, err := g.zeroTest(t)
		if err != nil {
			return nil, fmt.Errorf("bmmgen: empty members: %s.%s: %w", pc.BMMName, name, err)
		}
		out[name] = emptyMember{test: test}
	}
	return out, nil
}

// guardLines wraps lines, statements at one tab of indent, in `if cond { … }`.
func guardLines(cond, lines string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\tif %s {\n", cond)
	for line := range strings.Lines(lines) {
		b.WriteString("\t" + line)
	}
	b.WriteString("\t}\n")
	return b.String()
}

// emptyMembersOf returns the members of class pc that the canonical encoders
// leave out when empty, by BMM attribute name ([rmaccessGen.emptyMembers]).
func emptyMembersOf(plan *Plan, pc *PlannedClass) (map[string]emptyMember, error) {
	if plan.Target.GoPackage != "rm" {
		return nil, nil
	}
	g, err := newRMAccessGen(plan)
	if err != nil {
		return nil, err
	}
	return g.emptyMembers(pc)
}

// heldStructs returns the structs that sc's type parameter param admits by
// value: the class its bound names, declared on the class or inherited as the
// struct renderer reads it, and that class's concrete descendants, each a
// non-generic class rendered as a Go struct, sorted by Go name.
func (g *rmaccessGen) heldStructs(sc *bmm.SimpleClass, param string) ([]*goType, error) {
	def, ok := sc.GenericParameterDefs[param]
	if !ok || def == nil {
		return nil, fmt.Errorf("type parameter %s is not declared", param)
	}
	bound := def.ConformsToType
	if bound == "" {
		bound = inheritedGenericBound(g.plan, sc, param)
	}
	if bound == "" {
		return nil, nil
	}
	names := append([]string{bound}, g.plan.AbstractDescendants[bound]...)
	names = append(names, g.plan.ConcreteSubtypes[bound]...)
	var out []*goType
	seen := map[string]bool{}
	for _, name := range names {
		pc, ok := g.plan.Classes[name]
		if !ok || seen[pc.GoName] {
			continue
		}
		seen[pc.GoName] = true
		if _, ok := g.structs[pc.GoName]; !ok {
			continue
		}
		if c, ok := pc.Class.(*bmm.SimpleClass); !ok || c.IsGeneric() {
			continue
		}
		out = append(out, &goType{kind: goNamed, name: pc.GoName})
	}
	slices.SortFunc(out, func(a, b *goType) int { return strings.Compare(a.name, b.name) })
	return out, nil
}

// mentionsAny reports whether t names any of names, itself or in a type
// argument.
func mentionsAny(t *goType, names map[string]bool) bool {
	if t == nil {
		return false
	}
	if t.kind != goNamed {
		return mentionsAny(t.key, names) || mentionsAny(t.elem, names)
	}
	if names[t.name] {
		return true
	}
	return slices.ContainsFunc(t.args, func(a *goType) bool { return mentionsAny(a, names) })
}

// renderAccessAndZero renders the RM attribute access, nil for a target other
// than the RM, and the target's zero file from one generator, so the zero
// file registers exactly the tests the access looks up.
func renderAccessAndZero(plan *Plan) (access, zero []byte, err error) {
	g, err := newRMAccessGen(plan)
	if err != nil {
		return nil, nil, err
	}
	var floor []string
	if plan.Target.GoPackage == "rm" {
		if access, err = g.render(); err != nil {
			return nil, nil, err
		}
		floor = slices.Sorted(maps.Keys(g.zeroTests))
	}
	zero, err = g.renderZeroFile(floor)
	if err != nil {
		return nil, nil, err
	}
	return access, zero, nil
}

// RenderZeroFile renders <pkg>/zero_gen.go for a target: the zero tests its
// interval-bound emptiness test, its canonical encoders and, for the RM
// target, the validation floor read, and those they call. Returns (nil, nil)
// when the target reads none.
func RenderZeroFile(plan *Plan) ([]byte, error) {
	_, zero, err := renderAccessAndZero(plan)
	return zero, err
}

// renderZeroFile renders the zero file once the floor's render has registered
// the tests it reads, named by floor.
func (g *rmaccessGen) renderZeroFile(floor []string) ([]byte, error) {
	shaped, bounds, err := intervalBounds(g.plan)
	if err != nil {
		return nil, err
	}
	if shaped {
		for _, pc := range bounds {
			if _, err := g.zeroTest(&goType{kind: goNamed, name: pc.GoName}); err != nil {
				return nil, fmt.Errorf("bmmgen: interval bound %s: %w", pc.BMMName, err)
			}
		}
	}
	held := map[string]*goType{}
	for _, file := range g.plan.Files {
		classes, err := codecClassesIn(g.plan, file)
		if err != nil {
			return nil, err
		}
		for _, pc := range classes {
			members, err := g.emptyMembers(pc)
			if err != nil {
				return nil, err
			}
			for _, m := range members {
				for _, t := range m.held {
					name, err := g.zeroTest(t)
					if err != nil {
						return nil, err
					}
					held[name] = t
				}
			}
		}
	}
	if len(g.zeroTests) == 0 {
		return nil, nil
	}

	var tests bytes.Buffer
	if err := g.writeZeroTests(&tests); err != nil {
		return nil, err
	}

	var b bytes.Buffer
	b.WriteString(renderGeneratedHeader(g.plan))
	if len(floor) > 0 {
		fmt.Fprintf(&b, "\nimport %q\n", rmzeroImport)
		b.WriteString("\n// init hands the validation floor the zero tests it reads a member held\n")
		b.WriteString("// by value through, so where it reads a mandatory member by its zero\n")
		b.WriteString("// value, it reads it as absent exactly when the canonical encoders leave\n")
		b.WriteString("// it out.\n")
		b.WriteString("func init() {\n\trmzero.Register(\n")
		for _, name := range floor {
			fmt.Fprintf(&b, "\t\t%s,\n", name)
		}
		b.WriteString("\t)\n}\n")
	}
	if len(held) > 0 {
		writeEmptyHeld(&b, held)
	}
	b.Write(tests.Bytes())

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), fmt.Errorf("gofmt %s: %w", zeroFile, err)
	}
	return formatted, nil
}

// writeEmptyHeld writes [emptyHeldFunc], which reads the value of a mandatory
// member typed by its class's own generic parameter: empty when the parameter
// is bound to one of the structs in held, by name, and the value is that
// struct's zero value. A parameter bound to an interface holds its value
// through the interface, so whatever the interface holds is written.
func writeEmptyHeld(b *bytes.Buffer, held map[string]*goType) {
	fmt.Fprintf(b, "\n// %s reports whether v, the value of a mandatory member typed by its\n", emptyHeldFunc)
	b.WriteString("// class's own type parameter, is empty: T is a class held by value and v\n")
	b.WriteString("// is its zero value, every field empty. A T that is an interface holds\n")
	b.WriteString("// its value through the interface, which is never empty in this sense.\n")
	fmt.Fprintf(b, "func %s[T any](v T) bool {\n", emptyHeldFunc)
	b.WriteString("\tvar zero T\n")
	b.WriteString("\tif any(zero) == nil {\n\t\treturn false\n\t}\n")
	b.WriteString("\tswitch x := any(v).(type) {\n")
	for _, name := range slices.Sorted(maps.Keys(held)) {
		fmt.Fprintf(b, "\tcase %s:\n\t\treturn %s(x)\n", held[name].spell(""), name)
	}
	b.WriteString("\t}\n\treturn false\n}\n")
}

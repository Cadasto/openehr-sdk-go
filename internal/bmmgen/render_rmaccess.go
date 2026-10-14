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

// Generated RM attribute access (REQ-171 *Structural access*, ADR 0028).
//
// [RenderRMAccessFile] emits internal/rmaccess/access_gen.go: for every
// concrete class the type registry decodes, in every instantiation the
// reverse registry recognises (ADR 0013), a reader that answers each
// attribute the class declares, own and inherited, with its value and
// whether it is present, by plain field access, and a reader of the entries of
// each Hash attribute in ascending key order. The Go type of each field is
// read from the line the struct renderer writes for it ([renderField]), so
// the access cannot drift from the struct declarations; a field whose Go type
// the access misreads fails to compile in the generated file.
//
// Presence is decided as § REQ-171 *Structural access* states it: by the
// field's Go kind (pointer, interface, string, number or boolean, list), and
// for a value held by value by a generated zero test; at an attribute the RM
// marks mandatory, by a content rule keyed by the attribute's declared RM
// type ([contentPresence]); and, for the three readings § REQ-112 fixes, by
// the hand-written functions [fixedPresence] and [intervalBoundPresence]
// name. The content rules and the fixed readings are hand-written in
// internal/rmaccess/presence.go; this file names them, so a renamed or
// retyped function fails to compile.

// rmaccessRMImport is the import path of the RM package the access reads.
const rmaccessRMImport = "github.com/cadasto/openehr-sdk-go/openehr/rm"

// rmaccessFile is the generated access file, under internal/rmaccess.
const rmaccessFile = "access_gen.go"

// fixedPresence names, by class and attribute, the hand-written reading that
// decides presence in place of the field's Go kind for the two value-typed
// readings § REQ-112 fixes: EHR_STATUS subject is always present (*Value-typed
// mandatory presence*), and so are ARCHETYPED archetype_id and rm_version,
// whose emptiness is the *ARCHETYPED* entry's to report. The third fixed
// reading, the interval bound, applies to every interval-shaped class and is
// [intervalBoundPresence].
var fixedPresence = map[string]map[string]string{
	"EHR_STATUS": {"subject": "presentEHRStatusSubject"},
	"ARCHETYPED": {"archetype_id": "presentArchetypedID", "rm_version": "presentArchetypedRMVersion"},
}

// intervalBoundPresence is the hand-written reading of an interval bound
// (§ REQ-112 *Driver*): absent only when its side is marked unbounded and the
// bound is empty. It applies to `lower` and `upper` of every class whose
// fields carry the BASE Interval shape ([hasIntervalShape]).
const intervalBoundPresence = "presentIntervalBound"

// contentPresence names, by declared RM type, the hand-written content rule
// that decides presence at an attribute the RM marks mandatory, for a value
// held by value (and, for DV_TEXT, held through an interface): a DV_TEXT is
// empty when its value is; a DV_CODED_TEXT when its value and its defining
// code are; a CODE_PHRASE when its code_string and terminology are. At an
// optional attribute the reading of the field's Go kind decides instead, so
// the walk enters such a value and reports what it lacks. A pointer, and an
// interface for any other type, take the reading of their Go kind
// everywhere. An object reference has no content rule: held by value, it is
// present unless it is its zero value, so § REQ-112's reference entry reports
// each missing part at its own path.
var contentPresence = map[string]struct {
	fn    string
	kinds []accessKind
}{
	"DV_TEXT":       {"presentText", []accessKind{accessInterface, accessStruct}},
	"DV_CODED_TEXT": {"presentCodedText", []accessKind{accessStruct}},
	"CODE_PHRASE":   {"presentCodePhrase", []accessKind{accessStruct}},
}

// hashType is the declared RM type of a Hash attribute, as the fold rminfo is
// generated from names a BMM Hash<K, V> ([effectiveProperties]). Its Go field
// is a map keyed by string, or a pointer to one.
const hashType = "Hash"

// accessShape is how the access hands back an attribute's value when that is
// not the field itself.
type accessShape int

const (
	// shapeField hands back the field itself.
	shapeField accessShape = iota
	// shapeAddress hands back a pointer to a field held by value, into the
	// parent, so a reader can change the value in place.
	shapeAddress
	// shapeDeref hands back the value an optional field points to.
	shapeDeref
)

// accessShapes keeps, for six attributes of five classes, the shape the
// validation walks and the composition builder read, where it differs from
// the field's own: ITEM_SINGLE item and REFERENCE_RANGE range come back as a
// pointer into the parent (the builder writes through the first);
// ISM_TRANSITION careflow_step and transition come back as the DV_CODED_TEXT
// they point to, and DV_PROPORTION precision as its Integer, which the
// primitive binding reads; ORIGINAL_VERSION data comes back as the value its
// pointer holds, the payload of a version built in Go. That field is a
// pointer to the type parameter, which the one instantiation the registry
// recognises binds to any, and no walk can enter a *any. Presence is read
// from the field either way, so a pointer that is not nil and holds a nil
// interface comes back as nil and present.
var accessShapes = map[string]map[string]accessShape{
	"ITEM_SINGLE":      {"item": shapeAddress},
	"REFERENCE_RANGE":  {"range": shapeAddress},
	"ISM_TRANSITION":   {"careflow_step": shapeDeref, "transition": shapeDeref},
	"DV_PROPORTION":    {"precision": shapeDeref},
	"ORIGINAL_VERSION": {"data": shapeDeref},
}

// accessKind is the Go kind of a field, which decides its reading by kind.
type accessKind int

const (
	accessPointer accessKind = iota
	accessInterface
	accessString
	accessBool
	accessNumber
	accessList
	accessStruct
)

// rmPrimitiveKinds gives the Go kind of the named RM types the BMM primitives
// map to ([primitiveGoType]). A primitive mapped to a named type missing here
// fails generation.
var rmPrimitiveKinds = map[string]accessKind{
	"Character": accessString,
	"Integer":   accessNumber,
	"Real":      accessNumber,
}

// goPredeclaredKinds gives the Go kind of the predeclared types a field can
// carry. A predeclared name outside it fails generation.
var goPredeclaredKinds = map[string]accessKind{
	"any": accessInterface, "error": accessInterface,
	"string": accessString,
	"bool":   accessBool,
	"byte":   accessNumber, "rune": accessNumber,
	"int": accessNumber, "int8": accessNumber, "int16": accessNumber, "int32": accessNumber, "int64": accessNumber,
	"uint": accessNumber, "uint8": accessNumber, "uint16": accessNumber, "uint32": accessNumber, "uint64": accessNumber,
	"uintptr": accessNumber, "float32": accessNumber, "float64": accessNumber,
	"complex64": accessNumber, "complex128": accessNumber,
}

// RenderRMAccessFile produces internal/rmaccess/access_gen.go from a planned
// RM emission: the generated half of the attribute access the validators
// read RM values through.
//
// It emits, for every class in [Plan.ConcreteClasses] and every instantiation
// [reverseRegistryArms] lists for it, a reader of the single-valued
// attributes and a reader of the containers the class declares, the type
// switches single, multiple, hash and covers that dispatch on a value's Go
// type (pointer and value form alike), a reader of the entries of every Hash
// attribute, and a field-by-field zero test for every class an attribute
// holds by value. Which attributes a class has, whether each is a container
// and which are Hash attributes is read from the same fold rminfo is
// generated from ([effectiveProperties]).
//
// Returns nil for every target but RM. Output is deterministic and gofmt'd.
func RenderRMAccessFile(plan *Plan) ([]byte, error) {
	if plan.Target.GoPackage != "rm" {
		return nil, nil
	}
	g, err := newRMAccessGen(plan)
	if err != nil {
		return nil, err
	}
	return g.render()
}

// rmaccessGen holds the indexes one rendering reads.
type rmaccessGen struct {
	plan       *Plan
	structs    map[string]*PlannedClass          // Go name → class rendered as a Go struct
	interfaces map[string]bool                   // Go names rendered as Go interfaces
	enums      map[string]accessKind             // Go name → kind of an enumeration
	zeroTests  map[string]*goType                // zero test name → the type it tests
	shapes     map[string]map[string]accessShape // class → attribute → shape: [accessShapes]
}

func newRMAccessGen(plan *Plan) (*rmaccessGen, error) {
	g := &rmaccessGen{
		plan:       plan,
		structs:    map[string]*PlannedClass{},
		interfaces: map[string]bool{},
		enums:      map[string]accessKind{},
		zeroTests:  map[string]*goType{},
		shapes:     accessShapes,
	}
	for _, name := range sortedStringKeys(plan.Classes) {
		pc := plan.Classes[name]
		// A class the base schema lists among its primitive types is a Go
		// type of its own unless it maps to a Go primitive (Interval and the
		// other foundation structs are; String and Integer are not).
		if pc.External || isPrimitive(name) || isSkippedPrimitive(name) || isSkippedClass(name) {
			continue
		}
		switch cls := pc.Class.(type) {
		case *bmm.Enumeration:
			if cls.IsIntegerEnum() {
				g.enums[pc.GoName] = accessNumber
			} else {
				g.enums[pc.GoName] = accessString
			}
		case *bmm.Interface:
			g.interfaces[pc.GoName] = true
		case *bmm.SimpleClass:
			if cls.IsAbstract() && (!cls.IsGeneric() || codecPolymorphicAbstractGeneric(plan, pc)) {
				g.interfaces[pc.GoName] = true
			} else {
				g.structs[pc.GoName] = pc
			}
			if like, ok := narrowInterfaceGoName(plan, name); ok {
				g.interfaces[like] = true
			}
		}
	}
	for _, name := range sortedStringKeys(primitiveGoType) {
		goType := primitiveGoType[name]
		if _, ok := goPredeclaredKinds[goType]; ok {
			continue
		}
		if _, ok := rmPrimitiveKinds[goType]; !ok {
			return nil, fmt.Errorf("bmmgen: rmaccess: primitive %s maps to Go %s, whose kind is not known", name, goType)
		}
	}
	return g, nil
}

// accessArm is one Go type a registered class is held as: the class itself,
// or one instantiation of a generic class.
type accessArm struct {
	class *PlannedClass
	typ   *goType                       // unqualified, e.g. DVInterval[DVQuantity]
	binds map[string]map[string]*goType // BMM class → type parameter → argument
}

// accessAttr is one attribute of an arm, with its Go field.
type accessAttr struct {
	name      string // BMM attribute name
	field     string // Go field name
	declared  string // declared RM type
	container bool
	required  bool    // the RM marks it mandatory, as rminfo's Required
	typ       *goType // the field's Go type at the arm's instantiation
	kind      accessKind
}

// arms returns the Go types pc is held as, in the order the reverse registry
// lists them.
func (g *rmaccessGen) arms(pc *PlannedClass) ([]*accessArm, error) {
	names, err := reverseRegistryArms(g.plan, pc)
	if err != nil {
		return nil, err
	}
	out := make([]*accessArm, 0, len(names))
	for _, name := range names {
		t, err := parseGoType(name)
		if err != nil {
			return nil, fmt.Errorf("bmmgen: rmaccess: arm of %s: %w", pc.BMMName, err)
		}
		binds, err := g.bindings(pc, t)
		if err != nil {
			return nil, err
		}
		out = append(out, &accessArm{class: pc, typ: t, binds: binds})
	}
	return out, nil
}

// bindings returns the type arguments of pc instantiated as t, and of every
// struct ancestor pc embeds, as the struct renderer instantiates them: an
// embedded generic ancestor of the same arity takes the descendant's
// parameters in order, one of another arity its own default instantiation.
func (g *rmaccessGen) bindings(pc *PlannedClass, t *goType) (map[string]map[string]*goType, error) {
	out := map[string]map[string]*goType{}
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return nil, fmt.Errorf("bmmgen: rmaccess: %s is not a simple class", pc.BMMName)
	}
	own, err := bindParams(sc, t.args)
	if err != nil {
		return nil, fmt.Errorf("bmmgen: rmaccess: %s: %w", pc.BMMName, err)
	}
	out[pc.BMMName] = own
	var visit func(cur *PlannedClass, curSC *bmm.SimpleClass, curArgs map[string]*goType) error
	visit = func(cur *PlannedClass, curSC *bmm.SimpleClass, curArgs map[string]*goType) error {
		_, ancestors := embeddedStructAncestors(g.plan, cur)
		for _, ap := range ancestors {
			asc := ap.Class.(*bmm.SimpleClass)
			var args []*goType
			switch {
			case !asc.IsGeneric():
			case len(asc.GenericParameterDefs) == len(curSC.GenericParameterDefs):
				for _, k := range sortedStringKeys(curSC.GenericParameterDefs) {
					args = append(args, curArgs[k])
				}
			default:
				dt, err := parseGoType(ap.GoName + defaultGenericArgs(g.plan, asc))
				if err != nil {
					return fmt.Errorf("bmmgen: rmaccess: %s embeds %s: %w", cur.BMMName, ap.BMMName, err)
				}
				args = dt.args
			}
			apArgs, err := bindParams(asc, args)
			if err != nil {
				return fmt.Errorf("bmmgen: rmaccess: %s embeds %s: %w", cur.BMMName, ap.BMMName, err)
			}
			out[ap.BMMName] = apArgs
			if err := visit(ap, asc, apArgs); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(pc, sc, own); err != nil {
		return nil, err
	}
	return out, nil
}

// bindParams binds sc's type parameters, in sorted order, to args.
func bindParams(sc *bmm.SimpleClass, args []*goType) (map[string]*goType, error) {
	keys := sortedStringKeys(sc.GenericParameterDefs)
	if len(keys) != len(args) {
		return nil, fmt.Errorf("%d type arguments for %d type parameters", len(args), len(keys))
	}
	if len(keys) == 0 {
		return nil, nil
	}
	out := make(map[string]*goType, len(keys))
	for i, k := range keys {
		if args[i] == nil {
			return nil, fmt.Errorf("type parameter %s has no argument", k)
		}
		out[k] = args[i]
	}
	return out, nil
}

// attributes returns the attributes of arm, sorted by name: every attribute
// the class declares, own and inherited, each with the Go field the struct
// renderer declares for it.
func (g *rmaccessGen) attributes(arm *accessArm) ([]*accessAttr, error) {
	pc := arm.class
	declared, _ := effectiveProperties(g.plan, pc.BMMName)
	fields, err := effectiveFields(g.plan, pc)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]emittedField, len(fields))
	for _, ef := range fields {
		byName[ef.Prop.PropertyName()] = ef
	}
	for name := range byName {
		if _, ok := declared[name]; !ok {
			return nil, fmt.Errorf("bmmgen: rmaccess: %s has a Go field for %q, which the class does not declare", pc.BMMName, name)
		}
	}
	out := make([]*accessAttr, 0, len(declared))
	for _, name := range sortedStringKeys(declared) {
		ef, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("bmmgen: rmaccess: %s declares %q, which has no Go field", pc.BMMName, name)
		}
		typ, err := g.fieldType(ef.Owner, ef.OwnerName, ef.Prop, arm.binds[ef.OwnerName])
		if err != nil {
			return nil, err
		}
		kind, err := g.classify(typ)
		if err != nil {
			return nil, fmt.Errorf("bmmgen: rmaccess: %s.%s: %w", pc.BMMName, name, err)
		}
		out = append(out, &accessAttr{
			name:      name,
			field:     FieldName(name),
			declared:  declared[name].typeName,
			container: declared[name].container,
			required:  declared[name].required,
			typ:       typ,
			kind:      kind,
		})
	}
	return out, nil
}

// fieldType returns the Go type of prop's field, as the struct renderer
// declares it in owner, with owner's type parameters bound by binds.
func (g *rmaccessGen) fieldType(owner *bmm.SimpleClass, ownerName string, prop bmm.Property, binds map[string]*goType) (*goType, error) {
	line, err := renderField(g.plan, owner, ownerName, prop)
	if err != nil {
		return nil, err
	}
	expr, err := fieldDeclType(line, FieldName(prop.PropertyName()))
	if err != nil {
		return nil, fmt.Errorf("bmmgen: rmaccess: %s.%s: %w", ownerName, prop.PropertyName(), err)
	}
	t, err := parseGoType(expr)
	if err != nil {
		return nil, fmt.Errorf("bmmgen: rmaccess: %s.%s: %w", ownerName, prop.PropertyName(), err)
	}
	return t.bind(binds), nil
}

// fieldDeclType returns the type of the field declaration the struct renderer
// wrote for goName: the last line of rendered, "\t<goName> <type> <tag>".
func fieldDeclType(rendered, goName string) (string, error) {
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	line, ok := strings.CutPrefix(lines[len(lines)-1], "\t"+goName+" ")
	if !ok {
		return "", fmt.Errorf("field line %q does not declare %s", lines[len(lines)-1], goName)
	}
	typ, _, ok := strings.CutLast(line, " `")
	if !ok {
		return "", fmt.Errorf("field line %q has no tag", line)
	}
	return strings.TrimSpace(typ), nil
}

// classify returns the Go kind of t.
func (g *rmaccessGen) classify(t *goType) (accessKind, error) {
	switch t.kind {
	case goPointer:
		return accessPointer, nil
	case goSlice, goMap:
		return accessList, nil
	case goNamed:
	}
	if k, ok := goPredeclaredKinds[t.name]; ok {
		return k, nil
	}
	if k, ok := rmPrimitiveKinds[t.name]; ok {
		return k, nil
	}
	if k, ok := g.enums[t.name]; ok {
		return k, nil
	}
	if g.interfaces[t.name] {
		return accessInterface, nil
	}
	if _, ok := g.structs[t.name]; ok {
		return accessStruct, nil
	}
	return 0, fmt.Errorf("type %s is neither a planned class nor a known primitive", t.name)
}

// armAttrs is one arm with its attributes, split by the reader that answers
// them: a Hash attribute is in single and in hash.
type armAttrs struct {
	arm    *accessArm
	single []*accessAttr
	multi  []*accessAttr
	hash   []*accessAttr
}

// split sorts the attributes of arm by the reader that answers them: a
// container by multiple, any other attribute by single, and a Hash attribute
// by hash as well, so a walk can enter its entries. It refuses a container
// that is not a Go slice, a Hash that is not a map keyed by string, held
// directly or through a pointer, and a Go map on an attribute not declared
// Hash: a map whose entries no reader serves would reach a walk as one opaque
// value.
func (g *rmaccessGen) split(arm *accessArm, attrs []*accessAttr) (armAttrs, error) {
	entry := armAttrs{arm: arm}
	for _, a := range attrs {
		if a.container {
			if a.typ.kind != goSlice {
				// A container held as a Go map would need its items' order
				// decided before the access boxes them; no container of the
				// pinned RM is one.
				return armAttrs{}, fmt.Errorf("bmmgen: rmaccess: container %s.%s has Go type %s, not a slice", arm.class.BMMName, a.name, a.typ.qualified())
			}
			entry.multi = append(entry.multi, a)
			continue
		}
		entry.single = append(entry.single, a)
		m, isMap := hashMap(a.typ)
		switch {
		case a.declared == hashType:
			if !isMap || m.key.kind != goNamed || m.key.name != "string" {
				return armAttrs{}, fmt.Errorf("bmmgen: rmaccess: Hash %s.%s has Go type %s, not a map keyed by string", arm.class.BMMName, a.name, a.typ.qualified())
			}
			entry.hash = append(entry.hash, a)
		case isMap:
			return armAttrs{}, fmt.Errorf("bmmgen: rmaccess: %s.%s is declared %s, but its Go type %s is a map", arm.class.BMMName, a.name, a.declared, a.typ.qualified())
		}
	}
	return entry, nil
}

// hashMap returns the map t holds a Hash in: t itself, or the map t points
// to. ok is false when t is neither a map nor a pointer to one.
func hashMap(t *goType) (m *goType, ok bool) {
	switch {
	case t.kind == goMap:
		return t, true
	case t.kind == goPointer && t.elem.kind == goMap:
		return t.elem, true
	}
	return nil, false
}

// render writes the file.
func (g *rmaccessGen) render() ([]byte, error) {
	var all []armAttrs
	for _, pc := range g.plan.ConcreteClasses {
		arms, err := g.arms(pc)
		if err != nil {
			return nil, err
		}
		for _, arm := range arms {
			attrs, err := g.attributes(arm)
			if err != nil {
				return nil, err
			}
			entry, err := g.split(arm, attrs)
			if err != nil {
				return nil, err
			}
			all = append(all, entry)
		}
	}
	slices.SortStableFunc(all, func(a, b armAttrs) int {
		return strings.Compare(a.arm.class.BMMName, b.arm.class.BMMName)
	})

	var b bytes.Buffer
	b.WriteString("// Code generated by bmmgen; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "// Source: %s\n\n", g.plan.Target.SourceLabel)
	b.WriteString("package rmaccess\n\n")
	fmt.Fprintf(&b, "import %q\n", rmaccessRMImport)

	// covers: every arm, both forms.
	b.WriteString("\n// covers reports whether v is one of the RM classes the access reads,\n")
	b.WriteString("// in pointer or value form: every concrete class the type registry\n")
	b.WriteString("// decodes, in every instantiation the reverse registry recognises.\n")
	b.WriteString("func covers(v any) bool {\n\tswitch v.(type) {\n\tcase ")
	var forms []string
	for _, e := range all {
		q := e.arm.typ.qualified()
		forms = append(forms, "*"+q+", "+q)
	}
	b.WriteString(strings.Join(forms, ",\n\t\t"))
	b.WriteString(":\n\t\treturn true\n\t}\n\treturn false\n}\n")

	// single, multiple and hash dispatch.
	writeDispatch(&b, "single", "the single-valued attribute attr of parent: its value, whether it is\n// present, and whether parent's class declares attr as a single-valued\n// attribute at all", "any, bool, bool", "nil, false, false",
		all, func(e armAttrs) bool { return len(e.single) > 0 })
	writeDispatch(&b, "multiple", "the container attribute attr of parent: its list, whether the list\n// is present, and whether parent's class declares attr as a container at\n// all", "list, bool, bool", "nil, false, false",
		all, func(e armAttrs) bool { return len(e.multi) > 0 })
	writeDispatch(&b, "hash", "the Hash attribute attr of parent: the keys of its entries in\n// ascending order, their values, whether the hash is present, and whether\n// parent's class declares attr as a Hash at all", "[]string, []any, bool, bool", "nil, nil, false, false",
		all, func(e armAttrs) bool { return len(e.hash) > 0 })

	for _, e := range all {
		if len(e.single) > 0 {
			if err := g.writeSingle(&b, e.arm, e.single); err != nil {
				return nil, err
			}
		}
		if len(e.multi) > 0 {
			if err := g.writeMultiple(&b, e.arm, e.multi); err != nil {
				return nil, err
			}
		}
		if len(e.hash) > 0 {
			if err := g.writeHash(&b, e.arm, e.hash); err != nil {
				return nil, err
			}
		}
	}

	if err := g.writeZeroTests(&b); err != nil {
		return nil, err
	}

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), fmt.Errorf("gofmt internal/rmaccess/%s: %w", rmaccessFile, err)
	}
	return formatted, nil
}

// writeDispatch writes the type switch fn that hands parent to the reader of
// its arm, for every arm that has such a reader; results are the readers'
// result types and zero what fn returns for any other parent.
func writeDispatch(b *bytes.Buffer, fn, doc, results, zero string, all []armAttrs, has func(armAttrs) bool) {
	fmt.Fprintf(b, "\n// %s reads %s.\n", fn, doc)
	fmt.Fprintf(b, "func %s(parent any, attr string) (%s) {\n", fn, results)
	b.WriteString("\tswitch p := parent.(type) {\n")
	for _, e := range all {
		if !has(e) {
			continue
		}
		q := e.arm.typ.qualified()
		name := fn + e.arm.typ.suffix()
		fmt.Fprintf(b, "\tcase *%s:\n\t\treturn %s(p, attr)\n", q, name)
		fmt.Fprintf(b, "\tcase %s:\n\t\treturn %s(&p, attr)\n", q, name)
	}
	fmt.Fprintf(b, "\t}\n\treturn %s\n}\n", zero)
}

// writeSingle writes the reader of arm's single-valued attributes.
func (g *rmaccessGen) writeSingle(b *bytes.Buffer, arm *accessArm, attrs []*accessAttr) error {
	q := arm.typ.qualified()
	fmt.Fprintf(b, "\n// single%s reads the single-valued attributes of %s.\n", arm.typ.suffix(), q)
	fmt.Fprintf(b, "func single%s(x *%s, attr string) (any, bool, bool) {\n", arm.typ.suffix(), q)
	b.WriteString("\tif x == nil {\n\t\treturn nil, false, false\n\t}\n")
	b.WriteString("\tswitch attr {\n")
	interval, err := intervalShaped(g.plan, arm.class)
	if err != nil {
		return err
	}
	for _, a := range attrs {
		present, err := g.presence(arm, a, interval)
		if err != nil {
			return err
		}
		field := "x." + a.field
		fmt.Fprintf(b, "\tcase %q:\n", a.name)
		switch shape := g.shapes[arm.class.BMMName][a.name]; shape {
		case shapeField:
			fmt.Fprintf(b, "\t\treturn %s, %s, true\n", field, present)
		case shapeAddress:
			if a.kind != accessStruct {
				return fmt.Errorf("bmmgen: rmaccess: %s.%s is handed back by address, but its field is not held by value", arm.class.BMMName, a.name)
			}
			fmt.Fprintf(b, "\t\treturn &%s, %s, true\n", field, present)
		case shapeDeref:
			if a.kind != accessPointer {
				return fmt.Errorf("bmmgen: rmaccess: %s.%s is handed back dereferenced, but its field is not a pointer", arm.class.BMMName, a.name)
			}
			fmt.Fprintf(b, "\t\tif %s == nil {\n\t\t\treturn nil, false, true\n\t\t}\n", field)
			fmt.Fprintf(b, "\t\treturn *%s, true, true\n", field)
		default:
			return fmt.Errorf("bmmgen: rmaccess: %s.%s has shape %d, which the generator does not know", arm.class.BMMName, a.name, shape)
		}
	}
	b.WriteString("\t}\n\treturn nil, false, false\n}\n")
	return nil
}

// writeMultiple writes the reader of arm's containers, each a Go slice
// ([rmaccessGen.split] refuses any other), handed over as a list whose items
// are boxed only when they are read: those of a list of values as pointers
// into the backing array (boxPointers), those of a list of interfaces or
// pointers as they are (boxValues). A list is counted without boxing an
// item.
func (g *rmaccessGen) writeMultiple(b *bytes.Buffer, arm *accessArm, attrs []*accessAttr) error {
	q := arm.typ.qualified()
	fmt.Fprintf(b, "\n// multiple%s reads the containers of %s.\n", arm.typ.suffix(), q)
	fmt.Fprintf(b, "func multiple%s(x *%s, attr string) (list, bool, bool) {\n", arm.typ.suffix(), q)
	b.WriteString("\tif x == nil {\n\t\treturn nil, false, false\n\t}\n")
	b.WriteString("\tswitch attr {\n")
	for _, a := range attrs {
		field := "x." + a.field
		elem, err := g.classify(a.typ.elem)
		if err != nil {
			return fmt.Errorf("bmmgen: rmaccess: %s.%s: %w", arm.class.BMMName, a.name, err)
		}
		box := "boxPointers"
		if elem == accessInterface || elem == accessPointer {
			box = "boxValues"
		}
		fmt.Fprintf(b, "\tcase %q:\n\t\treturn %s(%s), %s != nil, true\n", a.name, box, field, field)
	}
	b.WriteString("\t}\n\treturn nil, false, false\n}\n")
	return nil
}

// writeHash writes the reader of arm's Hash attributes, each a map keyed by
// string or a pointer to one ([rmaccessGen.split] refuses any other): the
// keys in ascending order and the values, a value of a class or a primitive
// boxed as a pointer to a copy (entryPointers), one held as an interface or
// a pointer as it is (entryValues). Presence is the one single reads: a map
// is present when it is not nil, a pointer to a map when the pointer is not
// nil, whatever map it points to.
func (g *rmaccessGen) writeHash(b *bytes.Buffer, arm *accessArm, attrs []*accessAttr) error {
	q := arm.typ.qualified()
	fmt.Fprintf(b, "\n// hash%s reads the Hash attributes of %s.\n", arm.typ.suffix(), q)
	fmt.Fprintf(b, "func hash%s(x *%s, attr string) ([]string, []any, bool, bool) {\n", arm.typ.suffix(), q)
	b.WriteString("\tif x == nil {\n\t\treturn nil, nil, false, false\n\t}\n")
	b.WriteString("\tswitch attr {\n")
	for _, a := range attrs {
		field := "x." + a.field
		m, _ := hashMap(a.typ)
		elem, err := g.classify(m.elem)
		if err != nil {
			return fmt.Errorf("bmmgen: rmaccess: %s.%s: %w", arm.class.BMMName, a.name, err)
		}
		box := "entryPointers"
		if elem == accessInterface || elem == accessPointer {
			box = "entryValues"
		}
		fmt.Fprintf(b, "\tcase %q:\n", a.name)
		if a.typ.kind == goPointer {
			fmt.Fprintf(b, "\t\tif %s == nil {\n\t\t\treturn nil, nil, false, true\n\t\t}\n", field)
			fmt.Fprintf(b, "\t\tkeys, values := %s(*%s)\n\t\treturn keys, values, true, true\n", box, field)
			continue
		}
		present, err := g.presence(arm, a, false)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "\t\tkeys, values := %s(%s)\n\t\treturn keys, values, %s, true\n", box, field, present)
	}
	b.WriteString("\t}\n\treturn nil, nil, false, false\n}\n")
	return nil
}

// presence returns the Go expression that reports whether attribute a of arm
// is present, read from x: a fixed reading first, then, at an attribute the
// RM marks mandatory, the content rule of its declared type, and otherwise the
// reading of the field's Go kind, a value held by value through its zero
// test.
func (g *rmaccessGen) presence(arm *accessArm, a *accessAttr, interval bool) (string, error) {
	field := "x." + a.field
	if fn, ok := fixedPresence[arm.class.BMMName][a.name]; ok {
		return fmt.Sprintf("%s(%s)", fn, field), nil
	}
	if interval {
		if flag, ok := openBoundFlag(a.name); ok {
			return fmt.Sprintf("%s(x.%s, %s)", intervalBoundPresence, flag, field), nil
		}
	}
	if rule, ok := contentPresence[a.declared]; ok && a.required && slices.Contains(rule.kinds, a.kind) {
		return fmt.Sprintf("%s(%s)", rule.fn, field), nil
	}
	switch a.kind {
	case accessPointer, accessList:
		return field + " != nil", nil
	case accessInterface:
		return fmt.Sprintf("%s != nil && !rm.IsTypedNil(%s)", field, field), nil
	case accessString:
		return field + ` != ""`, nil
	case accessBool, accessNumber:
		return "true", nil
	case accessStruct:
		name, err := g.zeroTest(a.typ)
		if err != nil {
			return "", fmt.Errorf("bmmgen: rmaccess: %s.%s: %w", arm.class.BMMName, a.name, err)
		}
		return fmt.Sprintf("!%s(%s)", name, field), nil
	}
	return "", fmt.Errorf("bmmgen: rmaccess: %s.%s has no presence reading", arm.class.BMMName, a.name)
}

// zeroTest registers the zero test of the struct type t and returns its name.
func (g *rmaccessGen) zeroTest(t *goType) (string, error) {
	if t.kind != goNamed {
		return "", fmt.Errorf("zero test of non-named type %s", t.qualified())
	}
	if _, ok := g.structs[t.name]; !ok {
		return "", fmt.Errorf("zero test of %s, which is not a struct", t.qualified())
	}
	name := "isZero" + t.suffix()
	g.zeroTests[name] = t
	return name, nil
}

// writeZeroTests writes every registered zero test, and those they call,
// sorted by name. A zero test compares every field of the Go struct with its
// zero value: an embedded struct through its own test, a pointer, slice or
// map with nil, an interface with nil or a nil pointer, which § REQ-171
// *Structural access* counts as zero, a string with "", a boolean with false,
// a number with 0, and a struct held by value through its own test.
func (g *rmaccessGen) writeZeroTests(b *bytes.Buffer) error {
	rendered := map[string]string{}
	for {
		pending := slices.Sorted(maps.Keys(g.zeroTests))
		progress := false
		for _, name := range pending {
			if _, done := rendered[name]; done {
				continue
			}
			src, err := g.renderZeroTest(name, g.zeroTests[name])
			if err != nil {
				return err
			}
			rendered[name] = src
			progress = true
		}
		if !progress {
			break
		}
	}
	for _, name := range slices.Sorted(maps.Keys(rendered)) {
		b.WriteString("\n")
		b.WriteString(rendered[name])
	}
	return nil
}

// renderZeroTest renders the zero test name of struct type t.
func (g *rmaccessGen) renderZeroTest(name string, t *goType) (string, error) {
	pc := g.structs[t.name]
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return "", fmt.Errorf("bmmgen: rmaccess: zero test of %s, which is not a simple class", pc.BMMName)
	}
	binds, err := g.bindings(pc, t)
	if err != nil {
		return "", err
	}
	var conds []string
	embedded, ancestors := embeddedStructAncestors(g.plan, pc)
	for _, ap := range ancestors {
		asc := ap.Class.(*bmm.SimpleClass)
		at := &goType{kind: goNamed, name: ap.GoName}
		for _, k := range sortedStringKeys(asc.GenericParameterDefs) {
			at.args = append(at.args, binds[ap.BMMName][k])
		}
		test, err := g.zeroTest(at)
		if err != nil {
			return "", fmt.Errorf("bmmgen: rmaccess: %s embeds %s: %w", pc.BMMName, ap.BMMName, err)
		}
		conds = append(conds, fmt.Sprintf("%s(v.%s)", test, ap.GoName))
	}
	props := collectFlattenedProperties(g.plan, sc, embedded)
	for _, prop := range sortedStringKeys(props) {
		ft, err := g.fieldType(sc, pc.BMMName, props[prop], binds[pc.BMMName])
		if err != nil {
			return "", err
		}
		kind, err := g.classify(ft)
		if err != nil {
			return "", fmt.Errorf("bmmgen: rmaccess: %s.%s: %w", pc.BMMName, prop, err)
		}
		field := "v." + FieldName(prop)
		switch kind {
		case accessPointer, accessList:
			conds = append(conds, field+" == nil")
		case accessInterface:
			conds = append(conds, fmt.Sprintf("(%s == nil || rm.IsTypedNil(%s))", field, field))
		case accessString:
			conds = append(conds, field+` == ""`)
		case accessBool:
			conds = append(conds, "!"+field)
		case accessNumber:
			conds = append(conds, field+" == 0")
		case accessStruct:
			test, err := g.zeroTest(ft)
			if err != nil {
				return "", fmt.Errorf("bmmgen: rmaccess: %s.%s: %w", pc.BMMName, prop, err)
			}
			conds = append(conds, fmt.Sprintf("%s(%s)", test, field))
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "// %s reports whether v is the zero %s, field by field.\n", name, t.qualified())
	fmt.Fprintf(&b, "func %s(v %s) bool {\n", name, t.qualified())
	if len(conds) == 0 {
		b.WriteString("\treturn true\n")
	} else {
		fmt.Fprintf(&b, "\treturn %s\n", strings.Join(conds, " &&\n\t\t"))
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// goTypeKind is the outermost constructor of a parsed Go type.
type goTypeKind int

const (
	goNamed goTypeKind = iota
	goPointer
	goSlice
	goMap
)

// goType is a Go type expression, as the struct renderer writes one, parsed:
// a named type with its type arguments, or a pointer, slice or map of one.
// Names are unqualified, as in the RM package.
type goType struct {
	kind goTypeKind
	name string    // goNamed
	args []*goType // goNamed: type arguments
	key  *goType   // goMap
	elem *goType   // goPointer, goSlice, goMap
}

// parseGoType parses a type expression. A trailing /* … */ comment, which the
// renderer writes after an unmapped type's `any`, is dropped.
func parseGoType(s string) (*goType, error) {
	src := s
	if i := strings.Index(src, "/*"); i >= 0 {
		src = src[:i]
	}
	p := &goTypeParser{src: strings.TrimSpace(src)}
	t, err := p.parse()
	if err != nil {
		return nil, fmt.Errorf("parse Go type %q: %w", s, err)
	}
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("parse Go type %q: unexpected %q", s, p.src[p.pos:])
	}
	return t, nil
}

type goTypeParser struct {
	src string
	pos int
}

func (p *goTypeParser) consume(tok string) bool {
	if strings.HasPrefix(p.src[p.pos:], tok) {
		p.pos += len(tok)
		return true
	}
	return false
}

func (p *goTypeParser) parse() (*goType, error) {
	switch {
	case p.consume("*"):
		elem, err := p.parse()
		if err != nil {
			return nil, err
		}
		return &goType{kind: goPointer, elem: elem}, nil
	case p.consume("[]"):
		elem, err := p.parse()
		if err != nil {
			return nil, err
		}
		return &goType{kind: goSlice, elem: elem}, nil
	case p.consume("map["):
		key, err := p.parse()
		if err != nil {
			return nil, err
		}
		if !p.consume("]") {
			return nil, fmt.Errorf("map key not closed at %d", p.pos)
		}
		elem, err := p.parse()
		if err != nil {
			return nil, err
		}
		return &goType{kind: goMap, key: key, elem: elem}, nil
	}
	start := p.pos
	for p.pos < len(p.src) && isGoIdentByte(p.src[p.pos]) {
		p.pos++
	}
	if p.pos == start {
		return nil, fmt.Errorf("no type name at %d", start)
	}
	t := &goType{kind: goNamed, name: p.src[start:p.pos]}
	if !p.consume("[") {
		return t, nil
	}
	for {
		arg, err := p.parse()
		if err != nil {
			return nil, err
		}
		t.args = append(t.args, arg)
		if p.consume("]") {
			return t, nil
		}
		if !p.consume(", ") && !p.consume(",") {
			return nil, fmt.Errorf("type arguments not closed at %d", p.pos)
		}
	}
}

func isGoIdentByte(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9')
}

// bind returns t with every named type that binds names replaced by its
// argument.
func (t *goType) bind(binds map[string]*goType) *goType {
	if len(binds) == 0 {
		return t
	}
	switch t.kind {
	case goPointer, goSlice:
		return &goType{kind: t.kind, elem: t.elem.bind(binds)}
	case goMap:
		return &goType{kind: goMap, key: t.key.bind(binds), elem: t.elem.bind(binds)}
	case goNamed:
	}
	if arg, ok := binds[t.name]; ok && len(t.args) == 0 {
		return arg
	}
	out := &goType{kind: goNamed, name: t.name}
	for _, a := range t.args {
		out.args = append(out.args, a.bind(binds))
	}
	return out
}

// qualified writes t as the access package spells it: every name that is not
// a predeclared Go type qualified with rm.
func (t *goType) qualified() string {
	switch t.kind {
	case goPointer:
		return "*" + t.elem.qualified()
	case goSlice:
		return "[]" + t.elem.qualified()
	case goMap:
		return "map[" + t.key.qualified() + "]" + t.elem.qualified()
	case goNamed:
	}
	name := t.name
	if _, ok := goPredeclaredKinds[name]; !ok {
		name = "rm." + name
	}
	if len(t.args) == 0 {
		return name
	}
	args := make([]string, len(t.args))
	for i, a := range t.args {
		args[i] = a.qualified()
	}
	return name + "[" + strings.Join(args, ", ") + "]"
}

// suffix spells a named type t as an identifier suffix: its name and its type
// arguments', each with an upper-case first letter (DVInterval[DVQuantity] is
// DVIntervalDVQuantity, OriginalVersion[any] is OriginalVersionAny).
func (t *goType) suffix() string {
	var b strings.Builder
	b.WriteString(strings.ToUpper(t.name[:1]) + t.name[1:])
	for _, a := range t.args {
		b.WriteString(a.suffix())
	}
	return b.String()
}

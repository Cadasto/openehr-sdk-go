package bmmgen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// The direct canonical-JSON encoder (ADR 0022 § Encode writes each member
// itself). A generated MarshalJSONTo writes `{`, `_type` and each member of
// its class itself, as tokens on the encoder, unless the encoder carries an
// option under which json/v2 would write a member otherwise
// (typereg.DirectEncode). It then takes the wrapper of its wire shape,
// rendered as marshalJSONWrapped. The direct writer reproduces the wrapper's
// bytes: the members come in the order json/v2 writes the wrapper's fields,
// and each is left out by the rule json/v2 applies to its tag. A member whose
// value no rule of the writer covers sends the value back to the wrapper,
// before anything is written.

// directKind is how the direct encoder writes the value of a member, or of
// each element of a list member.
type directKind int

const (
	// directString is a Go string, written as a string token.
	directString directKind = iota
	// directBool is a Go bool, written as a boolean token.
	directBool
	// directClass is a class that carries a generated MarshalJSONTo, held by
	// value; the encoder calls the method.
	directClass
	// directNumber is a Real, Integer or Integer64 of the package that owns
	// their writers ([ownsNumberWriters]).
	directNumber
	// directSlot is a Go interface: the value it holds is classified by
	// jsonSlot.
	directSlot
	// directParam is a type parameter of the class: the value it holds is
	// classified by jsonParam.
	directParam
	// directEncode is any other type, written through json.MarshalEncode.
	directEncode
)

// encodeKind is the Go kind of a member written through json.MarshalEncode, which
// decides how its omit option leaves it out.
type encodeKind int

const (
	encodeOther encodeKind = iota
	encodePointer
	encodeSlice
	encodeMap
	encodeAny
)

// directMember is one member the direct encoder writes.
type directMember struct {
	name       string     // the JSON member name
	field      string     // the Go field name, selected on the receiver
	omit       string     // the omit option of the field's tag: "", "omitempty" or "omitzero"
	kind       directKind // how the value, or each element of a list, is written
	pointer    bool       // the field is a pointer to a value of kind
	slice      bool       // the field is a list of values of kind
	number     string     // for directNumber: Real, Integer or Integer64
	encodeKind encodeKind // for directEncode: the Go kind of the field
	bound      string     // for an interval bound: the variable that leaves it out on an open side
	decl       string     // the field's declaration, as the class's Go struct carries it
	empty      *emptyMember
}

// precomputed reports whether MarshalJSONTo classifies the value of m before
// it writes anything: an omitempty member typed by an interface, whose
// omission hangs on the value it holds, and a member typed by a type
// parameter. A value the classification cannot judge sends the class to its
// wrapper.
func (m directMember) precomputed() bool {
	return !m.slice && !m.pointer &&
		(m.kind == directSlot && m.omit == "omitempty" || m.kind == directParam)
}

// slotVar is the variable that holds the classification of a precomputed
// member.
func (m directMember) slotVar() string {
	return directVarBase(m.name) + "Slot"
}

// valueVar is the variable that holds the boxed value of a member typed by a
// type parameter.
func (m directMember) valueVar() string {
	return directVarBase(m.name) + "Value"
}

// directVarBase renders a member name as the stem of a local variable:
// `null_reason` becomes `nullReason`.
func directVarBase(name string) string {
	segs := strings.Split(name, "_")
	var b strings.Builder
	for i, s := range segs {
		if s == "" {
			continue
		}
		if i == 0 || b.Len() == 0 {
			b.WriteString(strings.ToLower(s))
			continue
		}
		b.WriteString(strings.ToUpper(s[:1]) + strings.ToLower(s[1:]))
	}
	return b.String()
}

// ownsNumberWriters reports whether the target's package declares the writers
// of its Real, Integer and Integer64 members (openehr/rm/json_direct_numbers.go):
// the RM package, which owns the three types. Any other target writes them
// through json.MarshalEncode.
func ownsNumberWriters(plan *Plan) bool {
	return plan.Target.GoPackage == "rm" && plan.Target.ExternalQualifier == ""
}

// directTypeIndex maps the Go type names the generated structs use to how the
// direct encoder writes a value of them: a class that carries a generated
// MarshalJSONTo to directClass, and a class rendered as a Go interface, a
// narrow `<Parent>Like` interface included, to directSlot. A name is
// qualified as the target's package refers to it ("rm.DVText" in AOM 1.4).
func directTypeIndex(plan *Plan) map[string]directKind {
	idx := map[string]directKind{}
	for _, pc := range plan.Classes {
		if pc.IsPrimitive {
			continue
		}
		ref := qualifyClassRef(plan, pc)
		switch cls := pc.Class.(type) {
		case *bmm.Interface:
			idx[ref] = directSlot
		case *bmm.SimpleClass:
			if cls.IsAbstract() && (!cls.IsGeneric() || codecPolymorphicAbstractGeneric(plan, pc)) {
				idx[ref] = directSlot
				continue
			}
			if bearsMarshalJSONTo(plan, pc) {
				idx[ref] = directClass
			}
			if iface, ok := narrowInterfaceGoName(plan, pc.BMMName); ok && isInterfaceTypeRef(plan, pc.BMMName) {
				idx[iface] = directSlot
			}
		}
	}
	return idx
}

// bearsMarshalJSONTo reports whether pc, of the target or of another one,
// carries a generated MarshalJSONTo: a concrete class, or the abstract BASE
// Interval struct.
func bearsMarshalJSONTo(plan *Plan, pc *PlannedClass) bool {
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok || pc.IsPrimitive {
		return false
	}
	if !sc.IsAbstract() {
		return true
	}
	if !sc.IsGeneric() || codecPolymorphicAbstractGeneric(plan, pc) {
		return false
	}
	shaped, err := intervalShaped(plan, pc)
	return err == nil && shaped
}

// directNumberTypes are the RM primitive types with a writer of their own.
var directNumberTypes = map[string]bool{"Real": true, "Integer": true, "Integer64": true}

// classifyDirectMember fills in how the direct encoder writes m, from the Go
// type of its field as owner declares it.
func classifyDirectMember(plan *Plan, idx map[string]directKind, owner *bmm.SimpleClass, typ ast.Expr, m *directMember) {
	elem := typ
	switch t := typ.(type) {
	case *ast.StarExpr:
		m.pointer = true
		elem = t.X
	case *ast.ArrayType:
		if t.Len == nil {
			m.slice = true
			elem = t.Elt
		}
	}
	kind, number := directValueKind(plan, idx, owner, elem)
	switch {
	case m.pointer && (kind == directSlot || kind == directParam || kind == directEncode):
		// A pointer to anything but a token or a class goes through
		// json.MarshalEncode whole.
		m.pointer, m.kind, m.encodeKind = false, directEncode, encodePointer
	case m.slice && kind != directString && kind != directClass && kind != directSlot:
		m.slice, m.kind, m.encodeKind = false, directEncode, encodeSlice
	default:
		m.kind, m.number = kind, number
		if kind == directEncode {
			m.encodeKind = encodeKindOf(typ)
		}
	}
}

// directValueKind classifies one Go type expression, without a pointer or a
// list around it.
func directValueKind(plan *Plan, idx map[string]directKind, owner *bmm.SimpleClass, typ ast.Expr) (directKind, string) {
	switch t := typ.(type) {
	case *ast.Ident:
		switch {
		case t.Name == "string":
			return directString, ""
		case t.Name == "bool":
			return directBool, ""
		case isOwnTypeParameter(owner, t.Name):
			return directParam, ""
		case directNumberTypes[t.Name]:
			if ownsNumberWriters(plan) {
				return directNumber, t.Name
			}
			return directEncode, ""
		}
		if k, ok := idx[t.Name]; ok {
			return k, ""
		}
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			if k, ok := idx[x.Name+"."+t.Sel.Name]; ok {
				return k, ""
			}
		}
	case *ast.IndexExpr:
		if k, _ := directValueKind(plan, idx, nil, t.X); k == directClass {
			return directClass, ""
		}
	case *ast.IndexListExpr:
		if k, _ := directValueKind(plan, idx, nil, t.X); k == directClass {
			return directClass, ""
		}
	}
	return directEncode, ""
}

// encodeKindOf returns the Go kind of a field type json.MarshalEncode writes.
func encodeKindOf(typ ast.Expr) encodeKind {
	switch t := typ.(type) {
	case *ast.StarExpr:
		return encodePointer
	case *ast.ArrayType:
		if t.Len == nil {
			return encodeSlice
		}
	case *ast.MapType:
		return encodeMap
	case *ast.InterfaceType:
		return encodeAny
	case *ast.Ident:
		if t.Name == "any" {
			return encodeAny
		}
	}
	return encodeOther
}

// declaredField is a struct field as the generator declares it.
type declaredField struct {
	goName   string
	typ      ast.Expr
	jsonName string
	omit     string
}

// parseDeclaredField parses the struct field source [renderField] returns.
func parseDeclaredField(src string) (declaredField, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "", "package p\ntype _ struct {\n"+src+"}\n", 0)
	if err != nil {
		return declaredField{}, fmt.Errorf("parse field %q: %w", strings.TrimSpace(src), err)
	}
	st := file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	if len(st.Fields.List) != 1 || len(st.Fields.List[0].Names) != 1 || st.Fields.List[0].Tag == nil {
		return declaredField{}, fmt.Errorf("field %q: want one named, tagged field", strings.TrimSpace(src))
	}
	f := st.Fields.List[0]
	tag, err := strconv.Unquote(f.Tag.Value)
	if err != nil {
		return declaredField{}, fmt.Errorf("field %q: tag: %w", strings.TrimSpace(src), err)
	}
	_, rest, ok := strings.Cut(tag, `json:"`)
	if !ok {
		return declaredField{}, fmt.Errorf("field %q: no json tag", strings.TrimSpace(src))
	}
	value, _, _ := strings.Cut(rest, `"`)
	name, opts, _ := strings.Cut(value, ",")
	if opts != "" && opts != "omitempty" && opts != "omitzero" {
		return declaredField{}, fmt.Errorf("field %q: json tag option %q", strings.TrimSpace(src), opts)
	}
	return declaredField{goName: f.Names[0].Name, typ: f.Type, jsonName: name, omit: opts}, nil
}

// wireField is a field the wrapper of a class marshals, at its embedding
// depth below the wrapper.
type wireField struct {
	ef    emittedField
	depth int
}

// goStructFields appends the JSON-visible fields of pc's Go struct, as
// [renderConcreteClass] declares it, at depth: the fields of each embedded
// ancestor first, one level deeper, then the class's own and flattened
// properties in name order.
func goStructFields(plan *Plan, pc *PlannedClass, depth int, out *[]wireField) {
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return
	}
	embedded, ancestors := embeddedStructAncestors(plan, pc)
	for _, ap := range ancestors {
		goStructFields(plan, ap, depth+1, out)
	}
	props := collectFlattenedProperties(plan, sc, embedded)
	for _, name := range sortedStringKeys(props) {
		*out = append(*out, wireField{ef: emittedField{Prop: props[name], Owner: sc, OwnerName: pc.BMMName}, depth: depth})
	}
}

// dominantFields keeps the fields json/v2 marshals of fields, in their order:
// of the fields sharing a JSON name, the shallowest. Two at the same depth
// would leave the name out altogether, which no generated class does.
func dominantFields(className string, fields []wireField) ([]wireField, error) {
	shallowest := map[string]int{}
	count := map[string]int{}
	for _, f := range fields {
		name := f.ef.Prop.PropertyName()
		d, seen := shallowest[name]
		switch {
		case !seen || f.depth < d:
			shallowest[name], count[name] = f.depth, 1
		case f.depth == d:
			count[name]++
		}
	}
	var out []wireField
	for _, f := range fields {
		name := f.ef.Prop.PropertyName()
		if f.depth != shallowest[name] {
			continue
		}
		if count[name] > 1 {
			return nil, fmt.Errorf("%s: %d fields named %q at one depth: json/v2 would write none", className, count[name], name)
		}
		out = append(out, f)
	}
	return out, nil
}

// directMembers returns the members the wrapper of pc, of wire shape shape,
// writes after `_type`, in the order json/v2 writes them.
func directMembers(plan *Plan, idx map[string]directKind, pc *PlannedClass, shape jsonWireShape, base *PlannedClass) ([]directMember, error) {
	var fields []wireField
	switch shape {
	case shapeFlat:
		efs, err := effectiveFields(plan, pc)
		if err != nil {
			return nil, err
		}
		for _, ef := range efs {
			fields = append(fields, wireField{ef: ef})
		}
	case shapeAlias, shapeIntervalAlias, shapeIntervalBase:
		// The wrapper embeds the class's alias one level down.
		goStructFields(plan, pc, 1, &fields)
	case shapeIntervalDerived:
		// The wrapper embeds the base's alias one level down and declares
		// the class's own members at its own level.
		goStructFields(plan, base, 1, &fields)
		own, err := intervalDerivedOwnFields(plan, pc)
		if err != nil {
			return nil, err
		}
		for _, ef := range own {
			fields = append(fields, wireField{ef: ef})
		}
	default:
		return nil, fmt.Errorf("%s: unhandled JSON wire shape %d", pc.BMMName, shape)
	}
	fields, err := dominantFields(pc.BMMName, fields)
	if err != nil {
		return nil, err
	}
	interval := shape == shapeIntervalAlias || shape == shapeIntervalBase || shape == shapeIntervalDerived
	empties, err := emptyMembersOf(plan, pc)
	if err != nil {
		return nil, err
	}
	members := make([]directMember, 0, len(fields))
	for _, f := range fields {
		src, err := renderField(plan, f.ef.Owner, f.ef.OwnerName, f.ef.Prop)
		if err != nil {
			return nil, fmt.Errorf("render field %s.%s: %w", pc.BMMName, f.ef.Prop.PropertyName(), err)
		}
		line := withoutCommentLines(src)
		decl, err := parseDeclaredField(line)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pc.BMMName, err)
		}
		m := directMember{name: decl.jsonName, field: decl.goName, omit: decl.omit, decl: line}
		if e, ok := empties[f.ef.Prop.PropertyName()]; ok {
			m.empty = &e
		}
		classifyDirectMember(plan, idx, f.ef.Owner, decl.typ, &m)
		if interval {
			switch m.name {
			case propLower:
				m.bound = "omitLower"
			case propUpper:
				m.bound = "omitUpper"
			}
		}
		members = append(members, m)
	}
	return members, nil
}

// renderDirectEncode renders the MarshalJSONTo of pc, which writes the class's
// members itself or takes the wrapper (marshalJSONWrapped), and the
// writeJSONMembers it writes them through.
func renderDirectEncode(plan *Plan, pc *PlannedClass, shape jsonWireShape, base *PlannedClass, recv, typeArgs string) (string, error) {
	members, err := directMembers(plan, directTypeIndex(plan), pc, shape, base)
	if err != nil {
		return "", err
	}
	class := pc.GoName + typeArgs
	interval := shape == shapeIntervalAlias || shape == shapeIntervalBase || shape == shapeIntervalDerived
	writesType := shape != shapeIntervalBase
	checks, err := jsonAnyChecks(plan, pc, recv, func(errExpr string) string {
		return fmt.Sprintf("typereg.EncodeRefusal[%s](enc, %s)", class, errExpr)
	})
	if err != nil {
		return "", err
	}

	// The variables MarshalJSONTo hands writeJSONMembers: the classification
	// of each precomputed member, the boxed value of each member typed by a
	// type parameter, and the open-side omissions of an interval.
	var slotVars, params, args []string
	var setup strings.Builder
	for _, m := range members {
		if !m.precomputed() {
			continue
		}
		value := recv + "." + m.field
		if m.kind == directParam {
			if m.omit != "omitempty" {
				return "", fmt.Errorf("%s.%s: a member typed by a type parameter takes omitempty, not %q", pc.BMMName, m.name, m.omit)
			}
			fmt.Fprintf(&setup, "\t%s := any(%s)\n", m.valueVar(), value)
			fmt.Fprintf(&setup, "\t%s := jsonParam(%s)\n", m.slotVar(), m.valueVar())
			if m.empty != nil {
				fmt.Fprintf(&setup, "\tif %s(%s) {\n\t\t%s = jsonSlotAbsent\n\t}\n", emptyHeldFunc, value, m.slotVar())
			}
			params = append(params, m.slotVar()+" uint8", m.valueVar()+" any")
			args = append(args, m.slotVar(), m.valueVar())
		} else {
			fmt.Fprintf(&setup, "\t%s := jsonSlot(%s)\n", m.slotVar(), value)
			params = append(params, m.slotVar()+" uint8")
			args = append(args, m.slotVar())
		}
		slotVars = append(slotVars, m.slotVar())
	}
	if interval {
		params = append(params, "omitLower bool", "omitUpper bool")
		args = append(args, "omitLower", "omitUpper")
	}

	var b strings.Builder
	renderDirectDoc(&b, pc, shape, checks != "", slices.ContainsFunc(members, func(m directMember) bool { return m.empty != nil }))
	fmt.Fprintf(&b, "func (%s %s) MarshalJSONTo(enc *jsontext.Encoder) error {\n", recv, class)
	b.WriteString("\tif !typereg.DirectEncode(enc) {\n")
	fmt.Fprintf(&b, "\t\treturn %s.marshalJSONWrapped(enc)\n", recv)
	b.WriteString("\t}\n")
	b.WriteString(checks)
	if interval {
		b.WriteString(omitFlags(recv))
	}
	b.WriteString(setup.String())
	if len(slotVars) > 0 {
		cond := slotVars[0]
		if len(slotVars) > 1 {
			cond = "(" + strings.Join(slotVars, "|") + ")"
		}
		fmt.Fprintf(&b, "\tif %s&jsonSlotOther != 0 {\n", cond)
		b.WriteString("\t\tmark := typereg.MarkEncode(enc)\n")
		fmt.Fprintf(&b, "\t\treturn typereg.EncodeFailureSince[%s](enc, mark, %s.marshalJSONWrapped(enc))\n", class, recv)
		b.WriteString("\t}\n")
	}
	b.WriteString("\tif err := enc.WriteToken(jsontext.BeginObject); err != nil {\n")
	fmt.Fprintf(&b, "\t\treturn typereg.EncodeFailure[%s](enc, false, err)\n", class)
	b.WriteString("\t}\n")
	fmt.Fprintf(&b, "\tif err := %s.writeJSONMembers(%s); err != nil {\n", recv, strings.Join(append([]string{"enc"}, args...), ", "))
	fmt.Fprintf(&b, "\t\treturn typereg.EncodeFailure[%s](enc, true, err)\n", class)
	b.WriteString("\t}\n")
	b.WriteString("\treturn nil\n")
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "// writeJSONMembers writes the members of %s after the opening brace\n", pc.GoName)
	b.WriteString("// MarshalJSONTo wrote, then the closing one. It returns a failure as it\n")
	b.WriteString("// meets it, for MarshalJSONTo to settle.\n")
	fmt.Fprintf(&b, "func (%s *%s) writeJSONMembers(%s) error {\n", recv, class, strings.Join(append([]string{"enc *jsontext.Encoder"}, params...), ", "))
	if writesType {
		fmt.Fprintf(&b, "\tif err := writeJSONString(enc, \"_type\", %q); err != nil {\n\t\treturn err\n\t}\n", pc.BMMName)
	}
	for _, m := range members {
		if err := renderDirectMember(&b, recv, m); err != nil {
			return "", fmt.Errorf("%s.%s: %w", pc.BMMName, m.name, err)
		}
	}
	b.WriteString("\treturn enc.WriteToken(jsontext.EndObject)\n")
	b.WriteString("}\n")
	return b.String(), nil
}

// renderDirectDoc writes the doc comment of a class's MarshalJSONTo.
func renderDirectDoc(b *strings.Builder, pc *PlannedClass, shape jsonWireShape, checks, empties bool) {
	if shape == shapeIntervalBase {
		fmt.Fprintf(b, "// MarshalJSONTo writes canonical openEHR JSON for %s, which is abstract\n", pc.GoName)
		b.WriteString("// and so carries no `_type`: an object holding each member that is not\n")
		b.WriteString("// left out.\n")
	} else {
		fmt.Fprintf(b, "// MarshalJSONTo writes canonical openEHR JSON for %s: an object whose\n", pc.GoName)
		fmt.Fprintf(b, "// leading member is `_type` (value %q), then each member that is not\n", pc.BMMName)
		b.WriteString("// left out.\n")
	}
	b.WriteString("// It writes the members itself, each as a token or through the encoder of\n")
	b.WriteString("// the value it holds, with json.Deterministic sorting any Hash keys. A\n")
	b.WriteString("// member is left out as its field's omit option leaves it out under\n")
	b.WriteString("// encoding/json/v2, such as a nil list or map, a nil pointer, or an\n")
	b.WriteString("// interface holding nil or a nil pointer to a class.\n")
	if empties {
		b.WriteString("// A mandatory member that holds a class by value is left out too when\n")
		b.WriteString("// every field of that value is empty.\n")
	}
	if shape == shapeIntervalBase || shape == shapeIntervalAlias || shape == shapeIntervalDerived {
		fmt.Fprintf(b, "// An open side (`%s` or `%s` set) whose bound is empty\n", propLowerUnbounded, propUpperUnbounded)
		fmt.Fprintf(b, "// writes no `%s` or `%s` member.\n", propLower, propUpper)
	}
	if checks {
		b.WriteString("// A member typed Any that holds a value its decoder would not read\n")
		b.WriteString("// back is refused, as the canonical-XML encoder refuses it.\n")
	}
	b.WriteString("//\n")
	b.WriteString("// Under an encoder option that changes how json/v2 writes a member, such\n")
	b.WriteString("// as a caller's marshalers, it marshals a wrapper struct through\n")
	b.WriteString("// json.MarshalEncode instead, and so it does for a member holding a value\n")
	b.WriteString("// of a type outside the model. Both write the same bytes otherwise, and a\n")
	b.WriteString("// failure names the class, as json/v2 names a type whose method failed.\n")
	if shape == shapeIntervalBase {
		b.WriteString("// The receiver is a value so an Interval held by value in a struct field\n")
		b.WriteString("// takes this encoder.\n")
	} else {
		b.WriteString("// The receiver is a value so a concrete instance sitting in a polymorphic\n")
		b.WriteString("// interface slot by value, the shape the like-interface accessors admit,\n")
		b.WriteString("// still carries its `_type`.\n")
	}
}

// renderDirectMember writes the statements of writeJSONMembers that write m,
// or leave it out, on the receiver recv.
func renderDirectMember(b *strings.Builder, recv string, m directMember) error {
	v := recv + "." + m.field
	var cond string    // when the member is written; "" for always
	var nullable bool  // a nil field is written as null
	var stmts []string // the statements that write the member's value
	writeName := true  // stmts start with the member's name
	switch {
	case m.kind == directParam:
		if m.omit != "omitempty" {
			return fmt.Errorf("a member typed by a type parameter takes omitempty, not %q", m.omit)
		}
		cond = m.slotVar() + " != jsonSlotAbsent"
		stmts = []string{fmt.Sprintf("writeJSONParam(enc, %s, %s)", m.valueVar(), m.slotVar())}
	case m.kind == directSlot && !m.slice:
		switch m.omit {
		case "omitempty":
			cond = m.slotVar() + " != jsonSlotAbsent"
			stmts = []string{fmt.Sprintf("writeJSONSlot(enc, %s, %s)", v, m.slotVar())}
		case "omitzero":
			cond = v + " != nil"
			stmts = []string{fmt.Sprintf("writeJSONSlot(enc, %s, jsonSlot(%s))", v, v)}
		default:
			stmts = []string{fmt.Sprintf("writeJSONSlot(enc, %s, jsonSlot(%s))", v, v)}
		}
	case m.slice:
		switch m.omit {
		case "omitempty":
			cond = "len(" + v + ") > 0"
		case "omitzero":
			cond = v + " != nil"
		default:
			nullable = true
		}
		var elem string
		switch m.kind {
		case directString:
			elem = "enc.WriteToken(jsontext.String(" + v + "[_k]))"
		case directClass:
			elem = v + "[_k].MarshalJSONTo(enc)"
		case directSlot:
			elem = fmt.Sprintf("writeJSONSlot(enc, %s[_k], jsonSlot(%s[_k]))", v, v)
		case directBool, directNumber, directParam, directEncode:
			return fmt.Errorf("a list of kind %d", m.kind)
		}
		stmts = []string{"enc.WriteToken(jsontext.BeginArray)", "for _k := range " + v, elem, "}", "enc.WriteToken(jsontext.EndArray)"}
	case m.kind == directEncode:
		switch m.encodeKind {
		case encodePointer:
			if m.omit != "" {
				cond = v + " != nil"
			}
			stmts = []string{fmt.Sprintf("json.MarshalEncode(enc, %s, typereg.MarshalOptions(enc))", v)}
		case encodeSlice, encodeMap:
			switch m.omit {
			case "omitempty":
				cond = "len(" + v + ") > 0"
			case "omitzero":
				cond = v + " != nil"
			}
		case encodeAny:
			switch m.omit {
			case "omitzero":
				cond = v + " != nil"
			case "omitempty":
				return errors.New("an any member takes omitzero, not omitempty")
			}
		case encodeOther:
			if m.omit != "" {
				return fmt.Errorf("a member of another type with option %q", m.omit)
			}
		}
		if stmts == nil {
			stmts = []string{"_v := " + v, "json.MarshalEncode(enc, &_v, typereg.MarshalOptions(enc))"}
		}
	case m.pointer:
		if m.omit != "" {
			cond = v + " != nil"
		} else {
			nullable = true
		}
		write, err := directValueWrite(m, "*"+v, v)
		if err != nil {
			return err
		}
		stmts = []string{write}
	default:
		switch {
		case m.kind == directString && m.omit != "":
			cond = v + ` != ""`
		case m.kind == directBool && m.omit == "omitzero":
			cond = v
		case m.kind == directNumber && m.omit == "omitzero":
			cond = v + " != 0"
		case m.kind == directClass && m.omit == "omitzero":
			return errors.New("a class held by value takes no omitzero")
		}
		switch m.kind {
		case directString:
			writeName = false
			stmts = []string{fmt.Sprintf("writeJSONString(enc, %q, %s)", m.name, v)}
		case directBool:
			writeName = false
			stmts = []string{fmt.Sprintf("writeJSONBool(enc, %q, %s)", m.name, v)}
		case directClass, directNumber, directSlot, directParam, directEncode:
			write, err := directValueWrite(m, v, v)
			if err != nil {
				return err
			}
			stmts = []string{write}
		}
	}
	if m.bound != "" {
		if cond == "" {
			cond = "!" + m.bound
		} else {
			cond = "!" + m.bound + " && " + cond
		}
	}
	if m.empty != nil && !m.empty.param {
		// A member typed by the class's parameter is left out through its
		// classification, which MarshalJSONTo settles before it writes.
		if cond == "" {
			cond = m.empty.cond(v)
		} else {
			cond = m.empty.cond(v) + " && " + cond
		}
	}

	indent := "\t"
	if cond != "" {
		fmt.Fprintf(b, "\tif %s {\n", cond)
		indent = "\t\t"
	}
	if writeName {
		writeDirectStmt(b, indent, fmt.Sprintf("writeJSONName(enc, %q)", m.name))
	}
	if nullable {
		fmt.Fprintf(b, "%sif %s == nil {\n", indent, v)
		writeDirectStmt(b, indent+"\t", "enc.WriteToken(jsontext.Null)")
		fmt.Fprintf(b, "%s} else {\n", indent)
		writeDirectStmts(b, indent+"\t", stmts)
		fmt.Fprintf(b, "%s}\n", indent)
	} else {
		writeDirectStmts(b, indent, stmts)
	}
	if cond != "" {
		b.WriteString("\t}\n")
	}
	return nil
}

// directValueWrite returns the call that writes a token, number or class
// value of m: value is the value expression, and holder the expression a
// class method is called on, which is the pointer itself for a pointer field.
func directValueWrite(m directMember, value, holder string) (string, error) {
	switch m.kind {
	case directString:
		return "enc.WriteToken(jsontext.String(" + value + "))", nil
	case directBool:
		return "enc.WriteToken(jsontext.Bool(" + value + "))", nil
	case directNumber:
		return fmt.Sprintf("writeJSON%s(enc, %s)", m.number, value), nil
	case directClass:
		return holder + ".MarshalJSONTo(enc)", nil
	case directSlot, directParam, directEncode:
	}
	return "", fmt.Errorf("a value of kind %d written as it stands", m.kind)
}

// writeDirectStmts writes stmts at indent: each call returning an error is
// checked; an assignment, a for clause and its closing brace are written as
// they stand, the loop body one level deeper.
func writeDirectStmts(b *strings.Builder, indent string, stmts []string) {
	for _, s := range stmts {
		switch {
		case strings.HasPrefix(s, "for "):
			fmt.Fprintf(b, "%s%s {\n", indent, s)
			indent += "\t"
		case s == "}":
			indent = indent[:len(indent)-1]
			fmt.Fprintf(b, "%s}\n", indent)
		case strings.Contains(s, " := "):
			fmt.Fprintf(b, "%s%s\n", indent, s)
		default:
			writeDirectStmt(b, indent, s)
		}
	}
}

// writeDirectStmt writes call, which returns an error, and the return of that
// error.
func writeDirectStmt(b *strings.Builder, indent, call string) {
	fmt.Fprintf(b, "%sif err := %s; err != nil {\n%s\treturn err\n%s}\n", indent, call, indent, indent)
}

// directHelpersFile returns the file whose canonical-JSON companion declares
// the helpers the direct encoders of the target share ([renderDirectHelpers]):
// the first of plan's files with a codec class. It returns nil when no file
// has one.
func directHelpersFile(plan *Plan) (*PlannedFile, error) {
	for _, f := range plan.Files {
		classes, err := codecClassesIn(plan, f)
		if err != nil {
			return nil, err
		}
		if len(classes) > 0 {
			return f, nil
		}
	}
	return nil, nil
}

// planHasDirectParams reports whether a codec class of plan has a member typed
// by a type parameter, which its direct encoder classifies through jsonParam.
func planHasDirectParams(plan *Plan) (bool, error) {
	idx := directTypeIndex(plan)
	for _, f := range plan.Files {
		classes, err := codecClassesIn(plan, f)
		if err != nil {
			return false, err
		}
		for _, pc := range classes {
			shape, base, err := classJSONWireShape(plan, pc)
			if err != nil {
				return false, err
			}
			members, err := directMembers(plan, idx, pc, shape, base)
			if err != nil {
				return false, err
			}
			for _, m := range members {
				if m.kind == directParam {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// renderDirectHelpers renders the helpers the direct encoders of the target's
// package share: the classification of the value an interface member holds
// (jsonSlot), over the closed set of the target's registered classes and, for
// a target that refers to another target's classes, that target's registry;
// the classification of the value a type-parameter member holds (jsonParam),
// when a class has one; and the small writers the members go through.
func renderDirectHelpers(plan *Plan) (string, error) {
	var b strings.Builder
	b.WriteString("// The classes of value [jsonSlot] and [jsonParam] tell apart, for the\n")
	b.WriteString("// direct canonical-JSON encoders of this package.\n")
	b.WriteString("const (\n")
	b.WriteString("\t// jsonSlotAbsent is nil, or a nil pointer to a class: json/v2 writes\n")
	b.WriteString("\t// it as null, and leaves out an omitempty member holding it.\n")
	b.WriteString("\tjsonSlotAbsent uint8 = 0\n")
	b.WriteString("\t// jsonSlotClass is a class value, or a non-nil pointer to one, written\n")
	b.WriteString("\t// through its MarshalJSONTo.\n")
	b.WriteString("\tjsonSlotClass uint8 = 1\n")
	b.WriteString("\t// jsonSlotValue is a non-empty string or a number, written as json/v2\n")
	b.WriteString("\t// writes it.\n")
	b.WriteString("\tjsonSlotValue uint8 = 2\n")
	b.WriteString("\t// jsonSlotOther is any other value. In a member that may be left out,\n")
	b.WriteString("\t// it sends the class to its wrapper encode, which judges it as json/v2\n")
	b.WriteString("\t// does; anywhere else it is written through json.MarshalEncode.\n")
	b.WriteString("\tjsonSlotOther uint8 = 4\n")
	b.WriteString(")\n\n")

	b.WriteString("// jsonSlot classifies v, the value of a member or list element typed by an\n")
	b.WriteString("// interface, as jsonSlotAbsent, jsonSlotClass or jsonSlotOther. A class is\n")
	b.WriteString("// one the type registry holds, so a value of any other type, a nil pointer\n")
	b.WriteString("// to one included, is jsonSlotOther. The check is a type switch and uses\n")
	b.WriteString("// no reflection.\n")
	var values []string
	for _, pc := range plan.ConcreteClasses {
		arms, err := reverseRegistryArms(plan, pc)
		if err != nil {
			return "", err
		}
		values = append(values, arms...)
	}
	b.WriteString("func jsonSlot(v any) uint8 {\n")
	if len(values) > 0 {
		b.WriteString("\tswitch x := v.(type) {\n")
	} else {
		b.WriteString("\tswitch v.(type) {\n")
	}
	b.WriteString("\tcase nil:\n\t\treturn jsonSlotAbsent\n")
	for _, arm := range values {
		fmt.Fprintf(&b, "\tcase *%s:\n\t\tif x == nil {\n\t\t\treturn jsonSlotAbsent\n\t\t}\n\t\treturn jsonSlotClass\n", arm)
	}
	if len(values) > 0 {
		b.WriteString("\tcase " + strings.Join(values, ",\n\t\t") + ":\n\t\treturn jsonSlotClass\n")
	}
	b.WriteString("\t}\n")
	if q := plan.Target.ExternalQualifier; q != "" {
		fmt.Fprintf(&b, "\tif _, ok := %s.RMTypeName(v); ok {\n\t\treturn jsonSlotClass\n\t}\n", q)
		fmt.Fprintf(&b, "\tif %s.IsTypedNil(v) {\n\t\treturn jsonSlotAbsent\n\t}\n", q)
	}
	b.WriteString("\treturn jsonSlotOther\n")
	b.WriteString("}\n\n")

	params, err := planHasDirectParams(plan)
	if err != nil {
		return "", err
	}
	if params {
		numbers := ownsNumberWriters(plan)
		b.WriteString("// jsonParam classifies v, the value of a member typed by a type parameter:\n")
		b.WriteString("// as [jsonSlot] does, except that an empty string is jsonSlotAbsent, as an\n")
		b.WriteString("// omitempty member leaves it out, and any other string ")
		if numbers {
			b.WriteString("or a Real, an\n// Integer or an Integer64 ")
		}
		b.WriteString("is jsonSlotValue.\n")
		b.WriteString("func jsonParam(v any) uint8 {\n")
		b.WriteString("\tswitch x := v.(type) {\n")
		b.WriteString("\tcase string:\n\t\tif x == \"\" {\n\t\t\treturn jsonSlotAbsent\n\t\t}\n\t\treturn jsonSlotValue\n")
		if numbers {
			b.WriteString("\tcase Real, Integer, Integer64:\n\t\treturn jsonSlotValue\n")
		}
		b.WriteString("\t}\n")
		b.WriteString("\treturn jsonSlot(v)\n")
		b.WriteString("}\n\n")

		b.WriteString("// writeJSONParam writes v, the value of a member typed by a type parameter,\n")
		b.WriteString("// which [jsonParam] classified as slot.\n")
		b.WriteString("func writeJSONParam(enc *jsontext.Encoder, v any, slot uint8) error {\n")
		b.WriteString("\tif slot == jsonSlotValue {\n")
		b.WriteString("\t\tswitch x := v.(type) {\n")
		b.WriteString("\t\tcase string:\n\t\t\treturn enc.WriteToken(jsontext.String(x))\n")
		if numbers {
			b.WriteString("\t\tcase Real:\n\t\t\treturn writeJSONReal(enc, x)\n")
			b.WriteString("\t\tcase Integer:\n\t\t\treturn writeJSONInteger(enc, x)\n")
			b.WriteString("\t\tcase Integer64:\n\t\t\treturn writeJSONInteger64(enc, x)\n")
		}
		b.WriteString("\t\t}\n")
		b.WriteString("\t}\n")
		b.WriteString("\treturn writeJSONSlot(enc, v, slot)\n")
		b.WriteString("}\n\n")
	}

	b.WriteString("// writeJSONSlot writes v, the value of a member or list element typed by\n")
	b.WriteString("// an interface, which [jsonSlot] classified as slot: jsonSlotAbsent as\n")
	b.WriteString("// null, a class through its MarshalJSONTo, and any other value through\n")
	b.WriteString("// json.MarshalEncode.\n")
	b.WriteString("func writeJSONSlot(enc *jsontext.Encoder, v any, slot uint8) error {\n")
	b.WriteString("\tswitch slot {\n")
	b.WriteString("\tcase jsonSlotAbsent:\n\t\treturn enc.WriteToken(jsontext.Null)\n")
	b.WriteString("\tcase jsonSlotClass:\n\t\treturn v.(json.MarshalerTo).MarshalJSONTo(enc)\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn json.MarshalEncode(enc, v, typereg.MarshalOptions(enc))\n")
	b.WriteString("}\n\n")

	b.WriteString("// writeJSONName writes the name of an object member.\n")
	b.WriteString("func writeJSONName(enc *jsontext.Encoder, name string) error {\n")
	b.WriteString("\treturn enc.WriteToken(jsontext.String(name))\n")
	b.WriteString("}\n\n")
	b.WriteString("// writeJSONString writes an object member whose value is a string.\n")
	b.WriteString("func writeJSONString(enc *jsontext.Encoder, name, value string) error {\n")
	b.WriteString("\tif err := enc.WriteToken(jsontext.String(name)); err != nil {\n\t\treturn err\n\t}\n")
	b.WriteString("\treturn enc.WriteToken(jsontext.String(value))\n")
	b.WriteString("}\n\n")
	b.WriteString("// writeJSONBool writes an object member whose value is a boolean.\n")
	b.WriteString("func writeJSONBool(enc *jsontext.Encoder, name string, value bool) error {\n")
	b.WriteString("\tif err := enc.WriteToken(jsontext.String(name)); err != nil {\n\t\treturn err\n\t}\n")
	b.WriteString("\treturn enc.WriteToken(jsontext.Bool(value))\n")
	b.WriteString("}\n")
	return b.String(), nil
}

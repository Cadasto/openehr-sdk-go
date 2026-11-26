package bmmgen

import (
	"fmt"
	"strings"
)

// The direct canonical-JSON decoder. A generated UnmarshalJSONFrom reads `{`,
// each member and `}` itself, decoding each member's value into the
// receiver's own field through json.UnmarshalDecode, unless the decoder
// carries an option under which json/v2's struct decode would read a member
// otherwise (typereg.DirectDecode). It then takes the wrapper of its wire
// shape, rendered as unmarshalJSONWrapped, which decodes a wrapper struct
// through typereg.DecodeInto as every generated decoder did before. The direct
// read reproduces the wrapper's outcome: the same members are known, a member
// the class does not declare is skipped, a `_type` naming another class is
// refused once the whole value is read, and a failure is classified by the
// same typereg helper. Each wire shape is covered:
//
//   - the alias shape decodes in place, as its wrapper does, an Any member
//     through the xmlschema decoder the wrapper gives it;
//   - the flat shape starts from the zero value and puts the receiver back
//     as it was on a failure or a `_type` naming another class, as its
//     wrapper copies the wire struct back only on success;
//   - an interval that carries the four boundary flags records which flags
//     and bounds the document carried in locals, and gives the absent flags
//     their defaults after a read that succeeds, as its wrapper does after
//     typereg.DecodeInto.

// decodeRead is how the direct decoder reads the value of one member.
type decodeRead int

const (
	// readField decodes the value into the field through json.UnmarshalDecode.
	readField decodeRead = iota
	// readAny decodes the value through the xmlschema decoder of a member
	// typed by the BMM root type Any ([jsonAnyMembers]).
	readAny
	// readFlag decodes an interval boundary flag and records that the
	// document carried it.
	readFlag
	// readBound decodes an interval bound and records whether the document
	// carried a value for it.
	readBound
)

// decodeMember is one member the direct decoder reads.
type decodeMember struct {
	name  string     // the JSON member name
	field string     // the Go field name, selected on the receiver
	read  decodeRead // how the value is read
	// anyMember is the decoder of a member read through readAny.
	anyMember jsonAnyMember
}

// directDecodeMembers returns the members the wrapper of pc, of wire shape
// shape, decodes besides `_type`: the members json/v2 matches by name in the
// wrapper struct, which are the members its encoder writes
// ([directMembers]), read as [decodeMembersOf] says.
func directDecodeMembers(plan *Plan, pc *PlannedClass, shape jsonWireShape, base *PlannedClass, anyMembers []jsonAnyMember, flagged bool) ([]decodeMember, error) {
	members, err := directMembers(plan, directTypeIndex(plan), pc, shape, base)
	if err != nil {
		return nil, err
	}
	return decodeMembersOf(pc.BMMName, members, anyMembers, flagged)
}

// decodeMembersOf returns how the direct decoder of class reads each of
// members: an Any member through its own decoder, and, for an interval that
// carries the four boundary flags, a flag or a bound through the helpers that
// record its presence. It refuses a member with no JSON name of its own, or
// named `_type`, which the decoder reads itself, and an Any member that is not
// among members or decodes into another field: the direct read would then
// know other members than the wrapper does.
func decodeMembersOf(class string, members []directMember, anyMembers []jsonAnyMember, flagged bool) ([]decodeMember, error) {
	anyByName := make(map[string]jsonAnyMember, len(anyMembers))
	for _, m := range anyMembers {
		anyByName[m.name] = m
	}
	out := make([]decodeMember, 0, len(members))
	for _, m := range members {
		if m.name == "" || m.name == "_type" {
			return nil, fmt.Errorf("%s: member %q of field %s: the direct decoder needs a JSON name of its own", class, m.name, m.field)
		}
		dm := decodeMember{name: m.name, field: m.field}
		if a, ok := anyByName[m.name]; ok {
			if a.field != m.field {
				return nil, fmt.Errorf("%s: Any member %q decodes into %s, not %s", class, m.name, a.field, m.field)
			}
			dm.read, dm.anyMember = readAny, a
			delete(anyByName, m.name)
		}
		if flagged {
			switch {
			case isIntervalFlag(m.name):
				dm.read = readFlag
			case isIntervalBound(m.name):
				dm.read = readBound
			}
		}
		out = append(out, dm)
	}
	if len(anyByName) > 0 {
		return nil, fmt.Errorf("%s: %d Any members are not among the members the wrapper decodes", class, len(anyByName))
	}
	return out, nil
}

// renderDirectDecode renders the UnmarshalJSONFrom of pc, which reads the
// class's members itself or takes the wrapper (unmarshalJSONWrapped), and
// the readJSONMembers it reads them through. fields are pc's codec fields;
// flagged says whether they carry the four interval boundary flags.
func renderDirectDecode(plan *Plan, pc *PlannedClass, fields []emittedField, shape jsonWireShape, base *PlannedClass, anyMembers []jsonAnyMember, flagged bool, recv, typeArgs string) (string, error) {
	members, err := directDecodeMembers(plan, pc, shape, base, anyMembers, flagged)
	if err != nil {
		return "", err
	}
	if shape == shapeFlat {
		if err := flatHoldsOnlyWireFields(plan, pc, fields); err != nil {
			return "", err
		}
	}
	class := pc.GoName + typeArgs
	readsType := shape != shapeIntervalBase
	seenParam := ""
	if flagged {
		seenParam = ", seen *intervalFlagsSeen"
	}

	var b strings.Builder
	renderDirectDecodeDoc(&b, pc, shape, flagged)
	fmt.Fprintf(&b, "func (%s *%s) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n", recv, class)
	fmt.Fprintf(&b, "\tif %s == nil {\n", recv)
	fmt.Fprintf(&b, "\t\treturn fmt.Errorf(\"canjson: %s: %%w\", typereg.ErrNilReceiver)\n", pc.BMMName)
	b.WriteString("\t}\n")
	b.WriteString("\tif !typereg.DirectDecode(dec) {\n")
	fmt.Fprintf(&b, "\t\treturn %s.unmarshalJSONWrapped(dec)\n", recv)
	b.WriteString("\t}\n")
	switch {
	case flagged:
		b.WriteString("\tvar seen intervalFlagsSeen\n")
		if readsType {
			fmt.Fprintf(&b, "\tgot, err := %s.readJSONMembers(dec, &seen)\n", recv)
			b.WriteString("\tif err != nil || got != \"\" {\n")
			fmt.Fprintf(&b, "\t\treturn typereg.DecodeDone(%q, got, err)\n", pc.BMMName)
		} else {
			fmt.Fprintf(&b, "\tif err := %s.readJSONMembers(dec, &seen); err != nil {\n", recv)
			fmt.Fprintf(&b, "\t\treturn typereg.DecodeDone(%q, \"\", err)\n", pc.BMMName)
		}
		b.WriteString("\t}\n")
		fmt.Fprintf(&b, "\tif err := %s; err != nil {\n", intervalFlagSettle("seen", recv))
		fmt.Fprintf(&b, "\t\treturn typereg.WrapShapeError(%q, err)\n", pc.BMMName)
		b.WriteString("\t}\n")
		b.WriteString("\treturn nil\n")
	case shape == shapeFlat:
		b.WriteString("\tsaved := *" + recv + "\n")
		fmt.Fprintf(&b, "\t*%s = %s{}\n", recv, class)
		fmt.Fprintf(&b, "\tgot, err := %s.readJSONMembers(dec)\n", recv)
		b.WriteString("\tif err != nil || got != \"\" {\n")
		fmt.Fprintf(&b, "\t\t*%s = saved\n", recv)
		b.WriteString("\t}\n")
		fmt.Fprintf(&b, "\treturn typereg.DecodeDone(%q, got, err)\n", pc.BMMName)
	case !readsType:
		return "", fmt.Errorf("%s reads no `_type` and carries no interval boundary flags", pc.BMMName)
	default:
		fmt.Fprintf(&b, "\tgot, err := %s.readJSONMembers(dec)\n", recv)
		fmt.Fprintf(&b, "\treturn typereg.DecodeDone(%q, got, err)\n", pc.BMMName)
	}
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "// readJSONMembers reads the members of %s into the receiver, from the\n", pc.GoName)
	b.WriteString("// opening brace to the closing one, skipping a member the class does not\n")
	switch {
	case !readsType:
		b.WriteString("// declare, `_type` among them, and records in seen the boundary flags and\n")
		b.WriteString("// bounds the document carries. It returns a failure as it meets it, for\n")
		b.WriteString("// UnmarshalJSONFrom to settle.\n")
	case flagged:
		b.WriteString("// declare, and records in seen the boundary flags and bounds the document\n")
		b.WriteString("// carries. It returns the class a `_type` member names when that is another\n")
		b.WriteString("// class, and a failure as it meets it, for UnmarshalJSONFrom to settle.\n")
	default:
		b.WriteString("// declare. It returns the class a `_type` member names when that is another\n")
		b.WriteString("// class, and a failure as it meets it, for UnmarshalJSONFrom to settle.\n")
	}
	results := "error"
	fail := "return err"
	if readsType {
		results = "(string, error)"
		fail = "return \"\", err"
	}
	fmt.Fprintf(&b, "func (%s *%s) readJSONMembers(dec *jsontext.Decoder%s) %s {\n", recv, class, seenParam, results)
	b.WriteString("\tif _, err := dec.ReadToken(); err != nil {\n")
	fmt.Fprintf(&b, "\t\t%s\n", fail)
	b.WriteString("\t}\n")
	if readsType {
		b.WriteString("\tvar got string\n")
	}
	b.WriteString("\tfor dec.PeekKind() != '}' {\n")
	b.WriteString("\t\tname, err := dec.ReadToken()\n")
	b.WriteString("\t\tif err != nil {\n")
	if readsType {
		b.WriteString("\t\t\treturn \"\", typereg.MemberNameFailure(err)\n")
	} else {
		b.WriteString("\t\t\treturn typereg.MemberNameFailure(err)\n")
	}
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tswitch name.String() {\n")
	if readsType {
		b.WriteString("\t\tcase \"_type\":\n")
		fmt.Fprintf(&b, "\t\t\tgot, err = typereg.DecodeTypeMember(dec, %q)\n", pc.BMMName)
	}
	for _, m := range members {
		fmt.Fprintf(&b, "\t\tcase %q:\n", m.name)
		fmt.Fprintf(&b, "\t\t\terr = %s\n", directDecodeRead(recv, m))
	}
	b.WriteString("\t\tdefault:\n")
	b.WriteString("\t\t\terr = dec.SkipValue()\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tif err != nil {\n")
	fmt.Fprintf(&b, "\t\t\t%s\n", fail)
	b.WriteString("\t\t}\n")
	b.WriteString("\t}\n")
	if readsType {
		b.WriteString("\t_, err := dec.ReadToken()\n")
		b.WriteString("\treturn got, err\n")
	} else {
		b.WriteString("\t_, err := dec.ReadToken()\n")
		b.WriteString("\treturn err\n")
	}
	b.WriteString("}\n\n")
	return b.String(), nil
}

// directDecodeRead returns the call that reads the value of m into the field
// of recv.
func directDecodeRead(recv string, m decodeMember) string {
	field := "&" + recv + "." + m.field
	switch m.read {
	case readAny:
		if m.anyMember.typesVar != "" {
			return fmt.Sprintf("json.UnmarshalDecode(dec, &%s{To: %s, Types: %s})", m.anyMember.decoder, field, m.anyMember.typesVar)
		}
		return fmt.Sprintf("json.UnmarshalDecode(dec, &%s{To: %s})", m.anyMember.decoder, field)
	case readFlag:
		return fmt.Sprintf("decodeIntervalFlagJSON(dec, %s, &seen.%s)", field, seenFlagField(m.name))
	case readBound:
		return fmt.Sprintf("decodeIntervalBoundJSON(dec, %s, &seen.%s)", field, seenFlagField(m.name))
	case readField:
	}
	return fmt.Sprintf("json.UnmarshalDecode(dec, %s)", field)
}

// flatHoldsOnlyWireFields checks that every member of pc's Go struct is a
// field of its flat wire struct, fields: the direct decoder of the flat shape
// starts from the zero value of the class, where the wrapper copies back the
// wire struct's fields only, so a member outside them would be zeroed by the
// one and kept by the other.
func flatHoldsOnlyWireFields(plan *Plan, pc *PlannedClass, fields []emittedField) error {
	wire := make(map[string]bool, len(fields))
	for _, ef := range fields {
		wire[ef.Prop.PropertyName()] = true
	}
	var all []wireField
	goStructFields(plan, pc, 0, &all)
	for _, f := range all {
		if name := f.ef.Prop.PropertyName(); !wire[name] {
			return fmt.Errorf("%s: member %q of the Go struct is not in the flat wire struct, "+
				"so the direct decoder cannot start from the zero value", pc.BMMName, name)
		}
	}
	return nil
}

// renderDirectDecodeDoc writes the doc comment of a class's UnmarshalJSONFrom.
func renderDirectDecodeDoc(b *strings.Builder, pc *PlannedClass, shape jsonWireShape, flagged bool) {
	if shape == shapeIntervalBase {
		fmt.Fprintf(b, "// UnmarshalJSONFrom decodes canonical openEHR JSON into %s, which is abstract\n", pc.GoName)
		b.WriteString("// and so reads no `_type`: a `_type` member is skipped like any other member\n")
		b.WriteString("// the class does not declare. A nil receiver is refused with\n")
		b.WriteString("// typereg.ErrNilReceiver rather than dereferenced. It reads each member\n")
		b.WriteString("// into the receiver's own field under the decoder's options. The receiver\n")
		b.WriteString("// is a pointer, so an Interval held by value in a struct field takes this\n")
		b.WriteString("// decoder.\n")
	} else {
		fmt.Fprintf(b, "// UnmarshalJSONFrom decodes canonical openEHR JSON into %s.\n", pc.GoName)
		b.WriteString("// A nil receiver is refused with typereg.ErrNilReceiver rather than\n")
		b.WriteString("// dereferenced. It reads each member into the receiver's own field under\n")
		b.WriteString("// the decoder's options, so a nested slot resolves through the polymorphic\n")
		b.WriteString("// decode hooks, skips a member the class does not declare, and refuses a\n")
		b.WriteString("// `_type` that names another class once the whole value is read.\n")
	}
	if shape == shapeFlat {
		b.WriteString("// A failure leaves the receiver as it was.\n")
	}
	b.WriteString("//\n")
	b.WriteString("// Under a decoder option that changes how json/v2 reads a member, such as\n")
	b.WriteString("// a caller's unmarshalers, and for a value that is not an object, it\n")
	b.WriteString("// decodes a wrapper struct through typereg.DecodeInto instead. Both read the\n")
	b.WriteString("// same value and word a failure the same, a shape failure wrapping\n")
	b.WriteString("// typereg.ErrInvalidShape behind the `canjson: <RM_TYPE>:` text.\n")
	if flagged {
		b.WriteString("//\n")
		b.WriteString(intervalFlagDecodeDoc())
	}
}

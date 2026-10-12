package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// intervalBoundFile is the per-target file that carries the interval-bound
// emptiness test the canonical encoders share, and the boundary-flag defaults
// the canonical decoders share.
const intervalBoundFile = "interval_bound_gen.go"

// The BASE Interval shape: the two bounds and the two open-side flags. A
// concrete class whose codec fields carry all four is interval-shaped,
// whether it descends from Interval (DV_INTERVAL, Proper_interval,
// Point_interval) or declares the members itself.
//
// Multiplicity_interval has no encoder of its own and marshals through the
// Proper_interval it embeds. The BMM makes it an interval of Integer, but the
// Go type embeds ProperInterval[any], so its bound is interface-typed: an open
// side's Integer(0) is a value and is kept, where ProperInterval[Integer]
// drops it.
const (
	propLower          = "lower"
	propUpper          = "upper"
	propLowerUnbounded = "lower_unbounded"
	propUpperUnbounded = "upper_unbounded"
	propLowerIncluded  = "lower_included"
	propUpperIncluded  = "upper_included"
)

// intervalFlagProps are the four BASE Interval boundary flags, in the order
// of the Go struct fields. A decoder reads each one as written when the
// document carries it and gives it a default when the document leaves it out
// (REQ-169).
var intervalFlagProps = []string{propLowerIncluded, propLowerUnbounded, propUpperIncluded, propUpperUnbounded}

// isIntervalFlag reports whether prop is one of the four boundary flags.
func isIntervalFlag(prop string) bool {
	return slices.Contains(intervalFlagProps, prop)
}

// seenFlagField is the field of the generated intervalFlagsSeen that records
// whether the document carried the boundary flag prop: the flag's Go field
// name with a lower-case first letter.
func seenFlagField(prop string) string {
	name := FieldName(prop)
	return strings.ToLower(name[:1]) + name[1:]
}

// intervalFlagged reports whether a class's codec fields carry the four
// boundary flags, so its decoders read an absent flag by REQ-169. A class that
// carries some of the flags but not all four, or a flag that is not a
// mandatory Boolean, is refused: the generated defaults need all four as plain
// bool fields.
func intervalFlagged(fields []emittedField) (bool, error) {
	var found []string
	for _, ef := range fields {
		name := ef.Prop.PropertyName()
		if !isIntervalFlag(name) {
			continue
		}
		p, ok := ef.Prop.(*bmm.SingleProperty)
		if !ok || !p.IsMandatory || !isPrimitive(p.TypeName) || primitiveGoType[p.TypeName] != "bool" {
			return false, fmt.Errorf("interval flag %s of %s is not a mandatory Boolean", name, ef.OwnerName)
		}
		found = append(found, name)
	}
	switch len(found) {
	case 0:
		return false, nil
	case len(intervalFlagProps):
		return true, nil
	}
	return false, fmt.Errorf("carries the interval flags %q but not all four of %q", found, intervalFlagProps)
}

// intervalFlagSettle renders the call that gives every boundary flag the
// document left out its default, on the flag fields of recv, and returns the
// refusal of a side written closed with no bound (REQ-169). Whether recv holds
// its bounds by value is read from its lower bound's Go type.
func intervalFlagSettle(seen, recv string) string {
	args := make([]string, 0, len(intervalFlagProps)+1)
	args = append(args, "intervalBoundHeldByValue(&"+recv+"."+FieldName(propLower)+")")
	for _, prop := range intervalFlagProps {
		args = append(args, "&"+recv+"."+FieldName(prop))
	}
	return fmt.Sprintf("%s.settle(%s)", seen, strings.Join(args, ", "))
}

// intervalBoundProps are the two bounds of BASE Interval, whose presence in a
// document decides the default of their side's `*_unbounded` (REQ-169).
var intervalBoundProps = []string{propLower, propUpper}

// isIntervalBound reports whether prop is one of the two bounds.
func isIntervalBound(prop string) bool {
	return slices.Contains(intervalBoundProps, prop)
}

// unmarshalXMLIntervalBoundSeen renders the statement that records, after
// the bound prop was read, that the document carried it.
func unmarshalXMLIntervalBoundSeen(prop string) string {
	return fmt.Sprintf("\t\t\t\t_seen.%s = true\n", seenFlagField(prop))
}

// intervalFlagDecodeDoc is the part of an interval decoder's doc comment that
// says how a flag the document leaves out is read.
func intervalFlagDecodeDoc() string {
	var b strings.Builder
	b.WriteString("// A boundary flag the document leaves out takes its default once the whole\n")
	fmt.Fprintf(&b, "// value is read: `%s` and `%s` read as false on a\n", propLowerUnbounded, propUpperUnbounded)
	b.WriteString("// side that carries a bound and as true on a side whose bound is absent or\n")
	fmt.Fprintf(&b, "// null, and `%s` and `%s` read as true on a side\n", propLowerIncluded, propUpperIncluded)
	b.WriteString("// that is not open and as false on a side that is. A flag the document\n")
	b.WriteString("// carries is read as written. A side written `*_unbounded` false with no\n")
	b.WriteString("// bound is refused when the interval holds its bounds by value, since a zero\n")
	b.WriteString("// bound would stand in for the absent one; held as an interface, the side\n")
	b.WriteString("// decodes as written, with no bound.\n")
	return b.String()
}

// intervalFlagXMLDoc is the part of an interval UnmarshalXML doc comment that
// says how an empty flag element is read.
const intervalFlagXMLDoc = "// An empty or whitespace-only flag element carries no boolean and reads as\n" +
	"// a flag the document leaves out.\n"

// unmarshalXMLIntervalFlag renders the UnmarshalXML case branch of one
// boundary flag of a class that carries all four. The shared
// decodeIntervalFlagXML reads the element's text, trimmed: an empty or
// whitespace-only element carries no boolean, so it leaves the field alone and
// the flag unmarked, and the flag takes its default like an absent one
// (REQ-169); any other text is parsed as a boolean, and a text that is none is
// a decode error. Only a flag read as a boolean is marked as present.
func unmarshalXMLIntervalFlag(recv, prop string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tif _err := decodeIntervalFlagXML(_dec, &_t, &%s.%s, &_seen.%s); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		prop, recv, FieldName(prop), seenFlagField(prop))
}

// renderIntervalFlagJSONDecode renders the body of the UnmarshalJSONFrom of a
// class that carries the four boundary flags, after the nil-receiver check.
//
// The decode target is an anonymous wrapper that embeds the class's wire value
// (embedType, set to embedValue) and declares the four flags and the two
// bounds at its own level, shallower than the members reached through the
// embedded value, so the wrapper's win. Each flag is an intervalFlagJSON and
// each bound an intervalBoundJSON: it decodes the member straight into recv's
// own field, so a failed decode leaves the field as an in-place decode would,
// and records that the document carried it. Only after a decode that succeeds
// are the absent flags given their defaults (REQ-169), so a default sees its
// side's bound and `*_unbounded` whatever the member order, and a side written
// closed with no bound is refused through typereg.WrapShapeError when recv
// holds its bounds by value. copyBack lists the class's own members besides the flags, which the
// wire carries by value and which come back whether or not the decode fails.
// typeDecl, when not empty, is a statement that declares the `_type` target
// gotType names.
//
// A JSON null in place of the whole interval is consumed first and decodes as
// the member left out (REQ-052, "a null member as absent"): the receiver keeps
// what it holds and no flag takes a default, so a null interval held by value
// does not read as a closed interval.
func renderIntervalFlagJSONDecode(b *strings.Builder, recv, rmType, embedType, embedValue, typeDecl, gotType string, copyBack []string) {
	embedField := strings.TrimPrefix(embedType, "*")
	if i := strings.IndexByte(embedField, '['); i >= 0 {
		embedField = embedField[:i]
	}
	b.WriteString("\t// A null in place of the interval reads as the member left out: the\n")
	b.WriteString("\t// receiver keeps what it holds and no flag takes a default.\n")
	b.WriteString("\tif dec.PeekKind() == 'n' {\n")
	b.WriteString("\t\t_, err := dec.ReadToken()\n")
	b.WriteString("\t\treturn err\n")
	b.WriteString("\t}\n")
	b.WriteString("\tvar seen intervalFlagsSeen\n")
	if typeDecl != "" {
		b.WriteString(typeDecl)
	}
	b.WriteString("\twire := &struct {\n")
	fmt.Fprintf(b, "\t\t%s\n", embedType)
	for _, prop := range intervalFlagProps {
		fmt.Fprintf(b, "\t\t%s intervalFlagJSON `json:%q`\n", FieldName(prop), prop)
	}
	for _, prop := range intervalBoundProps {
		fmt.Fprintf(b, "\t\t%s intervalBoundJSON `json:%q`\n", FieldName(prop), prop)
	}
	b.WriteString("\t}{\n")
	fmt.Fprintf(b, "\t\t%s: %s,\n", embedField, embedValue)
	for _, prop := range intervalFlagProps {
		fn := FieldName(prop)
		fmt.Fprintf(b, "\t\t%s: intervalFlagJSON{field: &%s.%s, seen: &seen.%s},\n", fn, recv, fn, seenFlagField(prop))
	}
	for _, prop := range intervalBoundProps {
		fn := FieldName(prop)
		fmt.Fprintf(b, "\t\t%s: intervalBoundJSON{field: &%s.%s, seen: &seen.%s},\n", fn, recv, fn, seenFlagField(prop))
	}
	b.WriteString("\t}\n")
	if len(copyBack) == 0 {
		fmt.Fprintf(b, "\tif err := typereg.DecodeInto(dec, %q, wire, %s); err != nil {\n", rmType, gotType)
		b.WriteString("\t\treturn err\n")
		b.WriteString("\t}\n")
	} else {
		fmt.Fprintf(b, "\terr := typereg.DecodeInto(dec, %q, wire, %s)\n", rmType, gotType)
		for _, fn := range copyBack {
			fmt.Fprintf(b, "\t%s.%s = wire.%s\n", recv, fn, fn)
		}
		b.WriteString("\tif err != nil {\n")
		b.WriteString("\t\treturn err\n")
		b.WriteString("\t}\n")
	}
	fmt.Fprintf(b, "\tif err := %s; err != nil {\n", intervalFlagSettle("seen", recv))
	fmt.Fprintf(b, "\t\treturn typereg.WrapShapeError(%q, err)\n", rmType)
	b.WriteString("\t}\n")
	b.WriteString("\treturn nil\n")
}

// intervalSettleHelpers is the generated source of intervalFlagsSeen, its
// settle method, the closed-side refusal, intervalBoundHeldByValue and
// intervalBoundJSON (REQ-169).
const intervalSettleHelpers = `// intervalFlagsSeen records which of an interval's four boundary flags and
// two bounds a canonical document carried, so a decoder can tell an absent
// flag from one written as false, and an absent or null bound from a bound
// that holds a zero value.
type intervalFlagsSeen struct {
	lowerIncluded  bool
	lowerUnbounded bool
	upperIncluded  bool
	upperUnbounded bool
	lower          bool
	upper          bool
}

// settle gives every boundary flag the document left out the value an absent
// flag reads as, and refuses a side the document writes closed but gives no
// bound. An absent ` + "`lower_unbounded` or `upper_unbounded`" + ` reads as false on a
// side that carries a bound and as true on a side whose bound is absent or
// null too, so an interval that gives only an upper bound is open below. An
// absent ` + "`lower_included` or `upper_included`" + ` reads as true on a side that is not
// open and as false on a side that is, the only value an open side admits. A
// flag the document carried keeps the value it was read as. The open flags
// are settled first, so an included default sees its side's open flag
// whatever order the members came in.
//
// A side written ` + "`*_unbounded`" + ` false whose bound is absent or null names a
// closed end with no value. byValue says whether the interval holds its
// bounds by value, where a zero bound would stand in for the absent one, so
// settle refuses such a side; an interval that holds its bounds as an
// interface keeps the side as written, with no bound.
func (s intervalFlagsSeen) settle(byValue bool, lowerIncluded, lowerUnbounded, upperIncluded, upperUnbounded *bool) error {
	if !s.lowerUnbounded {
		*lowerUnbounded = !s.lower
	} else if !*lowerUnbounded && !s.lower && byValue {
		return errIntervalClosedSide("lower")
	}
	if !s.upperUnbounded {
		*upperUnbounded = !s.upper
	} else if !*upperUnbounded && !s.upper && byValue {
		return errIntervalClosedSide("upper")
	}
	if !s.lowerIncluded {
		*lowerIncluded = !*lowerUnbounded
	}
	if !s.upperIncluded {
		*upperIncluded = !*upperUnbounded
	}
	return nil
}

// errIntervalClosedSide is the refusal of a side, "lower" or "upper", that a
// document writes closed with no bound, where the interval holds its bound by
// value. It names the flag and the bound, never a value.
func errIntervalClosedSide(side string) error {
	return errors.New(side + "_unbounded is false but the interval gives no " + side + " bound, which a bound held by value cannot leave out")
}

// intervalBoundHeldByValue reports whether an interval holds the bound its
// argument points to by value: a concrete type, whose zero value would stand in for an
// absent bound, rather than an interface, which holds no value at all.
func intervalBoundHeldByValue[T any](*T) bool {
	var zero T
	return any(zero) != nil
}

// intervalBoundJSON is the canonical-JSON decode target of one interval
// bound. It decodes the member straight into the interval's own bound field,
// as the field's own decoder would, and marks the bound seen when the member
// is not null.
type intervalBoundJSON struct {
	field any
	seen  *bool
}

// UnmarshalJSONFrom decodes one bound into its field and records whether the
// document carried a value for it.
func (f intervalBoundJSON) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	null := dec.PeekKind() == 'n'
	if err := json.UnmarshalDecode(dec, f.field, dec.Options()); err != nil {
		return err
	}
	*f.seen = !null
	return nil
}

`

// renderIntervalFlagHelpers renders the boundary-flag helpers the canonical
// decoders of every interval class share (REQ-169): intervalFlagsSeen, which
// records the flags and bounds a document carried, gives the absent flags
// their defaults and refuses a side written closed with no bound held by
// value; intervalBoundHeldByValue, which tells a bound held by value from one
// held as an interface; intervalFlagJSON and intervalBoundJSON, the
// canonical-JSON decode targets of one flag and one bound; and
// decodeIntervalFlagXML, which reads one canonical-XML flag element and reads
// an empty or whitespace-only one as absent. The flag order follows
// [intervalFlagProps].
func renderIntervalFlagHelpers() string {
	var b strings.Builder
	b.WriteString(intervalSettleHelpers)
	b.WriteString("// intervalFlagJSON is the canonical-JSON decode target of one interval\n")
	b.WriteString("// boundary flag. It decodes the member straight into the interval's own flag\n")
	b.WriteString("// field, so a failed decode leaves the field as an in-place decode would, and\n")
	b.WriteString("// marks the flag seen once it is read. A null member reads as an absent one.\n")
	b.WriteString("type intervalFlagJSON struct {\n")
	b.WriteString("\tfield *bool\n")
	b.WriteString("\tseen  *bool\n")
	b.WriteString("}\n\n")
	b.WriteString("// UnmarshalJSONFrom decodes one boundary flag into its field and records that\n")
	b.WriteString("// the document carried it.\n")
	b.WriteString("func (f intervalFlagJSON) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n")
	b.WriteString("\tif dec.PeekKind() == 'n' {\n")
	b.WriteString("\t\t_, err := dec.ReadToken()\n")
	b.WriteString("\t\treturn err\n")
	b.WriteString("\t}\n")
	b.WriteString("\tif err := json.UnmarshalDecode(dec, f.field, dec.Options()); err != nil {\n")
	b.WriteString("\t\treturn err\n")
	b.WriteString("\t}\n")
	b.WriteString("\t*f.seen = true\n")
	b.WriteString("\treturn nil\n")
	b.WriteString("}\n\n")

	b.WriteString("// decodeIntervalFlagXML reads one interval boundary flag element of canonical\n")
	b.WriteString("// XML into field and marks it seen. An empty or whitespace-only element\n")
	b.WriteString("// carries no boolean: it leaves field alone and the flag unmarked, so the\n")
	b.WriteString("// flag takes its default like one the document leaves out. Any other text,\n")
	b.WriteString("// trimmed, is parsed as a boolean, and a text that is none is an error.\n")
	b.WriteString("// The text is read through xmlunmatched.DecodeElement, so an attribute or child\n")
	b.WriteString("// element on the flag is unmatched content like any other.\n")
	b.WriteString("func decodeIntervalFlagXML(dec *xml.Decoder, start *xml.StartElement, field, seen *bool) error {\n")
	b.WriteString("\tvar text string\n")
	b.WriteString("\tif err := xmlunmatched.DecodeElement(dec, start, &text); err != nil {\n")
	b.WriteString("\t\treturn err\n")
	b.WriteString("\t}\n")
	b.WriteString("\ttext = strings.TrimSpace(text)\n")
	b.WriteString("\tif text == \"\" {\n")
	b.WriteString("\t\treturn nil\n")
	b.WriteString("\t}\n")
	b.WriteString("\tv, err := strconv.ParseBool(text)\n")
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\treturn err\n")
	b.WriteString("\t}\n")
	b.WriteString("\t*field = v\n")
	b.WriteString("\t*seen = true\n")
	b.WriteString("\treturn nil\n")
	b.WriteString("}\n\n")
	return b.String()
}

// renderUnmarshalIntervalBase renders the UnmarshalJSONFrom of BASE Interval.
// The class is abstract and writes no `_type`, so its decoder reads none: the
// wrapper declares no `_type` field, a `_type` member is skipped like any
// other member the class does not declare, and the discriminator target
// handed to typereg.DecodeInto stays empty, which leaves its check out. The
// shared helper still bounds the depth, threads the decode hooks and
// classifies a shape failure.
func renderUnmarshalIntervalBase(pc *PlannedClass, recv, typeArgs string) string {
	alias := aliasTypeName(pc.GoName)
	var b strings.Builder
	fmt.Fprintf(&b, "// UnmarshalJSONFrom decodes canonical openEHR JSON into %s, which is abstract\n", pc.GoName)
	b.WriteString("// and so reads no `_type`: a `_type` member is skipped like any other member\n")
	b.WriteString("// the class does not declare. A nil receiver is refused with\n")
	b.WriteString("// typereg.ErrNilReceiver rather than dereferenced. The shared helper threads\n")
	b.WriteString("// the polymorphic decode hooks and wraps a shape failure through\n")
	b.WriteString("// typereg.WrapShapeError. The receiver is a pointer, so an Interval held by\n")
	b.WriteString("// value in a struct field takes this decoder.\n")
	b.WriteString("//\n")
	b.WriteString(intervalFlagDecodeDoc())
	fmt.Fprintf(&b, "func (%s *%s%s) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n", recv, pc.GoName, typeArgs)
	fmt.Fprintf(&b, "\tif %s == nil {\n", recv)
	fmt.Fprintf(&b, "\t\treturn fmt.Errorf(\"canjson: %s: %%w\", typereg.ErrNilReceiver)\n", pc.BMMName)
	b.WriteString("\t}\n")
	typeDecl := "\t// No `_type` is declared, so the discriminator stays empty and is not checked.\n" +
		"\tvar noType string\n"
	renderIntervalFlagJSONDecode(&b, recv, pc.BMMName,
		"*"+alias+typeArgs, fmt.Sprintf("(*%s%s)(%s)", alias, typeArgs, recv),
		typeDecl, "&noType", nil)
	b.WriteString("}\n")
	return b.String()
}

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

// isIntervalBase reports whether pc is the abstract generic class that carries
// the BASE Interval shape and is emitted as a Go struct: BASE Interval itself.
// The base schema lists it among its primitive types, which the check
// therefore ignores. It has no concrete BMM instances, so it gets no type
// registration, but the
// SDK holds it by value (AOM 1.4's occurrences, existence and cardinality
// intervals are `Interval[Integer]`) and so its JSON and XML encoders leave out
// an open side's empty bound like every other interval-shaped class (REQ-052,
// REQ-056). An abstract generic class that is emitted as an interface (EVENT)
// is not a struct and never qualifies.
func isIntervalBase(plan *Plan, pc *PlannedClass) (bool, error) {
	if pc.External {
		return false, nil
	}
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok || !sc.IsAbstract() || !sc.IsGeneric() || codecPolymorphicAbstractGeneric(plan, pc) {
		return false, nil
	}
	return intervalShaped(plan, pc)
}

// codecClassesIn returns the classes of file that receive the canonical-JSON
// codec and the canonical-XML codec: the concrete classes, plus the abstract
// BASE Interval struct ([isIntervalBase]). Type registration and the
// discriminator methods stay with the concrete classes ([concreteClassesIn]);
// BASE Interval writes and reads no `_type`, and has a JSON decoder of its own
// only so that a flag the document leaves out takes its default (REQ-169).
func codecClassesIn(plan *Plan, file *PlannedFile) ([]*PlannedClass, error) {
	out := make([]*PlannedClass, 0, len(file.Classes))
	for _, pc := range file.Classes {
		if isConcreteCodecClass(pc) {
			out = append(out, pc)
			continue
		}
		base, err := isIntervalBase(plan, pc)
		if err != nil {
			return nil, err
		}
		if base {
			out = append(out, pc)
		}
	}
	return out, nil
}

// embeddedIntervalBase returns the BASE Interval struct pc embeds directly, or
// nil when pc embeds none. A class that embeds it through another embedded
// class embeds a marshaler-bearing concrete class too, which
// [jsonWireShape] refuses for an interval-shaped class, so only a direct
// embedding reaches the interval wire struct.
func embeddedIntervalBase(plan *Plan, pc *PlannedClass) (*PlannedClass, error) {
	_, ancestors := embeddedStructAncestors(plan, pc)
	for _, ap := range ancestors {
		base, err := isIntervalBase(plan, ap)
		if err != nil {
			return nil, err
		}
		if base {
			return ap, nil
		}
	}
	return nil, nil
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

// legacyIntervalXMLDoc is the part of the BASE Interval UnmarshalXML doc
// comment that explains the legacy element names it also reads.
const legacyIntervalXMLDoc = "// BASE Interval also reads the Go field names (Lower, LowerIncluded, ...) that\n" +
	"// SDK v0.28.0 and earlier wrote for it, onto the same fields, so a stored\n" +
	"// document does not decode to an empty interval without an error. The encoder\n" +
	"// writes the snake_case names only.\n"

// withLegacyElementName adds the Go field name of a BASE Interval member to
// the case label of its UnmarshalXML branch, so the decoder reads both
// spellings. Only BASE Interval takes it: SDK v0.28.0 wrote Go field names
// for that class alone, because the other interval classes had their own
// encoders.
func withLegacyElementName(caseBody, prop string) string {
	old := fmt.Sprintf("\t\t\tcase %q:\n", prop)
	return strings.Replace(caseBody, old, fmt.Sprintf("\t\t\tcase %q, %q:\n", prop, FieldName(prop)), 1)
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

// openBoundDoc is the part of an interval MarshalJSONTo doc comment that
// explains how an open side's empty bound drops out.
func openBoundDoc() string {
	var b strings.Builder
	fmt.Fprintf(&b, "// An open side (`%s` or `%s` set) whose bound is empty\n", propLowerUnbounded, propUpperUnbounded)
	fmt.Fprintf(&b, "// emits no `%s` or `%s` member. The wrapper then declares a zero-size\n", propLower, propUpper)
	b.WriteString("// field of that name at its own level, shallower than the bound embedded\n")
	b.WriteString("// through the alias, so it wins; it is always omitted, and the other\n")
	b.WriteString("// members keep their order.\n")
	return b.String()
}

// omitFlags renders the two omit decisions of an interval-shaped class.
func omitFlags(recv string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\tomitLower := omitIntervalBound(%s.%s, %s.%s)\n", recv, FieldName(propLowerUnbounded), recv, FieldName(propLower))
	fmt.Fprintf(&b, "\tomitUpper := omitIntervalBound(%s.%s, %s.%s)\n", recv, FieldName(propUpperUnbounded), recv, FieldName(propUpper))
	return b.String()
}

// omitSwitch renders the four-way switch over the omit decisions; ret renders
// the `return` for the bound properties named in omitted.
func omitSwitch(ret func(omitted ...string) string) string {
	var b strings.Builder
	b.WriteString("\tswitch {\n")
	b.WriteString("\tcase omitLower && omitUpper:\n")
	b.WriteString(ret(propLower, propUpper))
	b.WriteString("\tcase omitLower:\n")
	b.WriteString(ret(propLower))
	b.WriteString("\tcase omitUpper:\n")
	b.WriteString(ret(propUpper))
	b.WriteString("\t}\n")
	b.WriteString(ret())
	return b.String()
}

// renderMarshalAliasInterval is [renderMarshalAlias] for an interval-shaped
// class that declares the BASE Interval members itself. The alias wrapper
// stays the wire for every member; when a side is open and its bound empty,
// the wrapper declares a zero-size field of the bound's name at its own level,
// shallower than the bound embedded through the alias. The shallower field
// wins, and it is always omitted, so the member drops out and every other
// member keeps its place.
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
	b.WriteString(openBoundDoc())
	fmt.Fprintf(&b, "func (%s %s%s) MarshalJSONTo(enc *jsontext.Encoder) error {\n", recv, pc.GoName, typeArgs)
	b.WriteString(omitFlags(recv))
	b.WriteString(omitSwitch(func(omitted ...string) string {
		var r strings.Builder
		r.WriteString("\treturn json.MarshalEncode(enc, &struct {\n")
		r.WriteString("\t\tType string `json:\"_type\"`\n")
		fmt.Fprintf(&r, "\t\t*%s%s\n", alias, typeArgs)
		for _, prop := range omitted {
			fmt.Fprintf(&r, "\t\t%s struct{} `json:%q`\n", FieldName(prop), prop+",omitzero")
		}
		fmt.Fprintf(&r, "\t}{Type: %q, %s: (*%s%s)(&%s)}, typereg.MarshalOptions(enc))\n", pc.BMMName, alias, alias, typeArgs, recv)
		return r.String()
	}))
	b.WriteString("}\n")
	return b.String()
}

// renderMarshalIntervalBase emits the wire type and MarshalJSONTo of BASE
// Interval itself. It is abstract, so it writes no `_type`, and AOM 1.4 holds
// it by value in fields typed by it. The omission mechanism is the one
// [renderMarshalAliasInterval] uses.
func renderMarshalIntervalBase(pc *PlannedClass, recv, typeParams, typeArgs string) string {
	alias := aliasTypeName(pc.GoName)

	var b strings.Builder
	b.WriteString(renderMarshalAliasDecl(pc, typeParams, typeArgs))

	fmt.Fprintf(&b, "// MarshalJSONTo emits canonical openEHR JSON for %s, which is abstract and\n", pc.GoName)
	b.WriteString("// so carries no `_type`. Field order follows the struct declaration;\n")
	b.WriteString("// json.Deterministic sorts any Hash keys and the\n")
	b.WriteString("// FormatNil* options keep a mandatory nil container's `null` spelling.\n")
	b.WriteString("// The receiver is a value so an Interval held by value in a struct field\n")
	b.WriteString("// takes this encoder.\n")
	b.WriteString("//\n")
	b.WriteString(openBoundDoc())
	fmt.Fprintf(&b, "func (%s %s%s) MarshalJSONTo(enc *jsontext.Encoder) error {\n", recv, pc.GoName, typeArgs)
	b.WriteString(omitFlags(recv))
	b.WriteString(omitSwitch(func(omitted ...string) string {
		var r strings.Builder
		r.WriteString("\treturn json.MarshalEncode(enc, &struct {\n")
		fmt.Fprintf(&r, "\t\t*%s%s\n", alias, typeArgs)
		for _, prop := range omitted {
			fmt.Fprintf(&r, "\t\t%s struct{} `json:%q`\n", FieldName(prop), prop+",omitzero")
		}
		fmt.Fprintf(&r, "\t}{%s: (*%s%s)(&%s)}, typereg.MarshalOptions(enc))\n", alias, alias, typeArgs, recv)
		return r.String()
	}))
	b.WriteString("}\n")
	return b.String()
}

// intervalDerivedOwnFields returns the fields a class that embeds BASE Interval
// declares itself: the members the wire struct carries beside the embedded
// base. They come in the order of the class's Go struct body, which sorts by
// property name, so the wire keeps the encoding the class always had.
// Point_interval re-declares the four flags, so they are its own;
// Proper_interval and DV_INTERVAL declare none.
func intervalDerivedOwnFields(plan *Plan, pc *PlannedClass) ([]emittedField, error) {
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		return nil, err
	}
	var own []emittedField
	for _, ef := range fields {
		if ef.OwnerName == pc.BMMName {
			own = append(own, ef)
		}
	}
	slices.SortFunc(own, func(a, b emittedField) int {
		return strings.Compare(a.Prop.PropertyName(), b.Prop.PropertyName())
	})
	return own, nil
}

// intervalDecodeOwnFields returns the own members of a class that embeds BASE
// Interval that its decoder copies back from the wire struct: the members of
// [intervalDerivedOwnFields] that are not boundary flags. The decoder reads
// the flags straight into the receiver ([renderIntervalFlagJSONDecode]), so
// Point_interval, whose own members are exactly the four flags, copies none.
func intervalDecodeOwnFields(plan *Plan, pc *PlannedClass) ([]emittedField, error) {
	own, err := intervalDerivedOwnFields(plan, pc)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(own, func(ef emittedField) bool { return isIntervalFlag(ef.Prop.PropertyName()) }), nil
}

// renderIntervalDerivedWire emits the canonical-JSON wire struct of a class
// that embeds BASE Interval: `_type`, the method-free alias of the base, and
// the class's own members. The class cannot be marshalled through a method-free
// alias of itself: the alias would still embed BASE Interval and promote its
// MarshalJSONTo. With that method promoted, encoding/json/v2 encodes the
// wrapper through it, silently emitting the base's payload without `_type` and
// without the open-side omission, and decoding into the wrapper fails at run
// time with "must not implement marshal or unmarshal methods". The wire struct
// reaches the base through the base's own method-free alias, which has no
// methods, and so promotes none.
func renderIntervalDerivedWire(plan *Plan, pc, base *PlannedClass, typeParams string) (string, []emittedField, error) {
	own, err := intervalDerivedOwnFields(plan, pc)
	if err != nil {
		return "", nil, err
	}
	typeArgs := genericTypeArgList(pc.Class.(*bmm.SimpleClass))
	wire := flatWireTypeName(pc.GoName)

	var b strings.Builder
	fmt.Fprintf(&b, "// %s is the canonical-JSON wire struct for %s. %s embeds BASE\n", wire, pc.GoName, pc.GoName)
	b.WriteString("// Interval, so a method-free alias of the class would promote the base's\n")
	b.WriteString("// MarshalJSONTo. The wire struct embeds the method-free alias of the base\n")
	b.WriteString("// instead, and carries the class's own members beside it.\n")
	fmt.Fprintf(&b, "type %s%s struct {\n", wire, typeParams)
	b.WriteString("\tType string `json:\"_type\"`\n")
	fmt.Fprintf(&b, "\t*%s%s\n", aliasTypeName(base.GoName), typeArgs)
	for _, ef := range own {
		line, err := renderField(plan, ef.Owner, ef.OwnerName, ef.Prop)
		if err != nil {
			return "", nil, fmt.Errorf("render wire field %s.%s: %w", pc.BMMName, ef.Prop.PropertyName(), err)
		}
		b.WriteString(withoutCommentLines(line))
	}
	b.WriteString("}\n\n")
	return b.String(), own, nil
}

// withoutCommentLines drops the doc-comment lines of rendered field source.
func withoutCommentLines(src string) string {
	var b strings.Builder
	for line := range strings.Lines(src) {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		b.WriteString(line)
	}
	return b.String()
}

// intervalDerivedWireValue renders the composite literal that fills the wire
// struct of a class from its receiver: the base through the method-free alias,
// and each own member by value. The encoder sets `_type`; the decoder leaves it
// empty so that a missing discriminator is still seen as missing. The decoder
// passes only the own members that are not boundary flags
// ([intervalDecodeOwnFields]): it reads the flags through its own wrapper.
func intervalDerivedWireValue(pc, base *PlannedClass, recv, typeArgs string, own []emittedField, withType bool) string {
	alias := aliasTypeName(base.GoName)
	var b strings.Builder
	fmt.Fprintf(&b, "&%s%s{", flatWireTypeName(pc.GoName), typeArgs)
	if withType {
		fmt.Fprintf(&b, "Type: %q, ", pc.BMMName)
	}
	fmt.Fprintf(&b, "%s: (*%s%s)(&%s.%s)", alias, alias, typeArgs, recv, base.GoName)
	for _, ef := range own {
		fn := FieldName(ef.Prop.PropertyName())
		fmt.Fprintf(&b, ", %s: %s.%s", fn, recv, fn)
	}
	b.WriteString("}")
	return b.String()
}

// renderMarshalIntervalDerived emits the wire struct and MarshalJSONTo of an
// interval-shaped class that embeds BASE Interval. The open-side mechanism is
// [renderMarshalAliasInterval]'s: a zero-size `omitzero` field of the bound's
// name, one level shallower than the bound the wire struct embeds.
func renderMarshalIntervalDerived(plan *Plan, pc, base *PlannedClass, recv, typeParams, typeArgs string) (string, error) {
	wireDecl, own, err := renderIntervalDerivedWire(plan, pc, base, typeParams)
	if err != nil {
		return "", err
	}
	wire := flatWireTypeName(pc.GoName)

	var b strings.Builder
	b.WriteString(wireDecl)
	fmt.Fprintf(&b, "// MarshalJSONTo emits canonical openEHR JSON for %s with `_type`\n", pc.GoName)
	fmt.Fprintf(&b, "// (value %q) as the leading member. Field order otherwise follows the\n", pc.BMMName)
	b.WriteString("// struct declaration; json.Deterministic sorts any Hash keys and the\n")
	b.WriteString("// FormatNil* options keep a mandatory nil container's `null` spelling.\n")
	b.WriteString("// The receiver is a value so a concrete instance sitting\n")
	b.WriteString("// in a polymorphic interface slot by value, the shape the like-interface\n")
	b.WriteString("// accessors admit, still carries its `_type`.\n")
	b.WriteString("//\n")
	b.WriteString(openBoundDoc())
	fmt.Fprintf(&b, "func (%s %s%s) MarshalJSONTo(enc *jsontext.Encoder) error {\n", recv, pc.GoName, typeArgs)
	b.WriteString(omitFlags(recv))
	fmt.Fprintf(&b, "\tw := %s\n", intervalDerivedWireValue(pc, base, recv, typeArgs, own, true))
	b.WriteString(omitSwitch(func(omitted ...string) string {
		if len(omitted) == 0 {
			return "\treturn json.MarshalEncode(enc, w, typereg.MarshalOptions(enc))\n"
		}
		var r strings.Builder
		r.WriteString("\treturn json.MarshalEncode(enc, &struct {\n")
		fmt.Fprintf(&r, "\t\t*%s%s\n", wire, typeArgs)
		for _, prop := range omitted {
			fmt.Fprintf(&r, "\t\t%s struct{} `json:%q`\n", FieldName(prop), prop+",omitzero")
		}
		fmt.Fprintf(&r, "\t}{%s: w}, typereg.MarshalOptions(enc))\n", wire)
		return r.String()
	}))
	b.WriteString("}\n")
	return b.String(), nil
}

// RenderIntervalBoundFile renders <pkg>/interval_bound_gen.go for a target
// that owns an interval-shaped class (a concrete one, or the abstract BASE
// Interval struct, see [codecClassesIn]): the exported emptiness test the
// canonical encoders apply to an open side's bound, with one field-by-field
// zero predicate per concrete bound type, derived from the BMM class's
// properties and using no reflection. It also carries the boundary-flag
// helpers the canonical decoders share ([renderIntervalFlagHelpers]).
//
// The bound types are read from each interval class's `lower` and `upper`
// fields (see [intervalBounds]): the concrete classes a generic bound admits
// (DV_ORDERED's for DV_INTERVAL) or a class the field names directly. Every
// Go built-in scalar type and RM primitive is covered by a comparison with
// its zero value, which is what BASE Interval's primitive bound Ordered
// admits. A predicate for a class reached by value from a bound type
// (DV_ORDINAL's `symbol`) is emitted too.
//
// Returns (nil, nil) when the target owns no interval-shaped class.
func RenderIntervalBoundFile(plan *Plan) ([]byte, error) {
	// The open-side emptiness rule is REQ-052 and REQ-056; the no-reflection
	// rule is REQ-024.
	shaped, boundClasses, err := intervalBounds(plan)
	if err != nil {
		return nil, err
	}
	if !shaped {
		return nil, nil
	}
	scalars, err := scalarBoundGoTypes()
	if err != nil {
		return nil, err
	}

	predicates, err := renderZeroPredicates(plan, boundClasses)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	b.WriteString(renderGeneratedHeader(plan))
	b.WriteString("\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"encoding/json/jsontext\"\n")
	b.WriteString("\t\"errors\"\n")
	b.WriteString("\tjson \"encoding/json/v2\"\n")
	b.WriteString("\t\"encoding/xml\"\n")
	b.WriteString("\t\"strconv\"\n")
	b.WriteString("\t\"strings\"\n\n")
	b.WriteString("\t\"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched\"\n")
	b.WriteString(")\n\n")
	b.WriteString(renderIntervalFlagHelpers())
	b.WriteString("// omitIntervalBound reports whether the canonical encoders leave out an\n")
	b.WriteString("// interval side's bound: the side is open (its `lower_unbounded` or\n")
	b.WriteString("// `upper_unbounded` flag is set) and its bound is empty. A non-empty bound\n")
	b.WriteString("// beside its own open flag, and any bound on a bounded side, is emitted as it\n")
	b.WriteString("// stands.\n")
	b.WriteString("func omitIntervalBound[T any](unbounded bool, bound T) bool {\n")
	b.WriteString("\treturn unbounded && IsEmptyIntervalBound(bound)\n")
	b.WriteString("}\n\n")

	b.WriteString("// IsEmptyIntervalBound reports whether an interval bound holds no value.\n")
	b.WriteString("//\n")
	b.WriteString("// An interval marks an open side with its `lower_unbounded` or\n")
	b.WriteString("// `upper_unbounded` flag, and an open side has no bound. The canonical JSON\n")
	b.WriteString("// and XML encoders leave out the `lower` or `upper` of an open side whose\n")
	b.WriteString("// bound is empty, and the validation packages read such a bound as absent.\n")
	b.WriteString("// A caller that reads an interval can use this function to see the same\n")
	b.WriteString("// bound they do.\n")
	b.WriteString("//\n")
	b.WriteString("// A bound typed by an interface, such as the bound of a DVInterval[DVOrdered],\n")
	b.WriteString("// is empty when it is nil or holds a typed-nil pointer to an RM class; an\n")
	b.WriteString("// all-zero value behind the interface is still a value. A bound of a concrete\n")
	b.WriteString("// type is empty when it is that type's zero value: field by field for the RM\n")
	b.WriteString("// data value types an interval can bound, and by comparison for a Go built-in\n")
	b.WriteString("// scalar type or an RM primitive such as Integer and Real. A bound of any\n")
	b.WriteString("// other Go type is never empty, so it is emitted as it stands.\n")
	b.WriteString("func IsEmptyIntervalBound[T any](bound T) bool {\n")
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
	fmt.Fprintf(&b, "\tcase %s:\n", strings.Join(scalars, ", "))
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

// intervalBounds reports whether the target owns an interval-shaped concrete
// class, and returns the concrete classes its bounds admit, sorted by Go name.
// The list may be empty while shaped is true: a class whose bounds are all
// primitive needs no zero predicate, only the scalar case.
//
// The bound types come from each class's `lower` and `upper` fields. A field
// typed by a generic parameter admits the parameter's bound: the concrete
// descendants of an RM class (DV_ORDERED for DV_INTERVAL), or, for a
// primitive or absent bound (BASE Interval's Ordered), the scalar case. A
// field typed by a class names that class. A bound type with neither a zero
// predicate nor a scalar case is refused, so no bound is silently left
// "never empty".
func intervalBounds(plan *Plan) (bool, []*PlannedClass, error) {
	var shaped bool
	bounds := map[string]*PlannedClass{}
	for _, f := range plan.Files {
		emitting, err := codecClassesIn(plan, f)
		if err != nil {
			return false, nil, err
		}
		for _, pc := range emitting {
			fields, err := effectiveFields(plan, pc)
			if err != nil {
				return false, nil, err
			}
			if !hasIntervalShape(fields) {
				continue
			}
			shaped = true
			for _, ef := range fields {
				if name := ef.Prop.PropertyName(); name != propLower && name != propUpper {
					continue
				}
				classes, err := boundFieldClasses(plan, pc, ef)
				if err != nil {
					return false, nil, fmt.Errorf("bmmgen: %s.%s: %w", pc.BMMName, ef.Prop.PropertyName(), err)
				}
				for _, bc := range classes {
					bounds[bc.BMMName] = bc
				}
			}
		}
	}
	out := slices.SortedFunc(maps.Values(bounds), func(a, b *PlannedClass) int { return strings.Compare(a.GoName, b.GoName) })
	return shaped, out, nil
}

// boundFieldClasses returns the concrete classes one bound field of an
// interval class admits by value, each of which needs a zero predicate. It
// returns none when the field's Go type is an interface or a pointer (a nil
// test covers it) or a scalar (the scalar case covers it).
func boundFieldClasses(plan *Plan, pc *PlannedClass, ef emittedField) ([]*PlannedClass, error) {
	switch p := ef.Prop.(type) {
	case *bmm.SinglePropertyOpen:
		return boundTypeClasses(plan, genericParamBound(plan, pc, ef, p.TypeName))
	case *bmm.SingleProperty:
		typ, err := singlePropTypeExpr(plan, ef.Owner, ef.OwnerName, p)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(typ, "*") || isInterfaceTypeRef(plan, p.TypeName) {
			return nil, nil
		}
		return boundTypeClasses(plan, p.TypeName)
	}
	return nil, fmt.Errorf("a bound of property kind %T has no emptiness test", ef.Prop)
}

// genericParamBound returns the BMM bound of the generic parameter a bound
// field is typed by. The emitting class forwards its own parameter to the
// embedded Interval, so its declaration wins; otherwise the nearest ancestor
// that declares the parameter, then the field's owner, decide.
func genericParamBound(plan *Plan, pc *PlannedClass, ef emittedField, param string) string {
	sc := pc.Class.(*bmm.SimpleClass)
	if def, ok := sc.GenericParameterDefs[param]; ok {
		return def.ConformsToType
	}
	if bound := inheritedGenericBound(plan, sc, param); bound != "" {
		return bound
	}
	if def, ok := ef.Owner.GenericParameterDefs[param]; ok {
		return def.ConformsToType
	}
	return ""
}

// boundTypeClasses resolves one BMM bound type to the concrete classes that
// need a zero predicate. A type the Go code spells `any`, a Go scalar or an
// RM primitive needs none: the interface and scalar cases cover it. A class
// contributes itself when concrete and its concrete descendants when it has
// any. Anything else is refused.
func boundTypeClasses(plan *Plan, bound string) ([]*PlannedClass, error) {
	switch {
	case bound == "", bound == "Any", isSkippedPrimitive(bound), isSkippedClass(bound):
		return nil, nil
	case isPrimitive(bound):
		goType := primitiveGoType[bound]
		scalars, err := scalarBoundGoTypes()
		if err != nil {
			return nil, err
		}
		if goType == "any" || slices.Contains(scalars, scalarAlias(goType)) {
			return nil, nil
		}
		return nil, fmt.Errorf("bound type %s (Go %s) has no scalar case", bound, goType)
	}
	pc, ok := plan.Classes[bound]
	if !ok {
		return nil, fmt.Errorf("bound type %s is neither a planned class nor a primitive", bound)
	}
	var out []*PlannedClass
	if sc, isSimple := pc.Class.(*bmm.SimpleClass); isSimple && !sc.IsAbstract() {
		out = append(out, pc)
	}
	for _, name := range plan.AbstractDescendants[bound] {
		out = append(out, plan.Classes[name])
	}
	for _, name := range plan.ConcreteSubtypes[bound] {
		out = append(out, plan.Classes[name])
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("bound type %s has no concrete class to test and no scalar case", bound)
	}
	for _, c := range out {
		if err := checkPredicateClass(c); err != nil {
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

// builtinScalarGoTypes are Go's predeclared scalar types, under their own
// names; byte and rune are aliases of uint8 and int32.
var builtinScalarGoTypes = []string{
	"bool", "complex128", "complex64", "float32", "float64",
	"int", "int16", "int32", "int64", "int8", "string",
	"uint", "uint16", "uint32", "uint64", "uint8", "uintptr",
}

// scalarAlias maps Go's predeclared alias types to the type they alias.
func scalarAlias(goType string) string {
	switch goType {
	case "byte":
		return "uint8"
	case "rune":
		return "int32"
	}
	return goType
}

// scalarBoundGoTypes lists, sorted, every Go built-in scalar type and the
// named scalar types the BMM primitives map to (Integer, Real, Character).
// A bound of one of these types is empty when it equals its zero value. A
// primitive mapped to anything but an identifier (a slice, a pointer) is
// refused: it could not be compared with its zero value.
func scalarBoundGoTypes() ([]string, error) {
	set := map[string]bool{}
	for _, goType := range builtinScalarGoTypes {
		set[goType] = true
	}
	for _, name := range sortedStringKeys(primitiveGoType) {
		goType := scalarAlias(primitiveGoType[name])
		if goType == "any" {
			continue
		}
		if !isGoIdentifier(goType) {
			return nil, fmt.Errorf("bmmgen: primitive %s maps to Go %s, which has no zero comparison", name, goType)
		}
		set[goType] = true
	}
	return slices.Sorted(maps.Keys(set)), nil
}

// isGoIdentifier reports whether s is a plain Go identifier, so a type of
// that name is a named or predeclared type, not a composite type literal.
func isGoIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
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

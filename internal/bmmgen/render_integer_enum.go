package bmmgen

import (
	"bytes"
	"cmp"
	"fmt"
	"go/format"
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// integerEnumFile is the per-target file that carries the canonical codecs of
// the target's integer enumerations.
const integerEnumFile = "integer_enumeration_gen.go"

// integerEnums returns the integer enumerations plan emits, by Go name.
func integerEnums(plan *Plan) []*PlannedClass {
	var out []*PlannedClass
	for _, f := range plan.Files {
		for _, pc := range f.Classes {
			if e, ok := pc.Class.(*bmm.Enumeration); ok && e.IsIntegerEnum() && !pc.External {
				out = append(out, pc)
			}
		}
	}
	slices.SortFunc(out, func(a, b *PlannedClass) int { return cmp.Compare(a.GoName, b.GoName) })
	return out
}

// RenderIntegerEnumFile renders the target's integer_enumeration_gen.go, or
// returns nil when the target emits no integer enumeration.
//
// The rule holds for every integer enumeration the generator emits: an
// enumeration is written as its code, as element text in canonical XML and as
// a JSON number in canonical JSON; decode reads the code or the name of an
// item, as element text or as a JSON string, so two items that share a code,
// such as prohibited and disallowed, read as that code; and any other value is
// refused, with the canonical-XML shape sentinel through the text codec and
// with typereg.ErrInvalidShape in JSON. A value that is no code is an encode
// error. A JSON null leaves the value as it is, as for any member.
func RenderIntegerEnumFile(plan *Plan) ([]byte, error) {
	// The rule is REQ-176 § Shapes, the integer-enumeration bullet.
	enums := integerEnums(plan)
	if len(enums) == 0 {
		return nil, nil
	}
	var b bytes.Buffer
	b.WriteString(renderGeneratedHeader(plan))
	b.WriteString("\n")
	b.WriteString(`import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/refused"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

`)
	b.WriteString(integerEnumHelpers)
	for _, pc := range enums {
		renderIntegerEnumCodec(&b, plan, pc, pc.Class.(*bmm.Enumeration))
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), fmt.Errorf("gofmt %s: %w", integerEnumFile, err)
	}
	return formatted, nil
}

// renderIntegerEnumCodec writes the item table and the four codec methods of
// the integer enumeration e, planned as pc.
func renderIntegerEnumCodec(b *bytes.Buffer, plan *Plan, pc *PlannedClass, e *bmm.Enumeration) {
	goName := pc.GoName
	items := "itemsOf" + goName
	qualified := plan.Target.GoPackage + "." + goName

	fmt.Fprintf(b, "// %s are the items of %s, each by its name and its code.\n", items, goName)
	fmt.Fprintf(b, "var %s = []integerEnumItem[%s]{\n", items, goName)
	for _, n := range e.ItemNames {
		fmt.Fprintf(b, "\t{%q, %s},\n", n, enumConstName(goName, n))
	}
	b.WriteString("}\n\n")

	fmt.Fprintf(b, "// MarshalText returns the code of k, the text canonical XML writes for it.\n")
	fmt.Fprintf(b, "// A value that is no code of %s is an error.\n", goName)
	fmt.Fprintf(b, "func (k %s) MarshalText() ([]byte, error) {\n", goName)
	fmt.Fprintf(b, "\treturn marshalIntegerEnumText(%q, %s, k)\n}\n\n", qualified, items)

	fmt.Fprintf(b, "// UnmarshalText reads text, the code of an item of %s or its name, into k.\n", goName)
	b.WriteString("// XML white space around the text is ignored; any other text is an error, and\n")
	b.WriteString("// so is a nil receiver.\n")
	fmt.Fprintf(b, "func (k *%s) UnmarshalText(text []byte) error {\n", goName)
	fmt.Fprintf(b, "\treturn unmarshalIntegerEnumText(%q, %s, k, text)\n}\n\n", qualified, items)

	fmt.Fprintf(b, "// MarshalJSONTo writes the code of k as a JSON number, as canonical JSON writes\n")
	fmt.Fprintf(b, "// it. A value that is no code of %s is an error.\n", goName)
	fmt.Fprintf(b, "func (k %s) MarshalJSONTo(enc *jsontext.Encoder) error {\n", goName)
	fmt.Fprintf(b, "\treturn marshalIntegerEnumJSON(%q, %s, k, enc)\n}\n\n", qualified, items)

	fmt.Fprintf(b, "// UnmarshalJSONFrom reads a JSON number holding the code of an item of %s, or\n", goName)
	b.WriteString("// a JSON string holding its name, into k. A JSON null leaves k as it is. Any\n")
	b.WriteString("// other value is an error that carries typereg.ErrInvalidShape, and so is a\n")
	b.WriteString("// nil receiver, with typereg.ErrNilReceiver.\n")
	fmt.Fprintf(b, "func (k *%s) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n", goName)
	fmt.Fprintf(b, "\treturn unmarshalIntegerEnumJSON(%q, %s, k, dec)\n}\n\n", qualified, items)
}

// integerEnumHelpers is the code every integer enumeration's codec shares.
// The error messages name the Go type and never the value refused.
const integerEnumHelpers = `// integerEnumItem is one item of an integer enumeration: its name and its
// code.
type integerEnumItem[T ~int32] struct {
	name string
	code T
}

// errNotAnItem refuses a value that is neither the code of an item of its
// enumeration nor the name of one.
var errNotAnItem = errors.New("neither the code nor the name of an item of the enumeration")

// errNotACode refuses to encode a value that is no code of its enumeration.
var errNotACode = errors.New("not the code of an item of the enumeration")

// xmlWhiteSpace is the XML white space a text decode ignores around the text.
const xmlWhiteSpace = " \t\r\n"

// integerEnumByName returns the code of the item of items named name.
func integerEnumByName[T ~int32](items []integerEnumItem[T], name string) (T, bool) {
	for _, it := range items {
		if it.name == name {
			return it.code, true
		}
	}
	return 0, false
}

// integerEnumByCode returns the code the decimal text writes, when it is the
// code of an item of items.
func integerEnumByCode[T ~int32](items []integerEnumItem[T], text string) (T, bool) {
	n, err := strconv.ParseInt(text, 10, 32)
	if err != nil || !isIntegerEnumCode(items, T(n)) {
		return 0, false
	}
	return T(n), true
}

// isIntegerEnumCode reports whether v is the code of an item of items.
func isIntegerEnumCode[T ~int32](items []integerEnumItem[T], v T) bool {
	for _, it := range items {
		if it.code == v {
			return true
		}
	}
	return false
}

// marshalIntegerEnumText returns v as the decimal text of its code.
func marshalIntegerEnumText[T ~int32](typeName string, items []integerEnumItem[T], v T) ([]byte, error) {
	if !isIntegerEnumCode(items, v) {
		return nil, fmt.Errorf("%s: %w", typeName, errNotACode)
	}
	return strconv.AppendInt(nil, int64(v), 10), nil
}

// unmarshalIntegerEnumText reads text, less the XML white space around it, as
// the name or the code of an item of items into v.
func unmarshalIntegerEnumText[T ~int32](typeName string, items []integerEnumItem[T], v *T, text []byte) error {
	if v == nil {
		return fmt.Errorf("%s: %w", typeName, typereg.ErrNilReceiver)
	}
	s := strings.Trim(string(text), xmlWhiteSpace)
	code, ok := integerEnumByName(items, s)
	if !ok {
		code, ok = integerEnumByCode(items, s)
	}
	if !ok {
		return fmt.Errorf("%s: %w", typeName, errNotAnItem)
	}
	*v = code
	return nil
}

// marshalIntegerEnumJSON writes v as a JSON number, its code.
func marshalIntegerEnumJSON[T ~int32](typeName string, items []integerEnumItem[T], v T, enc *jsontext.Encoder) error {
	if !isIntegerEnumCode(items, v) {
		return fmt.Errorf("%s: %w", typeName, errNotACode)
	}
	return enc.WriteToken(jsontext.Int(int64(v)))
}

// unmarshalIntegerEnumJSON reads the next JSON value of dec into v: a number
// holding the code of an item of items, or a string holding its name. A null
// leaves v as it is. Any other value is refused with typereg.ErrInvalidShape,
// carrying its JSON text for the entry point's report.
func unmarshalIntegerEnumJSON[T ~int32](typeName string, items []integerEnumItem[T], v *T, dec *jsontext.Decoder) error {
	if v == nil {
		return fmt.Errorf("%s: %w", typeName, typereg.ErrNilReceiver)
	}
	val, err := dec.ReadValue()
	if err != nil {
		return err
	}
	var code T
	ok := false
	switch val.Kind() {
	case 'n':
		return nil
	case '0':
		code, ok = integerEnumByCode(items, string(val))
	case '"':
		if name, err := jsontext.AppendUnquote(nil, val); err == nil {
			code, ok = integerEnumByName(items, string(name))
		}
	}
	if !ok {
		return refused.Literal(val, typereg.ClassifyShape(fmt.Errorf("%s: %w", typeName, errNotAnItem)))
	}
	*v = code
	return nil
}

`

package canxml_test

import (
	"encoding/xml"
	"errors"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestREQ056_EncodeTextTakesFloatAndStringKindsOnly pins the type parameter
// of canxml.EncodeText (REQ-056): a type whose underlying type is float64 or
// string, with a MarshalText method, as rm.Real and rm.Character are, while a
// pointer, which EncodeElement writes nothing for when it is nil, and a struct
// are refused when the call compiles. Can-fail: with the constraint loosened
// to encoding.TextMarshaler, the pointer and struct calls compile.
func TestREQ056_EncodeTextTakesFloatAndStringKindsOnly(t *testing.T) {
	const decls = `package p

import (
	"encoding/xml"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

type number float64

func (number) MarshalText() ([]byte, error) { return nil, nil }

type text string

func (text) MarshalText() ([]byte, error) { return nil, nil }

type record struct{}

func (record) MarshalText() ([]byte, error) { return nil, nil }
`
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)
	check := func(call string) error {
		file, err := parser.ParseFile(fset, "p.go", decls+"\nfunc f(e *xml.Encoder) error { return "+call+" }\n", 0)
		if err != nil {
			t.Fatalf("parse %s: %v", call, err)
		}
		_, err = (&types.Config{Importer: imp}).Check("p", fset, []*ast.File{file}, nil)
		return err
	}
	for _, call := range []string{
		`canxml.EncodeText(e, "x", number(1))`,
		`canxml.EncodeText(e, "x", text("a"))`,
	} {
		if err := check(call); err != nil {
			t.Errorf("%s does not compile: %v; want it to", call, err)
		}
	}
	for _, call := range []string{
		`canxml.EncodeText(e, "x", new(number))`,
		`canxml.EncodeText[*text](e, "x", nil)`,
		`canxml.EncodeText(e, "x", record{})`,
	} {
		if err := check(call); err == nil || !strings.Contains(err.Error(), "does not satisfy") {
			t.Errorf("%s type-checks with %v; want the type parameter refused", call, err)
		}
	}
}

// textOnly is a type that marshals itself to text, refusing "fail".
type textOnly string

func (s textOnly) MarshalText() ([]byte, error) {
	if s == "fail" {
		return nil, errors.New("refused")
	}
	return []byte("text " + s), nil
}

// selfWritten marshals itself to the same text as a textOnly, and has a
// MarshalXML of its own.
type selfWritten string

func (s selfWritten) MarshalText() ([]byte, error) { return textOnly(s).MarshalText() }

func (s selfWritten) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement("xml "+string(s), start)
}

// TestREQ056_EncodeTextCallsNoMarshalXML pins what canxml.EncodeText's doc
// leaves out of EncodeElement's bytes (REQ-056): for a type with a MarshalXML
// of its own, EncodeText writes what EncodeElement writes for a type that
// only marshals itself to text, the same text included, where EncodeElement
// calls MarshalXML: for a text to escape, a newline, and a failure to marshal.
// Can-fail: hand v to EncodeElement where the text holds a newline or fails
// to marshal, and those rows write MarshalXML's text.
func TestREQ056_EncodeTextCallsNoMarshalXML(t *testing.T) {
	t.Parallel()
	start := xml.StartElement{Name: xml.Name{Local: "match"}}
	for _, s := range []string{"x", "a<b&c", "line\nbreak", "fail"} {
		got := encodeNested(func(e *xml.Encoder) error { return canxml.EncodeText(e, "match", selfWritten(s)) })
		want := encodeNested(func(e *xml.Encoder) error { return e.EncodeElement(textOnly(s), start) })
		if got != want {
			t.Errorf("EncodeText of %q, of a type with its own MarshalXML, wrote\n%s\nwant what EncodeElement writes for its text\n%s", s, got, want)
		}
	}
}

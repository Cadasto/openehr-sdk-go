package rmjsonwalk

import (
	"bytes"
	"encoding/json/jsontext"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// logHooks returns hooks that append each call to *log, as "unmatched
// <pointer>" or "missing <pointer>", so one log shows both hooks in the order
// they were called.
func logHooks(log *[]string) Hooks {
	return Hooks{
		Unmatched: func(p string) error { *log = append(*log, "unmatched "+p); return nil },
		Missing:   func(p string) error { *log = append(*log, "missing "+p); return nil },
	}
}

// jsonNode is a JSON value with each object's members kept in document order
// and their names kept as written, quotes included, so a test can change how
// a document spells its objects and write it out again.
type jsonNode struct {
	kind   jsontext.Kind // '{', '[', or the kind of a scalar
	raw    string        // a scalar as written
	names  []string      // an object's member names, as written
	vals   []*jsonNode   // an object's member values or an array's elements
	spaced bool          // write JSON whitespace around each member's name, colon and value
}

// parseNode reads the next value from dec.
func parseNode(dec *jsontext.Decoder) (*jsonNode, error) {
	kind := dec.PeekKind()
	if kind != '{' && kind != '[' {
		v, err := dec.ReadValue()
		if err != nil {
			return nil, err
		}
		return &jsonNode{kind: kind, raw: string(v)}, nil
	}
	if _, err := dec.ReadToken(); err != nil {
		return nil, err
	}
	n := &jsonNode{kind: kind}
	end := jsontext.Kind(']')
	if kind == '{' {
		end = '}'
	}
	for dec.PeekKind() != end {
		if kind == '{' {
			tok, err := dec.ReadToken()
			if err != nil {
				return nil, err
			}
			quoted, err := jsontext.AppendQuote(nil, tok.String())
			if err != nil {
				return nil, err
			}
			n.names = append(n.names, string(quoted))
		}
		v, err := parseNode(dec)
		if err != nil {
			return nil, err
		}
		n.vals = append(n.vals, v)
	}
	if _, err := dec.ReadToken(); err != nil {
		return nil, err
	}
	return n, nil
}

// write appends n as JSON to b.
func (n *jsonNode) write(b *bytes.Buffer) {
	if n.kind != '{' && n.kind != '[' {
		b.WriteString(n.raw)
		return
	}
	b.WriteByte(byte(n.kind))
	for i, v := range n.vals {
		if i > 0 {
			b.WriteByte(',')
		}
		if n.kind == '{' {
			if n.spaced {
				b.WriteString(" \n\t")
			}
			b.WriteString(n.names[i])
			if n.spaced {
				b.WriteString("\r\n :\t ")
			} else {
				b.WriteByte(':')
			}
		}
		v.write(b)
		if n.spaced {
			b.WriteString(" \n")
		}
	}
	if n.kind == '{' {
		b.WriteByte('}')
	} else {
		b.WriteByte(']')
	}
}

// objects returns every object in n, n included, in document order.
func (n *jsonNode) objects() []*jsonNode {
	var out []*jsonNode
	if n.kind == '{' {
		out = append(out, n)
	}
	for _, v := range n.vals {
		out = append(out, v.objects()...)
	}
	return out
}

// typeAt returns the position of the object's "_type" member, or -1.
func (n *jsonNode) typeAt() int {
	return slices.Index(n.names, `"_type"`)
}

// addStray adds a member no RM class has, after the object's own members, or
// before them when first is true.
func (n *jsonNode) addStray(first bool) {
	stray := &jsonNode{kind: '0', raw: "1"}
	if first {
		n.names = slices.Insert(n.names, 0, `"x_stray"`)
		n.vals = slices.Insert(n.vals, 0, stray)
		return
	}
	n.names = append(n.names, `"x_stray"`)
	n.vals = append(n.vals, stray)
}

// dropMember removes the object's first member other than "_type".
func (n *jsonNode) dropMember() {
	for i, name := range n.names {
		if name != `"_type"` {
			n.names = slices.Delete(n.names, i, i+1)
			n.vals = slices.Delete(n.vals, i, i+1)
			return
		}
	}
}

// moveTypeLast moves the object's "_type" member after its other members.
func (n *jsonNode) moveTypeLast() {
	if i := n.typeAt(); i >= 0 {
		name, v := n.names[i], n.vals[i]
		n.names = append(slices.Delete(n.names, i, i+1), name)
		n.vals = append(slices.Delete(n.vals, i, i+1), v)
	}
}

// escapeTypeName spells the object's "_type" member name with an escape.
func (n *jsonNode) escapeTypeName() {
	if i := n.typeAt(); i >= 0 {
		n.names[i] = `"\u005ftype"`
	}
}

// escapeTypeValue spells the first letter of the object's "_type" string
// with an escape.
func (n *jsonNode) escapeTypeValue() {
	if i := n.typeAt(); i >= 0 && n.vals[i].kind == '"' && len(n.vals[i].raw) > 2 {
		raw := n.vals[i].raw
		n.vals[i].raw = fmt.Sprintf(`"\u%04x`, raw[1]) + raw[2:]
	}
}

// onePassVariants rewrite a document's objects, each given its position in
// document order (the root is 0), and say whether the one pass stands for
// the two on the result. The stray members give both hooks something to
// report, so a pass that read an object as the wrong type, or reported a
// member twice, shows in the report.
var onePassVariants = []struct {
	name string
	edit func(i int, n *jsonNode)
	once bool
}{
	{
		name: "as vendored",
		once: true,
	},
	{
		name: "a stray member in every object, a member dropped from every fourth",
		edit: func(i int, n *jsonNode) {
			n.addStray(false)
			if i%4 == 1 {
				n.dropMember()
			}
		},
		once: true,
	},
	{
		name: "whitespace around every member, a stray member in every object",
		edit: func(_ int, n *jsonNode) {
			n.spaced = true
			n.addStray(false)
		},
		once: true,
	},
	{
		name: "_type last in every third object",
		edit: func(i int, n *jsonNode) {
			n.addStray(false)
			if i%3 == 0 {
				n.moveTypeLast()
			}
		},
	},
	{
		name: "_type spelled with an escape in every fifth object",
		edit: func(i int, n *jsonNode) {
			n.addStray(false)
			if i%5 == 0 {
				n.escapeTypeName()
			}
		},
	},
	{
		name: "the _type string spelled with an escape in every fifth object",
		edit: func(i int, n *jsonNode) {
			n.addStray(false)
			if i%5 == 0 {
				n.escapeTypeValue()
			}
		},
	},
	{
		name: "a stray member before _type in every seventh object",
		edit: func(i int, n *jsonNode) {
			n.addStray(i%7 == 0)
		},
	},
}

// TestREQ175_REQ052_OnePassReportsWhatTwoPassesReport pins that the one pass
// over a document changes nothing the hooks see: on every corpus composition,
// as vendored and rewritten with "_type" moved after other members, spelled
// with an escape, surrounded by whitespace, or preceded by a stray member,
// Walk reports exactly what walkIndexed, the walk that indexes every object's
// "_type" first, reports: the same calls, in the same order, and the same
// error. It also pins which documents the one pass completes on, so a pass
// that read a late "_type" as absent, or gave up on a leading one, fails here.
// The reports are canjson's WithMissing (REQ-175) and WithUnmatched and
// WithStrict (REQ-052), and ValidateJSON's report of missing members.
func TestREQ175_REQ052_OnePassReportsWhatTwoPassesReport(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	rels, err := fixtures.ListCompositionJSON()
	if err != nil {
		t.Fatalf("ListCompositionJSON(): %v", err)
	}
	var docs int
	for _, rel := range rels {
		if rel.Kind != "compositions" {
			continue
		}
		docs++
		path := filepath.Join(fixtures.CorpusRoot(), filepath.FromSlash(rel.Rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel.Rel, err)
		}
		rootType, err := fixtures.RootTypeFromJSON(path)
		if err != nil {
			t.Fatalf("RootTypeFromJSON(%s): %v", rel.Rel, err)
		}
		root := Root{Type: rootType}
		for _, v := range onePassVariants {
			t.Run(rel.Template+"/"+v.name, func(t *testing.T) {
				doc := data
				if v.edit != nil {
					tree, err := parseNode(jsontext.NewDecoder(bytes.NewReader(data)))
					if err != nil {
						t.Fatalf("parse %s: %v", rel.Rel, err)
					}
					for i, obj := range tree.objects() {
						v.edit(i, obj)
					}
					var b bytes.Buffer
					tree.write(&b)
					doc = b.Bytes()
				}
				var want, got []string
				wantErr := m.walkIndexed(doc, root, logHooks(&want))
				gotErr := m.Walk(doc, root, logHooks(&got))
				if wantErr != nil || gotErr != nil {
					t.Fatalf("walkIndexed error = %v, Walk error = %v; want both nil", wantErr, gotErr)
				}
				if !slices.Equal(got, want) {
					t.Errorf("Walk reported\n%q\nwant what walkIndexed reports,\n%q", got, want)
				}
				if v.edit != nil && len(want) == 0 {
					t.Errorf("walkIndexed reported nothing on the %q rewrite: the case cannot tell the passes apart", v.name)
				}
				if _, once := m.walkOnce(doc, root, logHooks(new([]string))); once != v.once {
					t.Errorf("walkOnce completed = %t, want %t", once, v.once)
				}
			})
		}
		t.Run(rel.Template+"/cut short", func(t *testing.T) {
			doc := data[:len(data)/2]
			wantErr := m.walkIndexed(doc, root, logHooks(new([]string)))
			var got []string
			gotErr := m.Walk(doc, root, logHooks(&got))
			if wantErr == nil || gotErr == nil || gotErr.Error() != wantErr.Error() {
				t.Errorf("Walk error = %v, want walkIndexed's %v", gotErr, wantErr)
			}
			if len(got) != 0 {
				t.Errorf("Walk on a malformed document reported %q, want no hook called", got)
			}
		})
	}
	if docs == 0 {
		t.Fatal("ListCompositionJSON() listed no composition: nothing was compared")
	}
}

// TestLeadingTypeReadsOnlyAPlainLeadingMember pins what the one pass takes
// as an object's "_type" from the bytes after the object's opening brace: a
// first member named "_type" with no escape, whitespace allowed around it,
// whose value is a string of ASCII text with no escape. Anything else
// is left to the walk, which gives the pass up when it meets the member.
func TestLeadingTypeReadsOnlyAPlainLeadingMember(t *testing.T) {
	cases := []struct {
		name, unread string
		typ          string
		ok           bool
	}{
		{name: "leading", unread: `"_type":"DV_TEXT","value":"v"}`, typ: "DV_TEXT", ok: true},
		{name: "whitespace around name, colon and value", unread: " \t\r\n\"_type\" \n:\t \"DV_TEXT\" ,\"value\":\"v\"}", typ: "DV_TEXT", ok: true},
		{name: "empty string", unread: `"_type":"","value":"v"}`, typ: "", ok: true},
		{name: "not first", unread: `"value":"v","_type":"DV_TEXT"}`},
		{name: "escaped name", unread: `"\u005ftype":"DV_TEXT"}`},
		{name: "escaped value", unread: `"_type":"DV_\u0054EXT"}`},
		{name: "escaped quote in value", unread: `"_type":"DV\"TEXT"}`},
		{name: "non-ASCII value", unread: `"_type":"DV_TÉXT"}`},
		{name: "control byte in value", unread: "\"_type\":\"DV\tTEXT\"}"},
		{name: "null value", unread: `"_type":null}`},
		{name: "number value", unread: `"_type":1}`},
		{name: "longer name", unread: `"_types":"DV_TEXT"}`},
		{name: "name only", unread: `"_type"`},
		{name: "no colon", unread: `"_type" "DV_TEXT"}`},
		{name: "unterminated value", unread: `"_type":"DV_TEXT`},
		{name: "empty object", unread: `}`},
		{name: "nothing", unread: ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			typ, ok := leadingType([]byte(tc.unread))
			if typ != tc.typ || ok != tc.ok {
				t.Errorf("leadingType(%q) = %q, %t; want %q, %t", tc.unread, typ, ok, tc.typ, tc.ok)
			}
		})
	}
}

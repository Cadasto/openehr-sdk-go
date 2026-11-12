package reportpath_test

// reportpath_test.go: REQ-170 and REQ-175 (wire.md § REQ-170 *Report*,
// § REQ-175 *Identification*). A node writes the JSON Pointer or XML path
// text the report options pass, shares the nodes of the path before it, and
// keeps its text once a later sibling is made. It gives its parent and its
// own segment, unescaped, without writing any text.

import (
	"bytes"
	"encoding/json/jsontext"
	"strings"
	"testing"
	"unsafe"

	"github.com/cadasto/openehr-sdk-go/internal/reportpath"
)

func TestNodeText(t *testing.T) {
	var root *reportpath.Node
	folder := root.Element("folder", 1)
	rows := []struct {
		name string
		node *reportpath.Node
		want string
	}{
		{name: "root", node: root, want: ""},
		{name: "token", node: root.Token("content"), want: "/content"},
		{name: "token escaped", node: root.Token("a~b/c~/"), want: "/a~0b~1c~0~1"},
		{name: "empty token", node: root.Token(""), want: "/"},
		{name: "token of several bytes per rune", node: root.Token("é/ü"), want: "/é~1ü"},
		{name: "name as written", node: root.Name("x-note"), want: "/x-note"},
		{name: "index", node: root.Token("items").Index(0).Index(12345), want: "/items/0/12345"},
		{name: "element", node: folder.Element("folders", 10), want: "/folder[1]/folders[10]"},
		{name: "attribute", node: folder.Attr("lang"), want: "/folder[1]/@lang"},
		{name: "text", node: folder.Text(), want: "/folder[1]/text()"},
		{name: "mixed", node: root.Name("description").Name("details").Token("en/GB").Name("use"), want: "/description/details/en~1GB/use"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if got := row.node.String(); got != row.want {
				t.Errorf("String() = %q, want %q", got, row.want)
			}
			if got := row.node.Len(); got != len(row.want) {
				t.Errorf("Len() = %d, want %d", got, len(row.want))
			}
			prefix := []byte("prefix")
			if got := row.node.AppendText(prefix); string(got) != "prefix"+row.want {
				t.Errorf("AppendText(%q) = %q, want %q", prefix, got, "prefix"+row.want)
			}
		})
	}
}

// TestNodeSharesItsParent holds that the paths of siblings share the nodes
// before them and stay what they were when a later sibling is made.
func TestNodeSharesItsParent(t *testing.T) {
	var root *reportpath.Node
	object := root.Token("content").Index(0)
	first := object.Token("name")
	second := object.Token("uid")
	if got, want := first.String(), "/content/0/name"; got != want {
		t.Errorf("first.String() = %q, want %q", got, want)
	}
	if got, want := second.String(), "/content/0/uid"; got != want {
		t.Errorf("second.String() = %q, want %q", got, want)
	}
}

func TestNodeSetToken(t *testing.T) {
	var root *reportpath.Node
	entry := root.Name("details").Token("")
	member := entry.Name("use")
	entry.SetToken("en~1")
	if got, want := member.String(), "/details/en~01/use"; got != want {
		t.Errorf("String() after SetToken = %q, want %q", got, want)
	}
	if got, want := entry.Last(), "en~1"; got != want {
		t.Errorf("Last() after SetToken = %q, want %q", got, want)
	}
	if parent, ok := member.Parent(); !ok || parent != entry {
		t.Errorf("member.Parent() = %p, %t, want the entry %p, true", parent, ok, entry)
	}
}

// escapeToken escapes s as one JSON Pointer reference token (RFC 6901).
var escapeToken = strings.NewReplacer("~", "~0", "/", "~1").Replace

// TestNodeParentAndLast holds that a node gives the node it was made from as
// its parent, and its own segment as a value: a token or a name as given,
// never escaped, a position in decimal, and an XML segment as written
// without its leading "/". The parent's text, "/" and the escaped segment
// give the node's text.
func TestNodeParentAndLast(t *testing.T) {
	var root *reportpath.Node
	folder := root.Element("folder", 1)
	items := root.Token("items")
	rows := []struct {
		name   string
		parent *reportpath.Node
		node   *reportpath.Node
		want   string
	}{
		{name: "token", parent: root, node: root.Token("content"), want: "content"},
		{name: "token with escape characters", parent: items, node: items.Token("a~b/c~/"), want: "a~b/c~/"},
		{name: "token that reads escaped", parent: items, node: items.Token("~0~1"), want: "~0~1"},
		{name: "empty token", parent: root, node: root.Token(""), want: ""},
		{name: "token of several bytes per rune", parent: items, node: items.Token("é/ü"), want: "é/ü"},
		{name: "name", parent: folder, node: folder.Name("x-note"), want: "x-note"},
		{name: "index 0", parent: items, node: items.Index(0), want: "0"},
		{name: "index", parent: items, node: items.Index(12345), want: "12345"},
		{name: "element", parent: folder, node: folder.Element("folders", 10), want: "folders[10]"},
		{name: "root element", parent: root, node: folder, want: "folder[1]"},
		{name: "attribute", parent: folder, node: folder.Attr("lang"), want: "@lang"},
		{name: "text", parent: folder, node: folder.Text(), want: "text()"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			parent, ok := row.node.Parent()
			if !ok || parent != row.parent {
				t.Errorf("Parent() = %p (%q), %t, want %p (%q), true", parent, parent.String(), ok, row.parent, row.parent.String())
			}
			got := row.node.Last()
			if got != row.want {
				t.Errorf("Last() = %q, want %q", got, row.want)
			}
			if joined := parent.String() + "/" + escapeToken(got); joined != row.node.String() {
				t.Errorf("parent text + \"/\" + escaped Last() = %q, want String() = %q", joined, row.node.String())
			}
		})
	}
}

// TestNodeRootHasNoParent holds that the empty path has no parent and no
// segment.
func TestNodeRootHasNoParent(t *testing.T) {
	var root *reportpath.Node
	if parent, ok := root.Parent(); ok || parent != nil {
		t.Errorf("root Parent() = %p, %t, want nil, false", parent, ok)
	}
	if got := root.Last(); got != "" {
		t.Errorf("root Last() = %q, want %q", got, "")
	}
}

var (
	parentSink *reportpath.Node
	lastSink   string
)

// TestNodeParentAndLastAllocateNothing holds that reading a node's parent,
// and the token, name or small position it holds, allocates nothing: a
// consumer walks a report's paths without writing their text.
func TestNodeParentAndLastAllocateNothing(t *testing.T) {
	var root *reportpath.Node
	for _, node := range []*reportpath.Node{root.Token("a~b/c"), root.Name("x-note"), root.Index(7), root.Element("a", 1).Text()} {
		allocs := testing.AllocsPerRun(100, func() {
			parentSink, _ = node.Parent()
			lastSink = node.Last()
		})
		if allocs != 0 {
			t.Errorf("Parent() and Last() of %q allocate %v times, want 0", node.String(), allocs)
		}
	}
}

// TestNodeTokenMatchesJSONText holds the token and the index to the text
// encoding/json/jsontext writes in a decoder's StackPointer.
func TestNodeTokenMatchesJSONText(t *testing.T) {
	doc := `{"a/b":{"c~d":[0,{"é~/x":1,"":{"~0":[true,null]}}]}}`
	dec := jsontext.NewDecoder(bytes.NewBufferString(doc))
	if n := matchJSONText(t, dec, nil); n != 9 {
		t.Fatalf("checked %d paths, want 9", n)
	}
}

// matchJSONText reads the next value of dec, whose path is at, and checks the
// path of each member name and each scalar against dec's StackPointer. It
// returns the number of paths it checked.
func matchJSONText(t *testing.T, dec *jsontext.Decoder, at *reportpath.Node) int {
	t.Helper()
	checked := 0
	kind := dec.PeekKind()
	if kind != '{' && kind != '[' {
		if err := dec.SkipValue(); err != nil {
			t.Fatal(err)
		}
		if got, want := at.String(), string(dec.StackPointer()); got != want {
			t.Errorf("value path = %q, want %q (jsontext)", got, want)
		}
		return 1
	}
	if _, err := dec.ReadToken(); err != nil {
		t.Fatal(err)
	}
	if kind == '[' {
		for i := 0; dec.PeekKind() != ']'; i++ {
			checked += matchJSONText(t, dec, at.Index(i))
		}
	}
	for kind == '{' && dec.PeekKind() != '}' {
		name, err := dec.ReadToken()
		if err != nil {
			t.Fatal(err)
		}
		member := at.Token(name.String())
		if got, want := member.String(), string(dec.StackPointer()); got != want {
			t.Errorf("member path = %q, want %q (jsontext)", got, want)
		}
		checked += 1 + matchJSONText(t, dec, member)
	}
	if _, err := dec.ReadToken(); err != nil {
		t.Fatal(err)
	}
	return checked
}

// TestNodeBytes holds the size [reportpath.NodeBytes] counts a node for to
// at least the node's own size.
func TestNodeBytes(t *testing.T) {
	if size := unsafe.Sizeof(reportpath.Node{}); size > reportpath.NodeBytes {
		t.Errorf("unsafe.Sizeof(Node{}) = %d, more than NodeBytes = %d", size, reportpath.NodeBytes)
	}
}

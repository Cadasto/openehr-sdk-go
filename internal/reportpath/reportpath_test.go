package reportpath_test

// reportpath_test.go: REQ-170 and REQ-175 (wire.md § REQ-170 *Report*,
// § REQ-175 *Identification*). A node writes the JSON Pointer or XML path
// text the report options pass, shares the nodes of the path before it, and
// keeps its text once a later sibling is made. It gives its parent and its
// own segment, unescaped, without writing any text.

import (
	"bytes"
	"encoding/json/jsontext"
	"strconv"
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

var stringSink string

// TestNodeStringAllocatesOnce holds that writing a path's text allocates the
// text and nothing else, for paths of every kind of segment. Each path is
// longer than 32 bytes: the compiler may keep a shorter buffer on the stack,
// so a shorter path would not show a second copy of the text.
func TestNodeStringAllocatesOnce(t *testing.T) {
	var root *reportpath.Node
	composition := root.Element("composition", 1)
	rows := []struct {
		name string
		node *reportpath.Node
	}{
		{name: "tokens", node: root.Token("content").Token("data").Token("events").Token("archetype_node_id")},
		{name: "escaped tokens", node: root.Token("details").Token("en/GB").Token("a~b/c~/").Token("value")},
		{name: "names", node: root.Name("description").Name("details").Name("purpose").Name("x-note")},
		{name: "indexes", node: root.Token("items").Index(0).Index(12345).Index(7).Index(1000000).Index(42).Index(99).Index(123456)},
		{name: "elements", node: composition.Element("content", 2).Element("data", 1).Element("events", 10)},
		{name: "attribute", node: composition.Element("language", 1).Element("terminology_id", 1).Attr("lang")},
		{name: "text", node: composition.Element("name", 1).Element("value", 1).Text()},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if n := row.node.Len(); n <= 32 {
				t.Fatalf("Len() = %d, want a path longer than 32 bytes", n)
			}
			allocs := testing.AllocsPerRun(100, func() {
				stringSink = row.node.String()
			})
			if allocs != 1 {
				t.Errorf("String() of %q allocates %v times, want 1", row.node.String(), allocs)
			}
		})
	}
	if allocs := testing.AllocsPerRun(100, func() { stringSink = root.String() }); allocs != 0 {
		t.Errorf("String() of the empty path allocates %v times, want 0", allocs)
	}
}

// TestNodeStringMatchesText holds the text String gives to the text
// AppendText gives and to the text the segments make one by one, for the
// empty path, every kind of segment, escaped tokens, long segments and a
// deep path.
func TestNodeStringMatchesText(t *testing.T) {
	var root *reportpath.Node
	long := strings.Repeat("abcdefgh", 40)
	deep, deepText := deepPath(1000)
	rows := []struct {
		name string
		node *reportpath.Node
		want string
	}{
		{name: "root", node: root, want: ""},
		{name: "token escaped", node: root.Token("a~b/c~/"), want: "/a~0b~1c~0~1"},
		{name: "empty tokens", node: root.Token("").Token(""), want: "//"},
		{name: "index", node: root.Index(0).Index(-1).Index(12345), want: "/0/-1/12345"},
		{name: "xml", node: root.Element("folder", 1).Name("x-note").Attr("lang").Text(), want: "/folder[1]/x-note/@lang/text()"},
		{name: "long token", node: root.Token("items").Token(long + "/~"), want: "/items/" + long + "~1~0"},
		{name: "long escaped token", node: root.Token(strings.Repeat("/~", 200)), want: "/" + strings.Repeat("~1~0", 200)},
		{name: "long name", node: root.Name(long).Name("use"), want: "/" + long + "/use"},
		{name: "long element", node: root.Element(long, 123456789), want: "/" + long + "[123456789]"},
		{name: "long attribute", node: root.Element("a", 1).Attr(long), want: "/a[1]/@" + long},
		{name: "deep", node: deep, want: deepText},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			got := row.node.String()
			if got != row.want {
				t.Errorf("String() = %q, want %q", got, row.want)
			}
			if appended := string(row.node.AppendText(nil)); got != appended {
				t.Errorf("String() = %q, want the text AppendText(nil) gives, %q", got, appended)
			}
		})
	}
}

// deepPath returns a path of n segments, cycling through every kind of
// segment, and its text.
func deepPath(n int) (*reportpath.Node, string) {
	var (
		node *reportpath.Node
		text strings.Builder
	)
	for i := range n {
		switch i % 6 {
		case 0:
			node = node.Token("en/GB~" + strconv.Itoa(i))
			text.WriteString("/en~1GB~0" + strconv.Itoa(i))
		case 1:
			node = node.Name("details")
			text.WriteString("/details")
		case 2:
			node = node.Index(i)
			text.WriteString("/" + strconv.Itoa(i))
		case 3:
			node = node.Element("items", i)
			text.WriteString("/items[" + strconv.Itoa(i) + "]")
		case 4:
			node = node.Attr("lang")
			text.WriteString("/@lang")
		case 5:
			node = node.Text()
			text.WriteString("/text()")
		}
	}
	return node, text.String()
}

// BenchmarkNodeString writes the text of paths as deep as a typical report
// path and as deep as a decode's depth limit allows.
func BenchmarkNodeString(b *testing.B) {
	for _, depth := range []int{8, 250, 1000} {
		node, _ := deepPath(depth)
		b.Run("depth="+strconv.Itoa(depth), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				stringSink = node.String()
			}
		})
	}
}

// TestNodeBytes holds the size [reportpath.NodeBytes] counts a node for to
// at least the node's own size.
func TestNodeBytes(t *testing.T) {
	if size := unsafe.Sizeof(reportpath.Node{}); size > reportpath.NodeBytes {
		t.Errorf("unsafe.Sizeof(Node{}) = %d, more than NodeBytes = %d", size, reportpath.NodeBytes)
	}
}

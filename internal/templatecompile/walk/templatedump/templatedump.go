package templatedump

import (
	"fmt"
	"io"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templatecompile/walk"
)

// Printer is a [walk.Visitor] that renders the compiled OPT tree as
// an indented text dump, one node per line. The zero value writes
// to a strings.Builder accessible via [Printer.String]; supply an
// alternative writer via NewPrinter.
//
// Each line carries the AQL path plus the RM type name, with a
// "(slot)" tag for *Slot leaves. Implicit attributes (rminfo-injected)
// are rendered with an asterisk before the attribute name.
//
// Use as:
//
//	p := &templatedump.Printer{}
//	walk.Walk(c, p)
//	fmt.Println(p.String())
type Printer struct {
	// Writer receives output. Nil means "buffer in String()".
	Writer io.Writer

	// Indent is the per-depth prefix. Defaults to two spaces when
	// the zero value is in use.
	Indent string

	buf strings.Builder
}

// NewPrinter constructs a Printer that writes to w with the given
// indent string. When indent is "", two spaces are used.
func NewPrinter(w io.Writer, indent string) *Printer {
	if indent == "" {
		indent = "  "
	}
	return &Printer{Writer: w, Indent: indent}
}

// PreHandle emits the current node's line. The compiled tree's
// post-order would re-emit each node — we use PreHandle only.
func (p *Printer) PreHandle(ctx *walk.Context) error {
	n := ctx.Node()
	indent := p.Indent
	if indent == "" {
		indent = "  "
	}

	var b strings.Builder
	for range ctx.Depth() {
		b.WriteString(indent)
	}
	b.WriteString(n.AQLPath())
	b.WriteByte('\t')
	if rm := n.RMTypeName(); rm != "" {
		b.WriteString(rm)
	}
	if id := n.NodeID(); id != "" {
		b.WriteByte('[')
		b.WriteString(id)
		b.WriteByte(']')
	}
	if n.IsSlot() {
		b.WriteString(" (slot)")
	}
	// Render the parent attribute marker so visitors that hand the
	// output to humans can see whether the current node is an
	// implicit RM-injected fill point. The marker is empty for the
	// root (no parent attribute).
	if pa := ctx.ParentAttribute(); pa != nil && pa.Implicit() {
		b.WriteString(" (implicit attr)")
	}
	b.WriteByte('\n')

	return p.write(b.String())
}

// PostHandle is a no-op; the printer emits one line per node in
// pre-order. Implements [walk.Visitor].
func (p *Printer) PostHandle(*walk.Context) error { return nil }

// String returns the accumulated output. When Writer was non-nil,
// the buffer is empty (output went elsewhere).
func (p *Printer) String() string { return p.buf.String() }

func (p *Printer) write(s string) error {
	if p.Writer != nil {
		_, err := io.WriteString(p.Writer, s)
		return err
	}
	p.buf.WriteString(s)
	return nil
}

// PathCollector is a [walk.Visitor] that accumulates every visited
// node's AQL path into a []string in pre-order. Use as:
//
//	pc := &templatedump.PathCollector{}
//	walk.Walk(c, pc)
//	for _, p := range pc.Paths { ... }
//
// The zero value is usable.
type PathCollector struct {
	// Paths accumulates one entry per visited node, in pre-order.
	Paths []string
}

// PreHandle appends ctx.Path() to c.Paths. Implements [walk.Visitor].
func (c *PathCollector) PreHandle(ctx *walk.Context) error {
	c.Paths = append(c.Paths, ctx.Path())
	return nil
}

// PostHandle is a no-op. Implements [walk.Visitor].
func (c *PathCollector) PostHandle(*walk.Context) error { return nil }

// Dump is a convenience that runs a [Printer] over c and returns
// the formatted output. Equivalent to constructing a Printer with
// the supplied indent (defaults to two spaces when empty) and
// calling [walk.Walk] / accumulating [Printer.String].
func Dump(c *templatecompile.Compiled, indent string) (string, error) {
	p := &Printer{Indent: indent}
	if err := walk.Walk(c, p); err != nil {
		return "", fmt.Errorf("templatedump.Dump: %w", err)
	}
	return p.String(), nil
}

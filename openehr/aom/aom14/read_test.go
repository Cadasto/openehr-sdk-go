package aom14_test

// REQ-108 (clinical-modeling.md § REQ-108, Operational template model reader):
// aom14.ReadOperationalTemplate refuses an input larger than the size limit,
// reading at most one byte past it, and an element nested deeper than the
// element limit, during the decode through canxml.WithMaxDepth, and inside
// content a reading leaves out, each with an error wrapping
// canxml.ErrLimitExceeded. REQ-176 (clinical-modeling.md § REQ-176): it reads
// every corpus operational template into the model as canxml.Unmarshal does,
// a byte-order-marked one among them, and does not match the root element by
// name.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	templateprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/template"
)

const (
	// readSizeLimit is the reader's documented size limit, 32 MiB.
	readSizeLimit = 32 << 20
	// readElementLimit is the reader's documented element limit.
	readElementLimit = 320
	// readSizeRefusal is the text of the size refusal.
	readSizeRefusal = "aom14: input exceeds 33554432 bytes: canxml: decode limit exceeded"
	// readTemplateHead opens a template whose identifier is "t".
	readTemplateHead = `<template xmlns="http://schemas.openehr.org/v1"><template_id><value>t</value></template_id>`
	// readTemplate is a whole template whose identifier is "t".
	readTemplate = readTemplateHead + `</template>`
)

// spaces is an endless stream of XML whitespace.
type spaces struct{}

func (spaces) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

// countingReader counts the bytes its reader gives.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// padded returns a reader of head, then XML whitespace, then tail, size bytes
// in all.
func padded(t *testing.T, head, tail string, size int64) io.Reader {
	t.Helper()
	pad := size - int64(len(head)) - int64(len(tail))
	if pad < 0 {
		t.Fatalf("padded: head and tail hold %d bytes, more than %d", len(head)+len(tail), size)
	}
	return io.MultiReader(strings.NewReader(head), io.LimitReader(spaces{}, pad), strings.NewReader(tail))
}

// nested returns a template whose deepest element stands at depth, the root
// element at depth 1, under elements the model does not read.
func nested(depth int) string {
	return `<template xmlns="http://schemas.openehr.org/v1">` +
		strings.Repeat("<x>", depth-1) + strings.Repeat("</x>", depth-1) + `</template>`
}

// TestREQ176_ReadOperationalTemplateReadsTheCorpus pins that the reader reads
// every corpus operational template into the value canxml.Unmarshal decodes
// it into, the byte-order-marked ones among them.
func TestREQ176_ReadOperationalTemplateReadsTheCorpus(t *testing.T) {
	t.Parallel()
	names, err := templateprobes.Probe111OperationalTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("the corpus holds no operational template")
	}
	bom := 0
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.HasPrefix(data, []byte("\ufeff")) {
			bom++
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := aom14.ReadOperationalTemplate(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("ReadOperationalTemplate: %v", err)
			}
			var want aom14.OperationalTemplate
			if err := canxml.Unmarshal(data, &want); err != nil {
				t.Fatalf("canxml.Unmarshal: %v", err)
			}
			gotXML, err := canxml.Marshal(got)
			if err != nil {
				t.Fatalf("canxml.Marshal(read): %v", err)
			}
			wantXML, err := canxml.Marshal(&want)
			if err != nil {
				t.Fatalf("canxml.Marshal(unmarshalled): %v", err)
			}
			if !bytes.Equal(gotXML, wantXML) {
				t.Errorf("ReadOperationalTemplate gives a value other than canxml.Unmarshal's: %d bytes encoded against %d", len(gotXML), len(wantXML))
			}
		})
	}
	if bom == 0 {
		t.Error("no corpus operational template starts with a byte-order mark; the corpus no longer covers one")
	}
}

// TestREQ176_ReadOperationalTemplateReadsAByteOrderMark pins that a document
// that starts with a UTF-8 byte-order mark reads.
func TestREQ176_ReadOperationalTemplateReadsAByteOrderMark(t *testing.T) {
	t.Parallel()
	for name, doc := range map[string]string{
		"before the root":            "\ufeff" + readTemplate,
		"before the XML declaration": "\ufeff" + `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + readTemplate,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ot, err := aom14.ReadOperationalTemplate(strings.NewReader(doc))
			if err != nil {
				t.Fatalf("ReadOperationalTemplate: %v", err)
			}
			if ot.TemplateID.Value != "t" {
				t.Errorf("TemplateID.Value = %q, want %q", ot.TemplateID.Value, "t")
			}
		})
	}
}

// TestREQ176_ReadOperationalTemplateReadsAnyRootName pins that the reader does
// not match the root element by name, so a document whose root is
// <OPERATIONAL_TEMPLATE> reads as one whose root is <template>.
func TestREQ176_ReadOperationalTemplateReadsAnyRootName(t *testing.T) {
	t.Parallel()
	doc := strings.ReplaceAll(readTemplate, "template>", "OPERATIONAL_TEMPLATE>")
	doc = strings.Replace(doc, "<template ", "<OPERATIONAL_TEMPLATE ", 1)
	ot, err := aom14.ReadOperationalTemplate(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate(%s): %v", doc, err)
	}
	if ot.TemplateID.Value != "t" {
		t.Errorf("TemplateID.Value = %q, want %q", ot.TemplateID.Value, "t")
	}
}

// TestREQ176_ReadOperationalTemplateAppliesTheCallersOptions pins that the
// caller's decode options apply: canxml.WithStrict refuses an element the
// model does not read, which the default decode skips.
func TestREQ176_ReadOperationalTemplateAppliesTheCallersOptions(t *testing.T) {
	t.Parallel()
	doc := readTemplateHead + `<x-extra/></template>`
	if _, err := aom14.ReadOperationalTemplate(strings.NewReader(doc)); err != nil {
		t.Fatalf("ReadOperationalTemplate without options: %v", err)
	}
	_, err := aom14.ReadOperationalTemplate(strings.NewReader(doc), canxml.WithStrict())
	if !errors.Is(err, canxml.ErrUnknownElement) {
		t.Errorf("ReadOperationalTemplate(WithStrict) = %v, want an error wrapping canxml.ErrUnknownElement", err)
	}
}

// TestREQ176_REQ111_ReadOperationalTemplateHoldsContentAfterTheRoot pins that
// the reader holds the input to one document. After the root, in every mode,
// comments, processing instructions and whitespace pass, and an element,
// text, a CDATA section or a stray end element is refused with an error
// wrapping canxml.ErrInvalidShape that carries none of it. A `<!…>`
// declaration, and a processing instruction whose target is xml in any
// letter case, which XML reserves for the XML declaration at the start, are
// read past in the default mode and refused so given canxml.WithStrict. A
// target that only starts with xml passes in every mode.
func TestREQ176_REQ111_ReadOperationalTemplateHoldsContentAfterTheRoot(t *testing.T) {
	t.Parallel()
	const planted = "PLANTED"
	refused := map[string]string{
		"a second root":         `<template><template_id><value>` + planted + `</value></template_id></template>`,
		"an empty element":      `<` + planted + `/>`,
		"text":                  planted,
		"a stray end element":   `</` + planted + `>`,
		"a CDATA section":       `<![CDATA[` + planted + `]]>`,
		"markup it cannot read": `<1>`,
	}
	defaultOnly := map[string]string{
		"a document type":           `<!DOCTYPE ` + planted + `>`,
		"another declaration":       `<!ENTITY ` + planted + ` "x">`,
		"an XML declaration":        `<?xml version="1.0" extra="` + planted + `"?>`,
		"an xml target in capitals": `<?XML ` + planted + `?>`,
	}
	accepted := map[string]string{
		"nothing":                       "",
		"whitespace":                    " \n\t\r\n",
		"a comment":                     `<!-- note -->`,
		"a processing instruction":      `<?pi data?>`,
		"a target that starts with xml": `<?xml-stylesheet href="a.xsl"?>`,
	}
	for mode, opts := range readModes {
		for name, tail := range refused {
			t.Run(mode+"/refused/"+name, func(t *testing.T) {
				t.Parallel()
				refusesContentOutsideTheRoot(t, readTemplate+tail, planted, opts...)
			})
		}
		for name, tail := range defaultOnly {
			t.Run(mode+"/default mode only/"+name, func(t *testing.T) {
				t.Parallel()
				if mode == "strict" {
					refusesContentOutsideTheRoot(t, readTemplate+tail, planted, opts...)
					return
				}
				readsTemplateT(t, readTemplate+tail, opts...)
			})
		}
		for name, tail := range accepted {
			t.Run(mode+"/accepted/"+name, func(t *testing.T) {
				t.Parallel()
				readsTemplateT(t, readTemplate+tail, opts...)
			})
		}
	}
}

// TestREQ176_REQ056_ReadOperationalTemplateRefusesAnXMLTargetInsideTheRoot
// pins that a processing instruction whose target is xml, in any letter case,
// inside the root element is refused in both modes, as a canonical-XML decode
// given any option refuses it, with an error wrapping canxml.ErrInvalidShape
// that carries none of it; a target that only starts with xml passes. The
// default mode reads past one before or after the root alone.
func TestREQ176_REQ056_ReadOperationalTemplateRefusesAnXMLTargetInsideTheRoot(t *testing.T) {
	t.Parallel()
	const planted = "PLANTED"
	refused := map[string]string{
		"an XML declaration":        `<?xml version="1.0" extra="` + planted + `"?>`,
		"an xml target in capitals": `<?XML ` + planted + `?>`,
	}
	for mode, opts := range readModes {
		for name, pi := range refused {
			t.Run(mode+"/refused/"+name, func(t *testing.T) {
				t.Parallel()
				refusesContentOutsideTheRoot(t, readTemplateHead+pi+`</template>`, planted, opts...)
			})
		}
		t.Run(mode+"/accepted/a target that starts with xml", func(t *testing.T) {
			t.Parallel()
			readsTemplateT(t, readTemplateHead+`<?xml-stylesheet href="a.xsl"?></template>`, opts...)
		})
	}
}

// TestREQ176_REQ111_ReadOperationalTemplateHoldsContentBeforeTheRoot pins
// what the reader reads before the root element. In every mode the XML
// declaration, comments, processing instructions, whitespace and one document
// type declaration pass, and a stray end element, or markup encoding/xml
// cannot read, is refused with an error wrapping canxml.ErrInvalidShape that
// carries none of it. Text, a CDATA section, a second document type, another
// declaration, and a processing instruction whose target is xml in any letter
// case other than the XML declaration at the very start, one after
// whitespace included, which encoding/xml reads,
// are read past in the default mode and refused so given canxml.WithStrict.
// A target that only starts with xml passes in every mode. An input with only
// such content and no root is the no-root case
// (TestREQ108_ReadOperationalTemplateRefusesAnInputWithNoRootElement).
func TestREQ176_REQ111_ReadOperationalTemplateHoldsContentBeforeTheRoot(t *testing.T) {
	t.Parallel()
	const planted = "PLANTED"
	refused := map[string]string{
		"a stray end element":   `</` + planted + `>`,
		"text, then bad markup": planted + `<1>`,
	}
	defaultOnly := map[string]string{
		"text":                                 planted,
		"text after a declaration":             `<?xml version="1.0"?>` + planted,
		"a CDATA section":                      `<![CDATA[` + planted + `]]>`,
		"a second document type":               `<!DOCTYPE template><!DOCTYPE ` + planted + `>`,
		"another declaration":                  `<!ENTITY ` + planted + ` "x">`,
		"a second XML declaration":             `<?xml version="1.0"?><?xml version="1.0" extra="` + planted + `"?>`,
		"a misplaced XML declaration":          `<!-- note --><?xml version="1.0"?>`,
		"an XML declaration after a space":     " " + `<?xml version="1.0"?>`,
		"an XML declaration after a line feed": "\n" + `<?xml version="1.0"?>`,
		"an xml target in capitals":            `<?XML ` + planted + `?>`,
		"an xml target in the subset":          `<!DOCTYPE template [<?xml ` + planted + `?>]>`,
	}
	accepted := map[string]string{
		"nothing":                       "",
		"whitespace":                    " \n\t\r\n",
		"a target that starts with xml": `<?xml version="1.0"?><?xml-stylesheet href="a.xsl"?>`,
		"the XML declaration":           `<?xml version="1.0" encoding="UTF-8"?>` + "\n",
		"a comment":                     `<!-- note -->`,
		"a processing instruction":      `<?pi data?>`,
		"one document type":             `<!DOCTYPE template>`,
		"all of them":                   `<?xml version="1.0"?><!-- note --><!DOCTYPE template><?pi data?>` + "\n",
	}
	for mode, opts := range readModes {
		for name, head := range refused {
			t.Run(mode+"/refused/"+name, func(t *testing.T) {
				t.Parallel()
				refusesContentOutsideTheRoot(t, head+readTemplate, planted, opts...)
			})
		}
		for name, head := range defaultOnly {
			t.Run(mode+"/default mode only/"+name, func(t *testing.T) {
				t.Parallel()
				for _, bom := range []string{"", "\ufeff"} {
					if mode == "strict" {
						refusesContentOutsideTheRoot(t, bom+head+readTemplate, planted, opts...)
						continue
					}
					readsTemplateT(t, bom+head+readTemplate, opts...)
				}
			})
		}
		for name, head := range accepted {
			t.Run(mode+"/accepted/"+name, func(t *testing.T) {
				t.Parallel()
				for _, bom := range []string{"", "\ufeff"} {
					readsTemplateT(t, bom+head+readTemplate, opts...)
				}
			})
		}
	}
}

// readModes are the reader's two modes.
var readModes = map[string][]canxml.DecoderOption{
	"default": nil,
	"strict":  {canxml.WithStrict()},
}

// refusesContentOutsideTheRoot fails the test unless the reader refuses doc
// with opts with an error that wraps canxml.ErrInvalidShape and
// canxml.ErrNotWellFormed and does not carry planted.
func refusesContentOutsideTheRoot(t *testing.T, doc, planted string, opts ...canxml.DecoderOption) {
	t.Helper()
	_, err := aom14.ReadOperationalTemplate(strings.NewReader(doc), opts...)
	if !errors.Is(err, canxml.ErrInvalidShape) || !errors.Is(err, canxml.ErrNotWellFormed) {
		t.Fatalf("ReadOperationalTemplate(%q) = %v, want an error wrapping canxml.ErrInvalidShape and canxml.ErrNotWellFormed", doc, err)
	}
	if strings.Contains(err.Error(), planted) {
		t.Errorf("error %q carries the content outside the root", err)
	}
}

// readsTemplateT fails the test unless the reader reads doc with opts into
// the template whose identifier is "t".
func readsTemplateT(t *testing.T, doc string, opts ...canxml.DecoderOption) {
	t.Helper()
	ot, err := aom14.ReadOperationalTemplate(strings.NewReader(doc), opts...)
	if err != nil {
		t.Fatalf("ReadOperationalTemplate(%q): %v", doc, err)
	}
	if ot.TemplateID.Value != "t" {
		t.Errorf("ReadOperationalTemplate(%q): TemplateID.Value = %q, want %q", doc, ot.TemplateID.Value, "t")
	}
}

// TestREQ108_ReadOperationalTemplateRefusesAnInputPastTheSizeLimit pins the
// size limit at 32 MiB: an input of that size reads and a larger one is
// refused with a plain error wrapping canxml.ErrLimitExceeded, whether the
// root element ends past the limit or before it, and the reader reads at
// most one byte past the limit. Each refused input is a template the decode
// reads whole without the limit. The cases run one at a time, since each
// decode holds up to 32 MiB of whitespace as one run of text.
func TestREQ108_ReadOperationalTemplateRefusesAnInputPastTheSizeLimit(t *testing.T) {
	cases := []struct {
		name       string
		head, tail string
		size       int64
		refused    bool
	}{
		{name: "root that ends at the limit", head: readTemplateHead, tail: "</template>", size: readSizeLimit},
		{name: "root that ends one byte past the limit", head: readTemplateHead, tail: "</template>", size: readSizeLimit + 1, refused: true},
		{name: "root that ends far past the limit", head: readTemplateHead, tail: "</template>", size: readSizeLimit + 1<<20, refused: true},
		{name: "whitespace after the root up to the limit", head: readTemplate, size: readSizeLimit},
		{name: "whitespace after the root one byte past the limit", head: readTemplate, size: readSizeLimit + 1, refused: true},
		{name: "whitespace after the root far past the limit", head: readTemplate, size: readSizeLimit + 1<<20, refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := &countingReader{r: padded(t, tc.head, tc.tail, tc.size)}
			ot, err := aom14.ReadOperationalTemplate(in)
			if in.n > readSizeLimit+1 {
				t.Errorf("read %d bytes, more than one past the limit of %d", in.n, readSizeLimit)
			}
			if !tc.refused {
				if err != nil {
					t.Fatalf("ReadOperationalTemplate(%d bytes): %v", tc.size, err)
				}
				if ot.TemplateID.Value != "t" {
					t.Errorf("TemplateID.Value = %q, want %q", ot.TemplateID.Value, "t")
				}
				return
			}
			if !errors.Is(err, canxml.ErrLimitExceeded) {
				t.Fatalf("ReadOperationalTemplate(%d bytes) = %v, want an error wrapping canxml.ErrLimitExceeded", tc.size, err)
			}
			if de, ok := errors.AsType[*canxml.DecodeError](err); ok {
				t.Errorf("size refusal is a *canxml.DecodeError (%v), want a plain error", de)
			}
			if err.Error() != readSizeRefusal {
				t.Errorf("size refusal reads %q, want %q", err, readSizeRefusal)
			}
		})
	}
}

// TestREQ108_ReadOperationalTemplateRefusesAnElementPastTheElementLimit pins
// the element limit at 320, the root at depth 1: an element at that depth
// reads, and one deeper is refused with the *canxml.DecodeError
// canxml.WithMaxDepth gives, wrapping canxml.ErrLimitExceeded and naming the
// depth. A caller's own canxml.WithMaxDepth does not raise the limit.
func TestREQ108_ReadOperationalTemplateRefusesAnElementPastTheElementLimit(t *testing.T) {
	t.Parallel()
	callers := map[string][]canxml.DecoderOption{
		"no option":               nil,
		"a larger limit":          {canxml.WithMaxDepth(10 * readElementLimit)},
		"no limit":                {canxml.WithMaxDepth(0)},
		"WithUnmatched":           {canxml.WithUnmatched(func(canxml.Path) {})},
		"WithMissing":             {canxml.WithMissing(func(canxml.Pointer) {})},
		"WithUnmatched, no limit": {canxml.WithUnmatched(func(canxml.Path) {}), canxml.WithMaxDepth(0)},
	}
	for name, opts := range callers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := aom14.ReadOperationalTemplate(strings.NewReader(nested(readElementLimit)), opts...); err != nil {
				t.Errorf("ReadOperationalTemplate(depth %d): %v", readElementLimit, err)
			}
			_, err := aom14.ReadOperationalTemplate(strings.NewReader(nested(readElementLimit+1)), opts...)
			if !errors.Is(err, canxml.ErrLimitExceeded) {
				t.Fatalf("ReadOperationalTemplate(depth %d) = %v, want an error wrapping canxml.ErrLimitExceeded", readElementLimit+1, err)
			}
			if _, ok := errors.AsType[*canxml.DecodeError](err); !ok {
				t.Errorf("depth refusal %v is not a *canxml.DecodeError", err)
			}
			if want := "element depth (321 > 320)"; !strings.Contains(err.Error(), want) {
				t.Errorf("depth refusal reads %q, want it to name %q", err, want)
			}
		})
	}
}

// TestREQ108_ReadOperationalTemplateAppliesTheCallersSmallerLimit pins that a
// caller's own canxml.WithMaxDepth applies as well as the element limit and
// the smaller holds, whatever the order of the caller's options: a document
// nested 200 deep, which the reader's limit admits, is refused under a caller's
// limit of 100 at the first element past it, and one nested 100 deep reads. An
// n <= 0 among the caller's options lifts neither limit.
//
// Can-fail control: let the last canxml.WithMaxDepth given replace the others,
// and the reader's own limit replaces the caller's, so the 200-deep document
// reads.
func TestREQ108_ReadOperationalTemplateAppliesTheCallersSmallerLimit(t *testing.T) {
	t.Parallel()
	const limit = 100
	callers := map[string][]canxml.DecoderOption{
		"the limit":                      {canxml.WithMaxDepth(limit)},
		"the limit, then a larger one":   {canxml.WithMaxDepth(limit), canxml.WithMaxDepth(2 * limit)},
		"a larger limit, then the limit": {canxml.WithMaxDepth(2 * limit), canxml.WithMaxDepth(limit)},
		"the limit, then no limit":       {canxml.WithMaxDepth(limit), canxml.WithMaxDepth(0)},
		"no limit, then the limit":       {canxml.WithMaxDepth(0), canxml.WithMaxDepth(limit)},
		"the limit with WithUnmatched":   {canxml.WithMaxDepth(limit), canxml.WithUnmatched(func(canxml.Path) {})},
	}
	for name, opts := range callers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := aom14.ReadOperationalTemplate(strings.NewReader(nested(limit)), opts...); err != nil {
				t.Errorf("ReadOperationalTemplate(depth %d) = %v, want a read within the caller's limit", limit, err)
			}
			_, err := aom14.ReadOperationalTemplate(strings.NewReader(nested(2*limit)), opts...)
			if !errors.Is(err, canxml.ErrLimitExceeded) {
				t.Fatalf("ReadOperationalTemplate(depth %d) = %v, want an error wrapping canxml.ErrLimitExceeded", 2*limit, err)
			}
			if want := "element depth (101 > 100)"; !strings.Contains(err.Error(), want) {
				t.Errorf("depth refusal reads %q, want it to name %q", err, want)
			}
		})
	}
}

// TestREQ108_ReadOperationalTemplateRefusesAnInputWithNoRootElement pins that
// an input that holds no root element, whether empty or holding only an XML
// declaration, a byte-order mark, whitespace, a comment or text, is refused
// with an error wrapping io.ErrUnexpectedEOF, not a bare io.EOF, that says no
// operational template was in the input and carries none of it. A document cut
// short after its root begins keeps the decode's own error.
//
// Can-fail control: return the decode's error as it comes and the empty input
// gives a bare io.EOF.
func TestREQ108_ReadOperationalTemplateRefusesAnInputWithNoRootElement(t *testing.T) {
	t.Parallel()
	const planted = "PLANTED"
	const want = "aom14: no operational template in the input"
	for name, in := range map[string]string{
		"nothing":                  "",
		"whitespace":               " \n\t\r\n",
		"a byte-order mark":        "\ufeff",
		"an XML declaration":       `<?xml version="1.0" encoding="UTF-8"?>`,
		"a comment":                `<!-- ` + planted + ` -->`,
		"text":                     planted,
		"a declaration, a comment": `<?xml version="1.0"?>` + "\n" + `<!-- ` + planted + ` -->`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ot, err := aom14.ReadOperationalTemplate(strings.NewReader(in))
			if ot != nil {
				t.Errorf("ReadOperationalTemplate(%s) = %v, want nil", name, ot)
			}
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("ReadOperationalTemplate(%s) = %v, want an error wrapping io.ErrUnexpectedEOF", name, err)
			}
			if errors.Is(err, io.EOF) {
				t.Errorf("ReadOperationalTemplate(%s) = %v, want an error that is not io.EOF", name, err)
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q, want it to say %q", err, want)
			}
			if strings.Contains(err.Error(), planted) {
				t.Errorf("error %q carries the input", err)
			}
		})
	}
	_, err := aom14.ReadOperationalTemplate(strings.NewReader(`<template xmlns="http://schemas.openehr.org/v1"><template_id>`))
	if err == nil || strings.Contains(err.Error(), want) {
		t.Errorf("ReadOperationalTemplate(cut short) = %v, want the decode's own error", err)
	}
}

// errSource is the failure of a source of input.
var errSource = errors.New("connection reset")

// failingReader returns a reader of the first n bytes of doc that then fails
// with errSource.
func failingReader(doc string, n int) io.Reader {
	return io.MultiReader(strings.NewReader(doc[:n]), iotest.ErrReader(errSource))
}

// TestREQ108_ReadOperationalTemplateReturnsTheSourcesError pins that a
// failure of the caller's reader comes back as the error the reader
// returned, whatever the options, where the decode meets it and where the
// reader reads the rest of an input it reads a second time: never as the
// syntax error of a decode of the bytes read before it, nor as the failure
// of the first decode. Strict mode refuses an <attributes> with no type
// before it reads on, so it returns that refusal.
//
// Can-fail control: decode the bytes read before the failure a second time,
// as a reading that reports to the caller's function does, or read the rest
// of the input without keeping the failure, and the error is the decode's.
func TestREQ108_ReadOperationalTemplateReturnsTheSourcesError(t *testing.T) {
	t.Parallel()
	// The first decode fails on the <attributes> with no type, which the
	// default mode reads a second time once it has read the rest.
	untyped := `<template xmlns="http://schemas.openehr.org/v1"><template_id><value>t</value></template_id>` +
		`<definition><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id><attributes>` +
		`<rm_attribute_name>items</rm_attribute_name><existence><lower>0</lower><upper>1</upper></existence></attributes>` +
		`<archetype_id><value>openEHR-EHR-CLUSTER.t.v0</value></archetype_id></definition></template>`
	sources := map[string]struct {
		open func() io.Reader
		// strict is what strict mode returns.
		strict error
	}{
		"failing after 60 bytes": {
			open:   func() io.Reader { return failingReader(readTemplate, 60) },
			strict: errSource,
		},
		"failing after a shape the default mode reads a second time": {
			open:   func() io.Reader { return failingReader(untyped, strings.Index(untyped, "<rm_attribute_name>")) },
			strict: typereg.ErrMissingType,
		},
	}
	options := []struct {
		name   string
		opts   []canxml.DecoderOption
		strict bool
	}{
		{name: "no option"},
		{name: "WithUnmatched", opts: []canxml.DecoderOption{canxml.WithUnmatched(func(canxml.Path) {})}},
		{name: "WithMissing", opts: []canxml.DecoderOption{canxml.WithMissing(func(canxml.Pointer) {})}},
		{name: "WithStrict", opts: []canxml.DecoderOption{canxml.WithStrict()}, strict: true},
		{name: "WithStrict and WithUnmatched", opts: []canxml.DecoderOption{canxml.WithStrict(), canxml.WithUnmatched(func(canxml.Path) {})}, strict: true},
	}
	for name, src := range sources {
		for _, o := range options {
			t.Run(name+"/"+o.name, func(t *testing.T) {
				t.Parallel()
				want := errSource
				if o.strict {
					want = src.strict
				}
				ot, err := aom14.ReadOperationalTemplate(src.open(), o.opts...)
				if ot != nil || !errors.Is(err, want) {
					t.Errorf("ReadOperationalTemplate = %v, %v; want nil and an error wrapping %v", ot, err, want)
				}
			})
		}
	}
}

// TestREQ108_ReadOperationalTemplateRefusesADeepElementDuringTheDecode pins
// that the reader refuses an element past the element limit when it reads
// the element's start tag, not once it has read the input: it stops reading
// a few kilobytes past it, far short of the input's end.
func TestREQ108_ReadOperationalTemplateRefusesADeepElementDuringTheDecode(t *testing.T) {
	t.Parallel()
	head := `<template xmlns="http://schemas.openehr.org/v1">` + strings.Repeat("<x>", readElementLimit)
	tail := strings.Repeat("</x>", readElementLimit) + `</template>`
	const size = 16 << 20
	in := &countingReader{r: padded(t, head, tail, size)}
	_, err := aom14.ReadOperationalTemplate(in)
	if !errors.Is(err, canxml.ErrLimitExceeded) {
		t.Fatalf("ReadOperationalTemplate(depth %d) = %v, want an error wrapping canxml.ErrLimitExceeded", readElementLimit+1, err)
	}
	if limit := int64(len(head)) + 64<<10; in.n > limit {
		t.Errorf("read %d bytes of %d before the refusal, want at most %d", in.n, size, limit)
	}
}

// leftOut returns a template whose deepest element stands at depth, the root
// element at depth 1, inside the <item> of a C_PRIMITIVE_OBJECT, at depth 5,
// whose type names no class, which the default mode leaves out with all it
// holds. The item carries planted.
func leftOut(depth int, planted string) string {
	const itemDepth = 5
	n := depth - itemDepth
	return itemDoc(`<item xsi:type="C_FUTURE" note="` + planted + `">` + strings.Repeat("<x>", n) + strings.Repeat("</x>", n) + `</item>`)
}

// TestREQ108_ReadOperationalTemplateRefusesADeepElementAReadingLeavesOut pins
// that the element limit, and a caller's smaller limit, hold inside content a
// reading of the default mode leaves out, which no decode reads: an element
// nested past the limit there is refused with the *canxml.DecodeError
// canxml.WithMaxDepth gives, wrapping canxml.ErrLimitExceeded, naming the
// depth and the element's path and none of the input. A left-out item whose
// deepest element stands at the limit reads.
//
// Can-fail control: let the reading leave the item out without following its
// elements, and the deep item reads.
func TestREQ108_ReadOperationalTemplateRefusesADeepElementAReadingLeavesOut(t *testing.T) {
	t.Parallel()
	const planted = "PLANTED"
	// The path of the first element past the limit: eight segments at each
	// end, the item and its parents first, and its elements past them.
	const wantPath = "/template[1]/definition[1]/attributes[1]/children[1]/item[1]/x[1]/x[1]/x[1]/..." +
		"/x[1]/x[1]/x[1]/x[1]/x[1]/x[1]/x[1]/x[1]"
	cases := []struct {
		name  string
		opts  []canxml.DecoderOption
		limit int
	}{
		{name: "the element limit", limit: readElementLimit},
		{name: "the element limit, with WithUnmatched", opts: []canxml.DecoderOption{canxml.WithUnmatched(func(canxml.Path) {})}, limit: readElementLimit},
		{name: "a larger limit of the caller's", opts: []canxml.DecoderOption{canxml.WithMaxDepth(2 * readElementLimit)}, limit: readElementLimit},
		{name: "a caller's limit of 50", opts: []canxml.DecoderOption{canxml.WithMaxDepth(50)}, limit: 50},
		{name: "a caller's limit of 50, with WithUnmatched", opts: []canxml.DecoderOption{canxml.WithMaxDepth(50), canxml.WithUnmatched(func(canxml.Path) {})}, limit: 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			within := leftOut(tc.limit, planted)
			if depth := maxDepth(t, within); depth != tc.limit {
				t.Fatalf("the document nests %d elements, want %d", depth, tc.limit)
			}
			if _, err := aom14.ReadOperationalTemplate(strings.NewReader(within), tc.opts...); err != nil {
				t.Errorf("ReadOperationalTemplate(a left-out item %d deep): %v", tc.limit, err)
			}
			for _, depth := range []int{tc.limit + 1, 400} {
				_, err := aom14.ReadOperationalTemplate(strings.NewReader(leftOut(depth, planted)), tc.opts...)
				if !errors.Is(err, canxml.ErrLimitExceeded) {
					t.Fatalf("ReadOperationalTemplate(a left-out item %d deep) = %v, want an error wrapping canxml.ErrLimitExceeded", depth, err)
				}
				de, ok := errors.AsType[*canxml.DecodeError](err)
				if !ok || de == nil {
					t.Fatalf("depth refusal %v is not a *canxml.DecodeError", err)
				}
				if de.Path != wantPath {
					t.Errorf("depth refusal names %q, want %q", de.Path, wantPath)
				}
				if want := fmt.Sprintf("element depth (%d > %d)", tc.limit+1, tc.limit); !strings.Contains(err.Error(), want) {
					t.Errorf("depth refusal reads %q, want it to name %q", err, want)
				}
				if strings.Contains(err.Error(), planted) {
					t.Errorf("depth refusal %q carries the input", err)
				}
			}
		})
	}
}

// TestREQ108_ReadOperationalTemplateReadsNoFurtherThanTheElementLimit pins
// that the reader stops at the first element past the element limit inside
// content a reading leaves out, as the decode does elsewhere, rather than
// following each element the content holds: refusing an item nested 100 000
// deep costs about as many allocations as refusing one nested just past the
// limit. It runs alone, since it counts the allocations of the process.
//
// Can-fail control: let the reading note the first element past the limit and
// read on, refusing the input at its end, and the deeper item costs a few
// allocations for each of its elements.
func TestREQ108_ReadOperationalTemplateReadsNoFurtherThanTheElementLimit(t *testing.T) {
	allocs := func(depth int) float64 {
		doc := leftOut(depth, "")
		return testing.AllocsPerRun(2, func() {
			if _, err := aom14.ReadOperationalTemplate(strings.NewReader(doc)); !errors.Is(err, canxml.ErrLimitExceeded) {
				t.Fatalf("ReadOperationalTemplate(a left-out item %d deep) = %v, want an error wrapping canxml.ErrLimitExceeded", depth, err)
			}
		})
	}
	const slack = 1000
	shallow, deep := allocs(readElementLimit+1), allocs(100_000)
	if deep > shallow+slack {
		t.Errorf("refusing a left-out item 100 000 deep costs %.0f allocations, one %d deep %.0f; want at most %d more", deep, readElementLimit+1, shallow, slack)
	}
}

// readCost returns the least time ReadOperationalTemplate takes to read doc
// with opts over three runs, whatever it returns.
func readCost(doc string, opts ...canxml.DecoderOption) time.Duration {
	best := time.Duration(math.MaxInt64)
	for range 3 {
		start := time.Now()
		_, _ = aom14.ReadOperationalTemplate(strings.NewReader(doc), opts...)
		best = min(best, time.Since(start))
	}
	return best
}

// costCase is a document whose reading would cost the square of its size
// were the reader to scan back, or through what it has done, once per match,
// and a document of about its size whose reading does neither.
type costCase struct {
	name          string
	doc, baseline string
	modes         []string
}

// holdsLinearCost fails the test unless the reader reads each case's document,
// in each of its modes, in at most ten times the time it takes to read its
// baseline. A reader that scans back to the document's start once per match,
// or through every edit it has made once per element, takes hundreds of times
// as long on these sizes. The times are the least of three runs each, taken
// one after the other, so that a load on the machine weighs on both alike.
func holdsLinearCost(t *testing.T, cases []costCase) {
	t.Helper()
	const factor = 10
	for _, tc := range cases {
		for _, mode := range tc.modes {
			opts := readModes[mode]
			got, base := readCost(tc.doc, opts...), readCost(tc.baseline, opts...)
			if got > factor*base {
				t.Errorf("%s, %s mode: reading %d bytes takes %v, more than %d times the %v a baseline of %d bytes takes",
					tc.name, mode, len(tc.doc), got, factor, base, len(tc.baseline))
			}
		}
	}
}

// TestREQ108_ReadOperationalTemplateChecksInLinearTime pins that the checks
// that decide whether a document needs a second reading cost time in
// proportion to its size, each match costing about as much as the bytes
// around it: an element-holding text of 50 000 slashes before a `>`, 20 000
// names of archetype_id under a prefix in text, a start tag that holds
// C_BOOLEAN 20 000 times, 50 000 character references in text, after a long
// start tag or before a C_BOOLEAN item, and 50 000 end tags after a comment
// each read in about the time a document of that size without them takes. It
// runs alone, since it times the reads.
//
// Can-fail control: find the start tag a match stands in by scanning back to
// the last `<`, or to the end of the tag it opens, once per match, look for
// the next C_BOOLEAN again after each character reference, or read the name
// of an end tag past its `>`, and each document takes hundreds of times as
// long as its baseline.
func TestREQ108_ReadOperationalTemplateChecksInLinearTime(t *testing.T) {
	inConcept := func(content string) string {
		return readTemplateHead + `<concept>` + content + `</concept></template>`
	}
	// A C_BOOLEAN item that holds both its flags, so that the document needs no
	// second reading for the item.
	booleanItem := `<definition><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id>` +
		`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` +
		`<existence><lower>0</lower><upper>1</upper></existence><children xsi:type="C_PRIMITIVE_OBJECT">` +
		`<rm_type_name>BOOLEAN</rm_type_name><node_id/><item xsi:type="C_BOOLEAN">` +
		`<true_valid>true</true_valid><false_valid>true</false_valid></item></children></attributes></definition>`
	// A template whose concept holds text and whose definition follows it.
	inConceptBefore := func(text, definition string) string {
		return `<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
			`<template_id><value>t</value></template_id><concept>` + text + `</concept>` + definition + `</template>`
	}
	// The root's name starts the name of none of its elements.
	inRootTag := func(words string) string {
		return `<OPERATIONAL_TEMPLATE xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" ` +
			`xsi:schemaLocation="` + words + `"><template_id><value>t</value></template_id>` + booleanItem + `</OPERATIONAL_TEMPLATE>`
	}
	holdsLinearCost(t, []costCase{
		{
			name:     "slashes before a > in an element's text",
			doc:      inConcept(strings.Repeat("/>", 50_000)),
			baseline: inConcept(strings.Repeat("a>", 50_000)),
			modes:    []string{"default", "strict"},
		},
		{
			name:     "archetype_id under a prefix in text",
			doc:      inConcept(strings.Repeat(":archetype_id ", 20_000)),
			baseline: inConcept(strings.Repeat("-archetype_id ", 20_000)),
			modes:    []string{"default"},
		},
		{
			name:     "C_BOOLEAN in one start tag",
			doc:      inRootTag(strings.Repeat("C_BOOLEAN ", 20_000)),
			baseline: inRootTag(strings.Repeat("X_BOOLEAN ", 20_000)),
			modes:    []string{"default", "strict"},
		},
		{
			name:     "character references in text after a long start tag",
			doc:      readTemplateHead + `<concept note="` + strings.Repeat("x", 100_000) + `">` + strings.Repeat("&#97;", 50_000) + `</concept></template>`,
			baseline: readTemplateHead + `<concept note="` + strings.Repeat("x", 100_000) + `">` + strings.Repeat("aaaaa", 50_000) + `</concept></template>`,
			modes:    []string{"default"},
		},
		{
			name:     "character references in text before a C_BOOLEAN",
			doc:      inConceptBefore(strings.Repeat("&#97;", 50_000), booleanItem),
			baseline: inConceptBefore(strings.Repeat("aaaaa", 50_000), booleanItem),
			modes:    []string{"default", "strict"},
		},
		{
			name:     "end tags after a comment",
			doc:      readTemplateHead + strings.Repeat("<x><!--c--></x>", 50_000) + `</template>`,
			baseline: readTemplateHead + strings.Repeat("<x><!--c-->.</x>", 50_000) + `</template>`,
			modes:    []string{"default"},
		},
	})
}

// TestREQ108_ReadOperationalTemplateRewritesInLinearTime pins that the second
// reading rewrites a document in time in proportion to its size, however
// many elements it rewrites and however many namespaces it declares: 50 000
// empty list entries of a C_INTEGER, 20 000 C_BOOLEAN items with an empty
// true_valid and false_valid, 20 000 <children> typed with a primitive class,
// 40 000 node ids in the XSI namespace, each written without its prefix,
// 20 000 <children> with no type under 20 000 namespace declarations, 2 000
// quantities whose assumed value the model cannot hold under 2 000
// declarations, an item typed C_PRIMITIVE_OBJECT that declares 4 000
// namespaces and holds 4 000 elements, and 40 000 names a fresh prefix could
// take in text each read in about the time a document of that size takes
// that needs no rewrite, or that needs one but holds none of these. It runs
// alone, since it times the reads.
//
// Can-fail control: let each rewritten element, or each element written
// without its prefix, forget the edits made inside it by scanning every edit
// made so far, render its content by scanning them all once for each member,
// look a prefix up by scanning back through every declaration in scope, bind
// every prefix in scope around each assumed value it checks, copy every
// declaration of an item it takes off to each element the item holds, or try
// one fresh prefix after another against the whole input, and the documents
// take tens of times as long as their baselines.
func TestREQ108_ReadOperationalTemplateRewritesInLinearTime(t *testing.T) {
	declarations := func(n int) string {
		var b strings.Builder
		for i := range n {
			fmt.Fprintf(&b, ` xmlns:p%d="urn:x-openehr-sdk-go:test:p%d"`, i, i)
		}
		return b.String()
	}
	headWith := func(rootAttrs string) string {
		return `<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` + rootAttrs + `>` +
			`<template_id><value>t</value></template_id><definition><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id>` +
			`<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
			`<existence><lower>0</lower><upper>1</upper></existence>` +
			`<cardinality><is_ordered>true</is_ordered><is_unique>false</is_unique><interval><lower>0</lower><upper_unbounded>true</upper_unbounded></interval></cardinality>`
	}
	head := headWith("")
	const tail = `</attributes></definition></template>`
	integer := func(entries string) string {
		return head + `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>INTEGER</rm_type_name><node_id/>` +
			`<item xsi:type="C_INTEGER">` + entries + `</item></children>` + tail
	}
	boolean := func(flags string) string {
		return `<children xsi:type="C_PRIMITIVE_OBJECT"><item xsi:type="C_BOOLEAN">` + flags + `</item></children>`
	}
	leaf := func(class string) string {
		return `<children xsi:type="` + class + `"><node_id/></children>`
	}
	quantity := func(magnitude string) string {
		return `<children xsi:type="C_DV_QUANTITY"><rm_type_name>DV_QUANTITY</rm_type_name><node_id/>` +
			`<assumed_value><magnitude>` + magnitude + `</magnitude><units>kg</units></assumed_value></children>`
	}
	item := func(class string, n int) string {
		return head + `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name><node_id/>` +
			`<item xsi:type="` + class + `"` + declarations(n) + `>` + strings.Repeat(`<x/>`, n) +
			`<item xsi:type="C_STRING"><list>a</list></item></item></children>` + tail
	}
	// A template with no namespace declaration of the XSI one, every slot of
	// which the second reading types, whose concept holds n names.
	untypedWith := func(name string, n int) string {
		var b strings.Builder
		for i := range n {
			fmt.Fprintf(&b, "%s%d: ", name, i)
		}
		return `<template xmlns="http://schemas.openehr.org/v1"><concept>` + b.String() + `</concept>` +
			`<template_id><value>t</value></template_id><definition><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id>` +
			`<attributes><rm_attribute_name>items</rm_attribute_name><children><node_id>at0001</node_id></children></attributes></definition></template>`
	}
	holdsLinearCost(t, []costCase{
		{
			name:     "empty list entries of a C_INTEGER",
			doc:      integer(strings.Repeat(`<list></list>`, 50_000)),
			baseline: integer(strings.Repeat(`<list>1</list>`, 50_000)),
			modes:    []string{"default", "strict"},
		},
		{
			name:     "C_BOOLEAN items with empty flags",
			doc:      head + strings.Repeat(boolean(`<true_valid/><false_valid/>`), 20_000) + tail,
			baseline: head + strings.Repeat(boolean(`<true_valid>1</true_valid><false_valid>1</false_valid>`), 20_000) + tail,
			modes:    []string{"default"},
		},
		{
			name:     "<children> typed with a primitive class",
			doc:      head + strings.Repeat(leaf("C_STRING"), 20_000) + tail,
			baseline: head + strings.Repeat(leaf("C_COMPLEX_OBJECT"), 20_000) + tail,
			modes:    []string{"default"},
		},
		{
			name:     "node ids in the XSI namespace",
			doc:      head + strings.Repeat(`<children xsi:type="C_COMPLEX_OBJECT"><xsi:node_id>at1</xsi:node_id></children>`, 40_000) + tail,
			baseline: head + strings.Repeat(`<children xsi:type="C_COMPLEX_OBJECT"><node_id>at1</node_id></children>`, 40_000) + tail,
			modes:    []string{"default"},
		},
		{
			name:     "<children> with no type under namespace declarations",
			doc:      headWith(declarations(20_000)) + strings.Repeat(`<children><node_id/></children>`, 20_000) + tail,
			baseline: headWith(declarations(20_000)) + strings.Repeat(leaf("C_COMPLEX_OBJECT"), 20_000) + tail,
			modes:    []string{"default"},
		},
		{
			name:     "quantities whose assumed value the model cannot hold under namespace declarations",
			doc:      headWith(declarations(2_000)) + strings.Repeat(quantity("x"), 2_000) + tail,
			baseline: headWith(declarations(2_000)) + strings.Repeat(quantity("1"), 2_000) + tail,
			modes:    []string{"default", "strict"},
		},
		{
			name:     "an item typed C_PRIMITIVE_OBJECT that declares namespaces and holds elements",
			doc:      item("C_PRIMITIVE_OBJECT", 4_000),
			baseline: item("C_STRING", 4_000),
			modes:    []string{"default"},
		},
		{
			name:     "names a fresh prefix could take",
			doc:      untypedWith("xsi", 40_000),
			baseline: untypedWith("xsj", 40_000),
			modes:    []string{"default"},
		},
	})
}

// TestREQ108_ReadOperationalTemplateRefusesADeepElementAfterTheRoot pins that
// an element nested past the element limit after the root element, in a
// document a reading of the default mode rewrites, is refused as the content
// after the root it is, with an error wrapping canxml.ErrInvalidShape that
// carries none of it.
func TestREQ108_ReadOperationalTemplateRefusesADeepElementAfterTheRoot(t *testing.T) {
	t.Parallel()
	const planted = "PLANTED"
	n := readElementLimit + 80
	doc := itemDoc(`<item xsi:type="C_FUTURE"/>`) + strings.Repeat("<x>", n) + planted + strings.Repeat("</x>", n)
	refusesContentOutsideTheRoot(t, doc, planted)
}

// TestREQ108_ReadOperationalTemplateReads128ConstraintLevels pins that the
// element limit leaves room for 128 levels of constraint objects below the
// definition, the deepest holding a C_DV_ORDINAL with its list.
func TestREQ108_ReadOperationalTemplateReads128ConstraintLevels(t *testing.T) {
	t.Parallel()
	const levels = 128
	interval := `<lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper>`
	var b strings.Builder
	b.WriteString(`<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`)
	b.WriteString(`<template_id><value>t</value></template_id><definition><rm_type_name>COMPOSITION</rm_type_name>`)
	b.WriteString(`<occurrences>` + interval + `</occurrences><node_id>at0000</node_id>`)
	for level := 1; level <= levels; level++ {
		b.WriteString(`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>a</rm_attribute_name>`)
		b.WriteString(`<existence>` + interval + `</existence>`)
		if level < levels {
			b.WriteString(`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>CLUSTER</rm_type_name>`)
			b.WriteString(`<occurrences>` + interval + `</occurrences><node_id>at0001</node_id>`)
			continue
		}
		b.WriteString(`<children xsi:type="C_DV_ORDINAL"><rm_type_name>DV_ORDINAL</rm_type_name>`)
		b.WriteString(`<occurrences>` + interval + `</occurrences><node_id></node_id>`)
		b.WriteString(`<list><value>1</value><symbol><defining_code><terminology_id><value>local</value></terminology_id>`)
		b.WriteString(`<code_string>at0002</code_string></defining_code></symbol></list></children></attributes>`)
	}
	for range levels - 1 {
		b.WriteString(`</children></attributes>`)
	}
	b.WriteString(`</definition></template>`)
	doc := b.String()
	if depth := maxDepth(t, doc); depth > readElementLimit || depth < 2+2*levels {
		t.Fatalf("the document nests %d elements, want between %d and %d", depth, 2+2*levels, readElementLimit)
	}

	ot, err := aom14.ReadOperationalTemplate(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate(%d constraint levels): %v", levels, err)
	}
	attrs := ot.Definition.Attributes
	for level := 1; level < levels; level++ {
		attr, ok := attrs[0].(*aom14.CSingleAttribute)
		if !ok || len(attr.Children) != 1 {
			t.Fatalf("level %d: attribute %T, want one *aom14.CSingleAttribute child", level, attrs[0])
		}
		obj, ok := attr.Children[0].(*aom14.CComplexObject)
		if !ok || len(obj.Attributes) != 1 {
			t.Fatalf("level %d: child %T, want a *aom14.CComplexObject with one attribute", level, attr.Children[0])
		}
		attrs = obj.Attributes
	}
	attr, ok := attrs[0].(*aom14.CSingleAttribute)
	if !ok || len(attr.Children) != 1 {
		t.Fatalf("level %d: attribute %T, want one *aom14.CSingleAttribute child", levels, attrs[0])
	}
	ordinal, ok := attr.Children[0].(*aom14.CDVOrdinal)
	if !ok || len(ordinal.List) != 1 {
		t.Fatalf("level %d: child %T, want a *aom14.CDVOrdinal with one list item", levels, attr.Children[0])
	}
}

// maxDepth returns the depth of the deepest element of doc, the root at
// depth 1.
func maxDepth(t *testing.T, doc string) int {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(doc))
	depth, deepest := 0, 0
	for {
		tok, err := dec.RawToken()
		if errors.Is(err, io.EOF) {
			return deepest
		}
		if err != nil {
			t.Fatalf("reading the document: %v", err)
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
			deepest = max(deepest, depth)
		case xml.EndElement:
			depth--
		}
	}
}

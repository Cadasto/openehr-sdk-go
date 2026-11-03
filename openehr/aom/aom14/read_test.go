package aom14_test

// REQ-108 (clinical-modeling.md § REQ-108, Operational template model reader):
// aom14.ReadOperationalTemplate refuses an input larger than the size limit,
// reading at most one byte past it, and an element nested deeper than the
// element limit, during the decode through canxml.WithMaxDepth, each with an
// error wrapping canxml.ErrLimitExceeded. REQ-176 (clinical-modeling.md
// § REQ-176): it reads every corpus operational template into the model as
// canxml.Unmarshal does, a byte-order-marked one among them, and does not
// match the root element by name.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
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

// TestREQ176_ReadOperationalTemplateRefusesContentAfterTheRoot pins that the
// reader holds the input to one document, as canxml.Unmarshal does: after the
// root, comments, processing instructions and whitespace pass, and anything
// else is refused with an error wrapping canxml.ErrInvalidShape that carries
// none of it.
func TestREQ176_ReadOperationalTemplateRefusesContentAfterTheRoot(t *testing.T) {
	t.Parallel()
	const planted = "PLANTED"
	refused := map[string]string{
		"a second root":       `<template><template_id><value>` + planted + `</value></template_id></template>`,
		"an empty element":    `<` + planted + `/>`,
		"text":                planted,
		"a stray end element": `</` + planted + `>`,
		"a document type":     `<!DOCTYPE ` + planted + `>`,
		"a CDATA section":     `<![CDATA[` + planted + `]]>`,
	}
	accepted := map[string]string{
		"nothing":                  "",
		"whitespace":               " \n\t\r\n",
		"a comment":                `<!-- note -->`,
		"a processing instruction": `<?pi data?>`,
	}
	for name, tail := range refused {
		t.Run("refused/"+name, func(t *testing.T) {
			t.Parallel()
			_, err := aom14.ReadOperationalTemplate(strings.NewReader(readTemplate + tail))
			if !errors.Is(err, canxml.ErrInvalidShape) {
				t.Fatalf("ReadOperationalTemplate(root + %s) = %v, want an error wrapping canxml.ErrInvalidShape", name, err)
			}
			if strings.Contains(err.Error(), planted) {
				t.Errorf("error %q carries the content after the root", err)
			}
		})
	}
	for name, tail := range accepted {
		t.Run("accepted/"+name, func(t *testing.T) {
			t.Parallel()
			ot, err := aom14.ReadOperationalTemplate(strings.NewReader(readTemplate + tail))
			if err != nil {
				t.Fatalf("ReadOperationalTemplate(root + %s): %v", name, err)
			}
			if ot.TemplateID.Value != "t" {
				t.Errorf("TemplateID.Value = %q, want %q", ot.TemplateID.Value, "t")
			}
		})
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
		"WithUnmatched":           {canxml.WithUnmatched(func(string) {})},
		"WithMissing":             {canxml.WithMissing(func(string) {})},
		"WithUnmatched, no limit": {canxml.WithUnmatched(func(string) {}), canxml.WithMaxDepth(0)},
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
		"the limit with WithUnmatched":   {canxml.WithMaxDepth(limit), canxml.WithUnmatched(func(string) {})},
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

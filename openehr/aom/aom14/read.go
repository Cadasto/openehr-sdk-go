package aom14

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/decodeopts"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// readMaxBytes is the size limit of [ReadOperationalTemplate]: 32 MiB.
const readMaxBytes = 32 << 20

// readMaxDepth is the element limit of [ReadOperationalTemplate], the root
// element at depth 1: an `<attributes>` and a `<children>` element for each
// of 128 levels of constraint objects below the definition, and 64 more for
// the root, the definition and the deepest content one constraint object
// holds, such as an assumed value or an expression tree.
const readMaxDepth = 2*128 + 64

// inputBuffers holds the buffers [ReadOperationalTemplate] keeps its input in,
// so that a read allocates none; a buffer larger than inputBufferMax is
// dropped rather than kept for the next read.
var inputBuffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// inputBufferMax is the largest buffer inputBuffers keeps: 4 MiB.
const inputBufferMax = 4 << 20

// ReadOperationalTemplate reads one ADL 1.4 operational template, in the XML
// form of the AM Release-1.4 template schema, from r into the model, within
// limits that bound what an untrusted document costs to read.
//
// It reads at most 32 MiB from r, and refuses a larger input: it reads one
// byte past the limit to tell the two apart. It refuses an element nested
// deeper than 320 elements, the root element at depth 1, when it reads the
// element's start tag, before it reads on; that leaves room for 128 levels of
// constraint objects below the definition. It does so wherever the element
// stands, inside content a reading below leaves out too. Both refusals wrap
// [canxml.ErrLimitExceeded], so one errors.Is test matches either, and
// neither carries any of the input. The size refusal names the limit; the
// depth refusal is the [canxml.DecodeError] that [canxml.WithMaxDepth]
// describes.
//
// opts apply to the decode as they do to a [canxml.Decoder], so
// [canxml.WithStrict] refuses content the model does not read. A
// [canxml.WithMaxDepth] among them applies as well as the element limit, and
// the smaller limit wins: it can lower the limit and never raise it.
//
// The input is one document. Given [canxml.WithStrict], it is held to the
// rules [canxml.Unmarshal] holds a document to: before the root it may hold
// the XML declaration, comments, processing instructions, whitespace and one
// document type declaration, after a UTF-8 byte-order mark at the very start;
// after the root it may hold comments, processing instructions and
// whitespace. Without it, the reader also reads past, before the root, any
// content encoding/xml reads, such as text or a CDATA section, and, after
// the root, a `<!…>` declaration. Anything else before
// or after the root, such as an element after it or a stray end tag before it,
// is refused with an error that wraps [canxml.ErrInvalidShape] and carries
// none of it. The root element may have any name, so a document whose root is
// `<OPERATIONAL_TEMPLATE>` reads as one whose root is `<template>`. An input
// with no root element, empty or holding only a declaration, comments,
// whitespace or text, is refused with an error that wraps
// [io.ErrUnexpectedEOF], not [io.EOF], and carries none of the input. An error
// r returns, other than [io.EOF], ends the read and comes back as r returned
// it, whatever the options and whatever a decode made of the bytes before it.
// An input whose elements do not nest, with an end tag that does not close the
// element open before it or with its end inside the root element, is refused
// with the [xml.SyntaxError] that encoding/xml gives for it, inside content a
// reading below leaves out too. Any other decode error comes back as the
// decode returns it.
//
// Operational templates in use hold shapes and values their schema does not
// allow. The reader reads them in a weaker form, some in every mode and the
// rest in the default mode alone. In every mode it reads:
//
//   - an element in the http://schemas.openehr.org/v2 namespace, which the
//     AM Release-1.4 schemas declare, as the element of that name in the
//     http://schemas.openehr.org/v1 namespace canonical XML reads;
//   - a C_BOOLEAN whose true_valid or false_valid is absent or empty with
//     that flag true;
//   - a C_STRING list_open of white space only as an empty one, so false;
//   - the assumed_value of a C_BOOLEAN, C_INTEGER or C_REAL that holds no
//     value of its type as absent, a boolean being true, false, 1 or 0 in
//     any letter case, and the assumed_value of a C_DV_QUANTITY that the
//     model cannot hold as absent;
//   - a bound of a C_INTEGER range or of a quantity's precision written as a
//     decimal number, such as 1.0 or 2.5, as the integer bound that admits
//     the same integers: a lower bound rounded up and an upper one down, the
//     side included where the number is not a whole one; and one written as
//     an infinity that admits every integer on its side, such as -Inf on the
//     lower side or +Infinity on the upper one, or as NaN, which bounds no
//     integer, as that side unbounded, included unless the range says
//     otherwise;
//   - an empty timezone_validity of a C_DATE, C_TIME or C_DATE_TIME as
//     absent, which is optional;
//   - a range bound on a side the range writes unbounded that holds no
//     number, or a number of a C_INTEGER or C_REAL or of a quantity's
//     magnitude or precision that canonical XML does not read, and a bound of
//     the magnitude or precision of a C_DV_QUANTITY that holds no number, as
//     absent, the side unbounded.
//
// Without [canxml.WithStrict] it also reads an element in a namespace other
// than none and those two, the root and the XML Schema instance one among
// them, as the element of that name in the http://schemas.openehr.org/v1
// namespace, matching it by its local name: under another default
// namespace, under a prefix bound to another namespace, the xsi of the
// xsi:type attributes among them, under the xml prefix, and under a prefix
// no declaration binds, which encoding/xml reads as a namespace of that
// name. It reads each such element so on its own, and a prefix keeps the
// namespace it is bound to, so that an xsi:type reads as one, and a type
// written with a prefix, such as xsd:string, reads as written. It reads a
// <definition> whose type names no class of the AOM 1.4 constraint model,
// or a class other than C_ARCHETYPE_ROOT and C_COMPLEX_OBJECT, which a
// definition cannot hold, but those it refuses in every mode (below), as a
// C_COMPLEX_OBJECT leaf with the rm_type_name, node_id and occurrences it
// holds and none of its other members, such as its attributes, archetype id
// and term definitions, as it reads such a <children>. And, inside the
// definition, it reads:
//
//   - an <attributes>, a <children> or the <item> of a C_PRIMITIVE_OBJECT
//     that names no type in the XML Schema instance namespace, none, an
//     empty one or one under a prefix bound to another namespace: such an
//     <attributes> as a C_MULTIPLE_ATTRIBUTE where it holds a <cardinality>
//     and as a C_SINGLE_ATTRIBUTE otherwise, such a <children> as a
//     C_ARCHETYPE_ROOT where it holds an archetype id and as a
//     C_COMPLEX_OBJECT otherwise, and such an <item> as no item;
//   - the type of an <attributes>, a <children> or an <item> without the
//     white space around it;
//   - a <children> typed C_COMPLEX_OBJECT that holds an archetype id, as a
//     C_ARCHETYPE_ROOT;
//   - a <children> whose type names no class of the AOM 1.4 constraint
//     model, or a class a <children> cannot hold, such as
//     C_SINGLE_ATTRIBUTE, as a C_COMPLEX_OBJECT with the rm_type_name,
//     node_id and occurrences it holds and none of its other members;
//   - a <children> typed with a C_PRIMITIVE class, such as C_STRING, as a
//     C_PRIMITIVE_OBJECT with the rm_type_name, node_id and occurrences it
//     holds, whose item, of that class, holds the rest;
//   - an <item> typed C_CODE_PHRASE, C_DV_QUANTITY, C_DV_ORDINAL or
//     CONSTRAINT_REF, as the object that holds it, of the item's class, with
//     the members of the item but an rm_type_name, node_id or occurrences;
//     and an <item> typed C_PRIMITIVE_OBJECT as the item it holds;
//   - an <item> whose type names no C_PRIMITIVE class and none of those, as
//     no item;
//   - a list entry of a C_INTEGER or C_REAL that holds no number of its
//     type as absent, and a bound of its range that holds none as absent,
//     the side unbounded;
//   - a value of a C_REAL, or a bound of a quantity's magnitude, that Go's
//     strconv.ParseFloat reads and XML Schema's double does not, such as Inf,
//     0x1p-2 or a decimal of more digits than a double holds, as the number
//     it reads;
//   - a timezone_validity that names no validity kind as absent.
//
// A side a bound is read out of keeps the included flag the range gives it;
// where the range gives none and does not write the side unbounded, the side
// is included.
//
// What a reading leaves out is unmatched content, which
// [canxml.WithUnmatched] reports at its path: the path it has in the input,
// or, for content an item a reading writes or takes off holds, its path in
// the object that holds the content then. Given [canxml.WithStrict], which
// refuses unmatched content, the readings of every mode leave it out
// unreported, and the reader refuses each shape and value the default mode
// alone reads, as it refuses a bare type attribute on such an element, which
// canonical XML reads as its type, and an empty list entry or range bound of
// a C_INTEGER or C_REAL, which canonical XML reads as 0. It refuses a
// <definition> typed with a class it cannot hold, or with white space around
// the type, which canonical XML does not dispatch on, with a
// [canxml.DecodeError] of the definition that wraps [typereg.ErrUnknownType]
// where the type names no class and [typereg.ErrTypeMismatch] otherwise, as
// canonical XML refuses such a type where it dispatches on it.
//
// In every mode the reader refuses an integer that the model's 32-bit
// Integer cannot hold, a bound of a C_INTEGER range or of a quantity's
// precision that rounds to one among them, with an error that wraps
// [strconv.ErrRange]. It refuses such a bound of positive infinity on the
// lower side or of negative infinity on the upper one, on a side the range
// does not write unbounded, which admits no integer, with the error of the
// decode that reads it as an integer: one that wraps [strconv.ErrSyntax],
// whose [strconv.NumError] holds the infinity as written. It refuses a
// match_negated of true, which AOM 1.4 cannot express: read past, it would
// have its meaning inverted. And it refuses a <definition> typed
// ARCHETYPE_SLOT, or with a class that carries a primitive constraint: a
// C_PRIMITIVE class, such as C_STRING, one of C_CODE_PHRASE, C_DV_QUANTITY,
// C_DV_ORDINAL and CONSTRAINT_REF, or C_PRIMITIVE_OBJECT where the items it
// holds carry such a constraint. Such a definition is a slot or a leaf that
// carries that constraint, and the model's definition, a C_ARCHETYPE_ROOT,
// holds neither. The refusal is a [canxml.DecodeError] of the definition,
// whose Type is the type as written, that wraps [typereg.ErrTypeMismatch].
// These are the four readings the reader refuses in every mode.
//
// A document that needs none of these readings is decoded once; one that
// needs one is decoded a second time, from the bytes the reader keeps of the
// input, but for what it reads past before and after the root, and so is
// every document when opts pass what the decode reports to a function, which
// sees each item once.
//
// A caller that decodes an operational template with [canxml.Unmarshal] or a
// [canxml.Decoder] itself gets neither limit and none of these readings
// unless it applies them.
func ReadOperationalTemplate(r io.Reader, opts ...canxml.DecoderOption) (*OperationalTemplate, error) {
	// lr.N reaches 0 only once r has given a byte past the limit.
	src := &sourceReader{r: r}
	lr := &io.LimitedReader{R: src, N: readMaxBytes + 1}
	// in keeps every byte read from r, for the checks and a second decode; it
	// is made as large as r says it is. Nothing the read returns refers to it.
	in := inputBuffers.Get().(*bytes.Buffer)
	in.Reset()
	defer func() {
		if in.Cap() <= inputBufferMax {
			inputBuffers.Put(in)
		}
	}()
	if l, ok := r.(interface{ Len() int }); ok {
		in.Grow(min(l.Len(), readMaxBytes) + 1)
	}
	br := bufio.NewReader(io.TeeReader(lr, in))
	settings := decodeopts.Of(opts)
	// A decode that reports to the caller's function runs once the reader
	// knows which input it decodes, so that the function sees each item once;
	// the first decode then reports to none.
	first := opts
	if settings.Reports {
		first = nil
		if settings.Strict {
			first = []canxml.DecoderOption{canxml.WithStrict()}
		}
	}
	ot, err := readDocument(br, first, settings.Strict)
	// An input past the size limit is refused as such, whatever the decode
	// made of the bytes it read: the limit may have cut it short.
	if lr.N == 0 {
		return nil, errInputTooLarge()
	}
	// A failure of r ends the read with its own error, whatever the decode
	// made of the bytes before it, since the input is cut short.
	if src.err != nil {
		return nil, src.err
	}
	var tolerant bool
	if err == nil {
		tolerant = needsTolerance(in.Bytes(), ot, settings.Strict)
	} else {
		tolerant = retryable(err, settings.Strict)
	}
	if !tolerant && !settings.Reports {
		return readResult(ot, err)
	}
	if tolerant && err != nil {
		// The decode stopped short of the end, which the second reading needs.
		if _, cerr := io.Copy(io.Discard, br); cerr != nil {
			return nil, cmp.Or(src.err, cerr)
		}
		if lr.N == 0 {
			return nil, errInputTooLarge()
		}
	}
	data := in.Bytes()
	if tolerant {
		// The second reading may leave out content no decode then reads, so
		// it holds the input to the decode's depth limit itself.
		maxDepth := readMaxDepth
		if settings.MaxDepth > 0 {
			maxDepth = min(maxDepth, settings.MaxDepth)
		}
		read, changed, err := tolerate(data, settings.Strict, maxDepth)
		if err != nil {
			return nil, err
		}
		if changed {
			return readResult(readDocument(bufio.NewReader(bytes.NewReader(read)), opts, settings.Strict))
		}
	}
	if settings.Reports {
		return readResult(readDocument(bufio.NewReader(bytes.NewReader(data)), opts, settings.Strict))
	}
	return readResult(ot, err)
}

// sourceReader reads r and keeps the first error it returns other than
// [io.EOF].
type sourceReader struct {
	r   io.Reader
	err error
}

func (s *sourceReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) && s.err == nil {
		s.err = err
	}
	return n, err
}

// readDocument decodes one document from br with opts and the element limit:
// what comes before the root, the root, and what comes after it. Unless
// strict, it reads past the content before the root that encoding/xml reads,
// and a `<!…>` declaration after it.
func readDocument(br *bufio.Reader, opts []canxml.DecoderOption, strict bool) (*OperationalTemplate, error) {
	// The stream decoder skips what comes before the root, so the prolog is
	// checked first; the decode then reads the bytes the check read again.
	head, err := xmlunmatched.ReadProlog(br)
	if err != nil && (strict || !errors.Is(err, canxml.ErrInvalidShape) || !readsPastProlog(head)) {
		return nil, err
	}
	// The decoder takes no byte past the root element from an
	// io.ByteReader, so what follows the root is left for afterRoot.
	src := bufio.NewReader(io.MultiReader(bytes.NewReader(head), br))
	// WithMaxDepth keeps the smallest limit it is given, so the caller's own
	// limit applies as well, in whatever order the options come.
	opts = slices.Concat(opts, []canxml.DecoderOption{canxml.WithMaxDepth(readMaxDepth)})
	ot := new(OperationalTemplate)
	if err := canxml.NewDecoder(src, opts...).Decode(ot); err != nil {
		return nil, err
	}
	if err := afterRoot(src, strict); err != nil {
		return nil, err
	}
	return ot, nil
}

// readsPastProlog reports whether the default mode reads past head, the bytes
// before the root element and its start tag that the prolog check refused:
// whether encoding/xml reads each token in it up to that start tag, whatever
// the token is. A byte-order mark at the very start is the one byte sequence
// it skips unread.
func readsPastProlog(head []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(head, byteOrderMark)))
	for {
		tok, err := d.Token()
		if err != nil {
			return false
		}
		if _, ok := tok.(xml.StartElement); ok {
			return true
		}
	}
}

// readResult returns what [ReadOperationalTemplate] returns for a decode that
// gave ot and err.
func readResult(ot *OperationalTemplate, err error) (*OperationalTemplate, error) {
	// The decode gives a bare io.EOF when the input holds no root element,
	// which a caller reading a stream would take for the end of its own.
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("aom14: no operational template in the input: %w", io.ErrUnexpectedEOF)
	}
	if err != nil {
		return nil, err
	}
	return ot, nil
}

// errInputTooLarge is the refusal of an input past the size limit.
func errInputTooLarge() error {
	return fmt.Errorf("aom14: input exceeds %d bytes: %w", readMaxBytes, canxml.ErrLimitExceeded)
}

// afterRoot reads r, the rest of the input after the root element, to its
// end. Comments, processing instructions and whitespace pass, and unless
// strict a `<!…>` declaration as well, which encoding/xml reads. Anything
// else, an element, other text or markup encoding/xml cannot read, is
// refused, with an error that wraps [canxml.ErrInvalidShape] and names none
// of it, as [canxml.Unmarshal] refuses it.
func afterRoot(r io.Reader, strict bool) error {
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if _, ok := errors.AsType[*xml.SyntaxError](err); ok {
			return errAfterRoot()
		}
		if err != nil {
			return fmt.Errorf("aom14: read after the root element: %w", err)
		}
		switch t := tok.(type) {
		case xml.Comment, xml.ProcInst:
			continue
		case xml.Directive:
			if !strict {
				continue
			}
		case xml.CharData:
			if len(bytes.Trim(t, " \t\r\n")) == 0 {
				continue
			}
		}
		return errAfterRoot()
	}
}

// errAfterRoot is the refusal of content after the root element.
func errAfterRoot() error {
	return fmt.Errorf("aom14: content after the root element: %w", canxml.ErrInvalidShape)
}

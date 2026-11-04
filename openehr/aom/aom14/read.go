package aom14

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"

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

// ReadOperationalTemplate reads one ADL 1.4 operational template, in the XML
// form of the AM Release-1.4 template schema, from r into the model, within
// limits that bound what an untrusted document costs to read.
//
// It reads at most 32 MiB from r, and refuses a larger input: it reads one
// byte past the limit to tell the two apart. It refuses an element nested
// deeper than 320 elements, the root element at depth 1, when it reads the
// element's start tag, before it reads on; that leaves room for 128 levels of
// constraint objects below the definition. Both refusals wrap
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
// The input is one document, held to the rules [canxml.Unmarshal] holds a
// document to. Before the root it may hold the XML declaration, comments,
// processing instructions, whitespace and one document type declaration,
// after a UTF-8 byte-order mark at the very start; after the root it may hold
// comments, processing instructions and whitespace. Anything else before or
// after the root, such as text or an element, is refused with an error that
// wraps [canxml.ErrInvalidShape] and carries none of it. The root element may
// have any name, so a document whose root is `<OPERATIONAL_TEMPLATE>` reads as
// one whose root is `<template>`. An input with no root element, empty or
// holding only a declaration, comments, whitespace or text, is refused with
// an error that wraps [io.ErrUnexpectedEOF], not [io.EOF], and carries none
// of the input. Any other decode error comes back as the decode returns it.
//
// A caller that decodes an operational template with [canxml.Unmarshal] or a
// [canxml.Decoder] itself gets neither limit unless it applies them.
func ReadOperationalTemplate(r io.Reader, opts ...canxml.DecoderOption) (*OperationalTemplate, error) {
	// lr.N reaches 0 only once r has given a byte past the limit.
	lr := &io.LimitedReader{R: r, N: readMaxBytes + 1}
	br := bufio.NewReader(lr)
	// The stream decoder skips what comes before the root, so the prolog is
	// checked first; the decode then reads the bytes the check read again.
	ot := new(OperationalTemplate)
	head, err := xmlunmatched.ReadProlog(br)
	if err == nil {
		// The decoder takes no byte past the root element from an
		// io.ByteReader, so what follows the root is left for afterRoot.
		src := bufio.NewReader(io.MultiReader(bytes.NewReader(head), br))
		// WithMaxDepth keeps the smallest limit it is given, so the caller's
		// own limit applies as well, in whatever order the options come.
		opts = slices.Concat(opts, []canxml.DecoderOption{canxml.WithMaxDepth(readMaxDepth)})
		err = canxml.NewDecoder(src, opts...).Decode(ot)
		if err == nil {
			err = afterRoot(src)
		}
	}
	// An input past the size limit is refused as such, whatever the decode
	// made of the bytes it read: the limit may have cut it short.
	if lr.N == 0 {
		return nil, fmt.Errorf("aom14: input exceeds %d bytes: %w", readMaxBytes, canxml.ErrLimitExceeded)
	}
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

// afterRoot reads r, the rest of the input after the root element, to its
// end. Comments, processing instructions and whitespace pass. Anything else,
// an element, other text, a declaration or markup encoding/xml cannot read,
// is refused, with an error that wraps [canxml.ErrInvalidShape] and names
// none of it, as [canxml.Unmarshal] refuses it.
func afterRoot(r io.Reader) error {
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

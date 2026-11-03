package xmlunmatched

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// scope holds the namespace prefixes declared on the elements open at a
// reader's last token, so that a prefix written in an attribute value, such
// as the `xs` of `xsi:type="xs:string"`, can be resolved where the attribute
// stands. bindings holds every declaration of the open elements in document
// order, and marks the length bindings had as each open element opened.
type scope struct {
	bindings []binding
	marks    []int
}

// binding is one namespace declaration: `xmlns:prefix="uri"`.
type binding struct {
	prefix, uri string
}

// open records the element whose attributes are attrs as opened, with the
// prefixes it declares, and returns attrs without its namespace declarations,
// attrs itself when it holds none.
func (s *scope) open(attrs []xml.Attr) []xml.Attr {
	s.marks = append(s.marks, len(s.bindings))
	decls := false
	for _, a := range attrs {
		switch {
		case a.Name.Space == "xmlns":
			s.bindings = append(s.bindings, binding{prefix: a.Name.Local, uri: a.Value})
			decls = true
		case isNamespaceDecl(a.Name):
			decls = true
		}
	}
	if !decls {
		return attrs
	}
	return withoutNamespaceDecls(attrs)
}

// close records the innermost open element as closed, dropping the prefixes it
// declared.
func (s *scope) close() {
	n := len(s.marks)
	if n == 0 {
		return
	}
	s.bindings = s.bindings[:s.marks[n-1]]
	s.marks = s.marks[:n-1]
}

// lookup returns the namespace prefix is bound to by the innermost declaration
// in scope, and false when no open element declares it.
func (s *scope) lookup(prefix string) (string, bool) {
	for _, b := range slices.Backward(s.bindings) {
		if b.prefix == prefix {
			return b.uri, true
		}
	}
	return "", false
}

var (
	// scoped maps the decoder of each scoped decode in progress to the reader
	// that keeps its namespace scope, for exactly as long as that decode runs.
	scoped sync.Map // *xml.Decoder → *trackingReader
	// scoping counts the scoped decodes in progress. While it is zero no
	// decoder is registered, and [LookupPrefix] skips the lookup.
	scoping atomic.Int64
)

// Scope keeps the namespace prefixes declared on the elements a decode opens,
// so that [LookupPrefix] resolves a prefix written in an attribute value, for a
// decode that applies no policy for unmatched content and follows no path:
// one through its decoder behaves as one through a plain encoding/xml decoder
// does. A Scope serves one decoder, from one goroutine at a time.
type Scope struct {
	reader *trackingReader
}

// NewDecoder returns a decoder that reads canonical XML from r and keeps the
// namespace scope of every element it opens in s. Decode values from it with
// [Scope.Decode].
func (s *Scope) NewDecoder(r io.Reader) *xml.Decoder {
	s.reader = &trackingReader{src: xml.NewDecoder(r)}
	return xml.NewTokenDecoder(s.reader)
}

// Decode decodes the next value of dec, a decoder from [Scope.NewDecoder],
// into v, with dec registered meanwhile, so that [LookupPrefix] finds the
// prefixes in scope.
func (s *Scope) Decode(dec *xml.Decoder, v any) error {
	scoping.Add(1)
	scoped.Store(dec, s.reader)
	defer func() {
		scoped.Delete(dec)
		scoping.Add(-1)
	}()
	return dec.Decode(v)
}

// LookupPrefix returns the namespace prefix is bound to where the attributes
// of start stand, start being the element the decoder read last: the
// declaration on start itself, or else the innermost one on an element around
// it. bound is false when no declaration in scope binds prefix. known is false
// when dec cannot tell the declarations of the elements around start, a plain
// encoding/xml decoder's, and start does not declare prefix itself; uri and
// bound are then meaningless.
//
// A tracked decode and a scoped one ([Scope]) know every declaration in
// scope.
func LookupPrefix(dec *xml.Decoder, start xml.StartElement, prefix string) (uri string, bound, known bool) {
	for _, a := range start.Attr {
		if a.Name.Space == "xmlns" && a.Name.Local == prefix {
			return a.Value, true, true
		}
	}
	if dec == nil {
		return "", false, false
	}
	if r := readerOf(dec); r != nil {
		uri, bound = r.scope.lookup(prefix)
		return uri, bound, true
	}
	return "", false, false
}

// readerOf returns the reader that keeps the namespace scope of dec, a tracked
// or scoped decode's decoder, and nil for any other decoder.
func readerOf(dec *xml.Decoder) *trackingReader {
	if t := trackerOf(dec); t != nil {
		return t.reader
	}
	if scoping.Load() == 0 {
		return nil
	}
	if r, ok := scoped.Load(dec); ok {
		return r.(*trackingReader)
	}
	return nil
}

// errUnresolvedPrefix is the refusal of an `xsi:type` whose prefix a decode
// through a plain encoding/xml decoder cannot resolve: the element does not
// declare it, and the decoder does not tell the declarations of the elements
// around it. It wraps typereg.ErrUnknownType, so a caller of the generated
// decoders that reads through its own encoding/xml decoder sees an unknown
// type; [Unmarshal] decodes the document again through a decoder that keeps
// the namespace scope instead ([IsUnresolvedPrefix]).
var errUnresolvedPrefix = fmt.Errorf("%w: the prefix of the discriminator is declared on no element this decode can see", typereg.ErrUnknownType)

// UnresolvedPrefix returns the refusal of an `xsi:type` whose prefix the
// decode cannot resolve, after [LookupPrefix] answered that it does not know
// the declarations in scope.
func UnresolvedPrefix() error { return errUnresolvedPrefix }

// IsUnresolvedPrefix reports whether err holds the refusal of an `xsi:type`
// whose prefix the decode could not resolve ([UnresolvedPrefix]).
func IsUnresolvedPrefix(err error) bool { return errors.Is(err, errUnresolvedPrefix) }

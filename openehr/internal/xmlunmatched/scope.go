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
	// rereading counts the scoped decodes in progress that decode a value a
	// second time ([Scope.DecodeAgain]). While it is zero, [Enter] looks no
	// decoder up for one.
	rereading atomic.Int64
)

// Scope keeps the namespace prefixes declared on the elements a decode opens,
// so that [LookupPrefix] resolves a prefix written in an attribute value, for a
// decode that applies no policy for unmatched content and follows no path:
// one through its decoder behaves as one through a plain encoding/xml decoder
// does, but for the depth limit [Scope.LimitDepth] sets. A Scope serves one
// decoder, from one goroutine at a time.
type Scope struct {
	reader *trackingReader
	// maxDepth is the depth limit [Scope.LimitDepth] set; 0 for none.
	maxDepth int
}

// LimitDepth makes the decodes through the decoder [Scope.NewDecoder] returns
// next refuse an element nested deeper than n, as [Tracker.LimitDepth] says,
// and the refusal comes back on its own. n <= 0 sets no limit, the default.
func (s *Scope) LimitDepth(n int) { s.maxDepth = max(n, 0) }

// NewDecoder returns a decoder that reads canonical XML from r and keeps the
// namespace scope of every element it opens in s. Decode values from it with
// [Scope.Decode].
func (s *Scope) NewDecoder(r io.Reader) *xml.Decoder {
	s.reader = &trackingReader{src: xml.NewDecoder(r)}
	if s.maxDepth > 0 {
		// The reader follows the open elements, for the depth and the path of
		// a refusal, in a tracker no decoder is registered with: the
		// generated decoders find no policy, and the decode stays untracked.
		t := NewTracker(nil, false)
		t.LimitDepth(s.maxDepth)
		s.reader.t = t
	}
	return xml.NewTokenDecoder(s.reader)
}

// Decode decodes the next value of dec, a decoder from [Scope.NewDecoder],
// into v, with dec registered meanwhile, so that [LookupPrefix] finds the
// prefixes in scope. Under a depth limit, the depth counts from the root of
// that value, and a refusal comes back on its own, outside the dispatch
// envelopes it passed through.
func (s *Scope) Decode(dec *xml.Decoder, v any) error {
	t := s.reader.t
	if t != nil {
		t.reset()
	}
	scoping.Add(1)
	scoped.Store(dec, s.reader)
	defer func() {
		scoped.Delete(dec)
		scoping.Add(-1)
	}()
	err := dec.Decode(v)
	if t != nil {
		return t.alone(err)
	}
	return err
}

// DecodeAgain decodes the next value of dec into v as [Scope.Decode] does, as
// the second decode of a value that a decode through a plain encoding/xml
// decoder filled in part before it stopped at a prefix it could not resolve
// ([IsUnresolvedPrefix]). The decoder of the value's root element clears its
// receiver before it reads, as [Enter] tells it to, so that the second decode
// does not add to what the first left in v, such as the items of a list.
func (s *Scope) DecodeAgain(dec *xml.Decoder, v any) error {
	s.reader.again = true
	rereading.Add(1)
	defer func() {
		rereading.Add(-1)
		s.reader.again = false
	}()
	return s.Decode(dec, v)
}

// againAtRoot reports whether dec is the decoder of a value a [Scope] decodes
// a second time ([Scope.DecodeAgain]) and the element it read last opens the
// root of that value.
func againAtRoot(dec *xml.Decoder) bool {
	if rereading.Load() == 0 {
		return false
	}
	r := readerOf(dec)
	return r != nil && r.again && len(r.scope.marks) == 1
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

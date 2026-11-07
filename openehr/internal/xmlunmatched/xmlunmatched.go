// Package xmlunmatched tracks unmatched content on the canonical-XML decode
// path: the content a decode does not read. It is the runtime the generated
// UnmarshalXML methods of openehr/rm and openehr/aom/aom14 call, and that
// canxml drives from its decode entry points; canxml re-exports its
// sentinels.
//
// Content is unmatched when the decode does not read it: an element or
// attribute that names no attribute of the RM or AOM 1.4 type its element
// decodes as, other than namespace declarations and attributes in the XML
// Schema instance namespace (`xsi:type`, `xsi:schemaLocation`, `xsi:nil`, …);
// an element in a namespace other than the openEHR one; an element whose
// content the codec does not model: a `Hash` of `Hash`es and a member whose Go
// type has no decoder of its own; and an entry of a `Hash` member whose key an
// earlier entry of the member holds. So is text other than
// XML whitespace directly inside an element whose content is elements, one
// item per element however many runs of text it holds. By default the decode
// skips it. A caller can instead track it: report each item, in document
// order, by its path, and optionally refuse the first one.
//
// # How it works
//
// encoding/xml hands a generated UnmarshalXML method only the *xml.Decoder
// and the start element, so the decode's policy cannot travel as an argument.
// It travels with the decoder instead:
//
//   - A tracked decode reads the input through the decoder [Tracker.NewDecoder]
//     returns, whose token source keeps the stack of open elements and, for
//     each, how many children of each name it has opened. Every token passes
//     through it, whichever code asks for it, so the path of the element just
//     read is always known.
//   - [Tracker.Decode] registers that decoder with the decode's [Tracker] for
//     exactly as long as one value decodes. Each tracked decode has its own
//     decoder and tracker, so concurrent decodes never see each other's
//     policy.
//   - The generated decoders call the helpers of this package where content
//     goes unread: [Enter] on entry, with the description of their class,
//     [LocalName] for the name they switch on, [SkipUnmatched] in the arm
//     for an element they do not read, [CheckUnmatchedText] for text
//     between the elements they read, [DecodeElement] for an element
//     decoded as text, [DecodeText] for one whose text they convert
//     themselves, such as an interval boundary flag, [DecodeBase64] for an
//     Octet list, [DecodeParam] for a member typed by an open generic
//     parameter, [DropElement] for an element the class's schema accepts
//     that no member holds, [DecodeChoice] for a member whose element
//     takes one of several shapes, which its first child chooses, and
//     [DecodeEntry] and [DecodeEntryKeyedBy] for one entry of a `Hash`
//     member, which take the key attribute off before the value is read. The
//     hand-written TERMINOLOGY_CODE decoder calls the same
//     helpers, [CheckUnmatchedAttrs] for the attributes of its inner
//     TERMINOLOGY_ID element, [CheckUnmatchedTextBefore] for text it keeps
//     until a child shows that the text is not its value, and
//     [SkipUnmatchedAfter] and [ApplyHeld] for an unmatched child after
//     that text. canxml calls [DiscriminatorRead] where its dispatch reads a
//     bare `type`. Each helper looks the decoder up; an untracked decode
//     finds nothing and behaves as encoding/xml does, but for one thing.
//
// # Namespace scope
//
// An `xsi:type` written with a prefix, such as `xs:string`, is resolved
// against the namespace declarations in scope where the attribute stands.
// encoding/xml resolves element and attribute names but not a prefix inside
// an attribute value, and hands a decoder none of the declarations of the
// elements around the one it reads. So the token source of a tracked decode
// keeps the declarations of the open elements, and so does that of a
// [Scope]; [LookupPrefix] answers from them. An untracked decode through a
// plain encoding/xml decoder sees only the declarations an element carries
// itself. When a prefix needs more ([UnresolvedPrefix]), [Unmarshal] decodes
// the document again through a tracker that applies no policy, and canxml's
// stream decoder decodes the value again through a Scope
// ([Scope.DecodeAgain]).
//
// # Missing members
//
// The dual of unmatched content is a mandatory member the document leaves
// out. The description each decoder hands [Enter], a [Class], names the
// members the Reference Model requires of its class, and a tracked decode
// given [Tracker.ReportMissing] reports, as each element closes, every one
// of them the element carried no value for: no child element of that name
// and no XML attribute of it where the class reads one; a child carrying
// `xsi:nil="true"`; or a primitive child with no text, or a string child
// whose text is XML whitespace only, unless the decode keeps the string's
// text ([Tracker.KeepStringText]). A struct element with no content, such as
// `<subject/>`, is a value. Each member is named by its JSON Pointer in the canonical-JSON
// form of the instance, so a consumer of either canonical format reads one
// list. The report is the whole policy: a missing member is never a decode
// error, and a decode that does not ask pays nothing for it.
//
// # Depth limit
//
// A decode given a depth limit ([Tracker.LimitDepth], [Scope.LimitDepth])
// refuses an element nested deeper than the limit below the root of the
// value, in its token source, as the start tag is read: no decoder sees the
// element, and every later read returns the refusal. The refusal wraps
// [ErrLimitExceeded], names the element by its path, with the middle of a
// long one left out, and comes back on its own. The token source follows the
// open elements for it; a scoped decode does so in a tracker no decoder is
// registered with, so the decode applies no other policy.
//
// encoding/xml has a limit of its own: it decodes no element nested deeper
// than 10 000 elements, and fails with an error of its own that no sentinel
// matches, inside one dispatch envelope per polymorphic slot it passes
// through on its way out. A tracked or scoped decode turns that error into a
// refusal of the same form, which names encoding/xml's limit
// ([IsXMLDepthError]); after an untracked decode, the value is decoded again
// through a tracker to find the element's path. A tracked decode converts the
// text of a primitive element through a decoder that sees that element alone,
// and so refuses a primitive element past encoding/xml's limit itself, with
// the same refusal: it refuses the primitive elements an untracked decode
// refuses.
//
// # Whole documents
//
// [Unmarshal] decodes one whole document, through a tracker or none: it
// holds the input to one root element, refusing other content before and
// after it as malformed input, and names a primitive failure by its path
// from the root either way, decoding the input again when nothing tracked
// the first decode. canxml.Unmarshal calls it with the tracker its options
// build, or none, so that a caller of this package that needs no option
// decodes a document by the same rule.
//
// # Primitive failures
//
// The text of a primitive element that does not decode as its type is a
// primitive failure, in every decode: [DecodeElement], [DecodeParam],
// [DecodeText] and [DecodeBase64] return a *typereg.DecodeError whose cause
// wraps [ErrInvalidShape] and the conversion's own error, never the bare
// error encoding/xml or a TextUnmarshaler gives, whose text may quote the
// value. [DropElement] returns one for an element whose text is not its one
// value, and [DecodeEntry] and [DecodeEntryKeyedBy] for a `Hash` entry with
// no key, each with a cause whose fixed text names no value. A tracked decode
// names the element by its path from the root of the value and
// [Tracker.Decode] returns the failure on its own; an untracked decode names
// the element alone, and the failure travels out inside the dispatch
// envelopes it passes through. [IsPrimitiveFailure] tells such a failure
// apart, so canxml can decode the input again, tracked, to find the path.
package xmlunmatched

import (
	"bytes"
	"encoding"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// Canonical XML namespaces: the values of canxml.NSDefault and canxml.NSXSI,
// which a canxml test holds equal to these.
const (
	// NSOpenEHR is the openEHR canonical-XML namespace.
	NSOpenEHR = "http://schemas.openehr.org/v1"
	// NSXSI is the XML Schema instance namespace.
	NSXSI = "http://www.w3.org/2001/XMLSchema-instance"
)

// ErrUnknownElement classifies content the decode does not read. canxml
// re-exports it as canxml.ErrUnknownElement, where it is documented.
var ErrUnknownElement = errors.New("canxml: unknown element or attribute")

// ErrNamespace classifies an element in a namespace other than the openEHR
// one. canxml re-exports it as canxml.ErrNamespace, where it is documented.
var ErrNamespace = errors.New("canxml: foreign XML namespace")

// ErrInvalidShape classifies input of the wrong shape, among it the text of a
// primitive element that does not decode as its type. canxml re-exports it as
// canxml.ErrInvalidShape, where it is documented.
var ErrInvalidShape = errors.New("canxml: invalid XML shape")

// ErrLimitExceeded classifies input past a limit the decode was given, among
// it an element nested deeper than [Tracker.LimitDepth] allows. canxml
// re-exports it as canxml.ErrLimitExceeded, where it is documented.
var ErrLimitExceeded = errors.New("canxml: decode limit exceeded")

// Tracker is the policy and position of one tracked decode: the caller's
// report function, whether the first unmatched item refuses the decode, and
// the elements open at the decoder's current token. A Tracker serves one
// decoder, from one goroutine at a time.
type Tracker struct {
	report func(path string)
	strict bool
	// checked counts the elements whose content a decoder of the SDK has
	// checked: each generated UnmarshalXML, through [Enter], and each
	// primitive element [DecodeElement] reads as text. A rise across a
	// decode shows that something read the element.
	checked int
	// missing is the caller's hook for the mandatory members a document
	// leaves out, set by [Tracker.ReportMissing]; nil when none asked, and
	// then no element is checked for them.
	missing func(pointer string)
	// presence is the caller's hook for the members a document sends without
	// a value, set by [Tracker.ReportPresence]; nil when none asked.
	presence func(pointer string, nilled bool)
	// stringText keeps the text of a string element sent empty or as XML
	// whitespace only, set by [Tracker.KeepStringText].
	stringText bool
	// open holds one frame per element opened and not yet closed, after a
	// first frame for the document itself, which counts the top-level
	// elements of the value being decoded.
	open []frame
	// failure is the primitive failure the value being decoded raised, kept
	// so that [Tracker.Decode] returns it on its own; nil until one is raised.
	failure *typereg.DecodeError
	// reader is the token source of the decoder [Tracker.NewDecoder] returned,
	// which [DecodeChoice] reads ahead through; nil until then.
	reader *trackingReader
	// textDec converts the text of each primitive element ([Tracker.convert]);
	// nil until the first, and after a conversion that failed.
	textDec *textDecoder
	// maxDepth is the deepest an element may nest below the root of the value,
	// the root at depth 1, set by [Tracker.LimitDepth]; 0 sets no limit.
	maxDepth int
	// refusal is the refusal of the first element nested deeper than
	// maxDepth, kept so that the decode returns it on its own and every later
	// read of the decoder returns it again, the input standing inside that
	// element; nil until an element is refused.
	refusal *typereg.DecodeError
	// held collects, while [DecodeEntryKeyedBy] reads the value of a Hash
	// entry whose key lies inside the value, the unmatched items and the
	// missing members the value holds, in place of applying the policy to
	// them ([Tracker.holdEntry]); nil otherwise.
	held *[]heldItem
}

// frame is one open element: its local name, its 1-based position among its
// earlier siblings of the same name plus one, the children it has opened so
// far, counted by local name, whether dispatch read its bare `type`
// attribute as the discriminator, and whether its text was found unmatched
// already. For the missing-member and presence reports it also keeps the
// description of its class, once its decoder has entered, which of the class's mandatory
// members it has a value for, what the element carries: `xsi:nil`, text
// other than whitespace, and whether a decoder read it as a primitive, and,
// for one entry of a Hash member, how to read its key. It also marks an
// element the decoder skipped or dropped, which is no member, and counts the
// children it skipped that are named like one of its class's list members.
//
// The child counts sit in a short slice while the element has opened children
// of few names, which is the canonical shape and costs no map. Once it opens
// children of more than [linearChildNames] names, the counts move to a map, so
// an element with many differently named children costs a lookup per child
// rather than a scan of every name seen before it.
type frame struct {
	name          string
	pos           int
	children      []nameCount
	byName        map[string]int // nil until the slice outgrows linearChildNames
	discriminator bool
	text          bool
	class         *Class
	seen          uint64 // a bit per class.Required member the element has a value for
	nilled        bool   // the start element, or one around it, carried xsi:nil="true"
	hasText       bool   // the element holds text that carries a value: other than XML whitespace, or any text when spaceIsText
	primitive     bool   // a decoder read the element's text as its value
	spaceIsText   bool   // a decoder reads the element's text into a primitive that is no string, whose XML whitespace is text ([noValue])
	kept          bool   // a string element whose text the decode keeps, empty or whitespace only ([Tracker.KeepStringText])
	unread        bool   // the decoder skipped or dropped the element: it is no member of its parent
	// skipped counts, for each member class.Items lists, the children of its
	// name the decoder skipped, which are no items of it ([Tracker.closing]);
	// empty until the first. It holds no more counts than the class lists
	// members, whatever the document skips.
	skipped []int
	// key reads the key of the element when it is one entry of a Hash member,
	// so the members inside it are named under the key; nil otherwise.
	key func() string
	// optional names the members of the element's class that the shape it
	// takes as one of a choice of shapes makes optional, which are never
	// missing; nil otherwise.
	optional []string
}

// linearChildNames is the number of distinct child names a frame counts in its
// slice before it moves the counts to a map. An RM class has a few dozen
// attributes at most, and an element of canonical XML repeats few names.
const linearChildNames = 8

// open makes f the frame of a newly opened element named local at position
// pos, keeping the slice a frame left over from an earlier element had, so a
// long decode does not allocate one per element. A map from an earlier wide
// element is dropped rather than cleared, so a later narrow element never pays
// for it.
func (f *frame) open(local string, pos int) {
	*f = frame{name: local, pos: pos, children: f.children[:0], skipped: f.skipped[:0]}
}

// skip notes one more skipped child named like the i-th of the n members
// the element's class lists in Items ([frame.skipped]).
func (f *frame) skip(i, n int) {
	if len(f.skipped) == 0 {
		f.skipped = append(f.skipped[:0], make([]int, n)...)
	}
	f.skipped[i]++
}

// skippedAt returns how many children named like the i-th member the
// element's class lists in Items the decoder has skipped so far
// ([frame.skipped]).
func (f *frame) skippedAt(i int) int {
	if i < len(f.skipped) {
		return f.skipped[i]
	}
	return 0
}

// nameCount is how many children of one local name an element has opened.
type nameCount struct {
	name string
	n    int
}

// NewTracker returns the tracker of a decode that passes every unmatched item
// to report, when report is not nil, and refuses the first one when strict is
// set.
func NewTracker(report func(path string), strict bool) *Tracker {
	return &Tracker{report: report, strict: strict, open: make([]frame, 1, 16)}
}

// NewDecoder returns a decoder that reads canonical XML from r and records
// every element it opens and closes in t. Decode values from it with
// [Tracker.Decode].
func (t *Tracker) NewDecoder(r io.Reader) *xml.Decoder {
	t.reader = &trackingReader{src: xml.NewDecoder(r), t: t}
	return xml.NewTokenDecoder(t.reader)
}

// Decode decodes the next value of dec, a decoder from [Tracker.NewDecoder],
// into v, with dec registered for t meanwhile, so the generated decoders
// apply t's policy. Paths are relative to the value being decoded. A strict
// refusal and a primitive failure come back on their own, not inside the
// dispatch envelopes they passed through on their way out: each names its
// item from the root of the value, where each envelope names only its own
// slot.
func (t *Tracker) Decode(dec *xml.Decoder, v any) error {
	t.reset()
	tracking.Add(1)
	trackers.Store(dec, t)
	defer func() {
		trackers.Delete(dec)
		tracking.Add(-1)
	}()
	return t.alone(dec.Decode(v))
}

// reset starts a new value: the document frame forgets the top-level elements
// of the values decoded before, so every path is relative to the value being
// decoded, and no primitive failure is kept.
func (t *Tracker) reset() {
	t.open = t.open[:1]
	t.open[0].open("", 0)
	t.failure = nil
}

// alone returns the failure in err's chain that names its item from the root
// of the value, a depth refusal, a strict refusal or the primitive failure t
// raised, on its own, and err unchanged when the chain holds none (a nil err
// included). encoding/xml's refusal of an element nested past its own limit
// becomes a depth refusal of that element ([Tracker.refuseXMLDepth]).
func (t *Tracker) alone(err error) error {
	if err == nil {
		return nil
	}
	if t.refusal != nil && errors.Is(err, t.refusal) {
		return t.refusal
	}
	if IsXMLDepthError(err) {
		return t.refuseXMLDepth()
	}
	if ue, ok := errors.AsType[*unknownElementError](err); ok && ue != nil {
		return ue
	}
	if t.failure != nil && errors.Is(err, t.failure) {
		return t.failure
	}
	return err
}

// push records that the decoder opened an element named local inside the
// innermost open element. A frame left over from an earlier element is
// reused ([frame.open]).
func (t *Tracker) push(local string) {
	pos := t.open[len(t.open)-1].count(local)
	if len(t.open) < cap(t.open) {
		t.open = t.open[:len(t.open)+1]
		t.open[len(t.open)-1].open(local, pos)
		return
	}
	t.open = append(t.open, frame{name: local, pos: pos})
}

// pop records that the decoder closed the innermost open element, after
// reporting the mandatory members it left out, and whether it is a member
// sent without a value, when the decode asked.
func (t *Tracker) pop() {
	n := len(t.open)
	if n <= 1 {
		return
	}
	if t.checksPresence() {
		t.closing(n)
	}
	t.open = t.open[:n-1]
}

// count notes one more child named local and returns its position among the
// children of that name. It scans at most [linearChildNames] names, then
// looks the name up in the map.
func (f *frame) count(local string) int {
	if f.byName != nil {
		f.byName[local]++
		return f.byName[local]
	}
	for i := range f.children {
		if f.children[i].name == local {
			f.children[i].n++
			return f.children[i].n
		}
	}
	if len(f.children) < linearChildNames {
		f.children = append(f.children, nameCount{name: local, n: 1})
		return 1
	}
	f.byName = make(map[string]int, 4*linearChildNames)
	for _, c := range f.children {
		f.byName[c.name] = c.n
	}
	f.byName[local] = 1
	return 1
}

// path returns the path of the innermost open element: one `/name[n]`
// segment per open element, from the root of the value.
func (t *Tracker) path() string {
	var b strings.Builder
	t.writePath(&b, len(t.open))
	return b.String()
}

// writePath writes the path of the open element whose frame is t.open[n-1]:
// one `/name[n]` segment per open element from the root of the value down to
// it, and nothing for the document frame.
func (t *Tracker) writePath(b *strings.Builder, n int) {
	for _, f := range t.open[1:n] {
		writeSegment(b, f.name, f.pos)
	}
}

// LimitDepth makes the decodes through t's decoder refuse an element nested
// deeper than n below the root of the value being decoded, the root at depth
// 1, as soon as the decoder reads its start tag, before anything decodes it.
// n <= 0 sets no limit, the default. The refusal is a *typereg.DecodeError
// that wraps [ErrLimitExceeded], names the element's depth and n, and names
// the element by its path, kept to [pathEnds] segments at each end; the
// decoder returns it again from every later read.
func (t *Tracker) LimitDepth(n int) { t.maxDepth = max(n, 0) }

// refuseDepth refuses the element the decoder opened last, nested one past the
// depth limit, and keeps the refusal, which it returns.
func (t *Tracker) refuseDepth() error {
	t.refusal = &typereg.DecodeError{
		Path:  t.endsPath(),
		Inner: fmt.Errorf("%w: element depth (%d > %d)", ErrLimitExceeded, len(t.open)-1, t.maxDepth),
	}
	return t.refusal
}

// refuseXMLDepth refuses the element the decoder opened last, which
// encoding/xml did not decode for its own depth limit, as [Tracker.refuseDepth]
// refuses one past t's limit, naming encoding/xml's limit, and keeps the
// refusal, which it returns, so that every later read of the decoder returns
// it again. The decoder then stands inside that element, so a later decode
// fails for its depth too, before it reads a token: the refusal kept is
// returned for it, and no other is made.
func (t *Tracker) refuseXMLDepth() error {
	if t.refusal == nil {
		t.refusal = &typereg.DecodeError{
			Path:  t.endsPath(),
			Inner: fmt.Errorf("%w: element depth (%d > %d)", ErrLimitExceeded, len(t.open)-1, xmlMaxDepth()),
		}
	}
	return t.refusal
}

// xmlMaxDepth returns encoding/xml's own depth limit, the number of nested
// elements past which it decodes no element: 10 000, and 5 000 on wasm.
// encoding/xml does not export it.
func xmlMaxDepth() int {
	if runtime.GOARCH == "wasm" {
		return 5_000
	}
	return 10_000
}

// xmlDepthText is the text of the error encoding/xml returns when it is asked
// to decode an element nested deeper than its own limit. encoding/xml exports
// neither that error nor the limit, so the error is told by its text.
const xmlDepthText = "exceeded max depth"

// IsXMLDepthError reports whether err is encoding/xml's refusal of an element
// nested deeper than its own limit, as it comes or inside the dispatch
// envelopes it passed through on its way out: a chain of errors, each
// wrapping one, that ends in an error with that refusal's text. A tracked or
// scoped decode returns a refusal that wraps [ErrLimitExceeded] in its place;
// [Unmarshal] and canxml's stream decoder decode the value again through one
// when an untracked decode returns it.
func IsXMLDepthError(err error) bool {
	for err != nil {
		u, ok := err.(interface{ Unwrap() error }) //nolint:errorlint // the chain is walked one link at a time, to its end
		if !ok {
			return err.Error() == xmlDepthText
		}
		err = u.Unwrap()
	}
	return false
}

// pathEnds is how many segments the path of a depth refusal keeps at each end
// of a longer path, so that the error stays short however deep the limit lets
// a document nest.
const pathEnds = 8

// endsPath returns the path of the innermost open element as [Tracker.path]
// does, but with at most pathEnds segments at each end: a longer path has a
// `/...` segment in place of those between, which no element's segment can
// be, since each carries its position.
func (t *Tracker) endsPath() string {
	frames := t.open[1:]
	if len(frames) <= 2*pathEnds {
		return t.path()
	}
	var b strings.Builder
	for _, f := range frames[:pathEnds] {
		writeSegment(&b, f.name, f.pos)
	}
	b.WriteString("/...")
	for _, f := range frames[len(frames)-pathEnds:] {
		writeSegment(&b, f.name, f.pos)
	}
	return b.String()
}

// writeSegment writes the path segment of the element named name at position
// pos among its siblings of that name.
func writeSegment(b *strings.Builder, name string, pos int) {
	b.WriteByte('/')
	b.WriteString(name)
	b.WriteByte('[')
	b.WriteString(strconv.Itoa(pos))
	b.WriteByte(']')
}

// place is where an element sits: its local name and its position among its
// siblings of that name. A decode keeps the place of an element it reads as
// text, so that it can name the element once the element has closed.
type place struct {
	name string
	pos  int
}

// here returns the place of the innermost open element.
func (t *Tracker) here() place {
	f := &t.open[len(t.open)-1]
	return place{name: f.name, pos: f.pos}
}

// closedPath returns the path of the element at p, a child of the innermost
// open element that has closed since its place was kept.
func (t *Tracker) closedPath(p place) string {
	var b strings.Builder
	t.writePath(&b, len(t.open))
	writeSegment(&b, p.name, p.pos)
	return b.String()
}

// unmatched applies the policy to the unmatched item at path: report it, then
// refuse it under strict. foreign marks an element in a foreign namespace.
func (t *Tracker) unmatched(path string, foreign bool) error {
	if t.held != nil {
		*t.held = append(*t.held, heldItem{path: path, foreign: foreign})
		return nil
	}
	if t.report != nil {
		t.report(path)
	}
	if t.strict {
		return &unknownElementError{path: path, foreign: foreign}
	}
	return nil
}

// attrs applies the policy to every attribute of the innermost open element
// that is not read: neither an XML Schema instance attribute (the `xsi:type`
// discriminator among them), nor a namespace declaration, nor one of known in
// no namespace, nor a bare `type` that dispatch read as the discriminator.
func (t *Tracker) attrs(attrs []xml.Attr, known []string) error {
	t.checked++
	var base string
	read := t.open[len(t.open)-1].discriminator
	for _, a := range attrs {
		if isSchemaInstance(a.Name) || isNamespaceDecl(a.Name) ||
			(a.Name.Space == "" && slices.Contains(known, a.Name.Local)) ||
			(read && isBareType(a.Name)) {
			continue
		}
		if base == "" {
			base = t.path()
		}
		if err := t.unmatched(base+"/@"+a.Name.Local, false); err != nil {
			return err
		}
	}
	return nil
}

// text applies the policy to text the decoder read directly inside the open
// element whose frame is t.open[n-1], whose content is elements: unless it is
// XML whitespace, it is unmatched, at the element's path with a last
// `/text()` segment. An element is one item however many runs of text it
// holds, so only its first unmatched run counts. With n below 2 no element
// holds the text, and it is left alone.
func (t *Tracker) text(n int, text []byte) error {
	if n < 2 {
		return nil
	}
	f := &t.open[n-1]
	if f.text || isXMLSpace(text) {
		return nil
	}
	f.text = true
	var b strings.Builder
	t.writePath(&b, n)
	b.WriteString("/text()")
	return t.unmatched(b.String(), false)
}

// isXMLSpace reports whether text holds XML whitespace only: spaces, tabs,
// carriage returns and line feeds.
func isXMLSpace(text []byte) bool {
	return len(bytes.TrimLeft(text, " \t\r\n")) == 0
}

// decodeText decodes the element start opens, whose content is text, into v.
// An attribute and a child element are unmatched, and the text around a child
// element is kept, as encoding/xml keeps it. The text then decodes into v
// through encoding/xml itself ([Tracker.convert]), so the conversion is the
// one an untracked decode applies. Text that does not convert is a primitive
// failure, which names the element by its path from the root of the value. An
// element nested deeper than encoding/xml's own depth limit is refused before
// anything of it is read, as an untracked decode refuses it
// ([Tracker.refuseXMLDepth]).
func (t *Tracker) decodeText(dec *xml.Decoder, start *xml.StartElement, v any) error {
	t.markPrimitive(v).kept = t.keepsText(v)
	return t.readText(dec, start, v)
}

// markPrimitive marks the innermost open element as a primitive whose text a
// decoder reads into v, and returns its frame. The text of a primitive that is
// no string carries a value whenever it is not empty, white space included, as
// [noValue] reads it: a Character may be one space, and the text of a number
// or boolean that is white space only fails as it converts.
func (t *Tracker) markPrimitive(v any) *frame {
	f := &t.open[len(t.open)-1]
	f.primitive = true
	_, isString := v.(*string)
	f.spaceIsText = !isString
	return f
}

// readText decodes the element start opens into v as [Tracker.decodeText]
// does, but leaves the element unmarked as a primitive: an entry of a Hash
// member, which is a value of the member whatever its text.
func (t *Tracker) readText(dec *xml.Decoder, start *xml.StartElement, v any) error {
	at := t.here()
	text, err := t.elementText(dec, start)
	if err != nil {
		return err
	}
	if err := t.convert(start.Name, text, v); err != nil {
		return t.fail(t.closedPath(at), err)
	}
	return nil
}

// decodeOptional decodes the element start opens into v as
// [Tracker.decodeText] does, unless the element carries no value
// ([noValue]) and t does not keep its text ([Tracker.KeepStringText]), and
// reports whether it decoded it.
func (t *Tracker) decodeOptional(dec *xml.Decoder, start *xml.StartElement, v any) (bool, error) {
	// The frame does not record kept: [Tracker.closing] reads it for a
	// required member only, and an optional member is none.
	t.markPrimitive(v)
	kept := t.keepsText(v)
	at := t.here()
	text, err := t.elementText(dec, start)
	if err != nil || (!kept && noValue(string(text), v)) {
		return false, err
	}
	if err := t.convert(start.Name, text, v); err != nil {
		return false, t.fail(t.closedPath(at), err)
	}
	return true, nil
}

// elementText reads the element start opens, whose content is text, to its
// end, and returns its text. An attribute and a child element are unmatched,
// and the text around a child element is kept, as encoding/xml keeps it. An
// element nested deeper than encoding/xml's own depth limit is refused before
// anything of it is read.
func (t *Tracker) elementText(dec *xml.Decoder, start *xml.StartElement) ([]byte, error) {
	// The element is the innermost open one, at the depth encoding/xml counts
	// for it: the tracker follows every element from the start of the input.
	if len(t.open)-1 > xmlMaxDepth() {
		return nil, t.refuseXMLDepth()
	}
	if err := t.attrs(start.Attr, nil); err != nil {
		return nil, err
	}
	var text []byte
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch tk := tok.(type) {
		case xml.CharData:
			text = append(text, tk...)
		case xml.StartElement:
			if err := t.unmatched(t.path(), isForeign(tk.Name)); err != nil {
				return nil, err
			}
			if err := dec.Skip(); err != nil {
				return nil, err
			}
		case xml.EndElement:
			return text, nil
		}
	}
}

// convert decodes text, the whole text of a primitive element named name,
// into v through encoding/xml, so that the conversion is the one an untracked
// decode applies; the tokens are whole, so it fails only where the text does
// not convert to v's type. An encoding/xml decoder is a large value, so t
// makes one at its first primitive element and hands the same one the tokens
// of each later element. It drops the decoder after a conversion that failed,
// which could leave it partway through the element.
func (t *Tracker) convert(name xml.Name, text []byte, v any) error {
	if t.textDec == nil {
		t.textDec = new(textDecoder)
		t.textDec.dec = xml.NewTokenDecoder(t.textDec)
	}
	td := t.textDec
	td.toks = [3]xml.Token{xml.StartElement{Name: name}, xml.CharData(text), xml.EndElement{Name: name}}
	td.next = 0
	if err := td.dec.Decode(v); err != nil {
		t.textDec = nil
		return err
	}
	return nil
}

var (
	// trackers maps the decoder of each tracked decode in progress to its
	// policy, for exactly as long as that decode runs.
	trackers sync.Map // *xml.Decoder → *Tracker
	// tracking counts the tracked decodes in progress. While it is zero no
	// decoder is registered, and the helpers skip the lookup, so an untracked
	// decode pays one atomic load per helper call.
	tracking atomic.Int64
)

// trackerOf returns the policy registered for dec, or nil when dec belongs to
// no tracked decode.
func trackerOf(dec *xml.Decoder) *Tracker {
	if tracking.Load() == 0 {
		return nil
	}
	if t, ok := trackers.Load(dec); ok {
		return t.(*Tracker)
	}
	return nil
}

// trackingReader is the token source of a tracked decode, and of a scoped one
// ([Scope]). It reads src with Token, so the namespaces are resolved and the
// nesting checked there, records in t every element opened and closed,
// refusing an element nested deeper than t's depth limit, and
// keeps in scope the namespace prefixes the open elements declare. It drops
// the namespace declarations from each start element: the decoder built on it
// then has no prefix to resolve and passes every name through as src resolved
// it.
type trackingReader struct {
	src *xml.Decoder
	// t is the tracker of a tracked decode. For a scoped decode it is a
	// tracker no decoder is registered with, which follows the open elements
	// for a depth refusal and applies no policy.
	t *Tracker
	// scope holds the prefixes the open elements declare, for [LookupPrefix].
	scope scope
	// pending holds the tokens [trackingReader.peekChild] read ahead of the
	// decoder, which Token returns, and records, before it reads src again.
	pending []xml.Token
}

// Token returns the next token, the first read ahead or else the next of src,
// after recording it. A start element nested deeper than the depth limit is
// refused, and so is every token after it.
func (r *trackingReader) Token() (xml.Token, error) {
	if r.t.refusal != nil {
		return nil, r.t.refusal
	}
	var tok xml.Token
	var err error
	if len(r.pending) > 0 {
		tok = r.pending[0]
		r.pending = r.pending[1:]
		if len(r.pending) == 0 {
			r.pending = nil
		}
	} else {
		tok, err = r.src.Token()
	}
	switch tk := tok.(type) {
	case xml.StartElement:
		r.t.push(tk.Name.Local)
		if r.t.maxDepth > 0 && len(r.t.open)-1 > r.t.maxDepth {
			return nil, r.t.refuseDepth()
		}
		if n, ok := repeatedAttr(tk.Attr); ok {
			return nil, r.t.fail(r.t.path()+"/"+attrSegment(n), errRepeatedAttr)
		}
		if r.t.checksPresence() {
			n := len(r.t.open)
			r.t.open[n-1].nilled = r.t.open[n-2].nilled || isNilled(tk.Attr)
		}
		tk.Attr = r.scope.open(tk.Attr)
		return tk, err
	case xml.EndElement:
		r.t.pop()
		r.scope.close()
	case xml.CharData:
		if r.t.checksPresence() {
			if f := &r.t.open[len(r.t.open)-1]; len(tk) > 0 && (f.spaceIsText || !isXMLSpace(tk)) {
				f.hasText = true
			}
		}
	case xml.ProcInst:
		// XML reserves the target xml for the declaration at the start of
		// a document, so one inside an element is not well-formed.
		if len(r.t.open) > 1 && isXMLTarget(tk.Target) {
			return nil, r.t.fail(r.t.path(), errXMLTargetInside)
		}
	case xml.Directive:
		// A directive inside an element, such as a document type
		// declaration, may not hold one either.
		if len(r.t.open) > 1 && holdsXMLTarget(tk) {
			return nil, r.t.fail(r.t.path(), errXMLTargetInside)
		}
	}
	return tok, err
}

// errXMLTargetInside is the cause of the refusal of a processing instruction
// whose target is xml inside an element, which is not well-formed XML.
var errXMLTargetInside error = notWellFormed("the element holds a processing instruction whose target is xml, which XML reserves for the declaration at the start of a document")

// withoutNamespaceDecls returns attrs without its namespace declarations. It
// returns attrs itself when it holds none.
func withoutNamespaceDecls(attrs []xml.Attr) []xml.Attr {
	if !slices.ContainsFunc(attrs, func(a xml.Attr) bool { return isNamespaceDecl(a.Name) }) {
		return attrs
	}
	return slices.DeleteFunc(slices.Clone(attrs), func(a xml.Attr) bool { return isNamespaceDecl(a.Name) })
}

// textDecoder is the decoder [Tracker.convert] converts the text of a
// primitive element with, and its token source: the element's start, its text
// and its end, which the decoder reads once each.
type textDecoder struct {
	dec  *xml.Decoder
	toks [3]xml.Token
	next int
}

// Token returns the next of the element's three tokens, and io.EOF after the
// last. It lets go of each token as it returns it, so a decoder kept between
// elements holds no element's text.
func (d *textDecoder) Token() (xml.Token, error) {
	if d.next == len(d.toks) {
		return nil, io.EOF
	}
	tok := d.toks[d.next]
	d.toks[d.next] = nil
	d.next++
	return tok, nil
}

// IsDiscriminator reports whether an attribute named name is the `xsi:type`
// discriminator in a form canxml.XSITypeOf reads: in the XSI namespace, in
// none (a bare `type`), or written literally as `xsi:type`.
func IsDiscriminator(name xml.Name) bool {
	return (name.Local == "type" && (name.Space == NSXSI || name.Space == "")) || name.Local == "xsi:type"
}

// isSchemaInstance reports whether an attribute named name belongs to the XML
// Schema instance namespace, such as `xsi:schemaLocation`, `xsi:nil` or the
// `xsi:type` discriminator, in a form encoding/xml resolves or as written
// literally. A bare `type` does not: it is in no namespace. Nor does an
// `xsi:` prefix bound to no namespace: encoding/xml leaves it unresolved, so
// the attribute is not in the namespace.
func isSchemaInstance(name xml.Name) bool {
	return name.Space == NSXSI || strings.HasPrefix(name.Local, "xsi:")
}

// isBareType reports whether an attribute named name is a `type` in no
// namespace, which canxml.XSITypeOf also reads as the discriminator.
func isBareType(name xml.Name) bool {
	return name.Space == "" && name.Local == "type"
}

// DiscriminatorRead records that dispatch read the attribute name of the
// element the decoder read last as its `xsi:type` discriminator. canxml's
// polymorphic dispatch calls it. Only a bare `type` needs the record: an
// attribute in the XML Schema instance namespace is never unmatched, but a
// bare `type` is matched only on an element whose dispatch read it, and is
// unmatched content everywhere else.
func DiscriminatorRead(dec *xml.Decoder, name xml.Name) {
	if !isBareType(name) {
		return
	}
	if t := trackerOf(dec); t != nil {
		t.open[len(t.open)-1].discriminator = true
	}
}

// isNamespaceDecl reports whether name is a namespace declaration: `xmlns`,
// or `xmlns:p` as encoding/xml resolves it or as written literally.
func isNamespaceDecl(name xml.Name) bool {
	return name.Space == "xmlns" || (name.Space == "" && name.Local == "xmlns") || strings.HasPrefix(name.Local, "xmlns:")
}

// isForeign reports whether an element named name lies outside the openEHR
// namespace. An element in no namespace counts as an openEHR one.
func isForeign(name xml.Name) bool {
	return name.Space != "" && name.Space != NSOpenEHR
}

// unknownElementError is the refusal of an unmatched item under strict
// tracking. It names the item by its path from the root of the value and
// carries nothing else from the input, so no value reaches the message. It
// wraps [ErrUnknownElement], and [ErrNamespace] too for an element in a
// foreign namespace.
type unknownElementError struct {
	path    string
	foreign bool
}

// Error names the item's path, quoted so that it reads as one token.
func (e *unknownElementError) Error() string {
	msg := ErrUnknownElement.Error() + " " + strconv.Quote(e.path)
	if e.foreign {
		msg += ": " + ErrNamespace.Error()
	}
	return msg
}

// Unwrap returns the sentinels this refusal carries.
func (e *unknownElementError) Unwrap() []error {
	if e.foreign {
		return []error{ErrUnknownElement, ErrNamespace}
	}
	return []error{ErrUnknownElement}
}

// primitiveError is the cause of a primitive failure: the text of a primitive
// element did not decode as its type. It wraps [ErrInvalidShape] and the
// conversion's own error, whose text may name the value. The DecodeError that
// holds it writes no value: its text keeps the sentinel's and renders the
// conversion's error by its structure alone.
type primitiveError struct {
	cause error
}

// Error names the class, then the conversion's own error.
func (e *primitiveError) Error() string {
	return ErrInvalidShape.Error() + ": " + e.cause.Error()
}

// Unwrap returns the shape sentinel and the conversion's own error.
func (e *primitiveError) Unwrap() []error {
	return []error{ErrInvalidShape, e.cause}
}

// primitiveFailure returns the failure of the element at path, whose text did
// not decode as its type; cause is the conversion's own error.
func primitiveFailure(path string, cause error) *typereg.DecodeError {
	return &typereg.DecodeError{Path: path, Inner: &primitiveError{cause: cause}}
}

// fail returns the primitive failure of the element at path and keeps it, so
// that [Tracker.Decode] returns it on its own.
func (t *Tracker) fail(path string, cause error) error {
	t.failure = primitiveFailure(path, cause)
	return t.failure
}

// IsPrimitiveFailure reports whether err holds a primitive failure: the text
// of a primitive element that did not decode as its type.
func IsPrimitiveFailure(err error) bool {
	pe, ok := errors.AsType[*primitiveError](err)
	return ok && pe != nil
}

// conversionRefusal returns the conversion's own error when err, which
// encoding/xml returned decoding an element's text into a primitive, is the
// conversion's refusal: a *strconv.NumError, which encoding/xml returns for a
// number or a boolean, or the refusal of a TextUnmarshaler read through
// [textConversion]. It returns nil for any other error, such as a syntax
// error or one reading the input, and for nil.
func conversionRefusal(err error) error {
	switch e := err.(type) { //nolint:errorlint // encoding/xml returns the conversion's error as it is
	case *strconv.NumError:
		return e
	case *conversionError:
		return e.cause
	}
	return nil
}

// textConversion reads an element's text through u, as encoding/xml does for
// a TextUnmarshaler, and marks u's refusal as a [conversionError], so that it
// is told apart from an error reading the input.
type textConversion struct {
	u encoding.TextUnmarshaler
}

// UnmarshalText hands text to u.
func (c *textConversion) UnmarshalText(text []byte) error {
	if err := c.u.UnmarshalText(text); err != nil {
		return &conversionError{cause: err}
	}
	return nil
}

// conversionError is the refusal of a TextUnmarshaler read through
// [textConversion].
type conversionError struct {
	cause error
}

// Error is the refusal's own text.
func (e *conversionError) Error() string { return e.cause.Error() }

// Unwrap returns the refusal.
func (e *conversionError) Unwrap() error { return e.cause }

// asText returns, in a [textConversion], the TextUnmarshaler whose
// UnmarshalText method encoding/xml reads v through, and v itself when
// encoding/xml reads v otherwise. That TextUnmarshaler is v itself when v is
// one and no xml.Unmarshaler. When v is a pointer to a pointer, it is the
// pointer v points to, under the same condition, which asText allocates when
// it is nil, as encoding/xml would: a member typed as a pointer to a
// TextUnmarshaler, such as *time.Time, is decoded through the address of
// that pointer.
func asText(v any) any {
	if _, ok := v.(xml.Unmarshaler); ok {
		return v
	}
	if u, ok := v.(encoding.TextUnmarshaler); ok {
		return &textConversion{u: u}
	}
	if p, ok := pointee(v); ok {
		if _, ok := p.(xml.Unmarshaler); !ok {
			if u, ok := p.(encoding.TextUnmarshaler); ok {
				return &textConversion{u: u}
			}
		}
	}
	return v
}

// LocalName returns the local name of an element in the openEHR namespace or
// in none, and "" for an element in any other namespace. Generated
// UnmarshalXML methods switch on it, so an element in a foreign namespace
// matches no RM attribute and reaches their [SkipUnmatched] arm.
func LocalName(name xml.Name) string {
	if isForeign(name) {
		return ""
	}
	return name.Local
}

// SkipUnmatched skips the element start opens, which the generated decoder
// does not read: its name matches no attribute of the RM type being decoded,
// it lies in a foreign namespace, or it holds content the codec does not
// model, such as a `Hash` of `Hash`es or a member whose Go type has no decoder
// of its own. The element is one item, whatever it
// holds. start must be the token the decoder read last.
//
// By default it only skips. A tracked decode first reports the element's
// path, and a strict one refuses the decode with an error that wraps
// [ErrUnknownElement], and [ErrNamespace] too for an element in a foreign
// namespace.
func SkipUnmatched(dec *xml.Decoder, start xml.StartElement) error {
	if t := trackerOf(dec); t != nil {
		t.skipping()
		if err := t.unmatched(t.path(), isForeign(start.Name)); err != nil {
			return err
		}
		return dec.Skip()
	}
	return skipUntracked(dec, &start)
}

// SkipUnmodelled skips the element start opens, the element of a member whose
// content the codec does not model, a `Hash` of `Hash`es or a member whose Go
// type has no decoder of its own, as [SkipUnmatched] skips an element: it is
// one item of unmatched content, skipped by default, reported or refused on
// request. Unlike an element SkipUnmatched skips, it is the member's own
// element, so it gives the member a value for the missing-member report, and
// a nilled one is a member sent without a value. start must be the token the
// decoder read last.
func SkipUnmodelled(dec *xml.Decoder, start xml.StartElement) error {
	if t := trackerOf(dec); t != nil {
		if err := t.unmatched(t.path(), isForeign(start.Name)); err != nil {
			return err
		}
		return dec.Skip()
	}
	return skipUntracked(dec, &start)
}

// CheckUnmatchedAttrs applies the decode's policy for unmatched content to
// the attributes of start that the decoder does not read: every attribute
// but those in the XML Schema instance namespace (the `xsi:type`
// discriminator, `xsi:schemaLocation`, `xsi:nil`, …), the namespace
// declarations, those named in known, which are read in no namespace, and a
// bare `type` that dispatch read as the discriminator ([DiscriminatorRead]).
// start must be the token the decoder read last. A decoder of a class calls
// [Enter] instead, which checks the attributes the same way and describes
// the class; CheckUnmatchedAttrs serves an element that is no class of its
// own, such as the TERMINOLOGY_ID inside a hand-written TERMINOLOGY_CODE.
//
// By default it does nothing. A tracked decode reports the path of each such
// attribute, in the order written, and a strict one refuses the first with an
// error that wraps [ErrUnknownElement].
func CheckUnmatchedAttrs(dec *xml.Decoder, start xml.StartElement, known ...string) error {
	t := trackerOf(dec)
	if t == nil {
		return checkRepeatedAttr(&start)
	}
	return t.attrs(start.Attr, known)
}

// CheckUnmatchedText applies the decode's policy for unmatched content to
// text, a run of character data the generated decoder read directly inside
// the element it decodes, whose content is elements. A CDATA section is such
// a run too: encoding/xml reports it as character data. dec must be the
// decoder the text was read from, with no child element of that element open.
//
// By default it does nothing, and costs no more than finding the decode
// untracked. Text that is XML whitespace only (spaces, tabs, carriage returns
// and line feeds) is never unmatched. A tracked decode reports any other text
// as one item per element, at the element's path with a last `/text()`
// segment, however many runs the element holds, and a strict one refuses it
// with an error that wraps [ErrUnknownElement]. The path never carries the
// text.
func CheckUnmatchedText(dec *xml.Decoder, text xml.CharData) error {
	t := trackerOf(dec)
	if t == nil {
		return nil
	}
	return t.text(len(t.open), text)
}

// CheckUnmatchedTextBefore applies the decode's policy for unmatched content
// to text read directly inside an element before the child element the
// decoder read last, as [CheckUnmatchedText] does for text read while no
// child is open. A decoder that keeps such text until a child tells it that
// the text is not the element's value calls it as that child opens, so the
// text is reported before anything inside the child. The path names the
// element that holds the text, with a last `/text()` segment, and that
// element remains one item however many runs of text it holds.
func CheckUnmatchedTextBefore(dec *xml.Decoder, text []byte) error {
	t := trackerOf(dec)
	if t == nil {
		return nil
	}
	return t.text(len(t.open)-1, text)
}

// Held holds the unmatched elements a decoder skipped after text it keeps for
// [CheckUnmatchedTextBefore], while it cannot yet tell whether that text is
// unmatched too. The text comes first in document order, so the policy for
// the elements waits for the text's own. The zero value holds nothing.
type Held struct {
	items []heldItem
}

// heldItem is one unmatched element a [Held] holds, or one item a held Hash
// entry holds ([Tracker.holdEntry]): its path from the root of the value, and
// whether it lies in a foreign namespace; or, when missing is set, the JSON
// Pointer of a mandatory member the entry leaves out; or, when sent is set,
// the JSON Pointer of a member the entry sends without a value, and whether
// it was nilled.
type heldItem struct {
	path    string
	foreign bool
	missing bool
	sent    bool
	nilled  bool
	pointer heldPointer // the pointer of a missing member or a member sent without a value
}

// SkipUnmatchedAfter skips the element start opens, which the decoder does
// not read, as [SkipUnmatched] does. text is the text the decoder read
// directly inside the enclosing element before start and keeps, for
// [CheckUnmatchedTextBefore] or as that element's value. start must be the
// token the decoder read last.
//
// Text other than XML whitespace may yet prove unmatched once a later child
// opens, and it comes before the element in document order. So a tracked
// decode then holds the element in h instead of reporting or refusing it at
// once, and [ApplyHeld] applies the policy later. Beside whitespace only,
// the element is reported or refused at once, as by [SkipUnmatched].
func SkipUnmatchedAfter(dec *xml.Decoder, start xml.StartElement, text []byte, h *Held) error {
	if t := trackerOf(dec); t != nil {
		t.skipping()
		if !isXMLSpace(text) {
			h.items = append(h.items, heldItem{path: t.path(), foreign: isForeign(start.Name)})
		} else if err := t.unmatched(t.path(), isForeign(start.Name)); err != nil {
			return err
		}
		return dec.Skip()
	}
	return skipUntracked(dec, &start)
}

// ApplyHeld applies the decode's policy for unmatched content to the elements
// h holds, in document order, as [SkipUnmatched] would have, and empties h. A
// decoder calls it once the text before them has been checked by
// [CheckUnmatchedTextBefore], or has proved to be the value of the element
// that holds it, at the latest as that element closes.
func ApplyHeld(dec *xml.Decoder, h *Held) error {
	if len(h.items) == 0 {
		return nil
	}
	items := h.items
	h.items = nil
	t := trackerOf(dec)
	if t == nil {
		return nil
	}
	for _, it := range items {
		if err := t.unmatched(it.path, it.foreign); err != nil {
			return err
		}
	}
	return nil
}

// DecodeElement decodes the element start opens into v, as
// dec.DecodeElement(v, start) does. v is a pointer to a primitive, such as a
// string, a number, a boolean or a TextUnmarshaler, a pointer to a pointer
// to a TextUnmarshaler, or a pointer to a type with an UnmarshalXML method.
// start must be the token the decoder read last.
//
// A primitive decodes from the element's text alone: by default, as with
// encoding/xml, an attribute or a child element on it is ignored. A tracked
// decode reports or refuses each as unmatched content, and the value decoded
// is the same as by default. A type with an UnmarshalXML method checks its
// own content. A member of interface type goes through [DecodeParam]
// instead, which sees its type.
//
// Text that does not decode as the primitive's type, such as a boolean
// written `maybe` or a string a TextUnmarshaler refuses, fails with a
// *typereg.DecodeError whose cause wraps [ErrInvalidShape] and the
// conversion's own error, and whose text carries none of the element's. A
// tracked decode names the element by its path from the root of the value;
// an untracked one, which does not know the path, by its name alone.
func DecodeElement(dec *xml.Decoder, start *xml.StartElement, v any) error {
	if t := trackerOf(dec); t != nil {
		if _, ok := v.(xml.Unmarshaler); ok {
			return dec.DecodeElement(v, start)
		}
		return t.decodeText(dec, start, v)
	}
	if err := checkRepeatedAttr(start); err != nil {
		return err
	}
	if err := dec.DecodeElement(asText(v), start); err != nil {
		return untrackedFailure(start, v, err)
	}
	return nil
}

// untrackedFailure returns the error of an untracked decode of the element
// start opens into v that failed with err: a primitive failure, which names
// the element by its name alone, when err is the conversion's refusal, and
// err itself otherwise. A type with an UnmarshalXML method reports its own
// failures.
func untrackedFailure(start *xml.StartElement, v any, err error) error {
	if _, ok := v.(xml.Unmarshaler); ok {
		return err
	}
	if cause := conversionRefusal(err); cause != nil {
		return primitiveFailure("/"+start.Name.Local, cause)
	}
	return err
}

// DecodeParam decodes the element start opens into v, a member typed by an
// open generic parameter T, choosing by T what [DecodeElement] chooses by v.
// start must be the token the decoder read last.
//
//   - T is an interface type, such as the `any` of ORIGINAL_VERSION<any> or
//     the DV_ORDERED of BASE Interval<DV_ORDERED>: the member is a
//     polymorphic slot. An empty one, nil or holding a nil pointer of some
//     type, decodes through dispatch, the canxml polymorphic dispatch on
//     `xsi:type` the generated code passes in, so a missing or unknown
//     `xsi:type` fails as at any polymorphic slot. One that already holds a
//     value is decoded into ([decodeInterface]).
//   - T is a pointer to a type with an UnmarshalXML method: encoding/xml
//     allocates it and that method checks its own content.
//   - Any other T, a primitive, a pointer to a TextUnmarshaler or a type
//     with an UnmarshalXML method on its pointer, goes to [DecodeElement].
//
// Text that does not decode as a primitive's type fails as in
// [DecodeElement], whichever way the member decodes.
func DecodeParam[T any](dec *xml.Decoder, start *xml.StartElement, v *T, dispatch func(*xml.Decoder, xml.StartElement) (T, error)) error {
	var zero T
	switch any(zero).(type) {
	case nil:
		if held := any(*v); held != nil && !isNilPointer(held) {
			return decodeInterface(dec, start, v, held)
		}
		got, err := dispatch(dec, *start)
		if err != nil {
			return err
		}
		*v = got
		return nil
	case xml.Unmarshaler:
		return dec.DecodeElement(v, start)
	}
	return DecodeElement(dec, start, v)
}

// decodeInterface decodes the element start opens into v, a pointer to a
// member of interface type that holds held.
//
// encoding/xml decodes into such a member only when it holds a non-nil
// pointer, and skips the element otherwise; the default decode does exactly
// that, and refuses an element that carries one attribute twice, at any depth
// of one it skips ([skipUntracked]). A tracked decode leaves the choice to
// encoding/xml as well: a pointer to an RM type then checks its own content,
// and when nothing of the SDK checked the element's content, because
// encoding/xml skipped it or read it into a primitive, the element is
// reported or refused as one item once it is read. Text that does not decode
// as the held primitive's type fails as in [DecodeElement].
func decodeInterface(dec *xml.Decoder, start *xml.StartElement, v, held any) error {
	target := v
	if tc, ok := asText(held).(*textConversion); ok {
		// encoding/xml reads the element into a held TextUnmarshaler through
		// its UnmarshalText method; decoding into tc is the same decode with
		// the method's refusal marked.
		target = tc
	}
	t := trackerOf(dec)
	if t == nil {
		if target == v && !isPointer(held) {
			return skipUntracked(dec, start)
		}
		if err := checkRepeatedAttr(start); err != nil {
			return err
		}
		err := dec.DecodeElement(target, start)
		if cause := conversionRefusal(err); cause != nil {
			return primitiveFailure("/"+start.Name.Local, cause)
		}
		return err
	}
	path, checked := t.path(), t.checked
	if err := dec.DecodeElement(target, start); err != nil {
		if cause := conversionRefusal(err); cause != nil {
			return t.fail(path, cause)
		}
		return err
	}
	if t.checked == checked {
		return t.unmatched(path, isForeign(start.Name))
	}
	return nil
}

// DecodeText reads the text of the element start opens, as [DecodeElement]
// reads a string, and hands it to convert, which reads the text as the
// element's value. start must be the token the decoder read last.
//
// A tracked decode reports an attribute or child element on the element, as
// for any primitive. convert's refusal is a primitive failure, as in
// [DecodeElement]: a *typereg.DecodeError whose cause wraps [ErrInvalidShape]
// and convert's own error, and whose text carries none of the element's, so
// convert's error may quote the text. A tracked decode names the element by
// its path from the root of the value; an untracked one by its name alone.
func DecodeText(dec *xml.Decoder, start *xml.StartElement, convert func(text string) error) error {
	t := trackerOf(dec)
	var at place
	if t != nil {
		at = t.here()
	}
	var text string
	if err := DecodeElement(dec, start, &text); err != nil {
		return err
	}
	if err := convert(text); err != nil {
		if t != nil {
			return t.fail(t.closedPath(at), err)
		}
		return primitiveFailure("/"+start.Name.Local, err)
	}
	return nil
}

// DecodeBase64 decodes the element start opens, an `xs:base64Binary` value
// such as DV_MULTIMEDIA's `data` and `integrity_check`, into v. The element's
// text is read as [DecodeText] reads it, so a tracked decode reports an
// attribute or child element on it. XML whitespace inside the text is
// ignored, as the XML Schema type allows. An empty element leaves v nil, as
// an empty byte field reads in canonical JSON. A text that is no standard,
// padded base64 fails as a primitive's text fails in [DecodeElement]: the
// cause names the offset of the first bad byte, never the text.
func DecodeBase64(dec *xml.Decoder, start *xml.StartElement, v *[]byte) error {
	return DecodeText(dec, start, func(text string) error {
		b, err := base64.StdEncoding.DecodeString(strings.Map(dropXMLSpace, text))
		if err != nil {
			return err
		}
		if len(b) == 0 {
			b = nil
		}
		*v = b
		return nil
	})
}

// dropXMLSpace maps the four XML whitespace characters to -1, which drops
// them in strings.Map, and keeps every other rune.
func dropXMLSpace(r rune) rune {
	switch r {
	case ' ', '\t', '\n', '\r':
		return -1
	}
	return r
}

// DropElement reads the element start opens and drops it: an element the
// schema type of the class being decoded accepts but no member of the class
// holds, which may hold one value only, written as one of wants, such as the
// legacy `match_negated` of an ADL 1.4 attribute constraint, false written
// `false` or `0`. start must be the token the decoder read last.
//
// With one of wants for text, less the XML whitespace around it, the element
// is read and dropped in every decode: neither reported nor refused. Any other
// text, none included, is refused in every decode, as a primitive failure is
// in [DecodeText]: a *typereg.DecodeError whose cause wraps [ErrInvalidShape]
// and whose text carries none of the element's. A tracked decode reports an
// attribute or child element on the element, as for any primitive.
func DropElement(dec *xml.Decoder, start *xml.StartElement, wants ...string) error {
	if t := trackerOf(dec); t != nil {
		t.skipping()
	}
	return DecodeText(dec, start, func(text string) error {
		if !slices.Contains(wants, strings.Trim(text, xmlSpace)) {
			return errDroppedValue
		}
		return nil
	})
}

// xmlSpace holds the four XML whitespace characters.
const xmlSpace = " \t\r\n"

// errDroppedValue is the refusal of a dropped element's text other than its
// one value. It names no text.
var errDroppedValue = errors.New("the element holds a value other than the one it may hold")

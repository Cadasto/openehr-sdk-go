package typereg

// Value-free text of a DecodeError (REQ-052). The text of a decode error
// travels to places a caller does not choose for clinical data, a log line or
// an HTTP error body, so [DecodeError.Error] writes no value from the
// document. It writes text from three sources only:
//
//   - Structure it reads off a field it knows: a DecodeError's Path and Type,
//     the RM type a shape failure names, a SemanticError's JSON kind, Go type
//     and pointer, a SyntacticError's pointer and byte offset, a NumError's
//     function name, the offset a base64 refusal names. The fields that
//     hold the value, a SemanticError's JSONValue, the text a
//     SyntacticError's cause quotes and a NumError's Num, are never written.
//   - An error it knows by identity: a sentinel whose text is fixed.
//   - A message the SDK's codecs write, matched against a closed list of
//     forms, whole or as a wrapper's own text around the errors it wraps.
//     Every variable part of a form is structure: an RM or Go type name, an
//     XML element name, a quoted `_type` value or member pointer, or a count.
//
// Any other text is left out. A wrapper it cannot place is followed to the
// error it wraps, as is any wrapper above a DecodeError, whose own text would
// repeat that DecodeError's; an error that wraps several has each written in
// turn, on the same line; and any other error is named by its class, its Go
// type, alone. So the guarantee does not rest on what an unknown error
// happens to say: text is kept only where it is known to hold no value. The
// causes themselves are untouched and stay reachable through Unwrap, where
// their own text may name the value.

import (
	"encoding/base64"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/refused"
)

// writeValueFree writes the value-free text of err into b. It follows the
// chain one link at a time, and it never asks an error above a DecodeError
// for its text, which would write that DecodeError's text once more for every
// such error above it. So its cost grows with the length of the text rather
// than with the square of the chain's length, or with a power of it.
func writeValueFree(b *strings.Builder, err error) {
	w := walk{b: b}
	w.run(err)
}

// walk is one pass along a chain of errors.
type walk struct {
	b        *strings.Builder
	suffixes []string // text wrappers write after what they wrap, outermost first
	// below is a DecodeError known to lie further down the chain, and none
	// reports that none does; with neither set, the chain below has not been
	// searched since the last DecodeError was passed.
	below *DecodeError
	none  bool
}

// run writes the text of the chain from err down.
func (w *walk) run(err error) {
	for err != nil {
		err = w.link(err)
	}
	for _, suffix := range slices.Backward(w.suffixes) {
		w.b.WriteString(suffix)
	}
}

// link writes the text err itself contributes and returns the error to
// continue with, or nil where the text ends. A suffix err writes after the
// error it wraps goes on the walk's suffixes.
func (w *walk) link(err error) error {
	b := w.b
	switch e := err.(type) { //nolint:errorlint // each link of the chain is read in turn, by its own type
	case *DecodeError:
		if e == nil {
			b.WriteString("decode: unspecified error")
			return nil
		}
		e.writeNode(b)
		if e.Inner == nil {
			b.WriteString("unspecified error")
		}
		w.below, w.none = nil, false
		return e.Inner
	case *shapeError:
		if e != nil {
			if e.rmType != "" {
				b.WriteString("canjson: ")
				b.WriteString(e.rmType)
				b.WriteString(": ")
			}
			return e.cause
		}
	case *refused.Error:
		if e != nil {
			// The carrier's message is its refusal's; the literal it holds is
			// never written.
			return e.Unwrap()
		}
	case *json.SemanticError:
		if e != nil {
			return writeSemantic(b, e)
		}
	case *jsontext.SyntacticError:
		if e != nil {
			writeSyntactic(b, e, false)
			return nil
		}
	case *xml.SyntaxError:
		if e != nil {
			// The line is structure, like a SyntacticError's offset; Msg can
			// quote the document's text.
			b.WriteString("XML syntax error on line ")
			b.WriteString(strconv.Itoa(e.Line))
			return nil
		}
	case *strconv.NumError:
		if e != nil {
			// Num holds the literal; the function and the cause name the class.
			b.WriteString("strconv.")
			if isIdentifier(e.Func) {
				b.WriteString(e.Func)
			}
			if e.Err == nil {
				return nil
			}
			b.WriteString(": ")
			return e.Err
		}
	case base64.CorruptInputError:
		// The text names the offset of the first bad byte, not the byte.
		b.WriteString(e.Error())
		return nil
	}
	if isKnownSentinel(err) {
		b.WriteString(err.Error())
		return nil
	}
	causes, inner := unwrap(err)
	if (inner != nil || len(causes) > 0) && w.decodeErrorBelow(err) {
		// The text of an error above a DecodeError holds that DecodeError's
		// text. Asking for it would write the chain below once more, at every
		// such error, so the error's own text is left out and what it wraps is
		// written from its fields.
		if inner != nil {
			return inner
		}
		w.writeCauses("", causes)
		return nil
	}
	text, ok := errorText(err)
	if !ok {
		writeClass(b, err)
		return nil
	}
	if wholeForms.MatchString(text) {
		b.WriteString(text)
		return nil
	}
	switch {
	case inner != nil:
		prefix, suffix := ownText(text, inner)
		b.WriteString(prefix)
		if suffix != "" {
			w.suffixes = append(w.suffixes, suffix)
		}
		return inner
	case len(causes) > 0:
		w.writeCauses(text, causes)
		return nil
	}
	writeClass(b, err)
	return nil
}

// decodeErrorBelow reports whether a DecodeError lies beneath err, which wraps
// one error or several. The chain below is searched once between two
// DecodeErrors: what the search finds stays true for every link down to the
// DecodeError it found, and a chain with none below a link has none below the
// links under it either.
func (w *walk) decodeErrorBelow(err error) bool {
	if w.below == nil && !w.none {
		w.below = findDecodeError(err)
		w.none = w.below == nil
	}
	return w.below != nil
}

// findDecodeError returns the first non-nil DecodeError beneath err, depth
// first as errors.As searches, or nil when there is none.
func findDecodeError(err error) *DecodeError {
	for {
		causes, inner := unwrap(err)
		if inner == nil {
			for _, c := range causes {
				if de := atOrBelow(c); de != nil {
					return de
				}
			}
			return nil
		}
		if de, ok := inner.(*DecodeError); ok && de != nil { //nolint:errorlint // each link is looked at in turn
			return de
		}
		err = inner
	}
}

// atOrBelow returns err when it is a non-nil DecodeError, and otherwise the
// first one beneath it.
func atOrBelow(err error) *DecodeError {
	if de, ok := err.(*DecodeError); ok && de != nil { //nolint:errorlint // each link is looked at in turn
		return de
	}
	return findDecodeError(err)
}

// unwrap returns the errors err wraps when it wraps several, or the error it
// wraps. Both are empty when err wraps nothing or its Unwrap method panics,
// as a method on a nil pointer can.
func unwrap(err error) (causes []error, inner error) {
	defer func() {
		if recover() != nil {
			causes, inner = nil, nil
		}
	}()
	switch u := err.(type) { //nolint:errorlint // the wrapper itself is read, not what it wraps
	case interface{ Unwrap() error }:
		return nil, u.Unwrap()
	case interface{ Unwrap() []error }:
		return u.Unwrap(), nil
	}
	return nil, nil
}

// writeCauses writes causes, the errors an error wraps when it wraps several,
// on one line. text is the wrapper's own message, or "" when it is not read.
// The parts of text around the causes' own texts are kept where they match a
// form the SDK's codecs write, before the first cause, between two causes or
// after the last; any other separator is written as "; ", never as the line
// break errors.Join puts between its causes. Each cause's chain is written by
// a walk of its own, which knows what this walk knows of the chains below.
func (w *walk) writeCauses(text string, causes []error) {
	if slices.Contains(causes, nil) {
		causes = slices.DeleteFunc(slices.Clone(causes), func(c error) bool { return c == nil })
	}
	parts := splitAround(text, causes)
	for i, c := range causes {
		var part string
		if parts != nil {
			part = parts[i]
		}
		switch {
		case i == 0:
			if prefixForms.MatchString(part) {
				w.b.WriteString(part)
			}
		case betweenForms.MatchString(part):
			w.b.WriteString(part)
		default:
			w.b.WriteString("; ")
		}
		cause := walk{b: w.b, none: w.none}
		cause.run(c)
	}
	if parts != nil && suffixForms.MatchString(parts[len(causes)]) {
		w.b.WriteString(parts[len(causes)])
	}
}

// splitAround splits text at the causes' own texts, found in order, into the
// len(causes)+1 parts around them. It returns nil when text is empty or does
// not hold the causes' texts in order.
func splitAround(text string, causes []error) []string {
	if text == "" {
		return nil
	}
	parts := make([]string, 0, len(causes)+1)
	rest := text
	for _, c := range causes {
		causeText, ok := errorText(c)
		if !ok {
			return nil
		}
		before, after, found := strings.Cut(rest, causeText)
		if !found {
			return nil
		}
		parts = append(parts, before)
		rest = after
	}
	return append(parts, rest)
}

// writeClass names err by its class alone: its Go type, which fmt reports
// without calling a method of err.
func writeClass(b *strings.Builder, err error) {
	fmt.Fprintf(b, "cause withheld (%T)", err)
}

// writeSemantic writes a SemanticError without the JSON value it holds and
// returns its cause, the error to continue with.
//
// json/v2 leaves out a SemanticError's own position where a SyntacticError
// beneath it gives that position, and trims the "jsontext: " prefix from the
// SyntacticError's text; this does the same, so the text reads as json/v2
// writes it, less the value.
func writeSemantic(b *strings.Builder, se *json.SemanticError) error {
	head := *se
	head.JSONValue = nil
	if se.Err == json.ErrUnknownName { //nolint:errorlint // json/v2's own form for it names the member, which is structure
		b.WriteString(head.Error())
		return nil
	}
	head.Err = nil
	sy, _ := se.Err.(*jsontext.SyntacticError) //nolint:errorlint // json/v2 reads only a SyntacticError held directly
	if sy != nil {
		switch {
		case head.JSONPointer != "":
			if head.JSONPointer.Contains(sy.JSONPointer) {
				head.JSONPointer, head.ByteOffset = "", 0
			}
		case head.ByteOffset > 0 && head.ByteOffset <= sy.ByteOffset:
			head.ByteOffset = 0
		}
	}
	b.WriteString(head.Error())
	if se.Err == nil {
		return nil
	}
	b.WriteString(": ")
	if sy != nil {
		writeSyntactic(b, sy, true)
		return nil
	}
	return se.Err
}

// writeSyntactic writes a SyntacticError with its cause named by class: the
// pointer and byte offset stay, and a refused character or escape sequence is
// left out. underSemantic trims the "jsontext: " prefix, as json/v2 does for
// a SyntacticError beneath a SemanticError.
func writeSyntactic(b *strings.Builder, sy *jsontext.SyntacticError, underSemantic bool) {
	named := *sy
	named.Err = syntacticClass(sy.Err)
	text := named.Error()
	if underSemantic {
		text = strings.TrimPrefix(text, "jsontext: ")
	}
	b.WriteString(text)
}

// syntacticClasses name the classes of jsontext's own syntactic refusals.
// Each is matched against the start of a refusal's text and written in its
// place, so whatever the refusal quotes after it is left out.
var syntacticClasses = []error{
	errors.New("invalid character"),
	errors.New("invalid escape sequence"),
	errors.New("invalid surrogate pair"),
	errors.New("invalid UTF-8"),
	errors.New("exceeded max depth"),
	errors.New("missing value after object name"),
	errors.New("mismatching structural token for object or array"),
}

// syntacticClass returns the error a SyntacticError's cause is written as:
// itself when it is a sentinel that holds no value (a duplicate member name,
// whose name is structure, a name that is not a string, a truncated input),
// its class from syntacticClasses, or nil, which jsontext writes as
// "syntactic error".
func syntacticClass(err error) error {
	switch err {
	case nil, jsontext.ErrDuplicateName, jsontext.ErrNonStringName, io.ErrUnexpectedEOF: //nolint:errorlint // jsontext holds these sentinels directly
		return err
	}
	text, ok := errorText(err)
	if !ok {
		return nil
	}
	for _, class := range syntacticClasses {
		if c := class.Error(); text == c || strings.HasPrefix(text, c+" ") {
			return class
		}
	}
	return nil
}

// isKnownSentinel reports whether err is a sentinel whose fixed text holds no
// value: one of this package's, or one of the standard library's that the
// decode reaches.
func isKnownSentinel(err error) bool {
	switch err {
	case ErrMissingType, ErrUnknownType, ErrTypeMismatch, ErrMaxDepthExceeded, //nolint:errorlint // identity is the point: a sentinel's text is fixed
		ErrNilReceiver, ErrNilArgument, ErrInvalidShape,
		jsontext.ErrDuplicateName, jsontext.ErrNonStringName, json.ErrUnknownName,
		strconv.ErrSyntax, strconv.ErrRange, io.EOF, io.ErrUnexpectedEOF, errors.ErrUnsupported:
		return true
	}
	return false
}

// errorText returns err's text, and false when its Error method panics, as a
// method on a nil pointer can.
func errorText(err error) (text string, ok bool) {
	defer func() {
		if recover() != nil {
			text, ok = "", false
		}
	}()
	return err.Error(), true
}

// ownText splits text, the message of a wrapper around inner, into the text
// the wrapper writes before and after inner's, and returns each part that
// matches a form the SDK's codecs write, or "" for one that does not.
func ownText(text string, inner error) (prefix, suffix string) {
	innerText, ok := errorText(inner)
	if !ok || innerText == "" {
		return "", ""
	}
	prefix, suffix, found := strings.CutLast(text, innerText)
	if !found {
		return "", ""
	}
	if !prefixForms.MatchString(prefix) {
		prefix = ""
	}
	if !suffixForms.MatchString(suffix) {
		suffix = ""
	}
	return prefix, suffix
}

// The variable parts a form admits. Each is structure: a Go-quoted `_type`
// value, RM type name or member pointer; an identifier; a Go type name. A
// quoted part admits only what strconv.Quote writes, printable characters and
// backslash escapes, so a text that holds a raw line break between quotes is
// not in a form.
const (
	formQuoted = `"(?:[^"\\\pC\pZ]| |\\[!-~])*"`
	formIdent  = `[A-Za-z_][A-Za-z0-9_]*`
	formGoType = `[*\[\]A-Za-z0-9_./]+`
)

// prefixForms are the texts the SDK's codecs write before the error they
// wrap, betweenForms those they write between two errors they wrap together,
// suffixForms those they write after it, and wholeForms the whole messages of
// the errors they raise that the renderer does not know by type. A wrapper's
// own text, or a message, outside these forms is left out.
var (
	prefixForms = anchoredForms(
		`(?:canjson|canxml): `,
		`canjson: `+formIdent+`: `,
		`canjson: expected `+formQuoted+`, got `+formQuoted+`: `,
		`canxml: `+formQuoted+`: `,
		`canxml: <`+formIdent+`>: `,
		`canxml: decoded `+formGoType+`: `,
		`rm\.(?:Integer|Real|Character): (?:parse quoted literal: )?`,
		`typereg\.Decode: (?:read _type: )?`,
		`typereg\.Decode `+formQuoted+`: `,
		`typereg\.DecodeAs: decoded `+formGoType+`: `,
		`typereg\.(?:DecodeInto|DecodePolymorphic): `,
	)
	betweenForms = anchoredForms(
		`: `,
		// canonical XML's interval readers, between canxml's shape sentinel
		// and the refusal of a side written bounded with no bound. The RM
		// type names are listed: a word between two causes may be a value.
		`: (?:DV_INTERVAL|Interval|Point_interval|Proper_interval): `,
	)
	suffixForms = anchoredForms(
		``,
		` \([0-9]+ > [0-9]+\)`,
		`: `+formIdent+` for (?:`+formGoType+`)?`,
		`: xmi:type on the wire is rejected \(use xsi:type\)`,
	)
	wholeForms = anchoredForms(
		// canxml's own shape sentinel, a value this package cannot name.
		`canxml: invalid XML shape`,
		`(?:lower|upper)_unbounded is false but the interval gives no (?:lower|upper) bound, which a bound held by value cannot leave out`,
		`rm\.(?:Integer|Real|Character): empty input`,
		`rm\.Real: literal carries more than [0-9]+ significant decimal digits`,
		`value is not valid UTF-8`,
		`must be exactly one character, got [0-9]+`,
		`rm\.Character: number is not a usable code point`,
		`canjson: unknown member(?: `+formQuoted+`)?`,
	)
)

// anchoredForms compiles forms into one expression that matches a whole text
// equal to one of them.
func anchoredForms(forms ...string) *regexp.Regexp {
	return regexp.MustCompile(`^(?:` + strings.Join(forms, `|`) + `)$`)
}

// isIdentifier reports whether s is a non-empty run of ASCII letters and
// digits, as a strconv function name is.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range []byte(s) {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

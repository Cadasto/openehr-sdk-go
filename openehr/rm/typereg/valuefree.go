package typereg

// Value-free text of a DecodeError (REQ-052). The text of a decode error
// travels to places a caller does not choose for clinical data, a log line or
// an HTTP error body, so [DecodeError.Error] writes no value from the
// document. It writes text from three sources only:
//
//   - Structure it reads off a field it knows: a DecodeError's Path and Type,
//     the RM type a shape failure names, a SemanticError's JSON kind, Go type
//     and pointer, a SyntacticError's pointer and byte offset, a NumError's
//     function name. The fields that hold the value, a SemanticError's
//     JSONValue, the text a SyntacticError's cause quotes and a NumError's
//     Num, are never written.
//   - An error it knows by identity: a sentinel whose text is fixed.
//   - A message the SDK's codecs write, matched against a closed list of
//     forms, whole or as a wrapper's own text around the error it wraps.
//     Every variable part of a form is structure: an RM or Go type name, a
//     quoted `_type` value or member pointer, or a count.
//
// Any other text is left out. A wrapper it cannot place is followed to the
// error it wraps, and any other error is named by its class, its Go type,
// alone. So the guarantee does not rest on what an unknown error happens to
// say: text is kept only where it is known to hold no value. The causes
// themselves are untouched and stay reachable through Unwrap, where their
// own text may name the value.

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/refused"
)

// writeValueFree writes the value-free text of err into b. It follows the
// chain one link at a time, so its cost grows with the length of the text
// rather than with the square of the chain's length.
func writeValueFree(b *strings.Builder, err error) {
	var suffixes []string // text wrappers write after what they wrap, outermost first
	for err != nil {
		err = writeLink(b, err, &suffixes)
	}
	for _, suffix := range slices.Backward(suffixes) {
		b.WriteString(suffix)
	}
}

// writeLink writes the text err itself contributes and returns the error to
// continue with, or nil where the text ends. A suffix err writes after the
// error it wraps goes on suffixes.
func writeLink(b *strings.Builder, err error, suffixes *[]string) error {
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
	}
	if isKnownSentinel(err) {
		b.WriteString(err.Error())
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
	switch w := err.(type) { //nolint:errorlint // the wrapper itself is read, not what it wraps
	case interface{ Unwrap() error }:
		inner := w.Unwrap()
		if inner == nil {
			break
		}
		prefix, suffix := ownText(text, inner)
		b.WriteString(prefix)
		if suffix != "" {
			*suffixes = append(*suffixes, suffix)
		}
		return inner
	case interface{ Unwrap() []error }:
		for i, inner := range w.Unwrap() {
			if i > 0 {
				b.WriteByte('\n') // as errors.Join separates its errors
			}
			writeValueFree(b, inner)
		}
		return nil
	}
	writeClass(b, err)
	return nil
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
// value, RM type name or member pointer; an identifier; a Go type name.
const (
	formQuoted = `"(?:[^"\\]|\\.)*"`
	formIdent  = `[A-Za-z_][A-Za-z0-9_]*`
	formGoType = `[*\[\]A-Za-z0-9_./]+`
)

// prefixForms are the texts the SDK's codecs write before the error they
// wrap, suffixForms those they write after it, and wholeForms the whole
// messages of the errors they raise that the renderer does not know by type.
// A wrapper's own text, or a message, outside these forms is left out.
var (
	prefixForms = anchoredForms(
		`(?:canjson|canxml): `,
		`canjson: `+formIdent+`: `,
		`canjson: expected `+formQuoted+`, got `+formQuoted+`: `,
		`canxml: `+formQuoted+`: `,
		`canxml: decoded `+formGoType+`: `,
		`rm\.(?:Integer|Real|Character): (?:parse quoted literal: )?`,
		`typereg\.Decode: (?:read _type: )?`,
		`typereg\.Decode `+formQuoted+`: `,
		`typereg\.DecodeAs: decoded `+formGoType+`: `,
		`typereg\.(?:DecodeInto|DecodePolymorphic): `,
	)
	suffixForms = anchoredForms(
		``,
		` \([0-9]+ > [0-9]+\)`,
		`: `+formIdent+` for (?:`+formGoType+`)?`,
	)
	wholeForms = anchoredForms(
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

// maxPathText is the most bytes of a path the text writes whole, the bound
// encoding/json/v2 puts on a pointer in its own messages.
const maxPathText = 100

// shortPath returns path as the text writes it: whole when it is at most
// maxPathText bytes, and otherwise its first and last segments around an
// ellipsis, the way encoding/json/v2 shortens a pointer (its unexported
// jsonwire.TruncatePointer). A document can make a path as long as it likes,
// through its nesting or its member names; the Path field keeps it whole.
func shortPath(s string) string {
	const n = maxPathText
	if len(s) <= n {
		return s
	}
	i, j := n/2, len(s)-n/2
	// Keep whole segments where there are several.
	if k := strings.LastIndexByte(s[:i], '/'); k > 0 {
		i = k
	}
	if k := strings.IndexByte(s[j:], '/'); k >= 0 {
		j += k + len("/")
	}
	// Never cut a UTF-8 sequence in two.
	for i > 0 {
		if r, size := utf8.DecodeLastRuneInString(s[:i]); r != utf8.RuneError || size != 1 {
			break
		}
		i--
	}
	for j < len(s) {
		if r, size := utf8.DecodeRuneInString(s[j:]); r != utf8.RuneError || size != 1 {
			break
		}
		j++
	}
	var middle string
	switch strings.Count(s[i:j], "/") {
	case 0:
		middle = "…"
	case 1:
		middle = "…/…"
	default:
		middle = "…/…/…"
	}
	if strings.HasPrefix(s[i:j], "/") && middle != "…" {
		middle = strings.TrimPrefix(middle, "…")
	}
	if strings.HasSuffix(s[i:j], "/") && middle != "…" {
		middle = strings.TrimSuffix(middle, "…")
	}
	return s[:i] + middle + s[j:]
}

package typereg_test

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/refused"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestDecodeError_REQ052_TextCarriesNoValue pins how DecodeError.Error renders
// its cause (REQ-052): text from a cause it knows holds no value is kept, the
// value-bearing parts of the standard library's errors are left out, and any
// other text is named by its class alone. Each case gives the cause, the text
// that must stay, and the values that must not appear.
//
// Can-fail control: render the cause with its own Error(), as before, and
// every case that lists a value fails.
func TestDecodeError_REQ052_TextCarriesNoValue(t *testing.T) {
	numErr := &strconv.NumError{Func: "ParseFloat", Num: "3.5", Err: strconv.ErrSyntax}
	cases := []struct {
		name   string
		cause  error
		keep   []string
		absent []string
	}{
		{
			name:   "a cause of a type it does not know",
			cause:  errors.New("Jane 3.5"),
			keep:   []string{"cause withheld (*errors.errorString)"},
			absent: []string{"Jane", "3.5"},
		},
		{
			name:   "a wrapper of a type it does not know, around a sentinel",
			cause:  fmt.Errorf("bad value %q: %w", "Jane", typereg.ErrUnknownType),
			keep:   []string{"decode /a: typereg: _type not in registry"},
			absent: []string{"Jane", "bad value"},
		},
		{
			name: "a SemanticError holding the literal",
			cause: &json.SemanticError{
				JSONPointer: "/magnitude", JSONKind: '0', JSONValue: jsontext.Value("3.5"),
				GoType: reflect.TypeFor[int64](), Err: strconv.ErrSyntax,
			},
			keep:   []string{"JSON number with Go int64 within \"/magnitude\": invalid syntax"},
			absent: []string{"3.5"},
		},
		{
			name:   "a SyntacticError naming a refused character",
			cause:  &jsontext.SyntacticError{JSONPointer: "/value", ByteOffset: 27, Err: errors.New("invalid character 'J' at start of value")},
			keep:   []string{`jsontext: invalid character within "/value" after offset 27`},
			absent: []string{"'J'", "start of value"},
		},
		{
			name:   "a SyntacticError naming an escape sequence",
			cause:  &jsontext.SyntacticError{JSONPointer: "/value", ByteOffset: 33, Err: errors.New("invalid escape sequence `\\q` in string")},
			keep:   []string{`jsontext: invalid escape sequence within "/value" after offset 33`},
			absent: []string{`\q`},
		},
		{
			name:   "a SyntacticError of a class it does not know",
			cause:  &jsontext.SyntacticError{JSONPointer: "/value", ByteOffset: 9, Err: errors.New("bad Jane")},
			keep:   []string{`jsontext: syntactic error within "/value" after offset 9`},
			absent: []string{"Jane"},
		},
		{
			name:   "a SyntacticError of a truncated input",
			cause:  &jsontext.SyntacticError{ByteOffset: 30, Err: io.ErrUnexpectedEOF},
			keep:   []string{"jsontext: unexpected EOF after offset 30"},
			absent: nil,
		},
		{
			name:   "a NumError",
			cause:  numErr,
			keep:   []string{"decode /a: strconv.ParseFloat: invalid syntax"},
			absent: []string{"3.5", "parsing"},
		},
		{
			name:   "an SDK codec's prefix",
			cause:  fmt.Errorf("rm.Real: parse quoted literal: %w", numErr),
			keep:   []string{"decode /a: rm.Real: parse quoted literal: strconv.ParseFloat: invalid syntax"},
			absent: []string{"3.5"},
		},
		{
			name:   "a prefix that is not an SDK codec's",
			cause:  fmt.Errorf("rm.Real: parse %s: %w", "3.5", numErr),
			keep:   []string{"decode /a: strconv.ParseFloat: invalid syntax"},
			absent: []string{"3.5", "rm.Real"},
		},
		{
			name:   "a suffix that is not an SDK codec's, after a sentinel",
			cause:  fmt.Errorf("%w: got %s", typereg.ErrUnknownType, "Jane"),
			keep:   []string{"decode /a: typereg: _type not in registry"},
			absent: []string{"Jane"},
		},
		{
			name:   "an SDK codec's message",
			cause:  errors.New("rm.Real: literal carries more than 17 significant decimal digits"),
			keep:   []string{"decode /a: rm.Real: literal carries more than 17 significant decimal digits"},
			absent: nil,
		},
		{
			name:   "a message that only begins like an SDK codec's",
			cause:  errors.New("rm.Real: literal carries more than 17 significant decimal digits: 3.5"),
			keep:   []string{"cause withheld"},
			absent: []string{"3.5", "rm.Real"},
		},
		{
			name:   "a refused literal",
			cause:  refused.Literal([]byte(`"3.5"`), fmt.Errorf("rm.Real: parse quoted literal: %w", numErr)),
			keep:   []string{"decode /a: rm.Real: parse quoted literal: strconv.ParseFloat: invalid syntax"},
			absent: []string{"3.5"},
		},
		{
			name: "a slot's envelope beneath a SemanticError",
			cause: &json.SemanticError{GoType: reflect.TypeFor[int](), Err: &typereg.DecodeError{
				Path: "/value", Type: "Jane Doe", Inner: fmt.Errorf("typereg.Decode %q: %w", "Jane Doe", typereg.ErrUnknownType),
			}},
			keep:   []string{`decode /value (_type="Jane Doe"): typereg.Decode "Jane Doe": typereg: _type not in registry`},
			absent: nil,
		},
		{
			name:   "a depth refusal",
			cause:  fmt.Errorf("typereg.Decode: %w (%d > %d)", typereg.ErrMaxDepthExceeded, 513, 512),
			keep:   []string{"decode /a: typereg.Decode: typereg: nesting depth exceeds limit (513 > 512)"},
			absent: nil,
		},
		{
			name:   "several causes joined",
			cause:  errors.Join(typereg.ErrMissingType, errors.New("Jane")),
			keep:   []string{"typereg: _type discriminator missing\ncause withheld"},
			absent: []string{"Jane"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &typereg.DecodeError{Path: "/a", Inner: tc.cause}
			got := e.Error()
			for _, k := range tc.keep {
				if !strings.Contains(got, k) {
					t.Errorf("Error() = %q; want it to keep %q", got, k)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(got, a) {
					t.Errorf("Error() = %q; it carries %q", got, a)
				}
			}
			if !errors.Is(e, tc.cause) {
				t.Errorf("errors.Is(DecodeError, its cause) = false; the cause must stay reachable")
			}
		})
	}
}

// TestDecodeError_REQ052_PathCannotSplitTheText pins how the text writes a
// path that holds a character a log line cannot carry as it is: a member name
// is structure and stays in the text, but Go-quoted, so a control character, a
// line separator or invalid UTF-8 in it cannot start a new line or forge one.
// A path with no such character is written as it is.
//
// Can-fail control: write Path unquoted and the control characters reach the
// text raw.
func TestDecodeError_REQ052_PathCannotSplitTheText(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		written string // the path as the text writes it
	}{
		{name: "a line feed", path: "/x\nERROR forged line", written: `"/x\nERROR forged line"`},
		{name: "a carriage return", path: "/x\rERROR", written: `"/x\rERROR"`},
		{name: "a line separator", path: "/x\u2028ERROR", written: `"/x\u2028ERROR"`},
		{name: "invalid UTF-8", path: "/x\xffERROR", written: `"/x\xffERROR"`},
		{name: "a quote", path: `/x"ERROR`, written: `"/x\"ERROR"`},
		{name: "an ordinary path", path: "/content/0/Jane Doe ✓", written: "/content/0/Jane Doe ✓"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, typed := range []bool{false, true} {
				e := &typereg.DecodeError{Path: tc.path, Inner: typereg.ErrUnknownType}
				head := "decode " + tc.written + ": "
				if typed {
					e.Type = "DV_TEXT"
					head = "decode " + tc.written + ` (_type="DV_TEXT"): `
				}
				got := e.Error()
				if !strings.HasPrefix(got, head) {
					t.Errorf("DecodeError{Path: %q, Type: %q}.Error() = %q; want it to open with %q", tc.path, e.Type, got, head)
				}
				if strings.ContainsAny(got, "\n\r\u2028") || !utf8.ValidString(got) {
					t.Errorf("DecodeError{Path: %q, Type: %q}.Error() = %q; want no line break and no invalid UTF-8 in the text", tc.path, e.Type, got)
				}
			}
		})
	}
}

// TestDecodeError_REQ052_LongPathIsWrittenWhole pins that the text names the
// failing value's JSON Pointer whole, however long a document makes it through
// its nesting or its member names: REQ-052 says the text must name the
// pointer, and a part of it does not. A long pointer that holds a character
// strconv.Quote escapes is written whole too, Go-quoted.
//
// Can-fail control: shorten a long path in the text, as an earlier version
// did around an ellipsis, and the equality assertions fail.
func TestDecodeError_REQ052_LongPathIsWrittenWhole(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		written string // the path as the text writes it
	}{
		{name: "a deep pointer", path: strings.Repeat("/folders/0", 300), written: strings.Repeat("/folders/0", 300)},
		{name: "a long member name", path: "/" + strings.Repeat("x", 4096), written: "/" + strings.Repeat("x", 4096)},
		{
			name:    "a long pointer with a line feed",
			path:    strings.Repeat("/folders/0", 30) + "/x\nERROR forged line",
			written: strconv.Quote(strings.Repeat("/folders/0", 30) + "/x\nERROR forged line"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &typereg.DecodeError{Path: tc.path, Inner: typereg.ErrMaxDepthExceeded}
			if got, want := e.Error(), "decode "+tc.written+": typereg: nesting depth exceeds limit"; got != want {
				t.Errorf("DecodeError{Path: %d bytes}.Error() is %d bytes, %.80q…; want %d bytes, %.80q…, the pointer whole", len(tc.path), len(got), got, len(want), want)
			}
			if e.Path != tc.path {
				t.Errorf("Path changed to %q; want it as it was set", e.Path)
			}
		})
	}
	short := &typereg.DecodeError{Path: "/content/0/items/1", Inner: typereg.ErrMissingType}
	if got, want := short.Error(), "decode /content/0/items/1: typereg: _type discriminator missing"; got != want {
		t.Errorf("Error() = %q; want %q, a short path written as it is", got, want)
	}
}

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
			cause:  typereg.RefuseLiteral([]byte(`"3.5"`), fmt.Errorf("rm.Real: parse quoted literal: %w", numErr)),
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

// TestDecodeError_REQ052_LongPathIsShortenedInText pins the bound on a path's
// share of the text: a pointer past 100 bytes is shortened there, keeping its
// first and last segments, the way encoding/json/v2 shortens its own, while
// Path keeps it whole. A hostile document can make a pointer as long as it
// likes, through its nesting or its member names.
//
// Can-fail control: write Path whole and the length assertion fails.
func TestDecodeError_REQ052_LongPathIsShortenedInText(t *testing.T) {
	path := strings.Repeat("/folders/0", 300)
	e := &typereg.DecodeError{Path: path, Inner: typereg.ErrMaxDepthExceeded}
	got := e.Error()
	if len(got) > 200 {
		t.Errorf("Error() is %d bytes for a %d-byte path; want the path shortened to about 100", len(got), len(path))
	}
	if !strings.HasPrefix(got, "decode /folders/0") || !strings.Contains(got, "…") || !strings.HasSuffix(got, "/folders/0: typereg: nesting depth exceeds limit") {
		t.Errorf("Error() = %q; want the first and last segments around an ellipsis", got)
	}
	if e.Path != path {
		t.Errorf("Path changed to %q; want it whole", e.Path)
	}
	short := &typereg.DecodeError{Path: "/content/0/items/1", Inner: typereg.ErrMissingType}
	if got, want := short.Error(), "decode /content/0/items/1: typereg: _type discriminator missing"; got != want {
		t.Errorf("Error() = %q; want %q, a short path written as it is", got, want)
	}
}

// TestEntryError_REQ052_EnvelopeAtTheTop pins the envelope a decode entry
// point returns (REQ-052): a *DecodeError at the top of the chain, whose Path
// is the failing value's pointer from the root of the document and whose
// Value holds the literal the decoder could not take, as written; an error
// that is already a *DecodeError keeps its path and gains the literal; and a
// nil error stays nil.
//
// Can-fail control: leave Path or Value unset and the matching rows fail.
func TestEntryError_REQ052_EnvelopeAtTheTop(t *testing.T) {
	numErr := &strconv.NumError{Func: "ParseInt", Num: "1.5", Err: strconv.ErrSyntax}
	slot := &typereg.DecodeError{Path: "/content/0", Type: "ELEMENT", Inner: &json.SemanticError{
		JSONPointer: "/value/magnitude", JSONKind: '0', JSONValue: jsontext.Value("3.5"), Err: strconv.ErrSyntax,
	}}
	cases := []struct {
		name      string
		err       error
		path      string
		value     any
		keepsSelf bool
	}{
		{
			name:  "a SemanticError on a field",
			err:   &json.SemanticError{JSONPointer: "/a/b", JSONKind: '0', JSONValue: jsontext.Value("98765.4321"), Err: strconv.ErrSyntax},
			path:  "/a/b",
			value: "98765.4321",
		},
		{
			name: "a SyntacticError",
			err:  &jsontext.SyntacticError{JSONPointer: "/value", ByteOffset: 27, Err: errors.New("invalid character 'J' at start of value")},
			path: "/value",
		},
		{
			name:  "a refused quoted literal beneath a SemanticError",
			err:   &json.SemanticError{JSONPointer: "/precision", JSONKind: '"', Err: typereg.RefuseLiteral([]byte(`"1.5"`), fmt.Errorf("rm.Integer: parse quoted literal: %w", numErr))},
			path:  "/precision",
			value: `"1.5"`,
		},
		{
			name:  "a slot decoded on its own decoder",
			err:   &json.SemanticError{JSONPointer: "/content", Err: slot},
			path:  "/content/0/value/magnitude",
			value: "3.5",
		},
		{
			name:      "an envelope already at the top",
			err:       slot,
			path:      "/content/0",
			value:     "3.5",
			keepsSelf: true,
		},
		{
			name: "a slot that fell back to its declared type",
			err: &json.SemanticError{JSONPointer: "/name", Err: &typereg.DecodeError{Path: "/name", Inner: &json.SemanticError{
				JSONPointer: "/value", JSONKind: '0', Err: errors.New("cannot take a number"),
			}}},
			path: "/name/value",
		},
		{
			name: "a _type mismatch on a nested value",
			err: &json.SemanticError{JSONPointer: "/lower", Err: &typereg.DecodeError{
				Path: "/_type", Inner: fmt.Errorf("canjson: expected %q, got %q: %w", "DV_QUANTITY", "DV_COUNT", typereg.ErrTypeMismatch),
			}},
			path: "/lower/_type",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := typereg.EntryError(tc.err)
			de, ok := got.(*typereg.DecodeError) //nolint:errorlint // the envelope is the error itself
			if !ok || de == nil {
				t.Fatalf("EntryError(%v) = %T; want a *typereg.DecodeError", tc.err, got)
			}
			if de.Path != tc.path {
				t.Errorf("EntryError(%v).Path = %q; want %q", tc.err, de.Path, tc.path)
			}
			if v := de.Value.Reveal(); v != tc.value {
				t.Errorf("EntryError(%v).Value.Reveal() = %#v; want %#v", tc.err, v, tc.value)
			}
			if !tc.keepsSelf && !errors.Is(got, tc.err) {
				t.Errorf("errors.Is(EntryError(err), err) = false; the error must stay reachable")
			}
			if tc.keepsSelf && de.Inner != slot.Inner { //nolint:errorlint // identity: the envelope keeps its own Inner
				t.Errorf("EntryError(envelope).Inner = %v; want the envelope's own Inner, not a second envelope", de.Inner)
			}
			if lit, ok := tc.value.(string); ok && strings.Contains(got.Error(), lit) {
				t.Errorf("EntryError(%v).Error() = %q; it carries the literal %s", tc.err, got, lit)
			}
		})
	}
	if got := typereg.EntryError(nil); got != nil {
		t.Errorf("EntryError(nil) = %v; want nil", got)
	}
	if slot.Value.Reveal() != nil {
		t.Errorf("EntryError changed the envelope it was given: Value = %#v; want it left as it was", slot.Value.Reveal())
	}
}

// TestRefuseLiteral_REQ052_MessageAndChainUnchanged pins what the literal
// carrier adds to a codec's refusal: nothing a caller reads. Its message is
// the refusal's, one errors.Unwrap step reaches the refusal, and a nil
// refusal stays nil.
func TestRefuseLiteral_REQ052_MessageAndChainUnchanged(t *testing.T) {
	cause := fmt.Errorf("rm.Real: parse quoted literal: %w", &strconv.NumError{Func: "ParseFloat", Num: "x", Err: strconv.ErrSyntax})
	err := typereg.RefuseLiteral([]byte(`"x"`), cause)
	if err.Error() != cause.Error() {
		t.Errorf("RefuseLiteral(...).Error() = %q; want the refusal's own %q", err, cause)
	}
	if got := errors.Unwrap(err); got != cause { //nolint:errorlint // identity: one step reaches the refusal
		t.Errorf("errors.Unwrap(RefuseLiteral(...)) = %v; want the refusal", got)
	}
	if got := typereg.RefuseLiteral([]byte(`"x"`), nil); got != nil {
		t.Errorf("RefuseLiteral(lit, nil) = %v; want nil", got)
	}
}

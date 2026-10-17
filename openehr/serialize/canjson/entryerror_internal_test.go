package canjson

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/refused"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

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
	slot := &DecodeError{Path: "/content/0", Type: "ELEMENT", Inner: &json.SemanticError{
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
			err:   &json.SemanticError{JSONPointer: "/precision", JSONKind: '"', Err: refused.Literal([]byte(`"1.5"`), fmt.Errorf("rm.Integer: parse quoted literal: %w", numErr))},
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
			err: &json.SemanticError{JSONPointer: "/name", Err: &DecodeError{Path: "/name", Inner: &json.SemanticError{
				JSONPointer: "/value", JSONKind: '0', Err: errors.New("cannot take a number"),
			}}},
			path: "/name/value",
		},
		{
			name: "a _type mismatch on a nested value",
			err: &json.SemanticError{JSONPointer: "/lower", Err: &DecodeError{
				Path: "/_type", Inner: fmt.Errorf("canjson: expected %q, got %q: %w", "DV_QUANTITY", "DV_COUNT", typereg.ErrTypeMismatch),
			}},
			path: "/lower/_type",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := entryError(tc.err)
			de, ok := got.(*DecodeError) //nolint:errorlint // the envelope is the error itself
			if !ok || de == nil {
				t.Fatalf("entryError(%v) = %T; want a *DecodeError", tc.err, got)
			}
			if de.Path != tc.path {
				t.Errorf("entryError(%v).Path = %q; want %q", tc.err, de.Path, tc.path)
			}
			if v := de.Value.Reveal(); v != tc.value {
				t.Errorf("entryError(%v).Value.Reveal() = %#v; want %#v", tc.err, v, tc.value)
			}
			if !tc.keepsSelf && !errors.Is(got, tc.err) {
				t.Errorf("errors.Is(entryError(err), err) = false; the error must stay reachable")
			}
			if tc.keepsSelf && de.Inner != slot.Inner { //nolint:errorlint // identity: the envelope keeps its own Inner
				t.Errorf("entryError(envelope).Inner = %v; want the envelope's own Inner, not a second envelope", de.Inner)
			}
			if lit, ok := tc.value.(string); ok && strings.Contains(got.Error(), lit) {
				t.Errorf("entryError(%v).Error() = %q; it carries the literal %s", tc.err, got, lit)
			}
		})
	}
	if got := entryError(nil); got != nil {
		t.Errorf("entryError(nil) = %v; want nil", got)
	}
	if slot.Value.Reveal() != nil {
		t.Errorf("EntryError changed the envelope it was given: Value = %#v; want it left as it was", slot.Value.Reveal())
	}
}

package canjson_test

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// typeSite finds every `_type` member and its value, the slots a mutation
// below can break.
var typeSite = regexp.MustCompile(`"_type"\s*:\s*"([A-Z_]+)"`)

// neverDecoded is a type no RM document holds. A hook set for it decodes
// exactly as the SDK's own, but is a caller's, and the SDK reads every
// polymorphic slot whole under a caller's hooks.
type neverDecoded struct{}

var callerHooks = jsonv2.UnmarshalFromFunc(func(*jsontext.Decoder, *neverDecoded) error { return nil })

// REQ-052 § the decode error contract: a polymorphic slot nested in a slot
// the decode read whole is decoded where it stands, and its failure is the
// one reading it whole gives, with every position the chain records. The
// oracle is the same document decoded under a caller's hook set, where every
// slot is read whole. Each mutation breaks one slot of the largest fixture: an
// unknown `_type`, a `_type` of a class the slot does not take, and a member
// of the wrong JSON kind just inside the slot, so the failures sit at every
// depth of nested slots. A placement that differed from the buffered decode,
// at the failing slot or at any value around it, fails the comparison.
func TestNestedSlotFailuresMatchTheBufferedDecode(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(fixtures.CompositionJSON(benchFixture))
	if err != nil {
		t.Fatalf("read fixture %s: %v", benchFixture, err)
	}
	sites := typeSite.FindAllSubmatchIndex(doc, -1)
	if len(sites) < 100 {
		t.Fatalf("fixture %s holds %d _type members, want at least 100 to reach nested slots", benchFixture, len(sites))
	}
	mutations := []struct {
		name string
		at   func(m []int) (start, end int, with string)
	}{
		{name: "unknown _type", at: func(m []int) (int, int, string) { return m[2], m[3], "NO_SUCH_TYPE" }},
		{name: "_type of another class", at: func(m []int) (int, int, string) { return m[2], m[3], "DV_TEXT" }},
		{name: "wrong kind inside", at: func(m []int) (int, int, string) { return m[1], m[1], `,"name":5` }},
	}
	// Every seventh site keeps the test quick and still reaches each depth.
	for i := 0; i < len(sites); i += 7 {
		for _, mu := range mutations {
			start, end, with := mu.at(sites[i])
			mutant := bytes.Join([][]byte{doc[:start], []byte(with), doc[end:]}, nil)
			var inPlace, buffered rm.Composition
			gotErr := jsonv2.Unmarshal(mutant, &inPlace)
			wantErr := jsonv2.Unmarshal(mutant, &buffered, jsonv2.WithUnmarshalers(callerHooks))
			got, want := outcome(&inPlace, gotErr), outcome(&buffered, wantErr)
			if got != want {
				t.Errorf("site %d (%s at offset %d): in-place decode\n%s\nwant, as read whole\n%s", i, mu.name, start, got, want)
			}
		}
	}
}

// outcome writes what a decode gave: the document it decoded, re-encoded, or
// the failure with every link of its chain and the positions each records.
func outcome(c *rm.Composition, err error) string {
	var b strings.Builder
	if err == nil {
		out, merr := canjson.Marshal(c)
		fmt.Fprintf(&b, "decoded %d bytes, marshal error %v\n", len(out), merr)
		b.Write(out)
		return b.String()
	}
	fmt.Fprintf(&b, "error %q\n", err.Error())
	for err != nil {
		switch e := err.(type) { //nolint:errorlint // each link is written by its own type
		case *jsonv2.SemanticError:
			fmt.Fprintf(&b, "SemanticError pointer=%q offset=%d go=%v\n", e.JSONPointer, e.ByteOffset, e.GoType)
		case *jsontext.SyntacticError:
			fmt.Fprintf(&b, "SyntacticError pointer=%q offset=%d\n", e.JSONPointer, e.ByteOffset)
		case *canjson.DecodeError:
			fmt.Fprintf(&b, "DecodeError path=%q type=%q\n", e.Path, e.Type)
		default:
			fmt.Fprintf(&b, "%s\n", reflect.TypeOf(err))
		}
		u, ok := err.(interface{ Unwrap() error }) //nolint:errorlint // walks the chain link by link
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return b.String()
}

package typereg_test

// unmatched_test.go — REQ-170 and REQ-052 on the typereg side of unmatched
// member tracking. canjson returns a strict refusal on its own, so its tests
// cannot see the error envelopes the refusal crossed. These tests decode
// through TrackUnmatched with bare encoding/json/v2, where those envelopes are
// visible, and pin that no envelope classifies the refusal as a shape failure
// (ErrInvalidShape) or a dispatch failure (*DecodeError), and that
// UnwrapUnknownMember returns the refusal alone.

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"strconv"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestREQ170_StrictRefusalKeepsItsClassification decodes a member the target
// does not read, at the root and inside a polymorphic slot, under strict
// tracking with bare encoding/json/v2.
//
// Can-fail: dropping the ErrUnknownMember pass-through in DecodeInto's
// classifyDecode wraps the refusal in ErrInvalidShape; dropping the one in
// DecodePolymorphic wraps it in a *DecodeError; and an UnwrapUnknownMember
// that returned err unchanged leaves the envelopes on.
func TestREQ170_StrictRefusalKeepsItsClassification(t *testing.T) {
	cases := []struct {
		name, in, pointer string
	}{
		{
			name:    "root",
			in:      `{"_type":"ELEMENT","x-root":"v","archetype_node_id":"at1"}`,
			pointer: "/x-root",
		},
		{
			name:    "inside a polymorphic slot",
			in:      `{"_type":"ELEMENT","archetype_node_id":"at1","value":{"_type":"DV_QUANTITY","magnitude":1,"units":"kg","x-slot":"v"}}`,
			pointer: "/value/x-slot",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var e rm.Element
			err := jsonv2.Unmarshal([]byte(tc.in), typereg.TrackUnmatched(&e, nil, true), typereg.Unmarshalers())
			if !errors.Is(err, typereg.ErrUnknownMember) {
				t.Fatalf("json/v2.Unmarshal(%s, strict tracking) err = %v; want errors.Is(_, typereg.ErrUnknownMember)", tc.in, err)
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; an unknown member must not be classified as a shape failure (REQ-052)", err)
			}
			if de, ok := errors.AsType[*typereg.DecodeError](err); ok && de != nil {
				t.Errorf("err = %v; an unknown member must not arrive inside a *DecodeError, which classifies dispatch failures", err)
			}
			refusal := typereg.UnwrapUnknownMember(err)
			if got := errors.Unwrap(refusal); got != typereg.ErrUnknownMember { //nolint:errorlint // identity by design: the refusal is one step from the sentinel
				t.Errorf("errors.Unwrap(UnwrapUnknownMember(err)) = %v; want typereg.ErrUnknownMember, the refusal alone", got)
			}
			if want := "canjson: unknown member " + strconv.Quote(tc.pointer); refusal.Error() != want {
				t.Errorf("UnwrapUnknownMember(err).Error() = %q; want %q", refusal.Error(), want)
			}
		})
	}
}

// TestUnwrapUnknownMemberLeavesOtherErrors pins that UnwrapUnknownMember
// returns an error without a refusal in its chain unchanged, nil included.
func TestUnwrapUnknownMemberLeavesOtherErrors(t *testing.T) { // REQ-170
	if got := typereg.UnwrapUnknownMember(nil); got != nil {
		t.Errorf("UnwrapUnknownMember(nil) = %v; want nil", got)
	}
	other := errors.New("some other failure")
	if got := typereg.UnwrapUnknownMember(other); got != other { //nolint:errorlint // identity by design: the same error value must come back
		t.Errorf("UnwrapUnknownMember(%v) = %v; want the same error back", other, got)
	}
}

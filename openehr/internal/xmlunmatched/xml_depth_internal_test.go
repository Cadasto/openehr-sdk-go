package xmlunmatched

// REQ-108 (clinical-modeling.md § REQ-108, Canonical-XML decode depth): the
// refusal of an element nested past encoding/xml's own depth limit rests on
// two things encoding/xml does not export, the text of its error and the
// limit, which these tests hold to the encoding/xml the module builds with.

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// nest is a type encoding/xml decodes by itself, as deep as its input nests.
type nest struct {
	Child *nest `xml:"n"`
}

// nestedN returns depth `<n>` elements, each inside the one before.
func nestedN(depth int) []byte {
	return []byte(strings.Repeat("<n>", depth) + strings.Repeat("</n>", depth))
}

// TestREQ108_XMLDepthErrorIsEncodingXMLs pins IsXMLDepthError and
// xmlMaxDepth to encoding/xml: a plain decode of a document as deep as
// xmlMaxDepth succeeds, one element deeper fails with an error IsXMLDepthError
// matches, as it comes and inside the envelopes a polymorphic slot puts
// around it, and IsXMLDepthError matches no other error.
//
// Can-fail control: let encoding/xml change the text of its error, or its
// limit, and the deeper decode is no longer matched, or the shallower fails.
func TestREQ108_XMLDepthErrorIsEncodingXMLs(t *testing.T) {
	if err := xml.Unmarshal(nestedN(xmlMaxDepth()), new(nest)); err != nil {
		t.Fatalf("xml.Unmarshal(%d nested elements) = %v; want nil, encoding/xml's limit being %d", xmlMaxDepth(), err, xmlMaxDepth())
	}
	err := xml.Unmarshal(nestedN(xmlMaxDepth()+1), new(nest))
	if err == nil {
		t.Fatalf("xml.Unmarshal(%d nested elements) = nil; want encoding/xml's refusal for the depth", xmlMaxDepth()+1)
	}
	slot := &typereg.DecodeError{Path: "/n", Type: "N", Inner: fmt.Errorf("canxml: %w", err)}
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "encoding/xml's error", err: err, want: true},
		{name: "inside one slot envelope", err: slot, want: true},
		{name: "inside two slot envelopes", err: &typereg.DecodeError{Path: "/n", Type: "N", Inner: slot}, want: true},
		{name: "nil", err: nil, want: false},
		{name: "another error", err: errors.New("exceeded max length"), want: false},
		{name: "a depth refusal", err: &typereg.DecodeError{Path: "/n[1]", Inner: fmt.Errorf("%w: element depth (2 > 1)", ErrLimitExceeded)}, want: false},
	} {
		if got := IsXMLDepthError(tc.err); got != tc.want {
			t.Errorf("IsXMLDepthError(%s: %.120v) = %t; want %t", tc.name, tc.err, got, tc.want)
		}
	}
}

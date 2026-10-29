package transport_test

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// pinErrorEnvelope is the only error example of the vendored ITS-REST
// overview (resources/its-rest/overview-validation.openapi.yaml), byte for
// byte apart from indentation: a numeric code and the coded list named
// errors, with DV_CODED_TEXT entries.
const pinErrorEnvelope = `{
    "message": "Error message",
    "code": 90000,
    "errors": [
        {
            "_type": "DV_CODED_TEXT",
            "value": "Error message",
            "defining_code": {
                "terminology_id": {"value": "local"},
                "code_string": "9000"
            }
        },
        {
            "_type": "DV_CODED_TEXT",
            "value": "Secondary error message",
            "defining_code": {
                "terminology_id": {"value": "local"},
                "code_string": "8000"
            }
        }
    ]
}`

// codedItem builds the CodedTextItem a test expects.
func codedItem(terminology, code string) transport.CodedTextItem {
	var it transport.CodedTextItem
	it.TerminologyID.Value = terminology
	it.CodeString = code
	return it
}

// TestErrorEnvelopePinErrorsList pins REQ-093 on the pin's own example: the
// coded entries of its errors list reach OpenEHR.CodedText, and Error()
// carries none of their free text.
func TestErrorEnvelopePinErrorsList(t *testing.T) {
	c := newDecodeClient(t, http.StatusUnprocessableEntity, pinErrorEnvelope, nil)
	_, err := c.Do(t.Context(), &transport.Request{Route: "/x", Path: "/x"})
	we, ok := errors.AsType[*transport.WireError](err)
	if !ok || we == nil {
		t.Fatalf("Do() error = %T (%v), want *transport.WireError", err, err)
	}
	if we.OpenEHR == nil {
		t.Fatal("OpenEHR = nil, want the decoded envelope")
	}
	want := []transport.CodedTextItem{codedItem("local", "9000"), codedItem("local", "8000")}
	if !slices.Equal(we.OpenEHR.CodedText, want) {
		t.Errorf("CodedText = %+v, want %+v", we.OpenEHR.CodedText, want)
	}
	for _, text := range []string{"Error message", "Secondary error message"} {
		if strings.Contains(we.Error(), text) {
			t.Errorf("Error() = %q, leaks the free text %q", we.Error(), text)
		}
	}
}

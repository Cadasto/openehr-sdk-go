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

// bothListsEnvelope carries a coded_text list and an errors list, each
// entry with free text that stands in for a patient's details.
const bothListsEnvelope = `{
    "code": "VALIDATION_FAILED",
    "errors": [
        {
            "_type": "DV_CODED_TEXT",
            "value": "errors entry names patient 1234",
            "defining_code": {
                "terminology_id": {"value": "local"},
                "code_string": "9000"
            }
        }
    ],
    "coded_text": [
        {
            "terminology_id": {"value": "openehr"},
            "code_string": "VALIDATION_FAILED",
            "value": "coded_text entry names patient 1234"
        }
    ]
}`

// malformedErrorsEnvelope carries one usable errors entry among entries
// that are no object, an object without a defining_code, a null
// defining_code, a null entry and an entry whose code_string has the wrong
// type.
const malformedErrorsEnvelope = `{
    "code": "VALIDATION_FAILED",
    "errors": [
        42,
        "patient 1234",
        null,
        {"_type": "DV_CODED_TEXT", "value": "no defining code for patient 1234"},
        {"_type": "DV_CODED_TEXT", "value": "null defining code", "defining_code": null},
        {"_type": "DV_CODED_TEXT", "value": "bad code", "defining_code": {"terminology_id": {"value": "local"}, "code_string": 7}},
        {"_type": "DV_CODED_TEXT", "value": "kept", "defining_code": {"terminology_id": {"value": "local"}, "code_string": "9000"}}
    ]
}`

// codedItem builds the CodedTextItem a test expects.
func codedItem(terminology, code, value string) transport.CodedTextItem {
	var it transport.CodedTextItem
	it.TerminologyID.Value = terminology
	it.CodeString = code
	it.Value = value
	return it
}

// TestErrorEnvelopePinErrorsList pins REQ-093 on the pin's own example and
// on the coded lists around it: the entries of the errors list reach
// OpenEHR.CodedText after those of coded_text, their codes stay whatever
// WithRawErrorBodies says, their free-text Value is kept only under that
// opt-in, a malformed errors entry or list is skipped without failing the
// envelope, and Error() carries none of the free text.
func TestErrorEnvelopePinErrorsList(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		raw      bool
		wantCode string
		want     []transport.CodedTextItem
		// leaks lists free text Error() never carries, opt-in or not.
		leaks []string
	}{
		{
			name:     "pin example, default client keeps the codes and drops the values",
			body:     pinErrorEnvelope,
			wantCode: "90000",
			want:     []transport.CodedTextItem{codedItem("local", "9000", ""), codedItem("local", "8000", "")},
			leaks:    []string{"Error message", "Secondary error message"},
		},
		{
			name:     "pin example, raw bodies keep the values",
			body:     pinErrorEnvelope,
			raw:      true,
			wantCode: "90000",
			want: []transport.CodedTextItem{
				codedItem("local", "9000", "Error message"),
				codedItem("local", "8000", "Secondary error message"),
			},
			leaks: []string{"Error message", "Secondary error message"},
		},
		{
			name:     "both lists, coded_text first, raw bodies keep both values",
			body:     bothListsEnvelope,
			raw:      true,
			wantCode: "VALIDATION_FAILED",
			want: []transport.CodedTextItem{
				codedItem("openehr", "VALIDATION_FAILED", "coded_text entry names patient 1234"),
				codedItem("local", "9000", "errors entry names patient 1234"),
			},
			leaks: []string{"1234"},
		},
		{
			name:     "both lists, default client drops both values",
			body:     bothListsEnvelope,
			wantCode: "VALIDATION_FAILED",
			want: []transport.CodedTextItem{
				codedItem("openehr", "VALIDATION_FAILED", ""),
				codedItem("local", "9000", ""),
			},
			leaks: []string{"1234"},
		},
		{
			name:     "malformed errors entries are skipped, the envelope is kept",
			body:     malformedErrorsEnvelope,
			raw:      true,
			wantCode: "VALIDATION_FAILED",
			want:     []transport.CodedTextItem{codedItem("local", "9000", "kept")},
			leaks:    []string{"1234"},
		},
		{
			name:     "an errors member that is not a list is ignored, the envelope is kept",
			body:     `{"code":"VALIDATION_FAILED","errors":{"value":"patient 1234"}}`,
			raw:      true,
			wantCode: "VALIDATION_FAILED",
			leaks:    []string{"1234"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []transport.Option
			if tt.raw {
				opts = append(opts, transport.WithRawErrorBodies(true))
			}
			c := newDecodeClient(t, http.StatusUnprocessableEntity, tt.body, nil, opts...)
			_, err := c.Do(t.Context(), &transport.Request{Route: "/x", Path: "/x"})
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil {
				t.Fatalf("Do() error = %T (%v), want *transport.WireError", err, err)
			}
			if we.OpenEHR == nil {
				t.Fatal("OpenEHR = nil, want the decoded envelope")
			}
			if we.OpenEHR.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", we.OpenEHR.Code, tt.wantCode)
			}
			if !slices.Equal(we.OpenEHR.CodedText, tt.want) {
				t.Errorf("CodedText = %+v, want %+v", we.OpenEHR.CodedText, tt.want)
			}
			for _, text := range tt.leaks {
				if strings.Contains(we.Error(), text) {
					t.Errorf("Error() = %q, leaks the free text %q", we.Error(), text)
				}
			}
		})
	}
}

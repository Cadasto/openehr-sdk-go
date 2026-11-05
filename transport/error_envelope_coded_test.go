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

// errorsOnlyEnvelope carries the pin's errors list and nothing else: no
// message, no code, no validationErrors.
const errorsOnlyEnvelope = `{
    "errors": [
        {
            "_type": "DV_CODED_TEXT",
            "value": "errors entry names patient 1234",
            "defining_code": {
                "terminology_id": {"value": "local"},
                "code_string": "9000"
            }
        }
    ]
}`

// codedTextOnlyEnvelope carries a coded_text list and nothing else.
const codedTextOnlyEnvelope = `{
    "coded_text": [
        {
            "terminology_id": {"value": "openehr"},
            "code_string": "VALIDATION_FAILED",
            "value": "coded_text entry names patient 1234"
        }
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
// envelope, a body that carries only one of the coded lists is still an
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
			name:  "errors list only, default client keeps the code and drops the value",
			body:  errorsOnlyEnvelope,
			want:  []transport.CodedTextItem{codedItem("local", "9000", "")},
			leaks: []string{"1234"},
		},
		{
			name:  "errors list only, raw bodies keep the value",
			body:  errorsOnlyEnvelope,
			raw:   true,
			want:  []transport.CodedTextItem{codedItem("local", "9000", "errors entry names patient 1234")},
			leaks: []string{"1234"},
		},
		{
			name:  "coded_text list only, default client keeps the code and drops the value",
			body:  codedTextOnlyEnvelope,
			want:  []transport.CodedTextItem{codedItem("openehr", "VALIDATION_FAILED", "")},
			leaks: []string{"1234"},
		},
		{
			name:  "coded_text list only, raw bodies keep the value",
			body:  codedTextOnlyEnvelope,
			raw:   true,
			want:  []transport.CodedTextItem{codedItem("openehr", "VALIDATION_FAILED", "coded_text entry names patient 1234")},
			leaks: []string{"1234"},
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

// TestErrorEnvelopeWrongTypedMembersKeepTheEnvelope pins REQ-093's
// best-effort decode member by member: a message, a validationErrors entry,
// a coded_text entry or list, or a coded value of the wrong JSON type is left
// empty or skipped, and the rest of the envelope survives, so the code still
// reaches OpenEHR.Code and Error(). A value that is not a string keeps its
// entry's terminology and code string, under the opt-in or not.
func TestErrorEnvelopeWrongTypedMembersKeepTheEnvelope(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		raw         bool
		wantCode    string
		want        []transport.CodedTextItem
		wantMessage string
		wantVE      []string
	}{
		{
			name:     "coded_text value that is a number keeps the codes, default client",
			body:     `{"code":"VALIDATION_FAILED","coded_text":[{"terminology_id":{"value":"openehr"},"code_string":"C","value":42}]}`,
			wantCode: "VALIDATION_FAILED",
			want:     []transport.CodedTextItem{codedItem("openehr", "C", "")},
		},
		{
			name:     "coded_text value that is a number keeps the codes, raw bodies",
			body:     `{"code":"VALIDATION_FAILED","coded_text":[{"terminology_id":{"value":"openehr"},"code_string":"C","value":42}]}`,
			raw:      true,
			wantCode: "VALIDATION_FAILED",
			want:     []transport.CodedTextItem{codedItem("openehr", "C", "")},
		},
		{
			name:     "coded_text value that is an object keeps the codes, raw bodies",
			body:     `{"code":"VALIDATION_FAILED","coded_text":[{"terminology_id":{"value":"openehr"},"code_string":"C","value":{"text":"patient 1234"}}]}`,
			raw:      true,
			wantCode: "VALIDATION_FAILED",
			want:     []transport.CodedTextItem{codedItem("openehr", "C", "")},
		},
		{
			name:     "errors value that is a number keeps the codes, default client",
			body:     `{"code":"X","errors":[{"_type":"DV_CODED_TEXT","value":5,"defining_code":{"terminology_id":{"value":"local"},"code_string":"9000"}}]}`,
			wantCode: "X",
			want:     []transport.CodedTextItem{codedItem("local", "9000", "")},
		},
		{
			name:     "errors value that is a number keeps the codes, raw bodies",
			body:     `{"code":"X","errors":[{"_type":"DV_CODED_TEXT","value":5,"defining_code":{"terminology_id":{"value":"local"},"code_string":"9000"}}]}`,
			raw:      true,
			wantCode: "X",
			want:     []transport.CodedTextItem{codedItem("local", "9000", "")},
		},
		{
			name:     "message that is an object is left empty, default client",
			body:     `{"code":"X","message":{"text":"m"}}`,
			wantCode: "X",
		},
		{
			name:     "message that is an object is left empty, raw bodies",
			body:     `{"code":"X","message":{"text":"m"}}`,
			raw:      true,
			wantCode: "X",
		},
		{
			name:     "validationErrors entries that are not strings are skipped",
			body:     `{"code":"X","validationErrors":["a",1,"b"]}`,
			raw:      true,
			wantCode: "X",
			wantVE:   []string{"a", "b"},
		},
		{
			name:     "validationErrors entries that are null, objects, lists or booleans are skipped",
			body:     `{"code":"X","validationErrors":["a",null,{"path":"/p"},["n"],true,"b"]}`,
			raw:      true,
			wantCode: "X",
			wantVE:   []string{"a", "b"},
		},
		{
			name:        "validationErrors that is not a list is ignored",
			body:        `{"code":"X","message":"m","validationErrors":"patient 1234"}`,
			raw:         true,
			wantCode:    "X",
			wantMessage: "m",
		},
		{
			name:     "coded_text entry whose code_string is not a string is skipped",
			body:     `{"code":"X","coded_text":[{"terminology_id":{"value":"openehr"},"code_string":7},{"terminology_id":{"value":"openehr"},"code_string":"K"}]}`,
			wantCode: "X",
			want:     []transport.CodedTextItem{codedItem("openehr", "K", "")},
		},
		{
			name:     "coded_text entries that are not objects, or whose terminology is not a string, are skipped",
			body:     `{"code":"X","coded_text":[null,42,"patient 1234",{"terminology_id":{"value":1},"code_string":"Z"},{"terminology_id":"openehr","code_string":"Z"},{"terminology_id":{"value":"openehr"},"code_string":"K"}]}`,
			raw:      true,
			wantCode: "X",
			want:     []transport.CodedTextItem{codedItem("openehr", "K", "")},
		},
		{
			name:     "coded_text entries whose terminology or code_string is null are skipped",
			body:     `{"code":"X","coded_text":[{"terminology_id":{"value":"openehr"},"code_string":null},{"terminology_id":{"value":null},"code_string":"Z"},{"terminology_id":null,"code_string":"Z"},{"terminology_id":{"value":"openehr"},"code_string":"K"}]}`,
			wantCode: "X",
			want:     []transport.CodedTextItem{codedItem("openehr", "K", "")},
		},
		{
			name:     "errors entries whose defining_code holds a null terminology or code_string are skipped",
			body:     `{"code":"X","errors":[{"_type":"DV_CODED_TEXT","value":"v","defining_code":{"terminology_id":{"value":"local"},"code_string":null}},{"_type":"DV_CODED_TEXT","value":"v","defining_code":{"terminology_id":{"value":null},"code_string":"9000"}},{"_type":"DV_CODED_TEXT","value":"v","defining_code":{"terminology_id":null,"code_string":"9000"}},{"_type":"DV_CODED_TEXT","value":"v","defining_code":{"terminology_id":{"value":"local"},"code_string":"8000"}}]}`,
			raw:      true,
			wantCode: "X",
			want:     []transport.CodedTextItem{codedItem("local", "8000", "v")},
		},
		{
			name:     "coded_text that is not a list is ignored, the errors list is kept",
			body:     `{"code":"X","coded_text":{"terminology_id":{"value":"openehr"},"code_string":"K"},"errors":[{"_type":"DV_CODED_TEXT","value":"v","defining_code":{"terminology_id":{"value":"local"},"code_string":"9000"}}]}`,
			raw:      true,
			wantCode: "X",
			want:     []transport.CodedTextItem{codedItem("local", "9000", "v")},
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
				t.Fatalf("OpenEHR = nil for %s, want the decoded envelope", tt.body)
			}
			if we.OpenEHR.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", we.OpenEHR.Code, tt.wantCode)
			}
			if !slices.Equal(we.OpenEHR.CodedText, tt.want) {
				t.Errorf("CodedText = %+v, want %+v", we.OpenEHR.CodedText, tt.want)
			}
			if we.OpenEHR.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", we.OpenEHR.Message, tt.wantMessage)
			}
			if !slices.Equal(we.OpenEHR.ValidationErrors, tt.wantVE) {
				t.Errorf("ValidationErrors = %q, want %q", we.OpenEHR.ValidationErrors, tt.wantVE)
			}
			if wantCode := `code="` + tt.wantCode + `"`; !strings.Contains(we.Error(), wantCode) {
				t.Errorf("Error() = %q, want it to carry %s", we.Error(), wantCode)
			}
			if strings.Contains(we.Error(), "1234") {
				t.Errorf("Error() = %q, leaks free text", we.Error())
			}
		})
	}
}

// TestErrorEnvelopeEmptyCodedListsAreNoEnvelope pins the edge of REQ-093's
// best-effort decode: a body whose only members are coded lists with no
// usable entry carries nothing to attach, so OpenEHR stays nil.
func TestErrorEnvelopeEmptyCodedListsAreNoEnvelope(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty errors list", body: `{"errors":[]}`},
		{name: "empty coded_text list", body: `{"coded_text":[]}`},
		{name: "errors entries that all are skipped", body: `{"errors":[42,{"value":"patient 1234"}]}`},
		{name: "errors member that is not a list", body: `{"errors":{"value":"patient 1234"}}`},
		{name: "coded_text entry whose code_string is null", body: `{"coded_text":[{"terminology_id":{"value":"openehr"},"code_string":null}]}`},
		{name: "errors entry whose terminology is null", body: `{"errors":[{"value":"patient 1234","defining_code":{"terminology_id":{"value":null},"code_string":"9000"}}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newDecodeClient(t, http.StatusUnprocessableEntity, tt.body, nil, transport.WithRawErrorBodies(true))
			_, err := c.Do(t.Context(), &transport.Request{Route: "/x", Path: "/x"})
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil {
				t.Fatalf("Do() error = %T (%v), want *transport.WireError", err, err)
			}
			if we.OpenEHR != nil {
				t.Errorf("OpenEHR = %+v, want nil for a body with no usable coded entry", we.OpenEHR)
			}
		})
	}
}

// TestREQ093ErrorEnvelopeRepeatedMemberIsNoEnvelope pins REQ-093's member
// names: a body that repeats a member name, in the envelope itself or inside
// one of its entries, leaves the envelope undecoded, so WireError.OpenEHR stays
// nil and the status alone classifies the failure. No member wins over its
// repeat.
func TestREQ093ErrorEnvelopeRepeatedMemberIsNoEnvelope(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "repeated message", body: `{"message":"a","message":"b","code":"C"}`},
		{name: "repeated code", body: `{"code":"A","code":"B"}`},
		{name: "repeated validationErrors", body: `{"validationErrors":["a"],"validationErrors":["b"]}`},
		{
			name: "repeated code_string inside a coded_text entry",
			body: `{"code":"X","coded_text":[{"terminology_id":{"value":"openehr"},"code_string":"A","code_string":"B"}]}`,
		},
		{
			name: "repeated code_string inside an errors entry's defining_code",
			body: `{"code":"X","errors":[{"value":"v","defining_code":{"terminology_id":{"value":"local"},"code_string":"9000","code_string":"8000"}}]}`,
		},
		{
			name: "repeated name inside a member the envelope does not read",
			body: `{"code":"X","details":{"a":1,"a":2}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newDecodeClient(t, http.StatusUnprocessableEntity, tt.body, nil, transport.WithRawErrorBodies(true))
			_, err := c.Do(t.Context(), &transport.Request{Route: "/x", Path: "/x"})
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil {
				t.Fatalf("Do() error = %T (%v), want *transport.WireError", err, err)
			}
			if we.OpenEHR != nil {
				t.Errorf("OpenEHR = %+v for %s, want nil for a body that repeats a member name", we.OpenEHR, tt.body)
			}
			if !errors.Is(err, transport.ErrUnprocessable) {
				t.Errorf("Do() error = %v, want it to match ErrUnprocessable", err)
			}
		})
	}
}

// TestREQ093ErrorEnvelopeMemberNamesMatchExactly pins REQ-093's member names:
// only a member spelled exactly as the envelope names it is read, so a case
// variant such as MESSAGE or Code is ignored like any other unknown member,
// whichever side of the exact name it sits on.
func TestREQ093ErrorEnvelopeMemberNamesMatchExactly(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantEnvelope bool
		wantMessage  string
		wantCode     string
		want         []transport.CodedTextItem
		wantVE       []string
	}{
		{
			name:         "a case variant after the exact name does not replace it",
			body:         `{"message":"y","MESSAGE":"x","code":"C"}`,
			wantEnvelope: true,
			wantMessage:  "y",
			wantCode:     "C",
		},
		{
			name:         "a case variant before the exact name does not replace it",
			body:         `{"MESSAGE":"x","message":"y","code":"C"}`,
			wantEnvelope: true,
			wantMessage:  "y",
			wantCode:     "C",
		},
		{
			name:         "case variants alone are ignored, the exact members are kept",
			body:         `{"MESSAGE":"x","Code":"Y","code":"C","ValidationErrors":["v"]}`,
			wantEnvelope: true,
			wantCode:     "C",
		},
		{
			name: "a body with case variants only is no envelope",
			body: `{"Message":"m","Code":"C","VALIDATIONERRORS":["v"]}`,
		},
		{
			name:         "a case variant inside a coded_text entry is ignored",
			body:         `{"code":"X","coded_text":[{"Terminology_ID":{"value":"openehr"},"code_string":"K","Value":"v"}]}`,
			wantEnvelope: true,
			wantCode:     "X",
			want:         []transport.CodedTextItem{codedItem("", "K", "")},
		},
		{
			name:         "a case variant of defining_code leaves the errors entry without one",
			body:         `{"code":"X","errors":[{"value":"v","Defining_Code":{"terminology_id":{"value":"local"},"code_string":"9000"}},{"value":"w","defining_code":{"terminology_id":{"VALUE":"local"},"code_string":"8000"}}]}`,
			wantEnvelope: true,
			wantCode:     "X",
			want:         []transport.CodedTextItem{codedItem("", "8000", "w")},
		},
		{
			name:         "the exact validationErrors is kept beside a case variant",
			body:         `{"validationErrors":["a"],"validationerrors":["b"]}`,
			wantEnvelope: true,
			wantVE:       []string{"a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newDecodeClient(t, http.StatusUnprocessableEntity, tt.body, nil, transport.WithRawErrorBodies(true))
			_, err := c.Do(t.Context(), &transport.Request{Route: "/x", Path: "/x"})
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil {
				t.Fatalf("Do() error = %T (%v), want *transport.WireError", err, err)
			}
			if !tt.wantEnvelope {
				if we.OpenEHR != nil {
					t.Errorf("OpenEHR = %+v for %s, want nil for a body with no exactly named member", we.OpenEHR, tt.body)
				}
				return
			}
			if we.OpenEHR == nil {
				t.Fatalf("OpenEHR = nil for %s, want the decoded envelope", tt.body)
			}
			if we.OpenEHR.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", we.OpenEHR.Message, tt.wantMessage)
			}
			if we.OpenEHR.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", we.OpenEHR.Code, tt.wantCode)
			}
			if !slices.Equal(we.OpenEHR.CodedText, tt.want) {
				t.Errorf("CodedText = %+v, want %+v", we.OpenEHR.CodedText, tt.want)
			}
			if !slices.Equal(we.OpenEHR.ValidationErrors, tt.wantVE) {
				t.Errorf("ValidationErrors = %q, want %q", we.OpenEHR.ValidationErrors, tt.wantVE)
			}
		})
	}
}

// TestREQ093OpenEHRErrorDetailUnmarshalJSONRefusesRepeatedNames pins the
// exported decode method to the same reading as the transport: it fails on a
// repeated member name and ignores a case variant.
func TestREQ093OpenEHRErrorDetailUnmarshalJSONRefusesRepeatedNames(t *testing.T) {
	var d transport.OpenEHRErrorDetail
	if err := d.UnmarshalJSON([]byte(`{"code":"A","code":"B"}`)); err == nil {
		t.Errorf("UnmarshalJSON(repeated code) = nil error, Code %q; want an error", d.Code)
	}
	d = transport.OpenEHRErrorDetail{}
	if err := d.UnmarshalJSON([]byte(`{"code":"A","CODE":"B"}`)); err != nil {
		t.Fatalf("UnmarshalJSON(code and CODE) = %v, want nil", err)
	}
	if d.Code != "A" {
		t.Errorf("UnmarshalJSON(code and CODE): Code = %q, want %q", d.Code, "A")
	}
}

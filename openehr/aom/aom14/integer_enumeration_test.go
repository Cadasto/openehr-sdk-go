package aom14_test

// REQ-176 § Shapes the pinned BMM lacks, the integer-enumeration bullet, and
// REQ-041 § SDK-authored root schemas: OPERATOR_KIND on EXPR_OPERATOR.operator
// and TIMEZONE_VALIDITY_KIND on timezone_validity are written as their integer
// codes, as element text in canonical XML and as a JSON number in canonical
// JSON; decode reads the code or the item name, so prohibited and disallowed
// both read as 1003; any other value is refused in every mode.

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// timeXML is a C_TIME whose timezone_validity element holds text.
func timeXML(text string) string {
	return `<c_time` + nsOpenEHR + `><timezone_validity>` + text + `</timezone_validity></c_time>`
}

// operatorXML is an EXPR_BINARY_OPERATOR whose operator element holds text.
func operatorXML(text string) string {
	return `<expr_binary_operator` + nsOpenEHR + `><type>Boolean</type><operator>` + text +
		`</operator><precedence_overridden>false</precedence_overridden></expr_binary_operator>`
}

// timeJSON is a C_TIME whose timezone_validity member holds the JSON value.
func timeJSON(value string) string {
	return `{"_type":"C_TIME","timezone_validity":` + value + `}`
}

// operatorJSON is an EXPR_BINARY_OPERATOR whose operator member holds the JSON
// value.
func operatorJSON(value string) string {
	return `{"_type":"EXPR_BINARY_OPERATOR","type":"Boolean","operator":` + value + `,"precedence_overridden":false}`
}

// jsonDecodeModes are the three canonical-JSON decode modes.
func jsonDecodeModes() map[string][]canjson.DecoderOption {
	return map[string][]canjson.DecoderOption{
		"default": nil,
		"report":  {canjson.WithUnmatched(func(canjson.Pointer) {})},
		"strict":  {canjson.WithStrict()},
	}
}

// TestREQ176_REQ041_IntegerEnumerationsDecodeTheCodeOrTheItemName pins decode
// in both formats and every mode: the code and the item name of each
// enumeration read as the item, and prohibited and disallowed both read as
// 1003.
func TestREQ176_REQ041_IntegerEnumerationsDecodeTheCodeOrTheItemName(t *testing.T) {
	for _, tc := range []struct {
		text string // the XML text; the JSON value is its number or its quoted name
		want aom14.TimezoneValidityKind
	}{
		{"1001", aom14.TimezoneValidityKindMandatory},
		{"1002", aom14.TimezoneValidityKindOptional},
		{"1003", 1003},
		{"mandatory", aom14.TimezoneValidityKindMandatory},
		{"optional", aom14.TimezoneValidityKindOptional},
		{"prohibited", 1003},
		{"disallowed", 1003},
	} {
		jsonValue := tc.text
		if !strings.HasPrefix(tc.text, "10") {
			jsonValue = `"` + tc.text + `"`
		}
		var reported []string
		for mode, opts := range decodeModes(&reported) {
			var c aom14.CTime
			if err := canxml.Unmarshal([]byte(timeXML(tc.text)), &c, opts...); err != nil {
				t.Errorf("canxml.Unmarshal(timezone_validity %q, %s) = %v; want nil", tc.text, mode, err)
				continue
			}
			if c.TimezoneValidity == nil || *c.TimezoneValidity != tc.want {
				t.Errorf("canxml.Unmarshal(timezone_validity %q, %s) read %v; want %d", tc.text, mode, c.TimezoneValidity, tc.want)
			}
		}
		if len(reported) != 0 {
			t.Errorf("canxml.Unmarshal(timezone_validity %q, report) reported %q; want nothing", tc.text, reported)
		}
		for mode, opts := range jsonDecodeModes() {
			var c aom14.CTime
			if err := canjson.Unmarshal([]byte(timeJSON(jsonValue)), &c, opts...); err != nil {
				t.Errorf("canjson.Unmarshal(timezone_validity %s, %s) = %v; want nil", jsonValue, mode, err)
				continue
			}
			if c.TimezoneValidity == nil || *c.TimezoneValidity != tc.want {
				t.Errorf("canjson.Unmarshal(timezone_validity %s, %s) read %v; want %d", jsonValue, mode, c.TimezoneValidity, tc.want)
			}
		}
	}

	for _, tc := range []struct {
		xmlText, jsonValue string
		want               aom14.OperatorKind
	}{
		{"2007", "2007", aom14.OperatorKindMatches},
		{"matches", `"matches"`, aom14.OperatorKindMatches},
		{"2001", "2001", aom14.OperatorKindEqual},
		{"not_equal", `"not_equal"`, aom14.OperatorKindNotEqual},
		{"2024", "2024", aom14.OperatorKindExponent},
	} {
		var reported []string
		for mode, opts := range decodeModes(&reported) {
			var e aom14.ExprBinaryOperator
			if err := canxml.Unmarshal([]byte(operatorXML(tc.xmlText)), &e, opts...); err != nil {
				t.Errorf("canxml.Unmarshal(operator %q, %s) = %v; want nil", tc.xmlText, mode, err)
			} else if e.Operator != tc.want {
				t.Errorf("canxml.Unmarshal(operator %q, %s) read %d; want %d", tc.xmlText, mode, e.Operator, tc.want)
			}
		}
		for mode, opts := range jsonDecodeModes() {
			var e aom14.ExprBinaryOperator
			if err := canjson.Unmarshal([]byte(operatorJSON(tc.jsonValue)), &e, opts...); err != nil {
				t.Errorf("canjson.Unmarshal(operator %s, %s) = %v; want nil", tc.jsonValue, mode, err)
			} else if e.Operator != tc.want {
				t.Errorf("canjson.Unmarshal(operator %s, %s) read %d; want %d", tc.jsonValue, mode, e.Operator, tc.want)
			}
		}
	}
}

// TestREQ176_REQ041_IntegerEnumerationsEncodeTheCode pins encode in both
// formats: the code, as element text and as a JSON number, whichever name the
// value was read by; so a timezone_validity read as disallowed is written 1003.
func TestREQ176_REQ041_IntegerEnumerationsEncodeTheCode(t *testing.T) {
	var c aom14.CTime
	if err := canxml.Unmarshal([]byte(timeXML("disallowed")), &c); err != nil {
		t.Fatalf("canxml.Unmarshal(timezone_validity disallowed) = %v; want nil", err)
	}
	b, err := canxml.Marshal(&c)
	if err != nil {
		t.Fatalf("canxml.Marshal(C_TIME) = %v; want nil", err)
	}
	if want := "<timezone_validity>1003</timezone_validity>"; !bytes.Contains(b, []byte(want)) {
		t.Errorf("canxml.Marshal(C_TIME read from disallowed) = %s; want it to hold %s", b, want)
	}
	j, err := canjson.Marshal(&c)
	if err != nil {
		t.Fatalf("canjson.Marshal(C_TIME) = %v; want nil", err)
	}
	if want := `"timezone_validity":1003`; !bytes.Contains(j, []byte(want)) {
		t.Errorf("canjson.Marshal(C_TIME read from disallowed) = %s; want it to hold %s", j, want)
	}

	var e aom14.ExprBinaryOperator
	if err := canjson.Unmarshal([]byte(operatorJSON(`"matches"`)), &e); err != nil {
		t.Fatalf("canjson.Unmarshal(operator \"matches\") = %v; want nil", err)
	}
	b, err = canxml.Marshal(&e)
	if err != nil {
		t.Fatalf("canxml.Marshal(EXPR_BINARY_OPERATOR) = %v; want nil", err)
	}
	if want := "<operator>2007</operator>"; !bytes.Contains(b, []byte(want)) {
		t.Errorf("canxml.Marshal(EXPR_BINARY_OPERATOR read from matches) = %s; want it to hold %s", b, want)
	}
	j, err = canjson.Marshal(&e)
	if err != nil {
		t.Fatalf("canjson.Marshal(EXPR_BINARY_OPERATOR) = %v; want nil", err)
	}
	if want := `"operator":2007`; !bytes.Contains(j, []byte(want)) {
		t.Errorf("canjson.Marshal(EXPR_BINARY_OPERATOR read from matches) = %s; want it to hold %s", j, want)
	}
}

// TestREQ176_REQ041_IntegerEnumerationsRefuseOtherValues pins the refusal of a
// value that is neither a code nor an item name of its enumeration, in both
// formats and every mode: with canxml.ErrInvalidShape in XML and
// canjson.ErrInvalidShape in JSON, and with no value in the message.
func TestREQ176_REQ041_IntegerEnumerationsRefuseOtherValues(t *testing.T) {
	for _, text := range []string{"1004", "2007", "0", "Mandatory", "required", "1001.0", ""} {
		var reported []string
		for mode, opts := range decodeModes(&reported) {
			err := canxml.Unmarshal([]byte(timeXML(text)), new(aom14.CTime), opts...)
			if !errors.Is(err, canxml.ErrInvalidShape) {
				t.Errorf("canxml.Unmarshal(timezone_validity %q, %s) = %v; want canxml.ErrInvalidShape", text, mode, err)
			}
		}
	}
	for _, text := range []string{"2008", "1001", "op_matches", "MATCHES", "2007 2007"} {
		var reported []string
		for mode, opts := range decodeModes(&reported) {
			err := canxml.Unmarshal([]byte(operatorXML(text)), new(aom14.ExprBinaryOperator), opts...)
			if !errors.Is(err, canxml.ErrInvalidShape) {
				t.Errorf("canxml.Unmarshal(operator %q, %s) = %v; want canxml.ErrInvalidShape", text, mode, err)
			}
		}
	}
	for _, value := range []string{"1004", `"1001"`, "1001.0", "1.001e3", `"Mandatory"`, "true", "{}", "[1001]"} {
		for mode, opts := range jsonDecodeModes() {
			err := canjson.Unmarshal([]byte(timeJSON(value)), new(aom14.CTime), opts...)
			if !errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("canjson.Unmarshal(timezone_validity %s, %s) = %v; want canjson.ErrInvalidShape", value, mode, err)
			}
		}
	}
	for _, value := range []string{"2008", `"2007"`, `"op_matches"`, "false"} {
		for mode, opts := range jsonDecodeModes() {
			err := canjson.Unmarshal([]byte(operatorJSON(value)), new(aom14.ExprBinaryOperator), opts...)
			if !errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("canjson.Unmarshal(operator %s, %s) = %v; want canjson.ErrInvalidShape", value, mode, err)
			} else if strings.Contains(err.Error(), "2008") || strings.Contains(err.Error(), "op_matches") {
				t.Errorf("canjson.Unmarshal(operator %s, %s) = %q; the message carries the value", value, mode, err)
			}
		}
	}
	err := canxml.Unmarshal([]byte(operatorXML("op_matches")), new(aom14.ExprBinaryOperator))
	if err == nil || strings.Contains(err.Error(), "op_matches") {
		t.Errorf("canxml.Unmarshal(operator op_matches) = %v; want a refusal that does not carry the text", err)
	}
}

// TestREQ176_REQ041_IntegerEnumerationsRefuseToEncodeAValueWithNoCode pins
// that a value that is no code of its enumeration, such as the zero value, is
// an encode error in both formats rather than a document no reader accepts.
func TestREQ176_REQ041_IntegerEnumerationsRefuseToEncodeAValueWithNoCode(t *testing.T) {
	for _, v := range []aom14.OperatorKind{0, 2008} {
		e := &aom14.ExprBinaryOperator{Type: "Boolean", Operator: v}
		if b, err := canxml.Marshal(e); err == nil {
			t.Errorf("canxml.Marshal(operator %d) = %s; want an error", v, b)
		}
		if b, err := canjson.Marshal(e); !errors.Is(err, canjson.ErrInvalidValue) {
			t.Errorf("canjson.Marshal(operator %d) = %s, %v; want canjson.ErrInvalidValue", v, b, err)
		}
	}
	bad := aom14.TimezoneValidityKind(1004)
	c := &aom14.CTime{TimezoneValidity: &bad}
	if b, err := canxml.Marshal(c); err == nil {
		t.Errorf("canxml.Marshal(timezone_validity 1004) = %s; want an error", b)
	}
	if b, err := canjson.Marshal(c); !errors.Is(err, canjson.ErrInvalidValue) {
		t.Errorf("canjson.Marshal(timezone_validity 1004) = %s, %v; want canjson.ErrInvalidValue", b, err)
	}
}

// TestREQ176_IntegerEnumerationAbsence pins what reads as an absent member
// rather than as a value to refuse: a JSON null, as for any member, and no
// element.
func TestREQ176_IntegerEnumerationAbsence(t *testing.T) {
	var c aom14.CTime
	if err := canjson.Unmarshal([]byte(timeJSON("null")), &c); err != nil || c.TimezoneValidity != nil {
		t.Errorf("canjson.Unmarshal(timezone_validity null) = %v, read %v; want nil, nil", err, c.TimezoneValidity)
	}
	var e aom14.ExprBinaryOperator
	if err := canjson.Unmarshal([]byte(operatorJSON("null")), &e); err != nil || e.Operator != 0 {
		t.Errorf("canjson.Unmarshal(operator null) = %v, read %d; want nil, no operator", err, e.Operator)
	}
	if err := canxml.Unmarshal([]byte(`<c_time`+nsOpenEHR+`/>`), &c, canxml.WithStrict()); err != nil || c.TimezoneValidity != nil {
		t.Errorf("canxml.Unmarshal(no timezone_validity) = %v, read %v; want nil, nil", err, c.TimezoneValidity)
	}
}

// TestIntegerEnumerationNilReceiver pins that the decode methods of an
// integer enumeration refuse a nil receiver with typereg.ErrNilReceiver
// rather than dereferencing it.
func TestIntegerEnumerationNilReceiver(t *testing.T) {
	var k *aom14.OperatorKind
	if err := k.UnmarshalText([]byte("2007")); !errors.Is(err, typereg.ErrNilReceiver) {
		t.Errorf("(*OperatorKind)(nil).UnmarshalText = %v; want typereg.ErrNilReceiver", err)
	}
	dec := jsontext.NewDecoder(strings.NewReader("2007"))
	if err := k.UnmarshalJSONFrom(dec); !errors.Is(err, typereg.ErrNilReceiver) {
		t.Errorf("(*OperatorKind)(nil).UnmarshalJSONFrom = %v; want typereg.ErrNilReceiver", err)
	}
	var v aom14.TimezoneValidityKind
	if err := jsonv2.Unmarshal([]byte(`"disallowed"`), &v); err != nil || v != 1003 {
		t.Errorf("json.Unmarshal(\"disallowed\") into TimezoneValidityKind = %v, read %d; want nil, 1003", err, v)
	}
}

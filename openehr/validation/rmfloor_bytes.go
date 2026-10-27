package validation

// rmfloor_bytes.go: REQ-112 § Value-typed mandatory presence, PROBE-081,
// PROBE-110: the entries that decode a canonical-JSON or canonical-XML
// document into an RM value and validate it with the decode's report of the
// members the document leaves out (wire.md § REQ-175, ADR 0033), in one call.
// A member held by value, a number, a boolean or a value struct such as
// EHR_STATUS.subject (rm.PartySelf, whose only field is optional), decodes to
// its zero value whether the document left it out or gave that value, so the
// value-based floor cannot see it missing; the report can, at every level of
// any root. A value built in Go has no report and still cannot say.
//
// The package's own files do not import openehr/serialize (REQ-013), so each
// entry runs the codec's default decode by the codec's own route:
// encoding/json/v2 with typereg's hooks drives the generated JSON methods as
// canjson.Unmarshal does, and xmlunmatched decodes the XML document as
// canxml.Unmarshal does. The report comes from internal/rmjsonwalk and from
// the xmlunmatched tracker, which serve the codecs' WithMissing options.
// ValidateRMEHRStatusBytes, the entry written for the EHR_STATUS root where
// the blind spot was found first, is kept as ValidateJSON with no template,
// and deprecated.

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// ValidateJSON decodes data, one canonical-JSON document, into root and
// validates root with the members the document leaves out, as a [Validator]
// built with no options does ([Validator.ValidateJSON]).
func ValidateJSON(data []byte, root any, c *templatecompile.Compiled) Result {
	var v Validator
	return v.ValidateJSON(data, root, c)
}

// ValidateJSON decodes data, one canonical-JSON document, into root, a
// non-nil pointer to a Reference Model value the SDK registers, and
// validates root in one call. It returns what v.Validate(root, c,
// WithMissing(pointers)) returns, where pointers are the members the
// Reference Model makes mandatory that the document leaves out or sends as
// null, as the canonical-JSON codec's WithMissing option reports them: with
// a nil c the Reference Model rules alone, with a compiled template the
// template rules too. Each such member is reported as missing at its path,
// so an omitted EHR_STATUS subject or DV_QUANTITY magnitude is `required`
// there, which the decoded value alone cannot show.
//
// The decode is the codec's default: members the Reference Model does not
// name are ignored, a member name given twice is refused, and root holds the
// decoded value afterwards, so a caller that needs the value and the result
// decodes once.
//
// Input that does not decode into root, malformed JSON or a document whose
// root is not a JSON object, is one `invalid_shape` issue at "/" and a
// not-OK [Result]. The decode error, whose text may quote the input, is in
// the issue's Value and not in its Detail; JSON null raises no decode error,
// so its issue's Value is empty. A nil root is `nil_root`, and a root that
// is no registered Reference Model value `rm_type_unknown` at "/", as
// [ValidateRM] reports them; data is not read then.
func (v *Validator) ValidateJSON(data []byte, root any, c *templatecompile.Compiled) Result {
	const entry = "ValidateJSON"
	if _, guard, ok := rmRootGuard(entry, root); !ok {
		return guard
	}
	missing, err := decodeJSONReporting(data, root)
	if err != nil || jsontext.Value(data).Kind() != '{' {
		return decodeFailure(entry, err)
	}
	return v.Validate(root, c, WithMissing(missing))
}

// decodeJSONReporting decodes data into root as canjson.Unmarshal does, then
// walks data against the Reference Model, as canjson's WithMissing option
// does, and returns the pointer of every missing member, or the first error
// of the two. A root whose Go type gives the walk no RM value to start from
// is not walked.
func decodeJSONReporting(data []byte, root any) ([]string, error) {
	if err := typereg.ClassifyDuplicate(json.Unmarshal(data, root, typereg.Unmarshalers())); err != nil {
		return nil, err
	}
	model, err := rmjsonwalk.Default()
	if err != nil {
		return nil, err
	}
	start, ok := model.RootOf(root)
	if !ok {
		return nil, nil
	}
	var missing []string
	err = model.Walk(data, start, rmjsonwalk.Hooks{Missing: func(pointer string) error {
		missing = append(missing, pointer)
		return nil
	}})
	return missing, err
}

// ValidateXML decodes data, one canonical-XML document, into root and
// validates root with the members the document leaves out, as a [Validator]
// built with no options does ([Validator.ValidateXML]).
func ValidateXML(data []byte, root any, c *templatecompile.Compiled) Result {
	var v Validator
	return v.ValidateXML(data, root, c)
}

// ValidateXML decodes data, one canonical-XML document, into root, a
// non-nil pointer to a Reference Model value the SDK registers, and
// validates root in one call, as [Validator.ValidateJSON] does for canonical
// JSON. The members it reads as missing are those the canonical-XML codec's
// WithMissing option reports: a mandatory member with no element, one whose
// element carries xsi:nil, and one whose element is primitive and holds no
// text but XML whitespace. They are named as the canonical-JSON form of the
// instance names them, so a document gives the same result in either
// format.
//
// The decode is the codec's default, and root holds the decoded value
// afterwards. Input that does not decode into root, malformed XML, content
// before or after the root element other than what XML allows there, or a
// root element that fails its decode, is one `invalid_shape` issue at "/"
// and a not-OK [Result], with the decode error in the issue's Value and not
// in its Detail. A nil root is `nil_root`, and a root that is no registered
// Reference Model value `rm_type_unknown` at "/", as [ValidateRM] reports
// them; data is not read then.
func (v *Validator) ValidateXML(data []byte, root any, c *templatecompile.Compiled) Result {
	const entry = "ValidateXML"
	if _, guard, ok := rmRootGuard(entry, root); !ok {
		return guard
	}
	var missing []string
	t := xmlunmatched.NewTracker(nil, false)
	t.ReportMissing(func(pointer string) { missing = append(missing, pointer) })
	if err := xmlunmatched.Unmarshal(data, root, t); err != nil {
		return decodeFailure(entry, err)
	}
	return v.Validate(root, c, WithMissing(missing))
}

// decodeFailure is the result of an entry that could not decode its input:
// one `invalid_shape` issue at "/" whose Value holds err, whose text may
// quote the input (REQ-168), and whose Detail names the entry alone. A nil
// err, as JSON null raises none, leaves Value empty.
func decodeFailure(entry string, err error) Result {
	return resultFromIssues([]Issue{{
		Path:     "/",
		Code:     "invalid_shape",
		Detail:   entry + ": the input does not decode into the root value",
		Severity: Error,
		Value:    constraints.Redact(err),
	}})
}

// ValidateRMEHRStatusBytes validates a canonical-JSON EHR_STATUS against
// the Reference Model rules alone, reading the members the document leaves
// out. It returns what ValidateJSON(data, new(rm.EHRStatus), nil) returns:
// an omitted or null `subject` is `required` at `/subject`, while a supplied
// subject, even the bare `{"_type":"PARTY_SELF"}`, draws none, though both
// decode to the same zero rm.PartySelf. The same holds for every member the
// Reference Model makes mandatory, at every level of the document.
//
// Input that does not decode into an EHR_STATUS, malformed JSON, a document
// whose root is not a JSON object, or an object repeating a member name, is
// one `invalid_shape` issue at "/", as [ValidateJSON] describes.
//
// Deprecated: Use [ValidateJSON] with a new(rm.EHRStatus) and a nil
// template, which returns the same result and works for every root.
func ValidateRMEHRStatusBytes(data []byte) Result {
	return ValidateJSON(data, new(rm.EHRStatus), nil)
}

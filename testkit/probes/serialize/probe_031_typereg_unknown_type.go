package serializeprobes

import (
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// Probe031TyperegUnknownType implements PROBE-031: a `_type` not in
// the type registry decodes to a typed error wrapping
// [typereg.ErrUnknownType], NOT silently to `map[string]any` or any
// other untyped fallback.
//
// The probe runs against a Composition input whose `composer` field
// carries an intentionally unregistered `_type` so the polymorphic
// dispatch path is exercised. The Go SDK MUST surface the failure
// such that `errors.Is(err, typereg.ErrUnknownType)` is true.
func Probe031TyperegUnknownType() (Result, error) {
	r := Result{Probe: "PROBE-031"}
	const body = `{
        "_type": "COMPOSITION",
        "archetype_node_id": "probe-031",
        "name": {"_type": "DV_TEXT", "value": "probe-031"},
        "language": {"_type": "CODE_PHRASE", "code_string": "en", "terminology_id": {"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},
        "territory": {"_type": "CODE_PHRASE", "code_string": "GB", "terminology_id": {"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},
        "category": {"_type": "DV_CODED_TEXT", "value": "event", "defining_code": {"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}},
        "composer": {"_type": "__PROBE_031_UNREGISTERED_TYPE__"}
    }`
	var c rm.Composition
	err := canjson.Unmarshal([]byte(body), &c)
	if err == nil {
		r.Status = "fail"
		r.Detail = "Unmarshal succeeded; expected typereg.ErrUnknownType"
		return r, nil
	}
	if !errors.Is(err, typereg.ErrUnknownType) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("err = %v; expected errors.Is(_, typereg.ErrUnknownType)", err)
		return r, nil
	}
	r.Status = "pass"
	return r, nil
}

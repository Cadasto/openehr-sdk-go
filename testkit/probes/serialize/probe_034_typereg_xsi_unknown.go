package serializeprobes

import (
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// Probe034TyperegXSIUnknown implements PROBE-034: an `xsi:type` not
// in the type registry decodes to a typed error wrapping
// [typereg.ErrUnknownType], never silently to an untyped fallback.
// It is the XML-wire counterpart of [Probe031TyperegUnknownType].
//
// The probe runs against a Composition input whose `composer` field
// carries an intentionally unregistered `xsi:type` so the polymorphic
// dispatch path is exercised. The Go SDK must surface the failure
// such that `errors.Is(err, typereg.ErrUnknownType)` is true.
func Probe034TyperegXSIUnknown() (Result, error) { // PROBE-034 (REQ-040, REQ-056)
	r := Result{Probe: "PROBE-034"}
	const body = `<composition xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><name><value>probe-034</value></name><archetype_node_id>probe-034</archetype_node_id><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language><territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory><category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category><composer xsi:type="__PROBE_034_UNREGISTERED_TYPE__"></composer></composition>`
	var c rm.Composition
	err := canxml.Unmarshal([]byte(body), &c)
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

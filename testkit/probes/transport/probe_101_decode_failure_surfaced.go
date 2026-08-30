package transportprobes

import (
	"context"
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe101DecodeFailureSurfaced implements PROBE-101: a 2xx response whose
// body cannot be decoded as the requested representation fails the call
// rather than returning a silently zero-valued result, on the shared
// transport decode and on a hand-rolled Definition list decode alike, while
// a non-2xx on the same route keeps failing as it always did (REQ-151).
//
// Three arms, in the catalog's order:
//
//   - (a) ehr.Get — a read through the shared transport.Decode, served a 200
//     whose body cannot decode as an EHR. It MUST fail, and MUST NOT hand
//     back a resource: a zero-valued success paired with a nil error is the
//     exact defect this probe exists to catch.
//   - (b) ehr.Get on the same route, served a 404. It MUST keep failing —
//     REQ-151 changes nothing about a wire failure.
//   - (c) definition.ListTemplates — a hand-rolled list decode, served a JSON
//     object where an array is expected. It MUST fail, and MUST NOT report an
//     empty catalog: an empty *body* is a successful empty catalog under
//     REQ-151's keyed exclusion, but a non-empty body that will not decode is
//     never one.
//
// Every arm is held to exactly one request. Without that, a leaf that refused
// the call before it reached the wire — an id guard, a config refusal —
// would credit the probe with a failure the server never provoked, and the
// probe would pass on an SDK that never decodes anything at all.
//
// Per REQ-080 the assertions stay at observable-behaviour level: whether the
// failure is a *transport.DecodeError rather than a *transport.WireError,
// what its Body carries, and what its Error() is allowed to say are pinned by
// transport/decode_error_test.go and the leaf packages' own
// decode_error_test.go files, never here.
//
// Inputs:
//   - captured returns the ESCAPED path of every request the backend has
//     received so far, in order. The caller wires it up (an httptest recorder
//     in Sandbox mode). The probe reads length deltas to count requests per
//     arm, so it MUST NOT be reset between arms.
//   - undecodableID is an EHR id the backend answers 200 for with a body that
//     cannot decode as an EHR; missingID is one it answers 404 for. The
//     Definition list route (GET /definition/template/adl1.4) answers 200
//     with a non-empty JSON object where an array is expected.
func Probe101DecodeFailureSurfaced(ctx context.Context, c *transport.Client, captured func() []string, undecodableID, missingID openehrclient.EHRID) (Result, error) {
	r := Result{Probe: "PROBE-101"}
	if c == nil {
		return r, errors.New("PROBE-101: nil transport.Client")
	}
	if captured == nil {
		return r, errors.New("PROBE-101: nil captured-path recorder")
	}
	if undecodableID == "" || missingID == "" {
		return r, errors.New("PROBE-101: missing required inputs (undecodable id / missing id)")
	}
	requests := func() int { return len(captured()) }

	// (a) A 200 the read cannot decode.
	before := requests()
	ehr, _, err := openehrclient.Get(ctx, c, undecodableID)
	if n := requests() - before; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("the undecodable-200 read issued %d requests, want exactly 1 — the failure must come from the response, not from a refusal before the wire", n)
		return r, nil
	}
	if err == nil {
		r.Status = "fail"
		r.Detail = "a read served a 200 whose body does not decode as the requested representation returned a nil error"
		switch {
		case ehr == nil:
			r.Detail += "; it handed back no resource either — a silent success with nothing decoded is exactly what REQ-151 forbids"
		case ehr.EHRID.Value == "":
			r.Detail += "; the resource it handed back is a zero value — a silently zero-valued success is exactly what REQ-151 forbids"
		}
		return r, nil
	}
	if ehr != nil {
		r.Status = "fail"
		r.Detail = "a failed 2xx decode handed back a resource beside the error; there is nothing decoded to return"
		return r, nil
	}

	// (b) A non-2xx on the same route still fails.
	before = requests()
	ehr, _, err = openehrclient.Get(ctx, c, missingID)
	if n := requests() - before; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("the 404 read issued %d requests, want exactly 1", n)
		return r, nil
	}
	if err == nil {
		r.Status = "fail"
		r.Detail = "a read served a 404 returned a nil error"
		return r, nil
	}
	if ehr != nil {
		r.Status = "fail"
		r.Detail = "a read served a 404 handed back a resource beside the error"
		return r, nil
	}

	// (c) A hand-rolled list decode served the wrong JSON shape.
	before = requests()
	list, _, err := definition.ListTemplates(ctx, c, definition.FormatADL14)
	if n := requests() - before; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("the list read issued %d requests, want exactly 1", n)
		return r, nil
	}
	if err == nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("a list leaf served a JSON object where an array is expected returned a nil error and %d entries", len(list))
		if len(list) == 0 {
			r.Detail += "; reporting an empty catalog for a body that did not decode reads a failure as success"
		}
		return r, nil
	}
	if len(list) != 0 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("a failed list decode handed back %d entries beside the error", len(list))
		return r, nil
	}

	r.Status = "pass"
	return r, nil
}

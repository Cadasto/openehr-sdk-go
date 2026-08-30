package definitionprobes

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe093TemplateListFilters implements PROBE-093: ListTemplates emits
// the ITS-REST list query parameters `template_id`, `concept`, `version`,
// `offset`, and `fetch` when the corresponding options are set, and omits
// them when unset (REQ-143).
//
// Emission-only by construction. A server that silently ignores unknown
// query parameters is indistinguishable, from the client side, from one
// that filters — so this probe never asserts that the result set narrows.
// It asserts what the SDK controls: which keys reach the wire.
//
// Per REQ-080 the negative-paging leg asserts fail-closed behaviour only —
// a non-nil error and zero captured requests. Sentinel identity
// (transport.ErrInvalidConfig) is pinned by the definition package's unit
// tests, not here.
//
// Inputs:
//   - captured accumulates the query of every request the backend
//     receives, in order. The caller wires it up (an httptest handler in
//     Sandbox mode); the probe reads length deltas to count requests, so
//     it MUST NOT be reset between legs.
//
// The backend answers each list call with a template-metadata body; the
// probe asserts only that the call returns no error. An empty catalog is a
// pass — ListTemplates yields a non-nil zero-length slice for an empty body
// or 204 (REQ-144), and REQ-143 licenses no assertion that a filtered
// deployment holds templates.
func Probe093TemplateListFilters(ctx context.Context, c *transport.Client, captured *[]url.Values) (Result, error) {
	r := Result{Probe: "PROBE-093"}
	if c == nil {
		return r, errors.New("PROBE-093: nil transport.Client")
	}
	if captured == nil {
		return r, errors.New("PROBE-093: nil captured-query recorder")
	}

	// Leg 1 — no options: the query carries none of the five keys.
	before := len(*captured)
	if _, _, err := definition.ListTemplates(ctx, c, definition.FormatADL14); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("unfiltered list failed: %v", err)
		return r, nil
	}
	q, ok := lastQuery(captured, before)
	if !ok {
		r.Status = "fail"
		r.Detail = "unfiltered list issued no request"
		return r, nil
	}
	for _, k := range []string{"template_id", "concept", "version", "offset", "fetch"} {
		if q.Has(k) {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("unfiltered list emitted %q=%q; unset options must omit their key", k, q.Get(k))
			return r, nil
		}
	}

	// Leg 2 — every option set: each reaches the wire under its pin name.
	before = len(*captured)
	// An empty list is not a failure: ListTemplates returns an empty slice
	// and a nil error for an empty body / 204 (REQ-144), which is a
	// legitimately empty catalog. A decode that genuinely failed comes back
	// as a non-nil error, so err is the only decode assertion REQ-143
	// licenses here.
	if _, _, err := definition.ListTemplates(ctx, c, definition.FormatADL14,
		definition.WithTemplateID("vital*"),
		definition.WithConcept("*signs*"),
		definition.WithVersion("1.2.*"),
		definition.WithOffset(10),
		definition.WithFetch(25),
	); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("filtered list failed: %v", err)
		return r, nil
	}
	q, ok = lastQuery(captured, before)
	if !ok {
		r.Status = "fail"
		r.Detail = "filtered list issued no request"
		return r, nil
	}
	// Ordered, not a map: a probe's failure detail is diagnostic evidence,
	// so the first reported mismatch must be reproducible.
	for _, kv := range []struct{ key, want string }{
		{"template_id", "vital*"},
		{"concept", "*signs*"},
		{"version", "1.2.*"},
		{"offset", "10"},
		{"fetch", "25"},
	} {
		k, want := kv.key, kv.want
		if got := q.Get(k); got != want {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("query %q = %q, want %q", k, got, want)
			return r, nil
		}
	}

	// Leg 3 — explicit zero paging is sent, not swallowed as "unset".
	before = len(*captured)
	if _, _, err := definition.ListTemplates(ctx, c, definition.FormatADL14,
		definition.WithOffset(0), definition.WithFetch(0),
	); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("zero-paging list failed: %v", err)
		return r, nil
	}
	q, ok = lastQuery(captured, before)
	if !ok {
		r.Status = "fail"
		r.Detail = "zero-paging list issued no request"
		return r, nil
	}
	for _, k := range []string{"offset", "fetch"} {
		if q.Get(k) != "0" {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("explicit zero %s = %q, want %q on the wire", k, q.Get(k), "0")
			return r, nil
		}
	}

	// Leg 4 — negative paging fails closed, with nothing on the wire.
	for _, tc := range []struct {
		name string
		opt  definition.ListOption
	}{
		{"offset", definition.WithOffset(-1)},
		{"fetch", definition.WithFetch(-1)},
	} {
		before = len(*captured)
		if _, _, err := definition.ListTemplates(ctx, c, definition.FormatADL14, tc.opt); err == nil {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("negative %s was accepted; want a refusal", tc.name)
			return r, nil
		}
		if n := len(*captured) - before; n != 0 {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("negative %s issued %d requests, want 0", tc.name, n)
			return r, nil
		}
	}

	r.Status = "pass"
	return r, nil
}

// lastQuery returns the query recorded after before, and whether exactly
// one request was captured since. More than one means the leaf issued a
// retry or a second call the probe did not account for.
func lastQuery(captured *[]url.Values, before int) (url.Values, bool) {
	if len(*captured) != before+1 {
		return nil, false
	}
	return (*captured)[before], true
}

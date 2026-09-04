package transportprobes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client/admin"
	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/directory"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/openehr/client/system"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// hostile ids, one per failure mode REQ-150 names.
const (
	// traversalID's segments are themselves illegal — `..` rewrites the
	// request URI upward.
	traversalID = "a/../../definition/query/evil"
	// smuggledID's segments are each individually legal; only the count
	// contradicts the leaf's Route template.
	smuggledID = "foo/bar"
	// dotDotID is a traversal that PRESERVES arity — `/ehr/..` has the same
	// segment count as `/ehr/{ehr_id}`. It is the input that separates the
	// two halves of REQ-150: the arity rule cannot see it, so only segment
	// validation refuses it. Without this id the probe passes even with
	// whole-path validation removed entirely.
	dotDotID = ".."
	// controlID likewise preserves arity and is refused only by the
	// control-character rule.
	controlID = "a\x00b"
)

// Probe091PathSegmentValidation implements PROBE-091: a path parameter
// that is `.` or `..`, is empty, or carries `/`, `\`, or a control
// character is refused before any HTTP request is issued, on every
// path-interpolating leaf package (REQ-150).
//
// Driven calls, at least one per `openehr/client` package that builds
// its own transport.Request:
//
//   - ehr.Get                      — /ehr/{ehr_id}
//   - composition.Get              — /ehr/{ehr_id}/composition/{uid}
//   - directory.Get                — /ehr/{ehr_id}/directory
//   - ehrstatus.Get                — /ehr/{ehr_id}/ehr_status
//   - ehrstatus.GetAtTime          — same, plus a trailing query parameter
//   - contribution.Commit          — /ehr/{ehr_id}/contribution (a write)
//   - contribution.Get             — /ehr/{ehr_id}/contribution/{contribution_uid}
//   - query.RunStored              — /query/{qualified_query_name}
//   - definition.GetTemplate       — /definition/template/{format}/{template_id}
//   - demographic.GetVersionedParty — /demographic/versioned_party/{uid}
//   - admin.DeleteEHR              — /admin/ehr/{ehr_id}
//
// Not driven, deliberately: `itemtags` builds no transport.Request of its
// own (it delegates wholly to composition / ehrstatus / directory, so
// probing it re-probes three leaves already here), and `system`'s only
// path is the fixed service root — which this probe covers as the
// POSITIVE case below rather than as an interpolation site, since REQ-150
// requires `OPTIONS /` to keep working.
//
// Each leaf is driven with all four hostile ids. Two change the path's
// segment count and two preserve it, so the probe exercises BOTH halves of
// REQ-150 independently — an arity-only implementation fails the dot-dot
// and control-character legs, and a segment-only implementation fails the
// smuggled-separator leg. Per REQ-080 the assertion is
// fail-closed behaviour only — a non-nil error and zero captured requests.
// Sentinel identity (ErrInvalidPathSegment / ErrInvalidConfig) is pinned
// by transport/path_test.go and the leaf unit tests, never here.
//
// Inputs:
//   - captured returns the ESCAPED path of every request the backend has
//     received so far, in order. The caller wires it up (an httptest
//     recorder in Sandbox mode). The probe reads length deltas to count
//     requests and inspects the last entry to check encode-once (REQ-095),
//     so it MUST NOT be reset between legs.
func Probe091PathSegmentValidation(ctx context.Context, c *transport.Client, captured func() []string) (Result, error) {
	r := Result{Probe: "PROBE-091"}
	if c == nil {
		return r, errors.New("PROBE-091: nil transport.Client")
	}
	if captured == nil {
		return r, errors.New("PROBE-091: nil captured-path recorder")
	}
	requests := func() int { return len(captured()) }

	for _, leaf := range hostileLeaves() {
		for _, id := range []string{traversalID, smuggledID, dotDotID, controlID} {
			before := requests()
			err := leaf.call(ctx, c, id)
			if err == nil {
				r.Status = "fail"
				r.Detail = fmt.Sprintf("%s accepted a hostile path parameter %q; want a refusal", leaf.name, id)
				return r, nil
			}
			if n := requests() - before; n != 0 {
				r.Status = "fail"
				r.Detail = fmt.Sprintf("%s issued %d requests for hostile path parameter %q; want 0 (fail closed)", leaf.name, n, id)
				return r, nil
			}
		}
	}

	// The positive half. A well-formed openEHR identifier that merely needs
	// percent-encoding MUST still reach the wire exactly once (REQ-095), and
	// the service root MUST still be reachable — REQ-150 carves it out
	// explicitly because the System API's only operation is `OPTIONS /`.
	before := requests()
	if _, _, err := definition.GetTemplate(ctx, c, "Blood Pressure.v1", definition.FormatADL14); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("a well-formed dotted/spaced template id was refused: %v", err)
		return r, nil
	}
	if n := requests() - before; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("a well-formed template id issued %d requests, want exactly 1", n)
		return r, nil
	}
	// Encode-once (REQ-095): the space rides as %20, never %2520. Counting
	// the request is not enough — a double-encoded path is still one request.
	paths := captured()
	escaped := paths[len(paths)-1]
	if strings.Contains(escaped, "%2520") || !strings.Contains(escaped, "%20") {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("a well-formed template id reached the wire as %q; want the space encoded exactly once (%%20)", escaped)
		return r, nil
	}

	before = requests()
	if _, _, err := system.Capabilities(ctx, c); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("the service root (OPTIONS /) was refused: %v", err)
		return r, nil
	}
	if n := requests() - before; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("the service root issued %d requests, want exactly 1", n)
		return r, nil
	}

	r.Status = "pass"
	return r, nil
}

// hostileLeaf names one leaf entry point and how to drive it with a
// hostile path parameter.
type hostileLeaf struct {
	name string
	call func(ctx context.Context, c *transport.Client, id string) error
}

// hostileLeaves is a function, not a package var, so the slice cannot be
// mutated by a caller between probe runs.
func hostileLeaves() []hostileLeaf {
	const versionUID openehrclient.VersionUID = "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::1"
	const ehrID openehrclient.EHRID = "1234abcd-5678-9012-3456-7890abcdef00"
	return []hostileLeaf{
		{"ehr.Get", func(ctx context.Context, c *transport.Client, id string) error {
			_, _, err := openehrclient.Get(ctx, c, openehrclient.EHRID(id))
			return err
		}},
		{"composition.Get", func(ctx context.Context, c *transport.Client, id string) error {
			_, _, err := composition.Get(ctx, c, openehrclient.EHRID(id), openehrclient.VersionOf(versionUID))
			return err
		}},
		{"directory.Get", func(ctx context.Context, c *transport.Client, id string) error {
			_, _, err := directory.Get(ctx, c, openehrclient.EHRID(id))
			return err
		}},
		{"ehrstatus.Get", func(ctx context.Context, c *transport.Client, id string) error {
			_, _, err := ehrstatus.Get(ctx, c, openehrclient.EHRID(id))
			return err
		}},
		{"ehrstatus.GetAtTime", func(ctx context.Context, c *transport.Client, id string) error {
			_, _, err := ehrstatus.GetAtTime(ctx, c, openehrclient.EHRID(id), time.Unix(0, 0).UTC())
			return err
		}},
		{"contribution.Commit", func(ctx context.Context, c *transport.Client, id string) error {
			// A write, so it needs a Submission that survives Commit's own
			// guards -- otherwise the leaf refuses on the body and the probe
			// would credit a refusal the transport never made.
			_, _, err := contribution.Commit(ctx, c, openehrclient.EHRID(id), minimalSubmission())
			return err
		}},
		{"contribution.Get", func(ctx context.Context, c *transport.Client, id string) error {
			// The read half drives the hostile value through the SECOND
			// interpolation site, {contribution_uid} — {ehr_id} is already
			// covered by the Commit leg above. Get's own guard refuses only
			// EMPTY ids, so a refusal here can only come from the transport.
			_, _, err := contribution.Get(ctx, c, ehrID, id)
			return err
		}},
		{"query.RunStored", func(ctx context.Context, c *transport.Client, id string) error {
			_, _, err := query.RunStored(ctx, c, id, nil)
			return err
		}},
		{"definition.GetTemplate", func(ctx context.Context, c *transport.Client, id string) error {
			_, _, err := definition.GetTemplate(ctx, c, id, definition.FormatADL14)
			return err
		}},
		{"demographic.GetVersionedParty", func(ctx context.Context, c *transport.Client, id string) error {
			_, _, err := demographic.GetVersionedParty(ctx, c, openehrclient.VersionedObjectID(id))
			return err
		}},
		{"admin.DeleteEHR", func(ctx context.Context, c *transport.Client, id string) error {
			return admin.DeleteEHR(ctx, c, openehrclient.EHRID(id))
		}},
	}
}

// minimalSubmission is the smallest batch that clears every guard
// contribution.Commit applies BEFORE the transport prepares the request, so a
// refusal can only come from the path check and not from the body. Commit
// canjson.Marshals the batch on its way to Do, so BOTH audits it touches have
// to be populated -- each is a distinct pre-Do gate:
//
//   - Submission.Audit, the batch commit audit: UpdateAudit.MarshalJSON
//     refuses a nil Committer. Left zero, all four hostile legs fail closed on
//     the body and credit the probe with a refusal the transport never made
//     -- the probe would pass with REQ-150 enforcement removed from this leaf.
//   - the wrapped version's CommitAudit: WrapOriginalVersion converts it to
//     the UpdateAudit DTO (a nil or typed-nil audit yields the empty DTO —
//     the #114 fix), and Submission.Validate refuses the DTO's nil Committer.
func minimalSubmission() *contribution.Submission {
	committer := "probe"
	party := &rm.PartyIdentified{Name: &committer}
	changeType := rm.DVCodedText{Value: "creation", DefiningCode: rm.CodePhrase{CodeString: "249"}}
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	return &contribution.Submission{
		Audit: contribution.UpdateAudit{
			SystemID:   "probe.example",
			Committer:  party,
			ChangeType: changeType,
		},
		Versions: []contribution.CommitVersion{
			contribution.WrapOriginalVersion(&rm.OriginalVersion[rm.Composition]{
				CommitAudit: rm.AuditDetails{
					SystemID:   "probe.example",
					Committer:  party,
					ChangeType: changeType,
				},
				UID:            rm.ObjectVersionID{Value: "1::probe.example::1"},
				LifecycleState: rm.DVCodedText{DVText: rm.DVText{Value: "complete"}, DefiningCode: rm.CodePhrase{CodeString: "532"}},
				Data:           &comp,
			}),
		},
	}
}

package versionedprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/directory"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// probe109AtTime is the pin's version_at_time example carried to
// nanoseconds: 19:30:22.765432101 at +01:00 on 20 January 2015. Nine digits
// tell a leaf that keeps the whole fraction from one that keeps only the
// milliseconds the pin's own example shows.
var probe109AtTime = time.Date(2015, 1, 20, 19, 30, 22, 765_432_101, time.FixedZone("", 3600))

// probe109AtTimeWire is probe109AtTime as it must reach the wire: in UTC,
// with all nine digits of its fraction. It is a literal rather than a call
// to the formatter the leaves share, so a formatter that drops, cuts or
// rounds the fraction fails the probe instead of agreeing with itself.
const probe109AtTimeWire = "2015-01-20T18:30:22.765432101Z"

// Probe109Inputs names the records [Probe109VersionedObjectReads] reads. The
// backend answers every read with a canonical-JSON body.
type Probe109Inputs struct {
	// EHRID is the EHR whose VERSIONED_COMPOSITION, VERSIONED_EHR_STATUS,
	// EHR_STATUS and directory are read.
	EHRID openehrclient.EHRID
	// CompositionVOID is a VERSIONED_COMPOSITION in EHRID. It is also the
	// composition read by a Ref with a time.
	CompositionVOID openehrclient.VersionedObjectID
	// CompositionVersionUID is a version of CompositionVOID, answered with an
	// ORIGINAL_VERSION whose data is a COMPOSITION.
	CompositionVersionUID openehrclient.VersionUID
	// StatusVersionUID is a version of EHRID's EHR_STATUS, answered with an
	// ORIGINAL_VERSION whose data is an EHR_STATUS.
	StatusVersionUID openehrclient.VersionUID
	// ImportedVersionUID is a version id that both families' by-id reads
	// answer with an IMPORTED_VERSION.
	ImportedVersionUID openehrclient.VersionUID
	// PartyVOID is a VERSIONED_PARTY whose version-at-time read is answered
	// with an ORIGINAL_VERSION.
	PartyVOID openehrclient.VersionedObjectID
}

// probe109Read is one read PROBE-109 drives and what its one request must
// look like.
type probe109Read struct {
	// name is the leaf call, as Detail names it.
	name string
	// path is the full request path the call must send: the client's base
	// path, then the pinned route.
	path string
	// atTime marks a read that sends version_at_time. Every other read must
	// send none, the latest-version reads among them.
	atTime bool
	// acceptJSON marks the versioned-object reads, which request canonical
	// JSON.
	acceptJSON bool
	// requestOnly marks a row judged on its request alone: the read's error
	// and result are not this probe's subject.
	requestOnly bool
	// call issues the read. It returns the read's error, or a non-empty
	// reason when the 200 body did not decode to what the read returns.
	call func(context.Context) (reason string, err error)
}

// Probe109VersionedObjectReads implements PROBE-109: the composition and
// EHR_STATUS leaves issue the versioned-object reads of the vendored EHR API
// and decode their bodies, and the version_at_time each of the probe's
// at-time reads sends keeps every digit of its fraction of a second.
//
// It drives the ten versioned-object reads (the versioned object, its
// revision history, the latest version, the version at a time and a version
// by id, for each family). Each must issue exactly one GET to its pinned path
// under the client's base path with Accept: application/json, and decode its
// 200 body. The version-at-time reads take the pin's example time carried to
// nanoseconds and must send exactly 2015-01-20T18:30:22.765432101Z, so a
// value cut to milliseconds fails; every other read must send no
// version_at_time. A by-id read answered with an IMPORTED_VERSION must fail
// with a [*transport.DecodeError], and its one request is held to the same
// method, path and Accept, with no version_at_time.
//
// It then sends the same time through the composition read by a Ref with a
// time, the EHR_STATUS and directory reads at a time, and the VERSIONED_PARTY
// version-at-time read. These four rows judge the request only: exactly one
// GET to the route, carrying that same value. Whether the read succeeds is
// not judged.
//
// Sentinel identity is not asserted here; the leaf unit tests own it.
//
// Inputs:
//   - captured accumulates every request the backend receives, in order.
//     The caller wires it up (a `sandbox.Backend` scripted route in Sandbox
//     mode); the probe reads length deltas to count requests, so it must
//     not be reset while the probe runs.
//   - in names the records the backend serves; see [Probe109Inputs].
//
// An error means the probe could not run: a nil client or recorder, an
// empty id, or a client with no openEHR REST service to take the base path
// from.
func Probe109VersionedObjectReads(ctx context.Context, c *transport.Client, captured *[]*http.Request, in Probe109Inputs) (Result, error) { // PROBE-109 (REQ-174)
	r := Result{Probe: "PROBE-109"}
	if c == nil || in.EHRID == "" || in.CompositionVOID == "" || in.CompositionVersionUID == "" ||
		in.StatusVersionUID == "" || in.ImportedVersionUID == "" || in.PartyVOID == "" {
		return r, errors.New("PROBE-109: missing required inputs (client/ehr id/composition id/version ids/party id)")
	}
	if captured == nil {
		return r, errors.New("PROBE-109: nil captured-request recorder")
	}
	svc, ok := c.Catalog().OpenEHRRest()
	if !ok || svc.BaseURL == nil {
		return r, errors.New("PROBE-109: the client has no openEHR REST service to take the base path from")
	}
	base := strings.TrimSuffix(svc.BaseURL.Path, "/")

	reads, refusals := probe109Reads(c, base, in)
	for _, rd := range reads {
		before := len(*captured)
		reason, err := rd.call(ctx)
		if err != nil && !rd.requestOnly {
			return probe109Fail(r, "%s failed: %v", rd.name, err), nil
		}
		if n := len(*captured) - before; n != 1 {
			return probe109Fail(r, "%s issued %d request(s), want exactly 1", rd.name, n), nil
		}
		if why := probe109CheckRequest((*captured)[before], rd); why != "" {
			return probe109Fail(r, "%s: %s", rd.name, why), nil
		}
		if reason != "" {
			return probe109Fail(r, "%s: %s", rd.name, reason), nil
		}
	}

	// The refusal rows: v1 reads only an ORIGINAL_VERSION, so an
	// IMPORTED_VERSION is the typed decode failure. The read's one request
	// is judged first, held to the shape of every other by-id read.
	for _, rd := range refusals {
		before := len(*captured)
		_, err := rd.call(ctx)
		if n := len(*captured) - before; n != 1 {
			return probe109Fail(r, "%s on an IMPORTED_VERSION issued %d request(s), want exactly 1", rd.name, n), nil
		}
		if why := probe109CheckRequest((*captured)[before], rd); why != "" {
			return probe109Fail(r, "%s on an IMPORTED_VERSION: %s", rd.name, why), nil
		}
		if err == nil {
			return probe109Fail(r, "%s on an IMPORTED_VERSION succeeded, want a typed decode error", rd.name), nil
		}
		if de, ok := errors.AsType[*transport.DecodeError](err); !ok || de == nil {
			return probe109Fail(r, "%s on an IMPORTED_VERSION failed with an error that is not a *transport.DecodeError: %v", rd.name, err), nil
		}
	}

	r.Status = "pass"
	r.Detail = "ten versioned-object reads: one GET each to the pinned path, Accept: application/json, bodies decoded; " +
		"version_at_time = " + probe109AtTimeWire + " on the six at-time reads and absent elsewhere; " +
		"IMPORTED_VERSION fails as a typed decode error on a request of the same shape"
	return r, nil
}

// probe109Reads lists the reads PROBE-109 drives. reads holds, in order, the
// ten versioned-object reads, then the at-time reads of the other leaves.
// refusals holds the two by-id reads of a version the backend answers with
// an IMPORTED_VERSION.
func probe109Reads(c *transport.Client, base string, in Probe109Inputs) (reads, refusals []probe109Read) {
	ehr := base + "/ehr/" + string(in.EHRID)
	vc := ehr + "/versioned_composition/" + string(in.CompositionVOID)
	vs := ehr + "/versioned_ehr_status"
	refusals = []probe109Read{
		{name: "composition.GetVersionByID", path: vc + "/version/" + string(in.ImportedVersionUID), acceptJSON: true, call: func(ctx context.Context) (string, error) {
			_, _, err := composition.GetVersionByID(ctx, c, in.EHRID, in.CompositionVOID, in.ImportedVersionUID)
			return "", err
		}},
		{name: "ehrstatus.GetVersionByID", path: vs + "/version/" + string(in.ImportedVersionUID), acceptJSON: true, call: func(ctx context.Context) (string, error) {
			_, _, err := ehrstatus.GetVersionByID(ctx, c, in.EHRID, in.ImportedVersionUID)
			return "", err
		}},
	}
	reads = []probe109Read{
		{name: "composition.GetVersionedObject", path: vc, acceptJSON: true, call: func(ctx context.Context) (string, error) {
			out, _, err := composition.GetVersionedObject(ctx, c, in.EHRID, in.CompositionVOID)
			if err != nil {
				return "", err
			}
			if out == nil || out.UID.Value == "" {
				return "the body did not decode to a VERSIONED_COMPOSITION with a uid", nil
			}
			return "", nil
		}},
		{name: "composition.GetRevisionHistory", path: vc + "/revision_history", acceptJSON: true, call: func(ctx context.Context) (string, error) {
			out, _, err := composition.GetRevisionHistory(ctx, c, in.EHRID, in.CompositionVOID)
			if err != nil {
				return "", err
			}
			if out == nil || len(out.Items) == 0 {
				return "the body did not decode to a REVISION_HISTORY with items", nil
			}
			return "", nil
		}},
		{name: "composition.GetVersion", path: vc + "/version", acceptJSON: true, call: func(ctx context.Context) (string, error) {
			return probe109CompositionVersion(composition.GetVersion(ctx, c, in.EHRID, in.CompositionVOID))
		}},
		{name: "composition.GetVersionAtTime", path: vc + "/version", atTime: true, acceptJSON: true, call: func(ctx context.Context) (string, error) {
			return probe109CompositionVersion(composition.GetVersionAtTime(ctx, c, in.EHRID, in.CompositionVOID, probe109AtTime))
		}},
		{name: "composition.GetVersionByID", path: vc + "/version/" + string(in.CompositionVersionUID), acceptJSON: true, call: func(ctx context.Context) (string, error) {
			return probe109CompositionVersion(composition.GetVersionByID(ctx, c, in.EHRID, in.CompositionVOID, in.CompositionVersionUID))
		}},
		{name: "ehrstatus.GetVersionedObject", path: vs, acceptJSON: true, call: func(ctx context.Context) (string, error) {
			out, _, err := ehrstatus.GetVersionedObject(ctx, c, in.EHRID)
			if err != nil {
				return "", err
			}
			if out == nil || out.UID.Value == "" {
				return "the body did not decode to a VERSIONED_EHR_STATUS with a uid", nil
			}
			return "", nil
		}},
		{name: "ehrstatus.GetRevisionHistory", path: vs + "/revision_history", acceptJSON: true, call: func(ctx context.Context) (string, error) {
			out, _, err := ehrstatus.GetRevisionHistory(ctx, c, in.EHRID)
			if err != nil {
				return "", err
			}
			if out == nil || len(out.Items) == 0 {
				return "the body did not decode to a REVISION_HISTORY with items", nil
			}
			return "", nil
		}},
		{name: "ehrstatus.GetVersion", path: vs + "/version", acceptJSON: true, call: func(ctx context.Context) (string, error) {
			return probe109StatusVersion(ehrstatus.GetVersion(ctx, c, in.EHRID))
		}},
		{name: "ehrstatus.GetVersionAtTime", path: vs + "/version", atTime: true, acceptJSON: true, call: func(ctx context.Context) (string, error) {
			return probe109StatusVersion(ehrstatus.GetVersionAtTime(ctx, c, in.EHRID, probe109AtTime))
		}},
		{name: "ehrstatus.GetVersionByID", path: vs + "/version/" + string(in.StatusVersionUID), acceptJSON: true, call: func(ctx context.Context) (string, error) {
			return probe109StatusVersion(ehrstatus.GetVersionByID(ctx, c, in.EHRID, in.StatusVersionUID))
		}},

		// The other leaves that send version_at_time. Their own reads are
		// not this probe's subject, so only the request is judged: its
		// method, path and version_at_time, not Accept or the read's result.
		{name: "composition.Get(LatestAtTime)", path: ehr + "/composition/" + string(in.CompositionVOID), atTime: true, requestOnly: true, call: func(ctx context.Context) (string, error) {
			_, _, err := composition.Get(ctx, c, in.EHRID, openehrclient.LatestAtTime(in.CompositionVOID, probe109AtTime))
			return "", err
		}},
		{name: "ehrstatus.GetAtTime", path: ehr + "/ehr_status", atTime: true, requestOnly: true, call: func(ctx context.Context) (string, error) {
			_, _, err := ehrstatus.GetAtTime(ctx, c, in.EHRID, probe109AtTime)
			return "", err
		}},
		{name: "directory.GetAtTime", path: ehr + "/directory", atTime: true, requestOnly: true, call: func(ctx context.Context) (string, error) {
			_, _, err := directory.GetAtTime(ctx, c, in.EHRID, probe109AtTime)
			return "", err
		}},
		{name: "demographic.GetVersionAtTime", path: base + "/demographic/versioned_party/" + string(in.PartyVOID) + "/version", atTime: true, requestOnly: true, call: func(ctx context.Context) (string, error) {
			_, _, err := demographic.GetVersionAtTime(ctx, c, in.PartyVOID, probe109AtTime)
			return "", err
		}},
	}
	return reads, refusals
}

// probe109CheckRequest returns why req does not match rd, or "" when it does.
func probe109CheckRequest(req *http.Request, rd probe109Read) string {
	if req.Method != http.MethodGet {
		return fmt.Sprintf("method = %q, want GET", req.Method)
	}
	if req.URL.Path != rd.path {
		return fmt.Sprintf("path = %q, want %q", req.URL.Path, rd.path)
	}
	if got := req.Header.Values("Accept"); rd.acceptJSON && !slices.Equal(got, []string{"application/json"}) {
		return fmt.Sprintf("Accept = %q, want [\"application/json\"] (canonical JSON)", got)
	}
	got, sent := req.URL.Query()["version_at_time"]
	switch {
	case rd.atTime && !slices.Equal(got, []string{probe109AtTimeWire}):
		return fmt.Sprintf("version_at_time = %q, want exactly [%q] (the instant in UTC, every fraction digit kept)", got, probe109AtTimeWire)
	case !rd.atTime && sent:
		return fmt.Sprintf("sent version_at_time = %q, want none", got)
	}
	return ""
}

// probe109CompositionVersion judges a composition version read's result.
func probe109CompositionVersion(v *composition.Version, _ *openehrclient.VersionMetadata, err error) (string, error) {
	switch {
	case err != nil:
		return "", err
	case v == nil || v.UID.Value == "":
		return "the body did not decode to an ORIGINAL_VERSION with a uid", nil
	case v.Composition == nil:
		return "the version's data did not decode to a COMPOSITION", nil
	}
	return "", nil
}

// probe109StatusVersion judges an EHR_STATUS version read's result.
func probe109StatusVersion(v *ehrstatus.Version, _ *openehrclient.VersionMetadata, err error) (string, error) {
	switch {
	case err != nil:
		return "", err
	case v == nil || v.UID.Value == "":
		return "the body did not decode to an ORIGINAL_VERSION with a uid", nil
	case v.Status == nil:
		return "the version's data did not decode to an EHR_STATUS", nil
	}
	return "", nil
}

func probe109Fail(r Result, format string, args ...any) Result {
	r.Status = "fail"
	r.Detail = fmt.Sprintf(format, args...)
	return r
}

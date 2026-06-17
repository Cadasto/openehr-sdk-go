package demographicprobes

import (
	"context"
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// probeVOID is the fixed versioned-object id the probe addresses; the fake
// server keys every read off the resource path, not the id.
const probeVOID openehrclient.VersionedObjectID = "demographic-probe-vo-1"

// Probe073DemographicRoundTrip implements PROBE-073: a PARTY of concrete type
// t round-trips through create → get → get-version with its `_type`
// discriminator decoded back into the same concrete Go type at every hop.
//
//   - Create (Prefer=representation) returns the created PARTY body.
//   - Get returns the latest PARTY body.
//   - GetVersion returns the ORIGINAL_VERSION<PARTY> envelope whose `data` is
//     decoded polymorphically (the envelope's generic data cannot decode into
//     the abstract rm.Party interface directly — REQ-040 via the type
//     registry).
//
// All three MUST yield the same concrete type as the input party. The caller
// wires c to a server that echoes the PARTY body for type t (Sandbox: an
// httptest server; Cassette / Live later).
func Probe073DemographicRoundTrip(ctx context.Context, c *transport.Client, party rm.Party, t demographic.Type) (Result, error) {
	r := Result{Probe: "PROBE-073"}
	if c == nil {
		return r, errors.New("PROBE-073: nil transport.Client")
	}
	if party == nil {
		return r, errors.New("PROBE-073: nil party")
	}
	want := fmt.Sprintf("%T", party)

	created, _, err := demographic.Create(ctx, c, party, demographic.WithPrefer(transport.PreferRepresentation))
	if err != nil {
		return fail(r, "create %s: %v", t, err)
	}
	if got := fmt.Sprintf("%T", created); got != want {
		return fail(r, "create decoded %s, want %s", got, want)
	}

	gotten, _, err := demographic.Get(ctx, c, t, openehrclient.LatestOf(probeVOID))
	if err != nil {
		return fail(r, "get %s: %v", t, err)
	}
	if got := fmt.Sprintf("%T", gotten); got != want {
		return fail(r, "get decoded %s, want %s", got, want)
	}

	pv, _, err := demographic.GetVersion(ctx, c, probeVOID)
	if err != nil {
		return fail(r, "get version %s: %v", t, err)
	}
	if pv == nil || pv.Party == nil {
		return fail(r, "get version %s: nil VERSION data", t)
	}
	if got := fmt.Sprintf("%T", pv.Party); got != want {
		return fail(r, "VERSION data decoded %s, want %s", got, want)
	}

	r.Status = "pass"
	return r, nil
}

func fail(r Result, format string, args ...any) (Result, error) {
	r.Status = "fail"
	r.Detail = fmt.Sprintf(format, args...)
	return r, nil
}

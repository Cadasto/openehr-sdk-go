package demographic_test

// decode_error_test.go — REQ-151 § Typed 2xx decode failure, held against this
// package's three hand-rolled 2xx response decodes: the polymorphic PARTY body
// in getParty (behind Get), and getVersion's two arms — the
// ORIGINAL_VERSION<PARTY> envelope and the polymorphic VERSION data inside it.
//
// None of the three routes through transport.Decode: the abstract rm.Party
// interface cannot be decoded by the generated unmarshallers, so these leaves
// call typereg.DecodeAs themselves (REQ-040). REQ-151 § Scope is explicit that
// this makes no difference to the caller — a hand-rolled response decode "MUST
// satisfy it identically, so which implementation route a leaf took stays
// invisible".
//
// The neighbouring empty-body arms are keyed exclusions and stay as they are:
// a 204 is a legitimate "no version at that time" success, and any other
// empty-body 2xx is transport.ErrInvalidShape. Every body below is therefore
// non-empty and undecodable, which is what this requirement is about.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// unregisteredParty is a well-formed JSON object whose `_type` no constructor
// is registered for, so the type registry refuses it (typereg.ErrUnknownType).
// It is the cheapest guaranteed-undecodable PARTY body: an object with merely
// unexpected keys decodes cleanly, since unknown fields are ignored.
const unregisteredParty = `{"_type":"NOT_A_PARTY","name":{"_type":"DV_TEXT","value":"Jane Doe"}}`

// brokenEnvelope is a JSON array served where the ORIGINAL_VERSION object is
// expected — the envelope arm's guaranteed-undecodable body.
const brokenEnvelope = `[{"_type":"ORIGINAL_VERSION"}]`

// serve returns a client aimed at a server that answers 200 with body and an
// ETag on every request, so the metadata assertions have something to find.
func serve(t *testing.T, body string) *transport.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"`+personVersion+`"`)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return newClient(t, srv)
}

// TestDemographicDecodeFailuresAreTyped is REQ-151's positive contract at all
// three sites. Each case serves a non-empty body that cannot be decoded as the
// requested representation and requires the same four things: the typed error
// through the leaf's operation-name wrap, the served bytes in Body, the
// request's own method and route template, and the codec's diagnostics still
// reachable behind Unwrap.
//
// The two getVersion rows are the point of the table. Both arms carry the
// FULL response bytes in Body — the data arm does not narrow Body to the
// `data` sub-slice it was decoding — so the consumer's recovery story is the
// same whichever arm failed.
func TestDemographicDecodeFailuresAreTyped(t *testing.T) { // REQ-151
	dataArmBody := string(originalVersionEnvelope([]byte(unregisteredParty)))

	cases := []struct {
		name  string
		body  string
		route string
		call  func(t *testing.T, c *transport.Client) (any, *openehrclient.VersionMetadata, error)
	}{
		{
			name:  "GetPartyBody",
			body:  unregisteredParty,
			route: "/demographic/person/{uid_based_id}",
			call: func(t *testing.T, c *transport.Client) (any, *openehrclient.VersionMetadata, error) {
				party, meta, err := demographic.Get(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID))
				return party, meta, err
			},
		},
		{
			name:  "GetVersionEnvelope",
			body:  brokenEnvelope,
			route: "/demographic/versioned_party/{versioned_object_uid}/version",
			call: func(t *testing.T, c *transport.Client) (any, *openehrclient.VersionMetadata, error) {
				pv, meta, err := demographic.GetVersion(t.Context(), c, personVOID)
				return pv, meta, err
			},
		},
		{
			name:  "GetVersionData",
			body:  dataArmBody,
			route: "/demographic/versioned_party/{versioned_object_uid}/version",
			call: func(t *testing.T, c *transport.Client) (any, *openehrclient.VersionMetadata, error) {
				pv, meta, err := demographic.GetVersion(t.Context(), c, personVOID)
				return pv, meta, err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, meta, err := tc.call(t, serve(t, tc.body))
			if err == nil {
				t.Fatalf("%s decoded %s without error; the premise of this test is gone", tc.name, tc.body)
			}
			if !isNil(out) {
				t.Errorf("%s returned %+v beside the decode error; a failed decode has nothing to hand back", tc.name, out)
			}
			// REQ-151 § Metadata still arrives.
			if meta == nil {
				t.Fatalf("%s returned nil metadata beside the decode error; REQ-151 § Metadata still arrives requires the response headers survive", tc.name)
			}
			if got := meta.ETag; got != personVersion {
				t.Errorf("VersionMetadata.ETag = %q, want %q — the headers must survive the decode failure intact", got, personVersion)
			}

			de, ok := errors.AsType[*transport.DecodeError](err)
			if !ok {
				t.Fatalf("errors.AsType[*transport.DecodeError] did not match %T (%v); REQ-151 requires a 2xx decode failure be recoverable as that type, hand-rolled decode or not", err, err)
			}
			if got := string(de.Body); got != tc.body {
				t.Errorf("DecodeError.Body = %q, want the full response bytes the server delivered, %q", got, tc.body)
			}
			if de.Method != http.MethodGet {
				t.Errorf("DecodeError.Method = %q, want %q", de.Method, http.MethodGet)
			}
			if de.Route != tc.route {
				t.Errorf("DecodeError.Route = %q, want the route template %q, not the expanded path", de.Route, tc.route)
			}
			if de.Unwrap() == nil {
				t.Error("DecodeError.Unwrap() = nil, want the decoder's error — REQ-151 requires the codec diagnostics stay reachable")
			}
			if op := "demographic"; !strings.HasPrefix(err.Error(), op) {
				t.Errorf("error message %q does not start with %q; the leaf's wrap must survive the conversion", err.Error(), op)
			}
		})
	}
}

// isNil reports whether an any holding one of the leaves' return types is
// absent. The leaves return rm.Party (an interface) and *PartyVersion, so a
// plain `out != nil` on the any would be true for a boxed typed nil; the type
// switch keeps this reflection-free (REQ-024).
func isNil(out any) bool {
	switch v := out.(type) {
	case nil:
		return true
	case *demographic.PartyVersion:
		return v == nil
	default:
		return false
	}
}

// TestGetVersionDataArmCarriesFullBody says out loud what the table above
// asserts in passing, because it is the one design decision of this phase that
// a future reader could plausibly get wrong: the VERSION data arm was decoding
// only the envelope's `data` sub-slice, but DecodeError.Body carries the whole
// response the server sent. REQ-151 § The typed error requires "the raw
// response bytes", and a caller who has to re-read the payload should not have
// to care which of getVersion's two decodes tripped.
func TestGetVersionDataArmCarriesFullBody(t *testing.T) { // REQ-151
	full := originalVersionEnvelope([]byte(unregisteredParty))

	_, _, err := demographic.GetVersion(t.Context(), serve(t, string(full)), personVOID)
	de, ok := errors.AsType[*transport.DecodeError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.DecodeError] did not match %T (%v)", err, err)
	}
	if got := string(de.Body); got != string(full) {
		t.Fatalf("DecodeError.Body = %q, want the full response %q", got, full)
	}
	if got := string(de.Body); got == unregisteredParty {
		t.Fatal("DecodeError.Body carries only the `data` sub-slice; REQ-151 wants the bytes the server delivered")
	}
	// The registry's own sentinel is still reachable underneath, so callers
	// already keying on it are unaffected by the conversion.
	if !errors.Is(err, typereg.ErrUnknownType) {
		t.Errorf("typereg.ErrUnknownType no longer reachable through the typed error: %v", err)
	}
}

// TestGetPartyEmptyBodyKeepsInvalidShape and its 204 sibling hold REQ-151's
// keyed exclusion at this package: an empty 2xx body is an absent body, not an
// unusable one, so it keeps today's contract and must never become a
// *transport.DecodeError.
func TestGetPartyEmptyBodyKeepsInvalidShape(t *testing.T) { // REQ-151
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, _, err := demographic.Get(t.Context(), newClient(t, srv), demographic.Person, openehrclient.LatestOf(personVOID))
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("err = %v, want transport.ErrInvalidShape on an empty 200 body", err)
	}
	if _, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Error("an empty 2xx body produced a *transport.DecodeError; REQ-151's keyed exclusion keeps this arm on ErrInvalidShape")
	}
}

// TestGetVersion204IsNotDecodeError pins the legitimate no-body success: a 204
// on a version read is "no version at that time", a nil-error nil-version
// answer, and it must not be dragged into this requirement either.
func TestGetVersion204IsNotDecodeError(t *testing.T) { // REQ-151
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	pv, meta, err := demographic.GetVersion(t.Context(), newClient(t, srv), personVOID)
	if err != nil {
		t.Fatalf("204 version read: unexpected error %v; a 204 is a legitimate no-version success", err)
	}
	if pv != nil {
		t.Errorf("204 version read returned %+v, want nil", pv)
	}
	if meta == nil {
		t.Error("204 version read returned nil metadata")
	}
}

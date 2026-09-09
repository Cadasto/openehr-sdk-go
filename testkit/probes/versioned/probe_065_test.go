package versionedprobes_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/versioned"
)

// probe065Backend serves the minimal-return round trip: POST answers 201 with
// an empty body (per return=minimal), asserting the SDK sent that Prefer, and a
// Location naming the new version; GET answers getStatus with getBody. The
// withLocation and getStatus/getBody knobs let the can-fail tests strip each
// half of the contract.
func probe065Backend(t *testing.T, withLocation bool, getStatus int, getBody string) *sandbox.Backend {
	t.Helper()
	return sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if p := r.Header.Get("Prefer"); p != "return=minimal" {
				t.Errorf("POST Prefer = %q, want return=minimal (the SDK write default)", p)
			}
			if withLocation {
				w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
				w.Header().Set("ETag", `"`+string(initialVUID)+`"`)
			}
			w.WriteHeader(http.StatusCreated) // return=minimal: no body
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(getStatus)
			_, _ = w.Write([]byte(getBody))
		default:
			t.Errorf("unexpected method %q", r.Method)
		}
	})
}

func TestProbe065MinimalReturnRoundTrip(t *testing.T) { // PROBE-065, REQ-094
	b := probe065Backend(t, true, http.StatusOK, bareCompositionBody)
	r, err := probes.Probe065MinimalReturnRoundTrip(context.Background(), newClient(t, b), ehrIDFixture, &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-065 status = %q (detail: %s)", r.Status, r.Detail)
	}
}

// TestProbe065FlagsMissingLocation: a minimal write with no Location leaves the
// caller unable to name what it committed — the probe must fail.
func TestProbe065FlagsMissingLocation(t *testing.T) { // PROBE-065
	b := probe065Backend(t, false, http.StatusOK, bareCompositionBody)
	r, err := probes.Probe065MinimalReturnRoundTrip(context.Background(), newClient(t, b), ehrIDFixture, &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-065 status = %q, want fail when the minimal write carried no Location (detail=%q)", r.Status, r.Detail)
	}
}

// TestProbe065FlagsUnreadableCommit: the committed version must read back in
// full; a GET that 404s is a broken round trip.
func TestProbe065FlagsUnreadableCommit(t *testing.T) { // PROBE-065
	b := probe065Backend(t, true, http.StatusNotFound, `{"message":"no such composition"}`)
	r, err := probes.Probe065MinimalReturnRoundTrip(context.Background(), newClient(t, b), ehrIDFixture, &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-065 status = %q, want fail when the committed version could not be read back (detail=%q)", r.Status, r.Detail)
	}
}

func TestProbe065RejectsMissingInputs(t *testing.T) { // PROBE-065
	b := probe065Backend(t, true, http.StatusOK, bareCompositionBody)
	if _, err := probes.Probe065MinimalReturnRoundTrip(context.Background(), nil, ehrIDFixture, &rm.Composition{}); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := probes.Probe065MinimalReturnRoundTrip(context.Background(), newClient(t, b), "", &rm.Composition{}); err == nil {
		t.Error("empty ehr id: want an error")
	}
	if _, err := probes.Probe065MinimalReturnRoundTrip(context.Background(), newClient(t, b), ehrIDFixture, nil); err == nil {
		t.Error("nil composition: want an error")
	}
}

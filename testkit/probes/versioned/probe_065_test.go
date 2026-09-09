package versionedprobes_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/versioned"
)

// probe065Backend serves the minimal-return round trip: POST answers 201 with
// an empty body (per return=minimal), asserting the SDK sent that Prefer, and a
// Location naming the new version; GET answers getStatus with getBody, but
// only for the version the POST named — any other path 404s, so a probe that
// mis-parsed the Location into the wrong VersionUID fails here instead of
// passing against a path-blind fake. The withLocation and getStatus/getBody
// knobs let the can-fail tests strip each half of the contract.
//
// Not planted, deliberately: a "201 with a body on the minimal path" case. On
// PreferMinimal, ehr.WriteResult returns metadata only without ever reading
// the body (openehr/client/ehr/write.go), so no backend response can make the
// probe's `out != nil` guard fire. That guard records the SDK contract and is
// unit-covered where the decode decision is made.
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
			if !strings.HasSuffix(r.URL.Path, "/composition/"+string(initialVUID)) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"no such composition"}`))
				return
			}
			w.WriteHeader(getStatus)
			_, _ = w.Write([]byte(getBody))
		default:
			t.Errorf("unexpected method %q", r.Method)
		}
	})
}

func TestProbe065MinimalReturnRoundTrip(t *testing.T) { // PROBE-065, REQ-094
	b := probe065Backend(t, true, http.StatusOK, bareCompositionBody)
	r, err := probes.Probe065MinimalReturnRoundTrip(t.Context(), newClient(t, b), ehrIDFixture, &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-065 status = %q (detail: %s)", r.Status, r.Detail)
	}
}

// TestProbe065FlagsBrokenRoundTrip breaks one half of the contract per case —
// the write that names nothing, the committed version that cannot be read
// back, and the read-back that is not the full resource — and pins the
// substring each Detail must carry, so a case cannot rot into failing for an
// unrelated reason.
func TestProbe065FlagsBrokenRoundTrip(t *testing.T) { // PROBE-065
	cases := []struct {
		name         string
		withLocation bool
		getStatus    int
		getBody      string
		wantDetail   string
	}{
		{
			name:         "minimal write carried no Location",
			withLocation: false,
			getStatus:    http.StatusOK,
			getBody:      bareCompositionBody,
			wantDetail:   "no usable Location",
		},
		{
			name:         "committed version cannot be read back",
			withLocation: true,
			getStatus:    http.StatusNotFound,
			getBody:      `{"message":"no such composition"}`,
			wantDetail:   "GET of the committed version",
		},
		{
			name:         "read-back is not the full resource",
			withLocation: true,
			getStatus:    http.StatusOK,
			getBody:      `{"_type":"COMPOSITION"}`,
			wantDetail:   "archetype_node_id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := probe065Backend(t, tc.withLocation, tc.getStatus, tc.getBody)
			r, err := probes.Probe065MinimalReturnRoundTrip(t.Context(), newClient(t, b), ehrIDFixture, &rm.Composition{})
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "fail" {
				t.Fatalf("PROBE-065 status = %q, want fail (detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.wantDetail) {
				t.Errorf("PROBE-065 failed for the wrong reason:\n got: %s\nwant it to mention: %s", r.Detail, tc.wantDetail)
			}
		})
	}
}

func TestProbe065RejectsMissingInputs(t *testing.T) { // PROBE-065
	b := probe065Backend(t, true, http.StatusOK, bareCompositionBody)
	if _, err := probes.Probe065MinimalReturnRoundTrip(t.Context(), nil, ehrIDFixture, &rm.Composition{}); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := probes.Probe065MinimalReturnRoundTrip(t.Context(), newClient(t, b), "", &rm.Composition{}); err == nil {
		t.Error("empty ehr id: want an error")
	}
	if _, err := probes.Probe065MinimalReturnRoundTrip(t.Context(), newClient(t, b), ehrIDFixture, nil); err == nil {
		t.Error("nil composition: want an error")
	}
}

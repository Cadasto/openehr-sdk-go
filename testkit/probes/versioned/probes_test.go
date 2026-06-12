package versionedprobes_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/versioned"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const (
	ehrIDFixture    openehrclient.EHRID             = "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"
	compositionVOID openehrclient.VersionedObjectID = "1234abcd-5678-9012-3456-7890abcdef00"
	initialVUID     openehrclient.VersionUID        = "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::1"
	updatedVUID     openehrclient.VersionUID        = "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::2"
)

func newClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestProbe010PutWithoutIfMatch(t *testing.T) {
	// PROBE-010 is a compile-time guard exercise — no network needed.
	// Construct a client against any throwaway server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("PROBE-010 must short-circuit before any network call")
	}))
	defer srv.Close()
	r, err := probes.Probe010PutWithoutIfMatch(context.Background(), newClient(t, srv), ehrIDFixture)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-010 status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe011PutStaleIfMatch_412(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"message":"stale","code":"PRECONDITION_FAILED"}`))
	}))
	defer srv.Close()
	r, err := probes.Probe011PutStaleIfMatch(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, "stale", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-011 (412) status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe011PutStaleIfMatch_409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"stale","code":"VERSION_CONFLICT"}`))
	}))
	defer srv.Close()
	r, err := probes.Probe011PutStaleIfMatch(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, "stale", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-011 (409) status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe012ETagRoundTrip(t *testing.T) {
	// Phase 1 of the probe: a GET returns the initial VersionUID via
	// Location; phase 2: PUT carries that as If-Match and the server
	// returns a fresh VersionUID. The fake server below alternates
	// between the two phases on a single shared state.
	var phase int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		phase++
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
			w.Header().Set("ETag", `"`+string(initialVUID)+`"`)
			// Body must be a minimal valid composition for canjson decode.
			_, _ = w.Write([]byte(`{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"},"archetype_node_id":"openEHR-EHR-COMPOSITION.x.v1","language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},"territory":{"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}}}`))
		case http.MethodPut:
			ifMatch := r.Header.Get("If-Match")
			if ifMatch != `"`+string(initialVUID)+`"` {
				t.Errorf("PUT If-Match = %q, want %q", ifMatch, `"`+string(initialVUID)+`"`)
			}
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(updatedVUID))
			w.Header().Set("ETag", `"`+string(updatedVUID)+`"`)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected method %q", r.Method)
		}
	}))
	defer srv.Close()

	r, err := probes.Probe012ETagRoundTrip(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-012 status = %q (detail: %s)", r.Status, r.Detail)
	}
	if phase < 2 {
		t.Errorf("expected at least 2 server hits (GET + PUT), got %d", phase)
	}
}

// bareCompositionBody is a minimal canonical-JSON COMPOSITION body
// used by PROBE-071 server fakes — keeps the canjson decode path
// honest while staying inline-readable.
const bareCompositionBody = `{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"},"archetype_node_id":"openEHR-EHR-COMPOSITION.x.v1","language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},"territory":{"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}}}`

func TestProbe071CompositionWriteResponseShape_BareBody_POSTOnly(t *testing.T) {
	// Happy path, POST-only: caller omits voID/ifMatch so the PUT arm
	// is skipped. The probe still passes on a clean bare-body decode.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
		w.Header().Set("ETag", `"`+string(initialVUID)+`"`)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(bareCompositionBody))
	}))
	defer srv.Close()

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, srv), ehrIDFixture, "", "", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-071 POST-only status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe071CompositionWriteResponseShape_BareBody_POSTPlusPUT(t *testing.T) {
	// Happy path, both arms: caller supplies voID + ifMatch so the
	// PUT arm runs. Server returns a bare COMPOSITION on both verbs.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
			w.Header().Set("ETag", `"`+string(initialVUID)+`"`)
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(updatedVUID))
			w.Header().Set("ETag", `"`+string(updatedVUID)+`"`)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected method %q", r.Method)
		}
		_, _ = w.Write([]byte(bareCompositionBody))
	}))
	defer srv.Close()

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, string(initialVUID), &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-071 POST+PUT status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe071CompositionWriteResponseShape_RejectsOriginalVersion_POST(t *testing.T) {
	// Non-conformant deployment: server returns ORIGINAL_VERSION on
	// POST. The strict-against-spec SDK MUST decode-fail; the probe
	// reports that as fail status (server side is the bug).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"x::y::1"},"data":{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"}}}`))
	}))
	defer srv.Close()

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, srv), ehrIDFixture, "", "", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-071 POST-OV-envelope status = %q (expected fail, detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe071CompositionWriteResponseShape_RejectsOriginalVersion_PUT(t *testing.T) {
	// Non-conformant deployment on the PUT path: POST returns the
	// spec-correct bare body but PUT returns ORIGINAL_VERSION. The
	// probe must fail on the PUT arm and surface the asymmetry.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
			w.Header().Set("ETag", `"`+string(initialVUID)+`"`)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(bareCompositionBody))
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"x::y::1"},"data":{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"}}}`))
		default:
			t.Errorf("unexpected method %q", r.Method)
		}
	}))
	defer srv.Close()

	r, err := probes.Probe071CompositionWriteResponseShape(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, string(initialVUID), &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-071 PUT-OV-envelope status = %q (expected fail, detail: %s)", r.Status, r.Detail)
	}
}

// newOriginalVersionFixture builds a minimal ORIGINAL_VERSION<COMPOSITION>
// for the PROBE-072 server fakes — the probe doesn't care about clinical
// content, only the wire shape, so ArchetypeNodeID is the only Composition
// field set. Returns the contribution write-side wrapper so commit_audit
// drops the server-assigned time_committed.
func newOriginalVersionFixture() *contribution.OriginalVersion[rm.Composition] {
	name := "alice"
	audit := rm.AuditDetails{
		SystemID:  "cdr.example",
		Committer: &rm.PartyIdentified{Name: &name},
		ChangeType: rm.DVCodedText{
			DVText:       rm.DVText{Value: "creation"},
			DefiningCode: rm.CodePhrase{CodeString: "249"},
		},
		TimeCommitted: rm.DVDateTime{Value: "2026-05-17T10:00:00Z"},
	}
	comp := rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1"}
	return contribution.WrapOriginalVersion(&rm.OriginalVersion[rm.Composition]{
		Version:        rm.Version[rm.Composition]{CommitAudit: audit},
		UID:            rm.ObjectVersionID{Value: "1::cdr.example::1"},
		LifecycleState: rm.DVCodedText{DVText: rm.DVText{Value: "complete"}, DefiningCode: rm.CodePhrase{CodeString: "532"}},
		Data:           &comp,
	})
}

func TestProbe072ContributionSubmissionShapePass(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	ov := newOriginalVersionFixture()
	sub := &contribution.Submission{
		Audit:    ov.CommitAudit,
		Versions: []contribution.CommitVersion{ov},
	}
	r, err := probes.Probe072ContributionSubmissionShape(context.Background(), newClient(t, srv), &capturedBody, ehrIDFixture, sub)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-072 status = %q detail=%q", r.Status, r.Detail)
	}
}

func TestProbe072ContributionSubmissionShapeRejectsObjectRef(t *testing.T) {
	// Server fake reads (and ignores) the real Commit request body and
	// plants the regression-shape — a persisted rm.Contribution with
	// versions[] of OBJECT_REF — into the captured slot. The probe
	// inspects *capturedBody, so it sees the planted body and MUST
	// flag the SDK-GAP-10 regression.
	planted := []byte(`{"_type":"CONTRIBUTION","audit":{"_type":"AUDIT_DETAILS","system_id":"x"},"versions":[{"_type":"OBJECT_REF","id":{"_type":"OBJECT_VERSION_ID","value":"1::x::1"}}]}`)
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		captured = planted
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	ov := newOriginalVersionFixture()
	sub := &contribution.Submission{
		Audit:    ov.CommitAudit,
		Versions: []contribution.CommitVersion{ov},
	}
	r, err := probes.Probe072ContributionSubmissionShape(context.Background(), newClient(t, srv), &captured, ehrIDFixture, sub)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("PROBE-072 status = %q (expected fail for OBJECT_REF body, detail=%q)", r.Status, r.Detail)
	}
}

func TestProbe013CrossEHRIsolation(t *testing.T) {
	const (
		ehrAID          openehrclient.EHRID      = "ehrA-1111-2222-3333-444444444444"
		ehrBID          openehrclient.EHRID      = "ehrB-aaaa-bbbb-cccc-dddddddddddd"
		versionUIDFromA openehrclient.VersionUID = "9999abcd-5678-9012-3456-7890abcdef00::cdr.example::1"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Tenant-isolated server: any composition GET under ehrBID for a
		// VersionUID that doesn't belong to ehrBID is a hard 404. The
		// probe MUST NOT see EHR A's id or data on this path.
		if got := r.URL.Path; !contains(got, string(ehrBID)) {
			t.Errorf("expected request path to target ehrBID, got %q", got)
		}
		if contains(r.URL.Path, string(ehrAID)) {
			t.Errorf("path should NOT contain ehrAID, got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found","code":"NOT_FOUND"}`))
	}))
	defer srv.Close()
	r, err := probes.Probe013CrossEHRIsolation(context.Background(), newClient(t, srv), ehrAID, ehrBID, versionUIDFromA)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-013 status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe013RejectsTenantLeak(t *testing.T) {
	// Negative branch: a server that returns 200 for the cross-EHR
	// read MUST be flagged as a tenant leak by the probe.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"leak"},"archetype_node_id":"openEHR-EHR-COMPOSITION.x.v1","language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},"territory":{"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}}}`))
	}))
	defer srv.Close()
	r, err := probes.Probe013CrossEHRIsolation(context.Background(), newClient(t, srv), "ehrA", "ehrB", "vuid")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("expected fail on 200 cross-EHR read, got %q", r.Status)
	}
}

// contains is a local micro-helper to keep the cross-EHR isolation
// test self-contained (no `strings` import churn in this file).
func contains(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

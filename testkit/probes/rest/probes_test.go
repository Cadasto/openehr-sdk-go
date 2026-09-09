package restprobes_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	restprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/rest"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func newClient(t *testing.T, b *sandbox.Backend) *transport.Client {
	t.Helper()
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", b.HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// capturing records every request the backend receives (as a clone) and lets
// handler render the response. mutate, when non-nil, rewrites the recorded
// clone so a can-fail test can plant a mis-shaped request the SDK did not send.
func capturing(mutate func(*http.Request), handler http.HandlerFunc) (*sandbox.Backend, func() []*http.Request) {
	var (
		mu   sync.Mutex
		reqs []*http.Request
	)
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		clone := r.Clone(r.Context())
		if mutate != nil {
			mutate(clone)
		}
		mu.Lock()
		reqs = append(reqs, clone)
		mu.Unlock()
		handler(w, r)
	})
	snap := func() []*http.Request {
		mu.Lock()
		defer mu.Unlock()
		return append([]*http.Request(nil), reqs...)
	}
	return b, snap
}

// --- shared response fixtures ------------------------------------------------

const capsBody = `{"solution":"test","vendor":"test","restapi_specs_version":"1.1.0-development"}`

const exampleCompositionBody = `{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"example"},"archetype_node_id":"openEHR-EHR-COMPOSITION.report.v1","language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},"territory":{"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}}}`

const ehrStatusJSON = `{"_type":"EHR_STATUS","name":{"_type":"DV_TEXT","value":"EHR Status"},"archetype_node_id":"openEHR-EHR-EHR_STATUS.generic.v1","subject":{"_type":"PARTY_SELF"},"is_queryable":true,"is_modifiable":false}`

// ehrStatusNotQueryable differs from ehrStatusJSON only in is_queryable, for the
// PROBE-060 round-trip-mismatch can-fail plant.
const ehrStatusNotQueryable = `{"_type":"EHR_STATUS","name":{"_type":"DV_TEXT","value":"EHR Status"},"archetype_node_id":"openEHR-EHR-EHR_STATUS.generic.v1","subject":{"_type":"PARTY_SELF"},"is_queryable":false,"is_modifiable":false}`

func ehrBody(id string) string {
	return `{"_type":"EHR","ehr_id":{"_type":"HIER_OBJECT_ID","value":"` + id + `"},` +
		`"system_id":{"_type":"HIER_OBJECT_ID","value":"cdr.example"},` +
		`"time_created":{"_type":"DV_DATE_TIME","value":"2026-01-01T00:00:00Z"},` +
		`"ehr_access":{"_type":"OBJECT_REF","id":{"_type":"OBJECT_VERSION_ID","value":"a::cdr.example::1"},"namespace":"local","type":"EHR_ACCESS"},` +
		`"ehr_status":{"_type":"OBJECT_REF","id":{"_type":"OBJECT_VERSION_ID","value":"b::cdr.example::1"},"namespace":"local","type":"EHR_STATUS"}}`
}

func contributionBody(uid, systemID string) string {
	return `{"_type":"CONTRIBUTION","uid":{"_type":"HIER_OBJECT_ID","value":"` + uid + `"},` +
		`"audit":{"_type":"AUDIT_DETAILS","system_id":"` + systemID + `",` +
		`"committer":{"_type":"PARTY_IDENTIFIED","name":"Alice"},` +
		`"change_type":{"_type":"DV_CODED_TEXT","value":"modification","defining_code":{"_type":"CODE_PHRASE","code_string":"251","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}},` +
		`"time_committed":{"_type":"DV_DATE_TIME","value":"2026-01-01T00:00:00Z"}},` +
		`"versions":[{"_type":"OBJECT_REF","id":{"_type":"OBJECT_VERSION_ID","value":"x::cdr.example::1"},"namespace":"local","type":"COMPOSITION"}]}`
}

// --- PROBE-102 System capabilities ------------------------------------------

func writeJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func TestProbe102SystemCapabilities(t *testing.T) { // PROBE-102, REQ-095
	b, snap := capturing(nil, writeJSON(capsBody))
	r, err := restprobes.Probe102SystemCapabilities(t.Context(), newClient(t, b), snap)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-102 = %s: %s", r.Status, r.Detail)
	}
}

func TestProbe102FlagsWrongVerb(t *testing.T) { // PROBE-102
	b, snap := capturing(func(req *http.Request) { req.Method = http.MethodGet }, writeJSON(capsBody))
	r, _ := restprobes.Probe102SystemCapabilities(t.Context(), newClient(t, b), snap)
	if r.Status != "fail" || !strings.Contains(r.Detail, "OPTIONS") {
		t.Fatalf("PROBE-102 = %s (%q), want fail naming OPTIONS", r.Status, r.Detail)
	}
}

func TestProbe102FlagsMissingVersion(t *testing.T) { // PROBE-102
	b, snap := capturing(nil, writeJSON(`{"solution":"test"}`))
	r, _ := restprobes.Probe102SystemCapabilities(t.Context(), newClient(t, b), snap)
	if r.Status != "fail" || !strings.Contains(r.Detail, "restapi_specs_version") {
		t.Fatalf("PROBE-102 = %s (%q), want fail naming restapi_specs_version", r.Status, r.Detail)
	}
}

func TestProbe102RejectsMissingInputs(t *testing.T) { // PROBE-102
	b, snap := capturing(nil, writeJSON(capsBody))
	if _, err := restprobes.Probe102SystemCapabilities(t.Context(), nil, snap); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := restprobes.Probe102SystemCapabilities(t.Context(), newClient(t, b), nil); err == nil {
		t.Error("nil recorder: want an error")
	}
}

// --- PROBE-103 Admin bulk-delete --------------------------------------------

func status204() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
}

func TestProbe103AdminBulkDelete(t *testing.T) { // PROBE-103, REQ-099
	ids := []openehrclient.EHRID{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}
	b, snap := capturing(nil, status204())
	r, err := restprobes.Probe103AdminBulkDelete(t.Context(), newClient(t, b), snap, ids)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-103 = %s: %s", r.Status, r.Detail)
	}
}

func TestProbe103FlagsMissingEHRIDParams(t *testing.T) { // PROBE-103
	// Plant a per-path-segment delete (the regression PROBE-103 guards against):
	// drop the ehr_id query parameters the subset delete requires.
	strip := func(req *http.Request) {
		q := req.URL.Query()
		q.Del("ehr_id")
		req.URL.RawQuery = q.Encode()
	}
	b, snap := capturing(strip, status204())
	r, _ := restprobes.Probe103AdminBulkDelete(t.Context(), newClient(t, b), snap, []openehrclient.EHRID{"id-1"})
	if r.Status != "fail" || !strings.Contains(r.Detail, "ehr_id query parameters") {
		t.Fatalf("PROBE-103 = %s (%q), want fail naming the ehr_id parameters", r.Status, r.Detail)
	}
}

func TestProbe103RejectsMissingInputs(t *testing.T) { // PROBE-103
	b, snap := capturing(nil, status204())
	ids := []openehrclient.EHRID{"id-1"}
	if _, err := restprobes.Probe103AdminBulkDelete(t.Context(), nil, snap, ids); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := restprobes.Probe103AdminBulkDelete(t.Context(), newClient(t, b), nil, ids); err == nil {
		t.Error("nil recorder: want an error")
	}
	if _, err := restprobes.Probe103AdminBulkDelete(t.Context(), newClient(t, b), snap, nil); err == nil {
		t.Error("empty ids: want an error")
	}
}

// --- PROBE-104 Definition example -------------------------------------------

func TestProbe104DefinitionExample(t *testing.T) { // PROBE-104, REQ-095
	b, snap := capturing(nil, writeJSON(exampleCompositionBody))
	r, err := restprobes.Probe104DefinitionExample(t.Context(), newClient(t, b), snap, "vital_signs.v1", definition.FormatADL14)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-104 = %s: %s", r.Status, r.Detail)
	}
}

func TestProbe104FlagsEmptyArchetype(t *testing.T) { // PROBE-104
	b, snap := capturing(nil, writeJSON(`{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"}}`))
	r, _ := restprobes.Probe104DefinitionExample(t.Context(), newClient(t, b), snap, "vital_signs.v1", definition.FormatADL14)
	if r.Status != "fail" || !strings.Contains(r.Detail, "archetype_node_id") {
		t.Fatalf("PROBE-104 = %s (%q), want fail naming archetype_node_id", r.Status, r.Detail)
	}
}

func TestProbe104RejectsMissingInputs(t *testing.T) { // PROBE-104
	b, snap := capturing(nil, writeJSON(exampleCompositionBody))
	if _, err := restprobes.Probe104DefinitionExample(t.Context(), nil, snap, "t.v1", definition.FormatADL14); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := restprobes.Probe104DefinitionExample(t.Context(), newClient(t, b), nil, "t.v1", definition.FormatADL14); err == nil {
		t.Error("nil recorder: want an error")
	}
	if _, err := restprobes.Probe104DefinitionExample(t.Context(), newClient(t, b), snap, "", definition.FormatADL14); err == nil {
		t.Error("empty templateID: want an error")
	}
}

// --- PROBE-060 EHR creation round trip --------------------------------------

const probe060EHRID = "33333333-3333-4333-8333-333333333333"

// ehrRoundTripBackend answers POST /ehr with an EHR carrying probe060EHRID and
// GET …/ehr_status with statusJSON.
func ehrRoundTripBackend(t *testing.T, statusJSON string) (*sandbox.Backend, func() []*http.Request) {
	t.Helper()
	return capturing(nil, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/ehr"):
			w.Header().Set("Location", "/ehr/"+probe060EHRID)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(ehrBody(probe060EHRID)))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/ehr_status"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(statusJSON))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}

func decodeStatus(t *testing.T) *rm.EHRStatus {
	t.Helper()
	var st rm.EHRStatus
	if err := canjson.Unmarshal([]byte(ehrStatusJSON), &st); err != nil {
		t.Fatalf("decode EHR_STATUS fixture: %v", err)
	}
	return &st
}

func TestProbe060EHRCreationRoundTrip(t *testing.T) { // PROBE-060, REQ-095
	b, snap := ehrRoundTripBackend(t, ehrStatusJSON)
	r, err := restprobes.Probe060EHRCreationRoundTrip(t.Context(), newClient(t, b), snap, decodeStatus(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-060 = %s: %s", r.Status, r.Detail)
	}
}

func TestProbe060FlagsStatusMismatch(t *testing.T) { // PROBE-060
	// The committed status is queryable; the read-back is not — a broken round
	// trip the probe must catch.
	b, snap := ehrRoundTripBackend(t, ehrStatusNotQueryable)
	r, _ := restprobes.Probe060EHRCreationRoundTrip(t.Context(), newClient(t, b), snap, decodeStatus(t))
	if r.Status != "fail" || !strings.Contains(r.Detail, "queryable") {
		t.Fatalf("PROBE-060 = %s (%q), want fail naming the queryable mismatch", r.Status, r.Detail)
	}
}

func TestProbe060RejectsMissingInputs(t *testing.T) { // PROBE-060
	b, snap := ehrRoundTripBackend(t, ehrStatusJSON)
	if _, err := restprobes.Probe060EHRCreationRoundTrip(t.Context(), nil, snap, decodeStatus(t)); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := restprobes.Probe060EHRCreationRoundTrip(t.Context(), newClient(t, b), snap, nil); err == nil {
		t.Error("nil status: want an error")
	}
	if _, err := restprobes.Probe060EHRCreationRoundTrip(t.Context(), newClient(t, b), nil, decodeStatus(t)); err == nil {
		t.Error("nil recorder: want an error")
	}
}

// --- PROBE-062 audit-details header -----------------------------------------

const (
	probe062EHRID      openehrclient.EHRID = "44444444-4444-4444-8444-444444444444"
	probe062ContribUID string              = "0826851c-c4c2-4d61-92b9-410fb8275ff0"
)

func probe062Audit() *rm.AuditDetails {
	name := "Alice"
	return &rm.AuditDetails{
		SystemID:   "cdr.example",
		Committer:  &rm.PartyIdentified{Name: &name},
		ChangeType: rm.DVCodedText{DVText: rm.DVText{Value: "modification"}, DefiningCode: rm.CodePhrase{CodeString: "251"}},
	}
}

// auditWriteBackend answers the composition write with a minimal 201 (Location
// only) and the contribution GET with a CONTRIBUTION whose audit carries
// systemID.
func auditWriteBackend(t *testing.T, mutate func(*http.Request), systemID string) (*sandbox.Backend, func() []*http.Request) {
	t.Helper()
	return capturing(mutate, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/composition"):
			w.Header().Set("Location", "/ehr/"+string(probe062EHRID)+"/composition/vuid::cdr.example::1")
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contribution/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(contributionBody(probe062ContribUID, systemID)))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}

func TestProbe062AuditDetailsHeader(t *testing.T) { // PROBE-062, REQ-059
	b, snap := auditWriteBackend(t, nil, "cdr.example")
	r, err := restprobes.Probe062AuditDetailsHeader(t.Context(), newClient(t, b), snap, probe062EHRID, probe062ContribUID, probe062Audit(), &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-062 = %s: %s", r.Status, r.Detail)
	}
}

func TestProbe062FlagsJSONHeader(t *testing.T) { // PROBE-062
	// Plant a JSON audit header on the write — the REQ-059 grammar is the dotted
	// form, not JSON.
	toJSON := func(req *http.Request) {
		if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/composition") {
			req.Header.Set("openehr-audit-details", `{"change_type":"251","committer":"Alice"}`)
		}
	}
	b, snap := auditWriteBackend(t, toJSON, "cdr.example")
	r, _ := restprobes.Probe062AuditDetailsHeader(t.Context(), newClient(t, b), snap, probe062EHRID, probe062ContribUID, probe062Audit(), &rm.Composition{})
	if r.Status != "fail" || !strings.Contains(r.Detail, "JSON") {
		t.Fatalf("PROBE-062 = %s (%q), want fail naming the JSON header", r.Status, r.Detail)
	}
}

func TestProbe062FlagsAuditMismatch(t *testing.T) { // PROBE-062
	// The read-back Contribution's audit carries a different system_id than the
	// one committed — the round-trip half must catch it.
	b, snap := auditWriteBackend(t, nil, "other.system")
	r, _ := restprobes.Probe062AuditDetailsHeader(t.Context(), newClient(t, b), snap, probe062EHRID, probe062ContribUID, probe062Audit(), &rm.Composition{})
	if r.Status != "fail" || !strings.Contains(r.Detail, "system_id") {
		t.Fatalf("PROBE-062 = %s (%q), want fail naming the system_id mismatch", r.Status, r.Detail)
	}
}

func TestProbe062RejectsMissingInputs(t *testing.T) { // PROBE-062
	b, snap := auditWriteBackend(t, nil, "cdr.example")
	c := newClient(t, b)
	if _, err := restprobes.Probe062AuditDetailsHeader(t.Context(), nil, snap, probe062EHRID, probe062ContribUID, probe062Audit(), &rm.Composition{}); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := restprobes.Probe062AuditDetailsHeader(t.Context(), c, nil, probe062EHRID, probe062ContribUID, probe062Audit(), &rm.Composition{}); err == nil {
		t.Error("nil recorder: want an error")
	}
	if _, err := restprobes.Probe062AuditDetailsHeader(t.Context(), c, snap, "", probe062ContribUID, probe062Audit(), &rm.Composition{}); err == nil {
		t.Error("empty ehr id: want an error")
	}
	if _, err := restprobes.Probe062AuditDetailsHeader(t.Context(), c, snap, probe062EHRID, probe062ContribUID, nil, &rm.Composition{}); err == nil {
		t.Error("nil audit: want an error")
	}
}

package restprobes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
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
// handler render the response. Each clone gets its own buffered copy of the
// request body — the live one is consumed once it is read — so a probe can
// inspect what the SDK put on the wire; the handler is handed a fresh reader
// over the same bytes. mutate, when non-nil, rewrites the recorded clone
// (never the live request, which is already served) so a can-fail test can
// plant a mis-shaped request the SDK did not send; it runs after the body is
// buffered, so it can rewrite that too.
func capturing(t *testing.T, mutate func(*http.Request), handler http.HandlerFunc) (*sandbox.Backend, func() []*http.Request) {
	t.Helper()
	var (
		mu   sync.Mutex
		reqs []*http.Request
	)
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil { // a GET or DELETE carries no body at all
			var err error
			if body, err = io.ReadAll(r.Body); err != nil {
				t.Errorf("reading the live %s %s request body: %v", r.Method, r.URL.Path, err)
			}
		}
		clone := r.Clone(r.Context())
		clone.Body = io.NopCloser(bytes.NewReader(body))
		clone.ContentLength = int64(len(body))
		if mutate != nil {
			mutate(clone)
		}
		mu.Lock()
		reqs = append(reqs, clone)
		mu.Unlock()
		r.Body = io.NopCloser(bytes.NewReader(body))
		handler(w, r)
	})
	snap := func() []*http.Request {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(reqs)
	}
	return b, snap
}

// setBody rewrites a captured clone's buffered body (and its declared length).
func setBody(r *http.Request, body string) {
	r.Body = io.NopCloser(strings.NewReader(body))
	r.ContentLength = int64(len(body))
}

// --- shared response fixtures ------------------------------------------------

const capsBody = `{"solution":"test","vendor":"test","restapi_specs_version":"1.1.0"}`

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

// contributionBody renders the CONTRIBUTION the PROBE-062 read-back fetches.
// committer and versionID are parameters because the probe binds both to the
// write: the committer name must match the one the audit header carried, and
// versionID must be the version uid the write's Location named.
func contributionBody(uid, systemID, committer, versionID string) string {
	return `{"_type":"CONTRIBUTION","uid":{"_type":"HIER_OBJECT_ID","value":"` + uid + `"},` +
		`"audit":{"_type":"AUDIT_DETAILS","system_id":"` + systemID + `",` +
		`"committer":{"_type":"PARTY_IDENTIFIED","name":"` + committer + `"},` +
		`"change_type":{"_type":"DV_CODED_TEXT","value":"modification","defining_code":{"_type":"CODE_PHRASE","code_string":"251","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}},` +
		`"time_committed":{"_type":"DV_DATE_TIME","value":"2026-01-01T00:00:00Z"}},` +
		`"versions":[{"_type":"OBJECT_REF","id":{"_type":"OBJECT_VERSION_ID","value":"` + versionID + `"},"namespace":"local","type":"COMPOSITION"}]}`
}

// --- PROBE-102 System capabilities ------------------------------------------

func writeJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func TestProbe102SystemCapabilities(t *testing.T) { // PROBE-102, REQ-095
	b, snap := capturing(t, nil, writeJSON(capsBody))
	r, err := restprobes.Probe102SystemCapabilities(t.Context(), newClient(t, b), snap)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-102 = %s: %s", r.Status, r.Detail)
	}
}

func TestProbe102FlagsWrongVerb(t *testing.T) { // PROBE-102
	b, snap := capturing(t, func(req *http.Request) { req.Method = http.MethodGet }, writeJSON(capsBody))
	r, err := restprobes.Probe102SystemCapabilities(t.Context(), newClient(t, b), snap)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" || !strings.Contains(r.Detail, "OPTIONS") {
		t.Fatalf("PROBE-102 = %s (%q), want fail naming OPTIONS", r.Status, r.Detail)
	}
}

// TestProbe102FlagsWrongPath moves the operation off the service root and onto
// a resource below it — the shape a deployment or SDK that answered
// capabilities at `/ehr` would produce.
func TestProbe102FlagsWrongPath(t *testing.T) { // PROBE-102
	descend := func(req *http.Request) {
		req.URL.Path = strings.TrimSuffix(req.URL.Path, "/") + "/ehr"
	}
	b, snap := capturing(t, descend, writeJSON(capsBody))
	r, err := restprobes.Probe102SystemCapabilities(t.Context(), newClient(t, b), snap)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" || !strings.Contains(r.Detail, "service root") {
		t.Fatalf("PROBE-102 = %s (%q), want fail naming the service root", r.Status, r.Detail)
	}
}

func TestProbe102FlagsMissingVersion(t *testing.T) { // PROBE-102
	b, snap := capturing(t, nil, writeJSON(`{"solution":"test"}`))
	r, err := restprobes.Probe102SystemCapabilities(t.Context(), newClient(t, b), snap)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" || !strings.Contains(r.Detail, "restapi_specs_version") {
		t.Fatalf("PROBE-102 = %s (%q), want fail naming restapi_specs_version", r.Status, r.Detail)
	}
}

func TestProbe102RejectsMissingInputs(t *testing.T) { // PROBE-102
	b, snap := capturing(t, nil, writeJSON(capsBody))
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
	b, snap := capturing(t, nil, status204())
	r, err := restprobes.Probe103AdminBulkDelete(t.Context(), newClient(t, b), snap, ids)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-103 = %s: %s", r.Status, r.Detail)
	}
}

// TestProbe103FlagsOffContractDeletes plants one off-contract shape per case,
// each landing on a different one of the probe's assertions — the subset must
// ride repeatable `ehr_id` query parameters on the exact `/admin/ehr/all`
// route, never a path segment per id and never a request body. Each case pins
// the substring its Detail must carry, so a case cannot rot into failing for
// an unrelated reason.
func TestProbe103FlagsOffContractDeletes(t *testing.T) { // PROBE-103
	const id = "id-1"
	cases := []struct {
		name       string
		mutate     func(*http.Request)
		wantDetail string
	}{
		{
			name: "subset moved off the ehr_id query parameters",
			// Strip the parameters the subset delete requires; nothing else
			// names the subset, so the delete has silently become "delete
			// every EHR".
			mutate: func(req *http.Request) {
				q := req.URL.Query()
				q.Del("ehr_id")
				req.URL.RawQuery = q.Encode()
			},
			wantDetail: "ehr_id query parameters",
		},
		{
			name: "one path segment per id",
			mutate: func(req *http.Request) {
				req.URL.Path = strings.TrimSuffix(req.URL.Path, "/") + "/" + id
			},
			wantDetail: "exact /admin/ehr/all path",
		},
		{
			name: "subset moved into a request body",
			mutate: func(req *http.Request) {
				setBody(req, `{"ehr_id":["`+id+`"]}`)
			},
			wantDetail: "request body",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, snap := capturing(t, tc.mutate, status204())
			r, err := restprobes.Probe103AdminBulkDelete(t.Context(), newClient(t, b), snap, []openehrclient.EHRID{id})
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "fail" {
				t.Fatalf("PROBE-103 = %s, want fail (detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.wantDetail) {
				t.Errorf("PROBE-103 failed for the wrong reason:\n got: %s\nwant it to mention: %s", r.Detail, tc.wantDetail)
			}
		})
	}
}

func TestProbe103RejectsMissingInputs(t *testing.T) { // PROBE-103
	b, snap := capturing(t, nil, status204())
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

const (
	probe104TemplateID = "vital_signs.v1"
	probe104Format     = definition.FormatADL14
)

func TestProbe104DefinitionExample(t *testing.T) { // PROBE-104, REQ-095
	b, snap := capturing(t, nil, writeJSON(exampleCompositionBody))
	r, err := restprobes.Probe104DefinitionExample(t.Context(), newClient(t, b), snap, probe104TemplateID, probe104Format)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-104 = %s: %s", r.Status, r.Detail)
	}
}

// replaceSegment rewrites the i-th path segment (counting from the end, 0 being
// the last) of req's path.
func replaceSegment(t *testing.T, req *http.Request, fromEnd int, to string) {
	t.Helper()
	segs := strings.Split(req.URL.Path, "/")
	i := len(segs) - 1 - fromEnd
	if i < 0 {
		t.Errorf("path %q has no segment %d from the end", req.URL.Path, fromEnd)
		return
	}
	segs[i] = to
	req.URL.Path = strings.Join(segs, "/")
}

// TestProbe104FlagsBrokenExampleRoute pins one broken shape per case: the
// format and template-id segments each carry meaning, so a route that landed
// on a different format or a different template is not this operation even
// though it still reads `…/definition/template/…/example`.
func TestProbe104FlagsBrokenExampleRoute(t *testing.T) { // PROBE-104
	cases := []struct {
		name       string
		mutate     func(*testing.T, *http.Request)
		body       string
		wantDetail string
	}{
		{
			name:       "wrong format segment",
			mutate:     func(t *testing.T, req *http.Request) { replaceSegment(t, req, 2, "adl2") },
			body:       exampleCompositionBody,
			wantDetail: "example route",
		},
		{
			name:       "wrong template id segment",
			mutate:     func(t *testing.T, req *http.Request) { replaceSegment(t, req, 1, "other.v1") },
			body:       exampleCompositionBody,
			wantDetail: "example route",
		},
		{
			name:       "body is not a full COMPOSITION",
			body:       `{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"}}`,
			wantDetail: "archetype_node_id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mutate func(*http.Request)
			if tc.mutate != nil {
				mutate = func(req *http.Request) { tc.mutate(t, req) }
			}
			b, snap := capturing(t, mutate, writeJSON(tc.body))
			r, err := restprobes.Probe104DefinitionExample(t.Context(), newClient(t, b), snap, probe104TemplateID, probe104Format)
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "fail" {
				t.Fatalf("PROBE-104 = %s, want fail (detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.wantDetail) {
				t.Errorf("PROBE-104 failed for the wrong reason:\n got: %s\nwant it to mention: %s", r.Detail, tc.wantDetail)
			}
		})
	}
}

func TestProbe104RejectsMissingInputs(t *testing.T) { // PROBE-104
	b, snap := capturing(t, nil, writeJSON(exampleCompositionBody))
	if _, err := restprobes.Probe104DefinitionExample(t.Context(), nil, snap, "t.v1", probe104Format); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := restprobes.Probe104DefinitionExample(t.Context(), newClient(t, b), nil, "t.v1", probe104Format); err == nil {
		t.Error("nil recorder: want an error")
	}
	if _, err := restprobes.Probe104DefinitionExample(t.Context(), newClient(t, b), snap, "", probe104Format); err == nil {
		t.Error("empty templateID: want an error")
	}
}

// --- PROBE-060 EHR creation round trip --------------------------------------

const (
	probe060EHRID      = "33333333-3333-4333-8333-333333333333"
	probe060OtherEHRID = "99999999-9999-4999-8999-999999999999"
)

// ehrRoundTripBackend answers POST /ehr with an EHR carrying probe060EHRID and
// GET …/ehr_status with statusJSON. withLocation controls whether the create
// names the new EHR in a Location header; mutate rewrites the recorded clone so
// a can-fail test can plant a broken create or read-back.
func ehrRoundTripBackend(t *testing.T, statusJSON string, withLocation bool, mutate func(*http.Request)) (*sandbox.Backend, func() []*http.Request) {
	t.Helper()
	return capturing(t, mutate, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/ehr"):
			if withLocation {
				w.Header().Set("Location", "/ehr/"+probe060EHRID)
			}
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
	b, snap := ehrRoundTripBackend(t, ehrStatusJSON, true, nil)
	r, err := restprobes.Probe060EHRCreationRoundTrip(t.Context(), newClient(t, b), snap, decodeStatus(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-060 = %s: %s", r.Status, r.Detail)
	}
}

// TestProbe060FlagsBrokenRoundTrip breaks one link of the round trip per case —
// the status that comes back, the Location that names the new EHR, the status
// that went out, and the route the read-back took. Each pins the substring its
// Detail must carry, so a probe that stopped checking one of them fails here
// rather than keeps reporting green.
func TestProbe060FlagsBrokenRoundTrip(t *testing.T) { // PROBE-060
	cases := []struct {
		name         string
		statusJSON   string
		withLocation bool
		mutate       func(*http.Request)
		wantDetail   string
	}{
		{
			// The committed status is queryable; the read-back is not.
			name:         "read-back status differs from the committed one",
			statusJSON:   ehrStatusNotQueryable,
			withLocation: true,
			wantDetail:   "queryable",
		},
		{
			name:         "create names the new EHR only in the body",
			statusJSON:   ehrStatusJSON,
			withLocation: false,
			wantDetail:   "no Location",
		},
		{
			name:         "create body is not the status that was handed in",
			statusJSON:   ehrStatusJSON,
			withLocation: true,
			mutate: func(req *http.Request) {
				if req.Method == http.MethodPost {
					setBody(req, `{}`)
				}
			},
			wantDetail: "submitted EHR_STATUS",
		},
		{
			name:         "read-back reads some other EHR",
			statusJSON:   ehrStatusJSON,
			withLocation: true,
			mutate: func(req *http.Request) {
				if req.Method == http.MethodGet {
					req.URL.Path = strings.Replace(req.URL.Path, probe060EHRID, probe060OtherEHRID, 1)
				}
			},
			wantDetail: "read-back path",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, snap := ehrRoundTripBackend(t, tc.statusJSON, tc.withLocation, tc.mutate)
			r, err := restprobes.Probe060EHRCreationRoundTrip(t.Context(), newClient(t, b), snap, decodeStatus(t))
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "fail" {
				t.Fatalf("PROBE-060 = %s, want fail (detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.wantDetail) {
				t.Errorf("PROBE-060 failed for the wrong reason:\n got: %s\nwant it to mention: %s", r.Detail, tc.wantDetail)
			}
		})
	}
}

func TestProbe060RejectsMissingInputs(t *testing.T) { // PROBE-060
	b, snap := ehrRoundTripBackend(t, ehrStatusJSON, true, nil)
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
	// probe062VersionUID is the version uid the fake's write Location names;
	// the happy-path CONTRIBUTION must list exactly it, because that binding
	// is what ties the read-back to the write.
	probe062VersionUID  = "vuid::cdr.example::1"
	probe062Committer   = "Alice"
	probe062SystemID    = "cdr.example"
	probe062AuditHeader = "openehr-audit-details"
)

func probe062Audit() *rm.AuditDetails {
	name := probe062Committer
	return &rm.AuditDetails{
		SystemID:   probe062SystemID,
		Committer:  &rm.PartyIdentified{Name: &name},
		ChangeType: rm.DVCodedText{DVText: rm.DVText{Value: "modification"}, DefiningCode: rm.CodePhrase{CodeString: "251"}},
	}
}

// contributionFixture is the read-back CONTRIBUTION a PROBE-062 case serves.
type contributionFixture struct {
	systemID  string
	committer string
	versionID string
}

func (f contributionFixture) body() string {
	return contributionBody(probe062ContribUID, f.systemID, f.committer, f.versionID)
}

func probe062Contribution() contributionFixture {
	return contributionFixture{systemID: probe062SystemID, committer: probe062Committer, versionID: probe062VersionUID}
}

// auditWriteBackend answers the composition write with a minimal 201 (Location
// only) and the contribution GET with fixture's CONTRIBUTION.
func auditWriteBackend(t *testing.T, mutate func(*http.Request), fixture contributionFixture) (*sandbox.Backend, func() []*http.Request) {
	t.Helper()
	return capturing(t, mutate, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/composition"):
			w.Header().Set("Location", "/ehr/"+string(probe062EHRID)+"/composition/"+probe062VersionUID)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contribution/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fixture.body()))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}

// onWriteHeader returns a mutate that rewrites the openehr-audit-details header
// on the composition write only, leaving the read-back request untouched.
func onWriteHeader(rewrite func(string) string) func(*http.Request) {
	return func(req *http.Request) {
		if req.Method != http.MethodPost || !strings.HasSuffix(req.URL.Path, "/composition") {
			return
		}
		req.Header.Set(probe062AuditHeader, rewrite(req.Header.Get(probe062AuditHeader)))
	}
}

func TestProbe062AuditDetailsHeader(t *testing.T) { // PROBE-062, REQ-059
	b, snap := auditWriteBackend(t, nil, probe062Contribution())
	r, err := restprobes.Probe062AuditDetailsHeader(t.Context(), newClient(t, b), snap, probe062EHRID, probe062ContribUID, probe062Audit(), &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-062 = %s: %s", r.Status, r.Detail)
	}
}

// TestProbe062FlagsBrokenContract breaks one clause of the contract per case.
// The first six plant a header the SDK did not send — a JSON object, a wrong
// separator, an unquoted value, a rewritten or dropped attribute, an
// undocumented key — so the independent parser and the value checks each get a
// case that only they can catch. The last three break the read-back: a
// different system_id, a different committer, and a CONTRIBUTION that lists
// some other version, which is the binding between the write and the read-back.
func TestProbe062FlagsBrokenContract(t *testing.T) { // PROBE-062
	cases := []struct {
		name       string
		mutate     func(*http.Request)
		fixture    contributionFixture
		wantDetail string
	}{
		{
			name:       "header is a JSON object",
			mutate:     onWriteHeader(func(string) string { return `{"change_type":"251","committer":"Alice"}` }),
			wantDetail: "JSON",
		},
		{
			name:       "assignments separated by semicolons",
			mutate:     onWriteHeader(func(got string) string { return strings.ReplaceAll(got, ",", ";") }),
			wantDetail: "dotted-attribute grammar",
		},
		{
			name:       "an unquoted value",
			mutate:     onWriteHeader(func(got string) string { return got + ",system_id=" + probe062SystemID }),
			wantDetail: "dotted-attribute grammar",
		},
		{
			name: "committer.name is not the committed one",
			mutate: onWriteHeader(func(got string) string {
				return strings.ReplaceAll(got, `committer.name="`+probe062Committer+`"`, `committer.name="Bob"`)
			}),
			wantDetail: "committer.name",
		},
		{
			name: "system_id dropped from the header",
			mutate: onWriteHeader(func(got string) string {
				return strings.ReplaceAll(got, `,system_id="`+probe062SystemID+`"`, "")
			}),
			wantDetail: "header system_id",
		},
		{
			name:       "an undocumented attribute",
			mutate:     onWriteHeader(func(got string) string { return got + `,foo="x"` }),
			wantDetail: "undocumented attribute",
		},
		{
			name:       "read-back system_id is not the committed one",
			fixture:    contributionFixture{systemID: "other.system", committer: probe062Committer, versionID: probe062VersionUID},
			wantDetail: "read-back system_id",
		},
		{
			name:       "read-back committer is not the committed one",
			fixture:    contributionFixture{systemID: probe062SystemID, committer: "Bob", versionID: probe062VersionUID},
			wantDetail: "read-back committer",
		},
		{
			name:       "the Contribution lists some other version",
			fixture:    contributionFixture{systemID: probe062SystemID, committer: probe062Committer, versionID: "other::cdr.example::1"},
			wantDetail: "does not list the version",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := tc.fixture
			if fixture == (contributionFixture{}) {
				fixture = probe062Contribution()
			}
			b, snap := auditWriteBackend(t, tc.mutate, fixture)
			r, err := restprobes.Probe062AuditDetailsHeader(t.Context(), newClient(t, b), snap, probe062EHRID, probe062ContribUID, probe062Audit(), &rm.Composition{})
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "fail" {
				t.Fatalf("PROBE-062 = %s, want fail (detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.wantDetail) {
				t.Errorf("PROBE-062 failed for the wrong reason:\n got: %s\nwant it to mention: %s", r.Detail, tc.wantDetail)
			}
		})
	}
}

func TestProbe062RejectsMissingInputs(t *testing.T) { // PROBE-062
	b, snap := auditWriteBackend(t, nil, probe062Contribution())
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
	if _, err := restprobes.Probe062AuditDetailsHeader(t.Context(), c, snap, probe062EHRID, "", probe062Audit(), &rm.Composition{}); err == nil {
		t.Error("empty contribution uid: want an error")
	}
	if _, err := restprobes.Probe062AuditDetailsHeader(t.Context(), c, snap, probe062EHRID, probe062ContribUID, nil, &rm.Composition{}); err == nil {
		t.Error("nil audit: want an error")
	}
	if _, err := restprobes.Probe062AuditDetailsHeader(t.Context(), c, snap, probe062EHRID, probe062ContribUID, probe062Audit(), nil); err == nil {
		t.Error("nil composition: want an error")
	}
}

// --- PROBE-108 composition formats ------------------------------------------

const (
	probe108EHRID openehrclient.EHRID = "55555555-5555-4555-8555-555555555555"
	// probe108TemplateID names the vendored OPT and canonical composition the
	// probe writes; PROBE-076 round-trips this pair through both simplified
	// codecs, and the composition leaf's format tests use it too.
	probe108TemplateID = "Test_dv_quantity_open_constraint.v0"
	// probe108OtherVersion is a version uid the backend never assigned, for
	// the plant that sends the read to another version's path.
	probe108OtherVersion = "99999999-0000-4000-8000-000000000000::cdr.example::1"
)

// The media types the backend recognises, spelled out so the test pins the
// strings on the wire rather than the constants the code under test uses.
const (
	probe108MediaJSON       = "application/json"
	probe108MediaXML        = "application/xml"
	probe108MediaFlat       = "application/openehr.wt.flat+json"
	probe108MediaStructured = "application/openehr.wt.structured+json"
)

// loadProbe108Fixture builds the Web Template for probe108TemplateID and
// decodes the matching canonical composition, the recipe the composition
// leaf's format tests use.
func loadProbe108Fixture(t *testing.T) (*webtemplate.WebTemplate, *rm.Composition) {
	t.Helper()
	optBody, err := os.ReadFile(fixtures.TemplateOpt(probe108TemplateID))
	if err != nil {
		t.Fatalf("read OPT: %v", err)
	}
	opt, err := fixtures.ParseOPTBytes(optBody)
	if err != nil {
		t.Fatalf("parse OPT: %v", err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile OPT: %v", err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatalf("build Web Template: %v", err)
	}
	if wt.TemplateID != probe108TemplateID {
		t.Fatalf("Web Template id = %q, want %q", wt.TemplateID, probe108TemplateID)
	}
	compBody, err := os.ReadFile(fixtures.CompositionJSON(probe108TemplateID))
	if err != nil {
		t.Fatalf("read composition: %v", err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(compBody, &comp); err != nil {
		t.Fatalf("decode composition: %v", err)
	}
	return wt, &comp
}

// wireFormat names the composition format a request asks for: the one its
// media type names, or, beside application/json, the one its `format` query
// parameter names. ok is false for a media type the backend does not serve.
func wireFormat(mediaType, formatParam string) (f composition.Format, ok bool) {
	switch mediaType {
	case probe108MediaXML:
		return composition.FormatXML, true
	case probe108MediaFlat:
		return composition.FormatFlat, true
	case probe108MediaStructured:
		return composition.FormatStructured, true
	case probe108MediaJSON, "":
		switch formatParam {
		case "FLAT":
			return composition.FormatFlat, true
		case "STRUCTURED":
			return composition.FormatStructured, true
		case "":
			return composition.FormatJSON, true
		default:
			return composition.FormatJSON, false
		}
	default:
		return composition.FormatJSON, false
	}
}

// wireMediaType is the media type the backend labels a body in f with.
func wireMediaType(f composition.Format) string {
	switch f {
	case composition.FormatJSON:
		return probe108MediaJSON
	case composition.FormatXML:
		return probe108MediaXML
	case composition.FormatFlat:
		return probe108MediaFlat
	case composition.FormatStructured:
		return probe108MediaStructured
	default:
		return ""
	}
}

func encodeComposition(f composition.Format, comp *rm.Composition, wt *webtemplate.WebTemplate) ([]byte, error) {
	switch f {
	case composition.FormatJSON:
		return canjson.Marshal(comp)
	case composition.FormatXML:
		return canxml.Marshal(comp)
	case composition.FormatFlat:
		return simplified.MarshalFlat(comp, wt)
	case composition.FormatStructured:
		return simplified.MarshalStructured(comp, wt)
	default:
		return nil, fmt.Errorf("no codec for %v", f)
	}
}

func decodeComposition(f composition.Format, body []byte, wt *webtemplate.WebTemplate) (*rm.Composition, error) {
	switch f {
	case composition.FormatJSON:
		var comp rm.Composition
		if err := canjson.Unmarshal(body, &comp); err != nil {
			return nil, err
		}
		return &comp, nil
	case composition.FormatXML:
		var comp rm.Composition
		if err := canxml.Unmarshal(body, &comp); err != nil {
			return nil, err
		}
		return &comp, nil
	case composition.FormatFlat:
		return simplified.UnmarshalFlat(body, wt)
	case composition.FormatStructured:
		return simplified.UnmarshalStructured(body, wt)
	default:
		return nil, fmt.Errorf("no codec for %v", f)
	}
}

// formatBackendConfig shapes the composition backend PROBE-108 runs against.
// The zero value is a faithful backend.
type formatBackendConfig struct {
	// mutate rewrites the recorded clone of a request, never the live one,
	// so a plant can hand the probe a request the SDK did not send.
	mutate func(*http.Request)
	// answerAs picks the format a response body is sent in, given the one
	// the request asked for. Nil answers in the format asked for.
	answerAs func(r *http.Request, asked composition.Format) composition.Format
	// tamper edits a shallow copy of the composition a response carries.
	tamper func(r *http.Request, comp *rm.Composition)
	// noVersion drops the ETag and Location that name the new version.
	noVersion bool
}

// formatBackend is a scripted composition backend that keeps what it is
// sent and answers in the format a request asks for. A write is decoded with
// the codec its Content-Type names (or, beside application/json, its
// `format` query parameter), kept under a fresh version uid named by the
// ETag and the Location, and, under `Prefer: return=representation`,
// serialised back in the format the Accept header (or the `format`
// parameter) asks for. A read of a kept version answers the same way.
func formatBackend(t *testing.T, wt *webtemplate.WebTemplate, cfg formatBackendConfig) (*sandbox.Backend, func() []*http.Request) {
	t.Helper()
	var (
		mu     sync.Mutex
		stored = map[string]*rm.Composition{}
		n      int
	)
	answer := func(w http.ResponseWriter, r *http.Request, status int, comp *rm.Composition) {
		asked, ok := wireFormat(r.Header.Get("Accept"), r.URL.Query().Get("format"))
		if !ok {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		out := asked
		if cfg.answerAs != nil {
			out = cfg.answerAs(r, asked)
		}
		if cfg.tamper != nil {
			cp := *comp
			cfg.tamper(r, &cp)
			comp = &cp
		}
		body, err := encodeComposition(out, comp, wt)
		if err != nil {
			t.Errorf("backend: encode %v: %v", out, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", wireMediaType(out))
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
	return capturing(t, cfg.mutate, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/composition"):
			in, ok := wireFormat(r.Header.Get("Content-Type"), r.URL.Query().Get("format"))
			if !ok {
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("backend: read write body: %v", err)
			}
			comp, err := decodeComposition(in, body, wt)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			n++
			uid := fmt.Sprintf("10800000-0000-4000-8000-%012d::cdr.example::1", n)
			stored[uid] = comp
			mu.Unlock()
			if !cfg.noVersion {
				w.Header().Set("ETag", `"`+uid+`"`)
				w.Header().Set("Location", r.URL.Path+"/"+uid)
			}
			if !strings.Contains(r.Header.Get("Prefer"), "return=representation") {
				w.WriteHeader(http.StatusCreated)
				return
			}
			answer(w, r, http.StatusCreated, comp)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/composition/"):
			uid := path.Base(r.URL.Path)
			mu.Lock()
			comp, ok := stored[uid]
			mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("ETag", `"`+uid+`"`)
			answer(w, r, http.StatusOK, comp)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}

// onRequests returns a mutate that rewrites the recorded clone of every
// request match selects and leaves the others alone.
func onRequests(match func(*http.Request) bool, rewrite func(*http.Request)) func(*http.Request) {
	return func(req *http.Request) {
		if match(req) {
			rewrite(req)
		}
	}
}

// isWrite selects the composition write sent with the given Content-Type.
func isWrite(contentType string) func(*http.Request) bool {
	return func(req *http.Request) bool {
		return req.Method == http.MethodPost && req.Header.Get("Content-Type") == contentType
	}
}

// isQueryWrite selects the composition write that names its format with the
// `format` query parameter.
func isQueryWrite(req *http.Request) bool {
	return req.Method == http.MethodPost && req.URL.Query().Has("format")
}

// isQueryRead selects the composition read that names its format with the
// `format` query parameter.
func isQueryRead(req *http.Request) bool {
	return req.Method == http.MethodGet && req.URL.Query().Has("format")
}

// isRead selects the composition read sent with the given Accept.
func isRead(accept string) func(*http.Request) bool {
	return func(req *http.Request) bool {
		return req.Method == http.MethodGet && req.Header.Get("Accept") == accept
	}
}

// rewriteBody rewrites the recorded clone's body with edit. An edit that
// changes nothing fails the test, so a plant cannot silently miss its target.
func rewriteBody(t *testing.T, edit func(string) string) func(*http.Request) {
	return func(req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("read the recorded body: %v", err)
		}
		edited := edit(string(body))
		if edited == string(body) {
			t.Errorf("the body edit changed nothing in %s", body)
		}
		setBody(req, edited)
	}
}

// withoutMember returns a copy of comp whose canonical JSON lacks the member
// at, a slash-separated path of object members and array indexes. comp itself
// is left as it is. A path that does not end on a member fails the test and
// returns comp, so a plant cannot silently miss its target.
func withoutMember(t *testing.T, comp *rm.Composition, at string) *rm.Composition {
	t.Helper()
	body, err := canjson.Marshal(comp)
	if err != nil {
		t.Errorf("render the composition as canonical JSON: %v", err)
		return comp
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		t.Errorf("decode the composition's canonical JSON: %v", err)
		return comp
	}
	segs := strings.Split(at, "/")
	node := tree
	for _, seg := range segs[:len(segs)-1] {
		switch n := node.(type) {
		case map[string]any:
			node = n[seg]
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(n) {
				node = nil
				continue
			}
			node = n[i]
		default:
			node = nil
		}
	}
	parent, ok := node.(map[string]any)
	member := segs[len(segs)-1]
	if !ok || parent[member] == nil {
		t.Errorf("the composition has no member at %s to drop", at)
		return comp
	}
	delete(parent, member)
	edited, err := json.Marshal(tree)
	if err != nil {
		t.Errorf("render the edited composition: %v", err)
		return comp
	}
	var out rm.Composition
	if err := canjson.Unmarshal(edited, &out); err != nil {
		t.Errorf("decode the composition without %s: %v", at, err)
		return comp
	}
	return &out
}

func TestProbe108CompositionFormats(t *testing.T) { // PROBE-108, REQ-173
	wt, comp := loadProbe108Fixture(t)
	b, snap := formatBackend(t, wt, formatBackendConfig{})
	r, err := restprobes.Probe108CompositionFormats(t.Context(), newClient(t, b), snap, probe108EHRID, wt, comp)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-108 = %s: %s", r.Status, r.Detail)
	}
	// Four writes, each read back: XML, FLAT and STRUCTURED by media type,
	// and FLAT under the format query options. A probe that skipped an arm or
	// its read would pass on fewer.
	reqs := snap()
	var writes, reads, queryReads int
	for _, req := range reqs {
		switch req.Method {
		case http.MethodPost:
			writes++
		case http.MethodGet:
			reads++
			if isQueryRead(req) {
				queryReads++
			}
		}
	}
	if writes != 4 || reads != 4 || queryReads != 1 || len(reqs) != 8 {
		t.Errorf("backend saw %d writes and %d reads, %d of them by format query (%d requests), want 4 writes and 4 reads, 1 by format query",
			writes, reads, queryReads, len(reqs))
	}
}

// TestProbe108FlagsBrokenWire breaks one assertion of the probe per case. The
// first group is a backend that does not answer in the format asked for, or
// answers with another composition; the second plants a write or read the
// SDK did not send — a wrong media type, a missing or foreign template id, a
// missing query parameter, a body that is not the composition, a wrong
// route; the last two plant a `.schema`-suffixed media type, so the detector
// that guards "no request carries one" is shown to fire. Each case pins the
// substring its Detail must carry, so a case cannot rot into failing for an
// unrelated reason. A case that plants another composition value, or drops
// one, also names the values its Detail must not carry: a mismatch names the
// path, not what the compositions hold there.
func TestProbe108FlagsBrokenWire(t *testing.T) { // PROBE-108
	wt, comp := loadProbe108Fixture(t)
	cases := []struct {
		name       string
		cfg        formatBackendConfig
		wantDetail string
		// notInDetail are compared values the Detail must not carry.
		notInDetail []string
	}{
		{
			name: "backend answers canonical JSON whatever the format asked",
			cfg: formatBackendConfig{answerAs: func(*http.Request, composition.Format) composition.Format {
				return composition.FormatJSON
			}},
			wantDetail: "XML write committed, but its representation did not decode as XML",
		},
		{
			name: "backend answers a FLAT write in canonical JSON",
			cfg: formatBackendConfig{answerAs: func(_ *http.Request, asked composition.Format) composition.Format {
				if asked == composition.FormatFlat {
					return composition.FormatJSON
				}
				return asked
			}},
			wantDetail: "FLAT write committed, but its representation did not decode as FLAT",
		},
		{
			name: "backend answers the read in canonical JSON",
			cfg: formatBackendConfig{answerAs: func(r *http.Request, asked composition.Format) composition.Format {
				if r.Method == http.MethodGet {
					return composition.FormatJSON
				}
				return asked
			}},
			wantDetail: "XML read failed",
		},
		{
			name: "representation is another composition",
			cfg: formatBackendConfig{tamper: func(r *http.Request, c *rm.Composition) {
				if r.Method == http.MethodPost {
					c.ArchetypeNodeID = "openEHR-EHR-COMPOSITION.other.v1"
				}
			}},
			wantDetail: "XML write: the representation differs from the composition written at archetype_node_id: value differs",
		},
		{
			name: "read-back is another composition",
			cfg: formatBackendConfig{tamper: func(r *http.Request, c *rm.Composition) {
				if r.Method == http.MethodGet {
					c.ArchetypeNodeID = "openEHR-EHR-COMPOSITION.other.v1"
				}
			}},
			wantDetail: "XML read: the composition read back differs from the one written at archetype_node_id: value differs",
		},
		{
			name: "read-back lacks the data value",
			cfg: formatBackendConfig{tamper: func(r *http.Request, c *rm.Composition) {
				if r.Method == http.MethodGet && r.Header.Get("Accept") == probe108MediaXML {
					*c = *withoutMember(t, c, "content/0/data/events/0/data/items/0/value")
				}
			}},
			wantDetail: "XML read: the composition read back differs from the one written at content/0/data/events/0/data/items/0/value: absent",
			// The value dropped is the DV_QUANTITY 130 mmHg.
			notInDetail: []string{"130", "mmHg"},
		},
		{
			name:       "write names no version",
			cfg:        formatBackendConfig{noVersion: true},
			wantDetail: "XML write named no version",
		},
		{
			name: "write on another EHR's route",
			cfg: formatBackendConfig{mutate: onRequests(isWrite(probe108MediaXML), func(req *http.Request) {
				req.URL.Path = strings.Replace(req.URL.Path, string(probe108EHRID), probe060OtherEHRID, 1)
			})},
			wantDetail: "XML write went to",
		},
		{
			name: "FLAT write labelled canonical JSON",
			cfg: formatBackendConfig{mutate: onRequests(isWrite(probe108MediaFlat), func(req *http.Request) {
				req.Header.Set("Content-Type", probe108MediaJSON)
			})},
			wantDetail: "FLAT write: Content-Type",
		},
		{
			name: "STRUCTURED write accepts canonical JSON",
			cfg: formatBackendConfig{mutate: onRequests(isWrite(probe108MediaStructured), func(req *http.Request) {
				req.Header.Set("Accept", probe108MediaJSON)
			})},
			wantDetail: "STRUCTURED write: Accept",
		},
		{
			name: "FLAT write without openehr-template-id",
			cfg: formatBackendConfig{mutate: onRequests(isWrite(probe108MediaFlat), func(req *http.Request) {
				req.Header.Del("openehr-template-id")
			})},
			wantDetail: "FLAT write: openehr-template-id",
		},
		{
			name: "STRUCTURED write naming another template",
			cfg: formatBackendConfig{mutate: onRequests(isWrite(probe108MediaStructured), func(req *http.Request) {
				req.Header.Set("openehr-template-id", "Other_template.v1")
			})},
			wantDetail: "STRUCTURED write: openehr-template-id",
		},
		{
			name:       "XML write body is not canonical XML",
			cfg:        formatBackendConfig{mutate: onRequests(isWrite(probe108MediaXML), func(req *http.Request) { setBody(req, `{}`) })},
			wantDetail: "XML write: the request body does not decode as XML",
		},
		{
			name: "FLAT write body carries another data value",
			cfg: formatBackendConfig{mutate: onRequests(isWrite(probe108MediaFlat), rewriteBody(t, func(body string) string {
				return strings.ReplaceAll(body, `/any_event:0/my_dv_quantity|magnitude":130`, `/any_event:0/my_dv_quantity|magnitude":131`)
			}))},
			wantDetail:  "FLAT write: the request body differs from the composition given at content/0/data/events/0/data/items/0/value: value differs",
			notInDetail: []string{"130", "131"},
		},
		{
			name: "read on another version's route",
			cfg: formatBackendConfig{mutate: onRequests(isRead(probe108MediaXML), func(req *http.Request) {
				req.URL.Path = path.Dir(req.URL.Path) + "/" + probe108OtherVersion
			})},
			wantDetail: "XML read went to",
		},
		{
			name: "XML read accepts canonical JSON",
			cfg: formatBackendConfig{mutate: onRequests(isRead(probe108MediaXML), func(req *http.Request) {
				req.Header.Set("Accept", probe108MediaJSON)
			})},
			wantDetail: "XML read: Accept",
		},
		{
			name: "format-query write labelled FLAT",
			cfg: formatBackendConfig{mutate: onRequests(isQueryWrite, func(req *http.Request) {
				req.Header.Set("Content-Type", probe108MediaFlat)
			})},
			wantDetail: "FLAT (format query) write: Content-Type",
		},
		{
			name: "format-query write without format",
			cfg: formatBackendConfig{mutate: onRequests(isQueryWrite, func(req *http.Request) {
				q := req.URL.Query()
				q.Del("format")
				req.URL.RawQuery = q.Encode()
			})},
			wantDetail: "FLAT (format query) write: query parameter format",
		},
		{
			name: "format-query write without templateId",
			cfg: formatBackendConfig{mutate: onRequests(isQueryWrite, func(req *http.Request) {
				q := req.URL.Query()
				q.Del("templateId")
				req.URL.RawQuery = q.Encode()
			})},
			wantDetail: "FLAT (format query) write: query parameter templateId",
		},
		{
			name: "format-query read without format",
			cfg: formatBackendConfig{mutate: onRequests(isQueryRead, func(req *http.Request) {
				q := req.URL.Query()
				q.Del("format")
				req.URL.RawQuery = q.Encode()
			})},
			wantDetail: "FLAT (format query) read: query parameter format",
		},
		{
			name: "format-query read accepts the FLAT media type",
			cfg: formatBackendConfig{mutate: onRequests(isQueryRead, func(req *http.Request) {
				req.Header.Set("Accept", probe108MediaFlat)
			})},
			wantDetail: "FLAT (format query) read: Accept",
		},
		{
			name: "format-query write without openehr-template-id",
			cfg: formatBackendConfig{mutate: onRequests(isQueryWrite, func(req *http.Request) {
				req.Header.Del("openehr-template-id")
			})},
			wantDetail: "FLAT (format query) write: openehr-template-id",
		},
		{
			name: ".schema media type as a write's Content-Type",
			cfg: formatBackendConfig{mutate: onRequests(isWrite(probe108MediaStructured), func(req *http.Request) {
				req.Header.Set("Content-Type", "application/openehr.wt.structured.schema+json")
			})},
			wantDetail: "STRUCTURED write: Content-Type carries the retired .schema media type",
		},
		{
			name: ".schema media type among a read's Accept values",
			cfg: formatBackendConfig{mutate: onRequests(isRead(probe108MediaFlat), func(req *http.Request) {
				req.Header.Set("Accept", probe108MediaFlat+", application/openehr.wt.flat.schema+json;q=0.5")
			})},
			wantDetail: "FLAT read: Accept carries the retired .schema media type",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, snap := formatBackend(t, wt, tc.cfg)
			r, err := restprobes.Probe108CompositionFormats(t.Context(), newClient(t, b), snap, probe108EHRID, wt, comp)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("PROBE-108 = %s: %s", r.Status, r.Detail)
			if r.Status != "fail" {
				t.Fatalf("PROBE-108 = %s, want fail (detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.wantDetail) {
				t.Errorf("PROBE-108 failed for the wrong reason:\n got: %s\nwant it to mention: %s", r.Detail, tc.wantDetail)
			}
			for _, value := range tc.notInDetail {
				if strings.Contains(r.Detail, value) {
					t.Errorf("PROBE-108 detail carries the compared value %q:\n%s", value, r.Detail)
				}
			}
		})
	}
}

func TestProbe108RejectsMissingInputs(t *testing.T) { // PROBE-108
	wt, comp := loadProbe108Fixture(t)
	b, snap := formatBackend(t, wt, formatBackendConfig{})
	c := newClient(t, b)
	noID := *wt
	noID.TemplateID = ""
	noContent := *comp
	noContent.Content = nil
	// FLAT takes the root archetype from the Web Template, so a composition
	// whose root names another archetype does not come back as it went.
	otherRoot := *comp
	otherRoot.ArchetypeNodeID = "openEHR-EHR-COMPOSITION.other.v1"
	cases := []struct {
		name     string
		c        *transport.Client
		captured func() []*http.Request
		ehrID    openehrclient.EHRID
		wt       *webtemplate.WebTemplate
		comp     *rm.Composition
		// wantErr, when set, is a substring the error must carry.
		wantErr string
		// notInErr are compared values the error must not carry.
		notInErr []string
	}{
		{name: "nil client", captured: snap, ehrID: probe108EHRID, wt: wt, comp: comp},
		{name: "nil recorder", c: c, ehrID: probe108EHRID, wt: wt, comp: comp},
		{name: "empty ehr id", c: c, captured: snap, wt: wt, comp: comp},
		{name: "nil Web Template", c: c, captured: snap, ehrID: probe108EHRID, comp: comp},
		{name: "Web Template without a template id", c: c, captured: snap, ehrID: probe108EHRID, wt: &noID, comp: comp},
		{name: "nil composition", c: c, captured: snap, ehrID: probe108EHRID, wt: wt},
		// The probe compares a data value, so a composition with no content
		// gives it nothing to compare: a precondition, not a deployment
		// finding.
		{name: "composition without content", c: c, captured: snap, ehrID: probe108EHRID, wt: wt, comp: &noContent},
		// The error names the path that does not come back, never the root
		// archetype given or the one FLAT puts in its place.
		{
			name: "composition that does not round-trip through FLAT", c: c, captured: snap, ehrID: probe108EHRID, wt: wt, comp: &otherRoot,
			wantErr:  "does not round-trip through FLAT on the compared paths (archetype_node_id: value differs)",
			notInErr: []string{otherRoot.ArchetypeNodeID, comp.ArchetypeNodeID},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := restprobes.Probe108CompositionFormats(t.Context(), tc.c, tc.captured, tc.ehrID, tc.wt, tc.comp)
			if err == nil {
				t.Fatalf("PROBE-108 = %s (%q), want an error", r.Status, r.Detail)
			}
			t.Logf("PROBE-108 error: %v", err)
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("PROBE-108 refused the input for the wrong reason:\n got: %v\nwant it to mention: %s", err, tc.wantErr)
			}
			for _, value := range tc.notInErr {
				if strings.Contains(err.Error(), value) {
					t.Errorf("PROBE-108 error carries the compared value %q:\n%v", value, err)
				}
			}
		})
	}
	if n := len(snap()); n != 0 {
		t.Errorf("backend saw %d requests, want none: a refused input must not reach the wire", n)
	}
}

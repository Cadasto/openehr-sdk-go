package restprobes_test

import (
	"bytes"
	"io"
	"net/http"
	"slices"
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

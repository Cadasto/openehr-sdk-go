package composition_test

// versioned_test.go — REQ-174 versioned-object reads at the composition leaf:
// the VERSIONED_COMPOSITION, its REVISION_HISTORY, and its versions (latest,
// at a time, by id).

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// versionTwoUID is the second version of compositionVOID; compositionVUID is
// the first, so it is the version the second one follows.
const versionTwoUID openehrclient.VersionUID = "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::2"

// versionedCompositionJSON is the VERSIONED_COMPOSITION of compositionVOID.
const versionedCompositionJSON = `{
  "_type": "VERSIONED_COMPOSITION",
  "uid": {"_type": "HIER_OBJECT_ID", "value": "1234abcd-5678-9012-3456-7890abcdef00"},
  "owner_id": {"_type": "OBJECT_REF", "id": {"_type": "HIER_OBJECT_ID", "value": "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"}, "namespace": "local", "type": "EHR"},
  "time_created": {"_type": "DV_DATE_TIME", "value": "2015-01-20T18:30:22.765Z"}
}`

// revisionHistoryJSON is the REVISION_HISTORY of compositionVOID: two
// versions, most recent last.
const revisionHistoryJSON = `{
  "_type": "REVISION_HISTORY",
  "items": [
    {
      "_type": "REVISION_HISTORY_ITEM",
      "version_id": {"_type": "OBJECT_VERSION_ID", "value": "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::1"},
      "audits": [` + commitAuditJSON + `]
    },
    {
      "_type": "REVISION_HISTORY_ITEM",
      "version_id": {"_type": "OBJECT_VERSION_ID", "value": "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::2"},
      "audits": [` + commitAuditJSON + `]
    }
  ]
}`

// commitAuditJSON is the AUDIT_DETAILS of a modification.
const commitAuditJSON = `{
  "_type": "AUDIT_DETAILS",
  "system_id": "cdr.example",
  "time_committed": {"_type": "DV_DATE_TIME", "value": "2015-01-20T18:30:22.765Z"},
  "change_type": {"_type": "DV_CODED_TEXT", "value": "modification", "defining_code": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "openehr"}, "code_string": "251"}},
  "committer": {"_type": "PARTY_IDENTIFIED", "name": "Dr Alice"}
}`

// versionMembersJSON opens the ORIGINAL_VERSION for versionTwoUID and holds
// every member but data; the bodies below close it with or without data.
const versionMembersJSON = `{
  "_type": "ORIGINAL_VERSION",
  "uid": {"_type": "OBJECT_VERSION_ID", "value": "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::2"},
  "preceding_version_uid": {"_type": "OBJECT_VERSION_ID", "value": "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::1"},
  "lifecycle_state": {"_type": "DV_CODED_TEXT", "value": "complete", "defining_code": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "openehr"}, "code_string": "532"}},
  "commit_audit": ` + commitAuditJSON + `,
  "contribution": {"_type": "OBJECT_REF", "id": {"_type": "HIER_OBJECT_ID", "value": "5c1a6d0e-2f3b-4c8d-9e7f-0a1b2c3d4e5f"}, "namespace": "local", "type": "CONTRIBUTION"}`

// compositionVersionJSON is versionTwoUID carrying compositionJSON (the
// example tests' COMPOSITION) as its data.
const compositionVersionJSON = versionMembersJSON + `,
  "data": ` + compositionJSON + `
}`

// noDataVersionJSON is versionTwoUID with no data member.
const noDataVersionJSON = versionMembersJSON + `
}`

// importedVersionJSON is an IMPORTED_VERSION wrapping compositionVersionJSON:
// the pin's version schema admits it, and v1 does not read it.
const importedVersionJSON = `{
  "_type": "IMPORTED_VERSION",
  "commit_audit": ` + commitAuditJSON + `,
  "contribution": {"_type": "OBJECT_REF", "id": {"_type": "HIER_OBJECT_ID", "value": "5c1a6d0e-2f3b-4c8d-9e7f-0a1b2c3d4e5f"}, "namespace": "local", "type": "CONTRIBUTION"},
  "item": ` + compositionVersionJSON + `
}`

// versionedPath is the wire path of compositionVOID's versioned object, with
// the test client's service base.
const versionedPath = "/openehr/v1/ehr/" + string(ehrIDFixture) + "/versioned_composition/" + string(compositionVOID)

// atTime is the pin's version_at_time example, 19:30:22.765 at +01:00 on 20
// January 2015; atTimeWire is how it goes on the wire.
var atTime = time.Date(2015, 1, 20, 19, 30, 22, 765_000_000, time.FixedZone("", 3600))

const atTimeWire = "2015-01-20T18:30:22.765Z"

// versionedRead is one of the five reads, called with valid arguments: the
// path and query it sends, a 200 body it decodes, and the call itself, which
// reports whether it returned a result.
type versionedRead struct {
	name  string
	path  string
	query url.Values
	body  string
	call  func(context.Context, *transport.Client) (bool, *openehrclient.VersionMetadata, error)
}

func versionedReads() []versionedRead {
	return []versionedRead{
		{
			name: "GetVersionedObject",
			path: versionedPath,
			body: versionedCompositionJSON,
			call: func(ctx context.Context, c *transport.Client) (bool, *openehrclient.VersionMetadata, error) {
				out, meta, err := composition.GetVersionedObject(ctx, c, ehrIDFixture, compositionVOID)
				return out != nil, meta, err
			},
		},
		{
			name: "GetRevisionHistory",
			path: versionedPath + "/revision_history",
			body: revisionHistoryJSON,
			call: func(ctx context.Context, c *transport.Client) (bool, *openehrclient.VersionMetadata, error) {
				out, meta, err := composition.GetRevisionHistory(ctx, c, ehrIDFixture, compositionVOID)
				return out != nil, meta, err
			},
		},
		{
			name: "GetVersion",
			path: versionedPath + "/version",
			body: compositionVersionJSON,
			call: func(ctx context.Context, c *transport.Client) (bool, *openehrclient.VersionMetadata, error) {
				out, meta, err := composition.GetVersion(ctx, c, ehrIDFixture, compositionVOID)
				return out != nil, meta, err
			},
		},
		{
			name:  "GetVersionAtTime",
			path:  versionedPath + "/version",
			query: url.Values{"version_at_time": {atTimeWire}},
			body:  compositionVersionJSON,
			call: func(ctx context.Context, c *transport.Client) (bool, *openehrclient.VersionMetadata, error) {
				out, meta, err := composition.GetVersionAtTime(ctx, c, ehrIDFixture, compositionVOID, atTime)
				return out != nil, meta, err
			},
		},
		{
			name: "GetVersionByID",
			path: versionedPath + "/version/" + string(versionTwoUID),
			body: compositionVersionJSON,
			call: func(ctx context.Context, c *transport.Client) (bool, *openehrclient.VersionMetadata, error) {
				out, meta, err := composition.GetVersionByID(ctx, c, ehrIDFixture, compositionVOID, versionTwoUID)
				return out != nil, meta, err
			},
		},
	}
}

// versionCall is one of the three version reads, called with valid arguments.
type versionCall struct {
	name string
	call func(context.Context, *transport.Client) (*composition.Version, *openehrclient.VersionMetadata, error)
}

func versionCalls() []versionCall {
	return []versionCall{
		{"GetVersion", func(ctx context.Context, c *transport.Client) (*composition.Version, *openehrclient.VersionMetadata, error) {
			return composition.GetVersion(ctx, c, ehrIDFixture, compositionVOID)
		}},
		{"GetVersionAtTime", func(ctx context.Context, c *transport.Client) (*composition.Version, *openehrclient.VersionMetadata, error) {
			return composition.GetVersionAtTime(ctx, c, ehrIDFixture, compositionVOID, atTime)
		}},
		{"GetVersionByID", func(ctx context.Context, c *transport.Client) (*composition.Version, *openehrclient.VersionMetadata, error) {
			return composition.GetVersionByID(ctx, c, ehrIDFixture, compositionVOID, versionTwoUID)
		}},
	}
}

// serveVersioned starts a server that answers every request with status and
// body, and returns a client for it.
func serveVersioned(t *testing.T, status int, body string) *transport.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return newClient(t, srv)
}

// TestVersionedReadsRequest pins REQ-174's request shape for each read: GET,
// the raw path, canonical JSON, and a version_at_time only on the
// version-at-time read, in UTC with its fraction of a second. The latest
// read sends no query at all.
func TestVersionedReadsRequest(t *testing.T) { // REQ-174
	for _, rd := range versionedReads() {
		t.Run(rd.name, func(t *testing.T) {
			var captured *http.Request
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = r.Clone(r.Context())
				_, _ = w.Write([]byte(rd.body))
			}))
			defer srv.Close()

			ok, _, err := rd.call(t.Context(), newClient(t, srv))
			if err != nil {
				t.Fatalf("%s: %v", rd.name, err)
			}
			if !ok {
				t.Fatalf("%s returned no result and no error", rd.name)
			}
			if captured.Method != http.MethodGet {
				t.Errorf("%s method = %q, want GET", rd.name, captured.Method)
			}
			if captured.URL.Path != rd.path {
				t.Errorf("%s path = %q, want %q", rd.name, captured.URL.Path, rd.path)
			}
			if got := captured.Header.Get("Accept"); got != "application/json" {
				t.Errorf("%s Accept = %q, want application/json", rd.name, got)
			}
			if got, want := captured.URL.Query().Encode(), rd.query.Encode(); got != want {
				t.Errorf("%s query = %q, want %q", rd.name, got, want)
			}
		})
	}
}

// TestGetVersionedObjectDecodes pins REQ-174: the versioned-object read
// returns the VERSIONED_COMPOSITION decoded as its RM type.
func TestGetVersionedObjectDecodes(t *testing.T) { // REQ-174
	c := serveVersioned(t, http.StatusOK, versionedCompositionJSON)
	vc, meta, err := composition.GetVersionedObject(t.Context(), c, ehrIDFixture, compositionVOID)
	if err != nil {
		t.Fatal(err)
	}
	if got := vc.UID.Value; got != string(compositionVOID) {
		t.Errorf("VERSIONED_COMPOSITION uid = %q, want %q", got, compositionVOID)
	}
	if got := vc.TimeCreated.Value; got != atTimeWire {
		t.Errorf("VERSIONED_COMPOSITION time_created = %q, want %q", got, atTimeWire)
	}
	if vc.OwnerID == nil {
		t.Error("VERSIONED_COMPOSITION owner_id = nil, want the EHR reference")
	}
	// The pin defines no ETag here: metadata arrives without one.
	if meta == nil {
		t.Error("metadata = nil, want the response headers")
	}
}

// TestGetRevisionHistoryDecodes pins REQ-174: the revision-history read
// returns the REVISION_HISTORY decoded as its RM type, items in wire order.
func TestGetRevisionHistoryDecodes(t *testing.T) { // REQ-174
	c := serveVersioned(t, http.StatusOK, revisionHistoryJSON)
	rh, _, err := composition.GetRevisionHistory(t.Context(), c, ehrIDFixture, compositionVOID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range rh.Items {
		got = append(got, item.VersionID.Value)
	}
	want := []string{string(compositionVUID), string(versionTwoUID)}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("revision history version ids = %q, want %q", got, want)
	}
	for i, item := range rh.Items {
		if len(item.Audits) != 1 || item.Audits[0] == nil {
			t.Errorf("revision history item %d audits = %v, want one AUDIT_DETAILS", i, item.Audits)
		}
	}
}

// TestGetVersionDecodesEnvelope pins REQ-174 on each version read: the
// ORIGINAL_VERSION envelope's uid, preceding_version_uid, lifecycle_state,
// commit_audit and contribution, and its data decoded as a COMPOSITION.
func TestGetVersionDecodesEnvelope(t *testing.T) { // REQ-174
	for _, vc := range versionCalls() {
		t.Run(vc.name, func(t *testing.T) {
			v, _, err := vc.call(t.Context(), serveVersioned(t, http.StatusOK, compositionVersionJSON))
			if err != nil {
				t.Fatal(err)
			}
			if v.UID.Value != string(versionTwoUID) {
				t.Errorf("uid = %q, want %q", v.UID.Value, versionTwoUID)
			}
			if v.VersionUID() != versionTwoUID {
				t.Errorf("VersionUID() = %q, want %q", v.VersionUID(), versionTwoUID)
			}
			if v.PrecedingVersionUID == nil || v.PrecedingVersionUID.Value != string(compositionVUID) {
				t.Errorf("preceding_version_uid = %+v, want %q", v.PrecedingVersionUID, compositionVUID)
			}
			if got := v.LifecycleState.DefiningCode.CodeString; got != "532" {
				t.Errorf("lifecycle_state code = %q, want 532 (complete)", got)
			}
			audit, ok := v.CommitAudit.(*rm.AuditDetails)
			if !ok || audit == nil {
				t.Fatalf("commit_audit = %T, want *rm.AuditDetails", v.CommitAudit)
			}
			if got := audit.ChangeType.DefiningCode.CodeString; got != "251" {
				t.Errorf("commit_audit change_type code = %q, want 251 (modification)", got)
			}
			if v.Contribution == nil {
				t.Error("contribution = nil, want the CONTRIBUTION reference")
			}
			if v.Composition == nil {
				t.Fatal("Composition = nil, want the version's data")
			}
			if got := v.Composition.ArchetypeNodeID; got != "openEHR-EHR-COMPOSITION.encounter.v1" {
				t.Errorf("Composition archetype_node_id = %q, want openEHR-EHR-COMPOSITION.encounter.v1", got)
			}
			if v.Composition.Name == nil || v.Composition.Name.GetValue() != "Encounter" {
				t.Errorf("Composition name = %v, want Encounter", v.Composition.Name)
			}
		})
	}
}

// TestGetVersionWithoutData pins REQ-174: a version that carries no data
// returns no payload and no error.
func TestGetVersionWithoutData(t *testing.T) { // REQ-174
	for _, vc := range versionCalls() {
		t.Run(vc.name, func(t *testing.T) {
			v, _, err := vc.call(t.Context(), serveVersioned(t, http.StatusOK, noDataVersionJSON))
			if err != nil {
				t.Fatalf("version without data: err = %v, want nil", err)
			}
			if v == nil {
				t.Fatal("version without data = nil, want the envelope")
			}
			if v.Composition != nil {
				t.Errorf("Composition = %+v, want nil", v.Composition)
			}
			if v.UID.Value != string(versionTwoUID) {
				t.Errorf("uid = %q, want %q", v.UID.Value, versionTwoUID)
			}
		})
	}
}

// TestGetVersionRefusesImportedVersion pins REQ-174: v1 reads only
// ORIGINAL_VERSION, so an IMPORTED_VERSION fails as the typed decode failure
// of REQ-151, never as an empty version.
func TestGetVersionRefusesImportedVersion(t *testing.T) { // REQ-174
	for _, vc := range versionCalls() {
		t.Run(vc.name, func(t *testing.T) {
			v, meta, err := vc.call(t.Context(), serveVersioned(t, http.StatusOK, importedVersionJSON))
			de, ok := errors.AsType[*transport.DecodeError](err)
			if !ok || de == nil {
				t.Fatalf("IMPORTED_VERSION: err = %v (%T), want a *transport.DecodeError", err, err)
			}
			if string(de.Body) != importedVersionJSON {
				t.Errorf("DecodeError.Body = %q, want the bytes the server sent", de.Body)
			}
			if v != nil {
				t.Errorf("IMPORTED_VERSION decoded to %+v, want nil", v)
			}
			if meta == nil {
				t.Error("metadata = nil beside the decode error, want the response headers")
			}
		})
	}
}

// TestVersionedReadsEmptyBodyIsInvalidShape pins REQ-174: the pin answers
// each read 200 with a body, or 404, so an empty or null 2xx body, a 204
// included, fails with ErrInvalidShape.
func TestVersionedReadsEmptyBodyIsInvalidShape(t *testing.T) { // REQ-174
	responses := []struct {
		name   string
		status int
		body   string
	}{
		{"empty 200", http.StatusOK, ""},
		{"null 200", http.StatusOK, "null"},
		{"204", http.StatusNoContent, ""},
	}
	for _, rd := range versionedReads() {
		for _, resp := range responses {
			t.Run(rd.name+"/"+resp.name, func(t *testing.T) {
				ok, _, err := rd.call(t.Context(), serveVersioned(t, resp.status, resp.body))
				if !errors.Is(err, transport.ErrInvalidShape) {
					t.Errorf("%s(%s) err = %v, want ErrInvalidShape", rd.name, resp.name, err)
				}
				if ok {
					t.Errorf("%s(%s) returned a result beside the error", rd.name, resp.name)
				}
			})
		}
	}
}

// TestVersionedReadsNotFound pins REQ-174: a 404 maps to ErrNotFound.
func TestVersionedReadsNotFound(t *testing.T) { // REQ-174
	for _, rd := range versionedReads() {
		t.Run(rd.name, func(t *testing.T) {
			c := serveVersioned(t, http.StatusNotFound, `{"message":"not found","code":"NOT_FOUND"}`)
			_, _, err := rd.call(t.Context(), c)
			if !errors.Is(err, transport.ErrNotFound) {
				t.Errorf("%s(404) err = %v, want ErrNotFound", rd.name, err)
			}
		})
	}
}

// TestVersionedReadGuards pins REQ-174's preconditions: an empty ehr_id,
// versioned_object_uid or version_uid, and a zero time, fail with
// ErrInvalidConfig before any request. The transport's REQ-150 validator
// would refuse an empty path segment too, so each case also requires the
// error not to be the transport's ErrInvalidPathSegment: the leaf refuses
// first, naming the argument.
func TestVersionedReadGuards(t *testing.T) { // REQ-174
	cases := []struct {
		name string
		call func(context.Context, *transport.Client) error
	}{
		{"GetVersionedObject/empty EHRID", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetVersionedObject(ctx, c, "", compositionVOID)
			return err
		}},
		{"GetVersionedObject/empty VersionedObjectID", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetVersionedObject(ctx, c, ehrIDFixture, "")
			return err
		}},
		{"GetRevisionHistory/empty EHRID", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetRevisionHistory(ctx, c, "", compositionVOID)
			return err
		}},
		{"GetRevisionHistory/empty VersionedObjectID", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetRevisionHistory(ctx, c, ehrIDFixture, "")
			return err
		}},
		{"GetVersion/empty EHRID", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetVersion(ctx, c, "", compositionVOID)
			return err
		}},
		{"GetVersion/empty VersionedObjectID", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetVersion(ctx, c, ehrIDFixture, "")
			return err
		}},
		{"GetVersionAtTime/empty EHRID", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetVersionAtTime(ctx, c, "", compositionVOID, atTime)
			return err
		}},
		{"GetVersionAtTime/empty VersionedObjectID", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetVersionAtTime(ctx, c, ehrIDFixture, "", atTime)
			return err
		}},
		{"GetVersionAtTime/zero time", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetVersionAtTime(ctx, c, ehrIDFixture, compositionVOID, time.Time{})
			return err
		}},
		{"GetVersionByID/empty EHRID", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetVersionByID(ctx, c, "", compositionVOID, versionTwoUID)
			return err
		}},
		{"GetVersionByID/empty VersionedObjectID", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetVersionByID(ctx, c, ehrIDFixture, "", versionTwoUID)
			return err
		}},
		{"GetVersionByID/empty VersionUID", func(ctx context.Context, c *transport.Client) error {
			_, _, err := composition.GetVersionByID(ctx, c, ehrIDFixture, compositionVOID, "")
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				_, _ = w.Write([]byte(compositionVersionJSON))
			}))
			defer srv.Close()

			err := tc.call(t.Context(), newClient(t, srv))
			if !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("%s: err = %v, want ErrInvalidConfig", tc.name, err)
			}
			if errors.Is(err, transport.ErrInvalidPathSegment) {
				t.Errorf("%s: err = %v, want the leaf's own refusal, not the transport's path check", tc.name, err)
			}
			if n := hits.Load(); n != 0 {
				t.Errorf("%s: server saw %d requests, want 0", tc.name, n)
			}
		})
	}
}

// TestGetVersionAtTimeETagReachesMetadata pins REQ-174's version metadata:
// the pin's ETag on a version-at-time response fills VersionUID.
func TestGetVersionAtTimeETagReachesMetadata(t *testing.T) { // REQ-174
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+string(versionTwoUID)+`"`)
		_, _ = w.Write([]byte(compositionVersionJSON))
	}))
	defer srv.Close()

	_, meta, err := composition.GetVersionAtTime(t.Context(), newClient(t, srv), ehrIDFixture, compositionVOID, atTime)
	if err != nil {
		t.Fatal(err)
	}
	if meta == nil || meta.VersionUID != versionTwoUID {
		t.Errorf("metadata = %+v, want VersionUID %q from the ETag", meta, versionTwoUID)
	}
}

// TestVersionedRepositoryWiring pins REQ-174's repository reads: each new
// Repository method reaches its own route.
func TestVersionedRepositoryWiring(t *testing.T) { // REQ-174
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case strings.HasSuffix(r.URL.Path, "/revision_history"):
			_, _ = w.Write([]byte(revisionHistoryJSON))
		case strings.HasSuffix(r.URL.Path, "/version") || strings.Contains(r.URL.Path, "/version/"):
			_, _ = w.Write([]byte(compositionVersionJSON))
		default:
			_, _ = w.Write([]byte(versionedCompositionJSON))
		}
	}))
	defer srv.Close()

	repo := composition.NewRepository(newClient(t, srv))
	ctx := t.Context()
	if _, _, err := repo.GetVersionedObject(ctx, ehrIDFixture, compositionVOID); err != nil {
		t.Fatalf("GetVersionedObject: %v", err)
	}
	if _, _, err := repo.GetRevisionHistory(ctx, ehrIDFixture, compositionVOID); err != nil {
		t.Fatalf("GetRevisionHistory: %v", err)
	}
	if _, _, err := repo.GetVersion(ctx, ehrIDFixture, compositionVOID); err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if _, _, err := repo.GetVersionAtTime(ctx, ehrIDFixture, compositionVOID, atTime); err != nil {
		t.Fatalf("GetVersionAtTime: %v", err)
	}
	if _, _, err := repo.GetVersionByID(ctx, ehrIDFixture, compositionVOID, versionTwoUID); err != nil {
		t.Fatalf("GetVersionByID: %v", err)
	}
	want := []string{
		versionedPath + "?",
		versionedPath + "/revision_history?",
		versionedPath + "/version?",
		versionedPath + "/version?" + url.Values{"version_at_time": {atTimeWire}}.Encode(),
		versionedPath + "/version/" + string(versionTwoUID) + "?",
	}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Errorf("repository requests:\n%s\nwant:\n%s", strings.Join(requests, "\n"), strings.Join(want, "\n"))
	}
}

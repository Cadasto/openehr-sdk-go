package ehrstatus_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// The bodies below are canonical JSON. A version's data is the package's
// corpus EHR_STATUS (testkit/corpus/its_rest/ehr/ehr_status.json), read at
// test time by versionBody; version 2 of it follows version 1 (ehrStatusUID).
const (
	statusVOUID                             = "f1e2d3c4-b5a6-4978-89ab-cdef01234567"
	statusVersion2 openehrclient.VersionUID = statusVOUID + "::cdr.example::2"
	contributionID                          = "6a4e5b7c-1d2e-4f30-8a9b-0c1d2e3f4a5b"
	versionedPath                           = "/openehr/v1/ehr/" + string(ehrIDFixture) + "/versioned_ehr_status"
	timeCreated                             = "2015-01-20T19:30:22.765+01:00"
)

const creationAuditJSON = `{
	"_type": "AUDIT_DETAILS",
	"system_id": "cdr.example",
	"committer": {"_type": "PARTY_IDENTIFIED", "name": "Dr. Alice Example"},
	"time_committed": {"_type": "DV_DATE_TIME", "value": "` + timeCreated + `"},
	"change_type": {
		"_type": "DV_CODED_TEXT",
		"value": "creation",
		"defining_code": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "openehr"}, "code_string": "249"}
	}
}`

const modificationAuditJSON = `{
	"_type": "AUDIT_DETAILS",
	"system_id": "cdr.example",
	"committer": {"_type": "PARTY_IDENTIFIED", "name": "Dr. Alice Example"},
	"time_committed": {"_type": "DV_DATE_TIME", "value": "2016-03-01T09:00:00.5Z"},
	"change_type": {
		"_type": "DV_CODED_TEXT",
		"value": "modification",
		"defining_code": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "openehr"}, "code_string": "251"}
	}
}`

const contributionJSON = `{"_type": "OBJECT_REF", "id": {"_type": "HIER_OBJECT_ID", "value": "` + contributionID + `"}, "namespace": "local", "type": "CONTRIBUTION"}`

// versionFields are the ORIGINAL_VERSION members other than _type and data.
const versionFields = `"uid": {"_type": "OBJECT_VERSION_ID", "value": "` + string(statusVersion2) + `"},
	"preceding_version_uid": {"_type": "OBJECT_VERSION_ID", "value": "` + string(ehrStatusUID) + `"},
	"lifecycle_state": {
		"_type": "DV_CODED_TEXT",
		"value": "complete",
		"defining_code": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "openehr"}, "code_string": "532"}
	},
	"commit_audit": ` + modificationAuditJSON + `,
	"contribution": ` + contributionJSON

const versionNoDataJSON = `{"_type": "ORIGINAL_VERSION", ` + versionFields + `}`

// versionBody returns an ORIGINAL_VERSION whose data is the corpus EHR_STATUS.
func versionBody(t *testing.T) string {
	t.Helper()
	return `{"_type": "ORIGINAL_VERSION", ` + versionFields + `, "data": ` + string(readFixture(t)) + `}`
}

// importedVersionBody returns the other VERSION the pin's UVersionOfEhrStatus
// admits: an IMPORTED_VERSION wrapping the ORIGINAL_VERSION of versionBody.
func importedVersionBody(t *testing.T) string {
	t.Helper()
	return `{"_type": "IMPORTED_VERSION", "commit_audit": ` + creationAuditJSON +
		`, "contribution": ` + contributionJSON + `, "item": ` + versionBody(t) + `}`
}

const versionedObjectJSON = `{
	"_type": "VERSIONED_EHR_STATUS",
	"uid": {"_type": "HIER_OBJECT_ID", "value": "` + statusVOUID + `"},
	"owner_id": {"_type": "OBJECT_REF", "id": {"_type": "HIER_OBJECT_ID", "value": "` + string(ehrIDFixture) + `"}, "namespace": "local", "type": "EHR"},
	"time_created": {"_type": "DV_DATE_TIME", "value": "` + timeCreated + `"}
}`

const revisionHistoryJSON = `{
	"_type": "REVISION_HISTORY",
	"items": [
		{"_type": "REVISION_HISTORY_ITEM", "version_id": {"_type": "OBJECT_VERSION_ID", "value": "` + string(ehrStatusUID) + `"}, "audits": [` + creationAuditJSON + `]},
		{"_type": "REVISION_HISTORY_ITEM", "version_id": {"_type": "OBJECT_VERSION_ID", "value": "` + string(statusVersion2) + `"}, "audits": [` + modificationAuditJSON + `]}
	]
}`

// atTime is the pin's own version_at_time example, 19:30:22.765 at +01:00 on
// 20 January 2015; on the wire it is 2015-01-20T18:30:22.765Z.
var atTime = time.Date(2015, 1, 20, 19, 30, 22, 765_000_000, time.FixedZone("", 3600))

// versionedRead is one of the five versioned-object reads, reachable both as
// a package function (call) and through the Repository (repoCall).
type versionedRead struct {
	name     string
	path     string     // the wire path under the service base
	query    url.Values // the query the read sends
	body     string     // a 200 body the read decodes
	call     func(ctx context.Context, c *transport.Client) error
	repoCall func(ctx context.Context, r ehrstatus.Repository) error
}

func versionedReads(t *testing.T) []versionedRead {
	t.Helper()
	version := versionBody(t)
	return []versionedRead{
		{
			name: "GetVersionedObject",
			path: versionedPath,
			body: versionedObjectJSON,
			call: func(ctx context.Context, c *transport.Client) error {
				_, _, err := ehrstatus.GetVersionedObject(ctx, c, ehrIDFixture)
				return err
			},
			repoCall: func(ctx context.Context, r ehrstatus.Repository) error {
				_, _, err := r.GetVersionedObject(ctx, ehrIDFixture)
				return err
			},
		},
		{
			name: "GetRevisionHistory",
			path: versionedPath + "/revision_history",
			body: revisionHistoryJSON,
			call: func(ctx context.Context, c *transport.Client) error {
				_, _, err := ehrstatus.GetRevisionHistory(ctx, c, ehrIDFixture)
				return err
			},
			repoCall: func(ctx context.Context, r ehrstatus.Repository) error {
				_, _, err := r.GetRevisionHistory(ctx, ehrIDFixture)
				return err
			},
		},
		{
			name: "GetVersion",
			path: versionedPath + "/version",
			body: version,
			call: func(ctx context.Context, c *transport.Client) error {
				_, _, err := ehrstatus.GetVersion(ctx, c, ehrIDFixture)
				return err
			},
			repoCall: func(ctx context.Context, r ehrstatus.Repository) error {
				_, _, err := r.GetVersion(ctx, ehrIDFixture)
				return err
			},
		},
		{
			name:  "GetVersionAtTime",
			path:  versionedPath + "/version",
			query: url.Values{"version_at_time": {"2015-01-20T18:30:22.765Z"}},
			body:  version,
			call: func(ctx context.Context, c *transport.Client) error {
				_, _, err := ehrstatus.GetVersionAtTime(ctx, c, ehrIDFixture, atTime)
				return err
			},
			repoCall: func(ctx context.Context, r ehrstatus.Repository) error {
				_, _, err := r.GetVersionAtTime(ctx, ehrIDFixture, atTime)
				return err
			},
		},
		{
			name: "GetVersionByID",
			path: versionedPath + "/version/" + string(statusVersion2),
			body: version,
			call: func(ctx context.Context, c *transport.Client) error {
				_, _, err := ehrstatus.GetVersionByID(ctx, c, ehrIDFixture, statusVersion2)
				return err
			},
			repoCall: func(ctx context.Context, r ehrstatus.Repository) error {
				_, _, err := r.GetVersionByID(ctx, ehrIDFixture, statusVersion2)
				return err
			},
		},
	}
}

// versionCall is one of the three reads that return a version.
type versionCall struct {
	name string
	call func(ctx context.Context, c *transport.Client) (*ehrstatus.Version, *openehrclient.VersionMetadata, error)
}

func versionCalls() []versionCall {
	return []versionCall{
		{"GetVersion", func(ctx context.Context, c *transport.Client) (*ehrstatus.Version, *openehrclient.VersionMetadata, error) {
			return ehrstatus.GetVersion(ctx, c, ehrIDFixture)
		}},
		{"GetVersionAtTime", func(ctx context.Context, c *transport.Client) (*ehrstatus.Version, *openehrclient.VersionMetadata, error) {
			return ehrstatus.GetVersionAtTime(ctx, c, ehrIDFixture, atTime)
		}},
		{"GetVersionByID", func(ctx context.Context, c *transport.Client) (*ehrstatus.Version, *openehrclient.VersionMetadata, error) {
			return ehrstatus.GetVersionByID(ctx, c, ehrIDFixture, statusVersion2)
		}},
	}
}

// serveBody starts a server that answers every request 200 with body.
func serveBody(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// REQ-174: each read sends GET to its route asking for canonical JSON, and
// only the version-at-time read sends version_at_time, in UTC with its
// fraction of a second. The server sends no ETag or Location, so each read
// also succeeds without populated version metadata.
func TestVersionedReadsWire(t *testing.T) {
	for _, tc := range versionedReads(t) {
		t.Run(tc.name, func(t *testing.T) {
			var captured *http.Request
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = r.Clone(r.Context())
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			if err := tc.call(t.Context(), newClient(t, srv)); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if captured.Method != http.MethodGet {
				t.Errorf("%s method = %q, want GET", tc.name, captured.Method)
			}
			if captured.URL.Path != tc.path {
				t.Errorf("%s path = %q, want %q", tc.name, captured.URL.Path, tc.path)
			}
			if got := captured.Header.Get("Accept"); got != "application/json" {
				t.Errorf("%s Accept = %q, want application/json", tc.name, got)
			}
			if got, want := captured.URL.Query().Encode(), tc.query.Encode(); got != want {
				t.Errorf("%s query = %q, want %q", tc.name, got, want)
			}
		})
	}
}

// REQ-174: the versioned-object read returns the VERSIONED_EHR_STATUS.
func TestGetVersionedObjectDecodes(t *testing.T) {
	srv := serveBody(t, versionedObjectJSON)
	vo, _, err := ehrstatus.GetVersionedObject(t.Context(), newClient(t, srv), ehrIDFixture)
	if err != nil {
		t.Fatal(err)
	}
	if vo.UID.Value != statusVOUID {
		t.Errorf("GetVersionedObject uid = %q, want %q", vo.UID.Value, statusVOUID)
	}
	if vo.OwnerID == nil {
		t.Error("GetVersionedObject owner_id = nil, want the EHR reference")
	}
	if vo.TimeCreated.Value != timeCreated {
		t.Errorf("GetVersionedObject time_created = %q, want %q", vo.TimeCreated.Value, timeCreated)
	}
}

// REQ-174: the revision-history read returns the REVISION_HISTORY.
func TestGetRevisionHistoryDecodes(t *testing.T) {
	srv := serveBody(t, revisionHistoryJSON)
	rh, _, err := ehrstatus.GetRevisionHistory(t.Context(), newClient(t, srv), ehrIDFixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(rh.Items) != 2 {
		t.Fatalf("GetRevisionHistory items = %d, want 2", len(rh.Items))
	}
	for i, want := range []openehrclient.VersionUID{ehrStatusUID, statusVersion2} {
		if got := rh.Items[i].VersionID.Value; got != string(want) {
			t.Errorf("GetRevisionHistory items[%d].version_id = %q, want %q", i, got, want)
		}
		if n := len(rh.Items[i].Audits); n != 1 {
			t.Errorf("GetRevisionHistory items[%d].audits = %d, want 1", i, n)
		}
	}
}

// REQ-174: a version read decodes the ORIGINAL_VERSION envelope and its
// EHR_STATUS.
func TestGetVersionDecodesEnvelope(t *testing.T) {
	body := versionBody(t)
	for _, tc := range versionCalls() {
		t.Run(tc.name, func(t *testing.T) {
			srv := serveBody(t, body)
			v, _, err := tc.call(t.Context(), newClient(t, srv))
			if err != nil {
				t.Fatal(err)
			}
			if v.UID.Value != string(statusVersion2) {
				t.Errorf("%s uid = %q, want %q", tc.name, v.UID.Value, statusVersion2)
			}
			if v.VersionUID() != statusVersion2 {
				t.Errorf("%s VersionUID() = %q, want %q", tc.name, v.VersionUID(), statusVersion2)
			}
			if v.PrecedingVersionUID == nil || v.PrecedingVersionUID.Value != string(ehrStatusUID) {
				t.Errorf("%s preceding_version_uid = %+v, want %q", tc.name, v.PrecedingVersionUID, ehrStatusUID)
			}
			if got := v.LifecycleState.DefiningCode.CodeString; got != "532" {
				t.Errorf("%s lifecycle_state code = %q, want 532", tc.name, got)
			}
			if v.CommitAudit == nil {
				t.Errorf("%s commit_audit = nil, want the envelope's audit", tc.name)
			}
			if v.Contribution == nil {
				t.Errorf("%s contribution = nil, want the envelope's contribution", tc.name)
			}
			if v.Status == nil {
				t.Fatalf("%s Status = nil, want the decoded EHR_STATUS", tc.name)
			}
			if !v.Status.IsQueryable || !v.Status.IsModifiable {
				t.Errorf("%s Status flags = queryable %t, modifiable %t; want both true", tc.name, v.Status.IsQueryable, v.Status.IsModifiable)
			}
			if ref := v.Status.Subject.ExternalRef; ref == nil || ref.Namespace != "demographic" {
				t.Errorf("%s Status subject external_ref = %+v, want namespace demographic", tc.name, ref)
			}
		})
	}
}

// REQ-174: a version that carries no data returns no EHR_STATUS and no error.
func TestGetVersionWithoutData(t *testing.T) {
	for _, tc := range versionCalls() {
		t.Run(tc.name, func(t *testing.T) {
			srv := serveBody(t, versionNoDataJSON)
			v, _, err := tc.call(t.Context(), newClient(t, srv))
			if err != nil {
				t.Fatalf("%s(version without data) err = %v, want nil", tc.name, err)
			}
			if v.Status != nil {
				t.Errorf("%s(version without data) Status = %+v, want nil", tc.name, v.Status)
			}
			if v.UID.Value != string(statusVersion2) {
				t.Errorf("%s(version without data) uid = %q, want %q", tc.name, v.UID.Value, statusVersion2)
			}
		})
	}
}

// REQ-174, REQ-151: a version body whose _type is not ORIGINAL_VERSION fails
// as the typed decode failure, keeping the body, and never decodes to an
// empty version.
func TestGetVersionRefusesImportedVersion(t *testing.T) {
	body := importedVersionBody(t)
	for _, tc := range versionCalls() {
		t.Run(tc.name, func(t *testing.T) {
			srv := serveBody(t, body)
			v, _, err := tc.call(t.Context(), newClient(t, srv))
			de, ok := errors.AsType[*transport.DecodeError](err)
			if !ok || de == nil {
				t.Fatalf("%s(IMPORTED_VERSION) err = %v (%T), want a *transport.DecodeError", tc.name, err, err)
			}
			if string(de.Body) != body {
				t.Errorf("%s(IMPORTED_VERSION) DecodeError.Body = %q, want the response body", tc.name, de.Body)
			}
			if v != nil {
				t.Errorf("%s(IMPORTED_VERSION) version = %+v, want nil", tc.name, v)
			}
		})
	}
}

// REQ-174: an empty or null 2xx body, a 204 included, fails with
// ErrInvalidShape on every read.
func TestVersionedReadsRefuseNoBody(t *testing.T) {
	answers := []struct {
		name   string
		status int
		body   string
	}{
		{name: "empty", status: http.StatusOK, body: ""},
		{name: "null", status: http.StatusOK, body: "null"},
		{name: "204", status: http.StatusNoContent, body: ""},
	}
	for _, tc := range versionedReads(t) {
		for _, a := range answers {
			t.Run(tc.name+"/"+a.name, func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(a.status)
					_, _ = w.Write([]byte(a.body))
				}))
				defer srv.Close()

				err := tc.call(t.Context(), newClient(t, srv))
				if !errors.Is(err, transport.ErrInvalidShape) {
					t.Errorf("%s(%d %q) err = %v, want ErrInvalidShape", tc.name, a.status, a.body, err)
				}
			})
		}
	}
}

// REQ-174: a 404 maps to ErrNotFound on every read.
func TestVersionedReadsNotFound(t *testing.T) {
	for _, tc := range versionedReads(t) {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"not found","code":"NOT_FOUND"}`))
			}))
			defer srv.Close()

			if err := tc.call(t.Context(), newClient(t, srv)); !errors.Is(err, transport.ErrNotFound) {
				t.Errorf("%s(404) err = %v, want ErrNotFound", tc.name, err)
			}
		})
	}
}

// REQ-174: an empty ehr_id or version_uid, and a zero time on the
// version-at-time read, fail with ErrInvalidConfig before any request.
func TestVersionedReadsRefuseBeforeRequest(t *testing.T) {
	cases := []struct {
		name string
		call func(ctx context.Context, c *transport.Client) error
	}{
		{name: "GetVersionedObject/empty EHRID", call: func(ctx context.Context, c *transport.Client) error {
			_, _, err := ehrstatus.GetVersionedObject(ctx, c, "")
			return err
		}},
		{name: "GetRevisionHistory/empty EHRID", call: func(ctx context.Context, c *transport.Client) error {
			_, _, err := ehrstatus.GetRevisionHistory(ctx, c, "")
			return err
		}},
		{name: "GetVersion/empty EHRID", call: func(ctx context.Context, c *transport.Client) error {
			_, _, err := ehrstatus.GetVersion(ctx, c, "")
			return err
		}},
		{name: "GetVersionAtTime/empty EHRID", call: func(ctx context.Context, c *transport.Client) error {
			_, _, err := ehrstatus.GetVersionAtTime(ctx, c, "", atTime)
			return err
		}},
		{name: "GetVersionAtTime/zero time", call: func(ctx context.Context, c *transport.Client) error {
			_, _, err := ehrstatus.GetVersionAtTime(ctx, c, ehrIDFixture, time.Time{})
			return err
		}},
		{name: "GetVersionByID/empty EHRID", call: func(ctx context.Context, c *transport.Client) error {
			_, _, err := ehrstatus.GetVersionByID(ctx, c, "", statusVersion2)
			return err
		}},
		{name: "GetVersionByID/empty VersionUID", call: func(ctx context.Context, c *transport.Client) error {
			_, _, err := ehrstatus.GetVersionByID(ctx, c, ehrIDFixture, "")
			return err
		}},
	}
	body := versionBody(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			err := tc.call(t.Context(), newClient(t, srv))
			if !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("%s err = %v, want ErrInvalidConfig", tc.name, err)
			}
			// The transport's path check refuses an empty segment with
			// ErrInvalidConfig too, but wraps ErrInvalidPathSegment beside it:
			// this case fails if the leaf's own refusal goes.
			if errors.Is(err, transport.ErrInvalidPathSegment) {
				t.Errorf("%s err = %v, want the leaf's refusal, not the transport's path check", tc.name, err)
			}
			if n := hits.Load(); n != 0 {
				t.Errorf("%s: server saw %d request(s), want none", tc.name, n)
			}
		})
	}
}

// REQ-174, REQ-054: the ETag of a version-at-time response fills the
// metadata's version id.
func TestGetVersionAtTimeETag(t *testing.T) {
	body := versionBody(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+string(statusVersion2)+`"`)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	_, meta, err := ehrstatus.GetVersionAtTime(t.Context(), newClient(t, srv), ehrIDFixture, atTime)
	if err != nil {
		t.Fatal(err)
	}
	if meta == nil || meta.VersionUID != statusVersion2 {
		t.Errorf("GetVersionAtTime metadata = %+v, want VersionUID %q", meta, statusVersion2)
	}
}

// REQ-174: each new Repository method reaches its route.
func TestRepositoryVersionedReads(t *testing.T) {
	for _, tc := range versionedReads(t) {
		t.Run(tc.name, func(t *testing.T) {
			var captured *http.Request
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = r.Clone(r.Context())
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			if err := tc.repoCall(t.Context(), ehrstatus.NewRepository(newClient(t, srv))); err != nil {
				t.Fatalf("Repository.%s: %v", tc.name, err)
			}
			if captured.URL.Path != tc.path {
				t.Errorf("Repository.%s path = %q, want %q", tc.name, captured.URL.Path, tc.path)
			}
			if got, want := captured.URL.Query().Encode(), tc.query.Encode(); got != want {
				t.Errorf("Repository.%s query = %q, want %q", tc.name, got, want)
			}
		})
	}
}

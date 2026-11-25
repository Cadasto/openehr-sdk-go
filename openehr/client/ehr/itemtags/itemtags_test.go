package itemtags_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/itemtags"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
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

// REQ-059: the openehr-item-tag response header is surfaced as typed item
// tags.
func TestGetCompositionTags(t *testing.T) {
	const tagHeader = `key="category",value="final"`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("openehr-item-tag", tagHeader)
		w.Header().Set("ETag", `"v-1"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"_type":"COMPOSITION","name":{}}`))
	}))
	defer srv.Close()

	tags, _, err := itemtags.GetComposition(t.Context(), newClient(t, srv),
		"ehr-1", openehrclient.VersionOf("uid::sys::1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tags.Object) != 1 || tags.Object[0].Key != "category" {
		t.Fatalf("tags = %#v", tags.Object)
	}
}

// TestGetEHRStatusAndDirectoryRefuseBadRefs: a nil Ref, a nil pointer to
// either Ref type and an empty id fail with ErrInvalidConfig, under the
// name of the call, before any request, and never panic (REQ-025,
// idiom.md § No panics).
func TestGetEHRStatusAndDirectoryRefuseBadRefs(t *testing.T) { // REQ-025
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotImplemented)
	}))
	defer srv.Close()
	c := newClient(t, srv)

	calls := []struct {
		op   string
		call func(ref openehrclient.Ref) error
	}{
		{"itemtags.GetEHRStatus", func(ref openehrclient.Ref) error {
			_, _, err := itemtags.GetEHRStatus(t.Context(), c, "ehr-1", ref)
			return err
		}},
		{"itemtags.GetDirectory", func(ref openehrclient.Ref) error {
			_, _, err := itemtags.GetDirectory(t.Context(), c, "ehr-1", ref)
			return err
		}},
	}
	refs := []struct {
		name string
		ref  openehrclient.Ref
	}{
		{"nil Ref", nil},
		{"nil *ByVersionUID", (*openehrclient.ByVersionUID)(nil)},
		{"nil *ByVersionedObjectID", (*openehrclient.ByVersionedObjectID)(nil)},
		{"empty versioned-object id", openehrclient.LatestOf("")},
		{"empty version uid", openehrclient.VersionOf("")},
	}
	for _, call := range calls {
		for _, bad := range refs {
			t.Run(call.op+"/"+bad.name, func(t *testing.T) {
				before := hits.Load()
				err := call.call(bad.ref)
				if !errors.Is(err, transport.ErrInvalidConfig) {
					t.Fatalf("%s(%s) err = %v, want ErrInvalidConfig", call.op, bad.name, err)
				}
				if !strings.HasPrefix(err.Error(), call.op+": ") {
					t.Errorf("%s(%s) err = %q, want it to start with %q", call.op, bad.name, err, call.op+": ")
				}
				if n := hits.Load() - before; n != 0 {
					t.Errorf("%s(%s) sent %d requests, want none", call.op, bad.name, n)
				}
			})
		}
	}
}

// TestGetEHRStatusAndDirectoryReadAPointerRef: a non-nil pointer to either
// Ref type, which composition.Get and ValidateRef accept, reads the same
// target as the value it points to (REQ-059).
func TestGetEHRStatusAndDirectoryReadAPointerRef(t *testing.T) { // REQ-059
	const tagHeader = `key="category",value="final"`
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("openehr-item-tag", tagHeader)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/directory") {
			_, _ = w.Write([]byte(`{"_type":"FOLDER","name":{"_type":"DV_TEXT","value":"root"},"archetype_node_id":"openEHR-EHR-FOLDER.generic.v1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"_type":"EHR_STATUS","name":{"_type":"DV_TEXT","value":"EHR Status"},"archetype_node_id":"openEHR-EHR-EHR_STATUS.generic.v1","subject":{"_type":"PARTY_SELF"},"is_queryable":true,"is_modifiable":true}`))
	}))
	defer srv.Close()
	c := newClient(t, srv)

	cases := []struct {
		name     string
		call     func(ref openehrclient.Ref) (itemtags.Tags, error)
		ref      openehrclient.Ref
		wantPath string
	}{
		{"GetEHRStatus/*ByVersionUID", func(ref openehrclient.Ref) (itemtags.Tags, error) {
			tags, _, err := itemtags.GetEHRStatus(t.Context(), c, "ehr-1", ref)
			return tags, err
		}, &openehrclient.ByVersionUID{UID: "uid::sys::1"}, "/openehr/v1/ehr/ehr-1/ehr_status/uid::sys::1"},
		{"GetEHRStatus/*ByVersionedObjectID", func(ref openehrclient.Ref) (itemtags.Tags, error) {
			tags, _, err := itemtags.GetEHRStatus(t.Context(), c, "ehr-1", ref)
			return tags, err
		}, &openehrclient.ByVersionedObjectID{VOID: "uid"}, "/openehr/v1/ehr/ehr-1/ehr_status"},
		{"GetDirectory/*ByVersionUID", func(ref openehrclient.Ref) (itemtags.Tags, error) {
			tags, _, err := itemtags.GetDirectory(t.Context(), c, "ehr-1", ref)
			return tags, err
		}, &openehrclient.ByVersionUID{UID: "uid::sys::1"}, "/openehr/v1/ehr/ehr-1/directory/uid::sys::1"},
		{"GetDirectory/*ByVersionedObjectID", func(ref openehrclient.Ref) (itemtags.Tags, error) {
			tags, _, err := itemtags.GetDirectory(t.Context(), c, "ehr-1", ref)
			return tags, err
		}, &openehrclient.ByVersionedObjectID{VOID: "uid"}, "/openehr/v1/ehr/ehr-1/directory"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paths = nil
			tags, err := tc.call(tc.ref)
			if err != nil {
				t.Fatalf("%s(%#v) err = %v, want nil", tc.name, tc.ref, err)
			}
			if len(paths) != 1 || paths[0] != tc.wantPath {
				t.Errorf("%s(%#v) requested %q, want [%q]", tc.name, tc.ref, paths, tc.wantPath)
			}
			if len(tags.Object) != 1 || tags.Object[0].Key != "category" {
				t.Errorf("%s(%#v) object tags = %#v, want the one category tag", tc.name, tc.ref, tags.Object)
			}
		})
	}
}

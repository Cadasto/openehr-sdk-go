package demographic_test

// tags_test.go — the Demographic API's ITEM_TAG routes (wire.md § REQ-059,
// ITEM_TAG resource routes), held to the pinned demographic OpenAPI file
// (REQ-050): GET, PUT and DELETE on /demographic/{type}/{uid_based_id}/tags,
// and GET /demographic/tags with its three filters.

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// tagsRequest is what a test server saw of one request.
type tagsRequest struct {
	method, path, escapedPath, rawQuery string
	header                              http.Header
	body                                []byte
}

// tagsServer answers every request with status and body, records what it
// saw, and counts the requests, so a test can tell a refusal before any
// request from one the server answered.
func tagsServer(t *testing.T, status int, body []byte) (*transport.Client, *[]tagsRequest, *atomic.Int32) {
	t.Helper()
	var seen []tagsRequest
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		b, _ := io.ReadAll(r.Body)
		seen = append(seen, tagsRequest{
			method:      r.Method,
			path:        r.URL.Path,
			escapedPath: r.URL.EscapedPath(),
			rawQuery:    r.URL.RawQuery,
			header:      r.Header.Clone(),
			body:        b,
		})
		if len(body) > 0 {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return newClient(t, srv), &seen, &hits
}

// tagIDValue returns an OBJECT_REF id's value; the type registry decodes an
// identifier to a pointer.
func tagIDValue(id rm.ObjectID) string {
	switch v := id.(type) {
	case *rm.HierObjectID:
		return v.Value
	case *rm.ObjectVersionID:
		return v.Value
	default:
		return ""
	}
}

// allTypes is every PARTY type, each with its own tags routes.
var allTypes = []demographic.Type{
	demographic.Person, demographic.Organisation, demographic.Group, demographic.Agent, demographic.Role,
}

// TestGetTagsRoutesEveryPartyTypeAndTarget: GetTags reads the list from
// /demographic/{type}/{uid_based_id}/tags for each of the five PARTY types,
// with the uid taken from a version Ref or a versioned-object Ref, and
// returns each tag with its target and owner (REQ-059, REQ-050).
func TestGetTagsRoutesEveryPartyTypeAndTarget(t *testing.T) {
	body := fixture(t, "person_tags.json")
	refs := []struct {
		name string
		ref  openehrclient.Ref
		seg  string
	}{
		{"version", openehrclient.VersionOf(personVersion), personVersion},
		{"versioned object", openehrclient.LatestOf(personVOID), personVOID},
	}
	for _, typ := range allTypes {
		for _, rc := range refs {
			t.Run(string(typ)+"/"+rc.name, func(t *testing.T) {
				c, seen, _ := tagsServer(t, http.StatusOK, body)
				tags, err := demographic.GetTags(t.Context(), c, typ, rc.ref)
				if err != nil {
					t.Fatalf("GetTags: %v", err)
				}
				if len(*seen) != 1 {
					t.Fatalf("server saw %d requests, want 1", len(*seen))
				}
				r := (*seen)[0]
				wantPath := "/openehr/v1/demographic/" + string(typ) + "/" + rc.seg + "/tags"
				if r.method != http.MethodGet || r.path != wantPath || r.rawQuery != "" {
					t.Errorf("request = %s %s?%s, want GET %s with no query", r.method, r.path, r.rawQuery, wantPath)
				}
				if len(tags) != 2 {
					t.Fatalf("GetTags returned %d tags, want 2", len(tags))
				}
				first := tags[0]
				if first.Key != "flag" || first.Value != "follow-up" || first.TargetPath != "" {
					t.Errorf("tags[0] = %+v, want key flag, value follow-up, no target path", first.ItemTag())
				}
				if got := tagIDValue(first.Target.ID); got != personVersion || first.Target.Type != "PERSON" {
					t.Errorf("tags[0].Target = %q type %q, want %q type PERSON", got, first.Target.Type, personVersion)
				}
				if got := tagIDValue(first.OwnerID.ID); got != "7d44b88c-4199-4bad-97dc-d78268e01398" {
					t.Errorf("tags[0].OwnerID = %q, want the fixture's owner", got)
				}
				second := tags[1]
				if second.Key != "verified" || second.Value != "" || second.TargetPath != "/identities" {
					t.Errorf("tags[1] = %+v, want key verified, no value, target path /identities", second.ItemTag())
				}
			})
		}
	}
}

// TestGetTagsEmptyListIsEmpty: a target with no tags answers `[]`, which is
// an empty list and no error.
func TestGetTagsEmptyListIsEmpty(t *testing.T) {
	c, _, _ := tagsServer(t, http.StatusOK, []byte(`[]`))
	tags, err := demographic.GetTags(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID))
	if err != nil || len(tags) != 0 {
		t.Errorf("GetTags on [] = %v, %v; want an empty list and no error", tags, err)
	}
}

// tagsCall runs one of the three target-addressed tag calls against c, so a
// refusal can be checked on all of them at once.
type tagsCall struct {
	name string
	call func(c *transport.Client, typ demographic.Type, ref openehrclient.Ref) error
}

func targetCalls(t *testing.T) []tagsCall {
	return []tagsCall{
		{"GetTags", func(c *transport.Client, typ demographic.Type, ref openehrclient.Ref) error {
			_, err := demographic.GetTags(t.Context(), c, typ, ref)
			return err
		}},
		{"SetTags", func(c *transport.Client, typ demographic.Type, ref openehrclient.Ref) error {
			_, err := demographic.SetTags(t.Context(), c, typ, ref, []openehrclient.ItemTag{{Key: "flag"}})
			return err
		}},
		{"DeleteTag", func(c *transport.Client, typ demographic.Type, ref openehrclient.Ref) error {
			return demographic.DeleteTag(t.Context(), c, typ, ref, "flag")
		}},
	}
}

// TestTagCallsRefuseARefThatCarriesATime: the tags routes take no
// version_at_time, so a Ref that carries a time is refused with
// ErrInvalidConfig before any request, never sent without its time
// (REQ-059).
func TestTagCallsRefuseARefThatCarriesATime(t *testing.T) {
	at := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	for _, tc := range targetCalls(t) {
		t.Run(tc.name, func(t *testing.T) {
			c, _, hits := tagsServer(t, http.StatusNoContent, nil)
			err := tc.call(c, demographic.Person, openehrclient.LatestAtTime(personVOID, at))
			if !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("%s(LatestAtTime) err = %v, want ErrInvalidConfig", tc.name, err)
			}
			if n := hits.Load(); n != 0 {
				t.Errorf("%s(LatestAtTime) sent %d requests, want none", tc.name, n)
			}
		})
	}
}

// TestTagCallsRefuseBadTargets: an unknown PARTY type, a nil Ref, a Ref
// holding a nil pointer and a Ref with an empty id fail with
// ErrInvalidConfig before any request, never with a panic (REQ-025,
// idiom.md § No panics).
func TestTagCallsRefuseBadTargets(t *testing.T) {
	cases := []struct {
		name string
		typ  demographic.Type
		ref  openehrclient.Ref
	}{
		{"unknown type", demographic.Type("party"), openehrclient.LatestOf(personVOID)},
		{"nil Ref", demographic.Person, nil},
		{"nil *ByVersionUID", demographic.Person, (*openehrclient.ByVersionUID)(nil)},
		{"nil *ByVersionedObjectID", demographic.Person, (*openehrclient.ByVersionedObjectID)(nil)},
		{"empty versioned-object id", demographic.Person, openehrclient.LatestOf("")},
		{"empty version uid", demographic.Person, openehrclient.VersionOf("")},
	}
	for _, tc := range targetCalls(t) {
		for _, bad := range cases {
			t.Run(tc.name+"/"+bad.name, func(t *testing.T) {
				c, _, hits := tagsServer(t, http.StatusNoContent, nil)
				if err := tc.call(c, bad.typ, bad.ref); !errors.Is(err, transport.ErrInvalidConfig) {
					t.Errorf("err = %v, want ErrInvalidConfig", err)
				}
				if n := hits.Load(); n != 0 {
					t.Errorf("sent %d requests, want none", n)
				}
			})
		}
	}
}

// TestSetTagsSendsTheWholeList: SetTags PUTs the whole list as a JSON array
// of UPDATE_ITEM_TAG, leaving out an empty value or target path, with
// Prefer: return=minimal by default and no If-Match, and a 204 returns a nil
// list (REQ-059, REQ-094).
func TestSetTagsSendsTheWholeList(t *testing.T) {
	c, seen, _ := tagsServer(t, http.StatusNoContent, nil)
	in := []openehrclient.ItemTag{
		{Key: "flag", Value: "follow-up"},
		{Key: "verified", TargetPath: "/identities"},
		{Key: `say "hi"`, Value: "a\nb", TargetPath: "/name"},
	}
	out, err := demographic.SetTags(t.Context(), c, demographic.Role, openehrclient.VersionOf(personVersion), in)
	if err != nil {
		t.Fatalf("SetTags: %v", err)
	}
	if out != nil {
		t.Errorf("SetTags under return=minimal returned %v, want a nil list", out)
	}
	if len(*seen) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(*seen))
	}
	r := (*seen)[0]
	wantPath := "/openehr/v1/demographic/role/" + personVersion + "/tags"
	if r.method != http.MethodPut || r.path != wantPath {
		t.Errorf("request = %s %s, want PUT %s", r.method, r.path, wantPath)
	}
	if got := r.header.Get("Prefer"); got != "return=minimal" {
		t.Errorf("Prefer = %q, want return=minimal", got)
	}
	if got := r.header.Get("If-Match"); got != "" {
		t.Errorf("If-Match = %q, want none: the tags PUT takes no precondition", got)
	}
	if got := r.header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var sent []map[string]string
	if err := json.Unmarshal(r.body, &sent); err != nil {
		t.Fatalf("request body %q is not a JSON array of objects: %v", r.body, err)
	}
	want := []map[string]string{
		{"key": "flag", "value": "follow-up"},
		{"key": "verified", "target_path": "/identities"},
		{"key": `say "hi"`, "value": "a\nb", "target_path": "/name"},
	}
	if len(sent) != len(want) {
		t.Fatalf("sent %d tags, want %d: %s", len(sent), len(want), r.body)
	}
	for i := range want {
		if len(sent[i]) != len(want[i]) {
			t.Errorf("tag[%d] = %v, want exactly %v", i, sent[i], want[i])
			continue
		}
		for k, v := range want[i] {
			if sent[i][k] != v {
				t.Errorf("tag[%d][%q] = %q, want %q", i, k, sent[i][k], v)
			}
		}
	}
}

// TestSetTagsEmptyListSendsAnEmptyArray: a nil or empty list is sent as `[]`,
// which clears the target's tags; never as `null` or no body (REQ-059).
func TestSetTagsEmptyListSendsAnEmptyArray(t *testing.T) {
	for _, tc := range []struct {
		name string
		tags []openehrclient.ItemTag
	}{
		{"nil", nil},
		{"empty", []openehrclient.ItemTag{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, seen, _ := tagsServer(t, http.StatusNoContent, nil)
			if _, err := demographic.SetTags(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID), tc.tags); err != nil {
				t.Fatalf("SetTags(%s): %v", tc.name, err)
			}
			if len(*seen) != 1 {
				t.Fatalf("server saw %d requests, want 1", len(*seen))
			}
			if got := strings.TrimSpace(string((*seen)[0].body)); got != "[]" {
				t.Errorf("SetTags(%s) body = %q, want []", tc.name, got)
			}
		})
	}
}

// TestSetTagsRefusesAnEmptyKey: UPDATE_ITEM_TAG requires a key, so a tag
// without one fails with ErrInvalidConfig before any request (REQ-059).
func TestSetTagsRefusesAnEmptyKey(t *testing.T) {
	c, _, hits := tagsServer(t, http.StatusNoContent, nil)
	in := []openehrclient.ItemTag{{Key: "flag"}, {Value: "orphan"}}
	_, err := demographic.SetTags(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID), in)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("SetTags(empty key) err = %v, want ErrInvalidConfig", err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("SetTags(empty key) sent %d requests, want none", n)
	}
}

// TestSetTagsRefusesInvalidUTF8: a tag string that is not valid UTF-8 cannot
// be written as JSON, so it fails with ErrInvalidConfig before any request.
func TestSetTagsRefusesInvalidUTF8(t *testing.T) {
	c, _, hits := tagsServer(t, http.StatusNoContent, nil)
	in := []openehrclient.ItemTag{{Key: "flag", Value: "bad\xffvalue"}}
	_, err := demographic.SetTags(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID), in)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("SetTags(invalid UTF-8) err = %v, want ErrInvalidConfig", err)
	}
	if err != nil && strings.Contains(err.Error(), "bad") {
		t.Errorf("SetTags(invalid UTF-8) error text %q carries the tag value", err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("SetTags(invalid UTF-8) sent %d requests, want none", n)
	}
}

// TestSetTagsPreferRepresentationReturnsTheStoredList: with
// return=representation the 200 body is the stored list (REQ-094).
func TestSetTagsPreferRepresentationReturnsTheStoredList(t *testing.T) {
	c, seen, _ := tagsServer(t, http.StatusOK, fixture(t, "person_tags.json"))
	out, err := demographic.SetTags(t.Context(), c, demographic.Person, openehrclient.VersionOf(personVersion),
		[]openehrclient.ItemTag{{Key: "flag", Value: "follow-up"}, {Key: "verified", TargetPath: "/identities"}},
		demographic.WithTagsPrefer(transport.PreferRepresentation))
	if err != nil {
		t.Fatalf("SetTags: %v", err)
	}
	if got := (*seen)[0].header.Get("Prefer"); got != "return=representation" {
		t.Errorf("Prefer = %q, want return=representation", got)
	}
	if len(out) != 2 || out[0].Key != "flag" || out[1].Key != "verified" {
		t.Fatalf("SetTags returned %+v, want the fixture's two tags", out)
	}
	if got := tagIDValue(out[0].Target.ID); got != personVersion {
		t.Errorf("out[0].Target.ID = %q, want %q", got, personVersion)
	}
}

// TestSetTagsRepresentationWithoutABodyIsInvalidShape: return=representation
// is never silently downgraded, so a 204, or a 200 with an empty or null
// body, fails with ErrInvalidShape and is not a decode failure (REQ-094,
// REQ-151).
func TestSetTagsRepresentationWithoutABodyIsInvalidShape(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   []byte
	}{
		{"204", http.StatusNoContent, nil},
		{"200 empty", http.StatusOK, nil},
		{"200 null", http.StatusOK, []byte("null")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := tagsServer(t, tc.status, tc.body)
			out, err := demographic.SetTags(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID),
				[]openehrclient.ItemTag{{Key: "flag"}}, demographic.WithTagsPrefer(transport.PreferRepresentation))
			if !errors.Is(err, transport.ErrInvalidShape) {
				t.Errorf("err = %v, want ErrInvalidShape", err)
			}
			if _, ok := errors.AsType[*transport.DecodeError](err); ok {
				t.Errorf("err = %v is a DecodeError, want only ErrInvalidShape for a missing body", err)
			}
			if out != nil {
				t.Errorf("out = %v, want nil", out)
			}
		})
	}
}

// TestSetTagsOtherPreferModesReturnNoList: only return=representation
// returns a list; with no Prefer header, or with return=identifier, the
// request carries that preference and a 204 returns a nil list.
func TestSetTagsOtherPreferModesReturnNoList(t *testing.T) {
	for _, tc := range []struct {
		prefer transport.Prefer
		header string
	}{
		{transport.PreferDefault, ""},
		{transport.PreferIdentifier, "return=identifier"},
	} {
		t.Run(string(tc.prefer), func(t *testing.T) {
			c, seen, _ := tagsServer(t, http.StatusNoContent, nil)
			out, err := demographic.SetTags(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID),
				[]openehrclient.ItemTag{{Key: "flag"}}, demographic.WithTagsPrefer(tc.prefer))
			if err != nil || out != nil {
				t.Errorf("SetTags = %v, %v; want nil, nil", out, err)
			}
			if got := (*seen)[0].header.Get("Prefer"); got != tc.header {
				t.Errorf("Prefer = %q, want %q", got, tc.header)
			}
		})
	}
}

// TestDeleteTagRoutesAndEncodesTheKey: DeleteTag sends DELETE
// …/tags/{key}, with the key a path parameter percent-encoded exactly once
// (REQ-059, REQ-150).
func TestDeleteTagRoutesAndEncodesTheKey(t *testing.T) {
	for _, tc := range []struct {
		key, wantDecoded, wantEscaped string
	}{
		{"flag", "flag", "flag"},
		{"follow up", "follow up", "follow%20up"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			c, seen, _ := tagsServer(t, http.StatusNoContent, nil)
			if err := demographic.DeleteTag(t.Context(), c, demographic.Group, openehrclient.LatestOf(personVOID), tc.key); err != nil {
				t.Fatalf("DeleteTag(%q): %v", tc.key, err)
			}
			r := (*seen)[0]
			base := "/openehr/v1/demographic/group/" + personVOID + "/tags/"
			if r.method != http.MethodDelete || r.path != base+tc.wantDecoded || r.escapedPath != base+tc.wantEscaped {
				t.Errorf("request = %s %s (wire %s), want DELETE %s (wire %s)",
					r.method, r.path, r.escapedPath, base+tc.wantDecoded, base+tc.wantEscaped)
			}
		})
	}
}

// TestDeleteTagRefusesAnEmptyKey: a missing key fails with ErrInvalidConfig
// before any request, named as the missing key itself rather than left to
// the transport's empty-segment check (REQ-059).
func TestDeleteTagRefusesAnEmptyKey(t *testing.T) {
	c, _, hits := tagsServer(t, http.StatusNoContent, nil)
	err := demographic.DeleteTag(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID), "")
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("DeleteTag(empty key) err = %v, want ErrInvalidConfig", err)
	}
	if errors.Is(err, transport.ErrInvalidPathSegment) {
		t.Errorf("DeleteTag(empty key) err = %v, want the leaf's empty-key refusal, not a path-segment one", err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("DeleteTag(empty key) sent %d requests, want none", n)
	}
}

// TestDeleteTagRefusesASeparatorInTheKey: a key carrying `/` would move the
// request onto another route, so it is refused as an invalid path segment
// before any request (REQ-150).
func TestDeleteTagRefusesASeparatorInTheKey(t *testing.T) {
	for _, key := range []string{"a/b", "..", "."} {
		t.Run(key, func(t *testing.T) {
			c, _, hits := tagsServer(t, http.StatusNoContent, nil)
			err := demographic.DeleteTag(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID), key)
			if !errors.Is(err, transport.ErrInvalidPathSegment) || !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("DeleteTag(%q) err = %v, want ErrInvalidPathSegment and ErrInvalidConfig", key, err)
			}
			if n := hits.Load(); n != 0 {
				t.Errorf("DeleteTag(%q) sent %d requests, want none", key, n)
			}
		})
	}
}

// TestListTagsSendsOnlyTheFiltersGiven: ListTags reads GET /demographic/tags
// with tag_key, tag_value and tag_target_path from their options, and with
// no option sends no query, which lists every tag (REQ-059, REQ-050).
func TestListTagsSendsOnlyTheFiltersGiven(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []demographic.ListTagsOption
		want string
	}{
		{"none", nil, ""},
		{"key", []demographic.ListTagsOption{demographic.WithTagKey("flag")}, "tag_key=flag"},
		{"value", []demographic.ListTagsOption{demographic.WithTagValue("follow up")}, "tag_value=follow%20up"},
		{"target path", []demographic.ListTagsOption{demographic.WithTagTargetPath("/identities")}, "tag_target_path=%2Fidentities"},
		{"all three", []demographic.ListTagsOption{
			demographic.WithTagTargetPath("/identities"), demographic.WithTagValue("follow-up"), demographic.WithTagKey("flag"),
		}, "tag_key=flag&tag_target_path=%2Fidentities&tag_value=follow-up"},
		{"empty values omitted", []demographic.ListTagsOption{
			demographic.WithTagKey(""), demographic.WithTagValue(""), demographic.WithTagTargetPath(""),
		}, ""},
		{"nil option skipped", []demographic.ListTagsOption{nil, demographic.WithTagKey("flag")}, "tag_key=flag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, seen, _ := tagsServer(t, http.StatusOK, fixture(t, "demographic_tags.json"))
			tags, err := demographic.ListTags(t.Context(), c, tc.opts...)
			if err != nil {
				t.Fatalf("ListTags: %v", err)
			}
			r := (*seen)[0]
			if r.method != http.MethodGet || r.path != "/openehr/v1/demographic/tags" || r.rawQuery != tc.want {
				t.Errorf("request = %s %s?%s, want GET /openehr/v1/demographic/tags?%s", r.method, r.path, r.rawQuery, tc.want)
			}
			if len(tags) != 3 {
				t.Fatalf("ListTags returned %d tags, want 3", len(tags))
			}
			var types []string
			for _, tag := range tags {
				types = append(types, tag.Target.Type)
			}
			if got := strings.Join(types, ","); got != "PERSON,ORGANISATION,AGENT" {
				t.Errorf("target types = %s, want PERSON,ORGANISATION,AGENT: target.type tells the party classes apart", got)
			}
		})
	}
}

// TestTagCallsMapWireErrors: a 404 (unknown uid or key) is ErrNotFound on
// the three target calls, and a 400 on the listing is a WireError with no
// sentinel, as everywhere else in the package.
func TestTagCallsMapWireErrors(t *testing.T) {
	for _, tc := range targetCalls(t) {
		t.Run(tc.name+"/404", func(t *testing.T) {
			c, _, _ := tagsServer(t, http.StatusNotFound, nil)
			if err := tc.call(c, demographic.Person, openehrclient.LatestOf(personVOID)); !errors.Is(err, transport.ErrNotFound) {
				t.Errorf("err = %v, want ErrNotFound", err)
			}
		})
	}
	t.Run("ListTags/400", func(t *testing.T) {
		c, _, _ := tagsServer(t, http.StatusBadRequest, nil)
		tags, err := demographic.ListTags(t.Context(), c, demographic.WithTagKey("flag"))
		we, ok := errors.AsType[*transport.WireError](err)
		if !ok || we.StatusCode != http.StatusBadRequest {
			t.Errorf("ListTags(400) err = %v, want a WireError with status 400", err)
		}
		if tags != nil {
			t.Errorf("ListTags(400) = %v, want nil", tags)
		}
	})
}

// TestTagListDecodeFailuresAreTyped: a 2xx list body that does not decode is
// a *transport.DecodeError carrying the method, the route template and the
// bytes the server sent, on every call that decodes a list (REQ-151).
func TestTagListDecodeFailuresAreTyped(t *testing.T) {
	const notAList = `{"key":"flag"}`
	cases := []struct {
		name, method, route string
		call                func(c *transport.Client) error
	}{
		{"GetTags", http.MethodGet, "/demographic/person/{uid_based_id}/tags", func(c *transport.Client) error {
			_, err := demographic.GetTags(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID))
			return err
		}},
		{"SetTags", http.MethodPut, "/demographic/person/{uid_based_id}/tags", func(c *transport.Client) error {
			_, err := demographic.SetTags(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID),
				[]openehrclient.ItemTag{{Key: "flag"}}, demographic.WithTagsPrefer(transport.PreferRepresentation))
			return err
		}},
		{"ListTags", http.MethodGet, "/demographic/tags", func(c *transport.Client) error {
			_, err := demographic.ListTags(t.Context(), c)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := tagsServer(t, http.StatusOK, []byte(notAList))
			err := tc.call(c)
			de, ok := errors.AsType[*transport.DecodeError](err)
			if !ok {
				t.Fatalf("err = %v, want a *transport.DecodeError", err)
			}
			if de.Method != tc.method || de.Route != tc.route || string(de.Body) != notAList {
				t.Errorf("DecodeError = %s %s body %q, want %s %s body %q", de.Method, de.Route, de.Body, tc.method, tc.route, notAList)
			}
			if _, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
				t.Errorf("err = %v, want a shape failure, not a syntax one", err)
			}
		})
	}
}

// TestTagReadsEmptyBodyIsInvalidShape: a 2xx with no body where a list was
// asked for is ErrInvalidShape, not an empty list (REQ-151).
func TestTagReadsEmptyBodyIsInvalidShape(t *testing.T) {
	c, _, _ := tagsServer(t, http.StatusOK, nil)
	if _, err := demographic.GetTags(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID)); !errors.Is(err, transport.ErrInvalidShape) {
		t.Errorf("GetTags(empty 200) err = %v, want ErrInvalidShape", err)
	}
	if _, err := demographic.ListTags(t.Context(), c); !errors.Is(err, transport.ErrInvalidShape) {
		t.Errorf("ListTags(empty 200) err = %v, want ErrInvalidShape", err)
	}
}

// TestTagsRepositoryWiring: the Repository delegates each tag call to the
// package function with the same arguments.
func TestTagsRepositoryWiring(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+r.Header.Get("Prefer"))
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case http.MethodPut:
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	repo := demographic.NewRepository(newClient(t, srv))
	ctx := t.Context()
	ref := openehrclient.VersionOf(personVersion)
	if _, err := repo.GetTags(ctx, demographic.Agent, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetTags(ctx, demographic.Agent, ref, nil, demographic.WithTagsPrefer(transport.PreferRepresentation)); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteTag(ctx, demographic.Agent, ref, "flag"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ListTags(ctx, demographic.WithTagKey("flag")); err != nil {
		t.Fatal(err)
	}
	base := "/openehr/v1/demographic/agent/" + personVersion + "/tags"
	want := []string{
		"GET " + base + "? ",
		"PUT " + base + "? return=representation",
		"DELETE " + base + "/flag? ",
		"GET /openehr/v1/demographic/tags?tag_key=flag ",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("repository requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

package definition_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestGetStoredQueryEmptyBody verifies that a 200 response with an
// empty body does not return a decode error. CDRs may legally return
// 200/204 with no body on a GET; without the guard, json.Unmarshal
// yields "unexpected end of JSON input".
func TestGetStoredQueryEmptyBody(t *testing.T) {
	const wantName = "org.openehr::vitals"
	const wantVersion = "1.0.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// intentionally empty body
	}))
	defer srv.Close()

	meta, _, err := definition.GetStoredQuery(t.Context(), newClient(t, srv), wantName, wantVersion)
	if err != nil {
		t.Fatalf("GetStoredQuery: unexpected error on empty body: %v", err)
	}
	if meta == nil {
		t.Fatal("GetStoredQuery: returned nil metadata")
	}
	if meta.Name != wantName {
		t.Errorf("Name = %q, want %q", meta.Name, wantName)
	}
	if meta.Version != wantVersion {
		t.Errorf("Version = %q, want %q", meta.Version, wantVersion)
	}
}

func TestPutStoredQuery(t *testing.T) {
	var captured *http.Request
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"name": "org.openehr::vitals",
			"type": "aql",
			"version": "1.0.0",
			"q": "SELECT c FROM EHR e CONTAINS COMPOSITION c"
		}`))
	}))
	defer srv.Close()

	meta, _, err := definition.PutStoredQuery(
		t.Context(), newClient(t, srv),
		"org.openehr::vitals",
		"SELECT c FROM EHR e CONTAINS COMPOSITION c",
	)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPut {
		t.Errorf("method = %q", captured.Method)
	}
	if !strings.Contains(captured.URL.Path, "org.openehr") {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if captured.Header.Get("Content-Type") != "text/plain" {
		t.Errorf("content-type = %q", captured.Header.Get("Content-Type"))
	}
	if body != "SELECT c FROM EHR e CONTAINS COMPOSITION c" {
		t.Errorf("body = %q", body)
	}
	if meta.Name != "org.openehr::vitals" {
		t.Errorf("name = %q", meta.Name)
	}
	if got := captured.URL.Query().Get("query_type"); got != "AQL" {
		t.Errorf("query_type = %q, want AQL", got)
	}
}

// TestPutStoredQueryParsesLocationHeader pins REQ-057 finding B: the
// canonical ITS-REST `200_StoredQuery_stored` response shape is a
// `Location` header and no body. The no-version PutStoredQuery MUST
// recover the server-assigned `{name, version}` from
// `Location: …/definition/query/{name}/{version}` so the caller learns
// the assigned version without relying on a non-spec body.
func TestPutStoredQueryParsesLocationHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/openehr/v1/definition/query/org.openehr::vitals/3.2.1")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	meta, _, err := definition.PutStoredQuery(
		t.Context(), newClient(t, srv),
		"org.openehr::vitals",
		"SELECT c FROM EHR e CONTAINS COMPOSITION c",
	)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name != "org.openehr::vitals" {
		t.Errorf("name = %q, want org.openehr::vitals", meta.Name)
	}
	if meta.Version != "3.2.1" {
		t.Errorf("version = %q, want 3.2.1 (parsed from Location)", meta.Version)
	}
}

// TestPutStoredQueryAbsoluteLocation accepts an absolute Location URL too
// — some servers return a full https://host/… URL on 201/200.
func TestPutStoredQueryAbsoluteLocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://example.org/openehr/v1/definition/query/org.openehr::vitals/4.0.0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	meta, _, err := definition.PutStoredQuery(
		t.Context(), newClient(t, srv),
		"org.openehr::vitals",
		"SELECT 1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Version != "4.0.0" {
		t.Errorf("version = %q, want 4.0.0 (parsed from absolute Location)", meta.Version)
	}
}

// TestPutStoredQueryMalformedLocationFallsThrough covers the malformed-
// Location branch: parse failure MUST drop through to body decode (or, if
// no body, the synthesised metadata with the caller's input version) —
// no error surfaces. A deficient server should not break the call.
func TestPutStoredQueryMalformedLocationFallsThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Location with only one path segment after the host; parser
		// requires two non-empty trailing segments.
		w.Header().Set("Location", "https://example.org/something")
		w.WriteHeader(http.StatusOK)
		// No body either, exercising the synthesised fallback.
	}))
	defer srv.Close()

	meta, _, err := definition.PutStoredQuery(
		t.Context(), newClient(t, srv),
		"org.openehr::vitals",
		"SELECT 1",
	)
	if err != nil {
		t.Fatalf("malformed Location should not surface an error: %v", err)
	}
	if meta.Name != "org.openehr::vitals" {
		t.Errorf("name = %q, want org.openehr::vitals (synthesised fallback)", meta.Name)
	}
	// no-version PutStoredQuery passes "" as version; synthesised fallback
	// returns that, not a spurious value.
	if meta.Version != "" {
		t.Errorf("version = %q, want \"\" (synthesised fallback for no-version put)", meta.Version)
	}
}

// TestPutStoredQueryVersionlessLocationFallsThrough covers a Location that
// names the query but omits the assigned version
// (…/definition/query/{name}). The parser is anchored on the `query`
// segment requiring exactly {name}/{version} after it, so this falls
// through to the synthesised metadata rather than mis-parsing
// "query"/{name} into a wrong {name, version}.
func TestPutStoredQueryVersionlessLocationFallsThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/openehr/v1/definition/query/org.openehr::vitals")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	meta, _, err := definition.PutStoredQuery(
		t.Context(), newClient(t, srv),
		"org.openehr::vitals",
		"SELECT 1",
	)
	if err != nil {
		t.Fatalf("version-less Location should not surface an error: %v", err)
	}
	if meta.Name != "org.openehr::vitals" {
		t.Errorf("name = %q, want org.openehr::vitals (synthesised, not mis-parsed 'query')", meta.Name)
	}
	if meta.Version != "" {
		t.Errorf("version = %q, want \"\" (no version assigned; must not mis-parse the name as version)", meta.Version)
	}
}

// TestPutStoredQueryLocationPreferredOverBody pins the decode order:
// Location wins over body when both are present.
func TestPutStoredQueryLocationPreferredOverBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/openehr/v1/definition/query/org.openehr::vitals/5.0.0")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"org.openehr::vitals","version":"4.0.0"}`))
	}))
	defer srv.Close()

	meta, _, err := definition.PutStoredQuery(
		t.Context(), newClient(t, srv),
		"org.openehr::vitals",
		"SELECT 1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Version != "5.0.0" {
		t.Errorf("version = %q, want 5.0.0 (Location wins over body)", meta.Version)
	}
}

func TestPutStoredQueryVersion(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	meta, _, err := definition.PutStoredQueryVersion(
		t.Context(), newClient(t, srv),
		"org.openehr::vitals", "1.2.0",
		"SELECT c FROM EHR e CONTAINS COMPOSITION c",
	)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPut {
		t.Errorf("method = %q", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/definition/query/org.openehr::vitals/1.2.0" {
		t.Errorf("path = %q, want …/org.openehr::vitals/1.2.0", captured.URL.Path)
	}
	if meta.Version != "1.2.0" {
		t.Errorf("version = %q, want 1.2.0", meta.Version)
	}
}

func TestPutStoredQueryVersionRejectsEmpty(t *testing.T) {
	_, _, err := definition.PutStoredQueryVersion(t.Context(), nil, "org.openehr::vitals", "", "SELECT 1")
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestPutStoredQueryVersionConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version already exists","code":"CONFLICT"}`))
	}))
	defer srv.Close()
	_, _, err := definition.PutStoredQueryVersion(t.Context(), newClient(t, srv),
		"org.openehr::vitals", "1.2.0", "SELECT c FROM EHR e CONTAINS COMPOSITION c")
	if !errors.Is(err, transport.ErrVersionConflict) {
		t.Errorf("409 should map to ErrVersionConflict, got %v", err)
	}
}

// TestPutStoredQueryReservedName pins the store side of REQ-057's reserved
// name: the upstream contract (query-validation.openapi.yaml, the
// Qualified_query_name description) reserves the query-name `aql`
// case-insensitively, so both store operations refuse it — for any case
// variant, with or without a namespace — before any request is issued.
func TestPutStoredQueryReservedName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server received %s %s; the reserved name must be refused before any request", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	c := newClient(t, srv)

	for _, name := range []string{"aql", "AQL", "Aql", " aql ", "ehr::aql", "org.openehr::AQL"} {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				label string
				call  func() error
			}{
				{"PutStoredQuery", func() error {
					_, _, err := definition.PutStoredQuery(t.Context(), c, name, "SELECT 1")
					return err
				}},
				{"PutStoredQueryVersion", func() error {
					_, _, err := definition.PutStoredQueryVersion(t.Context(), c, name, "1.0.0", "SELECT 1")
					return err
				}},
			} {
				t.Run(tc.label, func(t *testing.T) {
					err := tc.call()
					if !errors.Is(err, transport.ErrInvalidConfig) {
						t.Fatalf("err = %v, want one wrapping ErrInvalidConfig", err)
					}
					// The diagnostic names the reserved-name rule rather
					// than echoing the input.
					if !strings.Contains(err.Error(), "reserved") {
						t.Errorf("err = %q, want it to name the reserved-name rule", err)
					}
				})
			}
		})
	}
}

// TestPutStoredQueryReservedNameScope pins the rule's edges. The reservation
// covers only the query-name part of `[{namespace}::]{query-name}`: a name
// that merely contains "aql", and a namespace that IS "aql", are ordinary
// names and reach the wire verbatim. The read, list and delete operations
// pass a reserved name through too — a deployment that stored one anyway
// must stay reachable for retrieval and cleanup — so each of the three
// asserts the reserved name reached the request path unaltered (REQ-057).
func TestPutStoredQueryReservedNameScope(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := newClient(t, srv)

	for _, name := range []string{"aql.reports", "aqlx", "org.example.aql", "aql::reports", "myaql"} {
		t.Run("store_"+name, func(t *testing.T) {
			_, _, err := definition.PutStoredQuery(t.Context(), c, name, "SELECT 1")
			if err != nil {
				t.Fatalf("PutStoredQuery(%q) = %v, want it to pass through", name, err)
			}
			if want := "/openehr/v1/definition/query/" + name; gotPath != want {
				t.Errorf("path = %q, want %q", gotPath, want)
			}
		})
	}

	t.Run("get_reserved_name_passes_through", func(t *testing.T) {
		_, _, err := definition.GetStoredQuery(t.Context(), c, "aql", "1.0.0")
		if err != nil {
			t.Fatalf("GetStoredQuery(\"aql\") = %v, want pass-through (read side is for remediation)", err)
		}
		if want := "/openehr/v1/definition/query/aql/1.0.0"; gotPath != want {
			t.Errorf("path = %q, want %q", gotPath, want)
		}
	})

	t.Run("list_reserved_name_passes_through", func(t *testing.T) {
		_, _, err := definition.ListStoredQueries(t.Context(), c, "ehr::aql")
		if err != nil {
			t.Fatalf("ListStoredQueries(\"ehr::aql\") = %v, want pass-through (list side is for remediation)", err)
		}
		if want := "/openehr/v1/definition/query/ehr::aql"; gotPath != want {
			t.Errorf("path = %q, want %q", gotPath, want)
		}
	})

	t.Run("delete_reserved_name_passes_through", func(t *testing.T) {
		_, err := definition.DeleteStoredQuery(t.Context(), c, "aql", "1.0.0")
		if err != nil {
			t.Fatalf("DeleteStoredQuery(\"aql\") = %v, want pass-through (delete side is for remediation)", err)
		}
		if want := "/openehr/v1/definition/query/aql/1.0.0"; gotPath != want {
			t.Errorf("path = %q, want %q", gotPath, want)
		}
	})
}

// TestListStoredQueries pins the happy path: the documented descriptor
// fields decode, and `saved` in the pin's own example shape — zoned,
// fractional-second RFC 3339 — lands on the exact instant it names
// (REQ-144).
func TestListStoredQueries(t *testing.T) {
	c := jsonServerClient(t, `[{
		"name":"org.openehr::vitals",
		"type":"AQL",
		"version":"1.0.0",
		"saved":"2017-07-16T19:20:30.450+01:00",
		"q":"SELECT c FROM EHR e CONTAINS COMPOSITION c"
	}]`)

	list, _, err := definition.ListStoredQueries(t.Context(), c, "")
	if err != nil {
		t.Fatalf("ListStoredQueries = %v, want nil error", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	got := list[0]
	if got.Name != "org.openehr::vitals" {
		t.Errorf("Name = %q, want org.openehr::vitals", got.Name)
	}
	if got.Type != "AQL" {
		t.Errorf("Type = %q, want AQL", got.Type)
	}
	if got.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", got.Version)
	}
	if got.Q != "SELECT c FROM EHR e CONTAINS COMPOSITION c" {
		t.Errorf("Q = %q, want the stored AQL text", got.Q)
	}
	want := time.Date(2017, 7, 16, 19, 20, 30, 450_000_000, time.FixedZone("+01:00", 60*60))
	if !got.Saved.Equal(want) {
		t.Errorf("Saved = %s, want %s", got.Saved.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
	if _, leaked := got.Extras["saved"]; leaked {
		t.Error("saved leaked into Extras instead of Saved")
	}
}

// TestListStoredQueriesEmpty is the stored-query twin of
// TestListTemplatesEmpty: an empty 2xx body yields a non-nil zero-length
// slice and a nil error, so a caller who re-serialises the result publishes
// an empty JSON array rather than null. Restoring the bare `return nil`
// fails this test.
func TestListStoredQueriesEmpty(t *testing.T) {
	// REQ-144
	for _, status := range []int{http.StatusOK, http.StatusNoContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()
			list, _, err := definition.ListStoredQueries(t.Context(), newClient(t, srv), "")
			if err != nil {
				t.Fatal(err)
			}
			if list == nil {
				t.Errorf("list = nil on %d, want a non-nil empty slice (a nil slice marshals as JSON null)", status)
			}
			if len(list) != 0 {
				t.Errorf("expected empty list on %d, got %d items", status, len(list))
			}
		})
	}
}

// TestListStoredQueriesZoneLessSaved pins the `saved` arm of the tolerance
// — the keyed REQ-095 exception, since `saved` is pinned `format:
// date-time`: a zone-less value decodes as UTC rather than failing the
// whole list (REQ-144).
func TestListStoredQueriesZoneLessSaved(t *testing.T) {
	c := jsonServerClient(t, `[{"name":"org.openehr::vitals","saved":"2022-03-30T07:18:13.591"}]`)

	list, _, err := definition.ListStoredQueries(t.Context(), c, "")
	if err != nil {
		t.Fatalf("ListStoredQueries = %v, want nil error", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	want := time.Date(2022, 3, 30, 7, 18, 13, 591_000_000, time.UTC)
	if !list[0].Saved.Equal(want) {
		t.Errorf("Saved = %s, want %s", list[0].Saved.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

// TestListStoredQueriesUnparseableSavedFails gives the `saved` arm its own
// refusal pin: a non-empty value matching no accepted layout fails the
// list call naming the field, never a silent zero. Removing the failure
// guard fails this test (REQ-144).
func TestListStoredQueriesUnparseableSavedFails(t *testing.T) {
	c := jsonServerClient(t, `[{"name":"org.openehr::vitals","saved":"not-a-time"}]`)

	list, _, err := definition.ListStoredQueries(t.Context(), c, "")
	if err == nil {
		t.Fatalf("ListStoredQueries = nil error with list %+v, want a decode failure", list)
	}
	if !strings.Contains(err.Error(), "saved") {
		t.Errorf("err = %q, want it to name saved", err)
	}
	if list != nil {
		t.Errorf("list = %+v, want nil on decode failure", list)
	}
}

// TestStoredQueryMetadataExtrasRoundTrip pins the stored-query
// descriptor's encode contract (REQ-144): deployment-specific fields
// preserved in Extras on decode are re-emitted on encode, and an Extras
// key colliding with a documented field name is ignored — the documented
// field is authoritative. Deleting MarshalJSON drops the Extras keys
// silently, because Extras is `json:"-"`.
//
// Every row asserts the full documented field set, so losing `q` or
// `saved` on the merged (non-empty Extras) path fails here instead of
// passing unseen. The two collision rows cover both mechanisms: an
// emitted documented key (`name`) and an omitted one (`saved`, which
// carries omitzero — nothing is emitted for the overlay to win with, so
// only deleting the known key from the Extras clone ignores it).
func TestStoredQueryMetadataExtrasRoundTrip(t *testing.T) {
	const (
		withSaved    = `"name":"org.openehr::vitals","type":"AQL","version":"1.0.0","saved":"2017-07-16T19:20:30.450+01:00","q":"SELECT 1"`
		withoutSaved = `"name":"org.openehr::vitals","type":"AQL","version":"1.0.0","q":"SELECT 1"`
		uriJSON      = `"https://example.example/q"`
	)
	// Saved is compared with .Equal, never byte-wise: the decode re-
	// canonicalises `…30.450+01:00` as `…30.45+01:00`, so the re-emitted
	// spelling differs from the wire spelling while naming the same instant.
	saved := time.Date(2017, 7, 16, 19, 20, 30, 450_000_000, time.FixedZone("+01:00", 60*60))

	for _, tc := range []struct {
		label string
		body  string
		// inject is applied to the decoded value before re-encoding. A
		// collision is always caller-constructed: UnmarshalJSON never routes
		// a documented field name into Extras.
		inject map[string]json.RawMessage
		// wantExtras is the exact JSON expected under each surviving Extras
		// key after the round trip; any other key is a failure.
		wantExtras map[string]string
		wantSaved  time.Time
	}{
		{
			label:     "no extras",
			body:      `{` + withSaved + `}`,
			wantSaved: saved,
		},
		{
			label:      "one unknown extra",
			body:       `{` + withSaved + `,"uri":` + uriJSON + `}`,
			wantExtras: map[string]string{"uri": uriJSON},
			wantSaved:  saved,
		},
		{
			label: "colliding extra on an emitted field is ignored",
			body:  `{` + withSaved + `,"uri":` + uriJSON + `}`,
			inject: map[string]json.RawMessage{
				"name": json.RawMessage(`"caller-supplied name"`),
			},
			wantExtras: map[string]string{"uri": uriJSON},
			wantSaved:  saved,
		},
		{
			label: "colliding extra on an omitted field is ignored",
			// No `saved` on the wire, so Saved is zero and omitzero emits no
			// key. A last-wins merge would publish the caller's bogus value,
			// which this package's own UnmarshalJSON then rejects — the
			// re-decode below is the assertion that catches it.
			body: `{` + withoutSaved + `,"uri":` + uriJSON + `}`,
			inject: map[string]json.RawMessage{
				"saved": json.RawMessage(`"16/07/2017"`),
			},
			wantExtras: map[string]string{"uri": uriJSON},
			wantSaved:  time.Time{},
		},
	} {
		t.Run(tc.label, func(t *testing.T) {
			var meta definition.StoredQueryMetadata
			if err := json.Unmarshal([]byte(tc.body), &meta); err != nil {
				t.Fatalf("Unmarshal(wire) = %v, want nil error", err)
			}
			if len(tc.wantExtras) > 0 {
				if _, ok := meta.Extras["uri"]; !ok {
					t.Fatalf("premise gone: Extras = %v, want uri to land there on decode", meta.Extras)
				}
			}
			for k, v := range tc.inject {
				if meta.Extras == nil {
					meta.Extras = map[string]json.RawMessage{}
				}
				meta.Extras[k] = v
			}
			out, err := json.Marshal(meta)
			if err != nil {
				t.Fatalf("Marshal = %v, want nil error", err)
			}
			var got definition.StoredQueryMetadata
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("Unmarshal(re-encoded %s) = %v, want nil error — MarshalJSON emitted what this package cannot decode", out, err)
			}
			if got.Name != "org.openehr::vitals" {
				t.Errorf("Name = %q, want org.openehr::vitals (the documented field is authoritative)", got.Name)
			}
			if got.Type != "AQL" {
				t.Errorf("Type = %q, want AQL", got.Type)
			}
			if got.Version != "1.0.0" {
				t.Errorf("Version = %q, want 1.0.0", got.Version)
			}
			if got.Q != "SELECT 1" {
				t.Errorf("Q = %q, want SELECT 1", got.Q)
			}
			if !got.Saved.Equal(tc.wantSaved) {
				t.Errorf("Saved = %s, want %s", got.Saved.Format(time.RFC3339Nano), tc.wantSaved.Format(time.RFC3339Nano))
			}
			if len(got.Extras) != len(tc.wantExtras) {
				t.Errorf("Extras = %v, want exactly the %d unknown key(s) %v", got.Extras, len(tc.wantExtras), tc.wantExtras)
			}
			for k, want := range tc.wantExtras {
				raw, ok := got.Extras[k]
				if !ok {
					t.Errorf("Extras[%q] dropped on re-encode: %s", k, out)
					continue
				}
				if string(raw) != want {
					t.Errorf("Extras[%q] = %s, want %s", k, raw, want)
				}
			}
		})
	}
}

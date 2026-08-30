package definition_test

// decode_error_test.go — REQ-151 § Typed 2xx decode failure, held against the
// five hand-rolled 2xx decodes in this package.
//
// None of these leaves decodes through transport.Decode: each one calls
// json.Unmarshal on resp.Body itself. REQ-151 § Scope is explicit that this
// makes no difference to the caller — "hand-rolled response decodes on the same
// stack MUST satisfy it identically, so which implementation route a leaf took
// stays invisible" — and that the Definition upload and PUT leaves are in scope
// even though their requests are writes, because their 2xx metadata responses
// are decoded exactly as a read response is.
//
// The § Keyed exclusion is pinned here too, from the other side: an empty 2xx
// body on a list leaf is a successful empty catalog, never an error, and the
// metadata leaves keep their Location / synthesised-metadata arms. Those arms
// are untouched by this requirement and the guards below say so.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// objectWhereArrayExpected is a JSON object served where a catalog array is
// expected — the non-empty undecodable list body REQ-151 keeps in scope (a
// paging envelope wrapped around the array is one real-world source of it).
const objectWhereArrayExpected = `{"templates":[{"template_id":"body_weight.v1"}]}`

// arrayWhereObjectExpected is a JSON array served where a single metadata
// object is expected — the metadata-leaf counterpart. Both shapes are chosen
// because they are guaranteed to fail: an object with merely unexpected keys
// decodes cleanly, since unknown fields are ignored and nothing validates that
// a required field arrived.
const arrayWhereObjectExpected = `[{"template_id":"body_weight.v1"}]`

// newBodyClient serves status 200 with body on every request and returns a
// client aimed at it. No Location header is set, so the stored-query PUT leaves
// fall through to their body decode rather than short-circuiting on Location.
func newBodyClient(t *testing.T, body string) *transport.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return newClient(t, srv)
}

// pinAbsentValue records what every leaf owes its caller beside the typed
// error: nothing decoded, and the response headers still delivered
// (REQ-151 § Metadata still arrives).
func pinAbsentValue(t *testing.T, decoded bool, meta *transport.Metadata) {
	t.Helper()
	if decoded {
		t.Error("leaf returned a decoded value beside the decode error; a failed decode has nothing to hand back")
	}
	if meta == nil {
		t.Error("leaf returned a nil *transport.Metadata beside the decode error; REQ-151 § Metadata still arrives requires the response headers survive")
	}
}

// TestDefinitionDecodeFailuresAreTyped is the positive contract of REQ-151 at
// each of this package's five hand-rolled decode sites: a 2xx body that cannot
// be decoded as the requested representation surfaces as a
// *transport.DecodeError, recoverable with errors.AsType through the leaf's own
// operation-name wrap, carrying the bytes the server delivered and attributing
// the request by method and route template.
//
// PutStoredQueryVersion shares its decode statement with PutStoredQuery — one
// unexported putStoredQuery helper serves both — so testing either exercises
// the site. Both are listed anyway: they reach that statement with different
// route templates, which is the cheapest way to pin that Route comes from the
// request the leaf actually built rather than from a constant frozen into the
// helper.
func TestDefinitionDecodeFailuresAreTyped(t *testing.T) { // REQ-151
	const opt = "<template/>"
	const aql = "SELECT c FROM EHR e CONTAINS COMPOSITION c"

	cases := []struct {
		name   string // also the operation name the leaf wraps with, after "definition."
		body   string
		method string
		route  string
		call   func(t *testing.T, c *transport.Client) error
	}{
		{
			name:   "UploadTemplate",
			body:   arrayWhereObjectExpected,
			method: http.MethodPost,
			route:  "/definition/template/{format}",
			call: func(t *testing.T, c *transport.Client) error {
				out, meta, err := definition.UploadTemplate(t.Context(), c, definition.FormatADL14, strings.NewReader(opt))
				pinAbsentValue(t, out != nil, meta)
				return err
			},
		},
		{
			name:   "ListTemplates",
			body:   objectWhereArrayExpected,
			method: http.MethodGet,
			route:  "/definition/template/{format}",
			call: func(t *testing.T, c *transport.Client) error {
				list, meta, err := definition.ListTemplates(t.Context(), c, definition.FormatADL14)
				pinAbsentValue(t, list != nil, meta)
				return err
			},
		},
		{
			name:   "PutStoredQuery",
			body:   arrayWhereObjectExpected,
			method: http.MethodPut,
			route:  "/definition/query/{qualified_query_name}",
			call: func(t *testing.T, c *transport.Client) error {
				out, meta, err := definition.PutStoredQuery(t.Context(), c, "org.openehr::vitals", aql)
				pinAbsentValue(t, out != nil, meta)
				return err
			},
		},
		{
			name:   "PutStoredQueryVersion",
			body:   arrayWhereObjectExpected,
			method: http.MethodPut,
			route:  "/definition/query/{qualified_query_name}/{version}",
			call: func(t *testing.T, c *transport.Client) error {
				out, meta, err := definition.PutStoredQueryVersion(t.Context(), c, "org.openehr::vitals", "1.0.0", aql)
				pinAbsentValue(t, out != nil, meta)
				return err
			},
		},
		{
			name:   "GetStoredQuery",
			body:   arrayWhereObjectExpected,
			method: http.MethodGet,
			route:  "/definition/query/{qualified_query_name}/{version}",
			call: func(t *testing.T, c *transport.Client) error {
				out, meta, err := definition.GetStoredQuery(t.Context(), c, "org.openehr::vitals", "1.0.0")
				pinAbsentValue(t, out != nil, meta)
				return err
			},
		},
		{
			name:   "ListStoredQueries",
			body:   objectWhereArrayExpected,
			method: http.MethodGet,
			route:  "/definition/query/{qualified_query_name}",
			call: func(t *testing.T, c *transport.Client) error {
				list, meta, err := definition.ListStoredQueries(t.Context(), c, "org.openehr")
				pinAbsentValue(t, list != nil, meta)
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(t, newBodyClient(t, tc.body))
			if err == nil {
				t.Fatalf("%s decoded %s without error; the premise of this test is gone", tc.name, tc.body)
			}

			de, ok := errors.AsType[*transport.DecodeError](err)
			if !ok {
				t.Fatalf("errors.AsType[*transport.DecodeError] did not match %T (%v); REQ-151 requires a 2xx decode failure be recoverable as that type, hand-rolled decode or not", err, err)
			}

			if got := string(de.Body); got != tc.body {
				t.Errorf("DecodeError.Body = %q, want the bytes the server delivered, %q", got, tc.body)
			}
			if de.Method != tc.method {
				t.Errorf("DecodeError.Method = %q, want %q — the method the leaf put on its own request", de.Method, tc.method)
			}
			if de.Route != tc.route {
				t.Errorf("DecodeError.Route = %q, want the route template %q, not the expanded path", de.Route, tc.route)
			}
			if de.Unwrap() == nil {
				t.Error("DecodeError.Unwrap() = nil, want the decoder's error — REQ-151 requires the codec diagnostics stay reachable")
			}

			// The leaf's operation name still leads the message: the wrap is
			// kept, and the typed error travels inside it.
			if op := "definition." + tc.name + ":"; !strings.HasPrefix(err.Error(), op) {
				t.Errorf("error message %q does not start with the operation name %q; the leaf's wrap must survive the conversion", err.Error(), op)
			}
		})
	}
}

// TestListTemplatesDecodeErrorSurvivesOperationWrap states plainly what the
// table above assumes: the returned error is *not* the DecodeError itself — the
// leaf wraps it with its operation name — and errors.AsType reaches through
// that wrap to the typed value and its Body anyway. REQ-151 § The typed error
// puts it as "a leaf's wrap is presentation, never a barrier".
func TestListTemplatesDecodeErrorSurvivesOperationWrap(t *testing.T) { // REQ-151
	c := newBodyClient(t, objectWhereArrayExpected)

	_, _, err := definition.ListTemplates(t.Context(), c, definition.FormatADL14)
	if err == nil {
		t.Fatal("ListTemplates decoded a JSON object into a slice without error; the premise of this test is gone")
	}

	// There really is a wrap in the way — otherwise this test proves nothing.
	// A deliberately shallow type assertion, not errors.As: the point of this
	// line is that the *unwrapped* error is not the DecodeError.
	if _, direct := err.(*transport.DecodeError); direct { //nolint:errorlint // shallow by design — see above
		t.Fatal("ListTemplates returned the *transport.DecodeError unwrapped; this guard exists to prove AsType reaches through the operation-name wrap, so the wrap must be there")
	}
	if want := "definition.ListTemplates:"; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error message %q does not start with %q", err.Error(), want)
	}

	de, ok := errors.AsType[*transport.DecodeError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.DecodeError] did not reach through the wrap on %T (%v)", err, err)
	}
	if got := string(de.Body); got != objectWhereArrayExpected {
		t.Errorf("DecodeError.Body recovered through the wrap = %q, want %q", got, objectWhereArrayExpected)
	}
}

// TestListStoredQueriesDecodesCatalog is the success arm of the leaf converted
// below — the one Definition function that had no test at all — and pins both
// the request it issues and the catalog it decodes, so the REQ-151 conversion
// is visibly confined to the failure path.
func TestListStoredQueriesDecodesCatalog(t *testing.T) { // REQ-151
	const catalog = `[
		{"name":"org.openehr::vitals","type":"AQL","version":"1.0.0","q":"SELECT c FROM EHR e CONTAINS COMPOSITION c"},
		{"name":"org.openehr::weight","type":"AQL","version":"2.1.0","q":"SELECT o FROM EHR e CONTAINS OBSERVATION o"}
	]`
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(catalog))
	}))
	defer srv.Close()

	list, meta, err := definition.ListStoredQueries(t.Context(), newClient(t, srv), "")
	if err != nil {
		t.Fatal(err)
	}
	if meta == nil {
		t.Fatal("ListStoredQueries returned nil metadata on success")
	}
	if captured.Method != http.MethodGet {
		t.Errorf("method = %q, want %q", captured.Method, http.MethodGet)
	}
	if want := "/openehr/v1/definition/query"; captured.URL.Path != want {
		t.Errorf("path = %q, want %q — an empty pattern lists every stored query", captured.URL.Path, want)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].Name != "org.openehr::vitals" || list[0].Version != "1.0.0" {
		t.Errorf("list[0] = {Name:%q Version:%q}, want {org.openehr::vitals 1.0.0}", list[0].Name, list[0].Version)
	}
	if list[1].Name != "org.openehr::weight" || list[1].Version != "2.1.0" {
		t.Errorf("list[1] = {Name:%q Version:%q}, want {org.openehr::weight 2.1.0}", list[1].Name, list[1].Version)
	}
	if want := "SELECT o FROM EHR e CONTAINS OBSERVATION o"; list[1].Q != want {
		t.Errorf("list[1].Q = %q, want %q", list[1].Q, want)
	}
}

// TestListStoredQueriesEmptyBodyIsEmptyCatalog holds REQ-151's keyed exclusion
// on the leaf that had no test to hold it: an empty 2xx body on a Definition
// list operation is a successful empty catalog. It must produce neither
// transport.ErrInvalidShape nor a *transport.DecodeError. Only a non-empty body
// that fails to decode is REQ-151's, which is why the arm below is untouched by
// the conversion.
func TestListStoredQueriesEmptyBodyIsEmptyCatalog(t *testing.T) { // REQ-151
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	list, meta, err := definition.ListStoredQueries(t.Context(), newClient(t, srv), "")
	if err != nil {
		t.Fatalf("ListStoredQueries on an empty 204 body: unexpected error %v; an empty list body is success", err)
	}
	if _, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Error("an empty list body produced a *transport.DecodeError; REQ-151's keyed exclusion forbids it")
	}
	if errors.Is(err, transport.ErrInvalidShape) {
		t.Error("an empty list body produced transport.ErrInvalidShape; REQ-151's keyed exclusion forbids it")
	}
	if len(list) != 0 {
		t.Errorf("len(list) = %d, want 0 on an empty body", len(list))
	}
	if meta == nil {
		t.Error("ListStoredQueries returned nil metadata on an empty body")
	}
}

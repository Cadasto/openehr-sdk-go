package query_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// storedWireRequest is what the test server saw of one stored-query call.
type storedWireRequest struct {
	method string
	path   string
	query  url.Values
	body   []byte
}

// TestREQ057_StoredPagingOnlyWhenSet pins § REQ-057 *Paging* (ADR 0026):
// stored-query execution sends offset and fetch only when the caller sets
// them, sends an explicit 0, and sends offset without fetch as asked rather
// than refusing it. The POST body always carries query_parameters, as an
// empty object when the caller passes none and with the caller's parameters
// otherwise; a GET carries each parameter as a form query parameter.
//
// Each row is checked on the captured wire request by key presence, so a
// field that is sent with a default value fails as surely as a wrong value,
// and a parameter the request drops fails as surely as a wrong one.
func TestREQ057_StoredPagingOnlyWhenSet(t *testing.T) {
	fixture := readFixture(t, "result_set.json")

	entries := []struct {
		name     string
		wantPath string
		run      func(ctx context.Context, c *transport.Client, params map[string]any, opts ...query.ExecuteOption) error
	}{
		{
			name:     "RunStored",
			wantPath: "/openehr/v1/query/org.example::q",
			run: func(ctx context.Context, c *transport.Client, params map[string]any, opts ...query.ExecuteOption) error {
				_, _, err := query.RunStored(ctx, c, "org.example::q", params, opts...)
				return err
			},
		},
		{
			name:     "RunStoredVersion",
			wantPath: "/openehr/v1/query/org.example::q/1.0.0",
			run: func(ctx context.Context, c *transport.Client, params map[string]any, opts ...query.ExecuteOption) error {
				_, _, err := query.RunStoredVersion(ctx, c, "org.example::q", "1.0.0", params, opts...)
				return err
			},
		},
	}
	verbs := []struct {
		method string
		opts   []query.ExecuteOption
	}{
		{method: http.MethodPost},
		{method: http.MethodGet, opts: []query.ExecuteOption{query.WithGET()}},
	}
	paging := []struct {
		name string
		opts []query.ExecuteOption
		// want holds the paging fields that must reach the wire, with their
		// value; a paging field missing from it must be absent.
		want map[string]string
	}{
		{name: "no_options", want: map[string]string{}},
		{
			name: "offset_only",
			opts: []query.ExecuteOption{query.WithOffset(5)},
			want: map[string]string{"offset": "5"},
		},
		{
			name: "fetch_only",
			opts: []query.ExecuteOption{query.WithFetch(10)},
			want: map[string]string{"fetch": "10"},
		},
		{
			name: "both",
			opts: []query.ExecuteOption{query.WithOffset(5), query.WithFetch(10)},
			want: map[string]string{"offset": "5", "fetch": "10"},
		},
		{
			name: "explicit_zero",
			opts: []query.ExecuteOption{query.WithOffset(0), query.WithFetch(0)},
			want: map[string]string{"offset": "0", "fetch": "0"},
		},
	}
	parameters := []struct {
		name string
		in   map[string]any
		// wantPOST is the query_parameters object the POST body must carry,
		// decoded with UseNumber, so a number is a json.Number holding its
		// JSON text and a quoted number would be a string. wantGET is the
		// form value each parameter must carry on a GET.
		wantPOST map[string]any
		wantGET  map[string]string
	}{
		{name: "nil_params", wantPOST: map[string]any{}, wantGET: map[string]string{}},
		{
			name: "params",
			in:   map[string]any{"systolic_min": 120, "big": 1234567.0, "status": "active", "flag": true},
			wantPOST: map[string]any{
				"systolic_min": json.Number("120"), "big": json.Number("1234567"), "status": "active", "flag": true,
			},
			wantGET: map[string]string{"systolic_min": "120", "big": "1234567", "status": "active", "flag": "true"},
		},
	}

	for _, e := range entries {
		for _, v := range verbs {
			for _, p := range paging {
				for _, prm := range parameters {
					t.Run(e.name+"/"+v.method+"/"+p.name+"/"+prm.name, func(t *testing.T) {
						seen := make(chan storedWireRequest, 1)
						srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							b, _ := io.ReadAll(r.Body)
							select {
							case seen <- storedWireRequest{method: r.Method, path: r.URL.Path, query: r.URL.Query(), body: b}:
							default:
								t.Errorf("server received a second request %s %s, want one", r.Method, r.URL.Path)
							}
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write(fixture)
						}))
						defer srv.Close()

						opts := append(append([]query.ExecuteOption{}, v.opts...), p.opts...)
						if err := e.run(t.Context(), newClient(t, srv), prm.in, opts...); err != nil {
							t.Fatalf("%s(%s, %s, %s) = %v, want the request sent as set", e.name, v.method, p.name, prm.name, err)
						}
						var req storedWireRequest
						select {
						case req = <-seen:
						default:
							t.Fatalf("%s(%s, %s, %s) sent no request, want it sent as set", e.name, v.method, p.name, prm.name)
						}
						if req.method != v.method || req.path != e.wantPath {
							t.Errorf("request = %s %s, want %s %s", req.method, req.path, v.method, e.wantPath)
						}

						var sent map[string]string
						if v.method == http.MethodPost {
							sent = postPagingFields(t, req.body, prm.wantPOST)
						} else {
							sent = getPagingFields(t, req.query)
							if got := getParameterFields(t, req.query); !maps.Equal(got, prm.wantGET) {
								t.Errorf("GET %s with %s sent parameters %v, want exactly %v", e.wantPath, prm.name, got, prm.wantGET)
							}
						}
						if !maps.Equal(sent, p.want) {
							t.Errorf("%s %s with %s sent paging fields %v, want exactly %v", v.method, e.wantPath, p.name, sent, p.want)
						}
					})
				}
			}
		}
	}
}

// postPagingFields decodes a stored-query POST body, checks that it carries
// query_parameters as an object holding exactly wantParams (an empty object
// when wantParams is empty), and returns the offset and fetch members it
// holds, each as its JSON number text.
func postPagingFields(t *testing.T, body []byte, wantParams map[string]any) map[string]string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var members map[string]any
	if err := dec.Decode(&members); err != nil {
		t.Fatalf("decode POST body %q: %v", body, err)
	}
	qp, ok := members["query_parameters"].(map[string]any)
	switch {
	case !ok:
		t.Errorf("POST body %s: query_parameters = %#v, want an object", body, members["query_parameters"])
	case !reflect.DeepEqual(qp, wantParams):
		t.Errorf("POST body %s: query_parameters = %s, want %s", body, typedMembers(qp), typedMembers(wantParams))
	}
	sent := map[string]string{}
	for _, k := range []string{"offset", "fetch"} {
		val, ok := members[k]
		if !ok {
			continue
		}
		n, isNumber := val.(json.Number)
		if !isNumber {
			t.Errorf("POST body %s: %s = %#v, want a JSON number", body, k, val)
			continue
		}
		sent[k] = n.String()
	}
	return sent
}

// typedMembers renders each member of m with its Go type, sorted by name,
// so an unquoted JSON number (json.Number) and a quoted one (string) read
// differently in a failure message.
func typedMembers(m map[string]any) string {
	parts := make([]string, 0, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		parts = append(parts, fmt.Sprintf("%s: %T(%v)", k, m[k], m[k]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// getPagingFields returns the offset and fetch query keys of a stored-query
// GET, each with its value.
func getPagingFields(t *testing.T, q url.Values) map[string]string {
	t.Helper()
	sent := map[string]string{}
	for _, k := range []string{"offset", "fetch"} {
		vals, ok := q[k]
		if !ok {
			continue
		}
		if len(vals) != 1 {
			t.Errorf("query key %q sent %d times (%v), want once", k, len(vals), vals)
		}
		sent[k] = vals[0]
	}
	return sent
}

// getParameterFields returns every query key of a stored-query GET other
// than offset and fetch, each with its value: the named AQL parameters the
// request carries as form query parameters.
func getParameterFields(t *testing.T, q url.Values) map[string]string {
	t.Helper()
	sent := map[string]string{}
	for k, vals := range q {
		if k == "offset" || k == "fetch" {
			continue
		}
		if len(vals) != 1 {
			t.Errorf("query key %q sent %d times (%v), want once", k, len(vals), vals)
		}
		sent[k] = vals[0]
	}
	return sent
}

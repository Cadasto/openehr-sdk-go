package definition

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// StoredQueryMetadata is the Definition API stored-query descriptor
// (REQ-057).
type StoredQueryMetadata struct {
	Name    string                     `json:"name"`
	Type    string                     `json:"type"`
	Version string                     `json:"version"`
	Saved   time.Time                  `json:"saved,omitzero"`
	Q       string                     `json:"q"`
	Extras  map[string]json.RawMessage `json:"-"`
}

var knownStoredQueryFields = map[string]struct{}{
	"name": {}, "type": {}, "version": {}, "saved": {}, "q": {},
}

// UnmarshalJSON decodes documented fields and preserves Extras.
func (m *StoredQueryMetadata) UnmarshalJSON(data []byte) error {
	type alias StoredQueryMetadata
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*m = StoredQueryMetadata(a)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		if _, ok := knownStoredQueryFields[k]; ok {
			continue
		}
		if m.Extras == nil {
			m.Extras = map[string]json.RawMessage{}
		}
		m.Extras[k] = v
	}
	return nil
}

// storeConfig holds resolved options for storing a query.
type storeConfig struct {
	queryType string
}

// StoreOption mutates stored-query upload requests.
type StoreOption func(*storeConfig)

// QueryTypeAQL is the standard `query_type` value and the SDK default.
// The Definition API's QueryType is an open string (the spec defines no
// closed enum, only the default "AQL"), so [WithQueryType] does not
// restrict the value — a deployment supporting another formalism can pass
// its own.
const QueryTypeAQL = "AQL"

// WithQueryType sets the `query_type` query parameter. The default is
// [QueryTypeAQL] (case-sensitive on strict deployments).
func WithQueryType(t string) StoreOption {
	return func(c *storeConfig) { c.queryType = t }
}

// PutStoredQuery registers or updates a stored AQL query at the
// unversioned resource (the deployment assigns the next version).
//
// Wire: PUT /definition/query/{qualified_query_name} with
// Content-Type text/plain body (REQ-057).
func PutStoredQuery(ctx context.Context, c *transport.Client, qualifiedName, aqlText string, opts ...StoreOption) (*StoredQueryMetadata, *transport.Metadata, error) {
	name := strings.TrimSpace(qualifiedName)
	if name == "" {
		return nil, nil, fmt.Errorf("definition.PutStoredQuery: %w: empty qualified query name", transport.ErrInvalidConfig)
	}
	return putStoredQuery(ctx, c,
		"/definition/query/"+name,
		"/definition/query/{qualified_query_name}",
		"definition.PutStoredQuery", name, "", aqlText, opts...)
}

// PutStoredQueryVersion registers or updates a stored AQL query at an
// explicit version.
//
// Wire: PUT /definition/query/{qualified_query_name}/{version} with
// Content-Type text/plain body (REQ-057). A 409 (the version already
// exists with different content) surfaces as transport.ErrVersionConflict.
func PutStoredQueryVersion(ctx context.Context, c *transport.Client, qualifiedName, version, aqlText string, opts ...StoreOption) (*StoredQueryMetadata, *transport.Metadata, error) {
	name := strings.TrimSpace(qualifiedName)
	ver := strings.TrimSpace(version)
	if name == "" || ver == "" {
		return nil, nil, fmt.Errorf("definition.PutStoredQueryVersion: %w: name and version are required", transport.ErrInvalidConfig)
	}
	return putStoredQuery(ctx, c,
		"/definition/query/"+name+"/"+ver,
		"/definition/query/{qualified_query_name}/{version}",
		"definition.PutStoredQueryVersion", name, ver, aqlText, opts...)
}

// putStoredQuery is the shared PUT implementation for the versioned and
// unversioned stored-query endpoints.
//
// REQ-057 finding B: the canonical OAS `200_StoredQuery_stored` response
// defines a `Location` header and no body — the server-assigned version is
// conveyed via `Location: …/definition/query/{name}/{version}`. EHRbase
// returns the same `Location`-only shape when the request is
// `Content-Type: text/plain` (which the SDK always sends). The decode
// order is therefore: (1) Location header (canonical), (2) JSON body
// (lenient — some deployments return one), (3) synthesised metadata with
// the caller's input version (graceful fallback for a deficient server).
func putStoredQuery(ctx context.Context, c *transport.Client, path, route, op, name, version, aqlText string, opts ...StoreOption) (*StoredQueryMetadata, *transport.Metadata, error) {
	aqlText = strings.TrimSpace(aqlText)
	if aqlText == "" {
		return nil, nil, fmt.Errorf("%s: %w: empty AQL body", op, transport.ErrInvalidConfig)
	}
	cfg := storeConfig{queryType: QueryTypeAQL}
	for _, o := range opts {
		o(&cfg)
	}
	q := url.Values{}
	if cfg.queryType != "" {
		q.Set("query_type", cfg.queryType)
	}
	req := &transport.Request{
		Method:      http.MethodPut,
		Path:        path,
		Route:       route,
		Query:       q,
		Body:        []byte(aqlText),
		ContentType: "text/plain",
		Accept:      "application/json",
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, resp.Metadata, err
		}
		return nil, nil, err
	}
	if loc := resp.Metadata.Location; loc != "" {
		if locName, locVer, ok := parseStoredQueryLocation(loc); ok {
			return &StoredQueryMetadata{Name: locName, Version: locVer, Q: aqlText}, resp.Metadata, nil
		}
	}
	if len(resp.Body) == 0 {
		return &StoredQueryMetadata{Name: name, Version: version, Q: aqlText}, resp.Metadata, nil
	}
	var out StoredQueryMetadata
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, resp.Metadata, fmt.Errorf("%s: decode: %w", op, err)
	}
	return &out, resp.Metadata, nil
}

// parseStoredQueryLocation recovers the assigned {name, version} from a
// `Location: …/definition/query/{name}/{version}` response header
// (REQ-057 finding B). Returns ok=false on a malformed value so the
// caller can fall through to body / synthesised metadata; no error is
// surfaced for a malformed Location — a deficient server should not break
// the call.
func parseStoredQueryLocation(loc string) (name, version string, ok bool) {
	// Tolerate absolute and relative forms. Strip scheme+host if present,
	// then take the last two non-empty path segments — `{name}` and
	// `{version}` — decoding each below (the server MAY percent-encode the
	// Location; the client itself now sends the raw id — REQ-095).
	p := loc
	if u, err := url.Parse(loc); err == nil && u.Path != "" {
		p = u.Path
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	clean := make([]string, 0, len(parts))
	for _, seg := range parts {
		if seg != "" {
			clean = append(clean, seg)
		}
	}
	// Anchor on the canonical `…/definition/query/{name}/{version}` shape:
	// the (last) "query" segment must be followed by exactly two segments,
	// `{name}` then `{version}`. Without this anchor a version-less Location
	// (`…/definition/query/{name}`) mis-parses "query"/{name} as
	// {name}/{version} and returns confidently-wrong values; that case must
	// fall through to body / synthesised metadata instead.
	qi := -1
	for i, seg := range clean {
		if seg == "query" {
			qi = i
		}
	}
	if qi < 0 || qi+2 != len(clean)-1 {
		return "", "", false
	}
	// Decode each segment defensively: a server MAY percent-encode the
	// Location even though the client sends the raw id (REQ-095); PathUnescape
	// is a no-op for an already-decoded segment.
	n, err := url.PathUnescape(clean[qi+1])
	if err != nil {
		n = clean[qi+1]
	}
	v, err := url.PathUnescape(clean[qi+2])
	if err != nil {
		v = clean[qi+2]
	}
	if n == "" || v == "" {
		return "", "", false
	}
	return n, v, true
}

// GetStoredQuery retrieves a stored query at a specific version.
//
// Wire: GET /definition/query/{qualified_query_name}/{version}.
func GetStoredQuery(ctx context.Context, c *transport.Client, qualifiedName, version string) (*StoredQueryMetadata, *transport.Metadata, error) {
	name := strings.TrimSpace(qualifiedName)
	ver := strings.TrimSpace(version)
	if name == "" || ver == "" {
		return nil, nil, fmt.Errorf("definition.GetStoredQuery: %w: name and version are required", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/definition/query/" + name + "/" + ver,
		Route:  "/definition/query/{qualified_query_name}/{version}",
		Accept: "application/json",
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, resp.Metadata, err
		}
		return nil, nil, err
	}
	if len(resp.Body) == 0 {
		return &StoredQueryMetadata{Name: name, Version: ver}, resp.Metadata, nil
	}
	var out StoredQueryMetadata
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, resp.Metadata, fmt.Errorf("definition.GetStoredQuery: decode: %w", err)
	}
	return &out, resp.Metadata, nil
}

// ListStoredQueries lists stored queries matching qualifiedName as a
// prefix pattern. An empty pattern lists all queries on the deployment.
//
// Wire: GET /definition/query/{qualified_query_name}.
func ListStoredQueries(ctx context.Context, c *transport.Client, namePattern string) ([]StoredQueryMetadata, *transport.Metadata, error) {
	path := "/definition/query"
	route := "/definition/query"
	if strings.TrimSpace(namePattern) != "" {
		path += "/" + strings.TrimSpace(namePattern)
		route += "/{qualified_query_name}"
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   path,
		Route:  route,
		Accept: "application/json",
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, resp.Metadata, err
		}
		return nil, nil, err
	}
	if len(resp.Body) == 0 {
		return nil, resp.Metadata, nil
	}
	var out []StoredQueryMetadata
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, resp.Metadata, fmt.Errorf("definition.ListStoredQueries: decode: %w", err)
	}
	return out, resp.Metadata, nil
}

// DeleteStoredQuery removes a stored query version when the
// deployment supports DELETE on the Definition query resource.
//
// Wire: DELETE /definition/query/{qualified_query_name}/{version}.
func DeleteStoredQuery(ctx context.Context, c *transport.Client, qualifiedName, version string) (*transport.Metadata, error) {
	name := strings.TrimSpace(qualifiedName)
	if name == "" || strings.TrimSpace(version) == "" {
		return nil, fmt.Errorf("definition.DeleteStoredQuery: %w: name and version are required", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodDelete,
		Path:   "/definition/query/" + name + "/" + version,
		Route:  "/definition/query/{qualified_query_name}/{version}",
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return resp.Metadata, err
		}
		return nil, err
	}
	return resp.Metadata, nil
}

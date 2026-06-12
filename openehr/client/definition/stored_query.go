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

// WithQueryType sets the `query_type` query parameter (default "aql").
func WithQueryType(t string) StoreOption {
	return func(c *storeConfig) { c.queryType = t }
}

// PutStoredQuery registers or updates a stored AQL query.
//
// Wire: PUT /definition/query/{qualified_query_name} with
// Content-Type text/plain body (REQ-057).
func PutStoredQuery(ctx context.Context, c *transport.Client, qualifiedName, aqlText string, opts ...StoreOption) (*StoredQueryMetadata, *transport.Metadata, error) {
	name := strings.TrimSpace(qualifiedName)
	if name == "" {
		return nil, nil, fmt.Errorf("definition.PutStoredQuery: %w: empty qualified query name", transport.ErrInvalidConfig)
	}
	aqlText = strings.TrimSpace(aqlText)
	if aqlText == "" {
		return nil, nil, fmt.Errorf("definition.PutStoredQuery: %w: empty AQL body", transport.ErrInvalidConfig)
	}
	cfg := storeConfig{queryType: "aql"}
	for _, o := range opts {
		o(&cfg)
	}
	q := url.Values{}
	if cfg.queryType != "" {
		q.Set("query_type", cfg.queryType)
	}
	req := &transport.Request{
		Method:      http.MethodPut,
		Path:        "/definition/query/" + url.PathEscape(name),
		Route:       "/definition/query/{qualified_query_name}",
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
	if len(resp.Body) == 0 {
		return &StoredQueryMetadata{Name: name, Q: aqlText}, resp.Metadata, nil
	}
	var out StoredQueryMetadata
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, resp.Metadata, fmt.Errorf("definition.PutStoredQuery: decode: %w", err)
	}
	return &out, resp.Metadata, nil
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
		Path:   "/definition/query/" + url.PathEscape(name) + "/" + url.PathEscape(ver),
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
		path += "/" + url.PathEscape(strings.TrimSpace(namePattern))
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
		Path:   "/definition/query/" + url.PathEscape(name) + "/" + url.PathEscape(version),
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

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Execute runs an ad-hoc AQL query via POST /query/aql (REQ-055).
func Execute(ctx context.Context, c *transport.Client, q aql.Query, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error) {
	if err := q.Validate(); err != nil {
		return nil, nil, fmt.Errorf("query.Execute: %w: %v", ErrInvalidConfig, err)
	}
	cfg := executeConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.ehrID == "" {
		cfg.ehrID = q.EHRID
	}
	body := adhocBody(q, cfg)
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("query.Execute: encode body: %w", err)
	}
	req := &transport.Request{
		Method:      http.MethodPost,
		Path:        "/query/aql",
		Route:       "/query/aql",
		Body:        raw,
		ContentType: "application/json",
		Accept:      "application/json",
	}
	return doResultSet(ctx, c, req)
}

// ExecuteString is an escape hatch for raw AQL strings and parameters.
func ExecuteString(ctx context.Context, c *transport.Client, aqlText string, params map[string]any, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error) {
	q := aql.NewQuery(aqlText)
	q.Parameters = params
	return Execute(ctx, c, q, opts...)
}

// RunStored executes a stored query at the latest version via POST
// /query/{qualified_query_name} (REQ-057).
func RunStored(ctx context.Context, c *transport.Client, qualifiedName string, params map[string]any, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error) {
	return runStoredAtVersion(ctx, c, qualifiedName, "", params, opts...)
}

// RunStoredVersion executes a stored query at an explicit version.
func RunStoredVersion(ctx context.Context, c *transport.Client, qualifiedName, version string, params map[string]any, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error) {
	return runStoredAtVersion(ctx, c, qualifiedName, version, params, opts...)
}

func runStoredAtVersion(ctx context.Context, c *transport.Client, qualifiedName, version string, params map[string]any, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error) {
	name := strings.TrimSpace(qualifiedName)
	if name == "" {
		return nil, nil, fmt.Errorf("query.RunStored: %w: empty qualified query name", ErrInvalidConfig)
	}
	cfg := executeConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	body := storedBody(params, cfg)
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("query.RunStored: encode body: %w", err)
	}
	path := "/query/" + url.PathEscape(name)
	route := "/query/{qualified_query_name}"
	if version != "" {
		path += "/" + url.PathEscape(version)
		route += "/{version}"
	}
	req := &transport.Request{
		Method:      http.MethodPost,
		Path:        path,
		Route:       route,
		Body:        raw,
		ContentType: "application/json",
		Accept:      "application/json",
	}
	return doResultSet(ctx, c, req)
}

func adhocBody(q aql.Query, cfg executeConfig) map[string]any {
	body := map[string]any{"q": q.String()}
	if cfg.offset != 0 || q.Offset != 0 {
		if cfg.offset != 0 {
			body["offset"] = cfg.offset
		} else {
			body["offset"] = q.Offset
		}
	}
	if cfg.fetch != 0 || q.Fetch != 0 {
		if cfg.fetch != 0 {
			body["fetch"] = cfg.fetch
		} else {
			body["fetch"] = q.Fetch
		}
	}
	params := q.Parameters
	if params == nil {
		params = map[string]any{}
	}
	if len(params) > 0 {
		body["query_parameters"] = params
	}
	return body
}

func storedBody(params map[string]any, cfg executeConfig) map[string]any {
	body := map[string]any{}
	if cfg.offset != 0 {
		body["offset"] = cfg.offset
	}
	if cfg.fetch != 0 {
		body["fetch"] = cfg.fetch
	}
	if params == nil {
		params = map[string]any{}
	}
	body["query_parameters"] = params
	return body
}

func doResultSet(ctx context.Context, c *transport.Client, req *transport.Request) (*aql.ResultSet, *transport.Metadata, error) {
	out, meta, err := transport.Decode[aql.ResultSet](ctx, c, req)
	if err != nil {
		if meta != nil {
			return nil, meta, mapQueryError(err)
		}
		return nil, nil, mapQueryError(err)
	}
	return out, meta, nil
}

// Repository mirrors package-level query functions (REQ-023).
type Repository interface {
	Execute(ctx context.Context, q aql.Query, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error)
	ExecuteString(ctx context.Context, aqlText string, params map[string]any, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error)
	RunStored(ctx context.Context, qualifiedName string, params map[string]any, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) Execute(ctx context.Context, q aql.Query, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error) {
	return Execute(ctx, r.c, q, opts...)
}

func (r *repository) ExecuteString(ctx context.Context, aqlText string, params map[string]any, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error) {
	return ExecuteString(ctx, r.c, aqlText, params, opts...)
}

func (r *repository) RunStored(ctx context.Context, qualifiedName string, params map[string]any, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error) {
	return RunStored(ctx, r.c, qualifiedName, params, opts...)
}

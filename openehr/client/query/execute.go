package query

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Execute runs an ad-hoc AQL query via POST /query/aql (REQ-055).
func Execute(ctx context.Context, c *transport.Client, q aql.Query, opts ...ExecuteOption) (*aql.ResultSet, *transport.Metadata, error) {
	if err := q.Validate(); err != nil {
		return nil, nil, fmt.Errorf("query.Execute: %w: %w", ErrInvalidConfig, err)
	}
	cfg := executeConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.ehrID == "" {
		cfg.ehrID = q.EHRID
	}
	req := &transport.Request{
		Path:   "/query/aql",
		Route:  "/query/aql",
		Accept: "application/json",
	}
	if cfg.useGET {
		req.Method = http.MethodGet
		req.Query = adhocQueryValues(q, cfg)
	} else {
		raw, err := json.Marshal(adhocBody(q, cfg))
		if err != nil {
			return nil, nil, fmt.Errorf("query.Execute: encode body: %w", err)
		}
		req.Method = http.MethodPost
		req.Body = raw
		req.ContentType = "application/json"
	}
	applyEHRScope(req, cfg)
	return doResultSet(ctx, c, req)
}

// ExecuteString is an escape hatch for raw AQL. aqlText MUST be a
// static or programmatically validated statement; never interpolate
// caller-supplied values into it — pass them via params, which the CDR
// binds as named placeholders. String-built AQL is injectable.
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
	path := "/query/" + url.PathEscape(name)
	route := "/query/{qualified_query_name}"
	if version != "" {
		path += "/" + url.PathEscape(version)
		route += "/{version}"
	}
	req := &transport.Request{
		Path:   path,
		Route:  route,
		Accept: "application/json",
	}
	if cfg.useGET {
		req.Method = http.MethodGet
		req.Query = storedQueryValues(params, cfg)
	} else {
		raw, err := json.Marshal(storedBody(params, cfg))
		if err != nil {
			return nil, nil, fmt.Errorf("query.RunStored: encode body: %w", err)
		}
		req.Method = http.MethodPost
		req.Body = raw
		req.ContentType = "application/json"
	}
	applyEHRScope(req, cfg)
	return doResultSet(ctx, c, req)
}

// adhocQueryValues builds the GET query string for an ad-hoc query: q plus
// the optional offset/fetch and the form/explode query_parameters.
func adhocQueryValues(q aql.Query, cfg executeConfig) url.Values {
	v := url.Values{}
	v.Set("q", q.String())
	switch {
	case cfg.offsetSet:
		v.Set("offset", strconv.Itoa(cfg.offset))
	case q.Offset != 0:
		v.Set("offset", strconv.Itoa(q.Offset))
	}
	switch {
	case cfg.fetchSet:
		v.Set("fetch", strconv.Itoa(cfg.fetch))
	case q.Fetch != 0:
		v.Set("fetch", strconv.Itoa(q.Fetch))
	}
	addQueryParamValues(v, q.Parameters)
	return v
}

// storedQueryValues builds the GET query string for a stored query: offset
// (always, default 0) plus the optional fetch and form/explode
// query_parameters.
func storedQueryValues(params map[string]any, cfg executeConfig) url.Values {
	v := url.Values{}
	v.Set("offset", strconv.Itoa(cfg.offset))
	if cfg.fetchSet {
		v.Set("fetch", strconv.Itoa(cfg.fetch))
	}
	addQueryParamValues(v, params)
	return v
}

// addQueryParamValues encodes AQL named parameters as individual query
// parameters (the spec's style=form, explode=true for query_parameters).
func addQueryParamValues(v url.Values, params map[string]any) {
	for k, val := range params {
		v.Set(k, fmt.Sprint(val))
	}
}

// applyEHRScope sets the openEHR REST `ehr_id` query parameter when the
// caller scoped execution to one EHR (REQ-055).
func applyEHRScope(req *transport.Request, cfg executeConfig) {
	if cfg.ehrID == "" {
		return
	}
	if req.Query == nil {
		req.Query = url.Values{}
	}
	req.Query.Set("ehr_id", cfg.ehrID)
}

func adhocBody(q aql.Query, cfg executeConfig) map[string]any {
	// AdhocQueryExecute requires only `q`; offset/fetch are optional, so
	// emit them only when the caller (option) or the query literal set a
	// value. An explicit WithOffset(0)/WithFetch(0) is honoured.
	body := map[string]any{"q": q.String()}
	switch {
	case cfg.offsetSet:
		body["offset"] = cfg.offset
	case q.Offset != 0:
		body["offset"] = q.Offset
	}
	switch {
	case cfg.fetchSet:
		body["fetch"] = cfg.fetch
	case q.Fetch != 0:
		body["fetch"] = q.Fetch
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
	// The stored Query schema marks offset, fetch, and query_parameters as
	// required. `offset` has a documented default of 0, so it is always
	// emitted. `fetch` has no fixed default ("depends on the
	// implementation") — emitting fetch:0 would request zero rows — so it
	// is sent only when the caller set it explicitly; otherwise the field
	// is omitted to let the server apply its default.
	body := map[string]any{"offset": cfg.offset}
	if cfg.fetchSet {
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

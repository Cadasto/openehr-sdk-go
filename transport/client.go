package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// tracerName is the OTel instrumentation scope. Stable so callers can
// configure sampling per-package.
const tracerName = "github.com/cadasto/openehr-sdk-go/transport"

// DefaultCallerAttributionHeader is the header name used when
// WithCallerAttributionHeader is not overridden. Tracks REQ-066.
const DefaultCallerAttributionHeader = "X-Cadasto-Caller-Attribution"

// Client is the HTTP client wrapper every openEHR REST leaf client
// uses to reach a Cadasto CDR or any conformant openEHR backend. It
// owns:
//
//   - the injected *http.Client (REQ-021)
//   - the resolved smart/discovery.ServiceCatalog (REQ-070)
//   - the client-default auth.TokenSource (REQ-060)
//   - the retry policy (REQ-091)
//   - the OTel hooks (REQ-090)
//
// Per-client / per-tenant binding (REQ-065): each Client instance is
// bound to one issuer / tenant context. The federator use case
// constructs many Clients, not one shared Client.
//
// Safe for concurrent use by multiple goroutines (REQ-026).
type Client struct {
	cfg     config
	catalog *discovery.ServiceCatalog
}

// New constructs a Client. The ServiceCatalog is mandatory (REQ-070).
// The HTTP client must be injected via WithHTTPClient (REQ-021).
//
// A nil catalog or a missing HTTP client returns ErrInvalidConfig
// (wrapped).
func New(catalog *discovery.ServiceCatalog, opts ...Option) (*Client, error) {
	cfg := config{
		callerAttributionHeader: DefaultCallerAttributionHeader,
		tokenSrc:                auth.AnonymousTokenSource(),
		logger:                  slog.Default(),
	}
	for _, o := range opts {
		o(&cfg)
	}
	if catalog == nil {
		return nil, fmt.Errorf("%w: ServiceCatalog is required (REQ-070)", ErrInvalidConfig)
	}
	if cfg.httpClient == nil {
		return nil, fmt.Errorf("%w: WithHTTPClient is required (REQ-021)", ErrInvalidConfig)
	}
	if cfg.tokenSrc == nil {
		cfg.tokenSrc = auth.AnonymousTokenSource()
	}
	if cfg.logger == nil {
		cfg.logger = slog.Default()
	}
	if cfg.callerAttributionHeader == "" {
		cfg.callerAttributionHeader = DefaultCallerAttributionHeader
	}
	return &Client{cfg: cfg, catalog: catalog}, nil
}

// Catalog returns the ServiceCatalog the Client was constructed with.
// Useful for leaf clients that want to project entries before issuing
// a request (e.g. to extract the SpecVersion for observability).
func (c *Client) Catalog() *discovery.ServiceCatalog { return c.catalog }

// HTTPClient returns the injected *http.Client (REQ-021). Exposed so
// SDK packages that need to issue requests outside the catalog-routed
// Do pipeline — e.g. cadasto/admin/ deployment-level health probes
// (SDK-GAP-07) — can reuse the configured HTTP transport without
// re-injection or wrapping. Returns nil only when New rejected the
// configuration (which is impossible for a constructed Client).
func (c *Client) HTTPClient() *http.Client { return c.cfg.httpClient }

// Do executes req and returns the captured Response. Status codes in
// 2xx surface as a non-nil Response with err=nil; non-2xx surface as
// err of type *WireError plus a possibly-non-nil Response carrying the
// raw body. Network errors surface as the wrapped underlying error.
//
// The lifecycle:
//
//  1. Resolve the service base URL from the catalog by ServiceID.
//  2. Build the http.Request, plumb headers, attach the bearer token.
//  3. Emit an OTel span and propagate traceparent.
//  4. Execute via the injected *http.Client.
//  5. Retry per RetryPolicy on retriable statuses.
//  6. Parse the response body into Body + Metadata; map the wire
//     error envelope onto the typed-sentinel hierarchy.
func (c *Client) Do(ctx context.Context, req *Request) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	svc, ok := c.catalog.Service(req.effectiveServiceID())
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrServiceUnavailable, req.effectiveServiceID())
	}
	target, err := joinTarget(svc.BaseURL, req.Path, req.Query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}

	tracer := otel.GetTracerProvider().Tracer(tracerName)
	spanName := req.effectiveMethod() + " " + req.effectiveRoute()
	ctx, span := tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.method", req.effectiveMethod()),
			attribute.String("http.route", req.effectiveRoute()),
			attribute.String("http.url", sanitisedURL(target)),
			attribute.String("openehr.spec_version", svc.SpecVersion),
		),
	)
	defer span.End()

	// Skip the wall-clock capture when no observer is registered —
	// the cost is only sub-µs (time.Now is a vDSO call) but observerless
	// callers are the hot path the benchmark consumer of REQ-098 cares
	// about. Gate at the call site so emitObservation has no nil-check.
	var start time.Time
	if c.cfg.observer != nil {
		start = time.Now()
	}
	var (
		resp     *Response
		lastErr  error
		attempt  int
		reauthed bool
	)
	for {
		attempt++
		span.SetAttributes(attribute.Int("retry.attempt", attempt-1))
		resp, lastErr = c.doOnce(ctx, req, target)
		// One-shot 401-driven re-auth (REQ-063): a wire 401 on an
		// authenticated request can mean a stale cached token the source
		// could not self-detect (e.g. minted without expires_in). Invalidate
		// the token and retry once with a freshly acquired one, outside the
		// retry budget so a disabled retry policy still recovers.
		if !reauthed && c.reauthAfter401(ctx, req, resp) {
			reauthed = true
			span.AddEvent("auth.reauth_after_401")
			continue
		}
		if !c.shouldRetry(req, resp, lastErr, attempt) {
			break
		}
		wait := c.cfg.retry.backoff(attempt)
		span.SetAttributes(attribute.Int64("retry.backoff_ms", wait.Milliseconds()))
		if err := retryWait(ctx, wait); err != nil {
			lastErr = err
			break
		}
	}
	if lastErr != nil {
		span.SetStatus(codes.Error, lastErr.Error())
		span.RecordError(lastErr)
		if c.cfg.observer != nil {
			c.emitObservation(ctx, req, target, resp, lastErr, attempt, time.Since(start))
		}
		return resp, lastErr
	}
	if resp != nil {
		span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
		if resp.StatusCode >= 400 {
			span.SetStatus(codes.Error, http.StatusText(resp.StatusCode))
		}
	}
	if c.cfg.observer != nil {
		c.emitObservation(ctx, req, target, resp, nil, attempt, time.Since(start))
	}
	return resp, nil
}

// emitObservation delivers an Observation to the configured Observer
// (REQ-098). Caller MUST ensure c.cfg.observer is non-nil. Panics
// inside the observer are recovered and logged via the configured
// slog.Logger so a faulty observer cannot break request handling.
func (c *Client) emitObservation(ctx context.Context, req *Request, target *url.URL, resp *Response, err error, attempts int, dur time.Duration) {
	obs := Observation{
		Method:   req.effectiveMethod(),
		Route:    req.effectiveRoute(),
		URL:      sanitisedURL(target),
		Duration: dur,
		Attempts: attempts,
		Err:      err,
		Tags:     observationTagsFromContext(ctx),
	}
	if resp != nil {
		obs.StatusCode = resp.StatusCode
	}
	defer func() {
		if r := recover(); r != nil {
			c.cfg.logger.Error("transport: observer panicked", "panic", r, "route", obs.Route)
		}
	}()
	c.cfg.observer.OnRequest(obs)
}

// doOnce performs one HTTP attempt. Returns (resp, err) where err is
// non-nil for wire-level failures or transport errors; resp.StatusCode
// can be inspected even on err for diagnostic purposes.
func (c *Client) doOnce(ctx context.Context, req *Request, target *url.URL) (*Response, error) {
	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.effectiveMethod(), target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("transport: build request: %w", err)
	}
	if err := c.plumbHeaders(ctx, req, httpReq); err != nil {
		return nil, err
	}

	// Propagate W3C traceparent / tracestate from the active span.
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(httpReq.Header))

	httpResp, err := c.cfg.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("transport: %s %s: %w", req.effectiveMethod(), req.effectiveRoute(), err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("transport: %s %s: read body: %w", req.effectiveMethod(), req.effectiveRoute(), err)
	}
	resp := &Response{
		StatusCode: httpResp.StatusCode,
		Header:     httpResp.Header.Clone(),
		Body:       respBody,
		Metadata:   parseMetadata(httpResp.Header),
	}
	if httpResp.StatusCode >= 200 && httpResp.StatusCode < 300 {
		return resp, nil
	}
	return resp, c.mapWireError(req, target, resp)
}

func (c *Client) plumbHeaders(ctx context.Context, req *Request, httpReq *http.Request) error {
	httpReq.Header.Set("Accept", req.effectiveAccept())
	if len(req.Body) > 0 {
		httpReq.Header.Set("Content-Type", req.effectiveContentType())
	}
	if c.cfg.userAgent != "" {
		httpReq.Header.Set("User-Agent", c.cfg.userAgent)
	}
	if c.cfg.sendCadastoHeader && c.cfg.specVersion != "" {
		httpReq.Header.Set("Cadasto-OpenEhr-Spec-Version", c.cfg.specVersion)
	}
	if req.IfMatch != "" {
		httpReq.Header.Set("If-Match", quoteIfMatch(req.IfMatch))
	}
	if v := req.Prefer.HeaderValue(); v != "" {
		httpReq.Header.Set("Prefer", v)
	}
	if req.AuditDetailsHeader != "" {
		httpReq.Header.Set("openehr-audit-details", req.AuditDetailsHeader)
	}
	if req.RMVersion != "" {
		httpReq.Header.Set("openehr-version", req.RMVersion)
	}
	if req.TemplateID != "" {
		httpReq.Header.Set("openehr-template-id", req.TemplateID)
	}
	if req.URI != "" {
		httpReq.Header.Set("openehr-uri", req.URI)
	}
	if req.ItemTag != "" {
		httpReq.Header.Set("openehr-item-tag", req.ItemTag)
	}
	if req.VersionItemTag != "" {
		httpReq.Header.Set("openehr-version-item-tag", req.VersionItemTag)
	}

	// Caller attribution: per-request override wins over client default.
	attr := c.cfg.callerAttribution
	if v, ok := CallerAttributionFromContext(ctx); ok {
		attr = v
	}
	if !attr.IsEmpty() {
		if v := attr.HeaderJSON(); v != "" {
			httpReq.Header.Set(c.cfg.callerAttributionHeader, v)
			// Also surface on the active span so OTel-driven audit
			// reaches a destination beyond the wire (REQ-066).
			span := trace.SpanFromContext(ctx)
			if attr.AgentID != "" {
				span.SetAttributes(attribute.String("caller.agent_id", attr.AgentID))
			}
			if attr.ModelProvider != "" {
				span.SetAttributes(attribute.String("caller.model_provider", attr.ModelProvider))
			}
			for k, val := range attr.Attributes {
				span.SetAttributes(attribute.String("caller."+k, val))
			}
		}
	}

	// Caller-supplied extra headers — applied after so they may
	// override the standard plumbing on purpose. Use canonical keys so
	// overrides merge with values set via Header.Set above.
	for k, vv := range req.Headers {
		canon := http.CanonicalHeaderKey(k)
		httpReq.Header.Del(canon)
		for _, v := range vv {
			httpReq.Header.Add(canon, v)
		}
	}

	if !req.NoAuth {
		ts := c.tokenSourceFor(ctx)
		tok, err := ts.Token(ctx)
		if err != nil {
			return fmt.Errorf("transport: acquire token: %w", err)
		}
		if !tok.IsZero() {
			typ := tok.Type
			if typ == "" {
				typ = "Bearer"
			}
			httpReq.Header.Set("Authorization", typ+" "+tok.Value)
		}
	}
	return nil
}

// tokenSourceFor returns the per-request TokenSource attached to ctx,
// falling back to the client default.
func (c *Client) tokenSourceFor(ctx context.Context) auth.TokenSource {
	if ts, ok := auth.TokenSourceFromContext(ctx); ok {
		return ts
	}
	return c.cfg.tokenSrc
}

// reauthAfter401 reports whether the request should be retried once after a
// wire 401 because the active TokenSource supports invalidation (REQ-063).
// When it returns true it has already invalidated the cached token, so the
// next attempt's plumbHeaders acquires a fresh one. Requests that suppress
// auth (NoAuth) or whose source is not Invalidatable surface the 401 as-is.
func (c *Client) reauthAfter401(ctx context.Context, req *Request, resp *Response) bool {
	if req.NoAuth || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		return false
	}
	inv, ok := c.tokenSourceFor(ctx).(auth.Invalidatable)
	if !ok {
		return false
	}
	inv.Invalidate()
	return true
}

// mapWireError maps a non-2xx response onto a typed *WireError, decoding
// the openEHR error envelope when possible.
func (c *Client) mapWireError(req *Request, target *url.URL, resp *Response) error {
	we := &WireError{
		StatusCode: resp.StatusCode,
		Method:     req.effectiveMethod(),
		URL:        sanitisedURL(target),
		Route:      req.effectiveRoute(),
		RawBody:    append([]byte(nil), resp.Body...),
		Sentinel:   statusToSentinel(resp.StatusCode),
	}
	if detail, ok := decodeOpenEHRError(resp.Body); ok {
		we.OpenEHR = detail
	}
	return we
}

func statusToSentinel(s int) error {
	switch s {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusConflict:
		return ErrVersionConflict
	case http.StatusPreconditionFailed:
		return ErrPreconditionFailed
	case http.StatusPreconditionRequired:
		return ErrPreconditionRequired
	}
	if s >= 500 {
		return ErrServerError
	}
	return nil
}

// decodeOpenEHRError attempts to parse the openEHR error envelope
// (REQ-093). Returns ok=false when the body is empty, non-JSON, or
// missing both message and code (the envelope is best-effort).
func decodeOpenEHRError(body []byte) (*OpenEHRErrorDetail, bool) {
	if len(body) == 0 {
		return nil, false
	}
	var d OpenEHRErrorDetail
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, false
	}
	if d.Message == "" && d.Code == "" {
		return nil, false
	}
	return &d, true
}

// shouldRetry consults the configured RetryPolicy. Network errors are
// retried in addition to retriable HTTP statuses (mirroring the
// "transport-level transient failures" rationale of REQ-091).
func (c *Client) shouldRetry(req *Request, resp *Response, err error, attempt int) bool {
	if !c.cfg.retry.enabled() || attempt >= c.cfg.retry.MaxAttempts {
		return false
	}
	if !c.cfg.retry.retriableMethod(req.effectiveMethod()) {
		return false
	}
	if err != nil {
		// A *WireError carries a status — defer to the status-based
		// retriable check so RetriableStatus is the single gate.
		// Anything else is a network / transport / token error;
		// retry per the method's idempotency.
		var we *WireError
		if errors.As(err, &we) {
			return c.cfg.retry.retriable(req.effectiveMethod(), we.StatusCode)
		}
		return true
	}
	if resp == nil {
		return false
	}
	return c.cfg.retry.retriable(req.effectiveMethod(), resp.StatusCode)
}

func joinTarget(base *url.URL, path string, query url.Values) (*url.URL, error) {
	if base == nil {
		return nil, fmt.Errorf("nil base URL")
	}
	out := *base
	out.Path = joinPaths(base.Path, path)
	if len(query) > 0 {
		q := out.Query()
		for k, vv := range query {
			q[k] = append(q[k][:0], vv...)
		}
		out.RawQuery = q.Encode()
	}
	return &out, nil
}

func joinPaths(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	if a[len(a)-1] == '/' && b[0] == '/' {
		return a + b[1:]
	}
	if a[len(a)-1] != '/' && b[0] != '/' {
		return a + "/" + b
	}
	return a + b
}

// sanitisedURL strips any userinfo from u for emission on logs and
// OTel attributes (REQ-090). Bearer tokens never appear in u (they're
// in headers, not the URL) but defensive stripping costs nothing.
func sanitisedURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	out := *u
	out.User = nil
	return out.String()
}

// Decode is a typed wrapper around Do — executes req, decodes the
// response body as canonical JSON into a fresh *T, and returns the
// typed result plus the parsed Metadata.
//
// Decode honours the request-level NoAuth and Prefer fields; callers
// targeting Prefer=minimal endpoints typically use Do directly so the
// empty-body shape does not trip the decoder.
//
// Generic over T per REQ-024.
func Decode[T any](ctx context.Context, c *Client, req *Request) (*T, *Metadata, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, resp.Metadata, err
		}
		return nil, nil, err
	}
	if len(resp.Body) == 0 {
		return nil, resp.Metadata, fmt.Errorf("%w: response body is empty (Prefer mismatch?)", ErrInvalidShape)
	}
	out := new(T)
	if err := canjson.Unmarshal(resp.Body, out); err != nil {
		return nil, resp.Metadata, fmt.Errorf("transport: decode %s %s: %w", req.effectiveMethod(), req.effectiveRoute(), err)
	}
	return out, resp.Metadata, nil
}

package transport

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/internal/urlredact"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// tracerName is the OTel instrumentation scope. Stable so callers can
// configure sampling per-package.
const tracerName = "github.com/cadasto/openehr-sdk-go/transport"

// DefaultCallerAttributionHeader is the header name used when
// WithCallerAttributionHeader is not overridden.
const DefaultCallerAttributionHeader = "X-Cadasto-Caller-Attribution"

// Client is the HTTP client wrapper every openEHR REST leaf client
// uses to reach a Cadasto CDR or any conformant openEHR backend. It
// owns:
//
//   - the injected *http.Client
//   - the resolved smart/discovery.ServiceCatalog
//   - the client-default auth.TokenSource
//   - the retry policy
//   - the OTel hooks
//
// Each Client instance is bound to one issuer or tenant context. A
// caller that federates across several backends constructs one Client
// per backend instead of sharing one.
//
// A Client is safe for concurrent use by multiple goroutines.
type Client struct {
	cfg     config
	catalog *discovery.ServiceCatalog
}

// New constructs a Client. The ServiceCatalog is mandatory, and the
// HTTP client must be injected with WithHTTPClient.
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
		if o != nil {
			o(&cfg)
		}
	}
	if catalog == nil {
		return nil, fmt.Errorf("%w: ServiceCatalog is required", ErrInvalidConfig)
	}
	if cfg.httpClient == nil {
		return nil, fmt.Errorf("%w: WithHTTPClient is required", ErrInvalidConfig)
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
	// The client-default attribution is copied, so a later change to the
	// caller's Attributes map neither reaches Do nor races with it, and its
	// header value is encoded here once rather than on every request.
	cfg.callerAttribution.Attributes = maps.Clone(cfg.callerAttribution.Attributes)
	cfg.callerAttributionJSON = cfg.callerAttribution.HeaderJSON()
	return &Client{cfg: cfg, catalog: catalog}, nil
}

// Catalog returns the ServiceCatalog the Client was constructed with.
// Useful for leaf clients that want to project entries before issuing
// a request (e.g. to extract the SpecVersion for observability).
// A nil Client returns nil, matching Do's handling of a nil Client.
func (c *Client) Catalog() *discovery.ServiceCatalog {
	// REQ-025: a nil Client is caller-constructible — see Do.
	if c == nil {
		return nil
	}
	return c.catalog
}

// HTTPClient returns the injected *http.Client. SDK packages that issue
// requests outside the catalog-routed Do pipeline, such as the
// deployment-level health probes in cadasto/admin, use it to reuse the
// configured HTTP transport without injecting or wrapping it again. It
// returns nil only when the Client itself is nil; a Client built by New
// always has an HTTP client.
//
// The client comes back as it was injected. It lacks the refusal Do applies
// to a request that carries a credential header (Authorization,
// Proxy-Authorization, Cookie or Cookie2), which stops it following a
// redirect from https to a URL that is not https, so a caller that sends a
// credential through it must refuse that redirect itself.
func (c *Client) HTTPClient() *http.Client {
	// REQ-025: a nil Client is caller-constructible — see Do.
	if c == nil {
		return nil
	}
	return c.cfg.httpClient
}

// String describes c by the base URL of its service catalog, with the
// password of one that carries one masked. It never shows the token source,
// the reauther or anything else of c that can hold a credential, and it reads
// nothing a request changes, so it is safe to call while other goroutines use
// c. A nil c gives "<nil>", and a Client without a catalog an empty base URL.
//
// String, GoString and Format are methods of *Client, because New hands out
// only pointers. A Client value copied out of one has none of them, and some
// fmt verbs print its fields, a token source's credential included, so print
// the pointer.
func (c *Client) String() string {
	if c == nil {
		return "<nil>"
	}
	var baseURL string
	if c.catalog != nil {
		baseURL = urlredact.String(c.catalog.BaseURL)
	}
	return fmt.Sprintf("transport.Client{BaseURL: %q}", baseURL)
}

// GoString returns the text of [Client.String], so that printing c in Go
// syntax shows no credential either.
func (c *Client) GoString() string { return c.String() }

// Format writes the text of [Client.String] for every fmt verb, whatever its
// flags and width. Without it, a verb that does not suit a pointer, such as
// %s, %q or %t, would print the fields of c and the token source and
// reauther they hold, credentials included, since fmt calls no method of a
// value it reaches through an unexported field. With a nil f it does
// nothing.
func (c *Client) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, c.String())
}

// Do executes req and returns the captured Response. Status codes in
// 2xx surface as a non-nil Response with err=nil; non-2xx surface as
// an err from which errors.AsType recovers a *WireError, plus a
// possibly-non-nil Response carrying the raw body. Network errors
// surface as the wrapped underlying error.
//
// A response whose status and headers arrived but whose body could not
// be read (the connection dropped mid-body, the body did not match its
// Content-Encoding, or it was over the WithMaxResponseBody ceiling) still
// returns a non-nil Response carrying StatusCode, Header and Metadata,
// with an empty Body. On a 2xx the error is, or wraps, a *BodyReadError.
// On any other status the error wraps two errors side by side: the
// *WireError the status gives and the read failure. The *WireError itself
// does not wrap the read failure. Either way errors.Is on the returned
// error reaches the read failure: io.ErrUnexpectedEOF, the content-coding
// error, or ErrResponseTooLarge. The error's text names the kind of read
// failure, and the limit for a body over the ceiling, but never the read
// failure's own text.
//
// With retries enabled, a 2xx whose body could not be read is sent again
// only when its method is GET, HEAD or OPTIONS, because the server has
// acted on any other, and never when the body was over the
// WithMaxResponseBody ceiling, because every attempt would stop at the
// same point. Any other status is retried by its status, as any
// *WireError is, a body over the ceiling included. Neither is sent again
// once ctx has ended. When ctx ends during the backoff, the error keeps
// what it wrapped and wraps ctx's error beside it, and the Response is
// still returned.
//
// The lifecycle:
//
//  1. Resolve the service base URL from the catalog by ServiceID.
//  2. Build the http.Request, plumb headers, attach the bearer token.
//  3. Emit an OTel span and propagate traceparent.
//  4. Execute via the injected *http.Client, or, for a request carrying
//     a credential header (Authorization, Proxy-Authorization, Cookie or
//     Cookie2), via a per-request copy of it that refuses a redirect from
//     https to a URL that is not https (see WithHTTPClient).
//  5. Retry per RetryPolicy on retriable statuses.
//  6. Parse the response body into Body + Metadata; map the wire
//     error envelope onto the typed-sentinel hierarchy.
func (c *Client) Do(ctx context.Context, req *Request) (*Response, error) {
	// REQ-025: a nil Client is caller-constructible and reaches here
	// through every Do-routed leaf, so it fails closed with a typed error
	// rather than dereferencing. Leaves that bypass Do by design (the
	// admin probes) carry their own guard.
	if c == nil {
		return nil, fmt.Errorf("transport.Do: %w: nil Client", ErrInvalidConfig)
	}
	// Skip the wall-clock capture when no observer is registered —
	// the cost is only sub-µs (time.Now is a vDSO call) but observerless
	// callers are the hot path the benchmark consumer of REQ-098 cares
	// about. Gate at the call site so emitObservation has no nil-check.
	// It is taken first, so a refusal's Duration covers the checks too.
	var start time.Time
	if c.cfg.observer != nil {
		start = time.Now()
	}
	svc, target, err := c.prepare(ctx, req)
	if err != nil {
		// REQ-098: a call refused before any attempt is still one logical
		// call, so the observer fires for it too, with no attempt, no status
		// and no URL, since none was built.
		if c.cfg.observer != nil {
			c.emitObservation(ctx, req, "", nil, err, 0, time.Since(start))
		}
		return nil, err
	}
	// The URL is turned into text once per call: the request line on every
	// attempt uses targetURL, and the span, the observation and any
	// WireError use safeURL, the same text without userinfo.
	targetURL := target.String()
	safeURL := targetURL
	if target.User != nil {
		safeURL = sanitisedURL(target)
	}

	tp := c.cfg.tracerProvider
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	tracer := tp.Tracer(tracerName)
	spanName := req.effectiveMethod() + " " + req.effectiveRoute()
	ctx, span := tracer.Start(
		ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.method", req.effectiveMethod()),
			attribute.String("http.route", req.effectiveRoute()),
			attribute.String("http.url", safeURL),
			attribute.String("openehr.spec_version", svc.SpecVersion),
		),
	)
	defer span.End()

	var (
		resp        *Response
		lastErr     error
		attempt     int // retry-policy attempt index; may be capped at reauth
		otelAttempt int // monotonically-increasing total attempt index (never reset)
		reauthed    bool
	)
	for {
		attempt++
		otelAttempt++
		span.SetAttributes(attribute.Int("retry.attempt", otelAttempt-1))
		resp, lastErr = c.doOnce(ctx, req, targetURL, safeURL)
		if !c.shouldRetry(ctx, req, resp, lastErr, attempt) {
			// 401→reauth safety net (REQ-063, opt-in): if the final error is
			// a wire 401, a Reauther is configured, this Do call has not yet
			// reauthed, and the 401's Bearer challenge allows it (REQ-166),
			// invoke Reauth once and retry the request once.
			// The retry re-fetches the token via tokenSourceFor → now fresh.
			// On a second 401 the error is surfaced unchanged. If Reauth itself
			// fails, its error is surfaced wrapped, with the 401 wrapped beside
			// it, so errors.Is still reaches ErrUnauthorized and errors.AsType
			// the *WireError; neither text carries a request value. This guard
			// fires at most once per Do call.
			if !reauthed && c.cfg.reauther != nil && isWire401(lastErr) && challengePermitsReauth(lastErr) {
				reauthed = true
				if raErr := c.cfg.reauther.Reauth(ctx); raErr != nil {
					lastErr = fmt.Errorf("transport: reauth: %w: %w", raErr, lastErr)
					break
				}
				// Single reauth-retry: cap attempt at MaxAttempts so that
				// shouldRetry returns false on the very next iteration,
				// granting exactly one additional upstream call without
				// reopening the 5xx retry budget (REQ-063, REQ-091).
				// When retry is disabled (MaxAttempts ≤ 1), attempt is
				// already beyond the budget — shouldRetry still returns
				// false and the loop runs once more before the inner guard
				// (reauthed==true) prevents further reauth loops.
				if c.cfg.retry.enabled() {
					attempt = c.cfg.retry.MaxAttempts
				}
				continue
			}
			break
		}
		wait := c.cfg.retry.backoff(attempt)
		span.SetAttributes(attribute.Int64("retry.backoff_ms", wait.Milliseconds()))
		if err := retryWait(ctx, wait); err != nil {
			// REQ-091: a response whose body could not be read keeps its
			// arm's error when ctx ends during the backoff: the *BodyReadError
			// on a 2xx, the *WireError and the read failure on any other
			// status. The status the server sent is then not reported as a
			// bare context error; ctx's error is wrapped beside it.
			// Any other error is replaced by ctx's, as before.
			if isBodyReadFailure(lastErr) {
				lastErr = fmt.Errorf("%w: retry wait: %w", lastErr, err)
			} else {
				lastErr = err
			}
			break
		}
	}
	// REQ-090: the status code goes on the span whenever a response arrived,
	// a failed request's included, and so does the response size when the body
	// was read in full. A body that could not be read (REQ-151) has no size:
	// its Response carries an empty Body, so a 0 would be a false value.
	if resp != nil {
		span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
		if !isBodyReadFailure(lastErr) {
			span.SetAttributes(attribute.Int64("http.response_content_length", int64(len(resp.Body))))
		}
	}
	if lastErr != nil {
		span.SetStatus(codes.Error, lastErr.Error())
		span.RecordError(lastErr)
		if c.cfg.observer != nil {
			c.emitObservation(ctx, req, safeURL, resp, lastErr, otelAttempt, time.Since(start))
		}
		return resp, lastErr
	}
	// Here lastErr is nil, which only a 2xx gives: doOnce maps any other
	// status to a *WireError, and mapWireError never returns nil.
	if c.cfg.observer != nil {
		c.emitObservation(ctx, req, safeURL, resp, nil, otelAttempt, time.Since(start))
	}
	return resp, nil
}

// prepare runs the checks Do makes before any attempt and resolves the URL
// the request is sent to. Every error it returns is a refusal: nothing has
// left the process. The URL is nil with any error.
func (c *Client) prepare(ctx context.Context, req *Request) (discovery.ServiceEntry, *url.URL, error) {
	if req == nil {
		return discovery.ServiceEntry{}, nil, fmt.Errorf("transport.Do: %w: nil Request", ErrInvalidConfig)
	}
	if err := ctx.Err(); err != nil {
		return discovery.ServiceEntry{}, nil, err
	}
	svc, ok := c.catalog.Service(req.effectiveServiceID())
	if !ok {
		return discovery.ServiceEntry{}, nil, fmt.Errorf("%w: %q", ErrServiceUnavailable, req.effectiveServiceID())
	}
	// REQ-150: validate before the URL is built, so a traversal or
	// control-character segment never reaches the wire. Both checks live
	// here rather than inside joinTarget: the arity half needs req.Route,
	// and joinTarget's error is re-wrapped with ErrInvalidConfig below —
	// returning the validators' chain through that branch would wrap the
	// sentinel twice.
	if err := ValidateRequestPath(req.Path); err != nil {
		return discovery.ServiceEntry{}, nil, err
	}
	// A parameter carrying `/` yields individually legal segments, so only
	// the arity betrays it. Keyed on req.Route directly, never on
	// effectiveRoute() — that falls back to Path, and comparing a path's
	// arity against itself never fires. The error names the two counts and
	// the template, never the path, which holds the caller's identifiers
	// (REQ-093); a template holds none.
	if req.Route != "" {
		if got, want := segmentCount(req.Path), segmentCount(req.Route); got != want {
			return discovery.ServiceEntry{}, nil, fmt.Errorf("%w: %w: path has %d segments, route template %q has %d", ErrInvalidConfig, ErrInvalidPathSegment, got, req.Route, want)
		}
	}
	target, err := joinTarget(svc.BaseURL, req.Path, req.Query)
	if err != nil {
		return discovery.ServiceEntry{}, nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	return svc, target, nil
}

// emitObservation delivers an Observation to the configured Observer
// (REQ-098). Caller MUST ensure c.cfg.observer is non-nil. Panics
// inside the observer are recovered and logged via the configured
// slog.Logger so a faulty observer cannot break request handling.
// A nil req (a call refused for it) leaves Method and Route empty; safeURL
// is the request URL without userinfo, or "" when none was built.
func (c *Client) emitObservation(ctx context.Context, req *Request, safeURL string, resp *Response, err error, attempts int, dur time.Duration) {
	obs := Observation{
		URL:      safeURL,
		Duration: dur,
		Attempts: attempts,
		Err:      err,
		Tags:     observationTagsFromContext(ctx),
	}
	if req != nil {
		obs.Method = req.effectiveMethod()
		obs.Route = req.effectiveRoute()
	}
	if resp != nil {
		obs.StatusCode = resp.StatusCode
	}
	defer func() {
		if r := recover(); r != nil {
			// REQ-093: the log names the request by its route template, or
			// the placeholder, never by obs.Route, which falls back to the
			// Path and so can carry a caller-supplied identifier.
			route := unroutedRoute
			if req != nil {
				route = req.routeOrPlaceholder()
			}
			c.cfg.logger.ErrorContext(ctx, "transport: observer panicked", "panic", r, "route", route)
		}
	}()
	c.cfg.observer.OnRequest(obs)
}

// doOnce performs one HTTP attempt. Returns (resp, err) where err is
// non-nil for wire-level failures or transport errors. resp is nil only
// when no response arrived (the request could not be built or sent);
// once the status and headers arrived it is non-nil even on err, and its
// Body is empty when the body could not be read. The error is then a
// *BodyReadError on a 2xx, and on any other status an error that wraps
// both the *WireError and the read failure.
//
// targetURL is the request URL as text, and safeURL the same without
// userinfo, for the WireError.
func (c *Client) doOnce(ctx context.Context, req *Request, targetURL, safeURL string) (*Response, error) {
	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.effectiveMethod(), targetURL, body)
	if err != nil {
		// A URL that does not parse arrives as url.Parse's own *url.Error,
		// which quotes the resolved URL: its path may hold a caller-supplied
		// identifier and its base URL a password. Name the request by its
		// route slot instead, as the network-failure arm below does
		// (REQ-093). The marker keeps the retry policy from sending a
		// request again that fails the same way on every attempt (REQ-091).
		route := req.routeOrPlaceholder()
		return nil, &requestBuildError{err: fmt.Errorf("transport: build request %s %s: %w", req.effectiveMethod(), route, sanitiseURLError(err, route))}
	}
	if err := c.plumbHeaders(ctx, req, httpReq); err != nil {
		return nil, err
	}

	// Propagate W3C traceparent / tracestate from the active span.
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(httpReq.Header))

	// REQ-092: a request carrying a credential goes out on a per-request
	// copy that refuses an https to non-https redirect; see httpClientFor.
	httpResp, err := c.httpClientFor(httpReq).Do(httpReq)
	if err != nil {
		// A network failure arrives from net/http as *url.Error, whose own
		// Error() prints the resolved URL it was dialling — so this arm's
		// value-free prefix is not enough on its own (REQ-093).
		// sanitiseURLError substitutes the same route slot into that URL
		// field while keeping the error's type and cause intact. The
		// build-request arm above does the same; the body-read failures
		// below wrap errors that carry no request URL of their own.
		route := req.routeOrPlaceholder()
		return nil, fmt.Errorf("transport: %s %s: %w", req.effectiveMethod(), route, sanitiseURLError(err, route))
	}
	defer func() { _ = httpResp.Body.Close() }()

	limit := c.cfg.maxResponseBody
	if limit == 0 {
		limit = DefaultMaxResponseBody
	}
	// The reader stops one byte past the limit, so a body over it is detected
	// rather than cut short. No body can be longer than math.MaxInt64 bytes,
	// and the extra byte would overflow that limit into a negative count that
	// reads nothing, so it gets no reader and no check (REQ-093).
	bounded := limit > 0 && limit < math.MaxInt64
	var reader io.Reader = httpResp.Body
	if bounded {
		reader = io.LimitReader(httpResp.Body, limit+1)
	}
	respBody, readErr := readBody(reader, bodySizeHint(httpReq.Method, httpResp.ContentLength, limit, bounded))
	if readErr == nil && bounded && int64(len(respBody)) > limit {
		readErr = responseTooLargeError{limit: limit}
	}
	resp := &Response{
		StatusCode: httpResp.StatusCode,
		Header:     httpResp.Header.Clone(),
		Metadata:   parseMetadata(httpResp.Header),
	}
	is2xx := httpResp.StatusCode >= 200 && httpResp.StatusCode < 300
	if readErr != nil {
		// REQ-151: the status and headers arrived, so they are kept even
		// though the body is lost. The bytes read before the failure are
		// dropped: a partial body is not the representation, and an error
		// that carried them could leak payload values.
		if is2xx {
			return resp, &BodyReadError{
				Method:     req.effectiveMethod(),
				Route:      req.routeOrPlaceholder(),
				StatusCode: httpResp.StatusCode,
				Inner:      readErr,
			}
		}
		// A status that is not 2xx still classifies the failure. With no
		// body, mapWireError finds no envelope and keeps no RawBody. The
		// read failure is wrapped beside the WireError, so errors.AsType
		// still reaches the WireError for the status sentinel, the retry
		// policy and the 401 safety net. bodyReadFailure keeps the read
		// failure's own text out of the message, as BodyReadError does.
		return resp, fmt.Errorf("%w: %w", c.mapWireError(req, safeURL, resp), &bodyReadFailure{cause: readErr})
	}
	resp.Body = respBody
	if is2xx {
		return resp, nil
	}
	return resp, c.mapWireError(req, safeURL, resp)
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
		httpReq.Header.Set(headerCadastoSpecVersion, c.cfg.specVersion)
	}
	if req.IfMatch != "" {
		httpReq.Header.Set("If-Match", quoteIfMatch(req.IfMatch))
	}
	if v := req.Prefer.HeaderValue(); v != "" {
		httpReq.Header.Set("Prefer", v)
	}
	if req.AuditDetailsHeader != "" {
		httpReq.Header.Set(headerOpenEHRAuditDetails, req.AuditDetailsHeader)
	}
	if req.RMVersion != "" {
		httpReq.Header.Set(headerOpenEHRVersion, req.RMVersion)
	}
	if req.TemplateID != "" {
		httpReq.Header.Set(headerOpenEHRTemplateID, req.TemplateID)
	}
	if req.URI != "" {
		httpReq.Header.Set(headerOpenEHRURI, req.URI)
	}
	if req.ItemTag != "" {
		httpReq.Header.Set(headerOpenEHRItemTag, req.ItemTag)
	}
	if req.VersionItemTag != "" {
		httpReq.Header.Set(headerOpenEHRVersionItemTag, req.VersionItemTag)
	}

	// Caller attribution: per-request override wins over client default.
	// The client default's header value was encoded once, at New.
	attr, attrJSON := c.cfg.callerAttribution, c.cfg.callerAttributionJSON
	if v, ok := CallerAttributionFromContext(ctx); ok {
		attr, attrJSON = v, v.HeaderJSON()
	}
	if attrJSON != "" {
		httpReq.Header.Set(c.cfg.callerAttributionHeader, attrJSON)
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

	// Caller-supplied extra headers — applied after so they may
	// override the standard plumbing on purpose. Use canonical keys so
	// overrides merge with values set via Header.Set above. The token below
	// is set last, so a token that is not zero replaces a caller's own
	// Authorization.
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
			typ := cmp.Or(tok.Type, auth.TokenTypeBearer)
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

// mapWireError maps a non-2xx response onto a typed *WireError, decoding
// the openEHR error envelope when possible.
//
// By default (rawErrorBodies false) four fields are left empty, because
// each is free text from the server that may contain PHI: the openEHR
// envelope's Message and ValidationErrors, the raw response body
// (RawBody), and the Bearer challenge's ErrorDescription. The openEHR Code
// and CodedText (coded terminology identifiers) and the challenge's other
// fields are always preserved. Enable WithRawErrorBodies to retain the
// full payload for diagnostics in environments where WireError values
// cannot reach logs or traces.
func (c *Client) mapWireError(req *Request, safeURL string, resp *Response) error {
	we := &WireError{
		StatusCode: resp.StatusCode,
		Method:     req.effectiveMethod(),
		URL:        safeURL,
		Route:      req.routeOrPlaceholder(),
		Sentinel:   statusToSentinel(resp.StatusCode),
	}
	if detail, ok := decodeOpenEHRError(resp.Body); ok {
		if !c.cfg.rawErrorBodies {
			detail.Message = ""
			detail.ValidationErrors = nil
		}
		we.OpenEHR = detail
	}
	if c.cfg.rawErrorBodies {
		we.RawBody = append([]byte(nil), resp.Body...)
	}
	// REQ-166: the challenge is read on 401 and 403 only, from every
	// WWW-Authenticate line. It never feeds Error(), so its coded fields
	// are attached whatever WithRawErrorBodies says; error_description is
	// free text from the server and is kept only under that opt-in, like
	// the envelope's Message above.
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		we.Challenge = parseBearerChallenge(resp.Header.Values(headerWWWAuthenticate))
		if we.Challenge != nil && !c.cfg.rawErrorBodies {
			we.Challenge.ErrorDescription = ""
		}
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
	case http.StatusUnprocessableEntity:
		return ErrUnprocessable
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
// missing message, code and validation errors (the envelope is
// best-effort).
func decodeOpenEHRError(body []byte) (*OpenEHRErrorDetail, bool) {
	if len(body) == 0 {
		return nil, false
	}
	var d OpenEHRErrorDetail
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, false
	}
	if d.Message == "" && d.Code == "" && len(d.ValidationErrors) == 0 {
		return nil, false
	}
	return &d, true
}

// isWire401 reports whether err is a *WireError whose StatusCode is 401.
// Used by the opt-in 401→reauth guard in Do (REQ-063).
// A boxed typed-nil *WireError matches with ok=true and a nil pointer,
// so the nil check is load-bearing (REQ-025 nil-receiver axis).
func isWire401(err error) bool {
	we, ok := errors.AsType[*WireError](err)
	return ok && we != nil && we.StatusCode == 401
}

// challengePermitsReauth reports whether the Bearer challenge on err's
// *WireError lets the opt-in 401 safety net call Reauth (REQ-166, REQ-063).
// A 401 whose challenge names insufficient_scope or any error other than
// invalid_token is surfaced at once, since a fresh token from the same
// grant cannot fix it. A boxed typed-nil *WireError answers false, so the
// nil check is load-bearing (REQ-025 nil-receiver axis).
func challengePermitsReauth(err error) bool {
	we, ok := errors.AsType[*WireError](err)
	return ok && we != nil && we.Challenge.permitsReauth()
}

// isBodyReadFailure reports whether err is a response whose body could not be
// read, on either arm: a *BodyReadError on a 2xx, or the bodyReadFailure doOnce
// wraps beside the *WireError on any other status (REQ-151). A boxed typed-nil
// *BodyReadError counts too, since the callers read the request and ctx, never
// a field of the error.
func isBodyReadFailure(err error) bool {
	if _, ok := errors.AsType[*BodyReadError](err); ok {
		return true
	}
	_, ok := errors.AsType[*bodyReadFailure](err)
	return ok
}

// shouldRetry consults the configured RetryPolicy. Network errors are
// retried in addition to retriable HTTP statuses (mirroring the
// "transport-level transient failures" rationale of REQ-091). ctx is read
// only for a response whose body could not be read.
func (c *Client) shouldRetry(ctx context.Context, req *Request, resp *Response, err error, attempt int) bool {
	if !c.cfg.retry.enabled() || attempt >= c.cfg.retry.MaxAttempts {
		return false
	}
	if !c.cfg.retry.retriableMethod(req.effectiveMethod()) {
		return false
	}
	if err != nil {
		// REQ-091: once ctx has ended, a body read that failed was most likely
		// stopped by it, and another attempt could only fail before a response
		// arrives and lose the status and headers kept here (retries respect
		// ctx cancellation). This holds on both arms, so it comes
		// before the *WireError arm's status-based retry.
		if ctx.Err() != nil && isBodyReadFailure(err) {
			return false
		}
		// A *WireError carries a status — defer to the status-based
		// retriable check so RetriableStatus is the single gate. A boxed
		// typed-nil matches with ok=true and a nil pointer, so the nil
		// check is load-bearing (REQ-025 nil-receiver axis); such an error
		// carries no status and falls through to the generic branch below.
		if we, ok := errors.AsType[*WireError](err); ok && we != nil {
			return c.cfg.retry.retriable(req.effectiveMethod(), we.StatusCode)
		}
		// REQ-091: a 2xx whose body could not be read means the server
		// acted on the request, so only a safe method (RFC 9110 §9.2.1) is
		// sent again, whatever RetryNonIdempotent says. A second PUT with
		// the same If-Match would fail as stale and hide the version the
		// first one made. The decision reads the request's method, never a
		// field of the error, so a boxed typed-nil *BodyReadError takes
		// the same gate. The ctx guard above has already stopped a retry
		// once ctx ended.
		if _, ok := errors.AsType[*BodyReadError](err); ok {
			// A body over the WithMaxResponseBody ceiling would stop at the
			// same ceiling on every attempt, after reading up to it again, so
			// it is not sent again; REQ-091 allows the retry but does not
			// require it. This sits inside the 2xx arm: the *WireError arm
			// above has already returned, so a status over the ceiling is
			// still retried by its status.
			if errors.Is(err, ErrResponseTooLarge) {
				return false
			}
			switch req.effectiveMethod() {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				return true
			default:
				return false
			}
		}
		// Token-source errors that are deterministically terminal (the config
		// is invalid, or re-authentication is required and nothing can refresh
		// it) will fail identically on every attempt — retrying only burns the
		// budget and re-invokes Token(). Treat them as non-retriable.
		if errors.Is(err, auth.ErrReauthRequired) || errors.Is(err, auth.ErrInvalidConfig) {
			return false
		}
		// REQ-092: a refused https to non-https redirect is decided by the
		// server's Location and the request's own credential, so every
		// attempt would be refused the same way.
		if errors.Is(err, ErrInsecureRedirect) {
			return false
		}
		// REQ-091: a request that could not be built has the same method and
		// URL on every attempt, so every attempt would fail the same way.
		if _, ok := errors.AsType[*requestBuildError](err); ok {
			return false
		}
		// Anything else is a network / transport / transient token error;
		// retry per the method's idempotency.
		return true
	}
	if resp == nil {
		return false
	}
	return c.cfg.retry.retriable(req.effectiveMethod(), resp.StatusCode)
}

func joinTarget(base *url.URL, path string, query url.Values) (*url.URL, error) {
	if base == nil {
		return nil, errors.New("nil base URL")
	}
	out := base.Clone()
	out.Path = joinPaths(base.Path, path)
	// The clone keeps the base's RawPath, which no longer matches the joined
	// Path, so String would encode the whole path again from its decoded
	// form and turn an escaped "/" (%2F) in the base into a real separator.
	// The base keeps its own encoding instead; the request path is still
	// encoded from its decoded form alone (REQ-150, REQ-095).
	if base.RawPath != "" {
		out.RawPath = joinPaths(base.EscapedPath(), (&url.URL{Path: path}).EscapedPath())
	}
	if len(query) > 0 {
		out.RawQuery = mergeQuery(base.RawQuery, query)
	}
	return out, nil
}

// mergeQuery returns the raw query for a request whose base URL has the raw
// query baseRaw: the base pairs as configured and in their order, then query.
// A key in query replaces the base pair of the same name, even when query
// holds no value for it.
func mergeQuery(baseRaw string, query url.Values) string {
	// The base pairs are copied, not decoded and encoded again. A round trip
	// can change their bytes ('+' to %20, or %20 to '+'), and with them what
	// a server reads: '+' is a space to an HTML-form decoder but a plus to an
	// RFC 3986 decoder. Only a pair's key is decoded, to find the pairs a
	// request key replaces; a key that does not decode is kept.
	var pairs []string
	for pair := range strings.SplitSeq(baseRaw, "&") {
		if pair == "" {
			continue
		}
		rawKey, _, _ := strings.Cut(pair, "=")
		if key, err := url.QueryUnescape(rawKey); err == nil && query.Has(key) {
			continue
		}
		pairs = append(pairs, pair)
	}
	// REQ-095: the pinned OAS declares query parameters `style: form`,
	// defined by RFC 6570, where a space is %20; `+` is a space only under
	// HTML-form decoding. Encode writes a literal plus as %2B, so every `+`
	// it emits stands for a space.
	if enc := query.Encode(); enc != "" {
		pairs = append(pairs, strings.ReplaceAll(enc, "+", "%20"))
	}
	return strings.Join(pairs, "&")
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

// sanitisedURL returns u as text with its userinfo removed, for the
// http.url span attribute (REQ-090), Observation.URL (REQ-098) and
// WireError.URL. Only the userinfo goes: the path and the query are kept as
// sent, so they can still carry caller-supplied identifiers and any query
// parameter of the service base URL. Bearer tokens never appear in u (they
// are in headers, not the URL).
func sanitisedURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	out := *u
	out.User = nil
	return out.String()
}

// sanitiseURLError replaces the resolved URL a wrapped net/http
// *url.Error prints with route — the caller's route template, or the
// "(unrouted)" placeholder — so the diagnostic of a request that cannot be
// built or sent stays value-free (REQ-093). The resolved URL carries the
// expanded request path, which may hold a caller-supplied identifier such
// as a live ehr_id, and the base URL, which may carry a password.
//
// What it guarantees the caller: the substitution is a shallow copy, so
// the error is still a *url.Error under errors.AsType, with Op and Err
// unchanged — Timeout(), Temporary(), Unwrap() and errors.Is against the
// underlying cause all answer exactly as they did. Only the URL field's
// text differs.
//
// *http.Client.Do documents that it always returns a *url.Error at the
// top of the chain, and http.NewRequestWithContext returns url.Parse's
// *url.Error as it is, so only a top-level match is substituted; anything
// else is returned unchanged rather than rewritten mid-chain, where a
// *url.Error could belong to a cause the transport did not build.
func sanitiseURLError(err error, route string) error {
	ue, ok := errors.AsType[*url.Error](err)
	if !ok || ue == nil {
		return err
	}
	// Identity, not chain membership: substitute only when the match IS
	// the error net/http handed back, never one found deeper in the chain.
	if error(ue) != err { //nolint:errorlint // identity by design — see above
		return err
	}
	out := *ue
	out.URL = route
	return &out
}

// Decode is a typed wrapper around Do. It executes req, decodes the
// response body as canonical JSON into a fresh *T, and returns the
// typed result plus the parsed Metadata.
//
// Decode honours the request-level NoAuth and Prefer fields; callers
// targeting Prefer=minimal endpoints typically use Do directly so the
// empty-body shape does not trip the decoder.
//
// The 2xx failures are distinct. An empty, whitespace-only or
// JSON-null body (no representation, see [IsNoRepresentationBody])
// fails with [ErrInvalidShape]. A non-empty body that does not decode
// returns a [DecodeError] carrying the raw bytes the server delivered.
// A body that could not be read at all returns the [BodyReadError] from
// [Client.Do], with the Metadata beside it.
func Decode[T any](ctx context.Context, c *Client, req *Request) (*T, *Metadata, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, resp.Metadata, err
		}
		return nil, nil, err
	}
	if IsNoRepresentationBody(resp.Body) {
		return nil, resp.Metadata, fmt.Errorf("%w: response body is empty or null", ErrInvalidShape)
	}
	out := new(T)
	if err := canjson.Unmarshal(resp.Body, out); err != nil {
		// REQ-151: a 2xx body that does not decode is a typed failure that
		// keeps the bytes the server delivered, not a wrapped decoder error
		// that drops them.
		return nil, resp.Metadata, &DecodeError{
			Method: req.effectiveMethod(),
			Route:  req.routeOrPlaceholder(),
			Body:   resp.Body,
			Inner:  err,
		}
	}
	return out, resp.Metadata, nil
}

// maxBodyPrealloc caps the buffer readBody sizes from a server-declared
// Content-Length before any body byte has arrived, so a false Content-Length
// cannot force a large allocation. A longer body still reads in full, up to
// the WithMaxResponseBody ceiling: the buffer grows as bytes arrive.
const maxBodyPrealloc = 4 << 20

// bodySizeHint returns the size readBody preallocates for a response body: the
// declared Content-Length, at most limit when the read is bounded and at most
// maxBodyPrealloc, or -1 when the length is unknown. A HEAD response declares
// the length of a body it does not carry, so it gets -1 too.
func bodySizeHint(method string, contentLength, limit int64, bounded bool) int64 {
	if contentLength < 0 || method == http.MethodHead {
		return -1
	}
	hint := min(contentLength, maxBodyPrealloc)
	if bounded {
		hint = min(hint, limit)
	}
	return hint
}

// readBody reads r to its end, as io.ReadAll does. With a hint of 0 or more
// the buffer is sized for that many bytes up front, plus bytes.MinRead so
// that reading the end of a body of exactly that length needs no new
// allocation; with a negative hint it is io.ReadAll.
func readBody(r io.Reader, hint int64) ([]byte, error) {
	if hint < 0 {
		return io.ReadAll(r)
	}
	var buf bytes.Buffer
	buf.Grow(int(hint) + bytes.MinRead)
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}

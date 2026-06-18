package authprobes

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe009CallerAttributionOptIn implements PROBE-009: when caller
// attribution is configured (REQ-066), the SDK emits the configured header
// and the caller.agent_id OTel attribute; when not configured, neither
// appears on the wire (REQ-066).
//
// The probe runs two transport clients against the same in-process
// openEHR endpoint — one with WithCallerAttribution, one without — and
// captures the request header at the server plus the span attributes via
// an in-memory TracerProvider. To avoid taking a hard dependency on the
// OTel SDK in the probe tree, the recorder implements the otel/trace API
// types directly.
func Probe009CallerAttributionOptIn(ctx context.Context) (Result, error) { // PROBE-009 (REQ-066)
	r := Result{Probe: "PROBE-009"}

	const headerName = transport.DefaultCallerAttributionHeader

	// Install an in-memory TracerProvider for the duration of the probe so
	// the transport's span.SetAttributes calls are observable; restore the
	// previous global provider afterwards.
	rec := &spanRecorder{}
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(&recordingTP{rec: rec})
	defer otel.SetTracerProvider(prev)

	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		capturedHeader = req.Header.Get(headerName)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://probe009.example",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		return r, fmt.Errorf("PROBE-009: build catalog: %w", err)
	}

	// Configured client — should emit header + caller.agent_id attribute.
	rec.reset()
	capturedHeader = ""
	configured, err := transport.New(
		cat,
		transport.WithHTTPClient(srv.Client()),
		transport.WithCallerAttribution(transport.CallerAttribution{
			AgentID:       "mcp-probe/1.0.0",
			ModelProvider: "anthropic",
		}),
	)
	if err != nil {
		return r, fmt.Errorf("PROBE-009: build configured client: %w", err)
	}
	if _, err := configured.Do(ctx, &transport.Request{Path: "/ehr"}); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("configured client Do failed: %v", err)
		return r, nil
	}
	if capturedHeader == "" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("configured client emitted no %s header", headerName)
		return r, nil
	}
	if got := rec.attr("caller.agent_id"); got != "mcp-probe/1.0.0" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("configured client caller.agent_id span attr = %q; want mcp-probe/1.0.0", got)
		return r, nil
	}

	// Unconfigured client — neither header nor attribute on the wire.
	rec.reset()
	capturedHeader = ""
	plain, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		return r, fmt.Errorf("PROBE-009: build plain client: %w", err)
	}
	if _, err := plain.Do(ctx, &transport.Request{Path: "/ehr"}); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("plain client Do failed: %v", err)
		return r, nil
	}
	if capturedHeader != "" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("unconfigured client emitted %s = %q; want absent", headerName, capturedHeader)
		return r, nil
	}
	if _, ok := rec.lookup("caller.agent_id"); ok {
		r.Status = "fail"
		r.Detail = "unconfigured client set caller.agent_id span attribute; want absent"
		return r, nil
	}

	r.Status = "pass"
	r.Detail = "configured client emitted attribution header + caller.agent_id attr; unconfigured emitted neither"
	return r, nil
}

// spanRecorder collects the attributes the transport sets on its spans.
type spanRecorder struct {
	mu    sync.Mutex
	attrs []attribute.KeyValue
}

func (s *spanRecorder) reset() {
	s.mu.Lock()
	s.attrs = nil
	s.mu.Unlock()
}

func (s *spanRecorder) record(kv ...attribute.KeyValue) {
	s.mu.Lock()
	s.attrs = append(s.attrs, kv...)
	s.mu.Unlock()
}

func (s *spanRecorder) lookup(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, kv := range s.attrs {
		if string(kv.Key) == key {
			return kv.Value.AsString(), true
		}
	}
	return "", false
}

func (s *spanRecorder) attr(key string) string {
	v, _ := s.lookup(key)
	return v
}

// recordingTP / recordingTracer / recordingSpan implement the otel/trace
// API just enough to capture SetAttributes calls without depending on the
// OTel SDK. Forward-compatibility is provided by embedding the trace
// embedded.* interfaces.
type recordingTP struct {
	embedded.TracerProvider
	rec *spanRecorder
}

func (tp *recordingTP) Tracer(string, ...oteltrace.TracerOption) oteltrace.Tracer {
	return &recordingTracer{rec: tp.rec}
}

type recordingTracer struct {
	embedded.Tracer
	rec *spanRecorder
}

func (t *recordingTracer) Start(ctx context.Context, _ string, _ ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	sp := &recordingSpan{rec: t.rec}
	return oteltrace.ContextWithSpan(ctx, sp), sp
}

type recordingSpan struct {
	embedded.Span
	rec *spanRecorder
}

func (s *recordingSpan) SetAttributes(kv ...attribute.KeyValue) { s.rec.record(kv...) }
func (s *recordingSpan) End(...oteltrace.SpanEndOption)         {}
func (s *recordingSpan) AddEvent(string, ...oteltrace.EventOption) {
}
func (s *recordingSpan) AddLink(oteltrace.Link) {}
func (s *recordingSpan) IsRecording() bool      { return true }
func (s *recordingSpan) RecordError(error, ...oteltrace.EventOption) {
}

func (s *recordingSpan) SpanContext() oteltrace.SpanContext       { return oteltrace.SpanContext{} }
func (s *recordingSpan) SetStatus(codes.Code, string)             {}
func (s *recordingSpan) SetName(string)                           {}
func (s *recordingSpan) TracerProvider() oteltrace.TracerProvider { return &recordingTP{rec: s.rec} }

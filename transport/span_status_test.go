package transport_test

// span_status_test.go — REQ-090 § Spans: the span of a request carries the
// response status code and the response size, on a failed request as on a
// successful one.
//
// The module takes no dependency on the OTel SDK, so the tracer here is a small
// recording implementation of the trace API: it keeps the attributes each span
// is started with or given later, and does nothing else.

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// recordingTracerProvider hands out one recordingTracer, whatever the
// instrumentation scope.
type recordingTracerProvider struct {
	noop.TracerProvider
	tracer *recordingTracer
}

func newRecordingTracerProvider() *recordingTracerProvider {
	return &recordingTracerProvider{tracer: &recordingTracer{}}
}

func (p *recordingTracerProvider) Tracer(string, ...trace.TracerOption) trace.Tracer {
	return p.tracer
}

// recordingTracer starts a recordingSpan for each call and keeps them all.
type recordingTracer struct {
	noop.Tracer
	mu    sync.Mutex
	spans []*recordingSpan
}

func (tr *recordingTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	s := &recordingSpan{name: name, attrs: map[attribute.Key]attribute.Value{}}
	cfg := trace.NewSpanStartConfig(opts...)
	s.SetAttributes(cfg.Attributes()...)
	tr.mu.Lock()
	tr.spans = append(tr.spans, s)
	tr.mu.Unlock()
	return trace.ContextWithSpan(ctx, s), s
}

// only returns the one span the tracer started, failing the test when it
// started none or several.
func (tr *recordingTracer) only(t *testing.T) *recordingSpan {
	t.Helper()
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.spans) != 1 {
		t.Fatalf("the tracer started %d spans, want 1", len(tr.spans))
	}
	return tr.spans[0]
}

// recordingSpan keeps the last value set for each attribute key. Every other
// method is the no-op span's.
type recordingSpan struct {
	noop.Span
	name  string
	mu    sync.Mutex
	attrs map[attribute.Key]attribute.Value
}

func (s *recordingSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range kv {
		s.attrs[a.Key] = a.Value
	}
}

func (s *recordingSpan) attr(key attribute.Key) (attribute.Value, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.attrs[key]
	return v, ok
}

// TestSpanCarriesStatusCode pins REQ-090's "The span SHOULD carry as
// attributes … the status code" on every request a response arrived for. A
// 404 and a 201 whose body is cut (REQ-151) fail, and their spans still carry
// the status the server sent. The 200 read in full is the control that shows
// the recorder sees the attribute at all.
func TestSpanCarriesStatusCode(t *testing.T) { // REQ-090, REQ-151
	cases := []struct {
		name    string
		client  func(t *testing.T, tp trace.TracerProvider) *transport.Client
		req     func() *transport.Request
		status  int
		wantErr bool
	}{
		{
			name: "200 read in full",
			client: func(t *testing.T, tp trace.TracerProvider) *transport.Client {
				return newDecodeClient(t, http.StatusOK, `{}`, nil, transport.WithTracerProvider(tp))
			},
			req: func() *transport.Request {
				return &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"}
			},
			status: http.StatusOK,
		},
		{
			name: "404",
			client: func(t *testing.T, tp trace.TracerProvider) *transport.Client {
				return newDecodeClient(t, http.StatusNotFound, `{"code":"NOT_FOUND"}`, nil, transport.WithTracerProvider(tp))
			},
			req: func() *transport.Request {
				return &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"}
			},
			status:  http.StatusNotFound,
			wantErr: true,
		},
		{
			name: "201 cut mid-body",
			client: func(t *testing.T, tp trace.TracerProvider) *transport.Client {
				return newBodyReadClient(t, cutBodyHandler(http.StatusCreated, committedHeader(), cutPartialBody, nil), transport.WithTracerProvider(tp))
			},
			req:     compositionPost,
			status:  http.StatusCreated,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tp := newRecordingTracerProvider()
			c := tc.client(t, tp)

			_, err := c.Do(t.Context(), tc.req())
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Fatalf("Do returned error %v, want an error: %v; the premise of this test is gone", err, tc.wantErr)
			}
			span := tp.tracer.only(t)
			got, ok := span.attr("http.status_code")
			if !ok {
				t.Fatalf("span %q carries no http.status_code attribute, want %d", span.name, tc.status)
			}
			if got.Type() != attribute.INT64 || got.AsInt64() != int64(tc.status) {
				t.Errorf("span %q http.status_code = %v (%s), want %d", span.name, got.String(), got.Type(), tc.status)
			}
		})
	}
}

// responseSizeKey is the span attribute that carries the response size, named
// after the older OpenTelemetry HTTP conventions the span's other attributes
// follow (http.method, http.url, http.status_code).
const responseSizeKey attribute.Key = "http.response_content_length"

// TestSpanCarriesResponseSize pins REQ-090's "The span SHOULD carry as
// attributes … the response size". A body read in full puts the number of
// bytes read on the span, on a failed request as on a successful one; an empty
// body read in full is a size of 0. A body that could not be read has no size
// (REQ-151): it was cut, or it was over the WithMaxResponseBody ceiling, and
// the Response carries an empty Body. Its span carries no size attribute, since
// a 0 there would be a false value.
func TestSpanCarriesResponseSize(t *testing.T) { // REQ-090, REQ-151
	const (
		okBody       = `{"ehr_id":{"value":"abc"}}`
		notFoundBody = `{"code":"NOT_FOUND","message":"no such EHR"}`
		limit        = 16
	)
	getEHR := func() *transport.Request {
		return &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"}
	}
	cases := []struct {
		name    string
		client  func(t *testing.T, tp trace.TracerProvider) *transport.Client
		req     func() *transport.Request
		wantErr bool
		// wantSize is the size the span carries; noSize means it carries none.
		wantSize int64
		noSize   bool
	}{
		{
			name: "200 read in full",
			client: func(t *testing.T, tp trace.TracerProvider) *transport.Client {
				return newDecodeClient(t, http.StatusOK, okBody, nil, transport.WithTracerProvider(tp))
			},
			req:      getEHR,
			wantSize: int64(len(okBody)),
		},
		{
			name: "204 with no body",
			client: func(t *testing.T, tp trace.TracerProvider) *transport.Client {
				return newDecodeClient(t, http.StatusNoContent, "", nil, transport.WithTracerProvider(tp))
			},
			req:      getEHR,
			wantSize: 0,
		},
		{
			name: "404 with an envelope",
			client: func(t *testing.T, tp trace.TracerProvider) *transport.Client {
				return newDecodeClient(t, http.StatusNotFound, notFoundBody, nil, transport.WithTracerProvider(tp))
			},
			req:      getEHR,
			wantErr:  true,
			wantSize: int64(len(notFoundBody)),
		},
		{
			name: "201 cut mid-body",
			client: func(t *testing.T, tp trace.TracerProvider) *transport.Client {
				return newBodyReadClient(t, cutBodyHandler(http.StatusCreated, committedHeader(), cutPartialBody, nil), transport.WithTracerProvider(tp))
			},
			req:     compositionPost,
			wantErr: true,
			noSize:  true,
		},
		{
			name: "404 cut mid-body",
			client: func(t *testing.T, tp trace.TracerProvider) *transport.Client {
				return newBodyReadClient(t, cutBodyHandler(http.StatusNotFound, nil, cutPartialBody, nil), transport.WithTracerProvider(tp))
			},
			req:     getEHR,
			wantErr: true,
			noSize:  true,
		},
		{
			name: "200 over the ceiling",
			client: func(t *testing.T, tp trace.TracerProvider) *transport.Client {
				return newDecodeClient(t, http.StatusOK, strings.Repeat(okBody, 2), nil, transport.WithMaxResponseBody(limit), transport.WithTracerProvider(tp))
			},
			req:     getEHR,
			wantErr: true,
			noSize:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tp := newRecordingTracerProvider()
			c := tc.client(t, tp)

			_, err := c.Do(t.Context(), tc.req())
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Fatalf("Do returned error %v, want an error: %v; the premise of this test is gone", err, tc.wantErr)
			}
			span := tp.tracer.only(t)
			got, ok := span.attr(responseSizeKey)
			if tc.noSize {
				if ok {
					t.Errorf("span %q %s = %v, want no size: the body could not be read", span.name, responseSizeKey, got.String())
				}
				return
			}
			if !ok {
				t.Fatalf("span %q carries no %s attribute, want %d", span.name, responseSizeKey, tc.wantSize)
			}
			if got.Type() != attribute.INT64 || got.AsInt64() != tc.wantSize {
				t.Errorf("span %q %s = %v (%s), want %d", span.name, responseSizeKey, got.String(), got.Type(), tc.wantSize)
			}
		})
	}
}

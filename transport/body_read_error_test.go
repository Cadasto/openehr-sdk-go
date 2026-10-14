package transport_test

// body_read_error_test.go — REQ-172 § Typed 2xx body-read failure.
//
// A response can arrive with its status line and headers and then fail while
// its body is read. These tests pin both arms of what Client.Do returns then:
// on a 2xx, a *transport.Response carrying the status, the headers and the
// parsed Metadata beside a *transport.BodyReadError; on any other status, an
// error that wraps the *transport.WireError the status classifies and, beside
// it, the read failure. They also pin the retry gate (a 2xx whose body was
// lost is sent again only for a safe method, and never once the caller's
// context has ended) and the value-free Error() text.
//
// External, like decode_error_test.go, so every assertion goes through the
// surface a consumer sees. The servers are real httptest servers, so the read
// failures are the ones net/http itself produces: a connection cut mid-body, a
// Content-Encoding the body does not match, and a body over the
// WithMaxResponseBody ceiling.

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const (
	// cutContentLength is the Content-Length a cut response announces. Every
	// partial body below is shorter, so the client reads to the cut and fails
	// with io.ErrUnexpectedEOF.
	cutContentLength = 100

	// bodyReadHeaderMarker is planted in response header values, and
	// bodyReadBodyMarker in the bytes written before a cut, so a leak of either
	// into an error string is unmistakable.
	bodyReadHeaderMarker = "HDR-VALUE-3b9a52"
	bodyReadBodyMarker   = "PARTIAL-BODY-7d41e9"

	// cutBodyETag is the version a committed write names in its ETag.
	cutBodyETag = "5ea3b1b0-7b8f-4f0c-9d3e-0c1f2a3b4c5d::repro.example::1"
)

// cutPartialBody is what a cut response writes before the connection drops:
// the start of a representation, carrying bodyReadBodyMarker.
const cutPartialBody = `{"_type":"COMPOSITION","name":{"value":"` + bodyReadBodyMarker

// cutBodyHandler answers every request with status and header, announces a
// Content-Length of cutContentLength, writes and flushes partial, and then
// aborts the connection mid-body. It counts each request in hits when hits is
// not nil.
func cutBodyHandler(status int, header http.Header, partial string, hits *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		for k, vs := range header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set("Content-Length", strconv.Itoa(cutContentLength))
		w.WriteHeader(status)
		_, _ = io.WriteString(w, partial)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// ErrAbortHandler closes the connection without the server logging a
		// stack trace, so the client sees fewer bytes than Content-Length.
		panic(http.ErrAbortHandler)
	}
}

// newBodyReadClient starts an httptest server running h and returns a client
// aimed at it. Extra options are appended after the client's own.
func newBodyReadClient(t *testing.T, h http.Handler, opts ...transport.Option) *transport.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := transport.New(newDecodeCatalog(t, srv), append([]transport.Option{transport.WithHTTPClient(srv.Client())}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// committedHeader is the header set of a committed write: the version id in
// the ETag and the new version's address in Location.
func committedHeader() http.Header {
	return http.Header{
		"ETag":     {`"` + cutBodyETag + `"`},
		"Location": {"/ehr/abc/composition/" + cutBodyETag},
	}
}

// compositionPost is a POST of a new Composition, the write the bug report
// was about.
func compositionPost() *transport.Request {
	return &transport.Request{
		Method: http.MethodPost,
		Path:   "/ehr/abc/composition",
		Route:  "/ehr/{ehr_id}/composition",
		Body:   []byte(`{"_type":"COMPOSITION"}`),
	}
}

// assertTwoXXResponseKept checks the Response half of REQ-172 § The 2xx arm:
// non-nil, the status, the headers and the parsed Metadata as for any 2xx, and
// an empty Body.
func assertTwoXXResponseKept(t *testing.T, resp *transport.Response, status int) {
	t.Helper()
	if resp == nil {
		t.Fatal("Do returned a nil *Response beside a body-read failure; REQ-172 requires the status and headers survive")
	}
	if resp.StatusCode != status {
		t.Errorf("Response.StatusCode = %d, want %d", resp.StatusCode, status)
	}
	if resp.Header == nil {
		t.Error("Response.Header = nil, want the response headers")
	}
	if resp.Metadata == nil {
		t.Fatal("Response.Metadata = nil, want the parsed headers as for any 2xx")
	}
	if len(resp.Body) != 0 {
		t.Errorf("Response.Body = %q, want empty: a partial body is not the representation", resp.Body)
	}
}

// TestBodyReadErrorCutTwoXXKeepsStatusAndHeaders pins REQ-172 § The 2xx arm on
// the reported case: a 201 whose body is cut mid-read. The status, the
// headers and the Metadata come back on a non-nil Response, and the error is a
// *BodyReadError that names the request and reaches io.ErrUnexpectedEOF.
func TestBodyReadErrorCutTwoXXKeepsStatusAndHeaders(t *testing.T) { // REQ-172
	c := newBodyReadClient(t, cutBodyHandler(http.StatusCreated, committedHeader(), cutPartialBody, nil))

	resp, err := c.Do(t.Context(), compositionPost())
	if err == nil {
		t.Fatal("Do of a 201 whose body is cut succeeded; the premise of this test is gone")
	}
	assertTwoXXResponseKept(t, resp, http.StatusCreated)
	if got := resp.Header.Get("ETag"); got != `"`+cutBodyETag+`"` {
		t.Errorf("Response.Header ETag = %q, want %q", got, `"`+cutBodyETag+`"`)
	}
	if got := resp.Metadata.ETag; got != cutBodyETag {
		t.Errorf("Metadata.ETag = %q, want %q: the ETag names the version the server made", got, cutBodyETag)
	}
	if want := "/ehr/abc/composition/" + cutBodyETag; resp.Metadata.Location != want {
		t.Errorf("Metadata.Location = %q, want %q", resp.Metadata.Location, want)
	}

	bre, ok := errors.AsType[*transport.BodyReadError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
	}
	if bre.StatusCode != http.StatusCreated {
		t.Errorf("BodyReadError.StatusCode = %d, want %d", bre.StatusCode, http.StatusCreated)
	}
	if bre.Method != http.MethodPost {
		t.Errorf("BodyReadError.Method = %q, want %q", bre.Method, http.MethodPost)
	}
	if want := "/ehr/{ehr_id}/composition"; bre.Route != want {
		t.Errorf("BodyReadError.Route = %q, want the route template %q", bre.Route, want)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("errors.Is(err, io.ErrUnexpectedEOF) = false for %v; the read failure must stay reachable", err)
	}
	if !errors.Is(bre.Unwrap(), io.ErrUnexpectedEOF) {
		t.Errorf("BodyReadError.Unwrap() = %v, want the read failure", bre.Unwrap())
	}

	// REQ-172 § Disjoint classifications: a body that could not be read is
	// neither a wire failure nor a decode failure.
	if we, ok := errors.AsType[*transport.WireError](err); ok {
		t.Errorf("errors.AsType[*transport.WireError] matched a 2xx body-read failure (%v)", we)
	}
	if de, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Errorf("errors.AsType[*transport.DecodeError] matched a 2xx body-read failure (%v)", de)
	}

	// The operation-name wrap a leaf package adds does not hide it.
	wrapped := fmt.Errorf("composition.Save: %w", err)
	if got, ok := errors.AsType[*transport.BodyReadError](wrapped); !ok || got != bre {
		t.Errorf("errors.AsType[*transport.BodyReadError] through a leaf wrap = (%v, %v), want the same error", got, ok)
	}
}

// TestBodyReadErrorContentCoding pins REQ-172's content-coding cause: a 2xx
// whose Content-Encoding says gzip over a plain body gives the same shape as a
// cut body, with errors.Is reaching gzip.ErrHeader.
func TestBodyReadErrorContentCoding(t *testing.T) { // REQ-172
	var acceptEncoding atomic.Value
	c := newBodyReadClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptEncoding.Store(r.Header.Get("Accept-Encoding"))
		w.Header().Set("ETag", `"`+cutBodyETag+`"`)
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = io.WriteString(w, "this body is plain text, not gzip at all")
	}))

	resp, err := c.Do(t.Context(), &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"})
	// net/http decompresses only a response to a request it added
	// Accept-Encoding: gzip to itself. Without that, the body would read
	// cleanly and this test would measure nothing.
	if got, _ := acceptEncoding.Load().(string); !strings.Contains(got, "gzip") {
		t.Fatalf("the request carried Accept-Encoding %q, want gzip added by net/http; the premise of this test is gone", got)
	}
	if err == nil {
		t.Fatal("Do of a gzip-labelled plain body succeeded; the premise of this test is gone")
	}
	assertTwoXXResponseKept(t, resp, http.StatusOK)
	if got := resp.Metadata.ETag; got != cutBodyETag {
		t.Errorf("Metadata.ETag = %q, want %q", got, cutBodyETag)
	}
	bre, ok := errors.AsType[*transport.BodyReadError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
	}
	if bre.StatusCode != http.StatusOK {
		t.Errorf("BodyReadError.StatusCode = %d, want %d", bre.StatusCode, http.StatusOK)
	}
	if !errors.Is(err, gzip.ErrHeader) {
		t.Errorf("errors.Is(err, gzip.ErrHeader) = false for %v; the content-coding error must stay reachable", err)
	}
}

// TestBodyReadErrorOverLimit pins REQ-172 on the REQ-093 ceiling: a 2xx body
// over WithMaxResponseBody gives the same shape, with errors.Is reaching
// transport.ErrResponseTooLarge and a message that names the limit.
func TestBodyReadErrorOverLimit(t *testing.T) { // REQ-172
	const limit = 16
	body := strings.Repeat(bodyReadBodyMarker, 4)
	c := newDecodeClient(t, http.StatusOK, body, http.Header{"ETag": {`"` + cutBodyETag + `"`}}, transport.WithMaxResponseBody(limit))

	resp, err := c.Do(t.Context(), &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"})
	if err == nil {
		t.Fatalf("Do of a %d-byte body under a %d-byte limit succeeded; the premise of this test is gone", len(body), limit)
	}
	assertTwoXXResponseKept(t, resp, http.StatusOK)
	if got := resp.Metadata.ETag; got != cutBodyETag {
		t.Errorf("Metadata.ETag = %q, want %q", got, cutBodyETag)
	}
	bre, ok := errors.AsType[*transport.BodyReadError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
	}
	if bre.StatusCode != http.StatusOK {
		t.Errorf("BodyReadError.StatusCode = %d, want %d", bre.StatusCode, http.StatusOK)
	}
	if !errors.Is(err, transport.ErrResponseTooLarge) {
		t.Errorf("errors.Is(err, transport.ErrResponseTooLarge) = false for %v", err)
	}
	if !strings.Contains(bre.Error(), strconv.Itoa(limit)) {
		t.Errorf("BodyReadError.Error() = %q, want it to name the limit %d (REQ-093)", bre.Error(), limit)
	}
	if strings.Contains(bre.Error(), bodyReadBodyMarker) {
		t.Errorf("BodyReadError.Error() = %q carries body bytes", bre.Error())
	}
}

// TestBodyReadErrorNonTwoXXStaysWireError pins REQ-172 § The non-2xx arm: a
// status that is not 2xx still classifies the failure as the *WireError its
// status and headers give, with no envelope and no RawBody even under
// WithRawErrorBodies, beside a non-nil Response with an empty Body. The
// returned error also wraps the read failure, and is never a *BodyReadError.
func TestBodyReadErrorNonTwoXXStaysWireError(t *testing.T) { // REQ-172
	// An envelope's start, so a RawBody built from the bytes before the cut
	// would show.
	const partialEnvelope = `{"code":"VERSION_CONFLICT","message":"` + bodyReadBodyMarker
	// A whole envelope one byte over the ceiling. The transport reads one
	// byte past the limit to detect the overflow, so it holds every byte of
	// this envelope, and an OpenEHR detail built from them would decode.
	const wholeEnvelope = `{"code":"NOT_FOUND"}`

	cases := []struct {
		name     string
		status   int
		header   http.Header
		handler  func(status int, header http.Header) http.Handler
		opts     []transport.Option
		sentinel error
		cause    error
		// limit, when not zero, is the WithMaxResponseBody ceiling the error
		// text must name (REQ-093).
		limit int
	}{
		{
			name:   "409 cut mid-body",
			status: http.StatusConflict,
			header: http.Header{"ETag": {`"` + cutBodyETag + `"`}},
			handler: func(status int, header http.Header) http.Handler {
				return cutBodyHandler(status, header, partialEnvelope, nil)
			},
			sentinel: transport.ErrVersionConflict,
			cause:    io.ErrUnexpectedEOF,
		},
		{
			name:   "404 over the ceiling",
			status: http.StatusNotFound,
			header: http.Header{"ETag": {`"` + cutBodyETag + `"`}},
			handler: func(status int, header http.Header) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					maps.Copy(w.Header(), header)
					w.WriteHeader(status)
					_, _ = io.WriteString(w, wholeEnvelope)
				})
			},
			opts:     []transport.Option{transport.WithMaxResponseBody(int64(len(wholeEnvelope) - 1))},
			sentinel: transport.ErrNotFound,
			cause:    transport.ErrResponseTooLarge,
			limit:    len(wholeEnvelope) - 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]transport.Option{transport.WithRawErrorBodies(true)}, tc.opts...)
			c := newBodyReadClient(t, tc.handler(tc.status, tc.header), opts...)

			resp, err := c.Do(t.Context(), &transport.Request{
				Method: http.MethodDelete,
				Path:   "/ehr/abc/composition/def",
				Route:  "/ehr/{ehr_id}/composition/{preceding_version_uid}",
			})
			if err == nil {
				t.Fatal("Do succeeded; the premise of this test is gone")
			}
			if resp == nil {
				t.Fatal("Do returned a nil *Response; REQ-172 requires the status and headers survive on the non-2xx arm too")
			}
			if resp.StatusCode != tc.status {
				t.Errorf("Response.StatusCode = %d, want %d", resp.StatusCode, tc.status)
			}
			if got := resp.Header.Get("ETag"); got != `"`+cutBodyETag+`"` {
				t.Errorf("Response.Header ETag = %q, want the response headers", got)
			}
			if len(resp.Body) != 0 {
				t.Errorf("Response.Body = %q, want empty", resp.Body)
			}

			if !errors.Is(err, tc.sentinel) {
				t.Errorf("errors.Is(err, %v) = false for %v; the status must still classify the failure", tc.sentinel, err)
			}
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok {
				t.Fatalf("errors.AsType[*transport.WireError] did not match %T (%v)", err, err)
			}
			if we.StatusCode != tc.status {
				t.Errorf("WireError.StatusCode = %d, want %d", we.StatusCode, tc.status)
			}
			if want := "/ehr/{ehr_id}/composition/{preceding_version_uid}"; we.Route != want {
				t.Errorf("WireError.Route = %q, want %q", we.Route, want)
			}
			if we.OpenEHR != nil {
				t.Errorf("WireError.OpenEHR = %+v, want nil: an unread body has no envelope", we.OpenEHR)
			}
			if len(we.RawBody) != 0 {
				t.Errorf("WireError.RawBody = %q, want empty even under WithRawErrorBodies", we.RawBody)
			}
			if !errors.Is(err, tc.cause) {
				t.Errorf("errors.Is(err, %v) = false for %v; the read failure must stay reachable", tc.cause, err)
			}
			if tc.limit != 0 && !strings.Contains(err.Error(), strconv.Itoa(tc.limit)) {
				t.Errorf("err.Error() = %q, want it to name the limit %d (REQ-093)", err.Error(), tc.limit)
			}

			// REQ-172 § Disjoint classifications.
			if bre, ok := errors.AsType[*transport.BodyReadError](err); ok {
				t.Errorf("errors.AsType[*transport.BodyReadError] matched a %d (%v); it belongs to the 2xx arm only", tc.status, bre)
			}
			if de, ok := errors.AsType[*transport.DecodeError](err); ok {
				t.Errorf("errors.AsType[*transport.DecodeError] matched a %d body-read failure (%v)", tc.status, de)
			}
		})
	}
}

// TestBodyReadErrorNonTwoXXKeepsBearerChallenge pins the REQ-166 part of
// REQ-172 § The non-2xx arm: a 401 whose body is cut still carries its Bearer
// challenge, and the opt-in 401 safety net (REQ-063) still reads it through
// the wrap, so Reauth runs and the request is sent once more.
func TestBodyReadErrorNonTwoXXKeepsBearerChallenge(t *testing.T) { // REQ-172
	var hits, reauths atomic.Int32
	header := http.Header{"WWW-Authenticate": {`Bearer error="invalid_token"`}}
	c := newBodyReadClient(t, cutBodyHandler(http.StatusUnauthorized, header, cutPartialBody, &hits),
		transport.WithReauthOn401(auth.ReautherFunc(func(context.Context) error {
			reauths.Add(1)
			return nil
		})),
	)

	resp, err := c.Do(t.Context(), &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"})
	if err == nil {
		t.Fatal("Do of a cut 401 succeeded; the premise of this test is gone")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Do returned Response %+v, want a non-nil 401", resp)
	}
	if !errors.Is(err, transport.ErrUnauthorized) {
		t.Errorf("errors.Is(err, transport.ErrUnauthorized) = false for %v", err)
	}
	we, ok := errors.AsType[*transport.WireError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.WireError] did not match %T (%v)", err, err)
	}
	if we.Challenge == nil || we.Challenge.Error != "invalid_token" {
		t.Errorf("WireError.Challenge = %+v, want the Bearer challenge with error invalid_token", we.Challenge)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("errors.Is(err, io.ErrUnexpectedEOF) = false for %v", err)
	}
	if got := reauths.Load(); got != 1 {
		t.Errorf("Reauth ran %d times, want 1: the 401 safety net must see the WireError through the wrap", got)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("server hit %d times, want 2: the request and its one retry after Reauth", got)
	}
}

// TestBodyReadErrorStringIsValueFree pins REQ-172 § Error() stays value-free:
// the text names the method, the route template and the status, and never a
// header value or a byte read before the cut. The non-2xx arm's text is held
// to the same rule.
func TestBodyReadErrorStringIsValueFree(t *testing.T) { // REQ-172
	header := http.Header{
		"ETag":     {`"` + bodyReadHeaderMarker + `"`},
		"Location": {"/ehr/abc/composition/" + bodyReadHeaderMarker},
		"X-Echo":   {bodyReadHeaderMarker},
	}
	assertNoMarkers := func(t *testing.T, what, got string) {
		t.Helper()
		if strings.Contains(got, bodyReadHeaderMarker) {
			t.Errorf("%s = %q carries a response header value", what, got)
		}
		if strings.Contains(got, bodyReadBodyMarker) {
			t.Errorf("%s = %q carries bytes read before the cut", what, got)
		}
	}

	t.Run("2xx", func(t *testing.T) {
		c := newBodyReadClient(t, cutBodyHandler(http.StatusCreated, header, cutPartialBody, nil))
		_, err := c.Do(t.Context(), compositionPost())
		bre, ok := errors.AsType[*transport.BodyReadError](err)
		if !ok {
			t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
		}
		msg := bre.Error()
		assertNoMarkers(t, "BodyReadError.Error()", msg)
		assertNoMarkers(t, "err.Error()", err.Error())
		for _, want := range []string{http.MethodPost, "/ehr/{ehr_id}/composition", "201"} {
			if !strings.Contains(msg, want) {
				t.Errorf("BodyReadError.Error() = %q, want it to name %q", msg, want)
			}
		}
	})

	t.Run("2xx unrouted", func(t *testing.T) {
		// REQ-093's placeholder rule: an unrouted request names the
		// placeholder, never the identifier-bearing path.
		c := newBodyReadClient(t, cutBodyHandler(http.StatusOK, header, cutPartialBody, nil))
		_, err := c.Do(t.Context(), requestWithUnroutedID(http.MethodGet))
		bre, ok := errors.AsType[*transport.BodyReadError](err)
		if !ok {
			t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
		}
		if bre.Route != unroutedPlaceholder {
			t.Errorf("BodyReadError.Route = %q, want the placeholder %q", bre.Route, unroutedPlaceholder)
		}
		assertValueFree(t, bre.Error())
		assertNoMarkers(t, "BodyReadError.Error()", bre.Error())
	})

	t.Run("non-2xx", func(t *testing.T) {
		c := newBodyReadClient(t, cutBodyHandler(http.StatusConflict, header, cutPartialBody, nil), transport.WithRawErrorBodies(true))
		_, err := c.Do(t.Context(), compositionPost())
		if _, ok := errors.AsType[*transport.WireError](err); !ok {
			t.Fatalf("errors.AsType[*transport.WireError] did not match %T (%v)", err, err)
		}
		assertNoMarkers(t, "err.Error()", err.Error())
	})
}

// trailerSecretMarker is the line a malformed chunked trailer carries. It has
// no colon, so net/http fails the body read on it and quotes it in the read
// failure's own text.
const trailerSecretMarker = "PATIENT-SECRET-4711"

// malformedTrailerHandler takes over the connection and writes a raw chunked
// response with status: one five-byte chunk, the last chunk, and then the
// trailer line trailerSecretMarker. It closes the connection after.
func malformedTrailerHandler(t *testing.T, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("the test server's ResponseWriter is not an http.Hijacker; the premise of this test is gone")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = fmt.Fprintf(buf, "HTTP/1.1 %d %s\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n%s\r\n\r\n",
			status, http.StatusText(status), trailerSecretMarker)
		_ = buf.Flush()
	}
}

// TestBodyReadErrorTrailerTextStaysOut pins REQ-172 § Error() stays value-free
// on the read failure's own text: net/http quotes a malformed chunked trailer
// line in its read error, so a server can put any value there. That value
// appears nowhere in the error text on either arm, and the read failure is
// still reached by unwrapping.
func TestBodyReadErrorTrailerTextStaysOut(t *testing.T) { // REQ-172
	req := func() *transport.Request {
		return &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"}
	}

	t.Run("2xx", func(t *testing.T) {
		c := newBodyReadClient(t, malformedTrailerHandler(t, http.StatusOK))
		resp, err := c.Do(t.Context(), req())
		bre, ok := errors.AsType[*transport.BodyReadError](err)
		if !ok {
			t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v); the premise of this test is gone", err, err)
		}
		if bre.Inner == nil || !strings.Contains(bre.Inner.Error(), trailerSecretMarker) {
			t.Fatalf("BodyReadError.Inner = %v, want net/http's read error quoting %q; the premise of this test is gone", bre.Inner, trailerSecretMarker)
		}
		assertTwoXXResponseKept(t, resp, http.StatusOK)

		for what, got := range map[string]string{"BodyReadError.Error()": bre.Error(), "err.Error()": err.Error()} {
			if strings.Contains(got, trailerSecretMarker) {
				t.Errorf("%s = %q carries the read failure's own text, which quotes the server's trailer", what, got)
			}
		}
		for _, want := range []string{http.MethodGet, "/ehr/{ehr_id}", "200"} {
			if !strings.Contains(bre.Error(), want) {
				t.Errorf("BodyReadError.Error() = %q, want it to name %q", bre.Error(), want)
			}
		}
		if !errors.Is(err, bre.Inner) {
			t.Errorf("errors.Is(err, %v) = false; the read failure must stay reachable by unwrapping", bre.Inner)
		}
	})

	t.Run("non-2xx", func(t *testing.T) {
		c := newBodyReadClient(t, malformedTrailerHandler(t, http.StatusConflict))
		resp, err := c.Do(t.Context(), req())
		we, ok := errors.AsType[*transport.WireError](err)
		if !ok {
			t.Fatalf("errors.AsType[*transport.WireError] did not match %T (%v); the premise of this test is gone", err, err)
		}
		cause, ok := errors.AsType[textproto.ProtocolError](err)
		if !ok || !strings.Contains(cause.Error(), trailerSecretMarker) {
			t.Fatalf("errors.AsType[textproto.ProtocolError] = (%v, %v), want net/http's read error quoting %q; the premise of this test is gone", cause, ok, trailerSecretMarker)
		}
		if resp == nil || resp.StatusCode != http.StatusConflict {
			t.Fatalf("Do returned Response %+v, want a non-nil 409", resp)
		}
		if !errors.Is(err, transport.ErrVersionConflict) {
			t.Errorf("errors.Is(err, transport.ErrVersionConflict) = false for %v", err)
		}
		if got := err.Error(); strings.Contains(got, trailerSecretMarker) {
			t.Errorf("err.Error() = %q carries the read failure's own text, which quotes the server's trailer", got)
		}
		if got := err.Error(); !strings.HasPrefix(got, we.Error()) {
			t.Errorf("err.Error() = %q, want it to start with the WireError's own text %q", got, we.Error())
		}
	})
}

// TestBodyReadErrorRetryGate pins REQ-172 § Retry. With retries enabled, a 2xx
// whose body could not be read is sent again only for GET, HEAD or OPTIONS:
// the server acted on any other method, so a second send would repeat it. The
// non-2xx arm keeps the status-based retry of any *WireError.
func TestBodyReadErrorRetryGate(t *testing.T) { // REQ-172
	const attempts = 3
	cases := []struct {
		name          string
		method        string
		status        int
		nonIdempotent bool
		wantHits      int32
	}{
		{name: "GET 200 is sent again", method: http.MethodGet, status: http.StatusOK, wantHits: attempts},
		{name: "OPTIONS 200 is sent again", method: http.MethodOptions, status: http.StatusOK, wantHits: attempts},
		{name: "PUT 200 is sent once", method: http.MethodPut, status: http.StatusOK, wantHits: 1},
		{name: "DELETE 200 is sent once", method: http.MethodDelete, status: http.StatusOK, wantHits: 1},
		{name: "POST 201 under RetryNonIdempotent is sent once", method: http.MethodPost, status: http.StatusCreated, nonIdempotent: true, wantHits: 1},
		{name: "PATCH 200 under RetryNonIdempotent is sent once", method: http.MethodPatch, status: http.StatusOK, nonIdempotent: true, wantHits: 1},
		{name: "GET 503 is retried by status", method: http.MethodGet, status: http.StatusServiceUnavailable, wantHits: attempts},
		{name: "PUT 503 is retried by status", method: http.MethodPut, status: http.StatusServiceUnavailable, wantHits: attempts},
		{name: "GET 404 is not retried by status", method: http.MethodGet, status: http.StatusNotFound, wantHits: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			c := newBodyReadClient(t, cutBodyHandler(tc.status, committedHeader(), cutPartialBody, &hits),
				transport.WithRetry(transport.RetryPolicy{
					MaxAttempts:        attempts,
					InitialBackoff:     time.Millisecond,
					MaxBackoff:         time.Millisecond,
					RetryNonIdempotent: tc.nonIdempotent,
				}),
			)
			var body []byte
			if tc.method != http.MethodGet && tc.method != http.MethodOptions {
				body = []byte(`{"_type":"COMPOSITION"}`)
			}

			resp, err := c.Do(t.Context(), &transport.Request{Method: tc.method, Path: "/ehr/abc/composition", Route: "/ehr/{ehr_id}/composition", Body: body})
			if err == nil {
				t.Fatal("Do of a cut response succeeded; the premise of this test is gone")
			}
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("errors.Is(err, io.ErrUnexpectedEOF) = false for %v", err)
			}
			if resp == nil || resp.StatusCode != tc.status {
				t.Errorf("Do returned Response %+v, want a non-nil %d", resp, tc.status)
			}
			if got := hits.Load(); got != tc.wantHits {
				t.Errorf("%s with a cut %d body hit the server %d times, want %d", tc.method, tc.status, got, tc.wantHits)
			}
		})
	}
}

// TestBodyReadErrorRetryGateHEAD pins HEAD in REQ-172 § Retry's list of safe
// methods. net/http never reads the body of a response to HEAD, so an
// httptest server cannot make that read fail; a RoundTripper hands back a 2xx
// whose Body fails instead. A PUT through the same RoundTripper is the
// contrast that shows the gate is what decides.
func TestBodyReadErrorRetryGateHEAD(t *testing.T) { // REQ-172
	const attempts = 3
	cases := []struct {
		method   string
		wantHits int
	}{
		{method: http.MethodHead, wantHits: attempts},
		{method: http.MethodPut, wantHits: 1},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			hits := 0
			c := newBrokenTransportClient(t, func(*http.Request) (*http.Response, error) {
				hits++
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Etag": {`"` + cutBodyETag + `"`}},
					Body:       errReadCloser{err: io.ErrUnexpectedEOF},
				}, nil
			}, transport.WithRetry(transport.RetryPolicy{
				MaxAttempts:    attempts,
				InitialBackoff: time.Millisecond,
				MaxBackoff:     time.Millisecond,
			}))

			resp, err := c.Do(t.Context(), &transport.Request{Method: tc.method, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"})
			if _, ok := errors.AsType[*transport.BodyReadError](err); !ok {
				t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v); the premise of this test is gone", err, err)
			}
			assertTwoXXResponseKept(t, resp, http.StatusOK)
			if hits != tc.wantHits {
				t.Errorf("%s with a 200 body that cannot be read reached the RoundTripper %d times, want %d", tc.method, hits, tc.wantHits)
			}
		})
	}
}

// TestBodyReadErrorCallerDeadlineIsNotRetried pins REQ-172 § Retry beside
// REQ-091's "Retries MUST respect ctx cancellation". A GET whose 200 body
// read is stopped by the caller's own deadline is not sent again, so Do
// returns the Response and the *BodyReadError rather than a later attempt's
// token or dial failure. Only the "no backoff" case pins the ctx guard in
// shouldRetry: with no wait, a retry would start at once and fail with no
// response. The "20ms backoff" case pins the outcome only, because Do's
// retry-wait branch keeps the *BodyReadError even when the guard is gone;
// keep both, and never drop the first as a duplicate of the second.
func TestBodyReadErrorCallerDeadlineIsNotRetried(t *testing.T) { // REQ-172, REQ-091
	cases := []struct {
		name    string
		backoff time.Duration
	}{
		{name: "no backoff", backoff: 0},
		{name: "20ms backoff", backoff: 20 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			release := make(chan struct{})
			c := newBodyReadClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				w.Header().Set("ETag", `"`+cutBodyETag+`"`)
				w.Header().Set("Content-Length", strconv.Itoa(cutContentLength))
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, cutPartialBody)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				// Stall mid-body until the client gives up or the test ends.
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}), transport.WithRetry(transport.RetryPolicy{
				MaxAttempts:    3,
				InitialBackoff: tc.backoff,
				MaxBackoff:     tc.backoff,
			}))
			// Cleanups run last-in first-out, so the handler is released
			// before the server closes and waits for it.
			t.Cleanup(func() { close(release) })

			ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
			defer cancel()
			resp, err := c.Do(ctx, &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"})
			if err == nil {
				t.Fatal("Do of a stalled 200 under a deadline succeeded; the premise of this test is gone")
			}
			if resp == nil {
				t.Fatalf("Do returned a nil *Response beside %v; REQ-172 requires the status and headers survive", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Response.StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			if resp.Metadata == nil || resp.Metadata.ETag != cutBodyETag {
				t.Errorf("Response.Metadata = %+v, want the ETag %q", resp.Metadata, cutBodyETag)
			}
			if _, ok := errors.AsType[*transport.BodyReadError](err); !ok {
				t.Errorf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("errors.Is(err, context.DeadlineExceeded) = false for %v", err)
			}
			if got := hits.Load(); got != 1 {
				t.Errorf("server hit %d times, want 1: a read the caller's deadline stopped is not sent again", got)
			}
		})
	}
}

// TestBodyReadErrorSurvivesCancelledBackoff pins the other half of the same
// rule: a GET whose 200 body could not be read is due to be sent again, and the
// caller's deadline ends during the backoff. Do then returns the Response and
// an error that still carries the *BodyReadError, with the context's error
// reachable beside it. The fake clock of testing/synctest runs the hour-long
// backoff past the deadline at once.
func TestBodyReadErrorSurvivesCancelledBackoff(t *testing.T) { // REQ-172, REQ-091
	synctest.Test(t, func(t *testing.T) {
		hits := 0
		c := newBrokenTransportClient(t, func(*http.Request) (*http.Response, error) {
			hits++
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Etag": {`"` + cutBodyETag + `"`}},
				Body:       errReadCloser{err: io.ErrUnexpectedEOF},
			}, nil
		}, transport.WithRetry(transport.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: time.Hour,
			MaxBackoff:     time.Hour,
		}))

		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		resp, err := c.Do(ctx, &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"})
		if hits != 1 {
			t.Fatalf("RoundTripper reached %d times, want 1: the deadline ends during the first backoff; the premise of this test is gone", hits)
		}
		assertTwoXXResponseKept(t, resp, http.StatusOK)
		if resp.Metadata.ETag != cutBodyETag {
			t.Errorf("Metadata.ETag = %q, want %q", resp.Metadata.ETag, cutBodyETag)
		}
		if _, ok := errors.AsType[*transport.BodyReadError](err); !ok {
			t.Errorf("errors.AsType[*transport.BodyReadError] did not match %T (%v); the backoff's failure must not replace it", err, err)
		}
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("errors.Is(err, io.ErrUnexpectedEOF) = false for %v", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("errors.Is(err, context.DeadlineExceeded) = false for %v; the context's error stays reachable", err)
		}
	})
}

// TestBodyReadErrorObservation pins REQ-172's REQ-098 sentence: the
// observation of a call whose 2xx body could not be read carries the response
// status beside the error.
func TestBodyReadErrorObservation(t *testing.T) { // REQ-172
	obs := &recordingObserver{}
	c := newBodyReadClient(t, cutBodyHandler(http.StatusCreated, committedHeader(), cutPartialBody, nil), transport.WithObserver(obs))

	_, err := c.Do(t.Context(), compositionPost())
	if err == nil {
		t.Fatal("Do of a cut 201 succeeded; the premise of this test is gone")
	}
	if obs.last.StatusCode != http.StatusCreated {
		t.Errorf("Observation.StatusCode = %d, want %d", obs.last.StatusCode, http.StatusCreated)
	}
	if _, ok := errors.AsType[*transport.BodyReadError](obs.last.Err); !ok {
		t.Errorf("Observation.Err = %v, want the *BodyReadError", obs.last.Err)
	}
}

// TestBodyReadErrorDisjointFromDecode pins the remaining directions of REQ-172
// § Disjoint classifications through transport.Decode: a 2xx body that was
// read but does not decode is a *DecodeError and never a *BodyReadError; a
// 2xx body that could not be read is a *BodyReadError and never a
// *DecodeError, with the Metadata still beside it; and a non-2xx whose body
// was read is a *WireError and never a *BodyReadError.
func TestBodyReadErrorDisjointFromDecode(t *testing.T) { // REQ-172
	req := func() *transport.Request {
		return &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"}
	}

	t.Run("read but undecodable", func(t *testing.T) {
		c := newDecodeClient(t, http.StatusOK, `[1, 2, 3]`, nil)
		_, _, err := transport.Decode[decodeFake](t.Context(), c, req())
		if _, ok := errors.AsType[*transport.DecodeError](err); !ok {
			t.Fatalf("errors.AsType[*transport.DecodeError] did not match %T (%v); the premise of this test is gone", err, err)
		}
		if bre, ok := errors.AsType[*transport.BodyReadError](err); ok {
			t.Errorf("errors.AsType[*transport.BodyReadError] matched a decode failure (%v)", bre)
		}
	})

	t.Run("not read", func(t *testing.T) {
		c := newBodyReadClient(t, cutBodyHandler(http.StatusOK, committedHeader(), cutPartialBody, nil))
		out, meta, err := transport.Decode[decodeFake](t.Context(), c, req())
		if out != nil {
			t.Errorf("Decode returned %+v beside a body-read failure, want nil", out)
		}
		if _, ok := errors.AsType[*transport.BodyReadError](err); !ok {
			t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
		}
		if de, ok := errors.AsType[*transport.DecodeError](err); ok {
			t.Errorf("errors.AsType[*transport.DecodeError] matched a body that was never read (%v)", de)
		}
		if meta == nil || meta.ETag != cutBodyETag {
			t.Errorf("Decode returned Metadata %+v, want the ETag %q beside the error", meta, cutBodyETag)
		}
	})

	t.Run("non-2xx read in full", func(t *testing.T) {
		c := newDecodeClient(t, http.StatusConflict, `{"code":"VERSION_CONFLICT"}`, nil)
		_, err := c.Do(t.Context(), req())
		if _, ok := errors.AsType[*transport.WireError](err); !ok {
			t.Fatalf("errors.AsType[*transport.WireError] did not match %T (%v); the premise of this test is gone", err, err)
		}
		if bre, ok := errors.AsType[*transport.BodyReadError](err); ok {
			t.Errorf("errors.AsType[*transport.BodyReadError] matched a wire failure (%v)", bre)
		}
	})
}

// TestBodyReadErrorNilReceiver pins the REQ-025 nil-receiver axis for the new
// exported type: the typed nil a failed errors.As leaves behind answers every
// method instead of panicking.
func TestBodyReadErrorNilReceiver(t *testing.T) { // REQ-172, REQ-025
	var e *transport.BodyReadError

	t.Run("Error", func(t *testing.T) {
		got := e.Error()
		if got == "" {
			t.Fatal(`nil.Error() = "", want a non-empty string`)
		}
		if strings.Contains(got, "%!") {
			t.Errorf("nil.Error() = %q, which carries an fmt verb failure", got)
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		if got := e.Unwrap(); got != nil {
			t.Errorf("nil.Unwrap() = %v, want nil", got)
		}
	})

	t.Run("boxed", func(t *testing.T) {
		boxed := error(e)
		if boxed.Error() == "" {
			t.Error(`Error() on a boxed nil BodyReadError = "", want a non-empty string`)
		}
		if got := errors.Unwrap(boxed); got != nil {
			t.Errorf("errors.Unwrap(boxed nil BodyReadError) = %v, want nil", got)
		}
		if errors.Is(boxed, io.ErrUnexpectedEOF) {
			t.Error("errors.Is(boxed nil BodyReadError, io.ErrUnexpectedEOF) = true, want false")
		}
	})
}

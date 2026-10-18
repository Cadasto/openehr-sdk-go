package transport_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// benchRequest is a routed GET of one composition version, the shape of the
// most common leaf read.
var benchRequest = transport.Request{
	Method: http.MethodGet,
	Route:  "/ehr/{ehr_id}/composition/{version_uid}",
	Path:   "/ehr/7d44b88c-4199-4bad-97dc-d78268e01398/composition/8849182c-82ad-4088-a07f-48ead4180515::openEHRSys.example.com::1",
}

// benchClient starts an httptest server that answers every request with a
// size-byte 200 body, a Content-Length and the headers parseMetadata reads,
// and returns a Client for it with a static bearer token.
func benchClient(b *testing.B, size int, opts ...transport.Option) *transport.Client {
	b.Helper()
	body := bytes.Repeat([]byte("a"), size)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "application/json")
		h.Set("ETag", `"8849182c-82ad-4088-a07f-48ead4180515::openEHRSys.example.com::1"`)
		h.Set("Location", "https://cdr.example/openehr/v1/ehr/7d44b88c-4199-4bad-97dc-d78268e01398/composition/8849182c-82ad-4088-a07f-48ead4180515::openEHRSys.example.com::1")
		h.Set("Last-Modified", "Wed, 15 Oct 2026 10:00:00 GMT")
		h.Set("openehr-version", `lifecycle_state.code_string="532"`)
		h.Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	b.Cleanup(srv.Close)
	// Enough idle connections that the parallel run reuses them instead of
	// dialling a new one per request.
	tr := srv.Client().Transport.(*http.Transport).Clone()
	tr.MaxIdleConnsPerHost = 256
	b.Cleanup(tr.CloseIdleConnections)

	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://idp.example",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {BaseURL: discovery.MustParseURL(srv.URL + "/openehr/v1"), SpecVersion: discovery.SpecVersionPin},
		},
	})
	if err != nil {
		b.Fatalf("NewStaticCatalog: %v", err)
	}
	all := append([]transport.Option{
		transport.WithHTTPClient(&http.Client{Transport: tr}),
		transport.WithTokenSource(auth.StaticTokenSource(auth.Token{Value: "tok-abc"})),
	}, opts...)
	c, err := transport.New(cat, all...)
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	return c
}

// BenchmarkDo measures one routed GET through Client.Do against a local
// server, body read and metadata parsed: a 2 KiB and a 256 KiB body, a
// 2 KiB body with client-default caller attribution, and the 2 KiB GET from
// many goroutines at once. The server runs in the same process, so its
// allocations are counted too.
func BenchmarkDo(b *testing.B) {
	attribution := transport.WithCallerAttribution(transport.CallerAttribution{
		AgentID:       "mcp-claude-code/1.2.0",
		ModelProvider: "anthropic",
		Attributes:    map[string]string{"orchestrator": "ralph-loop"},
	})
	for _, bc := range []struct {
		name string
		size int
		opts []transport.Option
	}{
		{name: "2KiB", size: 2 << 10},
		{name: "256KiB", size: 256 << 10},
		{name: "2KiB_attribution", size: 2 << 10, opts: []transport.Option{attribution}},
	} {
		b.Run(bc.name, func(b *testing.B) {
			c := benchClient(b, bc.size, bc.opts...)
			ctx := b.Context()
			b.ReportAllocs()
			for b.Loop() {
				req := benchRequest
				if _, err := c.Do(ctx, &req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	b.Run("2KiB_parallel", func(b *testing.B) {
		c := benchClient(b, 2<<10)
		ctx := b.Context()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				req := benchRequest
				if _, err := c.Do(ctx, &req); err != nil {
					b.Error(err)
					return
				}
			}
		})
	})
}

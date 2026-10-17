package transport_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestREQ066_CallerAttributionDefaultIsCopiedAtNew pins that New keeps its
// own copy of the client-default attribution: a change the caller makes to
// its Attributes map afterwards reaches neither the header nor the span, so
// it cannot race with a request either (REQ-066).
func TestREQ066_CallerAttributionDefaultIsCopiedAtNew(t *testing.T) {
	t.Parallel()
	var (
		mu  sync.Mutex
		got string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = r.Header.Get(transport.DefaultCallerAttributionHeader)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://idp.example",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {BaseURL: discovery.MustParseURL(srv.URL + "/openehr/v1"), SpecVersion: discovery.SpecVersionPin},
		},
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	tp := newRecordingTracerProvider()
	attrs := map[string]string{"orchestrator": "ralph-loop"}
	c, err := transport.New(cat,
		transport.WithHTTPClient(srv.Client()),
		transport.WithTracerProvider(tp),
		transport.WithCallerAttribution(transport.CallerAttribution{AgentID: "agent/1.0", Attributes: attrs}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	attrs["orchestrator"] = "changed"
	attrs["added"] = "later"

	if _, err := c.Do(t.Context(), &transport.Request{Method: http.MethodGet, Path: "/x", Route: "/x"}); err != nil {
		t.Fatalf("Do: %v", err)
	}

	mu.Lock()
	header := got
	mu.Unlock()
	var wire struct {
		AgentID    string            `json:"agent_id"`
		Attributes map[string]string `json:"attributes"`
	}
	if err := json.Unmarshal([]byte(header), &wire); err != nil {
		t.Fatalf("attribution header %q does not decode: %v", header, err)
	}
	want := map[string]string{"orchestrator": "ralph-loop"}
	if wire.AgentID != "agent/1.0" || !maps.Equal(wire.Attributes, want) {
		t.Errorf("attribution header = %q, want agent_id %q and attributes %v", header, "agent/1.0", want)
	}
	span := tp.tracer.only(t)
	if v, ok := span.attr("caller.orchestrator"); !ok || v.AsString() != "ralph-loop" {
		t.Errorf("span caller.orchestrator = %v (set %v), want %q", v.AsString(), ok, "ralph-loop")
	}
	if v, ok := span.attr("caller.added"); ok {
		t.Errorf("span caller.added = %q, want no such attribute: it was added after New", v.AsString())
	}
}

package discovery_test

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// countingTransport fails every request and counts them.
type countingTransport struct{ calls atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errors.New("the transport the caller set after NewResolver")
}

// TestREQ021_LaterChangeToTheInjectedClientReachesNoMode pins REQ-021 for
// the discovery resolver: it fetches with its own copy of the injected
// client, with and without WithAllowInsecure, so a change the caller makes to
// that client after NewResolver reaches neither mode. Each resolver resolves
// after the caller has replaced its client's Transport with one that fails.
func TestREQ021_LaterChangeToTheInjectedClientReachesNoMode(t *testing.T) { // REQ-021
	tests := []struct {
		name      string
		plaintext bool
		opts      []discovery.Option
	}{
		{name: "https"},
		{name: "WithAllowInsecure", plaintext: true, opts: []discovery.Option{discovery.WithAllowInsecure()}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := startPlatform(t, tc.plaintext, serve(func(string) string { return serviceDocument(nil) }), notFound)
			injected := &http.Client{Transport: p.srv.Client().Transport}
			res, err := discovery.NewResolver(nil, append([]discovery.Option{discovery.WithHTTPClient(injected)}, tc.opts...)...)
			if err != nil {
				t.Fatal(err)
			}
			later := &countingTransport{}
			injected.Transport = later

			if _, err := res.Resolve(t.Context(), p.baseURL()); err != nil {
				t.Errorf("Resolve after the caller changed its client = %v, want the catalog fetched with the client as injected", err)
			}
			if n := later.calls.Load(); n != 0 {
				t.Errorf("the Transport the caller set after NewResolver served %d request(s), want 0", n)
			}
		})
	}
}

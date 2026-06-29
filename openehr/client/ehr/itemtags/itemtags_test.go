package itemtags_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/itemtags"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func newClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGetCompositionTags(t *testing.T) {
	const tagHeader = `key="category",value="final"`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("openehr-item-tag", tagHeader)
		w.Header().Set("ETag", `"v-1"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"_type":"COMPOSITION","name":{}}`))
	}))
	defer srv.Close()

	tags, _, err := itemtags.GetComposition(t.Context(), newClient(t, srv),
		"ehr-1", openehrclient.VersionOf("uid::sys::1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tags.Object) != 1 || tags.Object[0].Key != "category" {
		t.Fatalf("tags = %#v", tags.Object)
	}
}

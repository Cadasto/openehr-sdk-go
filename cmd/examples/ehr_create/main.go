// Example: end-to-end EHR creation against an in-process httptest
// server. Spins up a fake openEHR REST backend, wires a transport
// client via a static service catalog, and calls
// `openehr/client/ehr.Create`. Demonstrates the smallest REST path:
// discovery → transport → typed leaf client.
//
// Run: `go run ./cmd/examples/ehr_create` from any directory.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func main() {
	const assignedEHRID = "f0e1d2c3-b4a5-6789-0123-456789abcdef"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/openehr/v1/ehr" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/openehr/v1/ehr/"+assignedEHRID)
		w.WriteHeader(http.StatusCreated)
		// Prefer=representation is the default → server returns the EHR.
		// Minimal valid EHR shape — ehr_id wired so the client surfaces it.
		_, _ = fmt.Fprintf(w, `{"_type":"EHR","ehr_id":{"_type":"HIER_OBJECT_ID","value":%q},"system_id":{"_type":"HIER_OBJECT_ID","value":"example.system"},"time_created":{"_type":"DV_DATE_TIME","value":"2026-05-17T12:00:00Z"}}`, assignedEHRID)
	}))
	defer srv.Close()

	c := mustClient(srv)

	ehr, meta, err := openehrclient.Create(context.Background(), c)
	if err != nil {
		log.Fatalf("ehr.Create: %v", err)
	}
	fmt.Printf("created EHR: id=%s\n", ehr.EHRID.Value)
	fmt.Printf("  system_id=%s\n", ehr.SystemID.Value)
	if meta != nil {
		fmt.Printf("  metadata: VersionUID=%q Location=%q\n", meta.VersionUID, locationOf(meta))
	}
	fmt.Println("OK: end-to-end EHR creation against in-process httptest backend")
}

func mustClient(srv *httptest.Server) *transport.Client {
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://example.test",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		log.Fatalf("build static catalog: %v", err)
	}
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		log.Fatalf("transport.New: %v", err)
	}
	return c
}

// locationOf extracts the Location header from the version metadata
// if present. Kept as a tiny helper so the example reads cleanly.
func locationOf(meta *openehrclient.VersionMetadata) string {
	if meta == nil || meta.Metadata == nil {
		return ""
	}
	return meta.Location
}

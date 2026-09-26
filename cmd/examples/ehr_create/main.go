// Create an EHR through the SDK's REST client path. Three layers take part: a
// service catalog says where the openEHR REST API lives, a transport client
// carries the injected *http.Client, and the typed ehr.Create call sends
// POST /ehr and decodes the answer. Every other REST call in the SDK is wired
// the same way.
//
// It runs offline: a throwaway httptest server plays the openEHR backend and
// answers the one request the example makes.
//
//	go run ./cmd/examples/ehr_create
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// assignedEHRID is the id the fake backend hands out, so the printed output
// is the same on every run.
const assignedEHRID = "f0e1d2c3-b4a5-6789-0123-456789abcdef"

// ehrBodyFormat is the smallest EHR the SDK decodes: ehr_id, system_id and
// time_created. The %q slot receives the assigned id.
const ehrBodyFormat = `{
  "_type": "EHR",
  "ehr_id": {"_type": "HIER_OBJECT_ID", "value": %q},
  "system_id": {"_type": "HIER_OBJECT_ID", "value": "example.system"},
  "time_created": {"_type": "DV_DATE_TIME", "value": "2026-05-17T12:00:00Z"}
}`

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Step 1: a stand-in for a real clinical data repository (CDR). Point the
	// catalog at your deployment instead and everything below stays the same.
	backend := httptest.NewServer(http.HandlerFunc(handleCreateEHR))
	defer backend.Close()

	// Step 2: wire the client. The catalog and the transport are the two
	// pieces every REST call in the SDK goes through.
	client, err := newClient(backend)
	if err != nil {
		return err
	}

	// Step 3: the typed leaf call. POST /ehr with no body asks the server to
	// create an EHR with a default EHR_STATUS. The SDK decodes the returned
	// EHR and collects the response headers (Location, ETag, ...) into meta.
	created, meta, err := ehr.Create(context.Background(), client)
	if err != nil {
		return fmt.Errorf("ehr.Create: %w", err)
	}

	fmt.Printf("created EHR: id=%s\n", created.EHRID.Value)
	fmt.Printf("  system_id=%s\n", created.SystemID.Value)
	if meta != nil {
		// Location is the new resource's path as the server sent it.
		// VersionUID is its last path segment, which for an EHR is the ehr_id;
		// for versioned resources such as compositions it is the version uid.
		fmt.Printf("  metadata: VersionUID=%q Location=%q\n", meta.VersionUID, meta.Location)
	}
	fmt.Println("OK: end-to-end EHR creation against in-process httptest backend")
	return nil
}

// newClient wires the two layers every REST call goes through. The service
// catalog answers "where is the openEHR REST API?"; a static one is enough for
// a fixed deployment, and discovery can also fetch it from a SMART issuer. The
// transport client owns the HTTP plumbing and never allocates its own
// *http.Client: you inject one, so connection pooling, TLS and timeouts stay
// under your control.
func newClient(backend *httptest.Server) (*transport.Client, error) {
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://example.test",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				// The base URL is what "/ehr", "/query", ... are joined onto.
				BaseURL:     discovery.MustParseURL(backend.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build static catalog: %w", err)
	}
	client, err := transport.New(catalog, transport.WithHTTPClient(backend.Client()))
	if err != nil {
		return nil, fmt.Errorf("transport.New: %w", err)
	}
	return client, nil
}

// handleCreateEHR is the whole fake backend. It accepts POST /openehr/v1/ehr
// and answers the way an openEHR server does: 201 Created, a Location header
// for the new EHR, and the EHR itself in the body. The body is there because
// ehr.Create sends Prefer: return=representation by default.
func handleCreateEHR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/openehr/v1/ehr" {
		http.Error(w, "unexpected request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/openehr/v1/ehr/"+assignedEHRID)
	w.WriteHeader(http.StatusCreated)
	// A write failure here would surface as a decode error on the client side.
	_, _ = fmt.Fprintf(w, ehrBodyFormat, assignedEHRID)
}

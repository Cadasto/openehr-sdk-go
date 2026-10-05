package composition_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// compositionJSON is a small COMPOSITION in canonical JSON: the attributes
// the reference model requires, and no content.
const compositionJSON = `{
  "_type": "COMPOSITION",
  "name": {"_type": "DV_TEXT", "value": "Encounter"},
  "archetype_node_id": "openEHR-EHR-COMPOSITION.encounter.v1",
  "archetype_details": {
    "archetype_id": {"value": "openEHR-EHR-COMPOSITION.encounter.v1"},
    "template_id": {"value": "example.encounter.v1"},
    "rm_version": "1.1.0"
  },
  "language": {"terminology_id": {"value": "ISO_639-1"}, "code_string": "en"},
  "territory": {"terminology_id": {"value": "ISO_3166-1"}, "code_string": "NL"},
  "category": {"value": "event", "defining_code": {"terminology_id": {"value": "openehr"}, "code_string": "433"}},
  "composer": {"_type": "PARTY_SELF"}
}`

// Save a composition with the default Prefer: return=minimal. The server
// sends no body, so the returned composition is nil, and the caller works
// with the metadata: the new version uid, and the Location and ETag headers.
func ExampleSave() {
	const (
		ehrID      = "7d44b88c-4199-4bad-97dc-d78268e01398"
		versionUID = "8849182c-82ad-4088-a07f-48ead4180515::sandbox.local::1"
	)

	// A fake CDR, run in-process by the sandbox package. It answers the
	// create the way the openEHR REST API does for return=minimal: 201
	// Created, an empty body, and the new version in ETag and Location.
	backend := sandbox.New()
	backend.HandleFunc(http.MethodPost, "/ehr/"+ehrID+"/composition", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"`+versionUID+`"`)
		w.Header().Set("Location", "https://sandbox.local/openehr/v1/ehr/"+ehrID+"/composition/"+versionUID)
		w.WriteHeader(http.StatusCreated)
	})

	// The catalog says where the openEHR REST API lives; the transport
	// carries your *http.Client, here the one that reaches the fake.
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://sandbox.local",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL("https://sandbox.local/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	client, err := transport.New(catalog, transport.WithHTTPClient(backend.HTTPClient()))
	if err != nil {
		log.Fatal(err)
	}

	comp := new(rm.Composition)
	if err := canjson.Unmarshal([]byte(compositionJSON), comp); err != nil {
		log.Fatal(err)
	}

	saved, meta, err := composition.Save(context.Background(), client, ehrID, comp)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("composition returned:", saved != nil)
	fmt.Println("VersionUID:", meta.VersionUID)
	fmt.Println("Location:", meta.Location)
	fmt.Println("ETag:", meta.ETag)
	// Output:
	// composition returned: false
	// VersionUID: 8849182c-82ad-4088-a07f-48ead4180515::sandbox.local::1
	// Location: https://sandbox.local/openehr/v1/ehr/7d44b88c-4199-4bad-97dc-d78268e01398/composition/8849182c-82ad-4088-a07f-48ead4180515::sandbox.local::1
	// ETag: 8849182c-82ad-4088-a07f-48ead4180515::sandbox.local::1
}

// Update a composition with the version uid you last read in If-Match. The
// server applies the update only while that version is still the latest.
// A stale If-Match is refused with 412 Precondition Failed, which Update
// reports as transport.ErrPreconditionFailed.
func ExampleUpdate() {
	const (
		ehrID = "7d44b88c-4199-4bad-97dc-d78268e01398"
		voID  = "8849182c-82ad-4088-a07f-48ead4180515"
	)
	versionUID := func(n int) string { return voID + "::sandbox.local::" + strconv.Itoa(n) }

	// A fake CDR that holds version 1. It applies an update only when
	// If-Match names the latest version, and answers 412 otherwise. Either
	// way its ETag names the latest version.
	latest := 1
	backend := sandbox.New()
	backend.HandleFunc(http.MethodPut, "/ehr/"+ehrID+"/composition/"+voID, func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusPreconditionFailed
		if r.Header.Get("If-Match") == `"`+versionUID(latest)+`"` {
			latest++
			status = http.StatusNoContent
		}
		w.Header().Set("ETag", `"`+versionUID(latest)+`"`)
		w.WriteHeader(status)
	})

	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://sandbox.local",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL("https://sandbox.local/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	client, err := transport.New(catalog, transport.WithHTTPClient(backend.HTTPClient()))
	if err != nil {
		log.Fatal(err)
	}

	comp := new(rm.Composition)
	if err := canjson.Unmarshal([]byte(compositionJSON), comp); err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	// If-Match carries version 1, the latest, so the update is applied.
	_, updated, err := composition.Update(ctx, client, ehrID, voID, versionUID(1), comp)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("updated, new version:", updated.VersionUID)

	// A second update that still names version 1 is stale. The metadata
	// returned beside the error names the latest version: read that version
	// again, reapply your change, and update with its uid.
	_, current, err := composition.Update(ctx, client, ehrID, voID, versionUID(1), comp)
	switch {
	case errors.Is(err, transport.ErrPreconditionFailed):
		fmt.Println("stale update refused, latest version:", current.VersionUID)
	case err != nil:
		log.Fatal(err)
	default:
		log.Fatal("the stale update was accepted")
	}
	// Output:
	// updated, new version: 8849182c-82ad-4088-a07f-48ead4180515::sandbox.local::2
	// stale update refused, latest version: 8849182c-82ad-4088-a07f-48ead4180515::sandbox.local::2
}

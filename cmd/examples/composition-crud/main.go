// Save, read and update a COMPOSITION through the SDK's REST client, and
// handle the refusal a stale update meets. The steps follow the openEHR
// optimistic-concurrency loop: save a first version, read the latest version
// together with its version uid, update with that uid in If-Match, then send a
// second update that still carries the old uid and classify the error.
//
// It runs offline: an in-process fake CDR (fake_cdr.go) answers the three
// composition routes. It is plugged into the *http.Client as its transport, so
// no listener is opened and the host in the base URL is never dialled.
//
//	go run ./cmd/examples/composition-crud
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// baseURL is where the openEHR REST API lives. The fake answers it in-process.
const baseURL = "https://sandbox.local/openehr/v1"

// exampleEHRID is the EHR the composition goes into. In your app it comes from
// ehr.Create or from a lookup by subject.
const exampleEHRID ehr.EHRID = "5a8c6f2e-1b3d-4c7e-9f0a-2d4b6c8e0f1a"

// weightPath is the openEHR path of the weight value in the body_weight
// template. The update corrects the value found there.
const weightPath = "/content[openEHR-EHR-OBSERVATION.body_weight.v2]/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Step 1: one deadline for the whole exchange, and an HTTP client with a
	// timeout of its own for each request. The SDK never builds an
	// *http.Client: you inject it, here with the fake CDR as its transport.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpClient := newFakeCDR(baseURL, string(exampleEHRID)).httpClient()
	httpClient.Timeout = 5 * time.Second
	client, err := newClient(httpClient)
	if err != nil {
		return err
	}
	comp, err := loadComposition()
	if err != nil {
		return err
	}

	// Step 2: save the first version. The default Prefer is return=minimal,
	// so the server sends no body and the returned composition is nil. What
	// the caller gets is the response metadata: the new version uid, taken
	// from the ETag, plus the Location and ETag headers themselves.
	saved, created, err := composition.Save(ctx, client, exampleEHRID, comp)
	if err != nil {
		return fmt.Errorf("composition.Save: %w", err)
	}
	if created == nil || created.VersionUID == "" {
		return errors.New("composition.Save: the server named no version")
	}
	fmt.Printf("save: composition returned=%t (Prefer: return=minimal)\n", saved != nil)
	fmt.Printf("  VersionUID=%s\n", created.VersionUID)
	fmt.Printf("  Location=%s\n", created.Location)
	fmt.Printf("  ETag=%s\n", created.ETag)

	// Step 3: read the latest version. A version uid is
	// "<versioned object id>::<system id>::<version>"; the versioned object
	// id names the whole family, and LatestOf asks for its newest version.
	voID := created.VersionUID.VersionedObjectID()
	latest, current, err := composition.Get(ctx, client, exampleEHRID, ehr.LatestOf(voID))
	if err != nil {
		return fmt.Errorf("composition.Get: %w", err)
	}
	fmt.Printf("get: latest of %s is %s\n", voID, latest.ArchetypeNodeID)
	fmt.Printf("  VersionUID=%s\n", current.VersionUID)

	// Step 4: change one value and update. If-Match carries the version uid
	// just read: the server applies the update only while that version is
	// still the latest. return=representation asks for the stored
	// composition back.
	if err := correctWeight(latest, 72.5); err != nil {
		return err
	}
	updated, next, err := composition.Update(ctx, client, exampleEHRID, voID,
		current.VersionUID.String(), latest, composition.WithPrefer(transport.PreferRepresentation))
	if err != nil {
		return fmt.Errorf("composition.Update: %w", err)
	}
	fmt.Printf("update: composition returned=%t (Prefer: return=representation)\n", updated != nil)
	fmt.Printf("  VersionUID=%s\n", next.VersionUID)

	// Step 5: a stale update, as sent by a second user who read version 1
	// before the update above. The server refuses it with 412 Precondition
	// Failed and names its latest version in the ETag. Update maps 412 to
	// transport.ErrPreconditionFailed, and the metadata returned beside the
	// error carries that latest version uid.
	_, stale, err := composition.Update(ctx, client, exampleEHRID, voID, created.VersionUID.String(), latest)
	if err == nil {
		return errors.New("stale update: the server accepted an outdated If-Match")
	}
	if !errors.Is(err, transport.ErrPreconditionFailed) {
		return fmt.Errorf("stale update: %w", err)
	}
	status := 0
	if wireErr, ok := errors.AsType[*transport.WireError](err); ok && wireErr != nil {
		status = wireErr.StatusCode
	}
	fmt.Printf("stale update: If-Match=%s refused, status=%d\n", created.VersionUID, status)
	if stale != nil {
		// Re-read from this version, reapply the change, and update again.
		fmt.Printf("  server's latest VersionUID=%s\n", stale.VersionUID)
	}
	fmt.Println("OK: composition saved, read, updated, and a stale update refused, against an in-process fake CDR")
	return nil
}

// newClient wires the SDK's REST client: a static service catalog says where
// the openEHR REST API lives, and transport.New takes that catalog plus the
// injected *http.Client.
func newClient(httpClient *http.Client) (*transport.Client, error) {
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://sandbox.local",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(baseURL),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build static catalog: %w", err)
	}
	client, err := transport.New(catalog, transport.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("transport.New: %w", err)
	}
	return client, nil
}

// loadComposition decodes the vendored body_weight composition. In your app
// this is the composition your code built or received.
func loadComposition() (*rm.Composition, error) {
	body, err := os.ReadFile(fixtures.CompositionJSON("body_weight"))
	if err != nil {
		return nil, fmt.Errorf("read fixture: %w", err)
	}
	comp := new(rm.Composition)
	if err := canjson.Unmarshal(body, comp); err != nil {
		return nil, fmt.Errorf("decode canonical JSON: %w", err)
	}
	return comp, nil
}

// correctWeight sets the weight in comp to kg. rmpath finds the value by its
// openEHR path, and the change is made in place.
func correctWeight(comp *rm.Composition, kg rm.Real) error {
	v, err := rmpath.ItemAtPath(comp, weightPath)
	if err != nil {
		return fmt.Errorf("find the weight: %w", err)
	}
	q, ok := v.(*rm.DVQuantity)
	if !ok || q == nil {
		return fmt.Errorf("the weight is a %T, not a DV_QUANTITY", v)
	}
	q.Magnitude = kg
	return nil
}

// Create, read and update a PERSON through the SDK's Demographic REST
// client, tag it, and read an earlier version back. The program creates a
// first version and reads it in canonical JSON and in canonical XML. It
// changes the family name in an update that carries the version uid it read
// in If-Match, then sets two item tags on the versioned party and finds one
// by its key. Last, it reads version 1 from the versioned_party routes: that
// version still holds the old name.
//
// It runs offline: an in-process fake demographic server (fake_server.go)
// answers the routes the program calls. It is plugged into the *http.Client
// as its transport, so no listener is opened and the host in the base URL is
// never dialled.
//
//	go run ./cmd/examples/demographic-crud
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// baseURL is where the openEHR REST API lives. The fake answers it in-process.
const baseURL = "https://sandbox.local/openehr/v1"

// The PERSON is shaped after the CKM archetypes
// openEHR-DEMOGRAPHIC-PERSON.person.v0 and
// openEHR-DEMOGRAPHIC-PARTY_IDENTITY.person_name.v0, cut down to one name
// identity.
const (
	personArchetype = "openEHR-DEMOGRAPHIC-PERSON.person.v0"
	nameArchetype   = "openEHR-DEMOGRAPHIC-PARTY_IDENTITY.person_name.v0"
	// identityPath is the path of the name identity inside the PERSON. A
	// tag names it to mark that identity rather than the whole PERSON.
	identityPath = "/identities[" + nameArchetype + "]"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Step 1: one deadline for the whole exchange, and an HTTP client with a
	// timeout of its own for each request. The SDK never builds an
	// *http.Client: you inject it, here with the fake server as its transport.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpClient := (&fakeServer{base: baseURL}).httpClient()
	httpClient.Timeout = 5 * time.Second
	client, err := newClient(httpClient)
	if err != nil {
		return err
	}

	// Step 2: create the first version. Create picks the route from the
	// value's type: a *rm.Person goes to POST /demographic/person. The
	// default Prefer is return=minimal, so the new version uid comes from
	// the response headers, not from a body.
	_, created, err := demographic.Create(ctx, client, newPerson("Jane", "Doe"))
	if err != nil {
		return fmt.Errorf("demographic.Create: %w", err)
	}
	if created == nil || created.VersionUID == "" {
		return errors.New("demographic.Create: the server named no version")
	}
	fmt.Printf("create: VersionUID=%s\n", created.VersionUID)
	fmt.Printf("  Location=%s\n", created.Location)

	// Step 3: read the latest version in canonical JSON, the default. A
	// version uid is "<versioned object id>::<system id>::<version>", and
	// LatestOf asks for the newest version of the family.
	voID := created.VersionUID.VersionedObjectID()
	party, current, err := demographic.Get(ctx, client, demographic.Person, openehrclient.LatestOf(voID))
	if err != nil {
		return fmt.Errorf("demographic.Get: %w", err)
	}
	family, err := familyName(party)
	if err != nil {
		return err
	}
	fmt.Printf("get JSON: family name %s, VersionUID=%s\n", family.Value, current.VersionUID)

	// Read the same version in canonical XML. VersionOf names one version.
	// WithRawBody keeps what the server sent beside the decoded party.
	var raw demographic.Raw
	fromXML, _, err := demographic.Get(ctx, client, demographic.Person, openehrclient.VersionOf(current.VersionUID),
		demographic.WithGetFormat(demographic.FormatXML), demographic.WithRawBody(&raw))
	if err != nil {
		return fmt.Errorf("demographic.Get as XML: %w", err)
	}
	xmlFamily, err := familyName(fromXML)
	if err != nil {
		return err
	}
	fmt.Printf("get XML: family name %s, Content-Type=%s\n", xmlFamily.Value, raw.ContentType)

	// Step 4: change the family name in the party read as JSON, and update.
	// If-Match carries the version uid just read: the server applies the
	// update only while that version is still the latest.
	family.Value = "Smith"
	_, next, err := demographic.Update(ctx, client, demographic.Person, voID, current.VersionUID.String(), party)
	if err != nil {
		return fmt.Errorf("demographic.Update: %w", err)
	}
	fmt.Printf("update: family name Smith, VersionUID=%s\n", next.VersionUID)

	// Step 5: tag the versioned party. LatestOf here names the versioned
	// party itself, so the tags stay with it as new versions arrive.
	// SetTags replaces the whole list and commits no new version. One tag
	// marks the whole PERSON; the other marks the name identity, by its path.
	tags := []openehrclient.ItemTag{
		{Key: "source", Value: "registration-desk"},
		{Key: "verified", Value: "passport", TargetPath: identityPath},
	}
	stored, err := demographic.SetTags(ctx, client, demographic.Person, openehrclient.LatestOf(voID), tags,
		demographic.WithTagsPrefer(transport.PreferRepresentation))
	if err != nil {
		return fmt.Errorf("demographic.SetTags: %w", err)
	}
	fmt.Printf("set tags: %d stored\n", len(stored))

	// ListTags searches the tags of every party; WithTagKey narrows it.
	found, err := demographic.ListTags(ctx, client, demographic.WithTagKey("verified"))
	if err != nil {
		return fmt.Errorf("demographic.ListTags: %w", err)
	}
	for _, tag := range found {
		fmt.Printf("list tags with key verified: %s=%s on %s %s\n", tag.Key, tag.Value, tag.Target.Type, tag.TargetPath)
	}

	// Step 6: read version 1 from the versioned_party routes. It still holds
	// the family name it was created with, and its commit audit says how it
	// was made.
	first, _, err := demographic.GetVersionByID(ctx, client, voID, created.VersionUID)
	if err != nil {
		return fmt.Errorf("demographic.GetVersionByID: %w", err)
	}
	oldFamily, err := familyName(first.Party)
	if err != nil {
		return err
	}
	audit, ok := rm.AuditDetailsBase(first.CommitAudit)
	if !ok {
		return errors.New("demographic.GetVersionByID: the version has no commit audit")
	}
	fmt.Printf("version 1: family name %s, change type %s\n", oldFamily.Value, audit.ChangeType.Value)
	fmt.Println("OK: PERSON created, read in JSON and XML, updated, tagged, and version 1 read back, against an in-process fake server")
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

// newPerson builds a PERSON with one identity, its legal name. In your app
// the PERSON follows the archetypes your deployment uses.
func newPerson(given, family string) *rm.Person {
	element := func(code, name, value string) rm.Item {
		return &rm.Element{ArchetypeNodeID: code, Name: &rm.DVText{Value: name}, Value: &rm.DVText{Value: value}}
	}
	return &rm.Person{
		ArchetypeNodeID:  personArchetype,
		ArchetypeDetails: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: personArchetype}, RMVersion: "1.1.0"},
		Name:             &rm.DVText{Value: "Person"},
		Identities: []rm.PartyIdentity{{
			ArchetypeNodeID:  nameArchetype,
			ArchetypeDetails: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: nameArchetype}, RMVersion: "1.1.0"},
			Name:             &rm.DVText{Value: "Legal name"},
			Details: &rm.ItemTree{
				ArchetypeNodeID: "at0001",
				Name:            &rm.DVText{Value: "Name"},
				Items: []rm.Item{
					element("at0002", "Given name", given),
					element("at0003", "Family name", family),
				},
			},
		}},
	}
}

// familyName returns the family name in a PERSON, found through the Go
// fields: rmpath does not walk a PARTY yet. A change to the returned value
// changes the PERSON.
func familyName(party rm.Party) (*rm.DVText, error) {
	person, ok := party.(*rm.Person)
	if !ok || person == nil {
		return nil, fmt.Errorf("got a %T, not a PERSON", party)
	}
	for _, identity := range person.Identities {
		tree, ok := identity.Details.(*rm.ItemTree)
		if identity.ArchetypeNodeID != nameArchetype || !ok || tree == nil {
			continue
		}
		for _, item := range tree.Items {
			e, ok := item.(*rm.Element)
			if !ok || e == nil || e.ArchetypeNodeID != "at0003" {
				continue
			}
			if text, ok := e.Value.(*rm.DVText); ok && text != nil {
				return text, nil
			}
		}
	}
	return nil, errors.New("the PERSON has no family name")
}

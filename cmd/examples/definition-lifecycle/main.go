// Example: the lifecycle of a template through the Definition REST client. It
// uploads an ADL 1.4 operational template (OPT) to a server, finds it in the
// server's template listing, downloads the stored OPT again, compiles those
// bytes into the local template handle, and asks the server for an example
// composition.
//
// Two different things are called "the template" here. The template stored on
// the server is what the clinical data repository (CDR) checks every
// composition write against. The compiled handle is what this process's
// builder, validator and generator use. The SDK does not keep them in step for
// you: when a template changes, upload it and recompile it.
//
// Runs offline against an in-process fake CDR (see fake_cdr.go), with no
// network listener:
//
//	go run ./cmd/examples/definition-lifecycle
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// baseURL is where the fake CDR pretends to serve the openEHR REST API.
const baseURL = "https://sandbox.local/openehr/v1"

// fixtureID names the vendored OPT and the composition recorded against it.
const fixtureID = "body_weight"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Step 1: bound the whole run with a deadline and wire the client. Every
	// Definition call takes the context first and gives up when it expires.
	// The HTTP client comes from the fake here; against a real CDR, inject
	// your own. Either way, give it a timeout of its own.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdr, err := newFakeCDR()
	if err != nil {
		return err
	}
	httpClient := cdr.backend().HTTPClient()
	httpClient.Timeout = 10 * time.Second
	client, err := newClient(httpClient)
	if err != nil {
		return err
	}

	// Step 2: upload the OPT. The server answers 201 Created; UploadTemplate
	// reads the template id from a JSON body when there is one, and otherwise
	// from the last segment of the Location header.
	opt, err := os.ReadFile(fixtures.TemplateOpt(fixtureID))
	if err != nil {
		return fmt.Errorf("read OPT: %w", err)
	}
	uploaded, _, err := definition.UploadTemplate(ctx, client, definition.FormatADL14, bytes.NewReader(opt))
	if err != nil {
		return fmt.Errorf("upload template: %w", err)
	}
	id := uploaded.TemplateID
	fmt.Printf("uploaded template    : %s\n", id)

	// Step 3: look the template up in the listing. There is no call that
	// returns the metadata of one template, so filter the list by its id.
	listed, _, err := definition.ListTemplates(ctx, client, definition.FormatADL14, definition.WithTemplateID(id))
	if err != nil {
		return fmt.Errorf("list templates: %w", err)
	}
	if len(listed) != 1 {
		return fmt.Errorf("listing for %s has %d entries, want 1", id, len(listed))
	}
	fmt.Printf("listed metadata      : id=%s concept=%s\n", listed[0].TemplateID, listed[0].Concept)

	// Step 4: download the stored OPT. GetTemplate returns the bytes exactly
	// as the server sends them; this server stores them unchanged.
	raw, _, err := definition.GetTemplate(ctx, client, id, definition.FormatADL14)
	if err != nil {
		return fmt.Errorf("get template: %w", err)
	}
	if !bytes.Equal(raw, opt) {
		return errors.New("downloaded OPT differs from the uploaded one")
	}
	fmt.Printf("downloaded OPT       : %d bytes, matches the upload\n", len(raw))

	// Step 5: turn the bytes into the local handle. ParseOPTStrict fails on a
	// node type it does not know instead of silently dropping what is under
	// it. The compiled template is what the builder, validator and generator
	// take; compile it once and reuse it.
	parsed, err := template.ParseOPTStrict(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse OPT: %w", err)
	}
	compiled, err := templatecompile.Compile(parsed)
	if err != nil {
		return fmt.Errorf("compile template: %w", err)
	}
	fmt.Printf("compiled handle      : template=%s root=%s\n", compiled.TemplateID(), compiled.Root().ArchetypeID())

	// Step 6: ask the server for an example composition of the template. The
	// type and detail level are optional; without them the server uses input
	// and required.
	example, _, err := definition.ExampleComposition(ctx, client, id, definition.FormatADL14,
		definition.WithExampleType(definition.ExampleTypeInput),
		definition.WithExampleDetailLevel(definition.ExampleDetailRequired),
	)
	if err != nil {
		return fmt.Errorf("example composition: %w", err)
	}
	fmt.Printf("example composition  : %s with %d content item(s)\n", example.ArchetypeNodeID, len(example.Content))
	fmt.Println("OK: template uploaded, listed, downloaded and compiled; example received")
	return nil
}

// newClient wires the two layers every REST call goes through: a service
// catalog that says where the openEHR REST API lives, and a transport client
// that carries the *http.Client you inject. The transport never builds its own
// HTTP client, so timeouts, TLS and connection pooling stay under your control.
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

package transport_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func Example_errorClassification() {
	client, err := transport.New(mustSandboxCatalog(),
		transport.WithHTTPClient(newCompositionErrorBackend().HTTPClient()))
	if err != nil {
		fmt.Println(err)
		return
	}
	ctx := context.Background()
	edited := &rm.Composition{} // the composition you changed; the fake server ignores it

	// The fake server answers each of these updates with a different error.
	for _, voID := range []ehr.VersionedObjectID{unknownComposition, changedComposition, invalidComposition} {
		lastRead := string(voID) + "::cdr.example::1" // the version you read, sent as If-Match
		_, _, err := composition.Update(ctx, client, exampleEHRID, voID, lastRead, edited)
		// errors.Is matches the class of the failure, however the error
		// was wrapped on its way up.
		switch {
		case errors.Is(err, transport.ErrNotFound):
			fmt.Println("not found: the EHR or the composition does not exist")
		case errors.Is(err, transport.ErrPreconditionFailed):
			fmt.Println("precondition failed: a newer version was saved first")
		case errors.Is(err, transport.ErrUnprocessable):
			fmt.Println("unprocessable: the composition did not validate")
		case err != nil:
			fmt.Println("other error:", err)
		default:
			fmt.Println("updated")
		}
	}

	// Output:
	// not found: the EHR or the composition does not exist
	// precondition failed: a newer version was saved first
	// unprocessable: the composition did not validate
}

func ExampleWireError() {
	// By default the transport keeps the server's error message and body
	// out of the error, because they can hold patient data. Add
	// transport.WithRawErrorBodies(true) to keep them, in
	// WireError.OpenEHR.Message and WireError.RawBody.
	client, err := transport.New(mustSandboxCatalog(),
		transport.WithHTTPClient(newCompositionErrorBackend().HTTPClient()))
	if err != nil {
		fmt.Println(err)
		return
	}
	lastRead := string(invalidComposition) + "::cdr.example::1"
	_, _, err = composition.Update(context.Background(), client,
		exampleEHRID, invalidComposition, lastRead, &rm.Composition{})

	// errors.AsType finds the *transport.WireError in the chain. Check the
	// pointer as well as ok: a nil *WireError would also match.
	we, ok := errors.AsType[*transport.WireError](err)
	if !ok || we == nil {
		fmt.Println("not a wire error:", err)
		return
	}
	fmt.Println("status:", we.StatusCode)
	if we.OpenEHR != nil {
		fmt.Println("openEHR code:", we.OpenEHR.Code)
		fmt.Println("server message kept:", we.OpenEHR.Message != "")
	}
	fmt.Println("raw body kept:", len(we.RawBody) > 0)

	// The text names the class, the route template, the status and the
	// openEHR code only, so it is safe to log.
	fmt.Println(err)

	// Output:
	// status: 422
	// openEHR code: UNPROCESSABLE_ENTITY
	// server message kept: false
	// raw body kept: false
	// transport: unprocessable entity (PUT /ehr/{ehr_id}/composition/{versioned_object_id}) status=422 code=UNPROCESSABLE_ENTITY
}

// The ids the examples send. The fake server answers each versioned object
// id with a different error status.
const (
	exampleEHRID       ehr.EHRID             = "7d44b88c-4199-4bad-97dc-d78268e01398"
	unknownComposition ehr.VersionedObjectID = "0f1e2d3c-4b5a-4697-8877-665544332211"
	changedComposition ehr.VersionedObjectID = "8849182c-82ad-4088-a07f-48ead4180515"
	invalidComposition ehr.VersionedObjectID = "c0ffee00-1234-4abc-9def-0123456789ab"
)

// newCompositionErrorBackend is a fake openEHR server. It answers a
// composition update, PUT /ehr/{ehr_id}/composition/{versioned_object_uid},
// with one of the error statuses the openEHR REST API lists for that call,
// chosen by the versioned object id.
func newCompositionErrorBackend() *sandbox.Backend {
	b := sandbox.New()
	answer := func(voID ehr.VersionedObjectID, status int, etag, body string) {
		path := "/ehr/" + string(exampleEHRID) + "/composition/" + string(voID)
		b.HandleFunc(http.MethodPut, path, func(w http.ResponseWriter, r *http.Request) {
			if etag != "" {
				w.Header().Set("ETag", etag)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			// A failed write leaves the client with an error without an
			// openEHR code, which the examples would print.
			_, _ = io.WriteString(w, body)
		})
	}
	answer(unknownComposition, http.StatusNotFound, "", `{
  "message": "Versioned object not found: 0f1e2d3c-4b5a-4697-8877-665544332211",
  "code": "NOT_FOUND"
}`)
	// A 412 also names the latest version in ETag, so the caller can read
	// that version and retry.
	answer(changedComposition, http.StatusPreconditionFailed,
		`"8849182c-82ad-4088-a07f-48ead4180515::cdr.example::2"`, `{
  "message": "If-Match does not match the latest version",
  "code": "PRECONDITION_FAILED"
}`)
	answer(invalidComposition, http.StatusUnprocessableEntity, "", `{
  "message": "Composition does not validate against the operational template",
  "code": "UNPROCESSABLE_ENTITY",
  "coded_text": [
    {
      "terminology_id": {"value": "openehr"},
      "code_string": "VALIDATION_FAILED"
    }
  ]
}`)
	return b
}

// mustSandboxCatalog points the openEHR REST service at the base URL the
// sandbox backend serves. Against a real server, put its base URL here.
func mustSandboxCatalog() *discovery.ServiceCatalog {
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
		panic(err)
	}
	return catalog
}

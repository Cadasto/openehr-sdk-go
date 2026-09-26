// This example assembles a CONTRIBUTION with contribution.Builder and prints
// the Contribution_create request body the builder produces. A CONTRIBUTION
// is the openEHR unit of commit: several versions written to one EHR in a
// single atomic request. Two canonical-JSON compositions from the vendored
// cassettes go in, one as a first version and one as an amendment of a
// version that already exists. The program runs offline.
//
// Run:
//
//	go run ./cmd/examples/contribution-build
//	go run ./cmd/examples/contribution-build -commit
//
// With -commit the body is also POSTed through contribution.Commit to an
// in-process fake CDR (clinical data repository), and the request the fake
// CDR received is compared with the body that was built.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// The two vendored compositions the batch commits, named by their template
// id, and the two ids a real caller holds from earlier calls: the EHR to
// write into and the uid of the version the amendment replaces.
const (
	firstTemplateID  = "Test_dv_quantity_open_constraint.v0"
	secondTemplateID = "body_weight"
	precedingUID     = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"
	ehrID            = "f0e1d2c3-b4a5-6789-0123-456789abcdef"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	commit := flag.Bool("commit", false, "POST the built body to an in-process fake CDR")
	flag.Parse()

	// Step 1: load the two compositions. Any *rm.Composition works here;
	// the fixtures only save this example from building one by hand.
	created, err := loadComposition(firstTemplateID)
	if err != nil {
		return err
	}
	amended, err := loadComposition(secondTemplateID)
	if err != nil {
		return err
	}

	// Step 2: describe the batch once, then add one Change per version. The
	// builder derives each version's change_type from the operation
	// (Creation, Amendment, ...) and copies the committer and system id from
	// the batch audit into every version. The batch audit's own change_type
	// describes the contribution as a whole and is never derived from the
	// versions, so the caller declares it. Build validates everything
	// accumulated so far and returns every problem joined into one error.
	submission, err := contribution.NewBuilder().
		WithCommitterName("Dr. House").
		WithSystemID("cdr.example").
		WithDescription("worked example: one creation + one amendment").
		WithChangeType(contribution.ChangeTypeCreation).
		Add(contribution.Creation(created)).
		Add(contribution.Amendment(precedingUID, amended,
			contribution.WithLifecycleState(openehrclient.LifecycleStateComplete))).
		Build()
	if err != nil {
		return fmt.Errorf("build contribution: %w", err)
	}

	// Step 3: serialise to canonical JSON. These are the bytes that
	// contribution.Commit sends as the request body.
	body, err := canjson.Marshal(submission)
	if err != nil {
		return fmt.Errorf("marshal contribution body: %w", err)
	}
	fmt.Printf("built Contribution_create body (%d bytes):\n%s\n\n", len(body), indent(body))
	if err := summarise(body); err != nil {
		return err
	}

	if !*commit {
		fmt.Println("\nOK: body built. Re-run with -commit to POST it to an in-process fake CDR.")
		return nil
	}
	// Step 4 (only with -commit): send it the way an application would.
	return commitToFakeCDR(context.Background(), submission, body)
}

// loadComposition reads a vendored canonical-JSON composition and decodes it
// into the typed RM struct the builder takes.
func loadComposition(templateID string) (*rm.Composition, error) {
	raw, err := os.ReadFile(fixtures.CompositionJSON(templateID))
	if err != nil {
		return nil, fmt.Errorf("read composition fixture %s: %w", templateID, err)
	}
	var composition rm.Composition
	if err := canjson.Unmarshal(raw, &composition); err != nil {
		return nil, fmt.Errorf("decode composition fixture %s: %w", templateID, err)
	}
	return &composition, nil
}

// summarise picks the fields worth noticing out of the built body and prints
// them on one line per version, so the reader need not scan the JSON above:
// the batch change_type the caller declared, and per version the change_type
// and lifecycle_state the builder set plus the preceding version uid that an
// amendment carries.
func summarise(body []byte) error {
	var decoded struct {
		Audit struct {
			ChangeType codedText `json:"change_type"`
		} `json:"audit"`
		Versions []struct {
			Type        string `json:"_type"`
			CommitAudit struct {
				ChangeType codedText `json:"change_type"`
			} `json:"commit_audit"`
			LifecycleState      codedText `json:"lifecycle_state"`
			PrecedingVersionUID *struct {
				Value string `json:"value"`
			} `json:"preceding_version_uid"`
			Data struct {
				Type string `json:"_type"`
			} `json:"data"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("decode built body: %w", err)
	}
	fmt.Printf("batch audit change_type: %s (%s) — declared, not derived\n",
		decoded.Audit.ChangeType.Value, decoded.Audit.ChangeType.DefiningCode.CodeString)
	for i, version := range decoded.Versions {
		preceding := "(none — a first version)"
		if version.PrecedingVersionUID != nil {
			preceding = version.PrecedingVersionUID.Value
		}
		fmt.Printf("versions[%d]: %s<%s> change_type=%s/%s lifecycle_state=%s/%s preceding=%s\n",
			i, version.Type, version.Data.Type,
			version.CommitAudit.ChangeType.Value, version.CommitAudit.ChangeType.DefiningCode.CodeString,
			version.LifecycleState.Value, version.LifecycleState.DefiningCode.CodeString,
			preceding)
	}
	return nil
}

// codedText is the part of a DV_CODED_TEXT the summary reads: the display
// value and the code behind it.
type codedText struct {
	Value        string `json:"value"`
	DefiningCode struct {
		CodeString string `json:"code_string"`
	} `json:"defining_code"`
}

// commitToFakeCDR posts the submission through contribution.Commit, the call
// an application makes against a real CDR, to an httptest server that records
// the request. It then checks that the bytes on the wire are the bytes that
// were built.
func commitToFakeCDR(ctx context.Context, submission *contribution.Submission, built []byte) error {
	var captured []byte
	var readErr error
	fakeCDR := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, readErr = io.ReadAll(r.Body)
		// A CDR answers a commit with 201 Created and a Location header that
		// points at the new contribution; any uid will do for the fake.
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/openehr/v1/ehr/"+ehrID+"/contribution/"+precedingUID)
		w.WriteHeader(http.StatusCreated)
	}))
	defer fakeCDR.Close()

	client, err := newClient(fakeCDR)
	if err != nil {
		return err
	}
	// Commit asks for the minimal response by default, so its first return
	// value is nil; pass contribution.WithPrefer(transport.PreferRepresentation)
	// to get the persisted contribution back. It also returns the response
	// metadata and an error mapped to the transport sentinels.
	_, meta, err := contribution.Commit(ctx, client, openehrclient.EHRID(ehrID), submission)
	if err != nil {
		return fmt.Errorf("commit contribution: %w", err)
	}
	if readErr != nil {
		return fmt.Errorf("fake CDR read request body: %w", readErr)
	}
	fmt.Printf("\ncommitted: %d bytes reached the wire; Location=%q\n", len(captured), meta.Location)
	if !bytes.Equal(bytes.TrimSpace(captured), bytes.TrimSpace(built)) {
		return errors.New("the captured request body differs from the built body")
	}
	fmt.Println("OK: the captured request body is byte-identical to the built body")
	return nil
}

// newClient wires the SDK's REST client to the fake CDR. A static service
// catalog says where the openEHR REST base URL is, and transport.New takes
// that catalog plus the *http.Client to use; the SDK never allocates one.
func newClient(fakeCDR *httptest.Server) (*transport.Client, error) {
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://example.test",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(fakeCDR.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build static catalog: %w", err)
	}
	client, err := transport.New(catalog, transport.WithHTTPClient(fakeCDR.Client()))
	if err != nil {
		return nil, fmt.Errorf("create transport client: %w", err)
	}
	return client, nil
}

// indent pretty-prints the body for display, falling back to the raw bytes.
func indent(body []byte) []byte {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		return body
	}
	return pretty.Bytes()
}

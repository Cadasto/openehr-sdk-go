// Example: assemble a multi-version CONTRIBUTION with
// `contribution.Builder` (REQ-130) and print the `Contribution_create`
// body it produces. Two canonical compositions from the vendored
// cassettes go in — one as a first version, one as an amendment of an
// existing version — and the builder sets each version's change-type
// code, lifecycle state, and commit audit.
//
// Run: `go run ./cmd/examples/contribution-build` from any directory.
// Add `-commit` to POST the body to an in-process fake CDR and print the
// captured request, which is the Build → Commit path an integrator takes.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
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

// The two vendored compositions the batch commits, and the version the
// second one amends (a uid a real caller reads from an earlier write).
const (
	firstTemplateID  = "Test_dv_quantity_open_constraint.v0"
	secondTemplateID = "body_weight"
	precedingUID     = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::1"
	ehrID            = "f0e1d2c3-b4a5-6789-0123-456789abcdef"
)

func main() {
	commit := flag.Bool("commit", false, "POST the built body to an in-process fake CDR")
	flag.Parse()

	created := mustComposition(firstTemplateID)
	amended := mustComposition(secondTemplateID)

	// One batch, two versions: a creation and an amendment. The builder
	// derives each version's change_type from the operation and inherits
	// the batch committer / system id; the batch audit's own change_type
	// is declared by the caller and never derived (REQ-130).
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
		log.Fatalf("contribution.Builder.Build: %v", err)
	}

	body, err := canjson.Marshal(submission)
	if err != nil {
		log.Fatalf("canjson.Marshal: %v", err)
	}
	fmt.Printf("built Contribution_create body (%d bytes):\n%s\n\n", len(body), indent(body))
	summarise(body)

	if !*commit {
		fmt.Println("\nOK: body built. Re-run with -commit to POST it to an in-process fake CDR.")
		return
	}
	commitToFake(submission)
}

// commitToFake posts the built submission through contribution.Commit to a
// fake CDR, proving the same body survives the real client path.
func commitToFake(submission *contribution.Submission) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = readAll(r)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/openehr/v1/ehr/"+ehrID+"/contribution/"+precedingUID)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := mustClient(srv)
	_, meta, err := contribution.Commit(context.Background(), c, openehrclient.EHRID(ehrID), submission)
	if err != nil {
		log.Fatalf("contribution.Commit: %v", err)
	}
	fmt.Printf("\ncommitted: %d bytes reached the wire; Location=%q\n", len(captured), meta.Location)
	if !bytes.Equal(bytes.TrimSpace(captured), bytes.TrimSpace(mustMarshal(submission))) {
		log.Fatal("the captured request body differs from the built body")
	}
	fmt.Println("OK: the captured request body is byte-identical to the built body")
}

// summarise prints the per-version fields REQ-130 governs, so the example
// shows what to look for rather than leaving the reader to diff JSON.
func summarise(body []byte) {
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
		log.Fatalf("decode built body: %v", err)
	}
	fmt.Printf("batch audit change_type: %s (%s) — declared, not derived\n",
		decoded.Audit.ChangeType.Value, decoded.Audit.ChangeType.DefiningCode.CodeString)
	for i, v := range decoded.Versions {
		preceding := "(none — a first version)"
		if v.PrecedingVersionUID != nil {
			preceding = v.PrecedingVersionUID.Value
		}
		fmt.Printf("versions[%d]: %s<%s> change_type=%s/%s lifecycle_state=%s/%s preceding=%s\n",
			i, v.Type, v.Data.Type,
			v.CommitAudit.ChangeType.Value, v.CommitAudit.ChangeType.DefiningCode.CodeString,
			v.LifecycleState.Value, v.LifecycleState.DefiningCode.CodeString,
			preceding)
	}
}

type codedText struct {
	Value        string `json:"value"`
	DefiningCode struct {
		CodeString string `json:"code_string"`
	} `json:"defining_code"`
}

// mustComposition decodes a vendored canonical-JSON composition.
func mustComposition(templateID string) *rm.Composition {
	raw, err := os.ReadFile(fixtures.CompositionJSON(templateID))
	if err != nil {
		log.Fatalf("read composition fixture %s: %v", templateID, err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(raw, &comp); err != nil {
		log.Fatalf("decode composition fixture %s: %v", templateID, err)
	}
	return &comp
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

func mustMarshal(submission *contribution.Submission) []byte {
	b, err := canjson.Marshal(submission)
	if err != nil {
		log.Fatalf("canjson.Marshal: %v", err)
	}
	return b
}

func readAll(r *http.Request) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

// indent pretty-prints the body, falling back to the raw bytes.
func indent(body []byte) []byte {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		return body
	}
	return pretty.Bytes()
}

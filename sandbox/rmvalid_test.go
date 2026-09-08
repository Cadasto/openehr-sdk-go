package sandbox_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/sandbox"
)

// TestCreateEHRIsRMValid pins the sandbox EHR body against the RM
// floor (REQ-112 via [validation.ValidateRM]): ehr_access is
// RM-mandatory — rm.EHR.EHRAccess carries no `omitempty` json tag,
// and the vendored cassette testkit/cassettes/its_rest/ehr/ehr.json
// carries it — so a sandbox-created EHR must too.
//
// Two assertions, because openehr/validation's rmread does not
// currently model rm.EHR itself ([rmread.Handles] has no case for
// *rm.EHR): ValidateRM on the decoded *rm.EHR root always reports OK
// regardless of content, so it alone would not be can-fail (verified:
// removing ehr_access from the wire body does not flip it). The
// second assertion validates the extracted ehr_access value directly
// — OBJECT_REF is a modeled leaf type (checkObjectRef enforces its
// id/type/namespace floor even though rmread does not descend into
// it) — and a nil root is the [validation.Result] "nil_root" issue, so
// this half genuinely distinguishes present from absent. See
// TestCreateEHRIsRMValid_CanFail for the mutation-detection proof.
func TestCreateEHRIsRMValid(t *testing.T) {
	t.Parallel()
	body := createEHRBody(t)

	var decoded rm.EHR
	if err := canjson.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("canjson.Unmarshal: %v", err)
	}
	if r := validation.ValidateRM(&decoded); !r.OK {
		t.Fatalf("ValidateRM(sandbox EHR) not OK: %+v", r.Issues)
	}
	if r := validation.ValidateRM(decoded.EHRAccess); !r.OK {
		t.Fatalf("ValidateRM(sandbox EHR.EHRAccess) not OK: %+v", r.Issues)
	}
}

// TestCreateEHRIsRMValid_CanFail is the can-fail control: it proves
// the ehr_access assertion above is load-bearing rather than
// vacuously true. It strips ehr_access from the wire body and
// confirms ValidateRM(decoded.EHRAccess) flips to not-OK (a nil
// ObjectRefLike is a nil interface, which ValidateRM reports as
// "nil_root" before ever consulting the RM floor).
func TestCreateEHRIsRMValid_CanFail(t *testing.T) {
	t.Parallel()
	body := createEHRBody(t)

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	delete(raw, "ehr_access")
	stripped, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded rm.EHR
	if err := canjson.Unmarshal(stripped, &decoded); err != nil {
		t.Fatalf("canjson.Unmarshal (ehr_access stripped): %v", err)
	}
	if decoded.EHRAccess != nil {
		t.Fatalf("decoded.EHRAccess = %#v, want nil after stripping ehr_access from the wire body", decoded.EHRAccess)
	}
	if r := validation.ValidateRM(decoded.EHRAccess); r.OK {
		t.Fatal("ValidateRM(decoded.EHRAccess) reported OK after ehr_access was stripped — guard is not can-fail")
	}
}

// createEHRBody drives one POST /ehr through a fresh Backend and
// returns the raw response body.
func createEHRBody(t *testing.T) []byte {
	t.Helper()
	b := sandbox.New()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://sandbox.local/openehr/v1/ehr", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := b.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

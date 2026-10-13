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
// and the vendored fixture testkit/corpus/its_rest/ehr/ehr.json
// carries it — so a sandbox-created EHR must too.
//
// Two assertions. ValidateRM on the decoded *rm.EHR root reads every
// attribute of EHR through the generated access, so it reports an absent
// ehr_access as `required` at /ehr_access. The second validates the
// extracted ehr_access value directly: the floor checks an OBJECT_REF's
// id/type/namespace (checkObjectRef), and a nil root is the
// [validation.Result] "nil_root" issue, so this half distinguishes present
// from absent too. See TestCreateEHRIsRMValid_CanFail for the
// mutation-detection proof of both.
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
// both assertions above are load-bearing rather than vacuously true. It
// strips ehr_access from the wire body and confirms that ValidateRM on the
// decoded EHR reports `required` at /ehr_access, and that
// ValidateRM(decoded.EHRAccess) flips to not-OK (a nil ObjectRefLike is a
// nil interface, which ValidateRM reports as "nil_root" before ever
// consulting the RM floor).
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
	if r := validation.ValidateRM(&decoded); !hasIssue(r, "/ehr_access", "required") {
		t.Fatalf("ValidateRM(sandbox EHR without ehr_access) issues = %+v, want required at /ehr_access — guard is not can-fail", r.Issues)
	}
	if r := validation.ValidateRM(decoded.EHRAccess); r.OK {
		t.Fatal("ValidateRM(decoded.EHRAccess) reported OK after ehr_access was stripped — guard is not can-fail")
	}
}

// hasIssue reports whether r carries an issue of code at path.
func hasIssue(r validation.Result, path, code string) bool {
	for _, i := range r.Issues {
		if i.Path == path && i.Code == code {
			return true
		}
	}
	return false
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

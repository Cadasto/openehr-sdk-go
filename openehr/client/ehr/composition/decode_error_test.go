package composition_test

// decode_error_test.go — REQ-151 § Typed 2xx decode failure at the Composition
// read leaf, and the keyed exclusion that sits right next to it.
//
// Get decodes the response body itself via canjson rather than through
// transport.Decode, so it did not inherit the contract by construction;
// REQ-151 § Scope requires it satisfy the contract identically anyway. Update
// under Prefer=return=representation decodes through the REQ-094 write-result
// funnel, which keeps its own taxonomy — *ehr.NoRepresentationError — and
// REQ-151 § Keyed exclusions forbids re-typing it. Both are pinned here, so
// the boundary between the two is visible in one file.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// originalVersionBody is an ORIGINAL_VERSION<COMPOSITION> envelope served
// where a bare COMPOSITION is expected — the non-conformant server shape
// TestSaveRepresentationRejectsOriginalVersionShape already documents. It is
// guaranteed to fail the canjson decode on its `_type`, and it is a realistic
// undecodable body rather than a syntax error, so the codec's own typed
// diagnostics are exercised too.
const originalVersionBody = `{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"x::y::1"},"data":{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"}}}`

// TestGetDecodeFailureIsTyped is REQ-151's positive contract at composition.Get:
// a 2xx body that cannot be decoded as an rm.Composition surfaces as a
// *transport.DecodeError through the leaf's operation-name wrap, carrying the
// bytes the server delivered, the request's method and route template, and the
// canjson diagnostics behind Unwrap.
func TestGetDecodeFailureIsTyped(t *testing.T) { // REQ-151
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"`+string(compositionVUID)+`"`)
		_, _ = w.Write([]byte(originalVersionBody))
	}))
	defer srv.Close()

	comp, meta, err := composition.Get(t.Context(), newClient(t, srv), ehrIDFixture, openehrclient.LatestOf(compositionVOID))
	if err == nil {
		t.Fatalf("Get decoded an ORIGINAL_VERSION envelope as a Composition without error; the premise of this test is gone")
	}
	if comp != nil {
		t.Errorf("Get returned %+v beside the decode error; a failed decode has nothing to hand back", comp)
	}
	// REQ-151 § Metadata still arrives.
	if meta == nil {
		t.Fatal("Get returned nil metadata beside the decode error; REQ-151 § Metadata still arrives requires the response headers survive")
	}
	if got := meta.ETag; got != string(compositionVUID) {
		t.Errorf("VersionMetadata.ETag = %q, want %q — the headers must survive the decode failure intact", got, compositionVUID)
	}

	de, ok := errors.AsType[*transport.DecodeError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.DecodeError] did not match %T (%v); REQ-151 requires a 2xx decode failure be recoverable as that type, hand-rolled decode or not", err, err)
	}
	if got := string(de.Body); got != originalVersionBody {
		t.Errorf("DecodeError.Body = %q, want the bytes the server delivered, %q", got, originalVersionBody)
	}
	if de.Method != http.MethodGet {
		t.Errorf("DecodeError.Method = %q, want %q", de.Method, http.MethodGet)
	}
	if want := "/ehr/{ehr_id}/composition/{versioned_object_or_version_uid}"; de.Route != want {
		t.Errorf("DecodeError.Route = %q, want the route template %q, not the expanded path", de.Route, want)
	}
	// REQ-151 § The typed error: wrapping the decoder's error preserves the
	// codec's own typed diagnostics (path, type) through errors.AsType.
	if _, ok := errors.AsType[*canjson.DecodeError](err); !ok {
		t.Errorf("canjson's typed diagnostics no longer reachable through the conversion: %v", err)
	}
	if op := "composition.Get:"; !strings.HasPrefix(err.Error(), op) {
		t.Errorf("error message %q does not start with the operation name %q; the leaf's wrap must survive the conversion", err.Error(), op)
	}
}

// TestGetEmptyBodyKeepsInvalidShape holds REQ-151 § An empty 2xx body keeps its
// existing contract at this leaf: an absent body is not an unusable one.
func TestGetEmptyBodyKeepsInvalidShape(t *testing.T) { // REQ-151
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, _, err := composition.Get(t.Context(), newClient(t, srv), ehrIDFixture, openehrclient.LatestOf(compositionVOID))
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("err = %v, want transport.ErrInvalidShape on an empty 200 body", err)
	}
	if _, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Error("an empty 2xx body produced a *transport.DecodeError; REQ-151's keyed exclusion keeps this arm on ErrInvalidShape")
	}
}

// TestUpdateRepresentationDecodeFailureIsNotDecodeError is the keyed-exclusion
// pin for the write-result funnel (REQ-151 § Keyed exclusions, first bullet).
// Update under Prefer=return=representation decodes through
// ehr.WriteResult, whose committed-but-unusable arm is REQ-094's
// *ehr.NoRepresentationError — a materially different contract, since the
// write did commit and the caller's recovery is to re-read the version, not to
// re-parse the body. Re-typing it as *transport.DecodeError would tell the
// caller the opposite.
//
// composition.Update is one concrete leaf of that funnel; directory,
// ehrstatus and demographic reach the same statement in ehr.WriteResult.
func TestUpdateRepresentationDecodeFailureIsNotDecodeError(t *testing.T) { // REQ-151
	newVUID := openehrclient.VersionUID("1234abcd-5678-9012-3456-7890abcdef00::cdr.example::2")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+string(newVUID)+`"`)
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(newVUID))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(originalVersionBody))
	}))
	defer srv.Close()

	out, meta, err := composition.Update(
		t.Context(), newClient(t, srv), ehrIDFixture, compositionVOID, string(compositionVUID), readComposition(t),
		composition.WithPrefer(transport.PreferRepresentation),
	)
	if err == nil {
		t.Fatalf("Update decoded an ORIGINAL_VERSION envelope as a Composition without error; the premise of this test is gone")
	}
	// The arm's existing contract, unchanged.
	nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
	if !ok {
		t.Fatalf("err = %T (%v), want *ehr.NoRepresentationError — REQ-094 owns the write-result funnel", err, err)
	}
	if errors.Is(err, transport.ErrInvalidShape) {
		t.Error("a decode failure is not an empty-body ErrInvalidShape")
	}
	if out != nil {
		t.Errorf("no Composition on an undecodable representation body, got %+v", out)
	}
	if meta == nil || nre.Meta == nil || nre.Meta.VersionUID != newVUID {
		t.Errorf("commit metadata must survive: meta=%v nre.Meta=%v", meta, nre.Meta)
	}

	// The keyed exclusion itself: REQ-151 must NOT have re-typed this arm.
	if de, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Errorf("a write-result representation decode failure surfaced as *transport.DecodeError (%v); REQ-151 § Keyed exclusions leaves this arm to REQ-094's *ehr.NoRepresentationError", de)
	}
}

package ehr_test

// decode_error_test.go — REQ-151 § Keyed exclusions, held at
// VersionMetadata.ResolveIdentifierBody.
//
// The `Prefer: return=identifier` arm is REQ-094's negotiation surface, not a
// representation decode: what fails there is the ITS-REST Identifier wrapper
// the SDK asked the server to substitute FOR the representation, and REQ-094's
// no-silent-downgrade rule already gives it transport.ErrInvalidShape. REQ-151
// § Keyed exclusions names it explicitly and leaves that wrap alone.

import (
	"errors"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestResolveIdentifierBodyFailureIsNotDecodeError is the negative pin. Both
// ways the arm can fail — a body that does not parse, and one that parses but
// carries no uid — keep the transport.ErrInvalidShape wrap and must NOT
// surface as *transport.DecodeError.
func TestResolveIdentifierBodyFailureIsNotDecodeError(t *testing.T) { // REQ-151
	cases := []struct {
		name string
		body string
	}{
		{"undecodable", `}{ not json`},
		{"no uid", `{"not_uid":"x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := openehrclient.NewVersionMetadata(&transport.Metadata{})
			err := m.ResolveIdentifierBody([]byte(tc.body))
			if err == nil {
				t.Fatalf("ResolveIdentifierBody(%q) = nil; REQ-094 forbids silently discarding a non-empty identifier body", tc.body)
			}
			// The arm's existing contract, unchanged.
			if !errors.Is(err, transport.ErrInvalidShape) {
				t.Errorf("err = %v, want the transport.ErrInvalidShape wrap REQ-094 gives this arm", err)
			}
			// The keyed exclusion itself.
			if de, ok := errors.AsType[*transport.DecodeError](err); ok {
				t.Errorf("the Prefer=identifier arm surfaced as *transport.DecodeError (%v); REQ-151 § Keyed exclusions leaves it to REQ-094's ErrInvalidShape wrap", de)
			}
		})
	}
}

// TestResolveIdentifierBodyClassifiesNoRepresentation is the can-fail control
// for the shared predicate. REQ-094 and REQ-151 define an empty 2xx body as
// zero bytes, whitespace, OR JSON `null`, classified on the raw bytes through
// transport.IsNoRepresentationBody — the identifier arm is not exempt from the
// definition, only from the *DecodeError shape. A server that honours
// `Prefer: return=identifier` and sends `null` (or a padded `null`) has sent
// no identifier, not a malformed one: Location already carried the version, so
// the call must succeed and leave that VersionUID standing.
//
// This fails if the guard reverts to `len(body) == 0`: `null` decodes into
// Identifier as a no-op leaving uid empty (ErrInvalidShape "has no uid") and
// whitespace reaches encoding/json as an unexpected EOF.
func TestResolveIdentifierBodyClassifiesNoRepresentation(t *testing.T) { // REQ-094, REQ-151
	t.Parallel()
	const location = "/ehr/ehr-1/composition/obj-1::sys::1"
	for _, tc := range []struct {
		name string
		body string
	}{
		{"zero bytes", ""},
		{"whitespace", "  \n\t"},
		{"json null", "null"},
		{"padded json null", "\n null \t"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := openehrclient.NewVersionMetadata(&transport.Metadata{Location: location})
			want := m.VersionUID
			if want == "" {
				t.Fatalf("precondition: Location %q yielded no VersionUID", location)
			}
			if err := m.ResolveIdentifierBody([]byte(tc.body)); err != nil {
				t.Fatalf("ResolveIdentifierBody(%q) = %v, want nil: a no-representation body is not a bad identifier", tc.body, err)
			}
			if m.VersionUID != want {
				t.Errorf("ResolveIdentifierBody(%q) changed VersionUID to %q, want the Location-derived %q left standing", tc.body, m.VersionUID, want)
			}
		})
	}
}

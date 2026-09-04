package transport_test

// null_body_test.go — REQ-151 § An empty 2xx body keeps its existing
// per-surface contract: "empty" means zero bytes, whitespace only, or the JSON
// `null` literal, and transport.IsNoRepresentationBody is the one predicate.

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestDecodeNullBodyStaysInvalidShape pins the widened no-representation arm:
// a JSON `null` or whitespace-only 2xx body is "empty" in § REQ-151's sense —
// encoding/json would otherwise unmarshal `null` into a struct as a nil-error
// no-op and Decode would hand back a populated-looking all-zero *T. Reverting
// Decode to a len(body) == 0 check fails the `null` cases with a nil error.
func TestDecodeNullBodyStaysInvalidShape(t *testing.T) { // REQ-151
	for _, body := range []string{"null", " \n null \t", "   ", "\n"} {
		t.Run(fmt.Sprintf("%q", body), func(t *testing.T) {
			c := newDecodeClient(t, http.StatusOK, body, http.Header{"Content-Type": {"application/json"}})

			out, meta, err := transport.Decode[decodeFake](t.Context(), c, &transport.Request{
				Method: "GET",
				Path:   "/ehr/abc/ehr_status",
				Route:  "/ehr/{ehr_id}/ehr_status",
			})
			if out != nil {
				t.Fatalf("Decode(%q body) = %+v, want nil: a null body must not present as a populated struct", body, *out)
			}
			if !errors.Is(err, transport.ErrInvalidShape) {
				t.Fatalf("Decode(%q body) err = %v, want errors.Is(err, transport.ErrInvalidShape)", body, err)
			}
			if de, ok := errors.AsType[*transport.DecodeError](err); ok {
				t.Fatalf("Decode(%q body) err = %v (%T); the no-representation arm is not a DecodeError", body, de, err)
			}
			if meta == nil {
				t.Errorf("Decode(%q body) returned a nil *Metadata", body)
			}
		})
	}
}

// TestIsNoRepresentationBody pins the predicate's boundary: JSON `null` and
// whitespace count as no representation; anything else — including the
// string "null", a bare zero, an empty object or array — is a body to decode.
func TestIsNoRepresentationBody(t *testing.T) { // REQ-151, REQ-094
	cases := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"   ", true},
		{"\n\t", true},
		{"null", true},
		{"\tnull\n", true},
		{"{}", false},
		{"[]", false},
		{"nul", false},
		{"nullx", false},
		{`"null"`, false},
		{"0", false},
	}
	for _, tc := range cases {
		if got := transport.IsNoRepresentationBody([]byte(tc.in)); got != tc.want {
			t.Errorf("IsNoRepresentationBody(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

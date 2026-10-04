package transport_test

// reauth_challenge_test.go — REQ-166 as a consumer sees it: the Bearer
// challenge on the *transport.WireError of a real 401 or 403, the value-free
// Error() text beside it, and the opt-in 401 safety net that reads it
// (REQ-063). External, like decode_error_test.go, so every assertion goes
// through the exported surface; the parser's own table lives in
// challenge_test.go.

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestWireErrorCarriesBearerChallenge — REQ-166: a 401 or 403 carrying a
// Bearer challenge exposes it as WireError.Challenge, from any header line;
// without a parseable Bearer challenge, or on another status, Challenge stays
// nil, and the status still maps to its sentinel either way.
func TestWireErrorCarriesBearerChallenge(t *testing.T) { // REQ-166
	cases := []struct {
		name     string
		status   int
		lines    []string
		sentinel error
		want     *transport.BearerChallenge
	}{
		{
			name:     "401 insufficient_scope",
			status:   http.StatusUnauthorized,
			lines:    []string{`Bearer realm="cdr", error="insufficient_scope", scope="patient/composition-*.r"`},
			sentinel: transport.ErrUnauthorized,
			want:     &transport.BearerChallenge{Realm: "cdr", Error: "insufficient_scope", Scope: "patient/composition-*.r"},
		},
		{
			name:     "403 insufficient_scope",
			status:   http.StatusForbidden,
			lines:    []string{`Bearer error="insufficient_scope", scope="patient/ehr.r"`},
			sentinel: transport.ErrForbidden,
			want:     &transport.BearerChallenge{Error: "insufficient_scope", Scope: "patient/ehr.r"},
		},
		{
			name:     "401 Bearer on the second header line",
			status:   http.StatusUnauthorized,
			lines:    []string{`Basic realm="b"`, `bearer error="invalid_token", resource_metadata="https://rs.example/meta"`},
			sentinel: transport.ErrUnauthorized,
			want: &transport.BearerChallenge{Error: "invalid_token", Params: map[string]string{
				"resource_metadata": "https://rs.example/meta",
			}},
		},
		{
			name:     "401 without a header",
			status:   http.StatusUnauthorized,
			sentinel: transport.ErrUnauthorized,
		},
		{
			name:     "401 with an unparseable Bearer challenge",
			status:   http.StatusUnauthorized,
			lines:    []string{`Bearer error="insufficient_scope`},
			sentinel: transport.ErrUnauthorized,
		},
		{
			name:     "403 with a non-Bearer challenge only",
			status:   http.StatusForbidden,
			lines:    []string{`DPoP error="invalid_token", algs="ES256"`},
			sentinel: transport.ErrForbidden,
		},
		{
			name:     "404 with a Bearer challenge",
			status:   http.StatusNotFound,
			lines:    []string{`Bearer error="invalid_token"`},
			sentinel: transport.ErrNotFound,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newDecodeClient(t, tc.status, "", http.Header{"Www-Authenticate": tc.lines})

			_, err := c.Do(t.Context(), &transport.Request{Path: "/x"})
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("Do() error = %v, want errors.Is %v", err, tc.sentinel)
			}
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil {
				t.Fatalf("errors.AsType[*transport.WireError] did not match %T (%v)", err, err)
			}
			if !reflect.DeepEqual(we.Challenge, tc.want) {
				t.Errorf("WireError.Challenge = %+v, want %+v", we.Challenge, tc.want)
			}
		})
	}
}

// TestWireErrorErrorOmitsChallenge — REQ-166, REQ-093: Error() names no
// challenge value, while errors.AsType still reaches every one of them.
func TestWireErrorErrorOmitsChallenge(t *testing.T) { // REQ-166
	values := map[string]string{
		"realm":             "realm-7c1e",
		"error":             "insufficient_scope",
		"error_description": "token-for-patient-4411-lacks-scope",
		"error_uri":         "https://as.example/err-9d2a",
		"scope":             "patient/composition-5b8f.r",
		"resource_metadata": "https://rs.example/meta-0e6c",
	}
	var params []string
	for name, v := range values {
		params = append(params, name+`="`+v+`"`)
	}
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c := newDecodeClient(t, status, "", http.Header{"Www-Authenticate": {"Bearer " + strings.Join(params, ", ")}})

			_, err := c.Do(t.Context(), &transport.Request{Path: "/x"})
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil {
				t.Fatalf("errors.AsType[*transport.WireError] did not match %T (%v)", err, err)
			}
			if we.Challenge == nil {
				t.Fatal("WireError.Challenge = nil; the premise of this test is gone")
			}
			for name, v := range values {
				if strings.Contains(err.Error(), v) {
					t.Errorf("Error() = %q contains the %s value %q; challenge values stay out of the error text", err.Error(), name, v)
				}
			}
			if we.Challenge.ErrorDescription != values["error_description"] || we.Challenge.Params["resource_metadata"] != values["resource_metadata"] {
				t.Errorf("WireError.Challenge = %+v, want the values reachable through errors.AsType", we.Challenge)
			}
		})
	}
}

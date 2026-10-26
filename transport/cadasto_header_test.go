package transport_test

// cadasto_header_test.go — REQ-050, the Cadasto spec-version header: a client
// sends Cadasto-OpenEhr-Spec-Version only when it was built with the opt-in.

import (
	"net/http"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestCadastoSpecVersionHeaderOnlyWhenOptedIn pins the sentence "The SDK MUST
// keep it off by default and send it only from a client built with
// transport.WithCadastoSpecVersionHeader(true)". Setting the version with
// transport.WithSpecVersion is not enough: a client built without the opt-in,
// or with the opt-in turned off, sends no such header, and neither does an
// opted-in client with no version to send. The opted-in client with a version
// is the control that shows the server sees the header when it is sent.
func TestCadastoSpecVersionHeaderOnlyWhenOptedIn(t *testing.T) { // REQ-050
	const header = "Cadasto-OpenEhr-Spec-Version"
	cases := []struct {
		name string
		opts []transport.Option
		// want is every value of the header the server saw; none means the
		// request carried no such header.
		want []string
	}{
		{
			name: "spec version without the opt-in",
			opts: []transport.Option{transport.WithSpecVersion("1.1.0")},
		},
		{
			name: "spec version with the opt-in off",
			opts: []transport.Option{transport.WithSpecVersion("1.1.0"), transport.WithCadastoSpecVersionHeader(false)},
		},
		{
			name: "the opt-in on without a spec version",
			opts: []transport.Option{transport.WithCadastoSpecVersionHeader(true)},
		},
		{
			name: "spec version with the opt-in on",
			opts: []transport.Option{transport.WithSpecVersion("1.1.0"), transport.WithCadastoSpecVersionHeader(true)},
			want: []string{"1.1.0"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen http.Header
			c := newBodyReadClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = r.Header.Clone()
				_, _ = w.Write([]byte(`{}`))
			}), tc.opts...)

			if _, err := c.Do(t.Context(), &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"}); err != nil {
				t.Fatalf("Do error = %v, want nil; the premise of this test is gone", err)
			}
			if got := seen.Values(header); !slices.Equal(got, tc.want) {
				t.Errorf("the server saw %s = %q, want %q", header, got, tc.want)
			}
		})
	}
}

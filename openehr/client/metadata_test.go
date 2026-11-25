package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestNewVersionMetadataReadsALaterWellFormedETag pins REQ-054 when a
// response carries more than one ETag. The first value stays on
// Metadata.ETag. VersionUID is the first well-formed version id among
// those values, so the earlier of two well-formed ids wins, and the
// Location tail only when none of them is.
func TestNewVersionMetadataReadsALaterWellFormedETag(t *testing.T) { // REQ-054
	const (
		opaque       = "33a64df551425fcc55e4d42a148795d9f25f89d4"
		wellFormed   = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example::2"
		laterWell    = "22222222-2222-4222-8222-222222222222::cdr.example::3"
		locationTail = "11111111-1111-4111-8111-111111111111::cdr.example::1"
	)
	location := "/ehr/e/composition/" + locationTail
	tests := []struct {
		name  string
		etags []string
		want  openehrclient.VersionUID
	}{
		{
			name:  "opaque first and a later well-formed ETag",
			etags: []string{opaque, wellFormed},
			want:  wellFormed,
		},
		{
			name:  "two well-formed ETags",
			etags: []string{wellFormed, laterWell},
			want:  wellFormed,
		},
		{
			name:  "only an opaque ETag",
			etags: []string{opaque},
			want:  locationTail,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for _, etag := range tc.etags {
					w.Header().Add("ETag", `"`+etag+`"`)
				}
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(srv.Close)
			c := newClient(t, srv)
			resp, err := c.Do(t.Context(), &transport.Request{
				Method: http.MethodGet,
				Path:   "/etag",
				Route:  "/etag",
			})
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			meta := openehrclient.NewVersionMetadata(resp.Metadata)
			if meta == nil {
				t.Fatal("NewVersionMetadata returned nil")
			}
			if meta.VersionUID != tc.want {
				t.Errorf("NewVersionMetadata(ETags=%q, Location=%q).VersionUID = %q, want %q",
					tc.etags, location, meta.VersionUID, tc.want)
			}
			if meta.ETag != tc.etags[0] {
				t.Errorf("Metadata.ETag = %q, want the first ETag %q", meta.ETag, tc.etags[0])
			}
		})
	}
}

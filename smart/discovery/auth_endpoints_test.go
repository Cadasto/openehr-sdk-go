package discovery_test

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestREQ070_AuthEndpointsCloneSharesNothing pins that a clone shares no URL,
// user information or list with the endpoints it was made from. Every field
// is set, the clone is checked equal to the original, and then one side is
// edited in place: each URL, its user information and each list element. The
// other side must stay as it was, whichever side is edited. A nil URL, a nil
// user information and a nil list stay nil in the clone.
func TestREQ070_AuthEndpointsCloneSharesNothing(t *testing.T) { // REQ-070
	tests := []struct {
		name string
		// sides returns the endpoints the case edits, then the endpoints
		// the edit must leave as they were.
		sides func(orig, clone discovery.AuthEndpoints) (edited, kept discovery.AuthEndpoints)
	}{
		{
			name: "clone edited",
			sides: func(orig, clone discovery.AuthEndpoints) (discovery.AuthEndpoints, discovery.AuthEndpoints) {
				return clone, orig
			},
		},
		{
			name: "original edited",
			sides: func(orig, clone discovery.AuthEndpoints) (discovery.AuthEndpoints, discovery.AuthEndpoints) {
				return orig, clone
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := fullAuthEndpoints(t)
			clone := orig.Clone()
			if want := fullAuthEndpoints(t); !reflect.DeepEqual(clone, want) {
				t.Fatalf("Clone() = %+v, want the endpoints it was called on %+v", clone, want)
			}
			edited, kept := tc.sides(orig, clone)
			editAuthEndpoints(edited)
			if want := fullAuthEndpoints(t); !reflect.DeepEqual(kept, want) {
				t.Errorf("Clone(), then an edit to one side: the other side's endpoints changed:\n got %+v\nwant %+v", kept, want)
			}
		})
	}

	t.Run("nil stays nil", func(t *testing.T) {
		// One URL without user information; every other URL and every list
		// is nil.
		ep := discovery.AuthEndpoints{TokenEndpoint: &url.URL{Scheme: "https", Host: "idp.test", Path: "/token"}}
		if got := ep.Clone(); !reflect.DeepEqual(got, ep) {
			t.Errorf("Clone() of %+v = %+v, want every nil URL, user information and list kept nil", ep, got)
		}
	})
}

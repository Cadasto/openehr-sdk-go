package rm_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestREQ120_DVEHRURIFunctionsOnDecodedLinkTarget is the REQ-120 check that
// a LINK target decoded from canonical JSON answers DV_URI's functions, which
// DV_EHR_URI inherits, with the components of its value.
func TestREQ120_DVEHRURIFunctionsOnDecodedLinkTarget(t *testing.T) {
	const doc = `{"_type":"LINK",` +
		`"meaning":{"_type":"DV_TEXT","value":"follow up"},` +
		`"type":{"_type":"DV_TEXT","value":"problem"},` +
		`"target":{"_type":"DV_EHR_URI","value":"ehr://sys.example.org/7d44b88c-4199-4bad-97dc-d78268e01398/compositions?v=2#content"}}`
	var link rm.Link
	if err := canjson.Unmarshal([]byte(doc), &link); err != nil {
		t.Fatalf("canjson.Unmarshal(LINK) = %v", err)
	}
	target := link.Target
	for _, c := range []struct{ fn, got, want string }{
		{"Scheme", target.Scheme(), "ehr"},
		{"Path", target.Path(), "/7d44b88c-4199-4bad-97dc-d78268e01398/compositions"},
		{"Query", target.Query(), "v=2"},
		{"FragmentID", target.FragmentID(), "content"},
	} {
		if c.got != c.want {
			t.Errorf("LINK target %q: %s() = %q, want %q", target.Value, c.fn, c.got, c.want)
		}
	}
}

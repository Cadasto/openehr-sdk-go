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

// TestREQ120_DVURIComponents is the REQ-120 check that DV_URI's functions
// split a URI reference into its RFC 3986 components, each as written and
// empty when the reference has none. The scheme is held to the RFC 3986
// grammar, so text before a colon that is no scheme stays in the path, and
// the path leaves out the authority.
func TestREQ120_DVURIComponents(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, value                     string
		scheme, path, query, fragmentID string
	}{
		{
			name: "every component", value: "https://example.org:8443/a/b?x=1&y=2#sec-3",
			scheme: "https", path: "/a/b", query: "x=1&y=2", fragmentID: "sec-3",
		},
		{name: "no query", value: "https://example.org/a#f", scheme: "https", path: "/a", fragmentID: "f"},
		{name: "no fragment", value: "https://example.org/a?q", scheme: "https", path: "/a", query: "q"},
		{name: "no path", value: "https://example.org?q#f", scheme: "https", query: "q", fragmentID: "f"},
		{name: "an authority alone", value: "ehr://sys.example.org", scheme: "ehr"},
		{name: "an empty authority", value: "file:///etc/hosts", scheme: "file", path: "/etc/hosts"},
		{name: "no authority", value: "ehr:/7d44/compositions", scheme: "ehr", path: "/7d44/compositions"},
		{name: "a rootless path", value: "mailto:a@example.org", scheme: "mailto", path: "a@example.org"},
		{name: "a colon in the path", value: "urn:ehr:x", scheme: "urn", path: "ehr:x"},
		{name: "a scheme alone", value: "ehr:", scheme: "ehr"},
		{name: "a scheme in capitals", value: "EHR:/x", scheme: "EHR", path: "/x"},
		{name: "a scheme with digits and signs", value: "a1.b-c+d:x", scheme: "a1.b-c+d", path: "x"},
		{name: "a fragment holding ? and #", value: "x:/p#a?b#c", scheme: "x", path: "/p", fragmentID: "a?b#c"},
		{name: "a query holding ? and :", value: "x:/p?a?b:c", scheme: "x", path: "/p", query: "a?b:c"},
		{name: "empty query and fragment", value: "x:/p?#", scheme: "x", path: "/p"},
		{name: "a plain-text URI, not decoded", value: "file:///my docs/a%20b.txt", scheme: "file", path: "/my docs/a%20b.txt"},
		{name: "a network-path reference", value: "//example.org/x?q", path: "/x", query: "q"},
		{name: "a relative path", value: "a/b?q#f", path: "a/b", query: "q", fragmentID: "f"},
		{name: "a colon after a slash", value: "/x:y", path: "/x:y"},
		{name: "a colon after a query mark", value: "a?b:c", path: "a", query: "b:c"},
		{name: "a colon after a fragment mark", value: "a#b:c", path: "a", fragmentID: "b:c"},
		{name: "a scheme starting with a digit", value: "1http://x/y", path: "1http://x/y"},
		{name: "an empty scheme", value: ":x", path: ":x"},
		{name: "a space in the scheme", value: "x y:z", path: "x y:z"},
		{name: "no colon", value: "ehr", path: "ehr"},
		{name: "a fragment alone", value: "#f", fragmentID: "f"},
		{name: "empty", value: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uri := rm.DVURI{Value: tc.value}
			ehr := rm.DVEHRURI{Value: tc.value}
			for _, u := range []struct {
				kind                            string
				scheme, path, query, fragmentID string
			}{
				{"DV_URI", uri.Scheme(), uri.Path(), uri.Query(), uri.FragmentID()},
				{"DV_EHR_URI", ehr.Scheme(), ehr.Path(), ehr.Query(), ehr.FragmentID()},
			} {
				for _, c := range []struct{ fn, got, want string }{
					{"Scheme", u.scheme, tc.scheme},
					{"Path", u.path, tc.path},
					{"Query", u.query, tc.query},
					{"FragmentID", u.fragmentID, tc.fragmentID},
				} {
					if c.got != c.want {
						t.Errorf("%s %q: %s() = %q, want %q", u.kind, tc.value, c.fn, c.got, c.want)
					}
				}
			}
		})
	}
}

// TestREQ120_DVURINilReceiver is the REQ-120 check that DV_URI's functions
// answer a nil receiver with the empty string rather than a panic.
func TestREQ120_DVURINilReceiver(t *testing.T) {
	t.Parallel()
	var u *rm.DVURI
	for _, c := range []struct{ fn, got string }{
		{"Scheme", u.Scheme()},
		{"Path", u.Path()},
		{"Query", u.Query()},
		{"FragmentID", u.FragmentID()},
	} {
		if c.got != "" {
			t.Errorf("(*DVURI)(nil).%s() = %q, want \"\"", c.fn, c.got)
		}
	}
}

package discovery_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// documentCap is the most bytes of a document the resolver reads.
const documentCap = 1 << 20

// paddedToSize is doc with spaces after it, size bytes in all. JSON allows
// whitespace after the value, so the result parses as doc does. The stub
// server calls it, so it reports a doc too long for size with t.Errorf,
// which any goroutine may call.
func paddedToSize(t *testing.T, doc string, size int) string {
	t.Helper()
	if len(doc) > size {
		t.Errorf("document of %d bytes does not fit in %d", len(doc), size)
		return doc
	}
	return doc + strings.Repeat(" ", size-len(doc))
}

// stuffedToSize is doc, a JSON object, with a first member whose string
// value makes it size bytes in all, so cutting it short leaves JSON that
// does not parse. Like paddedToSize, it reports with t.Errorf.
func stuffedToSize(t *testing.T, doc string, size int) string {
	t.Helper()
	head, tail := `{"x_padding":"`, `",`+doc[1:]
	if len(head)+len(tail) > size {
		t.Errorf("document of %d bytes does not fit in %d", len(head)+len(tail), size)
		return doc
	}
	return head + strings.Repeat("x", size-len(head)-len(tail)) + tail
}

// TestREQ072_DocumentOverTheCapIsRefusedForItsSize pins the resolver's cap
// on a document it reads: a SMART configuration or an issuer's OpenID
// configuration of exactly 1048576 bytes is read whole, and one byte more
// fails with an error that says the document exceeds 1048576 bytes, not
// that its JSON ended early. The reasons stay those an unparsable document
// of each kind gets: ReasonParseError for the SMART configuration and
// ReasonFetchFailed for the OpenID configuration.
func TestREQ072_DocumentOverTheCapIsRefusedForItsSize(t *testing.T) { // REQ-072, REQ-073
	tests := []struct {
		name       string
		smart      func(t *testing.T, origin string) string
		openID     func(t *testing.T, origin string) string
		wantReason discovery.DiscoveryErrorReason // empty means success
		wantText   string
	}{
		{
			name:  "SMART configuration at the cap",
			smart: func(t *testing.T, _ string) string { return paddedToSize(t, smartDocument("", ""), documentCap) },
		},
		{
			name:       "SMART configuration over the cap",
			smart:      func(t *testing.T, _ string) string { return stuffedToSize(t, smartDocument("", ""), documentCap+1) },
			wantReason: discovery.ReasonParseError,
			wantText:   "SMART configuration exceeds 1048576 bytes",
		},
		{
			name:  "OpenID configuration at the cap",
			smart: func(_ *testing.T, origin string) string { return smartDocument(origin+idpPath, "") },
			openID: func(t *testing.T, origin string) string {
				return paddedToSize(t, openIDDocument(origin+idpPath, ""), documentCap)
			},
		},
		{
			name:  "OpenID configuration over the cap",
			smart: func(_ *testing.T, origin string) string { return smartDocument(origin+idpPath, "") },
			openID: func(t *testing.T, origin string) string {
				return stuffedToSize(t, openIDDocument(origin+idpPath, ""), documentCap+1)
			},
			wantReason: discovery.ReasonFetchFailed,
			wantText:   "openid-configuration exceeds 1048576 bytes",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			openID := documentFunc(notFound)
			if tc.openID != nil {
				openID = serve(func(origin string) string { return tc.openID(t, origin) })
			}
			p := startPlatform(t, false, serve(func(origin string) string { return tc.smart(t, origin) }), openID)
			_, err := p.resolver(t).Resolve(t.Context(), p.baseURL())
			if tc.wantReason == "" {
				if err != nil {
					t.Fatalf("Resolve(%q) = %v, want the document of exactly %d bytes read whole", p.baseURL(), err, documentCap)
				}
				return
			}
			de, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || de == nil || de.Reason != tc.wantReason {
				t.Fatalf("Resolve(%q) error = %v, want a DiscoveryError with Reason %q", p.baseURL(), err, tc.wantReason)
			}
			if !strings.Contains(err.Error(), tc.wantText) || strings.Contains(err.Error(), "unexpected end of JSON input") {
				t.Errorf("Resolve(%q) error = %q, want it to say %q", p.baseURL(), err, tc.wantText)
			}
		})
	}
}

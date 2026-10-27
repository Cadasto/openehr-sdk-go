package transport_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestREQ060_ClientPrintsNoStaticToken pins REQ-060 (auth.md § REQ-060): a
// *transport.Client keeps its token source in an unexported field, which fmt
// prints without calling the source's methods. Printing the client with %v,
// %+v or %#v still does not show the token of the source
// auth.StaticTokenSource returns.
func TestREQ060_ClientPrintsNoStaticToken(t *testing.T) { // REQ-060
	t.Parallel()
	const secret = "STATIC-TOKEN-s3cr3t"
	c, err := transport.New(mustSandboxCatalog(),
		transport.WithHTTPClient(&http.Client{}),
		transport.WithTokenSource(auth.StaticTokenSource(auth.Token{Value: secret, Type: auth.TokenTypeBearer})))
	if err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"%v", "%+v", "%#v"} {
		if out := fmt.Sprintf(verb, c); strings.Contains(out, secret) {
			t.Errorf("fmt.Sprintf(%q) of a client shows its static token: %s", verb, out)
		}
	}
}

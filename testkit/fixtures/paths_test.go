package fixtures_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The deprecated name must keep resolving the same tree, so a caller that
// has not moved to CorpusRoot yet still finds the fixtures.
func TestCassettesRootMatchesCorpusRoot(t *testing.T) {
	if got, want := fixtures.CassettesRoot(), fixtures.CorpusRoot(); got != want {
		t.Errorf("CassettesRoot() = %q, want CorpusRoot() = %q", got, want)
	}
}
